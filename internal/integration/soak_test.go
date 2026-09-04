//go:build soak

package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/testutil/fault"
)

func soakDuration() time.Duration {
	if d, ok := parseDurationEnv("SOAK_DURATION"); ok {
		return d
	}
	return 72 * time.Hour
}

func TestSoak_ScanEngineStability(t *testing.T) {
	duration := soakDuration()
	const (
		devices      = 100
		pointsPerDev = 10
		faultedCount = 5
		warmup       = 30 * time.Second
	)

	sc := core.NewShadowCore()
	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       32,
		MaxQueueSize:      20000,
		AntiStarvationSec: 300,
		GoroutineLimit:    512,
		ConnectionLimit:   200,
	})
	se.SetShadowCore(sc)
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	base := healthyMockDriver{}
	for i := 1; i <= devices; i++ {
		devID := fmt.Sprintf("soak-dev-%03d", i)
		d := fault.Wrap(base)
		if i <= faultedCount {
			d.RotateFaultModes = true
			switch i % 3 {
			case 0:
				d.Latency = 80 * time.Millisecond
			case 1:
				d.DropNextN = 1
			default:
				d.CorruptNextResponse = true
			}
		}
		se.RegisterDriver(devID, d)
		pointIDs := make([]string, pointsPerDev)
		for j := range pointIDs {
			pointIDs[j] = fmt.Sprintf("p%d", j)
		}
		se.AddTask(devID, "modbus-tcp", time.Second, 5, pointIDs, map[string]any{
			"degradeOnFailure": false,
		})
	}

	se.Run()
	time.Sleep(warmup)
	settleHeapForMeasurement()
	memStart := captureStableMemSnapshot()
	se.GetMetrics().ResetWindow()

	time.Sleep(duration)
	se.Stop()
	settleHeapForMeasurement()
	memEnd := captureFinalMemSnapshot()

	snap := se.GetMetrics().Snapshot()
	executed, _ := snap["tasks_executed"].(uint64)
	failed, _ := snap["tasks_failed"].(uint64)
	p95, _ := snap["scan_lag_p95_ms"].(float64)
	missDeadline, _ := snap["scan_miss_deadline_total"].(uint64)

	failRate := 0.0
	if executed > 0 {
		failRate = float64(failed) / float64(executed)
	}
	memPassed, memDrift := memoryDriftGateLogged(t, memStart, memEnd)

	gates := []productionGate{
		{
			Name:   "panic_free",
			Passed: true,
			Detail: "test completed without panic",
		},
		{
			Name:   "failure_rate_under_0.5pct_with_injected_faults",
			Passed: failRate <= prodGateSoakFailRateMax,
			Value:  failRate,
			Limit:  prodGateSoakFailRateMax,
		},
		{
			Name:   "scan_lag_p95_under_200ms",
			Passed: p95 <= prodGateSoakLagP95Ms,
			Value:  p95,
			Limit:  prodGateSoakLagP95Ms,
		},
		{
			Name:   "memory_drift_under_5pct",
			Passed: memPassed,
			Value:  memDrift,
			Limit:  prodGateMemDriftMaxPct,
		},
		{
			Name:   "scan_miss_deadline_within_threshold",
			Passed: missDeadline <= prodGateScanMissDeadlineMax,
			Value:  missDeadline,
			Limit:  prodGateScanMissDeadlineMax,
		},
	}

	summary := buildProductionReadinessSummary(t.Name(), duration, gates, snap)
	assertProductionGates(t, summary)
}
