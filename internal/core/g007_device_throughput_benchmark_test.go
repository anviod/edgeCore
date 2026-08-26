package core

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// G007 benchmark: verify §2.5.5 scheduling throughput ≥950 devices/sec.
// Test method aligns with plan §7.2: 1000 devices, 1s scan interval, mock driver (zero I/O).
// Target is 950 not 1000: ScanEngine 10ms tick, JitterBound 50ms (§2.2.2 task timing
// deviation <50ms). With ~25ms average scheduling overhead, fleet effective cadence ≈1.025s
// → theoretical reference ceiling ≈976 devices/s (1000÷1.025, or 1000×(1000ms/1025ms));
// per-device jitter is a fixed offset and steady-state period remains 1s. 950/s is the
// validated acceptance bar below that estimate (plan §2.5.6).

const (
	g007BenchmarkDevices        = 1000
	g007BenchmarkPointsPerDev   = 1
	g007BenchmarkScanInterval   = time.Second
	g007BenchmarkDefaultRuntime = 30 * time.Second
	g007BenchmarkWarmup         = 10 * time.Second
	g007ThroughputTarget        = 950.0
)

type g007BenchmarkConfig struct {
	Duration     time.Duration
	ScanInterval time.Duration
	Devices      int
	PointsPerDev int
}

type g007BenchmarkResult struct {
	Config               g007BenchmarkConfig
	Duration             time.Duration
	WarmupDuration       time.Duration
	TasksExecuted        uint64
	TasksSucceeded       uint64
	TasksFailed          uint64
	ScanLagP95Ms         float64
	ScanMissDeadline     uint64
	TaskOverdueTotal     uint64
	StarvationRescues    uint64
	ThroughputDevicesSec float64
	BackpressureRejects  uint64
	GoroutinesStart      int
	GoroutinesEnd        int
}

func g007BenchmarkDuration() time.Duration {
	if raw := os.Getenv("G007_BENCH_DURATION"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return g007BenchmarkDefaultRuntime
}

func runG007DeviceThroughputBenchmark(t *testing.T, cfg g007BenchmarkConfig) g007BenchmarkResult {
	t.Helper()

	if cfg.Duration <= 0 {
		cfg.Duration = g007BenchmarkDuration()
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = g007BenchmarkScanInterval
	}
	if cfg.Devices <= 0 {
		cfg.Devices = g007BenchmarkDevices
	}
	if cfg.PointsPerDev <= 0 {
		cfg.PointsPerDev = g007BenchmarkPointsPerDev
	}

	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       32,
		MaxQueueSize:      50000,
		AntiStarvationSec: 300,
		GoroutineLimit:    512,
		ConnectionLimit:   200,
	})
	se.SetShadowCore(sc)
	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)

	for i := 0; i < cfg.Devices; i++ {
		deviceKey := fmt.Sprintf("g007_dev_%04d", i)
		pointIDs := make([]string, cfg.PointsPerDev)
		for j := 0; j < cfg.PointsPerDev; j++ {
			pointIDs[j] = fmt.Sprintf("p%03d", j)
		}
		params := map[string]any{
			"channelID":        "g007-ch",
			"degradeOnFailure": false,
		}
		se.AddTask(deviceKey, "modbus-tcp", cfg.ScanInterval, 5, pointIDs, params)
		se.RegisterDriver(deviceKey, &mockStressDriver{})
	}

	se.Run()
	time.Sleep(g007BenchmarkWarmup)
	se.GetMetrics().ResetWindow()

	goroutinesStart := runtime.NumGoroutine()
	time.Sleep(cfg.Duration)
	se.Stop()

	goroutinesEnd := runtime.NumGoroutine()
	metrics := se.GetMetrics().Snapshot()

	tasksExecuted, _ := metrics["tasks_executed"].(uint64)
	tasksSucceeded, _ := metrics["tasks_succeeded"].(uint64)
	tasksFailed, _ := metrics["tasks_failed"].(uint64)
	starvation, _ := metrics["starvation_rescue_total"].(uint64)
	overdue, _ := metrics["task_overdue_total"].(uint64)
	p95Lag, _ := metrics["scan_lag_p95_ms"].(float64)
	missDeadline, _ := metrics["scan_miss_deadline_total"].(uint64)

	throughput := float64(tasksSucceeded) / cfg.Duration.Seconds()

	var backpressureRejects uint64
	if snap := se.OperationalSnapshot(); snap != nil {
		if v, ok := snap["backpressure_reject_total"].(uint64); ok {
			backpressureRejects = v
		}
	}

	result := g007BenchmarkResult{
		Config:               cfg,
		Duration:             cfg.Duration,
		WarmupDuration:       g007BenchmarkWarmup,
		TasksExecuted:        tasksExecuted,
		TasksSucceeded:       tasksSucceeded,
		TasksFailed:          tasksFailed,
		ScanLagP95Ms:         p95Lag,
		ScanMissDeadline:     missDeadline,
		TaskOverdueTotal:     overdue,
		StarvationRescues:    starvation,
		ThroughputDevicesSec: throughput,
		BackpressureRejects:  backpressureRejects,
		GoroutinesStart:      goroutinesStart,
		GoroutinesEnd:        goroutinesEnd,
	}

	logG007BenchmarkResult(t, result)
	return result
}

func logG007BenchmarkResult(t *testing.T, r g007BenchmarkResult) {
	t.Helper()
	t.Logf("G007 Device Throughput Benchmark (§2.5.5)")
	t.Logf("  duration=%s warmup=%s interval=%s devices=%d points/device=%d target=%.0f devices/s",
		r.Duration, r.WarmupDuration, r.Config.ScanInterval, r.Config.Devices, r.Config.PointsPerDev, g007ThroughputTarget)
	t.Logf("  scan: executed=%d succeeded=%d failed=%d lag_p95=%.2fms miss_deadline=%d overdue=%d rescue=%d backpressure_rejects=%d",
		r.TasksExecuted, r.TasksSucceeded, r.TasksFailed, r.ScanLagP95Ms, r.ScanMissDeadline, r.TaskOverdueTotal, r.StarvationRescues, r.BackpressureRejects)
	t.Logf("  throughput=%.0f devices/s goroutines=%d->%d pass=%v",
		r.ThroughputDevicesSec, r.GoroutinesStart, r.GoroutinesEnd, r.ThroughputDevicesSec >= g007ThroughputTarget)
}

func TestG007_DeviceThroughputBenchmark(t *testing.T) {
	if testing.Short() || raceDetectorEnabled {
		if raceDetectorEnabled {
			t.Skip("skipping throughput benchmark under race detector")
		}
		t.Skip("skipping G007 device throughput benchmark in short mode")
	}

	result := runG007DeviceThroughputBenchmark(t, g007BenchmarkConfig{
		Duration:     g007BenchmarkDuration(),
		ScanInterval: g007BenchmarkScanInterval,
		Devices:      g007BenchmarkDevices,
		PointsPerDev: g007BenchmarkPointsPerDev,
	})

	if result.TasksFailed > 0 {
		t.Errorf("expected zero failed tasks, got %d", result.TasksFailed)
	}
	if result.ScanMissDeadline > SLAScanMissDeadlineMax {
		t.Errorf("scan miss deadline total %d exceeds %d", result.ScanMissDeadline, SLAScanMissDeadlineMax)
	}
	if result.ThroughputDevicesSec < g007ThroughputTarget {
		t.Errorf("throughput %.0f devices/s below target %.0f devices/s (succeeded=%d duration=%s)",
			result.ThroughputDevicesSec, g007ThroughputTarget, result.TasksSucceeded, result.Duration)
	}
}
