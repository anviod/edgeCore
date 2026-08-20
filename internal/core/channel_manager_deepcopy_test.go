package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

// TestChannelManager_GetDeviceReturnsDeepCopy verifies that mutating the
// device returned by GetDevice (Points/Config) does not corrupt the stored
// channel. Regression for the MCP update_point bug where a shallow copy shared
// the Points backing array and silently skipped the scan-engine restart.
func TestChannelManager_GetDeviceReturnsDeepCopy(t *testing.T) {
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-deepcopy"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "DeepCopy Ch",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{{
			ID:       "dev-1",
			Name:     "Device 1",
			Enable:   true,
			Interval: model.Duration(10_000_000_000), // 10s
			Config:   map[string]any{"slave_id": 1},
			Points: []model.Point{
				{ID: "pt-1", Name: "P1", Address: "0", DataType: "int16", ScanClass: "fast"},
				{ID: "pt-2", Name: "P2", Address: "1", DataType: "int16", ScanClass: "normal"},
			},
		}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	// Mutate the returned copy.
	dev := cm.GetDevice(channelID, "dev-1")
	if dev == nil {
		t.Fatal("GetDevice returned nil")
	}
	dev.Points[0].ScanClass = "normal"
	dev.Points[0].Address = "999"
	dev.Config["slave_id"] = 99

	// Re-fetch and verify the stored device is unchanged.
	stored := cm.GetDevice(channelID, "dev-1")
	if stored.Points[0].ScanClass != "fast" {
		t.Fatalf("Points backing array aliased: scan_class=%q", stored.Points[0].ScanClass)
	}
	if stored.Points[0].Address != "0" {
		t.Fatalf("Points backing array aliased: address=%q", stored.Points[0].Address)
	}
	if stored.Config["slave_id"] != 1 {
		t.Fatalf("Config map aliased: slave_id=%v", stored.Config["slave_id"])
	}
}

// TestChannelManager_GetChannelDevicesDeepCopy verifies slice isolation.
func TestChannelManager_GetChannelDevicesDeepCopy(t *testing.T) {
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-deepcopy2"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "DeepCopy Ch2",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{{
			ID:       "dev-1",
			Name:     "Device 1",
			Enable:   true,
			Interval: model.Duration(10_000_000_000),
			Config:   map[string]any{"slave_id": 1},
			Points:   []model.Point{{ID: "pt-1", Name: "P1", Address: "0", DataType: "int16", ScanClass: "fast"}},
		}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	devs := cm.GetChannelDevices(channelID)
	requireLen(t, devs, 1)
	devs[0].Points[0].ScanClass = "slow"
	devs[0].Config["slave_id"] = 7

	stored := cm.GetDevice(channelID, "dev-1")
	if stored.Points[0].ScanClass != "fast" {
		t.Fatalf("GetChannelDevices Points aliased: scan_class=%q", stored.Points[0].ScanClass)
	}
	if stored.Config["slave_id"] != 1 {
		t.Fatalf("GetChannelDevices Config aliased: slave_id=%v", stored.Config["slave_id"])
	}
}

func requireLen(t *testing.T, items []model.Device, want int) {
	t.Helper()
	if len(items) != want {
		t.Fatalf("len = %d, want %d", len(items), want)
	}
}

// TestUpdatePoint_ScanClassChangeTriggersRestart reproduces the real-world bug:
// before the GetDevice deep-copy fix, mcpUpdatePoint mutated the point through a
// shallow copy whose Points slice shared the stored device's backing array, so
// UpdatePoint compared an already-mutated "before" against "after" and silently
// skipped the scan-engine restart. The stored scan_class stayed fast (100ms),
// which is why a 10s device flooded EAN events at ~167 msg/s.
func TestUpdatePoint_ScanClassChangeTriggersRestart(t *testing.T) {
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-scanclass"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "ScanClass Ch",
		Protocol: addChannelMockProtocol,
		Enable:   true,
		Config:   map[string]any{},
		Devices: []model.Device{{
			ID:       "dev-1",
			Name:     "Device 1",
			Enable:   true,
			Interval: model.Duration(10_000_000_000), // 10s
			Config:   map[string]any{"slave_id": 1},
			Points:   []model.Point{{ID: "pt-1", Name: "P1", Address: "0", DataType: "int16", ScanClass: "fast"}},
		}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	// Simulate mcpUpdatePoint: fetch a copy, mutate scan_class, persist via UpdatePoint.
	dev := cm.GetDevice(channelID, "dev-1")
	if dev == nil {
		t.Fatal("GetDevice returned nil")
	}
	pt := &dev.Points[0]
	pt.ScanClass = "normal"

	deviceRestarted, err := cm.UpdatePoint(channelID, "dev-1", pt)
	if err != nil {
		t.Fatalf("UpdatePoint: %v", err)
	}
	if !deviceRestarted {
		t.Fatal("scan_class change should trigger device restart (scan-engine re-registration)")
	}

	// The stored point must now be normal so the next registration uses 10s cadence.
	stored := cm.GetDevice(channelID, "dev-1")
	if stored.Points[0].ScanClass != "normal" {
		t.Fatalf("stored scan_class = %q, want normal", stored.Points[0].ScanClass)
	}
}

