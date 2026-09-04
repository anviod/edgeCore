package core

import (
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// MTUManager MTU管理器
type MTUManager struct {
	mtuData    map[string]int
	mtuHistory map[string][]model.MTUNegotiationRecord
	minMTU     map[string]int
	maxMTU     map[string]int
	hysteresis float64
	mu         sync.RWMutex
}

// NewMTUManager 创建MTU管理器
func NewMTUManager() *MTUManager {
	return &MTUManager{
		mtuData:    make(map[string]int),
		mtuHistory: make(map[string][]model.MTUNegotiationRecord),
		minMTU:     make(map[string]int),
		maxMTU:     make(map[string]int),
		hysteresis: 0.1,
	}
}

// NegotiateMTU 协商MTU
func (m *MTUManager) NegotiateMTU(deviceID string, rtt int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 初始化设备的MTU数据
	if _, exists := m.mtuData[deviceID]; !exists {
		m.mtuData[deviceID] = 512 // 默认MTU
		m.minMTU[deviceID] = 128
		m.maxMTU[deviceID] = 1500
		m.mtuHistory[deviceID] = make([]model.MTUNegotiationRecord, 0, 10)
	}

	currentMTU := m.mtuData[deviceID]

	// 根据RTT调整MTU
	var newMTU int
	if rtt < 100000 { // RTT < 100ms，尝试增加MTU
		newMTU = currentMTU * 2
		if newMTU > m.maxMTU[deviceID] {
			newMTU = m.maxMTU[deviceID]
		}
	} else if rtt > 500000 { // RTT > 500ms，尝试减少MTU
		newMTU = currentMTU / 2
		if newMTU < m.minMTU[deviceID] {
			newMTU = m.minMTU[deviceID]
		}
	} else {
		// RTT在合理范围内，保持当前MTU
		return currentMTU
	}

	// 记录协商结果
	record := model.MTUNegotiationRecord{
		AttemptValue: newMTU,
		ResponseTime: rtt,
		RetryCount:   0,
		Success:      true, // 假设成功，实际应用中需要根据实际结果更新
		Timestamp:    time.Now(),
	}

	m.mtuHistory[deviceID] = append(m.mtuHistory[deviceID], record)
	if len(m.mtuHistory[deviceID]) > 10 {
		m.mtuHistory[deviceID] = m.mtuHistory[deviceID][1:]
	}

	// 更新MTU
	m.mtuData[deviceID] = newMTU
	return newMTU
}

// GetCurrentMTU 获取当前MTU
func (m *MTUManager) GetCurrentMTU(deviceID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if mtu, exists := m.mtuData[deviceID]; exists {
		return mtu
	}
	return 512 // 默认MTU
}

// GetMTUHistory 获取MTU历史记录
func (m *MTUManager) GetMTUHistory(deviceID string) []model.MTUNegotiationRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if history, exists := m.mtuHistory[deviceID]; exists {
		// 返回副本，避免外部修改
		historyCopy := make([]model.MTUNegotiationRecord, len(history))
		copy(historyCopy, history)
		return historyCopy
	}
	return nil
}

// SetMTU 设置MTU
func (m *MTUManager) SetMTU(deviceID string, mtu int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.mtuData[deviceID]; !exists {
		m.minMTU[deviceID] = 128
		m.maxMTU[deviceID] = 1500
		m.mtuHistory[deviceID] = make([]model.MTUNegotiationRecord, 0, 10)
	}

	// 确保MTU在合理范围内
	if mtu < m.minMTU[deviceID] {
		mtu = m.minMTU[deviceID]
	} else if mtu > m.maxMTU[deviceID] {
		mtu = m.maxMTU[deviceID]
	}

	m.mtuData[deviceID] = mtu
}

// ClearMTUData 清除设备的MTU数据
func (m *MTUManager) ClearMTUData(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.mtuData, deviceID)
	delete(m.mtuHistory, deviceID)
	delete(m.minMTU, deviceID)
	delete(m.maxMTU, deviceID)
}
