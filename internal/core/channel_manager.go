package core

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

type ChannelStatus struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Protocol        string                `json:"protocol"`
	Status          string                `json:"status"`
	Enable          bool                  `json:"enable"`
	DeviceCount     int                   `json:"device_count"`
	OnlineCount     int                   `json:"online_count"`
	OfflineCount    int                   `json:"offline_count"`
	QualityScore    int                   `json:"qualityScore"`      // 质量评分
	SuccessRate     float64               `json:"successRate"`       // 成功率
	LastCollectTime string                `json:"last_collect_time"` // 最后采集时间
	Metrics         *model.ChannelMetrics `json:"metrics,omitempty"` // 详细指标
}

// ChannelManager 管理所有采集通道及其下的设备
type ChannelManager struct {
	channels              map[string]*model.Channel // channel.id -> channel
	drivers               map[string]drv.Driver     // channel.id -> driver
	driverMus             map[string]*sync.Mutex    // channel.id -> mutex for driver access
	pipeline              *DataPipeline
	stateManager          *CommunicationManageTemplate
	deviceAdapterManager  *DeviceAdapterManager
	protocolRegistry      *ProtocolAdapterRegistry
	scanEngineAdapter     *ScanEngineAdapter
	shadowCore            *ShadowCore
	mu                    sync.RWMutex
	ctx                   context.Context
	cancel                context.CancelFunc
	saveFunc              func([]model.Channel) error
	statusHandler         func(deviceID string, status int)
	topologyChangeHandler func()
	topologyDebounceMu    sync.Mutex
	topologyDebounceTimer *time.Timer
	tagRegistry           *TagRegistry
	pointDegradation      *PointDegradationManager
	soakMonitor           *SoakMonitor
	jobs                  *AsyncJobManager
}

func NewChannelManager(pipeline *DataPipeline, saveFunc func([]model.Channel) error) *ChannelManager {
	ctx, cancel := context.WithCancel(context.Background())
	deviceAdapterManager := NewDeviceAdapterManager()
	protocolRegistry := NewProtocolAdapterRegistry()

	scanEngine := NewScanEngine(ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       32,
		MaxQueueSize:      10000,
		AntiStarvationSec: 300,
		GoroutineLimit:    2048,
		ConnectionLimit:   500,
	})
	cm := &ChannelManager{
		channels:             make(map[string]*model.Channel),
		drivers:              make(map[string]drv.Driver),
		driverMus:            make(map[string]*sync.Mutex),
		pipeline:             pipeline,
		stateManager:         NewCommunicationManageTemplate(),
		deviceAdapterManager: deviceAdapterManager,
		protocolRegistry:     protocolRegistry,
		scanEngineAdapter:    NewScanEngineAdapter(scanEngine),
		ctx:                  ctx,
		cancel:               cancel,
		saveFunc:             saveFunc,
		tagRegistry:          NewTagRegistry(),
		pointDegradation:     NewPointDegradationManager(),
		jobs:                 NewAsyncJobManager(),
	}

	scanEngine.SetCollectFinalize(cm.finalizeScanCollect)
	cm.scanEngineAdapter.scanEngine.SetPointDegradation(cm.pointDegradation)
	cm.scanEngineAdapter.scanEngine.SetIOProfileProvider(cm.deviceIOProfile)
	cm.scanEngineAdapter.scanEngine.SetCircuitBreakerEventHandler(cm.recordCircuitBreakerEvent)
	cm.soakMonitor = NewSoakMonitor(cm)

	// Wire state manager events
	cm.stateManager.OnStateChange = func(deviceID string, oldState, newState NodeState) {
		cm.mu.RLock()
		handler := cm.statusHandler
		cm.mu.RUnlock()
		if handler != nil {
			handler(deviceID, int(newState))
		}
	}
	cm.soakMonitor.Start()

	return cm
}

func (cm *ChannelManager) SetShadowCore(sc *ShadowCore) {
	cm.mu.Lock()
	cm.shadowCore = sc
	cm.mu.Unlock()
	cm.scanEngineAdapter.scanEngine.SetShadowCore(sc)
}

func (cm *ChannelManager) SetShadowIngress(si *ShadowIngress) {
	if si == nil {
		return
	}
	cm.mu.Lock()
	cm.shadowCore = si.shadowCore
	cm.mu.Unlock()
	cm.scanEngineAdapter.scanEngine.SetShadowIngress(si)
}

func (cm *ChannelManager) deviceIOProfile(deviceID string) DeviceIOProfile {
	defaultProfile := DeviceIOProfile{Gap: 64, BatchSize: 120}
	cm.mu.RLock()
	sc := cm.shadowCore
	cm.mu.RUnlock()
	if sc == nil {
		return defaultProfile
	}
	opt := sc.GetDeviceOptimization(deviceID)
	if opt == nil {
		return defaultProfile
	}
	profile := defaultProfile
	if g, ok := opt["gap"].(int); ok && g > 0 {
		profile.Gap = g
	}
	if mtu, ok := opt["mtu"].(int); ok && mtu > 0 {
		batch := mtu / 4
		if batch < 16 {
			batch = 16
		}
		if batch > 125 {
			batch = 125
		}
		profile.BatchSize = batch
	}
	return profile
}

func (cm *ChannelManager) GetScanEngineMetricsSnapshot() map[string]any {
	se := cm.scanEngineAdapter.scanEngine
	if se == nil || se.GetMetrics() == nil {
		return map[string]any{}
	}
	snap := se.GetMetrics().Snapshot()
	snap["active_tasks"] = se.GetActiveTaskCount()
	snap["pending_tasks"] = se.GetPendingTaskCount()
	cb := se.GetCircuitBreaker()
	if cb != nil {
		cbSnap := cb.Snapshot()
		snap["driver_circuit_open_total"] = cbSnap["open_total"]
		snap["driver_circuit_reject_total"] = cbSnap["reject_total"]
		snap["driver_circuit_state"] = cbSnap["devices"]
		snap["circuit_breaker"] = cbSnap
	}
	if gc := se.GetGCMonitor(); gc != nil {
		for k, v := range gc.Metrics().Snapshot() {
			snap[k] = v
		}
	}
	snap["sla_warnings"] = se.GetMetrics().SLAWarnings(cb)
	for k, v := range se.OperationalSnapshot() {
		snap[k] = v
	}
	return snap
}

func (cm *ChannelManager) GetChannelScanEngineMetricsSnapshot(channelID string) map[string]any {
	se := cm.scanEngineAdapter.scanEngine
	if se == nil || se.GetMetrics() == nil || channelID == "" {
		return map[string]any{}
	}
	metrics := se.GetMetrics()
	snap := metrics.ChannelSnapshot(channelID)

	ch := cm.GetChannel(channelID)
	deviceKeys := make([]string, 0)
	openCount := 0
	cb := se.GetCircuitBreaker()
	if ch != nil {
		for _, dev := range ch.Devices {
			deviceKeys = append(deviceKeys, dev.ID)
			if cb != nil && cb.State(dev.ID) == CircuitOpen {
				openCount++
			}
		}
	}
	snap["circuit_breaker_open"] = openCount

	maxLag := float64(0)
	if p95, ok := snap["scan_lag_p95_ms"].(float64); ok {
		maxLag = p95
	}
	now := time.Now()
	for _, dev := range deviceKeys {
		for _, task := range se.GetTasksByDeviceKey(dev) {
			task.mu.RLock()
			lagMs := float64(0)
			if !task.NextRun.IsZero() && now.After(task.NextRun) {
				lagMs = float64(now.Sub(task.NextRun).Milliseconds())
			}
			task.mu.RUnlock()
			if lagMs > maxLag {
				maxLag = lagMs
			}
		}
	}
	if maxLag > 0 {
		snap["scan_lag_p95_ms"] = maxLag
	}

	snap["sla_warnings"] = metrics.ChannelSLAWarnings(channelID, cb, deviceKeys)
	return snap
}

func (cm *ChannelManager) GetDeviceDiagnostics(deviceID string) map[string]any {
	out := map[string]any{
		"device_id": deviceID,
	}
	cm.mu.RLock()
	sc := cm.shadowCore
	cm.mu.RUnlock()
	if sc != nil {
		out["io_profile"] = sc.GetDeviceOptimization(deviceID)
	}
	node := cm.stateManager.GetNode(deviceID)
	if node != nil {
		out["state"] = int(node.Runtime.State)
		total := node.Runtime.SuccessCount + node.Runtime.FailCount
		if total > 0 {
			out["success_rate"] = float64(node.Runtime.SuccessCount) / float64(total)
		}
		out["consecutive_failures"] = node.Runtime.FailCount
	}
	var pointIDs []string
	now := time.Now()
	scanTasks := make([]map[string]any, 0)
	for _, task := range cm.scanEngineAdapter.scanEngine.GetTasksByDeviceKey(deviceID) {
		pointIDs = append(pointIDs, task.PointIDs...)
		task.mu.RLock()
		lagMs := float64(0)
		if !task.NextRun.IsZero() && now.After(task.NextRun) {
			lagMs = float64(now.Sub(task.NextRun).Milliseconds())
		}
		degradeOnFailure := true
		if task.Params != nil {
			if v, ok := task.Params["degradeOnFailure"].(bool); ok {
				degradeOnFailure = v
			}
		}
		scanTasks = append(scanTasks, map[string]any{
			"task_id":              task.ID,
			"scan_class":           task.ScanClass,
			"interval_ms":          task.Interval.Milliseconds(),
			"base_interval_ms":     task.BaseInterval.Milliseconds(),
			"status":               task.Status.String(),
			"lag_ms":               lagMs,
			"consecutive_failures": task.ConsecutiveFailures,
			"point_count":          len(task.PointIDs),
			"degrade_on_failure":   degradeOnFailure,
		})
		task.mu.RUnlock()
	}
	if len(scanTasks) > 0 {
		out["scan_tasks"] = scanTasks
	}
	if cm.pointDegradation != nil && len(pointIDs) > 0 {
		out["point_degradation"] = cm.pointDegradation.SnapshotDevice(deviceID, pointIDs)
	}
	if se := cm.scanEngineAdapter.scanEngine; se != nil {
		if cb := se.GetCircuitBreaker(); cb != nil {
			out["circuit_breaker"] = cb.DeviceSnapshot(deviceID)
		}
	}
	return out
}

func (cm *ChannelManager) recordCircuitBreakerEvent(deviceKey, eventType, message string) {
	channelID := cm.channelIDForDevice(deviceKey)
	if channelID == "" {
		return
	}
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.RecordError(channelID, eventType, eventType, message)
	}
}

func (cm *ChannelManager) finalizeScanCollect(deviceID string, result *ExecuteResult) {
	channelID := cm.channelIDForDevice(deviceID)
	recordCollectCycle := func(success bool) {
		if channelID == "" {
			return
		}
		if mc := model.GetGlobalMetricsCollector(); mc != nil {
			mc.RecordCycle(channelID, success)
		}
	}

	if result != nil && isChannelLinkError(result.Error) {
		if channelID != "" {
			cm.mu.RLock()
			driver := cm.drivers[channelID]
			cm.mu.RUnlock()

			if isChannelLinkUp(driver) {
				node := cm.stateManager.GetNode(deviceID)
				if node != nil {
					ctx := &CollectContext{FailCmd: 1}
					cm.stateManager.FinalizeCollect(node, ctx)
				}
				recordCollectCycle(false)
				return
			}
			cm.markChannelDevicesOffline(channelID)
		}
		recordCollectCycle(false)
		return
	}

	node := cm.stateManager.GetNode(deviceID)
	if node == nil {
		return
	}

	pointCount := 0
	for _, task := range cm.scanEngineAdapter.scanEngine.GetTasksByDeviceKey(deviceID) {
		pointCount += len(task.PointIDs)
	}

	cm.stateManager.FinalizeCollect(node, collectContextFromExecuteResult(result, pointCount))
	recordCollectCycle(collectSucceededFromResult(result, pointCount))
}

func (cm *ChannelManager) SetStatusHandler(h func(deviceID string, status int)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.statusHandler = h
}

// SetTopologyChangeHandler registers a callback invoked when channels/devices/points change.
// Used to rebuild northbound OPC UA address space.
func (cm *ChannelManager) SetTopologyChangeHandler(h func()) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.topologyChangeHandler = h
}

func (cm *ChannelManager) notifyTopologyChange() {
	const debounce = 400 * time.Millisecond
	cm.topologyDebounceMu.Lock()
	defer cm.topologyDebounceMu.Unlock()
	if cm.topologyDebounceTimer != nil {
		cm.topologyDebounceTimer.Stop()
	}
	cm.topologyDebounceTimer = time.AfterFunc(debounce, func() {
		cm.mu.RLock()
		handler := cm.topologyChangeHandler
		cm.mu.RUnlock()
		if handler != nil {
			handler()
		}
	})
}

// parseTime 解析时间字符串
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func (cm *ChannelManager) GetChannelStats() []ChannelStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var stats []ChannelStatus
	for _, ch := range cm.channels {
		online := 0
		offline := 0
		lastCollectTime := ""

		for _, dev := range ch.Devices {
			node := cm.stateManager.GetNode(dev.ID)
			if node != nil && node.Runtime.State == NodeStateOnline {
				online++
				// 更新最后采集时间
				if node.Runtime.LastSuccess.After(time.Time{}) {
					if lastCollectTime == "" || node.Runtime.LastSuccess.After(parseTime(lastCollectTime)) {
						lastCollectTime = node.Runtime.LastSuccess.Format(time.RFC3339)
					}
				}
			} else {
				offline++
			}
		}

		status := "Disabled"
		qualityScore := 0
		if ch.Enable {
			linkUp := isChannelLinkUp(cm.drivers[ch.ID])
			if mc := model.GetGlobalMetricsCollector(); mc != nil {
				if metrics := mc.GetChannelMetrics(ch.ID); metrics != nil {
					qualityScore = metrics.QualityScore
				}
			}
			status = evaluateChannelStatus(linkUp, online, offline, len(ch.Devices), qualityScore)
		}

		stats = append(stats, ChannelStatus{
			ID:              ch.ID,
			Name:            ch.Name,
			Protocol:        ch.Protocol,
			Status:          status,
			Enable:          ch.Enable,
			DeviceCount:     len(ch.Devices),
			OnlineCount:     online,
			OfflineCount:    offline,
			QualityScore:    qualityScore,
			LastCollectTime: lastCollectTime,
		})
	}
	return stats
}

func (cm *ChannelManager) GetTagRegistry() *TagRegistry {
	return cm.tagRegistry
}

// AddChannel 添加一个采集通道
func (cm *ChannelManager) AddChannel(ch *model.Channel) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := model.EnsureChannelID(ch); err != nil {
		return err
	}

	if _, exists := cm.channels[ch.ID]; exists {
		return fmt.Errorf("channel %s already exists", ch.ID)
	}

	if ch.Protocol == "opc-ua" {
		model.NormalizeOpcUaChannelConfig(ch.Config)
	}
	if ch.Protocol == "modbus-tcp" || ch.Protocol == "modbus-rtu-over-tcp" {
		normalizeModbusChannelConfig(ch.Config)
	}

	// 格式化所有设备配置
	for i := range ch.Devices {
		sanitizeDeviceConfig(ch.Devices[i].Config)
		if (ch.Protocol == "modbus-tcp" || ch.Protocol == "modbus-rtu" || ch.Protocol == "modbus-rtu-over-tcp") && ch.Devices[i].Config != nil {
			if _, ok := ch.Devices[i].Config["auto_points_range"]; ok && len(ch.Devices[i].Points) == 0 {
				// 只有在设备的 Points 字段为空时，才自动生成点位配置
				cm.autoGenerateModbusPointsFromConfig(&ch.Devices[i])
			}
		}
		if ch.Protocol == "dlt645" && len(ch.Devices[i].Points) == 0 {
			cm.autoGenerateDLT645PointsFromConfig(&ch.Devices[i])
		}
	}

	// 初始化驱动
	d, ok := drv.GetDriver(ch.Protocol)
	if !ok {
		return fmt.Errorf("driver for protocol %s not found", ch.Protocol)
	}

	err := d.Init(model.DriverConfig{
		ChannelID: ch.ID,
		Config:    ch.Config,
	})
	if err != nil {
		return fmt.Errorf("failed to init driver: %v", err)
	}

	cm.channels[ch.ID] = ch
	for i := range ch.Devices {
		cm.tagRegistry.RegisterFromDevice(ch.ID, &ch.Devices[i])
	}
	cm.drivers[ch.ID] = d
	cm.driverMus[ch.ID] = &sync.Mutex{}
	cm.bindDriverLinkMutex(ch.ID, d)
	cm.wireBACnetAddressNotifier(ch.ID, d)
	cm.stateManager.RegisterNode(ch.ID, ch.Name)

	// Register all devices in state manager
	for _, dev := range ch.Devices {
		cm.stateManager.RegisterNode(dev.ID, dev.Name)
	}

	// Persist asynchronously — never block CRUD on bbolt I/O while holding cm.mu.
	_ = cm.saveChannels()

	//	zap.L().Info("Channel added", zap.String("channel", ch.Name), zap.String("protocol", ch.Protocol), zap.Int("device_count", len(ch.Devices)))
	cm.notifyTopologyChange()
	return nil
}

// UpdateChannel 更新采集通道
func (cm *ChannelManager) UpdateChannel(ch *model.Channel) error {
	// 1. Stop existing channel
	if err := cm.StopChannel(ch.ID); err != nil {
		// Ignore error if channel was not running or found (but we should check existence)
		zap.L().Warn("Stopping channel before update", zap.String("channel_id", ch.ID), zap.Error(err))
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ch.Protocol == "opc-ua" {
		model.NormalizeOpcUaChannelConfig(ch.Config)
	}
	if ch.Protocol == "modbus-tcp" || ch.Protocol == "modbus-rtu-over-tcp" {
		normalizeModbusChannelConfig(ch.Config)
	}

	// 格式化所有设备配置
	for i := range ch.Devices {
		sanitizeDeviceConfig(ch.Devices[i].Config)
	}

	// 2. Re-init driver with new config
	d, ok := drv.GetDriver(ch.Protocol)
	if !ok {
		return fmt.Errorf("driver for protocol %s not found", ch.Protocol)
	}
	err := d.Init(model.DriverConfig{
		ChannelID: ch.ID,
		Config:    ch.Config,
	})
	if err != nil {
		return fmt.Errorf("failed to init driver: %v", err)
	}

	// 3. Update map
	cm.channels[ch.ID] = ch
	for i := range ch.Devices {
		cm.tagRegistry.RegisterFromDevice(ch.ID, &ch.Devices[i])
	}
	cm.drivers[ch.ID] = d
	if _, ok := cm.driverMus[ch.ID]; !ok {
		cm.driverMus[ch.ID] = &sync.Mutex{}
	}
	cm.bindDriverLinkMutex(ch.ID, d)
	cm.wireBACnetAddressNotifier(ch.ID, d)

	// Register all devices in state manager
	for _, dev := range ch.Devices {
		cm.stateManager.RegisterNode(dev.ID, dev.Name)
	}

	// 4. Persist asynchronously
	_ = cm.saveChannels()

	zap.L().Info("Channel updated", zap.String("channel", ch.Name))
	cm.notifyTopologyChange()
	return nil
}

// RemoveChannel 删除采集通道，并级联清理该通道下的所有设备
func (cm *ChannelManager) RemoveChannel(channelID string) error {
	// 1. Stop channel (注销通道下所有设备的 ScanEngine 任务并断开驱动)
	_ = cm.StopChannel(channelID)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, exists := cm.channels[channelID]
	if !exists {
		return fmt.Errorf("channel not found")
	}

	// 2. 级联清理通道下所有设备的状态数据（shadow / tag / stateManager），避免删除通道后残留
	cm.stateManager.UnregisterNode(channelID)
	for _, dev := range ch.Devices {
		cm.stateManager.UnregisterNode(dev.ID)
		if sc := cm.shadowCore; sc != nil {
			// 设备可能从未建立影子状态，忽略"不存在"错误
			_ = sc.DeleteShadowDevice(dev.ID)
		}
		if tr := cm.tagRegistry; tr != nil {
			tr.UnregisterDevice(channelID, dev.ID)
		}
	}

	delete(cm.channels, channelID)
	delete(cm.drivers, channelID)
	delete(cm.driverMus, channelID)

	_ = cm.saveChannels()
	zap.L().Info("Channel removed",
		zap.String("channel_id", channelID),
		zap.Int("devices_removed", len(ch.Devices)),
	)
	cm.notifyTopologyChange()
	return nil
}

// bindDriverLinkMutex wires channelMu into ConnectionManager for shared-link drivers.
func (cm *ChannelManager) bindDriverLinkMutex(channelID string, d drv.Driver) {
	mu := cm.driverMus[channelID]
	if mu == nil || d == nil {
		return
	}
	if binder, ok := d.(drv.LinkMutexBinder); ok {
		binder.BindLinkMutex(mu)
	}
}

// registerDeviceToScanEngine 将设备注册到 ScanEngine（使用通道已连接的驱动与完整点位配置）。
func (cm *ChannelManager) registerDeviceToScanEngine(ch *model.Channel, dev *model.Device) error {
	interval, ok := cm.validateDeviceInterval(dev)
	if !ok {
		return nil
	}
	d, okDrv := cm.drivers[ch.ID]
	if !okDrv {
		return fmt.Errorf("driver not found for channel %s", ch.ID)
	}
	cm.registerProtocolToScanEngine(ch.Protocol)
	return cm.scanEngineAdapter.RegisterDevice(
		dev.ID,
		ch.Protocol,
		d,
		cm.driverMus[ch.ID],
		ch,
		dev,
		interval,
		5,
	)
}

// tryConnectChannel 尝试连接通道驱动（用于批量添加设备后尽快建立连接）。
func (cm *ChannelManager) tryConnectChannel(channelID string) {
	cm.mu.RLock()
	d, ok := cm.drivers[channelID]
	cm.mu.RUnlock()
	if !ok || d == nil {
		return
	}
	ctx, cancel := context.WithTimeout(cm.ctx, 10*time.Second)
	defer cancel()
	if err := d.Connect(ctx); err != nil {
		zap.L().Warn("Channel connect failed", zap.String("channel_id", channelID), zap.Error(err))
		cm.markChannelDevicesOffline(channelID)
	}
}

// StartChannel 启动一个采集通道
func (cm *ChannelManager) StartChannel(channelID string) error {
	cm.mu.RLock()
	chPtr, ok := cm.channels[channelID]
	d, okDrv := cm.drivers[channelID]
	var ch model.Channel
	if ok && chPtr != nil {
		ch = chPtr.DeepCopy()
	}
	cm.mu.RUnlock()

	if !ok || !okDrv {
		return fmt.Errorf("channel or driver not found")
	}

	if !ch.Enable {
		zap.L().Info("Channel is disabled, skipping connection", zap.String("channel", ch.Name))
		return nil
	}

	// Connect asynchronously — never dial on the API/request path. Offline
	// devices and backoff sleeps must not freeze StartChannel / UI.
	go func(driver drv.Driver, chID, chName string) {
		connectCtx, cancel := context.WithTimeout(cm.ctx, 10*time.Second)
		defer cancel()
		if err := driver.Connect(connectCtx); err != nil {
			cm.markChannelDevicesOffline(chID)
			zap.L().Error("Failed to connect driver for channel", zap.String("channel", chName), zap.Error(err))
			if sched, ok := driver.(drv.ReconnectScheduler); ok {
				sched.ScheduleReconnect(cm.ctx, 5*time.Minute)
			}
		}
	}(d, channelID, ch.Name)

	// 注册协议类型到ScanEngine
	cm.registerProtocolToScanEngine(ch.Protocol)

	// 为该通道下的每个设备注册到ScanEngine
	for i := range ch.Devices {
		dev := &ch.Devices[i]
		if !dev.Enable {
			zap.L().Info("Device is disabled, skipping", zap.String("device", dev.Name), zap.String("channel", ch.Name))
			continue
		}

		if err := cm.registerDeviceToScanEngine(&ch, dev); err != nil {
			zap.L().Error("Failed to register device to ScanEngine", zap.String("device", dev.Name), zap.Error(err))
		}
	}

	// 启动ScanEngine（仅第一次启动）
	cm.scanEngineAdapter.Start()

	return nil
}

// StopChannel 停止一个采集通道
func (cm *ChannelManager) StopChannel(channelID string) error {
	cm.mu.RLock()
	chPtr, ok := cm.channels[channelID]
	d, okDrv := cm.drivers[channelID]
	var ch model.Channel
	if ok && chPtr != nil {
		ch = chPtr.DeepCopy()
	}
	cm.mu.RUnlock()

	if !ok || !okDrv {
		return fmt.Errorf("channel or driver not found")
	}

	// 从 ScanEngine 注销所有设备
	for _, device := range ch.Devices {
		cm.scanEngineAdapter.UnregisterDevice(device.ID)
	}

	// 断开驱动连接
	d.Disconnect()

	return nil
}

// GetChannels 获取所有通道
func (cm *ChannelManager) GetChannels() []model.Channel {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	channels := make([]model.Channel, 0, len(cm.channels))
	for _, ch := range cm.channels {
		c := ch.DeepCopy()
		if node := cm.stateManager.GetNode(c.ID); node != nil {
			c.NodeRuntime = &model.NodeRuntime{
				FailCount:     node.Runtime.FailCount,
				SuccessCount:  node.Runtime.SuccessCount,
				LastFailTime:  node.Runtime.LastFailTime,
				NextRetryTime: node.Runtime.NextRetryTime,
				State:         int(node.Runtime.State),
			}
		}
		// Also update Device Runtime
		d := cm.drivers[c.ID]
		for i := range c.Devices {
			cm.applyDeviceRuntimeState(&c, d, &c.Devices[i])
		}

		channels = append(channels, c)
	}
	return channels
}

// GetStateManager 获取状态管理器
func (cm *ChannelManager) GetStateManager() *CommunicationManageTemplate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.stateManager
}

// GetDriver 获取通道的驱动实例
func (cm *ChannelManager) GetDriver(channelID string) drv.Driver {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.drivers[channelID]
}

func (cm *ChannelManager) GetAllPoints() []map[string]any {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var result []map[string]any
	for _, ch := range cm.channels {
		for _, dev := range ch.Devices {
			for _, p := range dev.Points {
				result = append(result, map[string]any{
					"channel_id":   ch.ID,
					"channel_name": ch.Name,
					"device_id":    dev.ID,
					"device_name":  dev.Name,
					"point_id":     p.ID,
					"point_name":   p.Name,
					"data_type":    p.DataType,
				})
			}
		}
	}
	return result
}

// GetChannel 获取指定通道
func (cm *ChannelManager) GetChannel(channelID string) *model.Channel {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if ch, ok := cm.channels[channelID]; ok {
		c := *ch
		// Deep-copy devices so callers cannot mutate stored Points/Config
		// through the shared slice/backing arrays.
		if ch.Devices != nil {
			devs := make([]model.Device, len(ch.Devices))
			for i, dev := range ch.Devices {
				devs[i] = dev.DeepCopy()
			}
			c.Devices = devs
		}
		if node := cm.stateManager.GetNode(c.ID); node != nil {
			c.NodeRuntime = &model.NodeRuntime{
				FailCount:     node.Runtime.FailCount,
				SuccessCount:  node.Runtime.SuccessCount,
				LastFailTime:  node.Runtime.LastFailTime,
				NextRetryTime: node.Runtime.NextRetryTime,
				State:         int(node.Runtime.State),
			}
		}
		return &c
	}
	return nil
}

// GetChannelDevices 获取指定通道的所有设备
func (cm *ChannelManager) GetChannelDevices(channelID string) []model.Device {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if ch, ok := cm.channels[channelID]; ok {
		d := cm.drivers[channelID]
		// Return deep copies with state populated so callers may not mutate
		// the stored device's Points/Config through the returned slice.
		devices := make([]model.Device, len(ch.Devices))
		for i, dev := range ch.Devices {
			devices[i] = dev.DeepCopy()
			cm.applyDeviceRuntimeState(ch, d, &devices[i])
			var metrics *model.DeviceMetrics
			if mc := model.GetGlobalMetricsCollector(); mc != nil {
				metrics = mc.GetDeviceMetrics(dev.ID)
			}
			devices[i].QualityScore = resolveDeviceQualityScore(&devices[i], metrics)
		}
		return devices
	}
	return nil
}

// GetDevice 获取指定通道下的指定设备
func (cm *ChannelManager) GetDevice(channelID, deviceID string) *model.Device {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if ch, ok := cm.channels[channelID]; ok {
		for i, dev := range ch.Devices {
			if dev.ID == deviceID {
				// Return a deep copy with state populated so callers may mutate
				// Points/Config without corrupting the stored channel.
				d := ch.Devices[i].DeepCopy()
				driver := cm.drivers[channelID]
				cm.applyDeviceRuntimeState(ch, driver, &d)
				var metrics *model.DeviceMetrics
				if mc := model.GetGlobalMetricsCollector(); mc != nil {
					metrics = mc.GetDeviceMetrics(d.ID)
				}
				d.QualityScore = resolveDeviceQualityScore(&d, metrics)
				return &d
			}
		}
	}
	return nil
}

// GetDevicePoints 获取指定设备的所有点位数据（优先 Shadow；缺失时返回 Uncertain 元数据，绝不在 API 路径 live 读）。
func (cm *ChannelManager) GetDevicePoints(channelID, deviceID string) ([]model.PointData, error) {
	cm.mu.RLock()

	ch, ok := cm.channels[channelID]
	if !ok {
		cm.mu.RUnlock()
		return nil, fmt.Errorf("channel not found")
	}

	var foundDev *model.Device
	for i := range ch.Devices {
		if ch.Devices[i].ID == deviceID {
			foundDev = &ch.Devices[i]
			break
		}
	}
	if foundDev == nil {
		cm.mu.RUnlock()
		return nil, fmt.Errorf("device not found")
	}

	pointsCopy := make([]model.Point, len(foundDev.Points))
	copy(pointsCopy, foundDev.Points)
	devCopy := *foundDev
	devCopy.Points = pointsCopy

	slaveIDVal := foundDev.Config["slave_id"]
	slaveID := uint8(0)
	if slaveIDVal != nil {
		switch val := slaveIDVal.(type) {
		case float64:
			slaveID = uint8(val)
		case int:
			slaveID = uint8(val)
		case int64:
			slaveID = uint8(val)
		case uint8:
			slaveID = val
		case string:
			if i, err := strconv.Atoi(val); err == nil {
				slaveID = uint8(i)
			}
		}
	}

	cm.mu.RUnlock()

	if len(pointsCopy) == 0 {
		return []model.PointData{}, nil
	}

	if cm.shadowCore != nil {
		if points, ok := cm.getDevicePointsFromShadow(&devCopy, slaveID, channelID); ok {
			return points, nil
		}
	}

	// Shadow miss: config metadata only — never live-read on REST/UI path.
	points := make([]model.PointData, 0, len(pointsCopy))
	now := time.Now()
	for _, point := range pointsCopy {
		points = append(points, model.PointData{
			ID:           point.ID,
			Name:         point.Name,
			SlaveID:      slaveID,
			RegisterType: point.RegisterType.String(),
			FunctionCode: point.FunctionCode,
			Address:      point.Address,
			DataType:     point.DataType,
			ParseType:    point.ParseType,
			Unit:         point.Unit,
			Timestamp:    now,
			Quality:      "Uncertain",
			Value:        nil,
			ReadWrite:    point.ReadWrite,
			Format:       point.Format,
			WordOrder:    point.WordOrder,
			Scale:        point.Scale,
			Offset:       point.Offset,
			ReadFormula:  point.ReadFormula,
			WriteFormula: point.WriteFormula,
			Group:        point.Group,
			ScanClass:    point.ScanClass,
			ReportMode:   point.ReportMode,
		})
	}
	return points, nil
}

// GetShadowPoint 从影子设备实时快照中读取单个点位数据。
// 供 OPC UA Server 的 ReadHandler 调用，使第三方客户端能按需获取实时值。
func (cm *ChannelManager) GetShadowPoint(channelID, deviceID, pointID string) (*model.ShadowPoint, error) {
	if cm.shadowCore == nil {
		return nil, fmt.Errorf("shadow core not initialized")
	}
	shadowID := fmt.Sprintf("shadow-%s", deviceID)
	if pt, err := cm.shadowCore.GetShadowPoint(shadowID, pointID); err == nil {
		return pt, nil
	}
	return cm.shadowCore.GetVirtualShadowPoint(deviceID, pointID)
}

func (cm *ChannelManager) getDevicePointsFromShadow(dev *model.Device, slaveID uint8, channelID string) ([]model.PointData, bool) {
	shadowID := fmt.Sprintf("shadow-%s", dev.ID)
	shadow, err := cm.shadowCore.GetShadowDevice(shadowID)
	if err != nil || shadow == nil || len(shadow.Points) == 0 {
		return nil, false
	}
	if channelID != "" && shadow.ChannelID != "" && shadow.ChannelID != channelID {
		return nil, false
	}

	points := make([]model.PointData, 0, len(dev.Points))
	for _, point := range dev.Points {
		collectedAt := time.Time{}
		updatedAt := time.Time{}
		pd := model.PointData{
			ID:           point.ID,
			Name:         point.Name,
			SlaveID:      slaveID,
			RegisterType: point.RegisterType.String(),
			FunctionCode: point.FunctionCode,
			Address:      point.Address,
			DataType:     point.DataType,
			ParseType:    point.ParseType,
			Unit:         point.Unit,
			Quality:      "Bad",
			Value:        nil,
			ReadWrite:    point.ReadWrite,
			Format:       point.Format,
			WordOrder:    point.WordOrder,
			Scale:        point.Scale,
			Offset:       point.Offset,
			ReadFormula:  point.ReadFormula,
			WriteFormula: point.WriteFormula,
			Group:        point.Group,
			ScanClass:    point.ScanClass,
			ReportMode:   point.ReportMode,
		}
		if sp, exists := shadow.Points[point.ID]; exists {
			pd.Value = sp.Value
			pd.Quality = sp.Quality
			collectedAt = sp.CollectedAt
			if collectedAt.IsZero() {
				collectedAt = sp.Timestamp
			}
			updatedAt = sp.UpdatedAt
			pd.Timestamp = collectedAt
			pd.CollectedAt = collectedAt
			pd.UpdatedAt = updatedAt
		}
		points = append(points, pd)
	}
	return points, true
}

// validateDeviceInterval 验证设备采集间隔
func (cm *ChannelManager) validateDeviceInterval(dev *model.Device) (time.Duration, bool) {
	// 检查设备是否为 nil
	if dev == nil {
		zap.L().Error("Device is nil in validateDeviceInterval")
		return 0, false
	}

	// 检查设备名称是否为空
	if dev.Name == "" {
		zap.L().Error("Device name is empty in validateDeviceInterval")
		return 0, false
	}

	// 检查设备采集间隔是否为正数
	if dev.Interval <= 0 {
		zap.L().Warn("Device interval must be positive", zap.String("device", dev.Name), zap.Duration("interval", time.Duration(dev.Interval)))
		return 0, false
	}

	// 转换为时间间隔
	interval := time.Duration(dev.Interval)

	// 确保 interval 至少为 1 纳秒
	if interval < time.Nanosecond {
		interval = time.Nanosecond
		zap.L().Warn("Device interval is too small, setting to 1ns", zap.String("device", dev.Name), zap.Duration("interval", interval))
	}

	return interval, true
}

// registerProtocolToScanEngine 注册协议类型到ScanEngine
func (cm *ChannelManager) registerProtocolToScanEngine(protocol string) {
	switch protocol {
	case "modbus-tcp", "modbus-rtu", "modbus-rtu-over-tcp", "dlt645", "omron-fins", "mitsubishi-slmp", "knxnet-ip", "snmp":
		cm.scanEngineAdapter.scanEngine.RegisterProtocol(protocol, ProtocolTypeSerial)
	case "opc-ua", "http", "rest", "mqtt", "bacnet-ip":
		cm.scanEngineAdapter.scanEngine.RegisterProtocol(protocol, ProtocolTypeParallel)
	case "s7", "ethernet-ip", "profinet-io", "iec60870-5-104", "ethercat":
		cm.scanEngineAdapter.scanEngine.RegisterProtocol(protocol, ProtocolTypeLimited)
	default:
		cm.scanEngineAdapter.scanEngine.RegisterProtocol(protocol, ProtocolTypeSerial)
	}
}

// validatePoint validates point configuration based on channel protocol
func (cm *ChannelManager) validatePoint(ch *model.Channel, point *model.Point) error {
	switch ch.Protocol {
	case "modbus-tcp", "modbus-rtu", "modbus-rtu-over-tcp":
		return cm.validateModbusPoint(point)
	case "bacnet-ip":
		return cm.validateBACnetPoint(point)
	case "s7":
		return cm.validateS7Point(point)
	case "dlt645":
		return cm.validateDLT645Point(point)
	case "ethernet-ip":
		return cm.validateEtherNetIPPoint(point)
	case "mitsubishi-slmp":
		return cm.validateMitsubishiPoint(point)
	case "omron-fins":
		return cm.validateOmronFinsPoint(point)
	case "knxnet-ip":
		return cm.validateKNXnetIPPoint(point)
	case "profinet-io":
		return cm.validateProfinetIOPoint(point)
	case "ethercat":
		return cm.validateEtherCATPoint(point)
	default:
		return nil
	}
}

func (cm *ChannelManager) validateOmronFinsPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("omron address cannot be empty")
	}
	// Basic regex for Omron FINS Address
	// Supports: D100, CIO1.2, W3.4, H4.15L, EM10.100
	re := regexp.MustCompile(`^(?i)(CIO|A|W|H|D|P|F|EM\d*)(\d+)(\.\d+)?([HL]|\.\d+[HL]?)?$`)
	if !re.MatchString(point.Address) {
		return fmt.Errorf("invalid omron address format: e.g. D100, W3.4, CIO1.2, EM10.100")
	}
	return nil
}

func (cm *ChannelManager) validateKNXnetIPPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("knxnet-ip address cannot be empty")
	}
	// main/middle/sub or main/sub, optional ,individual or ,bit
	re := regexp.MustCompile(`^\d+/\d+(/\d+)?(,\d+(\.\d+\.\d+)?(,\d+)?)?$`)
	if !re.MatchString(point.Address) {
		return fmt.Errorf("invalid knxnet-ip address format: e.g. 1/2/3 or 0/0/1,1.1.1,2")
	}
	return nil
}

func (cm *ChannelManager) validateMitsubishiPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("mitsubishi address cannot be empty")
	}
	// Basic check for AREA ADDRESS
	re := regexp.MustCompile(`^([A-Z]+)([0-9]+)`)
	if !re.MatchString(strings.ToUpper(point.Address)) {
		return fmt.Errorf("invalid mitsubishi address format: e.g. D100, M0, X10")
	}
	return nil
}

func (cm *ChannelManager) validateModbusPoint(point *model.Point) error {
	if _, err := strconv.Atoi(point.Address); err != nil {
		return fmt.Errorf("invalid modbus address '%s': must be an integer", point.Address)
	}
	switch point.DataType {
	case "int16", "uint16", "int32", "uint32", "float32", "float64", "bool":
		return nil
	default:
		return fmt.Errorf("invalid modbus datatype '%s'", point.DataType)
	}
}

func (cm *ChannelManager) validateBACnetPoint(point *model.Point) error {
	parts := strings.Split(point.Address, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid bacnet address '%s': format must be ObjectType:Instance", point.Address)
	}

	validTypes := map[string]bool{
		"AnalogInput": true, "AnalogOutput": true, "AnalogValue": true,
		"BinaryInput": true, "BinaryOutput": true, "BinaryValue": true,
		"MultiStateInput": true, "MultiStateOutput": true, "MultiStateValue": true,
	}
	if !validTypes[parts[0]] {
		return fmt.Errorf("invalid bacnet object type '%s'", parts[0])
	}

	if _, err := strconv.Atoi(parts[1]); err != nil {
		return fmt.Errorf("invalid bacnet instance '%s': must be an integer", parts[1])
	}
	return nil
}

func (cm *ChannelManager) validateS7Point(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("s7 address cannot be empty")
	}
	return nil
}

func (cm *ChannelManager) validateDLT645Point(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("dlt645 address cannot be empty")
	}
	// Basic format check: Address#DataID[#Extension]
	parts := strings.Split(point.Address, "#")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("invalid dlt645 address format: must be Address#DataID[#Extension]")
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("invalid dlt645 address format: must be Address#DataID")
	}
	return nil
}

func (cm *ChannelManager) validateEtherNetIPPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("ethernet/ip tag name cannot be empty")
	}
	return nil
}

func (cm *ChannelManager) validateProfinetIOPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("profinet-io address cannot be empty")
	}
	re := regexp.MustCompile(`^\d+:\d+:\d+(?:\.\d+)?(?:#(?:BE|LE|be|le))?$`)
	if !re.MatchString(point.Address) {
		return fmt.Errorf("invalid profinet-io address format: expected SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN], e.g. 3:1:0")
	}
	return nil
}

// validateEtherCATPoint validates EtherCAT point address format.
// Supports PDO: POSITION:Tx|Rx:OFFSET[.BIT][#ENDIAN]
// and SDO: POSITION:SDO:0xINDEX:0xSUBINDEX[#ENDIAN]
func (cm *ChannelManager) validateEtherCATPoint(point *model.Point) error {
	if point.Address == "" {
		return fmt.Errorf("ethercat address cannot be empty")
	}
	// PDO format: 1:Tx:0, 1:Tx:2.3, 2:Rx:4#LE
	// SDO format: 1:SDO:0x6041:0, 1:SDO:0x6064:0#BE
	re := regexp.MustCompile(`^\d+:(?:[Tt][Xx]|[Rr][Xx]|[01]):\d+(?:\.\d+)?(?:#(?:BE|LE|be|le))?$`)
	reSDO := regexp.MustCompile(`^\d+:[Ss][Dd][Oo]:0[xX][0-9A-Fa-f]+:\d+(?:#(?:BE|LE|be|le))?$`)
	if !re.MatchString(point.Address) && !reSDO.MatchString(point.Address) {
		return fmt.Errorf("invalid ethercat address format: expected POSITION:Tx|Rx:OFFSET[.BIT][#ENDIAN] or POSITION:SDO:0xINDEX:0xSUBINDEX[#ENDIAN]")
	}
	return nil
}

// WritePoint 写入指定通道下设备点位的值
func (cm *ChannelManager) WritePoint(channelID, deviceID, pointID string, value any) error {
	cm.mu.RLock()
	ch, ok := cm.channels[channelID]
	d, okDrv := cm.drivers[channelID]
	cm.mu.RUnlock()

	if !ok || !okDrv {
		return fmt.Errorf("channel not found")
	}

	dev := cm.GetDevice(channelID, deviceID)
	if dev == nil {
		return fmt.Errorf("device not found")
	}

	// 查找点位配置
	var targetPoint model.Point
	found := false
	for _, p := range dev.Points {
		if p.ID == pointID {
			targetPoint = p
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("point not found")
	}

	if !pointAllowsWrite(targetPoint.ReadWrite) {
		return fmt.Errorf("point %s is read-only", pointID)
	}

	// Ensure DeviceID is set
	targetPoint.DeviceID = dev.ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := cm.withDriverIO(channelID, ch.Protocol, func() error {
		if slaveID, ok := dev.Config["slave_id"]; ok {
			if slaveIDUint, ok := slaveID.(float64); ok {
				d.SetSlaveID(uint8(slaveIDUint))
			} else if slaveIDInt, ok := slaveID.(int); ok {
				d.SetSlaveID(uint8(slaveIDInt))
			}
		}
		config := buildDriverDeviceConfig(ch, dev.Config, map[string]any{
			"_internal_device_id": dev.ID,
		})
		d.SetDeviceConfig(config)
		return d.WritePoint(ctx, targetPoint, value)
	})
	if err != nil {
		return err
	}

	// Extract the actual written value from the map format (e.g. {"value": 3.14, "priority": 16})
	// used by BACnet and other protocols that support priority writes.
	// 从 map 格式中提取实际写入值（BACnet 等协议使用 {"value": x, "priority": n} 格式），
	// 避免将整个 map 发布到 ShadowCache 导致 UI 显示异常。
	actualValue := value
	if valMap, ok := value.(map[string]any); ok {
		if v, ok := valMap["value"]; ok {
			actualValue = v
		}
	}
	cm.publishWrittenValue(channelID, deviceID, pointID, actualValue)
	return nil
}

func pointAllowsWrite(readWrite string) bool {
	if readWrite == "" {
		return true
	}
	return strings.Contains(strings.ToUpper(readWrite), "W")
}

func (cm *ChannelManager) publishWrittenValue(channelID, deviceID, pointID string, value any) {
	now := time.Now()
	if cm.shadowCore != nil {
		msg := model.ShadowIngressMessage{
			DeviceID:  deviceID,
			ChannelID: channelID,
			Timestamp: now,
			Points: []model.ShadowIngressPoint{
				{
					PointID:     pointID,
					Value:       value,
					Quality:     "Good",
					CollectedAt: now,
				},
			},
			Meta: model.ShadowIngressMeta{Source: "write_point"},
		}
		if _, err := cm.shadowCore.WriteShadowDevice(msg); err != nil {
			zap.L().Warn("Failed to sync write to shadow",
				zap.String("device_id", deviceID),
				zap.String("point_id", pointID),
				zap.Error(err),
			)
		}
		return
	}

	if cm.pipeline != nil {
		cm.pipeline.Push(model.Value{
			ChannelID: channelID,
			DeviceID:  deviceID,
			PointID:   pointID,
			Value:     value,
			Quality:   "Good",
			TS:        now,
		})
	}
}

// ReadPoint 读取指定通道下设备点位的值
func (cm *ChannelManager) ReadPoint(channelID, deviceID, pointID string) (model.Value, error) {
	cm.mu.RLock()
	ch, ok := cm.channels[channelID]
	d, okDrv := cm.drivers[channelID]
	cm.mu.RUnlock()

	if !ok || !okDrv {
		return model.Value{}, fmt.Errorf("channel not found")
	}

	if !ch.Enable {
		return model.Value{}, fmt.Errorf("channel is disabled")
	}

	dev := cm.GetDevice(channelID, deviceID)
	if dev == nil {
		return model.Value{}, fmt.Errorf("device not found")
	}

	if !dev.Enable {
		return model.Value{}, fmt.Errorf("device is disabled")
	}

	// 查找点位配置
	var targetPoint model.Point
	found := false
	for _, p := range dev.Points {
		if p.ID == pointID {
			targetPoint = p
			found = true
			break
		}
	}
	if !found {
		return model.Value{}, fmt.Errorf("point not found")
	}

	// Ensure DeviceID is set
	targetPoint.DeviceID = dev.ID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var results map[string]model.Value
	err := cm.withDriverIO(channelID, ch.Protocol, func() error {
		if slaveID, ok := dev.Config["slave_id"]; ok {
			if slaveIDUint, ok := slaveID.(float64); ok {
				d.SetSlaveID(uint8(slaveIDUint))
			} else if slaveIDInt, ok := slaveID.(int); ok {
				d.SetSlaveID(uint8(slaveIDInt))
			}
		}
		config := buildDriverDeviceConfig(ch, dev.Config, map[string]any{
			"_internal_device_id": dev.ID,
		})
		d.SetDeviceConfig(config)
		var readErr error
		results, readErr = d.ReadPoints(ctx, []model.Point{targetPoint})
		return readErr
	})
	if err != nil {
		return model.Value{}, err
	}

	// Try finding by Name (most common)
	if v, ok := results[targetPoint.Name]; ok {
		return v, nil
	}
	// Try finding by ID
	if v, ok := results[targetPoint.ID]; ok {
		return v, nil
	}
	// Fallback: if single result, return it
	if len(results) == 1 {
		for _, v := range results {
			return v, nil
		}
	}

	return model.Value{}, fmt.Errorf("point value not returned")
}

// Shutdown 关闭所有通道
func (cm *ChannelManager) Shutdown() {
	cm.cancel()
	if cm.jobs != nil {
		cm.jobs.Stop()
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, ch := range cm.channels {
		for _, dev := range ch.Devices {
			cm.scanEngineAdapter.UnregisterDevice(dev.ID)
		}
	}

	for _, d := range cm.drivers {
		d.Disconnect()
	}
}

// Jobs returns the async job manager used for scan/browse APIs.
func (cm *ChannelManager) Jobs() *AsyncJobManager {
	return cm.jobs
}

func scanChannelTimeout(protocol string) time.Duration {
	switch protocol {
	case "bacnet-ip":
		return 45 * time.Second
	case "opc-ua":
		return 45 * time.Second
	default:
		return 45 * time.Second
	}
}

func scanDeviceTimeout(protocol string) time.Duration {
	switch protocol {
	case "opc-ua":
		return 180 * time.Second
	case "bacnet-ip":
		return 60 * time.Second
	default:
		return 45 * time.Second
	}
}

// StartScanChannelJob submits channel device discovery as an async job.
func (cm *ChannelManager) StartScanChannelJob(channelID string, params map[string]any) (*AsyncJob, error) {
	cm.mu.RLock()
	_, okDrv := cm.drivers[channelID]
	ch, okCh := cm.channels[channelID]
	cm.mu.RUnlock()
	if !okDrv {
		return nil, fmt.Errorf("channel driver not found")
	}
	protocol := ""
	if okCh {
		protocol = ch.Protocol
	}
	timeout := scanChannelTimeout(protocol)
	paramsCopy := copyScanParams(params)
	return cm.jobs.Submit(AsyncJobScanChannel, channelID, "", func(ctx context.Context) (any, error) {
		jobCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cm.ScanChannel(jobCtx, channelID, paramsCopy)
	}), nil
}

// StartScanDeviceJob submits device object/point browse as an async job.
func (cm *ChannelManager) StartScanDeviceJob(channelID, deviceID string, params map[string]any) (*AsyncJob, error) {
	cm.mu.RLock()
	_, okDrv := cm.drivers[channelID]
	ch, okCh := cm.channels[channelID]
	cm.mu.RUnlock()
	if !okDrv || !okCh {
		return nil, fmt.Errorf("channel or driver not found")
	}
	timeout := scanDeviceTimeout(ch.Protocol)
	paramsCopy := copyScanParams(params)
	return cm.jobs.Submit(AsyncJobScanDevice, channelID, deviceID, func(ctx context.Context) (any, error) {
		jobCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cm.ScanDevice(jobCtx, channelID, deviceID, paramsCopy)
	}), nil
}

func copyScanParams(params map[string]any) map[string]any {
	if params == nil {
		return make(map[string]any)
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

// ScanChannel 扫描通道下的设备。ctx 应由调用方设置超时（API job 或 sync 路径）。
func (cm *ChannelManager) ScanChannel(ctx context.Context, channelID string, params map[string]any) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cm.mu.RLock()
	d, okDrv := cm.drivers[channelID]
	chPtr, okCh := cm.channels[channelID]
	var ch model.Channel
	if okCh && chPtr != nil {
		ch = chPtr.DeepCopy()
	}
	cm.mu.RUnlock()

	if !okDrv {
		return nil, fmt.Errorf("channel driver not found")
	}

	// Cast to Scanner
	scanner, ok := d.(drv.Scanner)
	if !ok {
		return nil, fmt.Errorf("driver does not support scanning")
	}

	if params == nil {
		params = make(map[string]any)
	}

	// Inject existing device IDs for BACnet to mark duplicates
	if okCh && ch.Protocol == "bacnet-ip" {
		var existingIDs []int
		for _, dev := range ch.Devices {
			if id, ok := getDeviceID(dev.Config); ok {
				existingIDs = append(existingIDs, id)
			}
		}
		params["existing_device_ids"] = existingIDs

		// Inject interface_ip and target_ip from channel config if not already specified
		if _, has := params["interface_ip"]; !has {
			if ip, ok := ch.Config["interface_ip"].(string); ok && ip != "" {
				params["interface_ip"] = ip
			} else if ip, ok := ch.Config["ip"].(string); ok && ip != "" && ip != "0.0.0.0" {
				params["interface_ip"] = ip
			}
		}
		if _, has := params["target_ip"]; !has {
			if tip, ok := ch.Config["target_ip"].(string); ok && tip != "" {
				params["target_ip"] = tip
			}
		}
		// Inject target_devices (unicast-verified device list) from channel config
		if _, has := params["target_devices"]; !has {
			if td, ok := ch.Config["target_devices"]; ok && td != nil {
				params["target_devices"] = td
			}
		}

	}

	if okCh && ch.Protocol == "opc-ua" {
		merged := model.MergeOpcUaDeviceConfig(ch.Config, params)
		for k, v := range merged {
			params[k] = v
		}
	}

	// Scan may take a long time; do NOT hold driverMus during Scan.
	// Scan is a discovery operation (WhoIs/ReadProperty) that uses its own
	// ephemeral client. Holding driverMus would block all ReadPoint/WritePoint
	// on the same channel for the full discovery window.
	result, err := scanner.Scan(ctx, params)
	if err != nil {
		return result, err
	}

	// Post-scan: auto-update existing BACnet device addresses.
	// BACnet devices use dynamic UDP ports that change on reboot.
	// When a scan discovers an existing device with a new port, persist it
	// so the driver can reconnect without manual intervention.
	// 扫描后自动更新已有 BACnet 设备地址：设备重启后 UDP 端口会变化，
	// 扫描发现新端口后自动持久化，避免设备离线需要手动修正。
	if okCh && ch.Protocol == "bacnet-ip" {
		// Recover from any reflection/panic errors so the scan result
		// is still returned to the caller. Address auto-update is a
		// best-effort optimization, not a critical-path operation.
		// 地址自动更新是尽力而为的优化操作，任何异常都不应影响扫描结果返回。
		func() {
			defer func() {
				if r := recover(); r != nil {
					zap.L().Warn("autoUpdateBACnetAddresses panicked (recovered, scan result unaffected)",
						zap.Any("panic", r),
						zap.Stack("stack"))
				}
			}()
			cm.autoUpdateBACnetAddresses(&ch, result)
		}()
	}

	return result, nil
}

// autoUpdateBACnetAddresses extracts scan results via reflection and updates
// existing devices whose IP or port has changed. Uses reflection to avoid a
// hard dependency on the bacnet package's ScanResult type.
// autoUpdateBACnetAddresses 通过反射提取扫描结果，更新 IP 或端口已变化的已有设备。
func (cm *ChannelManager) autoUpdateBACnetAddresses(ch *model.Channel, scanResult any) {
	if scanResult == nil {
		return
	}

	// Build a lookup map: bacnet_device_id (int) → device ID (string)
	deviceIDByBacnetID := make(map[int]string)
	for _, dev := range ch.Devices {
		if id, ok := getDeviceID(dev.Config); ok {
			deviceIDByBacnetID[id] = dev.ID
		}
	}
	if len(deviceIDByBacnetID) == 0 {
		return
	}

	sliceVal := reflect.ValueOf(scanResult)
	if sliceVal.Kind() != reflect.Slice {
		return
	}

	updated := 0
	for i := 0; i < sliceVal.Len(); i++ {
		item := sliceVal.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}
		if item.Kind() != reflect.Struct {
			continue
		}

		// Extract fields by JSON tag (bacnet_device_id, ip, port, diff_status)
		bacnetID := extractIntField(item, "DeviceID")
		if bacnetID == 0 {
			bacnetID = extractIntByJSONTag(item, "bacnet_device_id")
		}
		if bacnetID == 0 {
			continue
		}

		// Only update existing devices (skip newly discovered ones)
		diffStatus := extractStringByJSONTag(item, "diff_status")
		if diffStatus == "new" {
			continue
		}

		deviceKey, found := deviceIDByBacnetID[bacnetID]
		if !found {
			continue
		}

		ip := extractStringByJSONTag(item, "ip")
		port := extractIntByJSONTag(item, "port")
		if ip == "" || port <= 0 {
			continue
		}

		// OnBACnetAddressDiscovered handles the actual config update + persistence
		cm.OnBACnetAddressDiscovered(deviceKey, ip, port)
		updated++
	}

	if updated > 0 {
		zap.L().Info("ScanChannel: auto-updated BACnet device addresses",
			zap.String("channel", ch.ID),
			zap.Int("updated", updated),
		)
	}
}

// extractIntField extracts an int from a struct field by Go field name.
// 安全提取 struct 字段的 int 值；非 struct 类型直接返回 0。
func extractIntField(v reflect.Value, fieldName string) int {
	if v.Kind() != reflect.Struct {
		return 0
	}
	f := v.FieldByName(fieldName)
	if !f.IsValid() {
		return 0
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(f.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(f.Uint())
	case reflect.Float32, reflect.Float64:
		return int(f.Float())
	}
	return 0
}

// extractIntByJSONTag extracts an int from a struct field by its JSON tag.
// 通过 JSON tag 匹配 struct 字段并提取 int 值。
func extractIntByJSONTag(v reflect.Value, jsonTag string) int {
	if v.Kind() != reflect.Struct {
		return 0
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		tagName := strings.Split(tag, ",")[0]
		if tagName == jsonTag {
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return int(f.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return int(f.Uint())
			case reflect.Float32, reflect.Float64:
				return int(f.Float())
			}
			return 0
		}
	}
	// Fallback: try by field name
	return extractIntField(v, jsonTag)
}

// extractStringByJSONTag extracts a string from a struct field by its JSON tag.
// 通过 JSON tag 匹配 struct 字段并提取 string 值。
func extractStringByJSONTag(v reflect.Value, jsonTag string) string {
	if v.Kind() != reflect.Struct {
		return ""
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		tagName := strings.Split(tag, ",")[0]
		if tagName == jsonTag {
			f := v.Field(i)
			if f.Kind() == reflect.String {
				return f.String()
			}
			return ""
		}
	}
	// Fallback: try by field name
	f := v.FieldByName(jsonTag)
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String()
	}
	return ""
}

// ScanDevice 扫描设备下的对象（点位）。ctx 应由调用方设置超时（OPC UA Browse 可达 180s）。
func (cm *ChannelManager) ScanDevice(ctx context.Context, channelID, deviceID string, params map[string]any) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cm.mu.RLock()
	d, okDrv := cm.drivers[channelID]
	chPtr, okCh := cm.channels[channelID]
	var ch model.Channel
	if okCh && chPtr != nil {
		ch = chPtr.DeepCopy()
	}
	cm.mu.RUnlock()

	if !okDrv || !okCh {
		return nil, fmt.Errorf("channel or driver not found")
	}

	// Cast to ObjectScanner
	scanner, ok := d.(drv.ObjectScanner)
	if !ok {
		return nil, fmt.Errorf("driver does not support object scanning")
	}

	// Find the device to extract configuration
	var targetDev *model.Device
	for _, dev := range ch.Devices {
		if dev.ID == deviceID {
			targetDev = &dev
			break
		}
	}
	if targetDev == nil {
		return nil, fmt.Errorf("device not found")
	}

	if params == nil {
		params = make(map[string]any)
	}

	// Inject protocol-specific device ID into params
	// For BACnet, we need "device_id" (int)
	if ch.Protocol == "bacnet-ip" {
		// bacnet_device_id: BACnet 通信使用的真实设备实例 ID（最高优先级）
		// 其次使用 instance_id，最后使用 device_id
		// 同时设置 params["device_id"] 和 params["bacnet_device_id"] 确保兼容
		var bacnetID any
		if v, ok := targetDev.Config["bacnet_device_id"]; ok {
			bacnetID = v
		} else if v, ok := targetDev.Config["instance_id"]; ok {
			bacnetID = v
		} else if v, ok := targetDev.Config["device_id"]; ok {
			bacnetID = v
		}
		if bacnetID != nil {
			params["device_id"] = bacnetID
			params["bacnet_device_id"] = bacnetID
		}
		// Pass IP and port for direct device addressing (bypasses WhoIs broadcast)
		if v, ok := targetDev.Config["ip"]; ok {
			params["ip"] = v
		}
		if v, ok := targetDev.Config["port"]; ok {
			params["port"] = v
		}
	} else if ch.Protocol == "opc-ua" {
		merged := model.MergeOpcUaDeviceConfig(ch.Config, targetDev.Config)
		for k, v := range merged {
			params[k] = v
		}
	}

	// NOTE: We intentionally do NOT hold the per-channel driver mutex (mu) during
	// ScanObjects. Holding the lock for a long Browse blocks ReadPoint/WritePoint
	// on the same channel. The driver's internal mutex protects shared state.
	return scanner.ScanObjects(ctx, params)
}

// appendDeviceLocked validates and appends a device (caller must hold cm.mu).
func (cm *ChannelManager) appendDeviceLocked(ch *model.Channel, channelID string, dev *model.Device) error {
	if ch.Protocol == "opc-ua" {
		model.NormalizeOpcUaDeviceID(dev)
	}

	if err := model.EnsureDeviceID(dev); err != nil {
		return err
	}

	for _, d := range ch.Devices {
		if d.ID == dev.ID {
			return fmt.Errorf("device %s already exists", dev.ID)
		}
		if ch.Protocol == "bacnet-ip" {
			newID, okNew := getDeviceID(dev.Config)
			oldID, okOld := getDeviceID(d.Config)
			if okNew && okOld && newID == oldID {
				return fmt.Errorf("BACnet device with Instance ID %d already exists", newID)
			}
		}
	}

	if (ch.Protocol == "modbus-tcp" || ch.Protocol == "modbus-rtu" || ch.Protocol == "modbus-rtu-over-tcp") && dev.Config != nil {
		if _, ok := dev.Config["auto_points_range"]; ok {
			cm.autoGenerateModbusPointsFromConfig(dev)
		}
	}

	if ch.Protocol == "opc-ua" {
		if dev.Config == nil {
			dev.Config = make(map[string]any)
		}
		dev.Config = model.MergeOpcUaDeviceConfig(ch.Config, dev.Config)
	}

	if ch.Protocol == "dlt645" && len(dev.Points) == 0 {
		cm.autoGenerateDLT645PointsFromConfig(dev)
	}

	sanitizeDeviceConfig(dev.Config)

	ch.Devices = append(ch.Devices, *dev)

	cm.stateManager.RegisterNode(dev.ID, dev.Name)
	cm.tagRegistry.RegisterFromDevice(ch.ID, &ch.Devices[len(ch.Devices)-1])

	if _, ok := cm.drivers[channelID]; ok && ch.Enable && dev.Enable {
		newDev := &ch.Devices[len(ch.Devices)-1]

		// BACnet: notify driver of device config so it can discover the device
		// and create the scheduler (device context). Without this, ReadPoints/WritePoint
		// fail with "scheduler not initialized".
		// BACnet 驱动需要 SetDeviceConfig 来发现设备并创建调度器上下文，
		// 否则 ReadPoints/WritePoint 会因 "scheduler not initialized" 失败。
		if ch.Protocol == "bacnet-ip" {
			d := cm.drivers[channelID]
			driverConfig := make(map[string]any)
			if dev.Config != nil {
				for k, v := range dev.Config {
					driverConfig[k] = v
				}
			}
			driverConfig["_internal_device_id"] = dev.ID
			// Use bacnet_device_id if present; instance_id is an alias for bacnet_device_id
			// bacnet_device_id/instance_id 用于真实设备通信，device_id 用于系统内部管理
			if _, hasBacnetID := driverConfig["bacnet_device_id"]; !hasBacnetID {
				if v, ok := driverConfig["instance_id"]; ok {
					driverConfig["bacnet_device_id"] = v
				} else {
					parts := strings.Split(dev.ID, "-")
					if len(parts) > 0 {
						if id, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
							driverConfig["bacnet_device_id"] = id
						}
					}
				}
			}
			if err := d.SetDeviceConfig(driverConfig); err != nil {
				zap.L().Warn("BACnet SetDeviceConfig failed", zap.String("device", dev.Name), zap.Error(err))
			}
		}

		if err := cm.registerDeviceToScanEngine(ch, newDev); err != nil {
			zap.L().Error("Failed to register device to ScanEngine", zap.String("device", dev.Name), zap.Error(err))
		} else {
			zap.L().Info("Device started via ScanEngine", zap.String("device", dev.Name))
		}
	}

	return nil
}

// AddDevice 添加设备到通道
func (cm *ChannelManager) AddDevice(channelID string, dev *model.Device) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	if err := cm.appendDeviceLocked(ch, channelID, dev); err != nil {
		return err
	}

	cm.notifyTopologyChange()
	return cm.saveChannels()
}

// AddDevices 批量添加设备（单次持久化与拓扑通知）。
func (cm *ChannelManager) AddDevices(channelID string, devices []model.Device) ([]model.Device, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}

	created := make([]model.Device, 0, len(devices))
	for i := range devices {
		dev := devices[i]
		if err := cm.appendDeviceLocked(ch, channelID, &dev); err != nil {
			return created, fmt.Errorf("failed to add device %s: %w", dev.Name, err)
		}
		created = append(created, ch.Devices[len(ch.Devices)-1])
	}

	if len(created) == 0 {
		return created, nil
	}

	if err := cm.saveChannels(); err != nil {
		return created, err
	}
	cm.notifyTopologyChange()
	return created, nil
}

// BatchAddModbusSlavesResult 批量添加 Modbus 从站结果。
type BatchAddModbusSlavesResult struct {
	Created []model.Device
	Skipped []int // slave_id
	Errors  []string
}

// BatchAddModbusSlaves 批量添加 Modbus 从站（单次持久化，跳过已存在设备）。
func (cm *ChannelManager) BatchAddModbusSlaves(channelID string, slaveStart, slaveEnd, regStart, regEnd int, interval model.Duration, enable bool, datatype, readwrite string, regType model.RegisterType, fc byte) (*BatchAddModbusSlavesResult, error) {
	if slaveEnd < slaveStart {
		slaveStart, slaveEnd = slaveEnd, slaveStart
	}
	if regEnd < regStart {
		regStart, regEnd = regEnd, regStart
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	if ch.Protocol != "modbus-tcp" && ch.Protocol != "modbus-rtu" && ch.Protocol != "modbus-rtu-over-tcp" {
		return nil, fmt.Errorf("channel protocol %s is not modbus", ch.Protocol)
	}

	existingID := make(map[string]struct{}, len(ch.Devices))
	for _, d := range ch.Devices {
		existingID[d.ID] = struct{}{}
	}

	result := &BatchAddModbusSlavesResult{
		Created: make([]model.Device, 0),
		Skipped: make([]int, 0),
		Errors:  make([]string, 0),
	}

	if datatype == "" {
		datatype = "int16"
	}
	if readwrite == "" {
		readwrite = "R"
	}
	if fc == 0 {
		fc = regType.FunctionCode()
	}

	// Collect devices to register after the loop (avoid per-device lock nesting inside cm.mu write lock).
	type pendingReg struct {
		dev *model.Device
	}
	var pendingRegs []pendingReg

	for slave := slaveStart; slave <= slaveEnd; slave++ {
		devID := fmt.Sprintf("modbus-slave-%d", slave)
		if _, exists := existingID[devID]; exists {
			result.Skipped = append(result.Skipped, slave)
			continue
		}

		dev := model.Device{
			ID:       devID,
			Name:     fmt.Sprintf("Modbus 从站 %d", slave),
			Enable:   enable,
			Interval: interval,
			Config: map[string]any{
				"slave_id":                  slave,
				"auto_points_range":         fmt.Sprintf("%d-%d", regStart, regEnd),
				"auto_points_datatype":      datatype,
				"auto_points_readwrite":     readwrite,
				"auto_points_register_type": regType.ShortString(),
				"auto_points_function_code": int(fc),
			},
		}
		if err := model.EnsureDeviceID(&dev); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("slave %d: %v", slave, err))
			continue
		}

		cm.autoGenerateModbusPointsFromConfig(&dev)
		sanitizeDeviceConfig(dev.Config)

		ch.Devices = append(ch.Devices, dev)
		existingID[devID] = struct{}{}
		newDev := &ch.Devices[len(ch.Devices)-1]
		cm.stateManager.RegisterNode(dev.ID, dev.Name)
		cm.tagRegistry.RegisterFromDevice(ch.ID, newDev)

		if _, ok := cm.drivers[channelID]; ok && ch.Enable && dev.Enable {
			pendingRegs = append(pendingRegs, pendingReg{dev: newDev})
		}

		result.Created = append(result.Created, dev)
	}

	shouldActivate := len(result.Created) > 0 && ch.Enable

	if len(result.Created) == 0 && len(result.Skipped) == 0 && len(result.Errors) > 0 {
		return result, fmt.Errorf("%s", result.Errors[0])
	}

	var saveErr error
	if err := cm.saveChannels(); err != nil {
		saveErr = err
	}

	// Register protocol once, then batch-register all pending devices.
	// Registration holds ScanEngineAdapter.mu/ScanEngine/ExecutionLayer locks
	// (nested inside cm.mu write lock — acceptable since these are short in-memory ops).
	// The expensive TCP connect (tryConnectChannel) is deferred to a goroutine
	// that runs AFTER cm.mu is released via defer.
	registeredDevices := make([]*model.Device, 0, len(pendingRegs))
	if len(pendingRegs) > 0 {
		cm.registerProtocolToScanEngine(ch.Protocol)
		for _, pr := range pendingRegs {
			if err := cm.registerDeviceToScanEngine(ch, pr.dev); err != nil {
				zap.L().Error("Failed to register device to ScanEngine",
					zap.String("device", pr.dev.Name), zap.Error(err))
				result.Errors = append(result.Errors, fmt.Sprintf("device %s register: %v", pr.dev.Name, err))
			} else {
				registeredDevices = append(registeredDevices, pr.dev)
			}
		}
	}

	cm.notifyTopologyChange()

	// tryConnectChannel performs synchronous TCP connect with 10s timeout —
	// launch as goroutine so it runs AFTER cm.mu write lock is released via defer.
	if shouldActivate && len(registeredDevices) > 0 {
		cm.scanEngineAdapter.Start()
		cid := channelID
		go cm.tryConnectChannel(cid)
	}

	if saveErr != nil {
		return result, saveErr
	}
	return result, nil
}

// AddPoint 添加点位到设备
func (cm *ChannelManager) AddPoint(channelID, deviceID string, point *model.Point) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	if err := model.EnsurePointID(point); err != nil {
		return err
	}

	dev := &ch.Devices[idx]

	// Check if point ID already exists
	for _, p := range dev.Points {
		if p.ID == point.ID {
			return fmt.Errorf("point %s already exists", point.ID)
		}
	}

	// Validate point based on protocol
	if err := cm.validatePoint(ch, point); err != nil {
		return err
	}

	// Add point
	dev.Points = append(dev.Points, *point)

	// Restart device to pick up the new point in the scan engine.
	// UI should prefer AddPoints (batch) for bulk imports to avoid
	// repeated unregister/register cycles.
	return cm.restartDeviceLocked(ch, idx)
}

// AddPoints 批量添加点位到设备（单次重启）
func (cm *ChannelManager) AddPoints(channelID, deviceID string, points []model.Point) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	dev := &ch.Devices[idx]

	// Build existing ID set for O(N+M) conflict detection instead of O(N*M) nested loop.
	existingIDs := make(map[string]struct{}, len(dev.Points))
	for i := range dev.Points {
		existingIDs[dev.Points[i].ID] = struct{}{}
	}

	// Pre-check: ID conflict & validate
	for i := range points {
		if err := model.EnsurePointID(&points[i]); err != nil {
			return err
		}

		if _, exists := existingIDs[points[i].ID]; exists {
			return fmt.Errorf("point %s already exists", points[i].ID)
		}

		// Mark as seen to catch duplicates within the same batch
		existingIDs[points[i].ID] = struct{}{}

		// Protocol-level validation
		if err := cm.validatePoint(ch, &points[i]); err != nil {
			return err
		}
	}

	// 追加到设备点位列表
	dev.Points = append(dev.Points, points...)

	return cm.restartDeviceLocked(ch, idx)
}

// pointUpdateRequiresDeviceRestart 判断点位变更是否影响南向采集任务（需重启设备）。
// 仅元数据变更（如读写权限、名称、单位）时返回 false，此时只重建北向 OPC UA 服务。
func pointUpdateRequiresDeviceRestart(before, after model.Point) bool {
	if before.Address != after.Address ||
		before.DataType != after.DataType ||
		before.RegisterType != after.RegisterType ||
		before.FunctionCode != after.FunctionCode ||
		before.Format != after.Format ||
		before.WordOrder != after.WordOrder ||
		before.ReadFormula != after.ReadFormula ||
		before.WriteFormula != after.WriteFormula ||
		before.Scale != after.Scale ||
		before.Offset != after.Offset ||
		before.ScanClass != after.ScanClass ||
		before.ReportMode != after.ReportMode ||
		before.Group != after.Group {
		return true
	}
	return false
}

// UpdatePoint 更新设备点位。返回值 deviceRestarted 表示是否重启了南向采集设备；
// 北向 OPC UA 地址空间会在保存后异步重建（仅重启 OPC UA 服务，不重启网关主进程）。
func (cm *ChannelManager) UpdatePoint(channelID, deviceID string, point *model.Point) (deviceRestarted bool, err error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return false, fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, fmt.Errorf("device not found")
	}

	dev := &ch.Devices[idx]

	if err := cm.validatePoint(ch, point); err != nil {
		return false, err
	}

	pointIdx := -1
	for i, p := range dev.Points {
		if p.ID == point.ID {
			pointIdx = i
			break
		}
	}
	if pointIdx == -1 {
		return false, fmt.Errorf("point not found")
	}

	before := dev.Points[pointIdx]
	dev.Points[pointIdx] = *point

	if pointUpdateRequiresDeviceRestart(before, *point) {
		if err := cm.restartDeviceLocked(ch, idx); err != nil {
			return true, err
		}
		return true, nil
	}

	if err := cm.saveChannels(); err != nil {
		return false, err
	}
	zap.L().Info("Point metadata updated, syncing northbound OPC UA only",
		zap.String("channel", channelID),
		zap.String("device", deviceID),
		zap.String("point", point.ID),
	)
	cm.notifyTopologyChange()
	return false, nil
}

// RemovePoint 删除设备点位
func (cm *ChannelManager) RemovePoint(channelID, deviceID, pointID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	dev := &ch.Devices[idx]

	// Find and remove point
	pointIdx := -1
	for i, p := range dev.Points {
		if p.ID == pointID {
			pointIdx = i
			break
		}
	}
	if pointIdx == -1 {
		return fmt.Errorf("point not found")
	}

	dev.Points = append(dev.Points[:pointIdx], dev.Points[pointIdx+1:]...)

	return cm.restartDeviceLocked(ch, idx)
}

// RemovePoints 批量删除设备点位
func (cm *ChannelManager) RemovePoints(channelID, deviceID string, pointIDs []string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	dev := &ch.Devices[idx]

	// Find and remove points
	newPoints := make([]model.Point, 0, len(dev.Points))
	idMap := make(map[string]bool)
	for _, id := range pointIDs {
		idMap[id] = true
	}

	removedCount := 0
	for _, p := range dev.Points {
		if !idMap[p.ID] {
			newPoints = append(newPoints, p)
		} else {
			removedCount++
		}
	}

	if removedCount == 0 {
		return fmt.Errorf("no points found to remove")
	}

	dev.Points = newPoints

	return cm.restartDeviceLocked(ch, idx)
}

// restartDeviceLocked 重启设备（需在持有锁的情况下调用）
func (cm *ChannelManager) restartDeviceLocked(ch *model.Channel, deviceIdx int) error {
	dev := &ch.Devices[deviceIdx]

	// 通过ScanEngine重新注册设备
	if _, ok := cm.drivers[ch.ID]; ok && ch.Enable && dev.Enable {
		cm.scanEngineAdapter.UnregisterDevice(dev.ID)
		if err := cm.registerDeviceToScanEngine(ch, dev); err != nil {
			zap.L().Error("Failed to restart device via ScanEngine", zap.String("device", dev.Name), zap.Error(err))
		} else {
			zap.L().Info("Device restarted via ScanEngine with updated points", zap.String("device", dev.Name))
		}
	}

	if err := cm.saveChannels(); err != nil {
		return err
	}
	cm.notifyTopologyChange()
	return nil
}

// UpdateDevice 更新通道下的设备
func (cm *ChannelManager) UpdateDevice(channelID string, dev *model.Device) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	// 查找设备索引
	idx := -1
	for i, d := range ch.Devices {
		if d.ID == dev.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	// 停止旧设备
	oldDev := &ch.Devices[idx]

	// 格式化配置
	sanitizeDeviceConfig(dev.Config)

	// Preserve points and storage from existing device — PUT device update
	// only modifies device-level fields (name, enable, interval, config),
	// not points or storage which are managed via separate API endpoints.
	// 保留已有点位和存储配置 — 设备 PUT 更新仅修改设备级字段
	//（名称、启用、间隔、配置），点位和存储通过独立 API 管理。
	dev.Points = oldDev.Points
	if dev.Storage.Enable == false && oldDev.Storage.Enable {
		dev.Storage = oldDev.Storage
	}

	// 替换
	ch.Devices[idx] = *dev

	// 如果启用，通过ScanEngine重新注册
	if _, ok := cm.drivers[channelID]; ok && ch.Enable && dev.Enable {
		newDev := &ch.Devices[idx]
		cm.scanEngineAdapter.UnregisterDevice(oldDev.ID)
		if err := cm.registerDeviceToScanEngine(ch, newDev); err != nil {
			zap.L().Error("Failed to register device to ScanEngine", zap.String("device", dev.Name), zap.Error(err))
		} else {
			zap.L().Info("Device restarted via ScanEngine", zap.String("device", dev.Name))
		}
	}

	cm.notifyTopologyChange()
	return cm.saveChannels()
}

// RemoveDevice 删除设备
func (cm *ChannelManager) RemoveDevice(channelID, deviceID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("device not found")
	}

	// 停止设备
	oldDev := &ch.Devices[idx]
	cm.scanEngineAdapter.UnregisterDevice(oldDev.ID)
	cm.stateManager.UnregisterNode(oldDev.ID)

	// 从切片移除
	ch.Devices = append(ch.Devices[:idx], ch.Devices[idx+1:]...)

	cm.notifyTopologyChange()
	return cm.saveChannels()
}

// RemoveDevices 批量删除设备
func (cm *ChannelManager) RemoveDevices(channelID string, deviceIDs []string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return fmt.Errorf("channel not found")
	}

	toRemove := make(map[string]bool)
	for _, id := range deviceIDs {
		toRemove[id] = true
	}

	newDevices := make([]model.Device, 0)
	for _, d := range ch.Devices {
		if toRemove[d.ID] {
			cm.scanEngineAdapter.UnregisterDevice(d.ID)
			cm.stateManager.UnregisterNode(d.ID)
		} else {
			newDevices = append(newDevices, d)
		}
	}
	ch.Devices = newDevices

	return cm.saveChannels()
}

// withDriverIO serializes shared-link REST I/O with Scan via channelMu (driverMus).
// Non-shared protocols still serialize config mutation on the per-channel mutex
// for a short critical section that includes the I/O call (5s bounded by caller ctx).
func (cm *ChannelManager) withDriverIO(channelID, protocol string, fn func() error) error {
	cm.mu.RLock()
	mu := cm.driverMus[channelID]
	cm.mu.RUnlock()
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	} else if isSharedLinkProtocol(protocol) {
		zap.L().Warn("shared-link write/read without channelMu",
			zap.String("channel_id", channelID),
			zap.String("protocol", protocol),
		)
	}
	return fn()
}

// saveChannels 辅助方法：保存所有通道配置。
// NOTE: Caller must hold cm.mu.Lock. This method copies the channel data
// and delegates to saveFunc in a goroutine so the lock is not held during I/O.
func (cm *ChannelManager) saveChannels() error {
	if cm.saveFunc == nil {
		return nil
	}
	// Copy channel data under caller's lock
	channels := make([]model.Channel, 0, len(cm.channels))
	for _, c := range cm.channels {
		channels = append(channels, c.DeepCopy())
	}

	// Save asynchronously to avoid holding cm.mu during disk I/O.
	go func() {
		if err := cm.saveFunc(channels); err != nil {
			zap.L().Warn("Failed to save config", zap.Error(err))
		}
	}()
	return nil
}

// getDeviceID extracts the BACnet device instance ID used for communication.
// Only bacnet_device_id is used for BACnet communication.
// device_id is the system management UUID and must NOT be used for communication.
func getDeviceID(config map[string]any) (int, bool) {
	if config == nil {
		return 0, false
	}
	// Primary: bacnet_device_id — the authoritative BACnet instance ID for communication
	if v, ok := config["bacnet_device_id"]; ok {
		if id, ok := coerceConfigInt(v); ok {
			return id, true
		}
	}
	// Fallback: instance_id — alias for bacnet_device_id (backward compatibility)
	if v, ok := config["instance_id"]; ok {
		if id, ok := coerceConfigInt(v); ok {
			return id, true
		}
	}
	return 0, false
}

func coerceConfigInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}

// buildDriverDeviceConfig 构建传给驱动的设备配置（OPC UA 自动继承通道 Endpoint 等参数）。
func buildDriverDeviceConfig(ch *model.Channel, deviceConfig map[string]any, extra map[string]any) map[string]any {
	base := make(map[string]any)
	for k, v := range deviceConfig {
		base[k] = v
	}
	if ch != nil && ch.Protocol == "opc-ua" {
		base = model.MergeOpcUaDeviceConfig(ch.Config, base)
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// normalizeModbusChannelConfig 规范化 Modbus TCP 通道连接 URL（补全 tcp:// scheme）。
func normalizeModbusChannelConfig(config map[string]any) {
	if config == nil {
		return
	}
	url, _ := config["url"].(string)
	if url == "" {
		// 从 ip/port 构造 URL
		ip, _ := config["ip"].(string)
		if ip == "" {
			ip, _ = config["host"].(string)
		}
		port := 502
		if p, ok := coerceConfigInt(config["port"]); ok {
			port = p
		}
		if ip != "" {
			url = fmt.Sprintf("tcp://%s:%d", ip, port)
			config["url"] = url
		}
		return
	}
	url = strings.TrimSpace(url)
	if strings.Contains(url, "://") {
		config["url"] = url
		return
	}
	config["url"] = "tcp://" + url
}

// sanitizeDeviceConfig 修正配置中的数值类型（如去除科学计数法）
func sanitizeDeviceConfig(config map[string]any) {
	if config == nil {
		return
	}
	// 处理 bacnet_device_id (防止 float64 科学计数法保存)
	if val, ok := config["bacnet_device_id"]; ok {
		if id, ok := coerceConfigInt(val); ok {
			config["bacnet_device_id"] = id
		}
	}
	// 处理 network_number
	if val, ok := config["network_number"]; ok {
		switch v := val.(type) {
		case float64:
			config["network_number"] = int(v)
		case float32:
			config["network_number"] = int(v)
		}
	}
}

func (cm *ChannelManager) autoGenerateModbusPointsFromConfig(dev *model.Device) {
	opts, ok := modbusGenOptionsFromDevice(dev)
	if !ok {
		return
	}
	dev.Points = GenerateModbusRegisterPoints(dev.Points, opts, true)
}

// GenerateDeviceRegisterPoints 为设备批量生成 Modbus 寄存器点位并持久化。
// mode: merge（保留同 ID 现有点）| replace（仅保留新区间点位）。
func (cm *ChannelManager) GenerateDeviceRegisterPoints(channelID, deviceID string, opts ModbusRegisterGenOptions, mode string) (*model.Device, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, ok := cm.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	if ch.Protocol != "modbus-tcp" && ch.Protocol != "modbus-rtu" && ch.Protocol != "modbus-rtu-over-tcp" {
		return nil, fmt.Errorf("channel protocol %s is not modbus", ch.Protocol)
	}

	idx := -1
	for i, d := range ch.Devices {
		if d.ID == deviceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("device not found")
	}

	dev := &ch.Devices[idx]
	opts.DeviceID = dev.ID
	merge := mode != "replace"
	dev.Points = GenerateModbusRegisterPoints(dev.Points, opts, merge)

	if dev.Config == nil {
		dev.Config = make(map[string]any)
	}
	dev.Config["auto_points_range"] = fmt.Sprintf("%d-%d", opts.Start, opts.End)
	dev.Config["auto_points_datatype"] = opts.DataType
	dev.Config["auto_points_readwrite"] = opts.ReadWrite
	dev.Config["auto_points_register_type"] = opts.RegisterType.ShortString()
	dev.Config["auto_points_function_code"] = int(opts.FunctionCode)

	if err := cm.restartDeviceLocked(ch, idx); err != nil {
		return nil, err
	}
	out := ch.Devices[idx]
	return &out, nil
}

func (cm *ChannelManager) wireBACnetAddressNotifier(channelID string, d drv.Driver) {
	if setter, ok := d.(drv.BACnetAddressNotifySetter); ok {
		setter.SetBACnetAddressNotifier(cm)
	}
}

// OnBACnetAddressDiscovered persists a runtime address change (e.g. UDP port after device reboot).
func (cm *ChannelManager) OnBACnetAddressDiscovered(deviceKey, ip string, port int) {
	if deviceKey == "" || ip == "" || port <= 0 {
		return
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for channelID, ch := range cm.channels {
		if ch.Protocol != "bacnet-ip" {
			continue
		}
		for i := range ch.Devices {
			if ch.Devices[i].ID != deviceKey {
				continue
			}
			if ch.Devices[i].Config == nil {
				ch.Devices[i].Config = map[string]any{}
			}
			oldPort, hasPort := coerceConfigInt(ch.Devices[i].Config["port"])
			oldIP, _ := ch.Devices[i].Config["ip"].(string)
			changed := false
			if !hasPort || oldPort != port {
				ch.Devices[i].Config["port"] = port
				changed = true
			}
			if oldIP != ip {
				ch.Devices[i].Config["ip"] = ip
				changed = true
			}
			if !changed {
				return
			}

			cm.channels[channelID] = ch
			cm.scanEngineAdapter.UpdateDeviceDriverConfig(deviceKey, map[string]any{
				"ip":   ip,
				"port": port,
			})

			if cm.saveFunc != nil {
				channels := make([]model.Channel, 0, len(cm.channels))
				for _, c := range cm.channels {
					channels = append(channels, *c)
				}
				if err := cm.saveFunc(channels); err != nil {
					zap.L().Warn("Failed to persist BACnet address update",
						zap.String("device", deviceKey),
						zap.Error(err),
					)
				}
			}

			zap.L().Info("Persisted BACnet device address update",
				zap.String("device", deviceKey),
				zap.String("ip", ip),
				zap.Int("port", port),
			)
			return
		}
	}
}
