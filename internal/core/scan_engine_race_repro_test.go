package core

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

// raceFailingDriver 模拟持续失败的下位机：触发 FeedbackAggregator 异步回写路径
// （updateTaskStateAggregated 在聚合器 goroutine 中改 task 字段），
// 与 dispatchLoop（enforceAntiStarvation / popReadyTaskEDF，se.mu 下读同字段）并发。
type raceFailingDriver struct{}

func (m *raceFailingDriver) Init(cfg model.DriverConfig) error { return nil }
func (m *raceFailingDriver) Connect(ctx context.Context) error { return nil }
func (m *raceFailingDriver) Disconnect() error                 { return nil }
func (m *raceFailingDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	time.Sleep(2 * time.Millisecond)
	return nil, context.DeadlineExceeded
}
func (m *raceFailingDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	return nil
}
func (m *raceFailingDriver) Health() driver.HealthStatus                 { return driver.HealthStatusBad }
func (m *raceFailingDriver) SetSlaveID(slaveID uint8) error              { return nil }
func (m *raceFailingDriver) SetDeviceConfig(config map[string]any) error { return nil }
func (m *raceFailingDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	return 0, 0, "", "", time.Time{}
}

// TestRaceScanEngineTaskFieldsHotPath 并发运行调度循环 + worker 执行 + 反馈聚合，
// 由 -race 检测 ScanTask 字段（NextRun/Priority/Interval/DeadlineAt）的锁混用竞态。
// 运行方式: go test -race -run TestRaceScanEngineTaskFieldsHotPath ./internal/core/
func TestRaceScanEngineTaskFieldsHotPath(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{
		TickInterval:      5 * time.Millisecond,
		WorkerCount:       4,
		MaxQueueSize:      10000,
		AntiStarvationSec: 1, // 快速触发 enforceAntiStarvation 扫描 se.tasks
		JitterBound:       20 * time.Millisecond,
	})

	se.RegisterProtocol("modbus-fail", ProtocolTypeParallel)
	for i := 0; i < 12; i++ {
		key := "dev-race-" + string(rune('a'+i))
		se.RegisterDriver(key, &raceFailingDriver{})
		se.AddTask(key, "modbus-fail", 10*time.Millisecond, 5,
			[]string{"p1", "p2"}, map[string]any{"channelID": "race-ch"})
	}

	se.Run()
	// 3s 覆盖多个 feedback 窗口（默认 2s）+ 多次反饿死扫描。
	time.Sleep(3 * time.Second)
	se.Stop()
}
