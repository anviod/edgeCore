//go:build integration

package core

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func BenchmarkShadowCore_WriteShadowDevice(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "bench-msg",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.5, Quality: "good"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MessageID = fmt.Sprintf("bench-msg-%d", i)
		sc.WriteShadowDevice(msg)
	}
}

func BenchmarkShadowCore_WriteShadowDevice_MultiPoint(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	points := make([]model.ShadowIngressPoint, 10)
	for i := 0; i < 10; i++ {
		points[i] = model.ShadowIngressPoint{
			PointID: fmt.Sprintf("point-%d", i),
			Value:   float64(i),
			Quality: "good",
		}
	}

	msg := model.ShadowIngressMessage{
		MessageID: "bench-msg",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points:    points,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MessageID = fmt.Sprintf("bench-msg-%d", i)
		sc.WriteShadowDevice(msg)
	}
}

func BenchmarkShadowCore_GetShadowDevice(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	for i := 0; i < 100; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-msg-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deviceID := fmt.Sprintf("shadow-device-%d", i%100)
		sc.GetShadowDevice(deviceID)
	}
}

func BenchmarkShadowIngress_Ingest(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 1000, time.Second)

	val := model.Value{
		ChannelID: "channel-1",
		DeviceID:  "device-1",
		PointID:   "point-1",
		Value:     42.5,
		Quality:   "good",
		TS:        time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si.Ingest(val)
	}
}

func BenchmarkShadowIngress_IngestBatch(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 1000, time.Second)

	values := make([]model.Value, 100)
	for i := 0; i < 100; i++ {
		values[i] = model.Value{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   fmt.Sprintf("point-%d", i),
			Value:     float64(i),
			Quality:   "good",
			TS:        time.Now(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		si.IngestBatch(values)
	}
}

func BenchmarkVirtualShadowEngine_CreateVirtualDevice(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"sum": "ch1.dev1.temp + ch1.dev2.temp",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vse.CreateVirtualDevice(fmt.Sprintf("virtual-%d", i), "ch1", formulaPoints)
	}
}

func BenchmarkShadowCore_CompareAndSwap(b *testing.B) {
	tmpDir := testOutputDir(b)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "init-msg",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 10.0, Quality: "good"},
		},
	}
	sc.WriteShadowDevice(msg)

	device, _ := sc.GetShadowDevice("shadow-device-1")
	version := device.Version

	updates := map[string]any{
		"point-1": 20.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc.CompareAndSwap("shadow-device-1", version, updates)
		version++
	}
}

func TestPerformance_WriteLatency(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	iterations := 1000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("perf-msg-%d", i),
			DeviceID:  "device-1",
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}

		start := time.Now()
		sc.WriteShadowDevice(msg)
		latencies[i] = time.Since(start)
	}

	var total time.Duration
	var max time.Duration
	var min time.Duration = time.Hour

	for _, lat := range latencies {
		total += lat
		if lat > max {
			max = lat
		}
		if lat < min {
			min = lat
		}
	}

	avg := total / time.Duration(iterations)

	t.Logf("Write Latency Statistics (n=%d):", iterations)
	t.Logf("  Average: %v", avg)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  P99: %v", calculatePercentile(latencies, 99))

	if avg > time.Millisecond {
		t.Logf("Warning: Average write latency exceeds 1ms: %v", avg)
	}
}

func TestPerformance_ReadLatency(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	for i := 0; i < 100; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-msg-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	iterations := 1000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		deviceID := fmt.Sprintf("shadow-device-%d", i%100)

		start := time.Now()
		sc.GetShadowDevice(deviceID)
		latencies[i] = time.Since(start)
	}

	var total time.Duration
	var max time.Duration
	var min time.Duration = time.Hour

	for _, lat := range latencies {
		total += lat
		if lat > max {
			max = lat
		}
		if lat < min {
			min = lat
		}
	}

	avg := total / time.Duration(iterations)

	t.Logf("Read Latency Statistics (n=%d):", iterations)
	t.Logf("  Average: %v", avg)
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  P99: %v", calculatePercentile(latencies, 99))

	if avg > time.Millisecond {
		t.Logf("Warning: Average read latency exceeds 1ms: %v", avg)
	}
}

func TestPerformance_Throughput(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	iterations := 10000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("perf-msg-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i%100),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	elapsed := time.Since(start)
	throughput := float64(iterations) / elapsed.Seconds()

	t.Logf("Throughput Statistics:")
	t.Logf("  Total operations: %d", iterations)
	t.Logf("  Total time: %v", elapsed)
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	if throughput < 1000 {
		t.Logf("Warning: Throughput below 1000 ops/sec: %.2f", throughput)
	}
}

func TestPerformance_MemoryUsage(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	deviceCount := 1000
	pointsPerDevice := 100

	for d := 0; d < deviceCount; d++ {
		points := make([]model.ShadowIngressPoint, pointsPerDevice)
		for p := 0; p < pointsPerDevice; p++ {
			points[p] = model.ShadowIngressPoint{
				PointID: fmt.Sprintf("point-%d", p),
				Value:   float64(p),
				Quality: "good",
			}
		}

		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("mem-msg-%d", d),
			DeviceID:  fmt.Sprintf("device-%d", d),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points:    points,
		}
		sc.WriteShadowDevice(msg)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	totalPoints := deviceCount * pointsPerDevice
	memoryPerPoint := float64(m2.Alloc-m1.Alloc) / float64(totalPoints)

	t.Logf("Memory Usage Statistics:")
	t.Logf("  Devices: %d", deviceCount)
	t.Logf("  Points per device: %d", pointsPerDevice)
	t.Logf("  Total points: %d", totalPoints)
	t.Logf("  Memory allocated: %.2f MB", float64(m2.Alloc-m1.Alloc)/1024/1024)
	t.Logf("  Memory per point: %.2f bytes", memoryPerPoint)

	if memoryPerPoint > 1024 {
		t.Logf("Warning: Memory per point exceeds 1KB: %.2f bytes", memoryPerPoint)
	}
}

func calculatePercentile(latencies []time.Duration, percentile int) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}
