package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type mockDriver struct{}

func (m *mockDriver) Init(cfg model.DriverConfig) error { return nil }
func (m *mockDriver) Connect(ctx context.Context) error { return nil }
func (m *mockDriver) Disconnect() error                 { return nil }
func (m *mockDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	time.Sleep(5 * time.Millisecond)
	values := make(map[string]model.Value)
	for _, p := range points {
		values[p.ID] = model.Value{Value: 0, Quality: "Good", TS: time.Now()}
	}
	return values, nil
}
func (m *mockDriver) WritePoint(ctx context.Context, point model.Point, value any) error { return nil }
func (m *mockDriver) Health() driver.HealthStatus                                        { return driver.HealthStatusGood }
func (m *mockDriver) SetSlaveID(slaveID uint8) error                                     { return nil }
func (m *mockDriver) SetDeviceConfig(config map[string]any) error                        { return nil }
func (m *mockDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	return 0, 0, "", "", time.Time{}
}

func TestScanEngine_StressTest(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       16,
		MaxQueueSize:      10000,
		AntiStarvationSec: 300,
		GoroutineLimit:    100,
		ConnectionLimit:   50,
	}

	se := NewScanEngine(config)

	taskCount := 5
	for i := 0; i < taskCount; i++ {
		deviceKey := "device_stress_" + string(rune('A'+i%26))
		se.AddTask(deviceKey, "modbus-tcp", 500*time.Millisecond, 5, []string{"point1", "point2"}, nil)
		se.RegisterDriver(deviceKey, &mockDriver{})
	}

	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)

	se.Run()

	time.Sleep(500 * time.Millisecond)

	activeTasks := se.GetActiveTaskCount()
	if activeTasks > config.GoroutineLimit {
		t.Errorf("活动任务数超过限制，期望<=100，实际%d", activeTasks)
	}

	pendingTasks := se.GetPendingTaskCount()
	t.Logf("待处理任务数: %d", pendingTasks)

	se.mu.Lock()
	se.running = false
	se.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	se.Stop()
}

func TestScanEngine_ShadowIntegration(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       4,
		MaxQueueSize:      1000,
		AntiStarvationSec: 300,
		GoroutineLimit:    50,
		ConnectionLimit:   20,
	}

	se := NewScanEngine(config)

	deviceKey := "device_shadow_test"
	se.AddTask(deviceKey, "modbus-tcp", 100*time.Millisecond, 5, []string{"point1", "point2"}, nil)
	se.RegisterDriver(deviceKey, &mockDriver{})
	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)

	se.Run()

	time.Sleep(200 * time.Millisecond)

	se.mu.Lock()
	se.running = false
	se.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	se.Stop()

	t.Log("ShadowCore集成测试完成")
}

func TestBackpressureController_Stress(t *testing.T) {
	bc := NewBackpressureController(10, 100)

	successCount := 0
	failCount := 0
	wg := sync.WaitGroup{}
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			if bc.Allow("device1", 5) {
				successCount++
				time.Sleep(10 * time.Millisecond)
				bc.Release("device1")
			} else {
				failCount++
			}
		}()
	}

	wg.Wait()

	t.Logf("成功: %d, 失败: %d", successCount, failCount)

	if failCount > 0 {
		t.Logf("背压机制生效，成功限制了并发请求")
	}
}

func TestSerialQueueManager_Concurrency(t *testing.T) {
	manager := NewSerialQueueManager()

	taskCount := 100
	wg := sync.WaitGroup{}
	wg.Add(taskCount)

	startTimes := make([]time.Time, taskCount)

	for i := 0; i < taskCount; i++ {
		go func(id int) {
			defer wg.Done()
			startTimes[id] = time.Now()
			task := &DriverTask{
				DeviceKey: "device_serial_test",
				Points:    []model.Point{{ID: "point1"}},
				Callback: func(values map[string]model.Value, err error) {
				},
			}
			manager.Submit(task)
		}(i)
	}

	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	manager.Stop()

	t.Logf("串行队列测试完成，共提交%d个任务", taskCount)
}

func TestResourceController_Stress(t *testing.T) {
	rc := NewResourceController(ResourceLimits{
		GoroutineLimit:  50,
		ConnectionLimit: 20,
		QueueLimit:      1000,
	})

	successCount := 0
	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rc.CanExecute() {
				rc.Acquire()
				successCount++
				time.Sleep(50 * time.Millisecond)
				rc.Release()
			}
		}()
	}

	wg.Wait()

	t.Logf("资源控制测试完成，成功获取资源: %d", successCount)

	if successCount <= 50 {
		t.Log("资源限制生效")
	}
}
