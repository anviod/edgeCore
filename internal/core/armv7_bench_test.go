//go:build bench

package core

import (
	"testing"
	"time"
)

/*
ARMv7 cross-compile benchmark gate (Sprint 3 production validation).

Compile-only (no ARM hardware required):

	make bench-armv7
	# or:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go test -c ./internal/core/ -o /tmp/edgeCore-armv7.test

Run on ARMv7 board or via qemu-user:

	Q3_BENCH_DURATION=60 /tmp/edgeCore-armv7.test -test.run TestARMv7_Q3BenchmarkGate -test.v -test.timeout=15m
	qemu-arm /tmp/edgeCore-armv7.test -test.run TestARMv7_Q3BenchmarkGate -test.v -test.timeout=15m

Micro-benchmarks (native or cross-compiled binary):

	go test -tags=bench ./internal/core/ -run '^$' -bench 'Benchmark(ExecutionLayer_LoadPoints_Pooled|ScanEngine_ApplyCollectToShadow_Pooled)' -benchmem -count=3
*/

func TestARMv7_Q3BenchmarkGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ARMv7 Q3 gate in short mode")
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
	if result.ScanLagP95Ms > SLAScanLagP95MsThreshold {
		t.Errorf("scan lag P95 %.2fms exceeds %.0fms SLA", result.ScanLagP95Ms, SLAScanLagP95MsThreshold)
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
}

func TestARMv7_ScanEngineSmoke(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
	})
	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)
	se.RegisterDriver("arm-smoke", &mockStressDriver{})
	se.AddTask("arm-smoke", "modbus-tcp", 100*time.Millisecond, 5, []string{"p1"}, nil)
	se.Run()
	time.Sleep(300 * time.Millisecond)
	se.Stop()
	if se.GetMetrics().TasksSucceeded.Load() == 0 {
		t.Fatal("expected at least one successful scan on ARM smoke gate")
	}
}
