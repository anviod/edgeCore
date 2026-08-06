package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

func TestScenario_CoolDownLinkErrorMarksAllOffline(t *testing.T) {
	cm := newTestChannelManager()
	cm.drivers["ch-1"] = &stubChannelDriver{health: drv.HealthStatusBad}
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: false,
		Error:   errors.New("Modbus connection failed, entering coolDown"),
	})

	for _, id := range []string{"dev-1", "dev-2"} {
		if cm.stateManager.GetNode(id).Runtime.State != NodeStateOffline {
			t.Fatalf("device %s expected offline after coolDown link error", id)
		}
	}
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	cm := newTestChannelManager()
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: false,
		Error:   errors.New("i/o timeout"),
	})

	if cm.stateManager.GetNode("dev-2").Runtime.State != NodeStateOnline {
		t.Fatalf("peer device must stay online when only one device times out")
	}

	stats := cm.GetChannelStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 channel stat, got %d", len(stats))
	}
	if stats[0].Status == "Offline" {
		t.Fatalf("channel must not be offline when link is up and one device fails, got %s", stats[0].Status)
	}
}

func TestScenario_MockReadWriteViaStubDriver(t *testing.T) {
	d := &stubChannelDriver{health: drv.HealthStatusGood}
	ctx := context.Background()

	readResults, err := d.ReadPoints(ctx, []model.Point{{ID: "p1", DataType: "INT16"}})
	if err != nil {
		t.Fatalf("mock read failed: %v", err)
	}
	if readResults != nil && len(readResults) != 0 {
		t.Fatalf("stub driver returns empty map, got %v", readResults)
	}
	if err := d.WritePoint(ctx, model.Point{ID: "p1"}, 42); err != nil {
		t.Fatalf("mock write failed: %v", err)
	}
}

func TestScenario_ConcurrentChannelStats(t *testing.T) {
	cm := newTestChannelManager()
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	var wg sync.WaitGroup
	var ops int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats := cm.GetChannelStats()
			if len(stats) == 1 && stats[0].Status != "Offline" {
				atomic.AddInt32(&ops, 1)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&ops) != 30 {
		t.Fatalf("concurrent GetChannelStats returned unexpected results")
	}
}
