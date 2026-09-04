package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestShadowBridge_PushToPipeline(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
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

	bridge := NewShadowBridge(pipeline)
	bridge.Attach(sc)

	collectedAt := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	msg := model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		ChannelID: "ch-1",
		Timestamp: collectedAt,
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 12.3, Quality: "good", CollectedAt: collectedAt},
			{PointID: "p2", Value: 45.6, Quality: "bad", CollectedAt: collectedAt},
		},
		Meta: model.ShadowIngressMeta{Source: "test"},
	}

	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("WriteShadowDevice failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 pipeline values, got %d", len(received))
	}

	byPoint := make(map[string]model.Value, len(received))
	for _, v := range received {
		byPoint[v.PointID] = v
	}

	p1, ok := byPoint["p1"]
	if !ok {
		t.Fatal("missing point p1 in pipeline")
	}
	if p1.ChannelID != "ch-1" || p1.DeviceID != "dev-1" {
		t.Errorf("unexpected routing: %+v", p1)
	}
	if p1.Value != 12.3 || p1.Quality != "Good" {
		t.Errorf("unexpected p1 value/quality: %+v", p1)
	}
	if !p1.TS.Equal(collectedAt) {
		t.Errorf("expected collected_at %v, got %v", collectedAt, p1.TS)
	}

	p2 := byPoint["p2"]
	if p2.Quality != "Bad" {
		t.Errorf("expected Bad quality, got %q", p2.Quality)
	}
}

func TestChannelManager_FinalizeScanCollect(t *testing.T) {
	cm := NewChannelManager(NewDataPipeline(10), nil)
	cm.stateManager.RegisterNode("dev-1", "Device 1")
	node := cm.stateManager.GetNode("dev-1")
	if node == nil {
		t.Fatal("device node not registered")
	}

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {Quality: "Good"},
			"p2": {Quality: "Bad"},
		},
	})
	if node.Runtime.State != NodeStateOnline {
		t.Errorf("expected Online after partial success, got %v", node.Runtime.State)
	}

	cm.finalizeScanCollect("dev-1", &ExecuteResult{Success: false, Error: ErrTimeout})
	if node.Runtime.State == NodeStateOnline {
		t.Error("expected offline/degraded after total failure")
	}
}
