package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/testutil/fault"
)

type healthyMockDriver struct{}

func (healthyMockDriver) Init(_ model.DriverConfig) error        { return nil }
func (healthyMockDriver) Connect(_ context.Context) error        { return nil }
func (healthyMockDriver) Disconnect() error                      { return nil }
func (healthyMockDriver) Health() driver.HealthStatus            { return driver.HealthStatusGood }
func (healthyMockDriver) SetDeviceConfig(_ map[string]any) error { return nil }
func (healthyMockDriver) SetSlaveID(_ uint8) error               { return nil }
func (healthyMockDriver) WritePoint(_ context.Context, _ model.Point, _ any) error {
	return nil
}
func (healthyMockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (healthyMockDriver) ReadPoints(_ context.Context, points []model.Point) (map[string]model.Value, error) {
	now := time.Now()
	if len(points) == 1 {
		return map[string]model.Value{
			points[0].ID: {PointID: points[0].ID, Value: 1.0, Quality: "Good", TS: now},
		}, nil
	}
	out := make(map[string]model.Value, len(points))
	for _, p := range points {
		out[p.ID] = model.Value{PointID: p.ID, Value: 1.0, Quality: "Good", TS: now}
	}
	return out, nil
}

type instantTimeoutDriver struct{ healthyMockDriver }

func (instantTimeoutDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, core.ErrTimeout
}

func TestFaultPropagation_HealthyDevicesUnaffected(t *testing.T) {
	const (
		totalDevices = 100
		faultedCount = 5
	)

	el := core.NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	base := healthyMockDriver{}

	for i := 1; i <= totalDevices; i++ {
		devID := fmt.Sprintf("dev-%03d", i)
		d := driver.Driver(base)
		if i <= faultedCount {
			inj := fault.Wrap(base)
			inj.DropNextN = 3
			inj.CorruptNextResponse = true
			d = inj
		}
		el.RegisterDriver(devID, d)
	}

	var wg sync.WaitGroup
	results := make(map[string]*core.ExecuteResult, totalDevices)
	var resultsMu sync.Mutex

	for i := 1; i <= totalDevices; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			devID := fmt.Sprintf("dev-%03d", idx)
			res := el.Execute(&core.ScanTask{
				DeviceKey: devID,
				Protocol:  "modbus-tcp",
				Interval:  time.Second,
				PointIDs:  []string{"p1"},
			})
			resultsMu.Lock()
			results[devID] = res
			resultsMu.Unlock()
		}(i)
	}
	wg.Wait()

	for i := faultedCount + 1; i <= totalDevices; i++ {
		devID := fmt.Sprintf("dev-%03d", i)
		res := results[devID]
		if res == nil || !res.Success {
			t.Fatalf("%s should remain healthy, result=%+v", devID, res)
		}
	}

	failedFaulted := 0
	for i := 1; i <= faultedCount; i++ {
		devID := fmt.Sprintf("dev-%03d", i)
		res := results[devID]
		if res == nil || res.Success {
			continue
		}
		failedFaulted++
	}
	if failedFaulted == 0 {
		t.Fatal("expected injected faults to fail at least one faulted device")
	}
}

func TestFaultPropagation_LatencyOnFaultedDevices(t *testing.T) {
	const (
		totalDevices = 50
		slowCount    = 3
	)

	el := core.NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	base := healthyMockDriver{}

	for i := 1; i <= totalDevices; i++ {
		devID := fmt.Sprintf("dev-%03d", i)
		d := driver.Driver(base)
		if i <= slowCount {
			inj := fault.Wrap(base)
			inj.Latency = 200 * time.Millisecond
			d = inj
		}
		el.RegisterDriver(devID, d)
	}

	var wg sync.WaitGroup
	results := make(map[string]*core.ExecuteResult, totalDevices)
	var resultsMu sync.Mutex

	start := time.Now()
	for i := 1; i <= totalDevices; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			devID := fmt.Sprintf("dev-%03d", idx)
			res := el.Execute(&core.ScanTask{
				DeviceKey: devID,
				Protocol:  "modbus-tcp",
				Interval:  time.Second,
				PointIDs:  []string{"p1"},
			})
			resultsMu.Lock()
			results[devID] = res
			resultsMu.Unlock()
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	for i := slowCount + 1; i <= totalDevices; i++ {
		devID := fmt.Sprintf("dev-%03d", i)
		res := results[devID]
		if res == nil || !res.Success {
			t.Fatalf("%s should remain healthy, result=%+v", devID, res)
		}
	}

	if elapsed > 2*time.Second {
		t.Fatalf("healthy devices blocked too long: elapsed=%v", elapsed)
	}
}

func TestFaultPropagation_CircuitBreakerHalfOpenRecovery(t *testing.T) {
	el := core.NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	base := healthyMockDriver{}
	timeoutDrv := instantTimeoutDriver{healthyMockDriver: base}
	el.RegisterDriver("dev-fault", timeoutDrv)

	task := &core.ScanTask{
		DeviceKey: "dev-fault",
		Protocol:  "modbus-tcp",
		Interval:  time.Second,
		PointIDs:  []string{"p1"},
	}

	cb := el.GetCircuitBreaker()
	const timeoutThreshold = 5
	for i := 0; i < timeoutThreshold; i++ {
		res := el.Execute(task)
		if res.Success {
			t.Fatalf("iteration %d: expected timeout failure before circuit open", i)
		}
	}

	if cb.State("dev-fault") != core.CircuitOpen {
		t.Fatalf("state = %v, want Open", cb.State("dev-fault"))
	}

	cb.SetOpenedAtForTest("dev-fault", time.Now().Add(-31*time.Second))

	inj := fault.Wrap(base)
	inj.HalfOpenDuration = 500 * time.Millisecond
	el.RegisterDriver("dev-fault", inj)

	if !cb.Allow("dev-fault") {
		t.Fatal("expected half-open probe to be allowed")
	}

	res := el.Execute(task)
	if !res.Success {
		t.Fatalf("expected half-open recovery success during HalfOpenDuration window, result=%+v", res)
	}
	if cb.State("dev-fault") != core.CircuitClosed {
		t.Fatalf("state = %v, want Closed after successful probe", cb.State("dev-fault"))
	}
}
