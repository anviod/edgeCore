package core

import (
	"encoding/json"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

const storeForwardSouthKey = "southbound_values"

// StoreForwardPolicy 统一南向历史与北向离线缓存策略。
type StoreForwardPolicy struct {
	MaxSouthboundRecords int
	MaxNorthboundPerID   int
}

// StoreForwardManager 统一 Store & Forward：南向 values 与北向 NorthboundCache。
type StoreForwardManager struct {
	store  *storage.Storage
	policy StoreForwardPolicy
}

func NewStoreForwardManager(store *storage.Storage, policy StoreForwardPolicy) *StoreForwardManager {
	if policy.MaxSouthboundRecords <= 0 {
		policy.MaxSouthboundRecords = 10000
	}
	if policy.MaxNorthboundPerID <= 0 {
		policy.MaxNorthboundPerID = 1000
	}
	return &StoreForwardManager{store: store, policy: policy}
}

func (m *StoreForwardManager) SetStorage(store *storage.Storage) {
	m.store = store
}

// HandleBatch 作为 Pipeline 批量处理器缓存南向采集值。
func (m *StoreForwardManager) HandleBatch(batch []model.Value) {
	if m == nil || m.store == nil || len(batch) == 0 {
		return
	}
	for _, v := range batch {
		m.cacheSouthbound(v)
	}
}

func (m *StoreForwardManager) cacheSouthbound(v model.Value) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	key := v.ChannelID + "/" + v.DeviceID + "/" + v.PointID + "_" + time.Now().Format(time.RFC3339Nano)
	_ = m.store.SaveData(storage.BucketDataCache, storeForwardSouthKey+"/"+key, payload)
	m.pruneSouthbound()
}

func (m *StoreForwardManager) pruneSouthbound() {
	count := 0
	_ = m.store.LoadAll(storage.BucketDataCache, func(k, _ []byte) error {
		if len(k) > len(storeForwardSouthKey) && string(k[:len(storeForwardSouthKey)]) == storeForwardSouthKey {
			count++
		}
		return nil
	})
	if count <= m.policy.MaxSouthboundRecords {
		return
	}
	toDelete := count - m.policy.MaxSouthboundRecords
	_ = m.store.LoadAll(storage.BucketDataCache, func(k, _ []byte) error {
		if toDelete <= 0 {
			return nil
		}
		if len(k) > len(storeForwardSouthKey) && string(k[:len(storeForwardSouthKey)]) == storeForwardSouthKey {
			_ = m.store.DeleteData(storage.BucketDataCache, string(k))
			toDelete--
		}
		return nil
	})
}

// CacheNorthbound 缓存北向离线消息（复用 NorthboundCache bucket）。
func (m *StoreForwardManager) CacheNorthbound(configID string, data []byte) error {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.SaveOfflineMessage(configID, data, m.policy.MaxNorthboundPerID)
}

// ReplaySouthbound 回放最近缓存的南向值。
func (m *StoreForwardManager) ReplaySouthbound(limit int) []model.Value {
	if m == nil || m.store == nil || limit <= 0 {
		return nil
	}
	var out []model.Value
	_ = m.store.LoadAll(storage.BucketDataCache, func(k, v []byte) error {
		if len(out) >= limit {
			return nil
		}
		if len(k) <= len(storeForwardSouthKey) || string(k[:len(storeForwardSouthKey)]) != storeForwardSouthKey {
			return nil
		}
		var val model.Value
		if err := json.Unmarshal(v, &val); err == nil {
			out = append(out, val)
		}
		return nil
	})
	return out
}
