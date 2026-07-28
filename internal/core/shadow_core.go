package core

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgex/internal/model"
)

const VirtualShadowPrefix = "virtual-"

func IsVirtualShadowID(shadowDeviceID string) bool {
	return strings.HasPrefix(shadowDeviceID, VirtualShadowPrefix)
}

func VirtualShadowID(virtualDeviceID string) string {
	return VirtualShadowPrefix + virtualDeviceID
}

type ShadowSubscriber func(deviceID string, points map[string]model.ShadowPoint)

func deepCloneValue(value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, val := range v {
			result[key] = deepCloneValue(val)
		}
		return result
	case map[any]any:
		result := make(map[any]any, len(v))
		for key, val := range v {
			result[key] = deepCloneValue(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = deepCloneValue(val)
		}
		return result
	default:
		return value
	}
}

func cloneShadowPoints(src map[string]model.ShadowPoint) map[string]model.ShadowPoint {
	if src == nil {
		return nil
	}
	dst := make(map[string]model.ShadowPoint, len(src))
	for k, v := range src {
		cloned := v
		cloned.Value = deepCloneValue(v.Value)
		dst[k] = cloned
	}
	return dst
}

func cloneCommunicationProfile(profile *model.DeviceCommunicationProfile) *model.DeviceCommunicationProfile {
	if profile == nil {
		return nil
	}
	result := *profile
	if profile.ProtocolParams != nil {
		result.ProtocolParams = make(map[string]interface{}, len(profile.ProtocolParams))
		for k, v := range profile.ProtocolParams {
			result.ProtocolParams[k] = deepCloneValue(v)
		}
	}
	if profile.RTTSamples != nil {
		result.RTTSamples = make([]int64, len(profile.RTTSamples))
		copy(result.RTTSamples, profile.RTTSamples)
	}
	return &result
}

func cloneShadowDevice(device *model.ShadowDevice) *model.ShadowDevice {
	if device == nil {
		return nil
	}
	copy := *device
	copy.Points = cloneShadowPoints(device.Points)
	copy.CommunicationProfile = cloneCommunicationProfile(device.CommunicationProfile)
	return &copy
}

// ShadowCore 维护每物理设备唯一的内存态影子设备（全量点位 + 通信画像），不落盘。
type ShadowCore struct {
	// versionCounter 置于 struct 首部，保证 ARMv7 32-bit 上 atomic.Uint64 8 字节对齐。
	versionCounter atomic.Uint64

	mu sync.RWMutex

	realShadows    map[string]*shadowDeviceEntry
	virtualShadows map[string]*model.VirtualDevice

	subscribers []ShadowSubscriber
	subMu       sync.RWMutex

	optimizer     *ShadowDeviceOptimizer
	notifyPool    *shadowNotifyPool
	notifyWorkers int
}

func NewShadowCore() *ShadowCore {
	return NewShadowCoreWithNotifyWorkers(defaultNotifyWorkers)
}

func NewShadowCoreWithNotifyWorkers(workers int) *ShadowCore {
	sc := &ShadowCore{
		realShadows:    make(map[string]*shadowDeviceEntry),
		virtualShadows: make(map[string]*model.VirtualDevice),
		subscribers:    make([]ShadowSubscriber, 0),
		optimizer:      NewShadowDeviceOptimizer(),
		notifyWorkers:  workers,
	}
	sc.notifyPool = newShadowNotifyPool(workers, sc.dispatchNotify)
	return sc
}

func (sc *ShadowCore) Start() {
	sc.notifyPool.Start()
	log.Println("[ShadowCore] Started (memory-only)")
}

func (sc *ShadowCore) Stop() {
	sc.notifyPool.Stop()
	log.Println("[ShadowCore] Stopped")
}

func (sc *ShadowCore) dispatchNotify(deviceID string, points map[string]model.ShadowPoint) {
	sc.subMu.RLock()
	subs := sc.subscribers
	sc.subMu.RUnlock()

	for _, sub := range subs {
		sub(deviceID, points)
	}
}

func (sc *ShadowCore) enqueueNotify(deviceID string, points map[string]model.ShadowPoint) {
	sc.notifyPool.Enqueue(deviceID, points)
}

func (sc *ShadowCore) WriteShadowDevice(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
	return sc.applyShadowWrite(msg)
}

// ApplyShadowWrites applies multiple ingress messages under a single lock,
// emitting one delta notify per device touched.
func (sc *ShadowCore) ApplyShadowWrites(msgs []model.ShadowIngressMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	if len(msgs) == 1 {
		_, err := sc.applyShadowWrite(msgs[0])
		return err
	}

	sc.mu.Lock()

	type deviceNotify struct {
		shadowDeviceID string
		prevPoints     map[string]model.ShadowPoint
		changed        map[string]model.ShadowPoint
	}
	pending := make(map[string]*deviceNotify)

	for _, msg := range msgs {
		shadowDeviceID, version, updatedAt, changed, prevPoints, _, err := sc.applyShadowWriteLocked(msg)
		if err != nil {
			for _, n := range pending {
				returnShadowPointsMap(n.changed)
			}
			return err
		}
		if len(changed) == 0 {
			returnShadowPointsMap(changed)
			continue
		}
		n, ok := pending[shadowDeviceID]
		if !ok {
			n = &deviceNotify{
				shadowDeviceID: shadowDeviceID,
				prevPoints:     prevPoints,
				changed:        borrowShadowPointsMap(len(changed)),
			}
			pending[shadowDeviceID] = n
		}
		for pid, pt := range changed {
			pt.Version = version
			pt.UpdatedAt = updatedAt
			n.changed[pid] = pt
		}
		returnShadowPointsMap(changed)
	}
	sc.mu.Unlock()

	for _, n := range pending {
		notifyPoints := cloneShadowDeltaForNotify(n.prevPoints, n.changed)
		returnShadowPointsMap(n.changed)
		sc.enqueueNotify(n.shadowDeviceID, notifyPoints)
	}
	return nil
}

func (sc *ShadowCore) applyShadowWrite(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
	sc.mu.Lock()
	shadowDeviceID, version, updatedAt, changed, prevPoints, _, err := sc.applyShadowWriteLocked(msg)
	sc.mu.Unlock()
	if err != nil {
		returnShadowPointsMap(changed)
		return nil, err
	}
	notifyPoints := cloneShadowDeltaForNotify(prevPoints, changed)
	returnShadowPointsMap(changed)
	sc.enqueueNotify(shadowDeviceID, notifyPoints)
	return &model.ShadowWriteResponse{
		Success:   true,
		Version:   version,
		Timestamp: updatedAt,
	}, nil
}

func (sc *ShadowCore) applyShadowWriteLocked(msg model.ShadowIngressMessage) (
	shadowDeviceID string,
	version uint64,
	updatedAt time.Time,
	changed map[string]model.ShadowPoint,
	prevPoints map[string]model.ShadowPoint,
	profile *model.DeviceCommunicationProfile,
	err error,
) {
	shadowDeviceID = "shadow-" + msg.DeviceID

	entry, exists := sc.realShadows[shadowDeviceID]
	if !exists {
		entry = newShadowDeviceEntry()
		sc.realShadows[shadowDeviceID] = entry
	}
	prev := entry.load()
	if prev != nil {
		prevPoints = prev.Points
	}

	version = sc.versionCounter.Add(1)
	updatedAt = time.Now()

	changed = borrowShadowPointsMap(len(msg.Points))
	for _, point := range msg.Points {
		collectedAt := point.CollectedAt
		if collectedAt.IsZero() {
			collectedAt = msg.Timestamp
		}
		if collectedAt.IsZero() {
			collectedAt = updatedAt
		}
		shadowPoint := model.ShadowPoint{
			Value:          point.Value,
			Unit:           point.Unit,
			Quality:        point.Quality,
			Degraded:       point.Degraded,
			SamplePeriodMs: point.SamplePeriodMs,
			Timestamp:      collectedAt,
			CollectedAt:    collectedAt,
			UpdatedAt:      updatedAt,
			Version:        version,
		}
		changed[point.PointID] = shadowPoint
	}

	channelID := msg.ChannelID
	if prev != nil && channelID == "" {
		channelID = prev.ChannelID
	}

	var commProfile *model.DeviceCommunicationProfile
	if prev != nil {
		commProfile = prev.CommunicationProfile
	}
	if sc.optimizer.UpdateShadowDeviceProfileIfNeeded(msg.DeviceID, channelID, &commProfile) {
		// profile updated or first publish
	}

	snap := buildSnapshotFromEntry(
		prev, shadowDeviceID, msg.DeviceID, channelID,
		version, updatedAt, changed, commProfile,
	)
	entry.publish(snap)
	return shadowDeviceID, version, updatedAt, changed, prevPoints, commProfile, nil
}

// UpdateDeviceRTT 更新设备的RTT数据
func (sc *ShadowCore) UpdateDeviceRTT(deviceID string, rtt int64) {
	sc.optimizer.UpdateDeviceRTT(deviceID, rtt)
	shadowDeviceID := "shadow-" + deviceID
	sc.mu.Lock()
	entry, exists := sc.realShadows[shadowDeviceID]
	if !exists {
		sc.mu.Unlock()
		return
	}
	prev := entry.load()
	if prev == nil {
		sc.mu.Unlock()
		return
	}
	profile := prev.CommunicationProfile
	if !sc.optimizer.UpdateShadowDeviceProfileIfNeeded(deviceID, prev.ChannelID, &profile) {
		sc.mu.Unlock()
		return
	}
	snap := &cowShadowSnapshot{
		ShadowDeviceID:       prev.ShadowDeviceID,
		PhysicalDeviceID:     prev.PhysicalDeviceID,
		ChannelID:            prev.ChannelID,
		Version:              prev.Version,
		UpdatedAt:            prev.UpdatedAt,
		Points:               prev.Points,
		CommunicationProfile: profile,
	}
	entry.publish(snap)
	sc.mu.Unlock()
}

// GetDeviceOptimization 返回设备 RTT/MTU/Gap 通信画像（微秒 RTT）。
func (sc *ShadowCore) GetDeviceOptimization(deviceID string) map[string]interface{} {
	if sc == nil || sc.optimizer == nil {
		return nil
	}
	return sc.optimizer.GetDeviceOptimization(deviceID)
}

func (sc *ShadowCore) WriteShadowPoint(req model.ShadowWriteRequest) (*model.ShadowWriteResponse, error) {
	sc.mu.Lock()

	entry, exists := sc.realShadows[req.ShadowDeviceID]
	if !exists {
		sc.mu.Unlock()
		return nil, fmt.Errorf("shadow device not found: %s", req.ShadowDeviceID)
	}
	prev := entry.load()
	if prev == nil {
		sc.mu.Unlock()
		return nil, fmt.Errorf("shadow device not found: %s", req.ShadowDeviceID)
	}

	version := sc.versionCounter.Add(1)
	updatedAt := time.Now()

	shadowPoint := model.ShadowPoint{
		Value:       req.Value,
		Timestamp:   req.Timestamp,
		CollectedAt: req.Timestamp,
		UpdatedAt:   updatedAt,
		Version:     version,
		Quality:     "good",
	}

	changed := borrowShadowPointsMap(1)
	changed[req.PointID] = shadowPoint

	var prevPoints map[string]model.ShadowPoint
	if prev != nil {
		prevPoints = prev.Points
	}

	snap := buildSnapshotFromEntry(
		prev, prev.ShadowDeviceID, prev.PhysicalDeviceID, prev.ChannelID,
		version, updatedAt, changed, prev.CommunicationProfile,
	)
	entry.publish(snap)

	notifyPoints := cloneShadowDeltaForNotify(prevPoints, changed)
	returnShadowPointsMap(changed)
	sc.mu.Unlock()
	sc.enqueueNotify(req.ShadowDeviceID, notifyPoints)

	return &model.ShadowWriteResponse{
		Success:   true,
		Version:   version,
		Timestamp: updatedAt,
	}, nil
}

func (sc *ShadowCore) GetShadowDevice(deviceID string) (*model.ShadowDevice, error) {
	sc.mu.RLock()
	entry, exists := sc.realShadows[deviceID]
	sc.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}
	snap := entry.load()
	if snap == nil {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}
	return viewFromSnapshot(snap), nil
}

func (sc *ShadowCore) GetAllShadowDevices() []*model.ShadowDevice {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	result := make([]*model.ShadowDevice, 0, len(sc.realShadows))
	for _, entry := range sc.realShadows {
		if snap := entry.load(); snap != nil {
			result = append(result, viewFromSnapshot(snap))
		}
	}
	return result
}

// RuntimePointStats returns in-memory shadow device and point counts for database stats.
func (sc *ShadowCore) RuntimePointStats() (deviceCount, pointCount int) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	for _, entry := range sc.realShadows {
		snap := entry.load()
		if snap == nil {
			continue
		}
		deviceCount++
		pointCount += len(snap.Points)
	}
	for _, vd := range sc.virtualShadows {
		if vd == nil {
			continue
		}
		deviceCount++
		pointCount += len(vd.Points)
	}
	return deviceCount, pointCount
}

func (sc *ShadowCore) GetShadowPoint(deviceID, pointID string) (*model.ShadowPoint, error) {
	sc.mu.RLock()
	entry, exists := sc.realShadows[deviceID]
	sc.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}
	snap := entry.load()
	if snap == nil {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}

	point, exists := snap.Points[pointID]
	if !exists {
		return nil, fmt.Errorf("point not found: %s", pointID)
	}

	copy := point
	return &copy, nil
}

func (sc *ShadowCore) CompareAndSwap(deviceID string, expectedVersion uint64, updates map[string]any) (*model.ShadowWriteResponse, error) {
	sc.mu.Lock()

	entry, exists := sc.realShadows[deviceID]
	if !exists {
		sc.mu.Unlock()
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}
	prev := entry.load()
	if prev == nil {
		sc.mu.Unlock()
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}

	if prev.Version != expectedVersion {
		sc.mu.Unlock()
		return &model.ShadowWriteResponse{
			Success: false,
			Version: prev.Version,
			Error:   "version mismatch",
		}, nil
	}

	version := sc.versionCounter.Add(1)
	updatedAt := time.Now()

	changed := borrowShadowPointsMap(len(updates))
	for pointID, value := range updates {
		if point, exists := prev.Points[pointID]; exists {
			point.Value = value
			point.Version = version
			point.UpdatedAt = updatedAt
			point.Timestamp = updatedAt
			point.CollectedAt = updatedAt
			changed[pointID] = point
		}
	}

	snap := buildSnapshotFromEntry(
		prev, prev.ShadowDeviceID, prev.PhysicalDeviceID, prev.ChannelID,
		version, updatedAt, changed, prev.CommunicationProfile,
	)
	entry.publish(snap)

	notifyPoints := cloneShadowDeltaForNotify(prev.Points, changed)
	returnShadowPointsMap(changed)
	sc.mu.Unlock()
	sc.enqueueNotify(deviceID, notifyPoints)

	return &model.ShadowWriteResponse{
		Success:   true,
		Version:   version,
		Timestamp: updatedAt,
	}, nil
}

func (sc *ShadowCore) Subscribe(sub ShadowSubscriber) {
	sc.subMu.Lock()
	defer sc.subMu.Unlock()
	sc.subscribers = append(sc.subscribers, sub)
}

func (sc *ShadowCore) CheckConsistency(deviceID string, t time.Time) (*model.ConsistencyCheckResult, error) {
	sc.mu.RLock()
	entry, exists := sc.realShadows[deviceID]
	sc.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}
	snap := entry.load()
	if snap == nil {
		return nil, fmt.Errorf("shadow device not found: %s", deviceID)
	}

	result := &model.ConsistencyCheckResult{
		Pass:       true,
		DiffPoints: make([]model.ShadowDiffPoint, 0),
	}

	for pointID, point := range snap.Points {
		if point.Timestamp.Before(t) {
			continue
		}

		if point.Quality != "good" {
			result.Pass = false
			result.DiffPoints = append(result.DiffPoints, model.ShadowDiffPoint{
				PointID:  pointID,
				Field:    "quality",
				Expected: "good",
				Actual:   point.Quality,
			})
		}
	}

	if !result.Pass {
		result.DiffSource = "quality_check"
		result.RepairSuggest = "re-collect data from source"
	}

	return result, nil
}

func (sc *ShadowCore) GetMetrics() map[string]interface{} {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return map[string]interface{}{
		"real_shadow_count":    len(sc.realShadows),
		"virtual_shadow_count": len(sc.virtualShadows),
		"version_counter":      sc.versionCounter.Load(),
		"notify_workers":       sc.notifyWorkers,
	}
}

func (sc *ShadowCore) DeleteShadowDevice(deviceID string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if _, exists := sc.realShadows[deviceID]; !exists {
		return fmt.Errorf("shadow device not found: %s", deviceID)
	}

	delete(sc.realShadows, deviceID)

	return nil
}

// ClearAllShadowDevices 清空全部内存态影子设备（含虚拟影子与优化画像）。
func (sc *ShadowCore) ClearAllShadowDevices() {
	sc.mu.Lock()
	for deviceID := range sc.realShadows {
		sc.optimizer.ClearDeviceData(deviceID)
	}
	sc.realShadows = make(map[string]*shadowDeviceEntry)
	sc.virtualShadows = make(map[string]*model.VirtualDevice)
	sc.mu.Unlock()
}

// WriteVirtualShadowDevice 将虚拟影子计算结果写入 ShadowCore 并通知订阅者（Pipeline / UI）。
func (sc *ShadowCore) WriteVirtualShadowDevice(channelID, virtualDeviceID string, points map[string]model.ShadowPoint) {
	if len(points) == 0 {
		return
	}

	sc.mu.Lock()
	vd, exists := sc.virtualShadows[virtualDeviceID]
	var prevPoints map[string]model.ShadowPoint
	if !exists {
		vd = &model.VirtualDevice{
			VirtualDeviceID: virtualDeviceID,
			ChannelID:       channelID,
			Points:          make(map[string]model.ShadowPoint),
		}
		sc.virtualShadows[virtualDeviceID] = vd
	} else if len(vd.Points) > 0 {
		// Snapshot prior values before in-place mutation (vd.Points is mutated below).
		prevPoints = make(map[string]model.ShadowPoint, len(points))
		for pid := range points {
			if old, ok := vd.Points[pid]; ok {
				prevPoints[pid] = old
			}
		}
	}
	if channelID != "" {
		vd.ChannelID = channelID
	}

	version := sc.versionCounter.Add(1)
	now := time.Now()
	vd.Version = version
	vd.UpdatedAt = now

	for pid, pt := range points {
		pt.Version = version
		if pt.UpdatedAt.IsZero() {
			pt.UpdatedAt = now
		}
		if pt.Timestamp.IsZero() {
			pt.Timestamp = now
			pt.CollectedAt = now
		}
		if pt.Quality == "" {
			pt.Quality = "good"
		}
		vd.Points[pid] = pt
	}
	sc.mu.Unlock()

	sc.enqueueNotify(VirtualShadowID(virtualDeviceID), cloneShadowDeltaForNotify(prevPoints, points))
}

// ResolvePublishTarget 解析订阅通知中的 channel / device，供 ShadowBridge 与 WebSocket 使用。
func (sc *ShadowCore) ResolvePublishTarget(shadowDeviceID string) (channelID, deviceID string, err error) {
	if IsVirtualShadowID(shadowDeviceID) {
		virtualID := strings.TrimPrefix(shadowDeviceID, VirtualShadowPrefix)
		sc.mu.RLock()
		vd, ok := sc.virtualShadows[virtualID]
		sc.mu.RUnlock()
		if !ok {
			return "", "", fmt.Errorf("virtual shadow not found: %s", virtualID)
		}
		return vd.ChannelID, virtualID, nil
	}

	sc.mu.RLock()
	entry, ok := sc.realShadows[shadowDeviceID]
	sc.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("shadow device not found: %s", shadowDeviceID)
	}
	snap := entry.load()
	if snap == nil {
		return "", "", fmt.Errorf("shadow device not found: %s", shadowDeviceID)
	}
	return snap.ChannelID, snap.PhysicalDeviceID, nil
}

// GetVirtualShadowPoint 读取虚拟影子设备单个点位。
func (sc *ShadowCore) GetVirtualShadowPoint(virtualDeviceID, pointID string) (*model.ShadowPoint, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	vd, exists := sc.virtualShadows[virtualDeviceID]
	if !exists {
		return nil, fmt.Errorf("virtual shadow device not found: %s", virtualDeviceID)
	}

	point, exists := vd.Points[pointID]
	if !exists {
		return nil, fmt.Errorf("point not found: %s", pointID)
	}

	copy := point
	return &copy, nil
}

// GetVirtualShadowDevice 返回虚拟影子设备快照。
func (sc *ShadowCore) GetVirtualShadowDevice(virtualDeviceID string) (*model.VirtualDevice, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	vd, exists := sc.virtualShadows[virtualDeviceID]
	if !exists {
		return nil, fmt.Errorf("virtual shadow device not found: %s", virtualDeviceID)
	}

	copy := *vd
	copy.Points = cloneShadowPoints(vd.Points)
	return &copy, nil
}

func (sc *ShadowCore) NotifyWorkerCount() int {
	return sc.notifyWorkers
}
