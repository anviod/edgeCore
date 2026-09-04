package shadow

import (
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

type ShadowCore struct {
	deviceSnapshots map[string]*DeviceSnapshot
	mu              sync.RWMutex
	versionCounter  int64
}

type DeviceSnapshot struct {
	DeviceKey    string
	Values       map[string]model.Value
	Version      int64
	LastUpdate   time.Time
	HealthStatus string
	mu           sync.RWMutex
}

func NewShadowCore() *ShadowCore {
	return &ShadowCore{
		deviceSnapshots: make(map[string]*DeviceSnapshot),
	}
}

func (sc *ShadowCore) Update(deviceKey string, values map[string]model.Value) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	snapshot, ok := sc.deviceSnapshots[deviceKey]
	if !ok {
		snapshot = &DeviceSnapshot{
			DeviceKey:  deviceKey,
			Values:     make(map[string]model.Value),
			Version:    0,
			LastUpdate: time.Now(),
		}
		sc.deviceSnapshots[deviceKey] = snapshot
	}

	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()

	for pointID, value := range values {
		snapshot.Values[pointID] = value
	}

	sc.versionCounter++
	snapshot.Version = sc.versionCounter
	snapshot.LastUpdate = time.Now()

	snapshot.HealthStatus = "Good"
	for _, v := range snapshot.Values {
		if v.Quality != "Good" {
			snapshot.HealthStatus = "Degraded"
			break
		}
	}

	zap.L().Debug("[ShadowCore] 更新设备快照",
		zap.String("deviceKey", deviceKey),
		zap.Int("pointCount", len(values)),
		zap.Int64("version", snapshot.Version),
		zap.String("health", snapshot.HealthStatus),
	)
}

func (sc *ShadowCore) Get(deviceKey string) *DeviceSnapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.deviceSnapshots[deviceKey]
}

func (sc *ShadowCore) GetValue(deviceKey, pointID string) (model.Value, bool) {
	sc.mu.RLock()
	snapshot, ok := sc.deviceSnapshots[deviceKey]
	sc.mu.RUnlock()

	if !ok {
		return model.Value{}, false
	}

	snapshot.mu.RLock()
	defer snapshot.mu.RUnlock()

	value, ok := snapshot.Values[pointID]
	return value, ok
}

func (sc *ShadowCore) GetAllDevices() []string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	devices := make([]string, 0, len(sc.deviceSnapshots))
	for key := range sc.deviceSnapshots {
		devices = append(devices, key)
	}
	return devices
}

func (sc *ShadowCore) Remove(deviceKey string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	delete(sc.deviceSnapshots, deviceKey)

	zap.L().Info("[ShadowCore] 移除设备快照",
		zap.String("deviceKey", deviceKey),
	)
}

func (sc *ShadowCore) GetMetrics() map[string]interface{} {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	metrics := make(map[string]interface{})
	metrics["deviceCount"] = len(sc.deviceSnapshots)
	metrics["totalVersion"] = sc.versionCounter

	return metrics
}
