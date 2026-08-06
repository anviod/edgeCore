package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

// blockingSlaveMock simulates one offline Modbus slave on a shared TCP link.
type blockingSlaveMock struct {
	mu            sync.Mutex
	slaveID       uint8
	blockSlave    uint8
	blockDuration time.Duration
}

func (m *blockingSlaveMock) Init(_ model.DriverConfig) error        { return nil }
func (m *blockingSlaveMock) Connect(_ context.Context) error        { return nil }
func (m *blockingSlaveMock) Disconnect() error                      { return nil }
func (m *blockingSlaveMock) Health() drv.HealthStatus               { return drv.HealthStatusGood }
func (m *blockingSlaveMock) SetDeviceConfig(_ map[string]any) error { return nil }
func (m *blockingSlaveMock) WritePoint(_ context.Context, _ model.Point, _ any) error {
	return nil
}
func (m *blockingSlaveMock) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func (m *blockingSlaveMock) SetSlaveID(slaveID uint8) error {
	m.mu.Lock()
	m.slaveID = slaveID
	m.mu.Unlock()
	return nil
}

func (m *blockingSlaveMock) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	m.mu.Lock()
	slave := m.slaveID
	m.mu.Unlock()

	if slave == m.blockSlave {
		select {
		case <-ctx.Done():
			return nil, context.DeadlineExceeded
		case <-time.After(m.blockDuration):
			return nil, fmt.Errorf("i/o timeout")
		}
	}

	results := make(map[string]model.Value, len(points))
	now := time.Now()
	for _, p := range points {
		results[p.ID] = model.Value{PointID: p.ID, Value: float64(slave), Quality: "Good", TS: now}
	}
	return results, nil
}

func newSevenSlaveChannelManager() (*ChannelManager, *blockingSlaveMock) {
	cm := NewChannelManager(NewDataPipeline(64), nil)
	mock := &blockingSlaveMock{blockSlave: 6, blockDuration: 6 * time.Second}

	devices := make([]model.Device, 0, 7)
	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("modbus-slave-%d", i)
		devices = append(devices, model.Device{
			ID:     devID,
			Name:   devID,
			Enable: true,
			Config: map[string]any{"slave_id": i},
			Points: []model.Point{{ID: "p1", Address: "0", DataType: "int16"}},
		})
		cm.stateManager.RegisterNode(devID, devID)
	}

	cm.channels["modbus-tcp-1"] = &model.Channel{
		ID:       "modbus-tcp-1",
		Name:     "modbus",
		Protocol: "modbus-tcp",
		Enable:   true,
		Devices:  devices,
	}
	cm.drivers["modbus-tcp-1"] = mock
	cm.driverMus["modbus-tcp-1"] = &sync.Mutex{}
	return cm, mock
}

func TestScenario_SevenSlavesOneOfflineIsolation(t *testing.T) {
	cm, mock := newSevenSlaveChannelManager()
	channelMu := cm.driverMus["modbus-tcp-1"]

	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("modbus-slave-%d", i)
		el.RegisterDriver(devID, mock)
	}

	var wg sync.WaitGroup
	results := make(map[string]*ExecuteResult, 7)
	var resultsMu sync.Mutex

	for i := 1; i <= 7; i++ {
		wg.Add(1)
		go func(slave int) {
			defer wg.Done()
			devID := fmt.Sprintf("modbus-slave-%d", slave)
			task := &ScanTask{
				DeviceKey: devID,
				Protocol:  "modbus-tcp",
				Interval:  time.Second,
				PointIDs:  []string{"p1"},
				Params: map[string]any{
					"channelID": "modbus-tcp-1",
					"channelMu": channelMu,
					"slave_id":  slave,
				},
			}
			res := el.Execute(task)
			resultsMu.Lock()
			results[devID] = res
			resultsMu.Unlock()
		}(i)
	}
	wg.Wait()

	for devID, res := range results {
		cm.finalizeScanCollect(devID, res)
	}

	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("modbus-slave-%d", i)
		state := cm.stateManager.GetNode(devID).Runtime.State
		if i == 6 {
			if state == NodeStateOnline {
				t.Fatalf("%s should not remain online after timeout", devID)
			}
			continue
		}
		if state != NodeStateOnline {
			t.Fatalf("%s should stay online when only slave-6 fails, got state=%d fail=%d",
				devID, state, cm.stateManager.GetNode(devID).Runtime.FailCount)
		}
	}

	stats := cm.GetChannelStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 channel stat, got %d", len(stats))
	}
	if stats[0].Status == "Offline" {
		t.Fatalf("channel must not be offline when link is up, got %s", stats[0].Status)
	}
	if stats[0].OnlineCount < 6 {
		t.Fatalf("expected at least 6 online devices, got online=%d offline=%d",
			stats[0].OnlineCount, stats[0].OfflineCount)
	}
}

func TestScenario_CircuitBreakerIsolatesOfflineSlave(t *testing.T) {
	cm, mock := newSevenSlaveChannelManager()
	channelMu := cm.driverMus["modbus-tcp-1"]

	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("modbus-slave-%d", i)
		el.RegisterDriver(devID, mock)
	}

	openOfflineDevice := func() {
		task := &ScanTask{
			DeviceKey: "modbus-slave-6",
			Protocol:  "modbus-tcp",
			Interval:  time.Second,
			PointIDs:  []string{"p1"},
			Params: map[string]any{
				"channelID": "modbus-tcp-1",
				"channelMu": channelMu,
				"slave_id":  6,
			},
		}
		for i := 0; i < circuitBreakerConsecutiveTimeoutThreshold; i++ {
			el.Execute(task)
		}
	}
	openOfflineDevice()

	if el.GetCircuitBreaker().State("modbus-slave-6") != CircuitOpen {
		t.Fatalf("offline slave circuit should be open")
	}

	healthyTask := &ScanTask{
		DeviceKey: "modbus-slave-1",
		Protocol:  "modbus-tcp",
		Interval:  time.Second,
		PointIDs:  []string{"p1"},
		Params: map[string]any{
			"channelID": "modbus-tcp-1",
			"channelMu": channelMu,
			"slave_id":  1,
		},
	}
	res := el.Execute(healthyTask)
	if !res.Success {
		t.Fatalf("healthy slave on shared channel should succeed, err=%v", res.Error)
	}
	if el.GetCircuitBreaker().State("modbus-slave-1") != CircuitClosed {
		t.Fatalf("healthy slave circuit should remain closed")
	}
}

func TestSerialQueueKey_UsesChannelForSharedLink(t *testing.T) {
	el := NewExecutionLayer()
	task := &ScanTask{
		DeviceKey: "modbus-slave-1",
		Protocol:  "modbus-tcp",
		Params: map[string]any{
			"channelID": "modbus-tcp-1",
		},
	}
	if got := el.serialQueueKey(task); got != "shared:modbus-tcp-1" {
		t.Fatalf("serialQueueKey() = %q, want shared:modbus-tcp-1", got)
	}
}

func TestSerialQueueKey_FallsBackToDeviceForParallelProtocol(t *testing.T) {
	el := NewExecutionLayer()
	task := &ScanTask{
		DeviceKey: "opc-device-1",
		Protocol:  "opc-ua",
		Params: map[string]any{
			"channelID": "opc-1",
		},
	}
	if got := el.serialQueueKey(task); got != "opc-device-1" {
		t.Fatalf("serialQueueKey() = %q, want opc-device-1", got)
	}
}
