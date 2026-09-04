package core

import (
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

type ScanEngineAdapter struct {
	scanEngine    *ScanEngine
	driverManager map[string]driver.Driver
	mu            sync.RWMutex
	started       bool
	startOnce     sync.Once
}

func NewScanEngineAdapter(scanEngine *ScanEngine) *ScanEngineAdapter {
	return &ScanEngineAdapter{
		scanEngine:    scanEngine,
		driverManager: make(map[string]driver.Driver),
		started:       false,
	}
}

func (a *ScanEngineAdapter) IsStarted() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.started
}

func (a *ScanEngineAdapter) RegisterDevice(
	deviceID string,
	protocol string,
	channelDriver driver.Driver,
	channelMu *sync.Mutex,
	ch *model.Channel,
	dev *model.Device,
	interval time.Duration,
	priority int,
) error {
	if channelDriver == nil {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.resetDeviceCollection(deviceID)
	a.scanEngine.RemoveTasksByDeviceKey(deviceID)
	a.scanEngine.UnregisterDriver(deviceID)

	points := make([]model.Point, len(dev.Points))
	copy(points, dev.Points)
	for i := range points {
		points[i].DeviceID = deviceID
	}

	driverConfig := buildDriverDeviceConfig(ch, dev.Config, map[string]any{
		"_internal_device_id": deviceID,
	})

	degradeOnFailure := true
	if dev.DegradeOnFailure != nil {
		degradeOnFailure = *dev.DegradeOnFailure
	}

	params := map[string]any{
		"deviceID":         deviceID,
		"protocol":         protocol,
		"channelID":        ch.ID,
		"points":           points,
		"driverConfig":     driverConfig,
		"channelMu":        channelMu,
		"degradeOnFailure": degradeOnFailure,
	}
	if slaveID, ok := dev.Config["slave_id"]; ok {
		params["slave_id"] = slaveID
	}
	if v, ok := dev.Config["degrade_on_failure"]; ok {
		switch val := v.(type) {
		case bool:
			params["degradeOnFailure"] = val
		}
	}

	groups := model.GroupPointsByScanClass(points)
	for scanClass, classPoints := range groups {
		if len(classPoints) == 0 {
			continue
		}
		classInterval := model.ScanClassInterval(scanClass, model.Duration(interval))
		classParams := make(map[string]any)
		for k, v := range params {
			classParams[k] = v
		}
		classParams["points"] = classPoints
		classParams["scanClass"] = scanClass

		pointIDs := make([]string, 0, len(classPoints))
		for _, p := range classPoints {
			pointIDs = append(pointIDs, p.ID)
		}

		a.scanEngine.AddTaskWithScanClass(deviceID, protocol, scanClass, classInterval, priority, pointIDs, classParams)
	}

	a.driverManager[deviceID] = channelDriver
	a.scanEngine.RegisterDriver(deviceID, channelDriver)

	zap.L().Info("[ScanEngineAdapter] 设备已注册",
		zap.String("deviceID", deviceID),
		zap.String("protocol", protocol),
		zap.Duration("interval", interval),
		zap.Int("priority", priority),
		zap.Int("pointCount", len(points)),
		zap.Int("scanClassGroups", len(groups)),
	)

	return nil
}

func (a *ScanEngineAdapter) UnregisterDevice(deviceID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.resetDeviceCollection(deviceID)
	delete(a.driverManager, deviceID)
	a.scanEngine.RemoveTasksByDeviceKey(deviceID)
	a.scanEngine.UnregisterDriver(deviceID)

	zap.L().Info("[ScanEngineAdapter] 设备已注销",
		zap.String("deviceID", deviceID),
	)
}

func (a *ScanEngineAdapter) GetDriver(deviceID string) driver.Driver {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.driverManager[deviceID]
}

func (a *ScanEngineAdapter) Start() {
	var started bool
	a.startOnce.Do(func() {
		a.mu.Lock()
		a.started = true
		a.mu.Unlock()
		a.scanEngine.Run()
		started = true
		zap.L().Info("[ScanEngineAdapter] 适配器已启动")
	})

	if !started {
		zap.L().Warn("[ScanEngineAdapter] 适配器已启动，忽略重复启动请求")
	}
}

func (a *ScanEngineAdapter) Stop() {
	a.scanEngine.Stop()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.driverManager = make(map[string]driver.Driver)

	zap.L().Info("[ScanEngineAdapter] 适配器已停止")
}

func (a *ScanEngineAdapter) UpdateDeviceInterval(deviceID string, interval time.Duration) {
	a.scanEngine.UpdateTaskInterval(deviceID, interval)
	zap.L().Info("[ScanEngineAdapter] 设备采集间隔已更新",
		zap.String("deviceID", deviceID),
		zap.Duration("interval", interval),
	)
}

func (a *ScanEngineAdapter) UpdateDevicePriority(deviceID string, priority int) {
	a.scanEngine.UpdateTaskPriority(deviceID, priority)
	zap.L().Info("[ScanEngineAdapter] 设备优先级已更新",
		zap.String("deviceID", deviceID),
		zap.Int("priority", priority),
	)
}

func (a *ScanEngineAdapter) UpdateDeviceDriverConfig(deviceID string, updates map[string]any) {
	a.scanEngine.UpdateTaskDriverConfig(deviceID, updates)
}

func (a *ScanEngineAdapter) GetTaskStatus(deviceID string) ScanTaskStatus {
	task := a.scanEngine.GetTaskByDeviceKey(deviceID)
	if task == nil {
		return ScanTaskStatusStopped
	}
	return task.GetStatus()
}

func (a *ScanEngineAdapter) GetActiveTaskCount() int {
	return a.scanEngine.GetActiveTaskCount()
}

func (a *ScanEngineAdapter) GetPendingTaskCount() int {
	return a.scanEngine.GetPendingTaskCount()
}

func (a *ScanEngineAdapter) GetTasks() []*ScanTask {
	return a.scanEngine.GetTasks()
}

func (a *ScanEngineAdapter) resetDeviceCollection(deviceID string) {
	if d, ok := a.driverManager[deviceID]; ok {
		if r, ok := d.(driver.DeviceCollectionResetter); ok {
			r.ResetDeviceCollection(deviceID)
		}
	}
}
