package core

import (
	"log"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

const (
	profileRTTAbsThresholdUs = int64(10000) // 10ms
	profileRTTPctThreshold   = 0.05         // 5%
)

// ShadowDeviceOptimizer 影子设备优化器，集成RTT、MTU、Gap优化
type ShadowDeviceOptimizer struct {
	rttManager     *RTTManager
	mtuManager     *MTUManager
	gapOptimizer   *GapOptimizer
	lastProfileRTT map[string]int64
	mu             sync.RWMutex
}

// NewShadowDeviceOptimizer 创建影子设备优化器
func NewShadowDeviceOptimizer() *ShadowDeviceOptimizer {
	return &ShadowDeviceOptimizer{
		rttManager:     NewRTTManager(),
		mtuManager:     NewMTUManager(),
		gapOptimizer:   NewGapOptimizer(),
		lastProfileRTT: make(map[string]int64),
	}
}

// UpdateDeviceRTT 更新设备RTT并触发相关优化
func (sdo *ShadowDeviceOptimizer) UpdateDeviceRTT(deviceID string, rtt int64) {
	sdo.rttManager.UpdateRTT(deviceID, rtt)

	// 根据RTT更新MTU
	sdo.mtuManager.NegotiateMTU(deviceID, rtt)

	// 获取当前MTU
	mtu := sdo.mtuManager.GetCurrentMTU(deviceID)

	// 根据MTU和RTT更新Gap
	sdo.gapOptimizer.OptimizeGap(deviceID, mtu, rtt)
}

// GetDeviceOptimization 获取设备优化参数
func (sdo *ShadowDeviceOptimizer) GetDeviceOptimization(deviceID string) map[string]interface{} {
	rtt := sdo.rttManager.GetEWMARTT(deviceID)
	mtu := sdo.mtuManager.GetCurrentMTU(deviceID)
	gap := sdo.gapOptimizer.GetCurrentGap(deviceID)

	return map[string]interface{}{
		"rtt": rtt,
		"mtu": mtu,
		"gap": gap,
	}
}

func (sdo *ShadowDeviceOptimizer) profileRTTChanged(deviceID string, currentRTT int64) bool {
	if currentRTT == 0 {
		return false
	}
	sdo.mu.RLock()
	prev, ok := sdo.lastProfileRTT[deviceID]
	sdo.mu.RUnlock()
	if !ok {
		return true
	}
	delta := currentRTT - prev
	if delta < 0 {
		delta = -delta
	}
	if delta >= profileRTTAbsThresholdUs {
		return true
	}
	if prev > 0 && float64(delta)/float64(prev) >= profileRTTPctThreshold {
		return true
	}
	return false
}

func (sdo *ShadowDeviceOptimizer) recordProfileRTT(deviceID string, rtt int64) {
	sdo.mu.Lock()
	sdo.lastProfileRTT[deviceID] = rtt
	sdo.mu.Unlock()
}

// UpdateShadowDeviceProfileIfNeeded 仅在 RTT 变化超过阈值时更新通信画像。
// profilePtr 指向当前快照上的 profile 指针；更新时会替换为新克隆。
func (sdo *ShadowDeviceOptimizer) UpdateShadowDeviceProfileIfNeeded(deviceID, channelID string, profilePtr **model.DeviceCommunicationProfile) bool {
	if profilePtr == nil {
		return false
	}
	rtt := sdo.rttManager.GetEWMARTT(deviceID)
	if *profilePtr == nil {
		profile := sdo.buildCommunicationProfile(deviceID, channelID, rtt)
		*profilePtr = profile
		sdo.recordProfileRTT(deviceID, rtt)
		return true
	}
	if !sdo.profileRTTChanged(deviceID, rtt) {
		return false
	}
	updated := sdo.buildCommunicationProfile(deviceID, (*profilePtr).ChannelID, rtt)
	*profilePtr = updated
	sdo.recordProfileRTT(deviceID, rtt)
	return true
}

func (sdo *ShadowDeviceOptimizer) buildCommunicationProfile(deviceID, channelID string, rtt int64) *model.DeviceCommunicationProfile {
	mtu := sdo.mtuManager.GetCurrentMTU(deviceID)
	gap := sdo.gapOptimizer.GetCurrentGap(deviceID)
	rttSamples := sdo.rttManager.GetRTTSamples(deviceID)
	return &model.DeviceCommunicationProfile{
		DeviceID:        deviceID,
		ChannelID:       channelID,
		ProtocolType:    "",
		LastUpdated:     time.Now(),
		RTTSamples:      rttSamples,
		RTTSampleWindow: 20,
		EWMARTT:         rtt,
		CurrentMTU:      mtu,
		MaxMTU:          1500,
		MinMTU:          128,
		CurrentGap:      gap,
		MaxGap:          512,
		GapFillStrategy: 1,
	}
}

// UpdateShadowDeviceProfile 更新影子设备的通信画像（无条件，供测试与显式刷新）。
func (sdo *ShadowDeviceOptimizer) UpdateShadowDeviceProfile(shadowDevice *model.ShadowDevice) {
	if shadowDevice == nil {
		return
	}
	rtt := sdo.rttManager.GetEWMARTT(shadowDevice.PhysicalDeviceID)
	shadowDevice.CommunicationProfile = sdo.buildCommunicationProfile(
		shadowDevice.PhysicalDeviceID,
		shadowDevice.ChannelID,
		rtt,
	)
	sdo.recordProfileRTT(shadowDevice.PhysicalDeviceID, rtt)
}

// GetRTTManager 获取RTT管理器
func (sdo *ShadowDeviceOptimizer) GetRTTManager() *RTTManager {
	return sdo.rttManager
}

// GetMTUManager 获取MTU管理器
func (sdo *ShadowDeviceOptimizer) GetMTUManager() *MTUManager {
	return sdo.mtuManager
}

// GetGapOptimizer 获取Gap优化器
func (sdo *ShadowDeviceOptimizer) GetGapOptimizer() *GapOptimizer {
	return sdo.gapOptimizer
}

// ClearDeviceData 清除设备数据
func (sdo *ShadowDeviceOptimizer) ClearDeviceData(deviceID string) {
	sdo.rttManager.ClearRTTData(deviceID)
	sdo.mtuManager.ClearMTUData(deviceID)
	sdo.gapOptimizer.ClearGapData(deviceID)
	sdo.mu.Lock()
	delete(sdo.lastProfileRTT, deviceID)
	sdo.mu.Unlock()
}

// GetAllDevices 获取所有设备的优化数据
func (sdo *ShadowDeviceOptimizer) GetAllDevices() []string {
	// 这里需要实现获取所有设备ID的逻辑
	// 暂时返回空列表
	return []string{}
}

// LogDeviceOptimization 记录设备优化数据
func (sdo *ShadowDeviceOptimizer) LogDeviceOptimization(deviceID string) {
	opts := sdo.GetDeviceOptimization(deviceID)
	log.Printf("Device %s optimization: RTT=%d, MTU=%d, Gap=%d",
		deviceID, opts["rtt"], opts["mtu"], opts["gap"])
}
