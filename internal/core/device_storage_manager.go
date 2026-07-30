package core

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/storage"
)

const DefaultMaxHistoryRecords = 1000

// DeviceStorageManager 从 ShadowCore 读取影子设备全量点位，按策略写入 device_history_*。
type DeviceStorageManager struct {
	storage      *storage.Storage
	pipeline     *DataPipeline
	shadowCore   *ShadowCore
	mu           sync.Mutex
	deviceCfgs   map[string]model.DeviceStorage
	tickers      map[string]*time.Ticker
	stopChans    map[string]chan struct{}
	intervalUnit time.Duration // For testing
}

func NewDeviceStorageManager(s *storage.Storage, dp *DataPipeline) *DeviceStorageManager {
	dsm := &DeviceStorageManager{
		storage:      s,
		pipeline:     dp,
		deviceCfgs:   make(map[string]model.DeviceStorage),
		tickers:      make(map[string]*time.Ticker),
		stopChans:    make(map[string]chan struct{}),
		intervalUnit: time.Minute,
	}

	dp.AddHandler(dsm.handleValue)

	return dsm
}

// SetStorage 在安装完成后绑定运行时存储（采集值落库）。
func (m *DeviceStorageManager) SetStorage(s *storage.Storage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storage = s
}

// SetShadowCore 绑定影子设备核心，快照数据从影子设备全量点位读取。
func (m *DeviceStorageManager) SetShadowCore(sc *ShadowCore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shadowCore = sc
}

// IsStorageEnabled reports whether history storage is enabled for a device.
// Used by MCP get_point_history to give actionable feedback when no data exists.
// IsStorageEnabled 报告设备是否已启用历史存储
func (m *DeviceStorageManager) IsStorageEnabled(deviceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, ok := m.deviceCfgs[deviceID]
	return ok && cfg.Enable
}

func (m *DeviceStorageManager) handleValue(val model.Value) {
	m.mu.Lock()
	cfg, ok := m.deviceCfgs[val.DeviceID]
	m.mu.Unlock()

	if !ok || !cfg.Enable || cfg.Strategy != "realtime" {
		return
	}

	go m.saveSnapshot(val.DeviceID, time.Now())
}

// UpdateDeviceConfig updates the storage configuration for a device
func (m *DeviceStorageManager) UpdateDeviceConfig(deviceID string, cfg model.DeviceStorage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stop, ok := m.stopChans[deviceID]; ok {
		close(stop)
		delete(m.stopChans, deviceID)
	}
	if ticker, ok := m.tickers[deviceID]; ok {
		ticker.Stop()
		delete(m.tickers, deviceID)
	}

	m.deviceCfgs[deviceID] = cfg

	if !cfg.Enable {
		return
	}

	if cfg.Strategy == "" || cfg.Strategy == "minute_aligned" || cfg.Strategy == "interval" {
		intervalDuration := time.Duration(cfg.Interval) * m.intervalUnit
		if cfg.Interval <= 0 {
			cfg.Interval = 1
			intervalDuration = m.intervalUnit
		}

		if m.intervalUnit == time.Minute {
			now := time.Now()
			nextMinute := now.Truncate(time.Minute).Add(time.Minute)
			initialDelay := nextMinute.Sub(now)

			go func() {
				timer := time.NewTimer(initialDelay)
				defer timer.Stop()

				stop := make(chan struct{})
				m.mu.Lock()
				m.stopChans[deviceID] = stop
				m.mu.Unlock()

				select {
				case <-timer.C:
					m.saveSnapshot(deviceID, time.Now())
				case <-stop:
					return
				}

				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()

				for {
					select {
					case t := <-ticker.C:
						m.saveSnapshot(deviceID, t.Truncate(time.Minute))
					case <-stop:
						return
					}
				}
			}()
		} else {
			ticker := time.NewTicker(intervalDuration)
			stop := make(chan struct{})
			m.tickers[deviceID] = ticker
			m.stopChans[deviceID] = stop

			go func() {
				for {
					select {
					case t := <-ticker.C:
						m.saveSnapshot(deviceID, t)
					case <-stop:
						return
					}
				}
			}()
		}
	}
}

func (m *DeviceStorageManager) collectSnapshotFromShadow(deviceID string) map[string]any {
	m.mu.Lock()
	sc := m.shadowCore
	m.mu.Unlock()

	if sc == nil {
		return nil
	}

	shadowID := fmt.Sprintf("shadow-%s", deviceID)
	device, err := sc.GetShadowDevice(shadowID)
	if err != nil || len(device.Points) == 0 {
		return nil
	}

	data := make(map[string]any, len(device.Points))
	for pointID, pt := range device.Points {
		if pt.Value == nil {
			continue
		}
		data[pointID] = pt.Value
	}
	return data
}

func (m *DeviceStorageManager) saveSnapshot(deviceID string, ts time.Time) {
	data := m.collectSnapshotFromShadow(deviceID)
	if len(data) == 0 {
		return
	}

	m.mu.Lock()
	cfg := m.deviceCfgs[deviceID]
	m.mu.Unlock()

	record := map[string]any{
		"ts":   ts.Unix(),
		"data": data,
	}

	bucket := fmt.Sprintf("device_history_%s", deviceID)
	key := ts.Format(time.RFC3339Nano)

	if m.storage == nil {
		return
	}
	if err := m.storage.SaveData(bucket, key, record); err != nil {
		log.Printf("[Storage] Failed to save history for device %s: %v", deviceID, err)
		return
	}

	max := cfg.MaxRecords
	if max <= 0 {
		max = DefaultMaxHistoryRecords
	}

	go m.pruneHistory(bucket, max)
}

func (m *DeviceStorageManager) pruneHistory(bucket string, maxRecords int) {
	if m.storage == nil {
		return
	}
	m.storage.PruneOldest(bucket, maxRecords)
}

func (m *DeviceStorageManager) RemoveDevice(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stop, ok := m.stopChans[deviceID]; ok {
		close(stop)
		delete(m.stopChans, deviceID)
	}
	if ticker, ok := m.tickers[deviceID]; ok {
		ticker.Stop()
		delete(m.tickers, deviceID)
	}
	delete(m.deviceCfgs, deviceID)
}

func (m *DeviceStorageManager) ClearAllHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.storage == nil {
		return
	}

	for deviceID := range m.deviceCfgs {
		bucket := fmt.Sprintf("device_history_%s", deviceID)
		m.storage.ClearBucket(bucket)
	}
}

// HistoryBucketNames returns device_history_* bucket names for enabled storage configs.
func (m *DeviceStorageManager) HistoryBucketNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.deviceCfgs))
	for deviceID, cfg := range m.deviceCfgs {
		if cfg.Enable {
			names = append(names, fmt.Sprintf("device_history_%s", deviceID))
		}
	}
	sort.Strings(names)
	return names
}

func (m *DeviceStorageManager) GetHistory(deviceID string, limit int) ([]map[string]any, error) {
	bucket := fmt.Sprintf("device_history_%s", deviceID)
	var records []map[string]any

	if m.storage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	err := m.storage.LoadLatest(bucket, limit, func(k, v []byte) error {
		var rec map[string]any
		if err := json.Unmarshal(v, &rec); err != nil {
			return nil
		}
		records = append(records, rec)
		return nil
	})

	return records, err
}

func (m *DeviceStorageManager) GetHistoryByTimeRange(deviceID string, start, end time.Time, limit int) ([]map[string]any, error) {
	bucket := fmt.Sprintf("device_history_%s", deviceID)
	var records []map[string]any

	minKey := start.Format(time.RFC3339Nano)
	maxKey := end.Format(time.RFC3339Nano)

	startTime := time.Now()

	err := m.storage.LoadRange(bucket, minKey, maxKey, func(k, v []byte) error {
		var rec map[string]any
		if err := json.Unmarshal(v, &rec); err != nil {
			return nil
		}
		records = append(records, rec)
		return nil
	})

	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	duration := time.Since(startTime)
	if duration > 1*time.Second {
		log.Printf("[DeviceStorage] Slow query for %s: %v, records: %d", bucket, duration, len(records))
	}

	return records, err
}
