//go:build integration

package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestStress_ConcurrentWrites(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	goroutines := 100
	opsPerGoroutine := 100

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	start := time.Now()

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				msg := model.ShadowIngressMessage{
					MessageID: fmt.Sprintf("stress-%d-%d", goroutineID, i),
					DeviceID:  fmt.Sprintf("device-%d", goroutineID%10),
					ChannelID: "channel-1",
					Timestamp: time.Now(),
					Points: []model.ShadowIngressPoint{
						{PointID: "point-1", Value: float64(i), Quality: "good"},
					},
				}

				_, err := sc.WriteShadowDevice(msg)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalOps := goroutines * opsPerGoroutine
	throughput := float64(totalOps) / elapsed.Seconds()

	t.Logf("Concurrent Write Stress Test Results:")
	t.Logf("  Goroutines: %d", goroutines)
	t.Logf("  Operations per goroutine: %d", opsPerGoroutine)
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Total time: %v", elapsed)
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	if errorCount > 0 {
		t.Logf("Warning: %d errors occurred during stress test", errorCount)
	}
}

func TestStress_ConcurrentReadWrite(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	for i := 0; i < 50; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	var writeCount int64
	var readCount int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					if id%2 == 0 {
						msg := model.ShadowIngressMessage{
							MessageID: fmt.Sprintf("rw-%d-%d", id, time.Now().UnixNano()),
							DeviceID:  fmt.Sprintf("device-%d", id%50),
							ChannelID: "channel-1",
							Timestamp: time.Now(),
							Points: []model.ShadowIngressPoint{
								{PointID: "point-1", Value: float64(time.Now().Unix()), Quality: "good"},
							},
						}
						sc.WriteShadowDevice(msg)
						atomic.AddInt64(&writeCount, 1)
					} else {
						deviceID := fmt.Sprintf("shadow-device-%d", id%50)
						sc.GetShadowDevice(deviceID)
						atomic.AddInt64(&readCount, 1)
					}
				}
			}
		}(i)
	}

	duration := 5 * time.Second
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()

	t.Logf("Concurrent Read/Write Stress Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Write operations: %d", writeCount)
	t.Logf("  Read operations: %d", readCount)
	t.Logf("  Total operations: %d", writeCount+readCount)
	t.Logf("  Write throughput: %.2f ops/sec", float64(writeCount)/duration.Seconds())
	t.Logf("  Read throughput: %.2f ops/sec", float64(readCount)/duration.Seconds())
}

func TestStress_HighVolumeIngest(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10000, 100*time.Millisecond)
	si.Start()
	defer si.Stop()

	totalMessages := 100000
	batchSize := 100

	start := time.Now()

	for i := 0; i < totalMessages; i += batchSize {
		values := make([]model.Value, batchSize)
		for j := 0; j < batchSize; j++ {
			values[j] = model.Value{
				ChannelID: "channel-1",
				DeviceID:  fmt.Sprintf("device-%d", (i+j)%100),
				PointID:   fmt.Sprintf("point-%d", (i+j)%10),
				Value:     float64(i + j),
				Quality:   "good",
				TS:        time.Now(),
			}
		}
		si.IngestBatch(values)
	}

	elapsed := time.Since(start)
	time.Sleep(500 * time.Millisecond)

	metrics := si.GetMetrics()
	throughput := float64(totalMessages) / elapsed.Seconds()

	t.Logf("High Volume Ingest Stress Test Results:")
	t.Logf("  Total messages: %d", totalMessages)
	t.Logf("  Batch size: %d", batchSize)
	t.Logf("  Ingest time: %v", elapsed)
	t.Logf("  Ingest throughput: %.2f msgs/sec", throughput)
	t.Logf("  Total points ingested: %d", metrics.TotalPoints)
	t.Logf("  Final buffer size: %d", si.GetBufferSize())
}

func TestStress_LongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running stress test in short mode")
	}

	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	duration := 30 * time.Second
	var opCount int64

	start := time.Now()
	end := start.Add(duration)

	for time.Now().Before(end) {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("long-%d", opCount),
			DeviceID:  fmt.Sprintf("device-%d", opCount%100),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(opCount), Quality: "good"},
			},
		}

		sc.WriteShadowDevice(msg)
		opCount++

		if opCount%10000 == 0 {
			t.Logf("Progress: %d operations completed", opCount)
		}
	}

	elapsed := time.Since(start)
	throughput := float64(opCount) / elapsed.Seconds()

	t.Logf("Long Running Stress Test Results:")
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  Total operations: %d", opCount)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Average latency: %v", elapsed/time.Duration(opCount))
}

func TestStress_MemoryPressure(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	iterations := 10000
	pointsPerMsg := 50

	for i := 0; i < iterations; i++ {
		points := make([]model.ShadowIngressPoint, pointsPerMsg)
		for p := 0; p < pointsPerMsg; p++ {
			points[p] = model.ShadowIngressPoint{
				PointID: fmt.Sprintf("point-%d", p),
				Value:   float64(i*p) / 100.0,
				Unit:    "V",
				Quality: "good",
			}
		}

		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("mem-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i%100),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points:    points,
		}

		_, err := sc.WriteShadowDevice(msg)
		if err != nil {
			t.Fatalf("Write failed at iteration %d: %v", i, err)
		}

		if i%1000 == 0 {
			metrics := sc.GetMetrics()
			t.Logf("Iteration %d: %d shadow devices", i, metrics["real_shadow_count"])
		}
	}

	metrics := sc.GetMetrics()
	t.Logf("Memory Pressure Stress Test Results:")
	t.Logf("  Total iterations: %d", iterations)
	t.Logf("  Points per message: %d", pointsPerMsg)
	t.Logf("  Total points: %d", iterations*pointsPerMsg)
	t.Logf("  Shadow devices created: %d", metrics["real_shadow_count"])

	if metrics["real_shadow_count"].(int) > 100 {
		t.Errorf("Expected at most 100 shadow devices, got %d", metrics["real_shadow_count"])
	}
}

func TestStress_SubscriberNotification(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	subscriberCount := 10
	var notificationCount int64
	var wg sync.WaitGroup

	for i := 0; i < subscriberCount; i++ {
		wg.Add(1)
		sc.Subscribe(func(deviceID string, points map[string]model.ShadowPoint) {
			atomic.AddInt64(&notificationCount, 1)
		})
	}

	writeCount := 1000

	for i := 0; i < writeCount; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("sub-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i%10),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	time.Sleep(100 * time.Millisecond)

	t.Logf("Subscriber Notification Stress Test Results:")
	t.Logf("  Subscriber count: %d", subscriberCount)
	t.Logf("  Write count: %d", writeCount)
	t.Logf("  Expected notifications: %d", writeCount)
	t.Logf("  Actual notifications: %d", notificationCount)

	if notificationCount < int64(writeCount) {
		t.Logf("Warning: Not all notifications received")
	}
}

func TestStress_VirtualDeviceComputation(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	for i := 0; i < 10; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-%d", i),
			DeviceID:  fmt.Sprintf("dev%d", i),
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "temp", Value: float64(20 + i), Quality: "good"},
				{PointID: "humidity", Value: float64(50 + i), Quality: "good"},
			},
		}
		sc.WriteShadowDevice(msg)
	}

	virtualDeviceCount := 100

	for i := 0; i < virtualDeviceCount; i++ {
		formulaPoints := map[string]string{
			"sum": fmt.Sprintf("ch1.dev%d.temp + ch1.dev%d.humidity", i%10, i%10),
		}
		vse.CreateVirtualDevice(fmt.Sprintf("virtual-%d", i), "ch1", formulaPoints)
	}

	time.Sleep(200 * time.Millisecond)

	metrics := vse.GetMetrics()

	t.Logf("Virtual Device Computation Stress Test Results:")
	t.Logf("  Real devices: 10")
	t.Logf("  Virtual devices: %d", virtualDeviceCount)
	t.Logf("  Virtual device count: %d", metrics["virtual_device_count"])
	t.Logf("  Total formulas: %d", metrics["total_formulas"])
}
