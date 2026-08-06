package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type instantTimeoutDriver struct{}

func (instantTimeoutDriver) Init(_ model.DriverConfig) error        { return nil }
func (instantTimeoutDriver) Connect(_ context.Context) error        { return nil }
func (instantTimeoutDriver) Disconnect() error                      { return nil }
func (instantTimeoutDriver) Health() driver.HealthStatus            { return driver.HealthStatusGood }
func (instantTimeoutDriver) SetSlaveID(_ uint8) error               { return nil }
func (instantTimeoutDriver) SetDeviceConfig(_ map[string]any) error { return nil }
func (instantTimeoutDriver) WritePoint(_ context.Context, _ model.Point, _ any) error {
	return nil
}
func (instantTimeoutDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (instantTimeoutDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, ErrTimeout
}

func newSevenSlaveScanEngine(t *testing.T, offlineDriver driver.Driver, healthy *blockingSlaveMock) (*ScanEngine, *sync.Mutex, string) {
	t.Helper()

	channelMu := &sync.Mutex{}
	channelID := "modbus-tcp-1"

	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
		JitterBound:  0,
	})
	se.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)

	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("modbus-slave-%d", i)
		d := driver.Driver(healthy)
		if i == 6 {
			d = offlineDriver
		}
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", 100*time.Millisecond, 5, []string{"p1"}, map[string]any{
			"channelID":        channelID,
			"channelMu":        channelMu,
			"slave_id":         i,
			"degradeOnFailure": false,
		})
	}
	return se, channelMu, channelID
}

func waitForCircuitState(t *testing.T, cb *DriverCircuitBreaker, key string, want CircuitState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cb.State(key) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("circuit %s state = %v, want %v within %s", key, cb.State(key), want, timeout)
}

func TestScanEngine_CircuitBreakerE2E(t *testing.T) {
	healthy := &blockingSlaveMock{blockSlave: 0}
	se, _, _ := newSevenSlaveScanEngine(t, instantTimeoutDriver{}, healthy)
	cb := se.GetCircuitBreaker()

	se.Run()
	defer se.Stop()

	waitForCircuitState(t, cb, "modbus-slave-6", CircuitOpen, 10*time.Second)
	if cb.OpenTotal() == 0 {
		t.Fatal("expected open_total > 0")
	}

	healthyBefore := se.GetMetrics().TasksSucceeded.Load()
	time.Sleep(500 * time.Millisecond)
	if se.GetMetrics().TasksSucceeded.Load() <= healthyBefore {
		t.Fatalf("healthy slaves should keep scanning while offline slave is open")
	}

	rejectsBefore := cb.RejectTotal()
	time.Sleep(400 * time.Millisecond)
	if cb.RejectTotal() <= rejectsBefore {
		t.Fatalf("expected circuit rejects while open, before=%d after=%d", rejectsBefore, cb.RejectTotal())
	}

	cb.SetOpenedAtForTest("modbus-slave-6", time.Now().Add(-circuitBreakerOpenDuration))
	se.RegisterDriver("modbus-slave-6", healthy)

	waitForCircuitState(t, cb, "modbus-slave-6", CircuitClosed, 10*time.Second)
}

func TestScanEngine_CircuitBreakerFastFailWhenOpen(t *testing.T) {
	healthy := &blockingSlaveMock{blockSlave: 0}
	se, channelMu, channelID := newSevenSlaveScanEngine(t, instantTimeoutDriver{}, healthy)
	cb := se.GetCircuitBreaker()

	offlineTask := &ScanTask{
		DeviceKey: "modbus-slave-6",
		Protocol:  "modbus-tcp",
		Interval:  100 * time.Millisecond,
		PointIDs:  []string{"p1"},
		Params: map[string]any{
			"channelID": channelID,
			"channelMu": channelMu,
			"slave_id":  6,
		},
	}
	for i := 0; i < circuitBreakerConsecutiveTimeoutThreshold; i++ {
		se.executionLayer.Execute(offlineTask)
	}
	if cb.State("modbus-slave-6") != CircuitOpen {
		t.Fatal("precondition: offline slave circuit should be open")
	}

	start := time.Now()
	result := se.executionLayer.Execute(offlineTask)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("open circuit should fast-fail, took %s", time.Since(start))
	}
	if result.Error != ErrCircuitOpen {
		t.Fatalf("error = %v, want ErrCircuitOpen", result.Error)
	}
}

func TestScanEngine_SerialModbusFaultPropagation(t *testing.T) {
	healthy := &blockingSlaveMock{blockSlave: 0}
	se, _, _ := newSevenSlaveScanEngine(t, instantTimeoutDriver{}, healthy)
	cb := se.GetCircuitBreaker()

	se.Run()
	defer se.Stop()

	waitForCircuitState(t, cb, "modbus-slave-6", CircuitOpen, 10*time.Second)

	snap := se.GetMetrics().Snapshot()
	p95, _ := snap["scan_lag_p95_ms"].(float64)
	if p95 > 2000 {
		t.Fatalf("healthy devices lag P95 should stay bounded during fault, got %.2fms", p95)
	}

	healthyTask := se.GetTaskByDeviceKey("modbus-slave-1")
	if healthyTask == nil {
		t.Fatal("missing healthy slave task")
	}
	result := se.executionLayer.Execute(healthyTask)
	if !result.Success {
		t.Fatalf("healthy slave should succeed under shared channel fault, err=%v", result.Error)
	}
	if cb.State("modbus-slave-1") != CircuitClosed {
		t.Fatalf("healthy slave circuit should remain closed, state=%v", cb.State("modbus-slave-1"))
	}
}

func TestDriverCircuitBreaker_Metrics(t *testing.T) {
	cb := NewDriverCircuitBreaker()
	key := "dev-metrics"

	for i := 0; i < circuitBreakerConsecutiveTimeoutThreshold; i++ {
		cb.Record(key, false, true)
	}
	if cb.OpenTotal() != 1 {
		t.Fatalf("open_total = %d, want 1", cb.OpenTotal())
	}
	if cb.Allow(key) {
		t.Fatal("open circuit should reject")
	}
	if cb.RejectTotal() == 0 {
		t.Fatal("expected reject_total > 0")
	}

	snap := cb.Snapshot()
	if snap["reject_total"].(uint64) == 0 {
		t.Fatalf("snapshot reject_total = 0")
	}
	devices, ok := snap["devices"].(map[string]any)
	if !ok || devices[key] == nil {
		t.Fatalf("expected device entry in snapshot: %+v", snap)
	}
}

func TestScanEngineMetrics_SLAWarnings(t *testing.T) {
	m := &ScanEngineMetrics{}
	for i := 0; i < 20; i++ {
		m.RecordExecute(true, 150_000)
	}
	cb := NewDriverCircuitBreaker()
	cb.rejectTotal.Add(1)

	warnings := m.SLAWarnings(cb)
	if len(warnings) == 0 {
		t.Fatal("expected SLA warnings for high lag and circuit rejects")
	}
	foundLag := false
	foundReject := false
	for _, w := range warnings {
		switch w["code"] {
		case "scan_lag_p95_exceeded":
			foundLag = true
		case "circuit_breaker_rejects":
			foundReject = true
		}
	}
	if !foundLag || !foundReject {
		t.Fatalf("missing expected warning codes: %+v", warnings)
	}
}
