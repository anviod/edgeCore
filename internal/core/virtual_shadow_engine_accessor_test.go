package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestVirtualShadowEngine_GetAllVirtualDevices(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	if err := vse.CreateVirtualDevice("virt-1", "ch1", map[string]string{"out": "1 + 1"}); err != nil {
		t.Fatalf("CreateVirtualDevice virt-1: %v", err)
	}
	if err := vse.CreateVirtualDevice("virt-2", "ch1", map[string]string{"out": "2 + 2"}); err != nil {
		t.Fatalf("CreateVirtualDevice virt-2: %v", err)
	}

	all := vse.GetAllVirtualDevices()
	if len(all) != 2 {
		t.Fatalf("GetAllVirtualDevices = %d, want 2", len(all))
	}
}

func TestIsNumber(t *testing.T) {
	cases := map[string]bool{
		"1.2":           true,
		"ch1.dev1.temp": false,
		"abc":           false,
		"1.2.3":         false,
		"":              false,
	}
	for input, want := range cases {
		if got := isNumber(input); got != want {
			t.Fatalf("isNumber(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestVirtualShadowEngine_UpdateFormulaAccessor(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	_, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID: "dev-1", ChannelID: "ch1", Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 10, Quality: "Good"},
			{PointID: "hum", Value: 50, Quality: "Good"},
		},
	})
	if err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	if err := vse.CreateVirtualDevice("virt-formula", "ch1", map[string]string{
		"sum": "ch1.dev1.temp + ch1.dev1.hum",
	}); err != nil {
		t.Fatalf("CreateVirtualDevice: %v", err)
	}

	if err := vse.UpdateFormula("virt-formula", "sum", "ch1.dev1.temp * 2"); err != nil {
		t.Fatalf("UpdateFormula: %v", err)
	}
	if err := vse.UpdateFormula("missing", "sum", "1"); err == nil {
		t.Fatal("expected error updating missing device")
	}

	time.Sleep(30 * time.Millisecond)
	vd, err := vse.GetVirtualDevice("virt-formula")
	if err != nil {
		t.Fatalf("GetVirtualDevice: %v", err)
	}
	if vd.FormulaPoints["sum"] != "ch1.dev1.temp * 2" {
		t.Fatalf("formula not updated: %+v", vd.FormulaPoints)
	}
}
