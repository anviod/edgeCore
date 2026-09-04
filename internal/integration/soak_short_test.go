package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/testutil/fault"
)

/*
Soak gate (CI-friendly, no //go:build soak required):

	SOAK_DURATION=30s go test ./internal/integration/ -run TestSoak -count=1 -timeout=5m
	make test-soak-short

Long soak (manual / nightly, requires //go:build soak):

	SOAK_DURATION=72h go test -tags=soak ./internal/integration/ -run TestSoak_ScanEngineStability -count=1 -timeout=80h
	make test-soak
*/

func shortSoakDuration() time.Duration {
	if d, ok := parseDurationEnv("SOAK_DURATION", "SOAK_SHORT_DURATION"); ok {
		return d
	}
	return 30 * time.Second
}

func TestSoak_ScanEngineShortGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak gate in short mode")
	}

	duration := shortSoakDuration()
	const (
		devices      = 40
		faultedCount = 5
		warmup       = 45 * time.Second
	)

	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       16,
		MaxQueueSize:      10000,
		AntiStarvationSec: 300,
		GoroutineLimit:    256,
		ConnectionLimit:   100,
	})
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	base := healthyMockDriver{}
	for i := 1; i <= devices; i++ {
		devID := fmt.Sprintf("short-soak-%03d", i)
		d := fault.Wrap(base)
		if i <= faultedCount {
			switch i % 3 {
			case 0:
				d.Latency = 100 * time.Millisecond
			case 1:
				d.DropNextN = 1
			default:
				d.CorruptNextResponse = true
			}
		} else if i%4 == 0 {
			d.Latency = 40 * time.Millisecond
		}
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", 500*time.Millisecond, 5, []string{"p1"}, nil)
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
