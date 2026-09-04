package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

// TestShadowPipelineIntegration 验证 Shadow → Pipeline → 单值/批量 handler 四路扇出。
func TestShadowPipelineIntegration(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	pipeline := NewDataPipeline(100)

	var mu sync.Mutex
	var singleReceived []model.Value
	var batchReceived []model.Value

	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		singleReceived = append(singleReceived, v)
		mu.Unlock()
	})
	pipeline.AddBatchHandler(func(batch []model.Value) {
		mu.Lock()
		batchReceived = append(batchReceived, batch...)
		mu.Unlock()
	})
	pipeline.Start()

	NewShadowBridge(pipeline).Attach(sc)

	collectedAt := time.Now().UTC().Truncate(time.Millisecond)
	msg := model.ShadowIngressMessage{
		DeviceID:  "dev-int",
		ChannelID: "ch-int",
		Timestamp: collectedAt,
		Points: []model.ShadowIngressPoint{
			{PointID: "t1", Value: 1, Quality: "Good", CollectedAt: collectedAt},
			{PointID: "t2", Value: 2, Quality: "Good", CollectedAt: collectedAt},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(singleReceived)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(singleReceived) < 2 {
		t.Fatalf("single handlers got %d values, want >=2", len(singleReceived))
	}
	if len(batchReceived) < 2 {
		t.Fatalf("batch handlers got %d values, want >=2", len(batchReceived))
	}

	for _, v := range singleReceived {
		if v.ChannelID != "ch-int" || v.DeviceID != "dev-int" {
			t.Errorf("unexpected routing: %+v", v)
		}
	}
}

type scanPipelineMockDriver struct {
	counter atomicCounter
}

type atomicCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCounter) next() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return float64(c.n)
}

func (d *scanPipelineMockDriver) Init(cfg model.DriverConfig) error           { return nil }
func (d *scanPipelineMockDriver) Connect(ctx context.Context) error           { return nil }
func (d *scanPipelineMockDriver) Disconnect() error                           { return nil }
func (d *scanPipelineMockDriver) Health() driver.HealthStatus                 { return driver.HealthStatusGood }
func (d *scanPipelineMockDriver) SetSlaveID(slaveID uint8) error              { return nil }
func (d *scanPipelineMockDriver) SetDeviceConfig(config map[string]any) error { return nil }
func (d *scanPipelineMockDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	return nil
}
func (d *scanPipelineMockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (d *scanPipelineMockDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value, len(points))
	now := time.Now()
	val := d.counter.next()
	for _, p := range points {
		results[p.ID] = model.Value{PointID: p.ID, Value: val, Quality: "Good", TS: now}
	}
	return results, nil
}

// TestScanEngine_ShadowPipelineEndToEnd 验证 ScanEngine → ShadowCore → Pipeline 采集闭环。
func TestScanEngine_ShadowPipelineEndToEnd(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	pipeline := NewDataPipeline(100)
	var mu sync.Mutex
	var received []model.Value
	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		received = append(received, v)
		mu.Unlock()
	})
	pipeline.Start()
	NewShadowBridge(pipeline).Attach(sc)

	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
	})
	se.SetShadowCore(sc)
	se.RegisterProtocol("modbus-tcp", ProtocolTypeParallel)
	se.AddTask("dev-e2e", "modbus-tcp", 50*time.Millisecond, 5, []string{"p1", "p2"}, map[string]any{
		"channelID": "ch-e2e",
	})
	se.RegisterDriver("dev-e2e", &scanPipelineMockDriver{})

	se.Run()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	se.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("pipeline received %d values, want >=2 from scan engine", len(received))
	}
	for _, v := range received {
		if v.ChannelID != "ch-e2e" || v.DeviceID != "dev-e2e" {
			t.Errorf("unexpected routing: %+v", v)
		}
		if v.Quality != "Good" {
			t.Errorf("expected Good quality, got %q", v.Quality)
		}
	}
}

func TestScanEngineMetrics_RecordExecute(t *testing.T) {
	m := &ScanEngineMetrics{}
	m.RecordExecute(true, 50_000)
	m.RecordStarvationRescue()
	snap := m.Snapshot()
	if snap["tasks_succeeded"].(uint64) != 1 {
		t.Fatalf("expected 1 success, got %v", snap["tasks_succeeded"])
	}
	if snap["starvation_rescue_total"].(uint64) != 1 {
		t.Fatalf("expected 1 rescue, got %v", snap["starvation_rescue_total"])
	}
}
