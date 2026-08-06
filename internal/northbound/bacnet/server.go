// Package bacnet 实现北向 BACnet Server，以从机模式对外暴露 edgeCore 点位数据。
// BACnet Server 将南向设备的点位映射为 BACnet 标准对象（AnalogInput/BinaryInput等），
// 支持 Who-Is/I-Am 设备发现、ReadProperty/WriteProperty 属性读写和 COV 订阅通知。
package bacnet

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/server"
	"github.com/anviod/edgeCore/internal/model"
)

// Server 是 BACnet Server 的核心实现，将 edgeCore 南向设备和点位映射为 BACnet 对象。
// 遵循 OPC UA Server 的架构模式：Start/Stop 生命周期管理、Update 数据流、SyncAddressSpace 热更新。
type Server struct {
	config       model.BACnetServerConfig
	sb           model.SouthboundManager
	srv          server.Server           // 底层 BACnet 库 Server
	mu           sync.RWMutex            // 保护 pointMap 和 deviceIDs
	lifecycleMu  sync.Mutex              // 保护 Start/Stop/UpdateConfig 生命周期
	pointMap     map[string]pointMapping // key: channelID/deviceID/pointID → BACnet 对象信息
	deviceIDs    map[string]struct{}     // 已暴露的虚拟设备 ID 集合
	ctx          context.Context
	cancel       context.CancelFunc
	stats        Stats
	writeHistory []WriteHistoryItem
}

// pointMapping 记录一个 edgeCore 点位到 BACnet 对象的映射关系
type pointMapping struct {
	ObjectType btypes.ObjectType     // BACnet 对象类型 (AnalogInput, BinaryInput, etc.)
	Instance   btypes.ObjectInstance // BACnet 对象实例号
	DeviceID   string                // 所属 edgeCore 设备 ID
	PointID    string                // edgeCore 点位 ID
	PointName  string                // 点位名称
	Writable   bool                  // 是否可写
}

// Stats 统计信息
type Stats struct {
	ObjectCount   int       `json:"object_count"`    // BACnet 对象总数
	PointCount    int       `json:"point_count"`     // 已映射点位总数
	WriteCount    int64     `json:"write_count"`     // 外部写入次数
	UpdateCount   int64     `json:"update_count"`    // 南向数据更新次数
	LastWriteTime time.Time `json:"last_write_time"` // 最近一次外部写入时间
	StartTime     time.Time `json:"start_time"`      // 服务启动时间
}

// WriteRequest 外部写入请求
type WriteRequest struct {
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	PointID   string `json:"point_id"`
	Value     any    `json:"value"`
}

// BatchWriteResult 批量写入结果
type BatchWriteResult struct {
	ChannelID string `json:"channel_id"`
	DeviceID  string `json:"device_id"`
	PointID   string `json:"point_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// WriteHistoryItem 写入历史记录
type WriteHistoryItem struct {
	Time      time.Time `json:"time"`
	ChannelID string    `json:"channel_id"`
	DeviceID  string    `json:"device_id"`
	PointID   string    `json:"point_id"`
	Value     any       `json:"value"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// NewServer 创建新的 BACnet Server
func NewServer(cfg model.BACnetServerConfig, sb model.SouthboundManager) *Server {
	return &Server{
		config:    cfg,
		sb:        sb,
		pointMap:  make(map[string]pointMapping),
		deviceIDs: make(map[string]struct{}),
	}
}

// Start 启动 BACnet Server
func (s *Server) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.startLocked()
}

// startLocked 在持有 lifecycleMu 锁的情况下启动 Server
func (s *Server) startLocked() error {
	log.Printf("[BACnetServer] ========== Starting BACnet Server [%s] ==========", s.config.Name)

	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 构建 DeviceConfig
	devCfg := s.buildDeviceConfig()
	log.Printf("[BACnetServer] Config: DeviceID=%d, DeviceName=%s, VendorID=%d",
		devCfg.DeviceID, devCfg.DeviceName, devCfg.VendorID)
	log.Printf("[BACnetServer] Network: IP=%s, Interface=%s, Port=%d, SubnetCIDR=%d, MaxPDU=%d",
		devCfg.Ip, devCfg.Interface, devCfg.Port, devCfg.SubnetCIDR, devCfg.MaxPDU)

	// 创建底层 BACnet Server
	srv, err := server.NewServer(devCfg)
	if err != nil {
		log.Printf("[BACnetServer] Failed to create server: %v", err)
		return fmt.Errorf("bacnet server: failed to create server: %w", err)
	}
	s.srv = srv

	// 构建 BACnet 地址空间（将 edgeCore 点位映射为 BACnet 对象）
	if err := s.buildAddressSpace(); err != nil {
		s.srv.Close()
		s.srv = nil
		log.Printf("[BACnetServer] Failed to build address space: %v", err)
		return fmt.Errorf("bacnet server: failed to build address space: %w", err)
	}

	s.stats.StartTime = time.Now()
	s.stats.ObjectCount = s.countObjects()
	s.stats.PointCount = len(s.pointMap)
	log.Printf("[BACnetServer] Address space: %d objects, %d points mapped", s.stats.ObjectCount, s.stats.PointCount)

	// 启动 BACnet 服务（阻塞，在 goroutine 中运行）
	go func() {
		log.Printf("[BACnetServer] +++ Serve() goroutine starting on %s:%d (DeviceID=%d) +++",
			devCfg.Ip, devCfg.Port, devCfg.DeviceID)
		if err := srv.Serve(); err != nil {
			// Serve 在 Close 后返回错误，这是正常的停止流程
			select {
			case <-s.ctx.Done():
				// 正常停止，忽略错误
				log.Printf("[BACnetServer] Serve() goroutine stopped normally")
			default:
				log.Printf("[BACnetServer] BACnet Server [%s] serve error: %v", s.config.Name, err)
			}
		}
	}()

	log.Printf("[BACnetServer] BACnet Server [%s] started successfully", s.config.Name)
	return nil
}

// buildDeviceConfig 从 BACnetServerConfig 构建底层库的 DeviceConfig
func (s *Server) buildDeviceConfig() *server.DeviceConfig {
	cfg := &server.DeviceConfig{
		DeviceID:   btypes.ObjectInstance(s.config.DeviceID),
		DeviceName: s.config.DeviceName,
		VendorID:   s.config.VendorID,
		Interface:  s.config.Interface,
		Ip:         s.config.IP,
		Port:       s.config.Port,
		SubnetCIDR: s.config.SubnetCIDR,
		MaxPDU:     s.config.MaxPDU,
	}

	// 应用默认值
	if cfg.DeviceID == 0 {
		cfg.DeviceID = s.generateDeviceID()
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = s.config.Name
		if cfg.DeviceName == "" {
			cfg.DeviceName = "edgeCore-Gateway"
		}
	}
	if cfg.VendorID == 0 {
		cfg.VendorID = 999
	}
	if cfg.Port == 0 {
		cfg.Port = 47808
	}
	if cfg.SubnetCIDR == 0 {
		cfg.SubnetCIDR = 24
	}
	if cfg.MaxPDU == 0 {
		cfg.MaxPDU = 1476
	}

	return cfg
}

// generateDeviceID 根据 Name 生成确定性的 BACnet 设备实例 ID（范围 1000-4194303）
func (s *Server) generateDeviceID() btypes.ObjectInstance {
	h := fnv.New32a()
	h.Write([]byte(s.config.Name))
	// 映射到 1000 - 4194303 范围
	return btypes.ObjectInstance(1000 + (h.Sum32() % 4193303))
}

// Stop 停止 BACnet Server。可安全多次调用。
func (s *Server) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	wasRunning := s.srv != nil || s.cancel != nil
	s.stopLocked()
	if !wasRunning {
		return
	}
	log.Printf("[BACnetServer] BACnet Server [%s] stopped", s.config.Name)
}

// stopLocked 在持有 lifecycleMu 锁的情况下停止 Server
func (s *Server) stopLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

// UpdateConfig 更新配置。端口/IP/DeviceID 等结构性变更需要重启，设备映射变更可热更新。
func (s *Server) UpdateConfig(cfg model.BACnetServerConfig) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if requiresServerRestart(s.config, cfg) {
		s.stopLocked()
		s.config = cfg
		return s.startLocked()
	}

	s.config = cfg
	if s.srv == nil {
		return s.startLocked()
	}
	return s.rebuildAddressSpaceLocked()
}

// SyncAddressSpace 同步地址空间，不停 TCP 监听器（热更新）
func (s *Server) SyncAddressSpace() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.srv == nil {
		if !s.config.Enable {
			return nil
		}
		return s.startLocked()
	}
	return s.rebuildAddressSpaceLocked()
}

// requiresServerRestart 判断是否需要重启 Server（IP/Port/DeviceID/MaxPDU 变更）
func requiresServerRestart(oldCfg, newCfg model.BACnetServerConfig) bool {
	if oldCfg.Interface != newCfg.Interface {
		return true
	}
	if oldCfg.IP != newCfg.IP {
		return true
	}
	if oldCfg.Port != newCfg.Port {
		return true
	}
	if oldCfg.SubnetCIDR != newCfg.SubnetCIDR {
		return true
	}
	if oldCfg.DeviceID != newCfg.DeviceID {
		return true
	}
	if oldCfg.MaxPDU != newCfg.MaxPDU {
		return true
	}
	return false
}

// Update 从数据管道接收南向数据更新，写入对应 BACnet 对象的 PresentValue
func (s *Server) Update(v model.Value) {
	s.mu.RLock()
	mapping, ok := s.pointMap[pointKey(v.ChannelID, v.DeviceID, v.PointID)]
	s.mu.RUnlock()

	if !ok {
		return
	}

	// 将 edgeCore 值转换为 BACnet 兼容类型
	bacnetValue := convertToBACnetValue(v.Value, mapping.ObjectType)

	if err := s.srv.SetProperty(mapping.ObjectType, mapping.Instance, btypes.PROP_PRESENT_VALUE, bacnetValue); err != nil {
		log.Printf("[BACnetServer] Update SetProperty failed: obj=%v inst=%d err=%v",
			mapping.ObjectType, mapping.Instance, err)
		return
	}

	s.mu.Lock()
	s.stats.UpdateCount++
	s.mu.Unlock()
}

// WriteViaBACnet 通过 BACnet 外部写入请求，将值写回南向设备
func (s *Server) WriteViaBACnet(channelID, deviceID, pointID string, value any) error {
	s.mu.RLock()
	mapping, ok := s.pointMap[pointKey(channelID, deviceID, pointID)]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("bacnet server: point %s/%s/%s not found in address space", channelID, deviceID, pointID)
	}

	if !mapping.Writable {
		return fmt.Errorf("bacnet server: point %s/%s/%s is not writable", channelID, deviceID, pointID)
	}

	// 写入南向设备
	if err := s.sb.WritePoint(channelID, deviceID, pointID, value); err != nil {
		s.recordWriteHistory(channelID, deviceID, pointID, value, false, err.Error())
		return fmt.Errorf("bacnet server: write to device failed: %w", err)
	}

	// 同步更新 BACnet 对象的 PresentValue
	bacnetValue := convertToBACnetValue(value, mapping.ObjectType)
	if err := s.srv.SetProperty(mapping.ObjectType, mapping.Instance, btypes.PROP_PRESENT_VALUE, bacnetValue); err != nil {
		log.Printf("[BACnetServer] WriteViaBACnet SetProperty failed: obj=%v inst=%d err=%v",
			mapping.ObjectType, mapping.Instance, err)
	}

	s.recordWriteHistory(channelID, deviceID, pointID, value, true, "")
	return nil
}

// BatchWrite 批量写入
func (s *Server) BatchWrite(requests []WriteRequest) []BatchWriteResult {
	results := make([]BatchWriteResult, len(requests))
	for i, req := range requests {
		err := s.WriteViaBACnet(req.ChannelID, req.DeviceID, req.PointID, req.Value)
		results[i] = BatchWriteResult{
			ChannelID: req.ChannelID,
			DeviceID:  req.DeviceID,
			PointID:   req.PointID,
			Success:   err == nil,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return results
}

// recordWriteHistory 记录写入历史
func (s *Server) recordWriteHistory(channelID, deviceID, pointID string, value any, success bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.WriteCount++
	s.stats.LastWriteTime = time.Now()

	item := WriteHistoryItem{
		Time:      time.Now(),
		ChannelID: channelID,
		DeviceID:  deviceID,
		PointID:   pointID,
		Value:     value,
		Success:   success,
		Error:     errMsg,
	}

	// 保持最近 100 条记录
	s.writeHistory = append(s.writeHistory, item)
	if len(s.writeHistory) > 100 {
		s.writeHistory = s.writeHistory[len(s.writeHistory)-100:]
	}
}

// GetStats 获取统计信息
func (s *Server) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// GetWriteHistory 获取写入历史
func (s *Server) GetWriteHistory(limit int) []WriteHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.writeHistory
	if limit <= 0 || limit > len(history) {
		limit = len(history)
	}
	// 返回最新的 limit 条
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	return history[start:]
}

// IsRunning 检查 Server 是否在运行
func (s *Server) IsRunning() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.srv != nil
}

// buildAddressSpace 从南向设备构建 BACnet 地址空间
func (s *Server) buildAddressSpace() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清空旧映射
	s.pointMap = make(map[string]pointMapping)
	s.deviceIDs = make(map[string]struct{})

	channels := s.sb.GetChannels()
	sort.Slice(channels, func(i, j int) bool { return channels[i].ID < channels[j].ID })

	// 用于分配唯一 BACnet 对象实例号
	nextInstance := btypes.ObjectInstance(1)

	for _, ch := range channels {
		devices := s.sb.GetChannelDevices(ch.ID)
		sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })

		for _, dev := range devices {
			// 设备过滤
			if len(s.config.Devices) > 0 && !s.config.Devices.AllowsDevice(dev.ID) {
				continue
			}

			// 记录设备 ID
			s.deviceIDs[dev.ID] = struct{}{}

			points, err := s.sb.GetDevicePoints(ch.ID, dev.ID)
			if err != nil {
				log.Printf("[BACnetServer] GetDevicePoints failed: channel=%s device=%s err=%v",
					ch.ID, dev.ID, err)
				continue
			}

			for _, pt := range points {
				objType := inferBACnetObjectType(pt)
				key := pointKey(ch.ID, dev.ID, pt.ID)
				writable := strings.Contains(strings.ToUpper(pt.ReadWrite), "W")

				s.pointMap[key] = pointMapping{
					ObjectType: objType,
					Instance:   nextInstance,
					DeviceID:   dev.ID,
					PointID:    pt.ID,
					PointName:  pt.Name,
					Writable:   writable,
				}

				// 创建 BACnet 对象并设置初始值
				// 必须包含 BACnet 标准必需属性，否则 Yabe 等客户端读取 Object_List 时失败
				obj := btypes.Object{
					Name:        pt.Name,
					Description: fmt.Sprintf("%s/%s/%s", ch.Name, dev.Name, pt.Name),
					ID:          btypes.ObjectID{Type: objType, Instance: nextInstance},
					Properties: []btypes.Property{
						{
							Type:       btypes.PROP_OBJECT_IDENTIFIER,
							ArrayIndex: btypes.ArrayAll,
							Data:       btypes.ObjectID{Type: objType, Instance: nextInstance},
						},
						{
							Type:       btypes.PROP_OBJECT_NAME,
							ArrayIndex: btypes.ArrayAll,
							Data:       pt.Name,
						},
						{
							Type:       btypes.PROP_OBJECT_TYPE,
							ArrayIndex: btypes.ArrayAll,
							Data:       btypes.Enumerated(objType),
						},
						{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
							Data:       convertToBACnetValue(pt.Value, objType),
						},
						{
							Type:       btypes.PROP_DESCRIPTION,
							ArrayIndex: btypes.ArrayAll,
							Data:       fmt.Sprintf("edgeCore Point: %s/%s/%s", ch.Name, dev.Name, pt.Name),
						},
						{
							Type:       btypes.PROP_STATUS_FLAGS,
							ArrayIndex: btypes.ArrayAll,
							Data:       newStatusFlags(), // {inAlarm, fault, overridden, outOfService} = all normal
						},
					},
				}

				if err := s.srv.AddObject(obj); err != nil {
					log.Printf("[BACnetServer] AddObject failed: type=%v inst=%d err=%v",
						objType, nextInstance, err)
				}

				nextInstance++
			}
		}
	}

	// 处理虚拟设备
	if s.config.VirtualDevices != nil {
		// 虚拟设备由外部管理，此处仅记录
		for devID := range s.config.VirtualDevices {
			s.deviceIDs[devID] = struct{}{}
		}
	}

	return nil
}

// rebuildAddressSpaceLocked 原地重建地址空间（热更新）
func (s *Server) rebuildAddressSpaceLocked() error {
	// 移除旧对象
	s.mu.Lock()
	oldPointMap := s.pointMap
	s.mu.Unlock()

	for _, mapping := range oldPointMap {
		if err := s.srv.RemoveObject(mapping.ObjectType, mapping.Instance); err != nil {
			log.Printf("[BACnetServer] RemoveObject failed: type=%v inst=%d err=%v",
				mapping.ObjectType, mapping.Instance, err)
		}
	}

	return s.buildAddressSpace()
}

// countObjects 统计当前 BACnet 对象数量
func (s *Server) countObjects() int {
	count := 0
	store := s.srv.GetObjectStore()
	if store == nil {
		return count
	}
	for _, objs := range store.GetAllObjects() {
		count += len(objs)
	}
	return count
}

// inferBACnetObjectType 根据 edgeCore 点位 DataType 推断 BACnet 对象类型
// 映射规则:
//   - float32/float64/float → AnalogInput(0) / AnalogValue(2)
//   - bool → BinaryInput(3) / BinaryValue(5)
//   - int/uint → AnalogInput(0)
//   - string → CharacterStringValue(40) 或 MultiStateValue(19)
//   - 其他 → AnalogInput(0)
func inferBACnetObjectType(pt model.PointData) btypes.ObjectType {
	dt := strings.ToLower(pt.DataType)
	writable := strings.Contains(strings.ToUpper(pt.ReadWrite), "W")

	switch {
	case dt == "bool" || dt == "boolean":
		if writable {
			return btypes.BinaryValue // 5
		}
		return btypes.BinaryInput // 3
	case dt == "string" || dt == "charstring" || dt == "characterstring":
		if writable {
			return btypes.MultiStateValue // 19
		}
		return btypes.MultiStateInput // 13
	case dt == "float32" || dt == "float64" || dt == "float" || dt == "double" || dt == "real":
		fallthrough
	default:
		if writable {
			return btypes.AnalogValue // 2
		}
		return btypes.AnalogInput // 0
	}
}

// convertToBACnetValue 将 edgeCore 值转换为 BACnet 兼容的类型
func convertToBACnetValue(value any, objType btypes.ObjectType) any {
	if value == nil {
		return nil
	}

	switch objType {
	case btypes.AnalogInput, btypes.AnalogOutput, btypes.AnalogValue:
		return toFloat64(value)
	case btypes.BinaryInput, btypes.BinaryOutput, btypes.BinaryValue:
		return toBool(value)
	case btypes.MultiStateInput, btypes.MultiStateValue:
		return toUint32(value)
	default:
		return value
	}
}

// toFloat64 将任意数值类型转换为 float64
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case bool:
		if val {
			return 1.0
		}
		return 0.0
	default:
		return 0.0
	}
}

// toBool 将任意类型转换为 bool
func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case float32:
		return val != 0
	case int:
		return val != 0
	case int32:
		return val != 0
	case int64:
		return val != 0
	case uint32:
		return val != 0
	case string:
		lower := strings.ToLower(val)
		return lower == "true" || lower == "1" || lower == "on"
	default:
		return false
	}
}

// toUint32 将任意类型转换为 uint32
func toUint32(v any) uint32 {
	switch val := v.(type) {
	case uint32:
		return val
	case float64:
		return uint32(val)
	case float32:
		return uint32(val)
	case int:
		return uint32(val)
	case int32:
		return uint32(val)
	case int64:
		return uint32(val)
	case uint:
		return uint32(val)
	case uint64:
		return uint32(val)
	case bool:
		if val {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// pointKey 生成点位键
func pointKey(channelID, deviceID, pointID string) string {
	return channelID + "/" + deviceID + "/" + pointID
}

// newStatusFlags 创建 BACnet StatusFlags BitString (4 bits: inAlarm, fault, overridden, outOfService)
// 所有位初始为 false，表示设备正常运行状态。
func newStatusFlags() *btypes.BitString {
	bs := btypes.NewBitString(1) // 4 bits fit in 1 byte
	bs.SetBit(0, false)          // inAlarm
	bs.SetBit(1, false)          // fault
	bs.SetBit(2, false)          // overridden
	bs.SetBit(3, false)          // outOfService
	return bs
}
