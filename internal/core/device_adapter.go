package core

import (
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// DeviceAdapter 设备适配器接口
type DeviceAdapter interface {
	GetDeviceProfile(deviceID string) (*model.DeviceCommunicationProfile, error)
	UpdateDeviceProfile(profile *model.DeviceCommunicationProfile) error
	OptimizeCollectionParams(deviceID string) (map[string]interface{}, error)
	UpdateRTT(deviceID string, rtt int64) error
	UpdateMTU(deviceID string, mtu int) error
	UpdateGap(deviceID string, gap int) error
	CalculateStabilityScore(deviceID string) (float64, error)
}

// DeviceAdapterManager 设备适配器管理器
type DeviceAdapterManager struct {
	adapters     map[string]DeviceAdapter
	profiles     map[string]*model.DeviceCommunicationProfile
	rttManager   *RTTManager
	mtuManager   *MTUManager
	gapOptimizer *GapOptimizer
	mu           sync.RWMutex
}

// NewDeviceAdapterManager 创建设备适配器管理器
func NewDeviceAdapterManager() *DeviceAdapterManager {
	return &DeviceAdapterManager{
		adapters:     make(map[string]DeviceAdapter),
		profiles:     make(map[string]*model.DeviceCommunicationProfile),
		rttManager:   NewRTTManager(),
		mtuManager:   NewMTUManager(),
		gapOptimizer: NewGapOptimizer(),
	}
}

// RegisterAdapter 注册设备适配器
func (m *DeviceAdapterManager) RegisterAdapter(deviceID string, adapter DeviceAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.adapters[deviceID] = adapter
}

// GetAdapter 获取设备适配器
func (m *DeviceAdapterManager) GetAdapter(deviceID string) DeviceAdapter {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if adapter, exists := m.adapters[deviceID]; exists {
		return adapter
	}

	// 如果没有适配器，创建一个默认的
	adapter := NewDefaultDeviceAdapter(m.rttManager, m.mtuManager, m.gapOptimizer, m)
	m.mu.RUnlock()
	m.mu.Lock()
	m.adapters[deviceID] = adapter
	m.mu.Unlock()
	m.mu.RLock()
	return adapter
}

// GetDeviceProfile 获取设备画像
func (m *DeviceAdapterManager) GetDeviceProfile(deviceID string) (*model.DeviceCommunicationProfile, error) {
	m.mu.RLock()
	profile, exists := m.profiles[deviceID]
	m.mu.RUnlock()

	if exists {
		return profile, nil
	}

	// 如果没有画像，创建一个默认的
	profile = &model.DeviceCommunicationProfile{
		DeviceID:              deviceID,
		AvgResponseTime:       0,
		MaxResponseTime:       0,
		ErrorRate:             0,
		StabilityScore:        100,
		OptimalTimeout:        5 * time.Second,
		OptimalInterval:       10 * time.Second,
		RetryCount:            3,
		BatchSize:             100,
		ProtocolParams:        make(map[string]interface{}),
		LastUpdated:           time.Now(),
		CollectionSuccessRate: 100,
		AbnormalPointCount:    0,
		ConsecutiveFailures:   0,
		RTTSamples:            make([]int64, 0),
		RTTSampleWindow:       20,
		EWMARTT:               0,
		CurrentMTU:            512,
		MaxMTU:                1500,
		MinMTU:                128,
		CurrentGap:            64,
		MaxGap:                256,
		GapFillStrategy:       0,
		HeartbeatInterval:     60,
		LastActivity:          time.Now(),
	}

	m.mu.Lock()
	m.profiles[deviceID] = profile
	m.mu.Unlock()

	return profile, nil
}

// UpdateDeviceProfile 更新设备画像
func (m *DeviceAdapterManager) UpdateDeviceProfile(profile *model.DeviceCommunicationProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile.LastUpdated = time.Now()
	m.profiles[profile.DeviceID] = profile
	return nil
}

// DefaultDeviceAdapter 默认设备适配器
type DefaultDeviceAdapter struct {
	rttManager   *RTTManager
	mtuManager   *MTUManager
	gapOptimizer *GapOptimizer
	manager      *DeviceAdapterManager
}

// NewDefaultDeviceAdapter 创建默认设备适配器
func NewDefaultDeviceAdapter(rttManager *RTTManager, mtuManager *MTUManager, gapOptimizer *GapOptimizer, manager *DeviceAdapterManager) *DefaultDeviceAdapter {
	return &DefaultDeviceAdapter{
		rttManager:   rttManager,
		mtuManager:   mtuManager,
		gapOptimizer: gapOptimizer,
		manager:      manager,
	}
}

// GetDeviceProfile 获取设备画像
func (a *DefaultDeviceAdapter) GetDeviceProfile(deviceID string) (*model.DeviceCommunicationProfile, error) {
	return a.manager.GetDeviceProfile(deviceID)
}

// UpdateDeviceProfile 更新设备画像
func (a *DefaultDeviceAdapter) UpdateDeviceProfile(profile *model.DeviceCommunicationProfile) error {
	return a.manager.UpdateDeviceProfile(profile)
}

// OptimizeCollectionParams 优化采集参数
func (a *DefaultDeviceAdapter) OptimizeCollectionParams(deviceID string) (map[string]interface{}, error) {
	// 获取设备画像
	profile, err := a.GetDeviceProfile(deviceID)
	if err != nil {
		return nil, err
	}

	// 获取RTT
	rtt := a.rttManager.GetEWMARTT(deviceID)
	if rtt == 0 {
		rtt = 100000 // 默认100ms
	}

	// 协商MTU
	mtu := a.mtuManager.NegotiateMTU(deviceID, rtt)

	// 优化Gap
	gap := a.gapOptimizer.OptimizeGap(deviceID, mtu, rtt)

	// 更新设备画像
	profile.EWMARTT = rtt
	profile.CurrentMTU = mtu
	profile.CurrentGap = gap
	profile.OptimalTimeout = time.Duration(rtt*3) * time.Microsecond
	if profile.OptimalTimeout < 1*time.Second {
		profile.OptimalTimeout = 1 * time.Second
	} else if profile.OptimalTimeout > 10*time.Second {
		profile.OptimalTimeout = 10 * time.Second
	}

	a.UpdateDeviceProfile(profile)

	// 返回优化后的参数
	params := map[string]interface{}{
		"rtt":     rtt,
		"mtu":     mtu,
		"gap":     gap,
		"timeout": profile.OptimalTimeout,
	}

	return params, nil
}

// UpdateRTT 更新RTT
func (a *DefaultDeviceAdapter) UpdateRTT(deviceID string, rtt int64) error {
	a.rttManager.UpdateRTT(deviceID, rtt)

	// 更新设备画像
	profile, err := a.GetDeviceProfile(deviceID)
	if err != nil {
		return err
	}

	profile.EWMARTT = a.rttManager.GetEWMARTT(deviceID)
	profile.RTTSamples = a.rttManager.GetRTTSamples(deviceID)
	a.UpdateDeviceProfile(profile)

	return nil
}

// UpdateMTU 更新MTU
func (a *DefaultDeviceAdapter) UpdateMTU(deviceID string, mtu int) error {
	a.mtuManager.SetMTU(deviceID, mtu)

	// 更新设备画像
	profile, err := a.GetDeviceProfile(deviceID)
	if err != nil {
		return err
	}

	profile.CurrentMTU = mtu
	a.UpdateDeviceProfile(profile)

	return nil
}

// UpdateGap 更新Gap
func (a *DefaultDeviceAdapter) UpdateGap(deviceID string, gap int) error {
	a.gapOptimizer.SetGap(deviceID, gap)

	// 更新设备画像
	profile, err := a.GetDeviceProfile(deviceID)
	if err != nil {
		return err
	}

	profile.CurrentGap = gap
	a.UpdateDeviceProfile(profile)

	return nil
}

// CalculateStabilityScore 计算稳定性评分
func (a *DefaultDeviceAdapter) CalculateStabilityScore(deviceID string) (float64, error) {
	// 获取设备画像
	profile, err := a.GetDeviceProfile(deviceID)
	if err != nil {
		return 0, err
	}

	// 基于错误率和连续失败次数计算稳定性评分
	score := 100.0
	score -= profile.ErrorRate * 100
	score -= float64(profile.ConsecutiveFailures) * 5

	if score < 0 {
		score = 0
	}

	// 更新设备画像
	profile.StabilityScore = score
	a.UpdateDeviceProfile(profile)

	return score, nil
}
