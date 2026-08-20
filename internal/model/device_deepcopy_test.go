package model

import (
	"testing"
)

// TestDeviceDeepCopy_Isolation verifies that mutating a DeepCopy result does not
// corrupt the original device (Points slice, Config map, pointer fields).
// Regression for the MCP update_point bug where GetDevice's shallow copy shared
// the Points backing array, so scan_class changes leaked into the stored device
// and skipped the scan-engine restart.
func TestDeviceDeepCopy_Isolation(t *testing.T) {
	orig := Device{
		ID:       "dev-1",
		Name:     "SimulationServer",
		Enable:   true,
		Interval: Duration(10 * 1e9),
		Config: map[string]any{
			"ip":            "192.168.3.104",
			"security_mode": "None",
		},
		DegradeOnFailure: boolPtr(true),
		Points: []Point{
			{ID: "pt-1", Name: "Counter", Address: "ns=3;i=1001", DataType: "int32", ScanClass: "fast"},
			{ID: "pt-2", Name: "Random", Address: "ns=3;i=1002", DataType: "float64", ScanClass: "normal"},
		},
	}

	cp := orig.DeepCopy()

	// Mutate every aliasable field on the copy.
	cp.Config["ip"] = "10.0.0.1"
	cp.Config["new_key"] = true
	cp.Points[0].ScanClass = "normal"
	cp.Points[0].Address = "ns=9;i=9999"
	cp.Points = append(cp.Points, Point{ID: "pt-3"})
	*cp.DegradeOnFailure = false

	// Original must be untouched.
	if orig.Config["ip"] != "192.168.3.104" {
		t.Fatalf("Config map was aliased: ip=%v", orig.Config["ip"])
	}
	if _, ok := orig.Config["new_key"]; ok {
		t.Fatal("Config map was aliased: new_key leaked into original")
	}
	if orig.Points[0].ScanClass != "fast" {
		t.Fatalf("Points backing array was aliased: scan_class=%q", orig.Points[0].ScanClass)
	}
	if orig.Points[0].Address != "ns=3;i=1001" {
		t.Fatalf("Points backing array was aliased: address=%q", orig.Points[0].Address)
	}
	if len(orig.Points) != 2 {
		t.Fatalf("Points slice was aliased: len=%d", len(orig.Points))
	}
	if orig.DegradeOnFailure == nil || !*orig.DegradeOnFailure {
		t.Fatal("DegradeOnFailure pointer was aliased")
	}
}

// TestPointClone_ThresholdIsolation verifies the Threshold pointer is duplicated.
func TestPointClone_ThresholdIsolation(t *testing.T) {
	orig := Point{
		ID:        "pt-1",
		Threshold: &ThresholdConfig{High: 100, Low: 10},
	}
	cp := orig.Clone()
	cp.Threshold.High = 999

	if orig.Threshold.High != 100 {
		t.Fatalf("Threshold pointer was aliased: high=%v", orig.Threshold.High)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
