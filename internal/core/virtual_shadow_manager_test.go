package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestVirtualShadowManager_ReloadAll(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	vse := NewVirtualShadowEngine(sc)
	cm := NewChannelManager(nil, nil)
	mgr := NewVirtualShadowManager(vse, cm, sc, nil)

	cfg := model.VirtualShadowDeviceConfig{
		ID:     "virtual-reload",
		Enable: true,
		Points: []model.VirtualShadowPointDef{
			{PointID: "p1", Mode: "map", SourceRef: "ch1.dev1.temp"},
		},
	}
	mgr.mu.Lock()
	mgr.configs = []model.VirtualShadowDeviceConfig{cfg}
	mgr.mu.Unlock()

	mgr.ReloadAll()
	time.Sleep(50 * time.Millisecond)

	if _, err := vse.GetVirtualDevice("virtual-reload"); err != nil {
		t.Fatalf("expected virtual device after reload: %v", err)
	}

	cfg.Enable = false
	mgr.mu.Lock()
	mgr.configs = []model.VirtualShadowDeviceConfig{cfg}
	mgr.mu.Unlock()

	mgr.ReloadAll()
	time.Sleep(50 * time.Millisecond)

	if _, err := vse.GetVirtualDevice("virtual-reload"); err == nil {
		t.Fatal("expected disabled virtual device to be removed")
	}
}
