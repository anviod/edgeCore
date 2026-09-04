package core

import (
	"testing"

	_ "github.com/anviod/edgeCore/internal/driver/dlt645"
	"github.com/anviod/edgeCore/internal/model"
)

func TestAddDevice_DLT645_AutoDefaultPoints(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dlt645-auto"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "DLT645 Auto Points",
		Protocol: "dlt645",
		Config:   map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	dev := &model.Device{
		ID:   "dev-meter-1",
		Name: "Meter 1",
		Config: map[string]any{
			"station_address":     "123456789012",
			"auto_points_enabled": true,
		},
	}
	if err := cm.AddDevice(channelID, dev); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}

	stored := cm.GetDevice(channelID, "dev-meter-1")
	if stored == nil {
		t.Fatal("device not found after AddDevice")
	}
	if len(stored.Points) != len(DLT645StandardPointTemplates) {
		t.Fatalf("expected %d auto points, got %d", len(DLT645StandardPointTemplates), len(stored.Points))
	}
	for _, p := range stored.Points {
		if err := cm.validateDLT645Point(&p); err != nil {
			t.Fatalf("generated point %q failed validation: %v", p.ID, err)
		}
		if p.DeviceID != "dev-meter-1" {
			t.Fatalf("point %q has wrong device id: %s", p.ID, p.DeviceID)
		}
	}
}

func TestAddDevice_DLT645_AutoPointsDisabled(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dlt645-off"
	if err := cm.AddChannel(&model.Channel{
		ID: channelID, Name: "DLT645 Off", Protocol: "dlt645", Config: map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	dev := &model.Device{
		ID:   "dev-meter-2",
		Name: "Meter 2",
		Config: map[string]any{
			"station_address":     "123456789012",
			"auto_points_enabled": false,
		},
	}
	if err := cm.AddDevice(channelID, dev); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	stored := cm.GetDevice(channelID, "dev-meter-2")
	if len(stored.Points) != 0 {
		t.Fatalf("expected no auto points when disabled, got %d", len(stored.Points))
	}
}

func TestAddDevice_DLT645_SkipsAutoWhenPointsProvided(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dlt645-existing"
	if err := cm.AddChannel(&model.Channel{
		ID: channelID, Name: "DLT645 Existing", Protocol: "dlt645", Config: map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	dev := &model.Device{
		ID:   "dev-meter-3",
		Name: "Meter 3",
		Config: map[string]any{
			"station_address":     "123456789012",
			"auto_points_enabled": true,
		},
		Points: []model.Point{{
			ID: "custom_voltage", Name: "Custom", Address: "123456789012#02-01-01-00", DataType: "uint16",
		}},
	}
	if err := cm.AddDevice(channelID, dev); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	stored := cm.GetDevice(channelID, "dev-meter-3")
	if len(stored.Points) != 1 || stored.Points[0].ID != "custom_voltage" {
		t.Fatalf("expected only provided point, got %+v", stored.Points)
	}
}

func TestAddDevice_DLT645_NoAddressSkipsAutoPoints(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dlt645-noaddr"
	if err := cm.AddChannel(&model.Channel{
		ID: channelID, Name: "DLT645 No Addr", Protocol: "dlt645", Config: map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	dev := &model.Device{
		ID:     "dev-meter-4",
		Name:   "Meter 4",
		Config: map[string]any{"auto_points_enabled": true},
	}
	if err := cm.AddDevice(channelID, dev); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	stored := cm.GetDevice(channelID, "dev-meter-4")
	if len(stored.Points) != 0 {
		t.Fatalf("expected no points without station address, got %d", len(stored.Points))
	}
}

func TestGenerateDLT645StandardPoints_ContainsExtensionDI(t *testing.T) {
	points := GenerateDLT645StandardPoints("123456789012", "dev-1")
	extensionChecks := map[string]string{
		"forward_active_max_demand_time":                   "123456789012#01-01-00-00#T",
		"reverse_active_max_demand_time":                   "123456789012#01-02-00-00#T",
		"last_timed_freeze_forward_active_max_demand_time": "123456789012#05-00-09-01#T",
	}
	for id, wantAddr := range extensionChecks {
		var found bool
		for _, p := range points {
			if p.ID != id {
				continue
			}
			found = true
			if p.Address != wantAddr {
				t.Fatalf("point %q: unexpected address %s, want %s", id, p.Address, wantAddr)
			}
			if p.DataType != "string" {
				t.Fatalf("point %q: expected string datatype, got %s", id, p.DataType)
			}
		}
		if !found {
			t.Fatalf("expected #T extension point %q in template", id)
		}
	}
}

func TestDLT645StandardPointTemplates_KeyDIs(t *testing.T) {
	want := map[string]struct {
		dataID   string
		dataType string
		scale    float64
		unit     string
	}{
		"forward_active_energy":                     {dataID: "00-01-00-00", dataType: "uint64", scale: 0.01, unit: "kWh"},
		"b_phase_voltage":                           {dataID: "02-01-02-00", dataType: "uint16", scale: 0.1, unit: "V"},
		"total_active_power":                        {dataID: "02-03-00-00", dataType: "int32", scale: 0.0001, unit: "kW"},
		"reverse_active_max_demand":                 {dataID: "01-02-00-00", dataType: "uint64", scale: 0.0001, unit: "kW"},
		"last_instant_freeze_forward_active_energy": {dataID: "05-01-01-01", dataType: "uint32", scale: 0.01, unit: "kWh"},
		"tariff_count":                              {dataID: "04-00-02-04", dataType: "uint8", scale: 0, unit: ""},
	}
	byID := make(map[string]DLT645PointTemplate, len(DLT645StandardPointTemplates))
	for _, tpl := range DLT645StandardPointTemplates {
		if _, dup := byID[tpl.ID]; dup {
			t.Fatalf("duplicate template id: %s", tpl.ID)
		}
		byID[tpl.ID] = tpl
	}
	if len(DLT645StandardPointTemplates) != 61 {
		t.Fatalf("expected 61 standard templates, got %d", len(DLT645StandardPointTemplates))
	}
	for id, spec := range want {
		tpl, ok := byID[id]
		if !ok {
			t.Fatalf("missing template id %q", id)
		}
		if tpl.DataID != spec.dataID {
			t.Fatalf("template %q: dataID %s, want %s", id, tpl.DataID, spec.dataID)
		}
		if tpl.DataType != spec.dataType {
			t.Fatalf("template %q: dataType %s, want %s", id, tpl.DataType, spec.dataType)
		}
		if tpl.Scale != spec.scale {
			t.Fatalf("template %q: scale %v, want %v", id, tpl.Scale, spec.scale)
		}
		if tpl.Unit != spec.unit {
			t.Fatalf("template %q: unit %q, want %q", id, tpl.Unit, spec.unit)
		}
	}
}

func TestGenerateDLT645StandardPoints_AllValidate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()
	points := GenerateDLT645StandardPoints("123456789012", "dev-1")
	if len(points) != len(DLT645StandardPointTemplates) {
		t.Fatalf("expected %d points, got %d", len(DLT645StandardPointTemplates), len(points))
	}
	for _, p := range points {
		if err := cm.validateDLT645Point(&p); err != nil {
			t.Fatalf("point %q (%s) failed validation: %v", p.ID, p.Address, err)
		}
	}
}

func TestIsDLT645AutoPointsEnabled(t *testing.T) {
	if !IsDLT645AutoPointsEnabled(nil) {
		t.Fatal("expected default enabled")
	}
	if !IsDLT645AutoPointsEnabled(map[string]any{}) {
		t.Fatal("expected default enabled when key missing")
	}
	if IsDLT645AutoPointsEnabled(map[string]any{"auto_points_enabled": false}) {
		t.Fatal("expected disabled")
	}
}
