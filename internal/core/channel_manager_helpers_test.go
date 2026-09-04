package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestParseTime(t *testing.T) {
	if !parseTime("").IsZero() {
		t.Fatal("empty string should be zero time")
	}
	ts := parseTime("2026-07-05T12:00:00Z")
	if ts.IsZero() {
		t.Fatal("RFC3339 time should parse")
	}
	if !parseTime("not-a-time").IsZero() {
		t.Fatal("invalid time should be zero")
	}
}

func TestPointAllowsWrite(t *testing.T) {
	cases := map[string]bool{
		"RW": true,
		"W":  true,
		"R":  false,
		"":   true,
	}
	for rw, want := range cases {
		if got := pointAllowsWrite(rw); got != want {
			t.Fatalf("pointAllowsWrite(%q) = %v, want %v", rw, got, want)
		}
	}
}

func TestPointUpdateRequiresDeviceRestart_Helper(t *testing.T) {
	before := model.Point{Address: "40001", DataType: "int16", FunctionCode: 3}
	after := before
	if pointUpdateRequiresDeviceRestart(before, after) {
		t.Fatal("identical points should not require restart")
	}

	after.Address = "40002"
	if !pointUpdateRequiresDeviceRestart(before, after) {
		t.Fatal("address change should require restart")
	}

	after = before
	after.DataType = "float32"
	if !pointUpdateRequiresDeviceRestart(before, after) {
		t.Fatal("datatype change should require restart")
	}
}

func TestModbusGenOptionsFromDevice(t *testing.T) {
	dev := &model.Device{
		ID: "dev1",
		Config: map[string]any{
			"auto_points_range":         "0-5",
			"auto_points_datatype":      "float32",
			"auto_points_readwrite":     "RW",
			"auto_points_register_type": "input",
			"auto_points_function_code": 4,
		},
	}
	opts, ok := modbusGenOptionsFromDevice(dev)
	if !ok {
		t.Fatal("expected valid options")
	}
	if opts.Start != 0 || opts.End != 5 {
		t.Fatalf("range = %d-%d", opts.Start, opts.End)
	}
	if opts.DataType != "float32" || opts.ReadWrite != "RW" {
		t.Fatalf("datatype/rw = %s/%s", opts.DataType, opts.ReadWrite)
	}
	if opts.RegisterType != model.RegInput || opts.FunctionCode != 4 {
		t.Fatalf("reg type/fc = %v/%d", opts.RegisterType, opts.FunctionCode)
	}

	if _, ok := modbusGenOptionsFromDevice(nil); ok {
		t.Fatal("nil device should fail")
	}
	if _, ok := modbusGenOptionsFromDevice(&model.Device{Config: map[string]any{}}); ok {
		t.Fatal("missing range should fail")
	}
}

func TestChannelManager_ChannelIDForDevice(t *testing.T) {
	cm := newTestChannelManager()
	if got := cm.channelIDForDevice("dev-1"); got != "ch-1" {
		t.Fatalf("channelIDForDevice = %q, want ch-1", got)
	}
	if got := cm.channelIDForDevice("missing"); got != "" {
		t.Fatalf("missing device = %q, want empty", got)
	}
}

func TestChannelManager_DeviceIOProfile(t *testing.T) {
	cm := newTestChannelManager()
	profile := cm.deviceIOProfile("dev-1")
	if profile.Gap != 64 || profile.BatchSize != 120 {
		t.Fatalf("default profile = %+v", profile)
	}
}

func TestChannelManager_GetTagRegistry(t *testing.T) {
	cm := newTestChannelManager()
	if cm.GetTagRegistry() == nil {
		t.Fatal("tag registry should be initialized")
	}
}

func TestChannelManager_GetStateManager(t *testing.T) {
	cm := newTestChannelManager()
	if cm.GetStateManager() == nil {
		t.Fatal("state manager should not be nil")
	}
}

func TestRegisterPointPrefix(t *testing.T) {
	cases := map[model.RegisterType]string{
		model.RegInput:         "ir",
		model.RegCoil:          "coil",
		model.RegDiscreteInput: "di",
		model.RegHolding:       "hr",
		model.RegisterType(99): "hr",
	}
	for reg, want := range cases {
		if got := registerPointPrefix(reg); got != want {
			t.Fatalf("registerPointPrefix(%v) = %q, want %q", reg, got, want)
		}
	}
}

func TestGenerateModbusRegisterPoints_ReversedRange(t *testing.T) {
	opts := ModbusRegisterGenOptions{Start: 5, End: 2, RegisterType: model.RegCoil, FunctionCode: 1}
	points := GenerateModbusRegisterPoints(nil, opts, false)
	if len(points) != 4 {
		t.Fatalf("expected 4 coil points, got %d", len(points))
	}
	if points[0].ID != "coil_2" {
		t.Fatalf("first point = %s", points[0].ID)
	}
}

func TestParseAutoPointsRange_Invalid(t *testing.T) {
	cases := []string{"", "abc", "1", "1-2-3"}
	for _, c := range cases {
		if _, _, ok := ParseAutoPointsRange(c); ok {
			t.Fatalf("ParseAutoPointsRange(%q) should fail", c)
		}
	}
	start, end, ok := ParseAutoPointsRange("10-5")
	if !ok || start != 5 || end != 10 {
		t.Fatalf("reversed range = %d-%d ok=%v", start, end, ok)
	}
}

func TestScoreToChannelStatus(t *testing.T) {
	cases := map[int]string{
		95: "Excellent",
		80: "Good",
		60: "Degraded",
		10: "Offline",
	}
	for score, want := range cases {
		if got := scoreToChannelStatus(score); got != want {
			t.Fatalf("scoreToChannelStatus(%d) = %q, want %q", score, got, want)
		}
	}
}

func TestChannelManager_NotifyTopologyChange(t *testing.T) {
	cm := newTestChannelManager()
	called := make(chan struct{}, 1)
	cm.SetTopologyChangeHandler(func() {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	cm.notifyTopologyChange()
	select {
	case <-called:
	case <-time.After(700 * time.Millisecond):
		t.Fatal("topology change handler not invoked")
	}
}
