package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestTagRegistry_RegisterResolveAndScale(t *testing.T) {
	tr := NewTagRegistry()
	dev := &model.Device{
		ID: "dev1",
		Points: []model.Point{
			{ID: "p1", Name: "temp", Unit: "°C", Scale: 0.1, Offset: 1.0, ScanClass: "fast"},
			{ID: "p2", Name: "humidity", Group: "env", Unit: "%"},
		},
	}
	tr.RegisterFromDevice("ch1", dev)

	if tr.Count() != 2 {
		t.Fatalf("Count = %d, want 2", tr.Count())
	}

	entry, ok := tr.Get("ch1", "dev1", "p1")
	if !ok || entry.EU != "°C" || entry.Scale != 0.1 {
		t.Fatalf("Get p1 = %+v, ok=%v", entry, ok)
	}

	resolved, err := tr.Resolve("temp")
	if err != nil || resolved.PointID != "p1" {
		t.Fatalf("Resolve(temp) = %+v, err=%v", resolved, err)
	}

	resolved, err = tr.Resolve("env.humidity")
	if err != nil || resolved.PointID != "p2" {
		t.Fatalf("Resolve(env.humidity) = %+v, err=%v", resolved, err)
	}

	scaled := tr.ApplyScaling(model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   "p1",
		Value:     float64(100),
	})
	if scaled.Value != float64(11) {
		t.Fatalf("ApplyScaling = %v, want 11", scaled.Value)
	}

	unchanged := tr.ApplyScaling(model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   "unknown",
		Value:     42,
	})
	if unchanged.Value != 42 {
		t.Fatalf("unknown tag should pass through, got %v", unchanged.Value)
	}
}

func TestTagRegistry_UnregisterDevice(t *testing.T) {
	tr := NewTagRegistry()
	dev := &model.Device{
		ID: "dev1",
		Points: []model.Point{
			{ID: "p1", Name: "a"},
			{ID: "p2", Name: "b"},
		},
	}
	tr.RegisterFromDevice("ch1", dev)
	tr.RegisterFromDevice("ch1", &model.Device{ID: "dev2", Points: []model.Point{{ID: "x", Name: "x"}}})

	tr.UnregisterDevice("ch1", "dev1")
	if tr.Count() != 1 {
		t.Fatalf("after unregister Count = %d, want 1", tr.Count())
	}
	if _, ok := tr.Get("ch1", "dev1", "p1"); ok {
		t.Fatal("unregistered point should not be found")
	}
}

func TestTagRegistry_ResolveNotFound(t *testing.T) {
	tr := NewTagRegistry()
	if _, err := tr.Resolve("missing"); err == nil {
		t.Fatal("expected error for missing tag")
	}
}

func TestToFloat64(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{float64(1.5), 1.5, true},
		{float32(2.5), 2.5, true},
		{int(3), 3, true},
		{int64(4), 4, true},
		{uint(5), 5, true},
		{"not-a-number", 0, false},
	}
	for _, tc := range cases {
		got, ok := toFloat64(tc.in)
		if ok != tc.ok || (tc.ok && got != tc.want) {
			t.Fatalf("toFloat64(%v) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
