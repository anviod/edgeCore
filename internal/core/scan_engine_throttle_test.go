package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type variableLatencyDriver struct {
	latency time.Duration
}

func (d variableLatencyDriver) Init(_ model.DriverConfig) error        { return nil }
func (d variableLatencyDriver) Connect(_ context.Context) error        { return nil }
func (d variableLatencyDriver) Disconnect() error                      { return nil }
func (d variableLatencyDriver) Health() driver.HealthStatus            { return driver.HealthStatusGood }
func (d variableLatencyDriver) SetDeviceConfig(_ map[string]any) error { return nil }
func (d variableLatencyDriver) SetSlaveID(_ uint8) error               { return nil }
func (d variableLatencyDriver) WritePoint(_ context.Context, _ model.Point, _ any) error {
	return nil
}
func (d variableLatencyDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (d variableLatencyDriver) ReadPoints(_ context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.latency > 0 {
		time.Sleep(d.latency)
	}
	now := time.Now()
	out := make(map[string]model.Value, len(points))
	for _, p := range points {
		out[p.ID] = model.Value{PointID: p.ID, Value: 1.0, Quality: "Good", TS: now}
	}
	return out, nil
}

func runThrottleClusterBenchmark(t *testing.T, slowCount int, slowLatency time.Duration) float64 {
	t.Helper()

	const totalDevices = 100
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval:      5 * time.Millisecond,
		WorkerCount:       32,
		MaxQueueSize:      10000,
		AntiStarvationSec: 300,
		GoroutineLimit:    512,
		ConnectionLimit:   200,
		JitterBound:       0,
	})
	se.SetShadowCore(sc)
	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)

	for i := 0; i < totalDevices; i++ {
		devID := fmt.Sprintf("throttle-dev-%03d", i)
		latency := time.Duration(0)
		if i < slowCount {
			latency = slowLatency
		}
		se.RegisterDriver(devID, variableLatencyDriver{latency: latency})
		se.AddTask(devID, "modbus-tcp", 200*time.Millisecond, 5, []string{"p1"}, map[string]any{
			"degradeOnFailure": false,
		})
	}

	se.Run()
	time.Sleep(3 * time.Second)
	se.Stop()

	return se.GetMetrics().Snapshot()["scan_lag_p95_ms"].(float64)
}

func TestScanEngine_ThrottlePressure_ClusterLag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throttle pressure test in short mode")
	}

	baselineP95 := runThrottleClusterBenchmark(t, 0, 0)
	slowP95 := runThrottleClusterBenchmark(t, 5, 120*time.Millisecond)

	if baselineP95 <= 0 {
		t.Fatalf("baseline P95 lag invalid: %.2f", baselineP95)
	}
	increase := (slowP95 - baselineP95) / baselineP95
	const slowClusterP95MaxMs = 5.0
	if slowP95 > slowClusterP95MaxMs {
		t.Fatalf("slow-cluster P95 %.2fms exceeds %.0fms absolute cap (baseline=%.2fms)",
			slowP95, slowClusterP95MaxMs, baselineP95)
	}
	if baselineP95 > 1.0 && increase > 0.30 {
		t.Fatalf("cluster lag P95 increase %.1f%% exceeds 30%% (baseline=%.2fms slow=%.2fms)",
			increase*100, baselineP95, slowP95)
	}
	t.Logf("throttle pressure: baseline P95=%.2fms slow-cluster P95=%.2fms increase=%.1f%%",
		baselineP95, slowP95, increase*100)
}
