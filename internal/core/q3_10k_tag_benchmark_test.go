package core

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

const (
	q3BenchmarkDevices        = 100
	q3BenchmarkPointsPerDev   = 100
	q3BenchmarkTotalTags      = q3BenchmarkDevices * q3BenchmarkPointsPerDev
	q3BenchmarkScanInterval   = time.Second
	q3BenchmarkDefaultRuntime = 60 * time.Second
	q3BenchmarkWarmup         = 10 * time.Second
)

type q3BenchmarkConfig struct {
	Duration       time.Duration
	ScanInterval   time.Duration
	Devices        int
	PointsPerDev   int
	WithPipeline   bool
	WithVirtualDev bool
}

type q3BenchmarkResult struct {
	Config              q3BenchmarkConfig
	Duration            time.Duration
	WarmupDuration      time.Duration
	MemInuseStartMB     float64
	MemInuseEndMB       float64
	MemInuseDriftPct    float64
	HeapObjectsStart    uint64
	HeapObjectsEnd      uint64
	TasksExecuted       uint64
	TasksSucceeded      uint64
	TasksFailed         uint64
	ScanLagAvgMs        float64
	ScanLagP95Ms        float64
	ScanLagMaxMs        float64
	ScanDriftAvgMs      float64
	ScanMissDeadline    uint64
	StarvationRescues   uint64
	TaskOverdueTotal    uint64
	PipelineValues      uint64
	ShadowDevices       int
	ThroughputPointsSec float64
	GoroutinesStart     int
	GoroutinesEnd       int
	GCPauseMaxMs        float64
}

func q3BenchmarkStabilizeHeap() {
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}
}

func q3BenchmarkDuration() time.Duration {
	if raw := os.Getenv("Q3_BENCH_DURATION"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return q3BenchmarkDefaultRuntime
}

func runQ3TenThousandTagBenchmark(t *testing.T, cfg q3BenchmarkConfig) q3BenchmarkResult {
	t.Helper()

	if cfg.Duration <= 0 {
		cfg.Duration = q3BenchmarkDuration()
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = q3BenchmarkScanInterval
	}
	if cfg.Devices <= 0 {
		cfg.Devices = q3BenchmarkDevices
	}
	if cfg.PointsPerDev <= 0 {
		cfg.PointsPerDev = q3BenchmarkPointsPerDev
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

	var pipelineCount atomic.Uint64
	var pipeline *DataPipeline
	if cfg.WithPipeline {
		pipeline = NewDataPipeline(10000)
		pipeline.AddHandler(func(v model.Value) {
			pipelineCount.Add(1)
		})
		pipeline.Start()
		NewShadowBridge(pipeline).Attach(sc)
	}

	if cfg.WithVirtualDev {
		_ = NewVirtualShadowEngine(sc)
	}

	for i := 0; i < cfg.Devices; i++ {
		deviceKey := fmt.Sprintf("bench_dev_%03d", i)
		pointIDs := make([]string, cfg.PointsPerDev)
		for j := 0; j < cfg.PointsPerDev; j++ {
			pointIDs[j] = fmt.Sprintf("p%03d", j)
		}
		params := map[string]any{
			"channelID":        "bench-ch",
			"degradeOnFailure": false,
		}
		se.AddTask(deviceKey, "modbus-tcp", cfg.ScanInterval, 5, pointIDs, params)
		se.RegisterDriver(deviceKey, &mockStressDriver{})
	}

	var memStart runtime.MemStats
	goroutinesStart := runtime.NumGoroutine()

	se.Run()
	time.Sleep(q3BenchmarkWarmup)
	se.GetMetrics().ResetWindow()
	q3BenchmarkStabilizeHeap()
	runtime.ReadMemStats(&memStart)
	goroutinesStart = runtime.NumGoroutine()

	time.Sleep(cfg.Duration)
	se.Stop()
	runtime.GC()

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)
	goroutinesEnd := runtime.NumGoroutine()

	metrics := se.GetMetrics().Snapshot()
	shadowMetrics := sc.GetMetrics()

	gcPauseMax := 0.0
	if gc := se.GetGCMonitor(); gc != nil {
		if v, ok := gc.Metrics().Snapshot()["gc_pause_max_ms"].(float64); ok {
			gcPauseMax = v
		}
	}

	memStartMB := float64(memStart.HeapInuse) / (1024 * 1024)
	memEndMB := float64(memEnd.HeapInuse) / (1024 * 1024)
	driftPct := 0.0
	if memStartMB > 0 {
		driftPct = (memEndMB - memStartMB) / memStartMB * 100
	}

	tasksExecuted, _ := metrics["tasks_executed"].(uint64)
	tasksSucceeded, _ := metrics["tasks_succeeded"].(uint64)
	tasksFailed, _ := metrics["tasks_failed"].(uint64)
	starvation, _ := metrics["starvation_rescue_total"].(uint64)
	overdue, _ := metrics["task_overdue_total"].(uint64)
	avgLag, _ := metrics["scan_lag_avg_ms"].(float64)
	p95Lag, _ := metrics["scan_lag_p95_ms"].(float64)
	maxLag, _ := metrics["scan_lag_max_ms"].(float64)
	driftAvg, _ := metrics["scan_drift_avg_ms"].(float64)
	missDeadline, _ := metrics["scan_miss_deadline_total"].(uint64)

	shadowCount := 0
	if v, ok := shadowMetrics["real_shadow_count"].(int); ok {
		shadowCount = v
	}

	totalPoints := uint64(tasksSucceeded) * uint64(cfg.PointsPerDev)
	throughput := float64(totalPoints) / cfg.Duration.Seconds()

	result := q3BenchmarkResult{
		Config:              cfg,
		Duration:            cfg.Duration,
		WarmupDuration:      q3BenchmarkWarmup,
		MemInuseStartMB:     memStartMB,
		MemInuseEndMB:       memEndMB,
		MemInuseDriftPct:    driftPct,
		HeapObjectsStart:    memStart.HeapObjects,
		HeapObjectsEnd:      memEnd.HeapObjects,
		TasksExecuted:       tasksExecuted,
		TasksSucceeded:      tasksSucceeded,
		TasksFailed:         tasksFailed,
		ScanLagAvgMs:        avgLag,
		ScanLagP95Ms:        p95Lag,
		ScanLagMaxMs:        maxLag,
		ScanDriftAvgMs:      driftAvg,
		ScanMissDeadline:    missDeadline,
		StarvationRescues:   starvation,
		TaskOverdueTotal:    overdue,
		PipelineValues:      pipelineCount.Load(),
		ShadowDevices:       shadowCount,
		ThroughputPointsSec: throughput,
		GoroutinesStart:     goroutinesStart,
		GoroutinesEnd:       goroutinesEnd,
		GCPauseMaxMs:        gcPauseMax,
	}

	logQ3BenchmarkResult(t, result)
	return result
}

func logQ3BenchmarkResult(t *testing.T, r q3BenchmarkResult) {
	t.Helper()
	totalTags := r.Config.Devices * r.Config.PointsPerDev
	t.Logf("Q3 10k Tag Benchmark")
	t.Logf("  duration=%s warmup=%s interval=%s devices=%d points/device=%d total_tags=%d pipeline=%v virtual=%v",
		r.Duration, r.WarmupDuration, r.Config.ScanInterval, r.Config.Devices, r.Config.PointsPerDev, totalTags, r.Config.WithPipeline, r.Config.WithVirtualDev)
	t.Logf("  memory(heap_inuse): start=%.2fMB end=%.2fMB drift=%.2f%% heap_objs=%d->%d",
		r.MemInuseStartMB, r.MemInuseEndMB, r.MemInuseDriftPct, r.HeapObjectsStart, r.HeapObjectsEnd)
	t.Logf("  scan: executed=%d succeeded=%d failed=%d lag_avg=%.2fms lag_p95=%.2fms lag_max=%.2fms drift_avg=%.2fms miss_deadline=%d overdue=%d rescue=%d",
		r.TasksExecuted, r.TasksSucceeded, r.TasksFailed, r.ScanLagAvgMs, r.ScanLagP95Ms, r.ScanLagMaxMs, r.ScanDriftAvgMs, r.ScanMissDeadline, r.TaskOverdueTotal, r.StarvationRescues)
	t.Logf("  pipeline_values=%d shadow_devices=%d throughput=%.0f points/s goroutines=%d->%d gc_pause_max=%.2fms",
		r.PipelineValues, r.ShadowDevices, r.ThroughputPointsSec, r.GoroutinesStart, r.GoroutinesEnd, r.GCPauseMaxMs)
}

func TestQ3_TenThousandTagBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10k tag benchmark in short mode")
	}

	result := runQ3TenThousandTagBenchmark(t, q3BenchmarkConfig{
		Duration:       q3BenchmarkDuration(),
		ScanInterval:   q3BenchmarkScanInterval,
		Devices:        q3BenchmarkDevices,
		PointsPerDev:   q3BenchmarkPointsPerDev,
		WithPipeline:   true,
		WithVirtualDev: false,
	})

	if result.TasksFailed > 0 {
		t.Errorf("expected zero failed tasks, got %d", result.TasksFailed)
	}
	if result.ScanLagP95Ms > 100 {
		t.Errorf("scan lag P95 %.2fms exceeds 100ms SLA", result.ScanLagP95Ms)
	}
	if result.ScanDriftAvgMs > SLAScanDriftAvgMsThreshold {
		t.Errorf("scan drift avg %.2fms exceeds %.0fms SLA", result.ScanDriftAvgMs, SLAScanDriftAvgMsThreshold)
	}
	if result.ScanMissDeadline > SLAScanMissDeadlineMax {
		t.Errorf("scan miss deadline total %d exceeds %d", result.ScanMissDeadline, SLAScanMissDeadlineMax)
	}
	if result.MemInuseDriftPct > 5 {
		t.Errorf("memory drift %.2f%% exceeds 5%% threshold", result.MemInuseDriftPct)
	}
	if result.GCPauseMaxMs >= 20 {
		t.Errorf("gc_pause_max_ms %.2f exceeds 20ms SLA gate", result.GCPauseMaxMs)
	}
	expectedMinPipeline := uint64(result.TasksSucceeded) * uint64(result.Config.PointsPerDev) / 2
	if result.PipelineValues < expectedMinPipeline {
		t.Errorf("pipeline received %d values, expected >= %d", result.PipelineValues, expectedMinPipeline)
	}
}

func TestScanEngineMetrics_LagP95(t *testing.T) {
	m := &ScanEngineMetrics{}
	lags := []int64{10_000, 20_000, 30_000, 40_000, 100_000, 200_000}
	for _, lag := range lags {
		m.RecordExecute(true, lag)
	}
	snap := m.Snapshot()
	p95, ok := snap["scan_lag_p95_ms"].(float64)
	if !ok {
		t.Fatal("missing scan_lag_p95_ms")
	}
	if p95 < 100 || p95 > 200 {
		t.Fatalf("expected P95 near 100-200ms, got %.2f", p95)
	}
}

func TestScanEngine_TenThousandTagsBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10k tag benchmark in short mode")
	}

	runQ3TenThousandTagBenchmark(t, q3BenchmarkConfig{
		Duration:     3 * time.Second,
		ScanInterval: time.Second,
		WithPipeline: false,
	})
}
