package core

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestVirtualShadowEngine_CreateVirtualDevice(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"total": "ch1.device1.temp + ch1.device2.temp",
	}

	err = vse.CreateVirtualDevice("virtual-1", "ch1", formulaPoints)
	if err != nil {
		t.Fatalf("CreateVirtualDevice failed: %v", err)
	}

	device, err := vse.GetVirtualDevice("virtual-1")
	if err != nil {
		t.Fatalf("GetVirtualDevice failed: %v", err)
	}

	if device.VirtualDeviceID != "virtual-1" {
		t.Errorf("Expected virtual-1, got %s", device.VirtualDeviceID)
	}

	if len(device.FormulaPoints) != 1 {
		t.Errorf("Expected 1 formula point, got %d", len(device.FormulaPoints))
	}

	if len(device.Dependencies) < 2 {
		t.Errorf("Expected at least 2 dependencies, got %d: %v", len(device.Dependencies), device.Dependencies)
	}
}

func TestVirtualShadowEngine_DependencyExtraction(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"sum":   "ch1.dev1.temp + ch1.dev2.humidity",
		"avg":   "(ch1.dev1.temp + ch1.dev2.temp) / 2",
		"mixed": "ch1.dev1.pressure * 2 + ch1.dev2.flow",
	}

	err = vse.CreateVirtualDevice("virtual-2", "ch1", formulaPoints)
	if err != nil {
		t.Fatalf("CreateVirtualDevice failed: %v", err)
	}

	device, _ := vse.GetVirtualDevice("virtual-2")

	expectedDeps := []string{
		"ch1.dev1.temp",
		"ch1.dev2.humidity",
		"ch1.dev2.temp",
		"ch1.dev1.pressure",
		"ch1.dev2.flow",
	}

	if len(device.Dependencies) < 4 {
		t.Errorf("Expected at least 4 dependencies, got %d: %v", len(device.Dependencies), device.Dependencies)
	}

	for _, expected := range expectedDeps {
		found := false
		for _, dep := range device.Dependencies {
			if dep == expected {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Warning: expected dependency %s not found in %v", expected, device.Dependencies)
		}
	}
}

func TestVirtualShadowEngine_MapModeHyphenatedDevice(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	ref := "mzp8f02lusxvk0da.modbus-slave-1.hr_0"
	formulaPoints := map[string]string{
		"hr_0": ref,
	}

	if err := vse.CreateVirtualDevice("v1", "mzp8f02lusxvk0da", formulaPoints); err != nil {
		t.Fatalf("CreateVirtualDevice failed: %v", err)
	}

	device, err := vse.GetVirtualDevice("v1")
	if err != nil {
		t.Fatalf("GetVirtualDevice failed: %v", err)
	}
	if len(device.Dependencies) != 1 || device.Dependencies[0] != ref {
		t.Fatalf("expected dependency %q, got %v", ref, device.Dependencies)
	}

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "modbus-slave-1",
		ChannelID: "mzp8f02lusxvk0da",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "hr_0", Value: 42.0, Quality: "good"},
		},
	})

	vse.RecomputeDevice("v1")

	device, _ = vse.GetVirtualDevice("v1")
	pt, ok := device.Points["hr_0"]
	if !ok {
		t.Fatal("expected mapped point hr_0")
	}
	if pt.Value != float64(42) {
		t.Errorf("expected value 42, got %v", pt.Value)
	}
}

func TestVirtualShadowEngine_MapModeOpcUaNodeID(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	channelID := "u3rellnz1jgz0ljg"
	deviceID := "3n39qx886h1qaxst"
	pointID := "ns=3;i=1001"
	ref := channelID + "." + deviceID + "." + pointID
	formulaPoints := map[string]string{
		"node_1001": ref,
	}

	if err := vse.CreateVirtualDevice("v1", channelID, formulaPoints); err != nil {
		t.Fatalf("CreateVirtualDevice failed: %v", err)
	}

	device, err := vse.GetVirtualDevice("v1")
	if err != nil {
		t.Fatalf("GetVirtualDevice failed: %v", err)
	}
	if len(device.Dependencies) != 1 || device.Dependencies[0] != ref {
		t.Fatalf("expected dependency %q, got %v", ref, device.Dependencies)
	}

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  deviceID,
		ChannelID: channelID,
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: pointID, Value: 123.45, Quality: "good"},
		},
	})

	vse.RecomputeDevice("v1")

	device, _ = vse.GetVirtualDevice("v1")
	pt, ok := device.Points["node_1001"]
	if !ok {
		t.Fatal("expected mapped point node_1001")
	}
	if pt.Value != 123.45 {
		t.Errorf("expected value 123.45, got %v", pt.Value)
	}

	graph := vse.GetDependencyGraph()
	if len(graph[ref]) == 0 {
		t.Fatalf("expected dependency graph entry for %q", ref)
	}
}

func TestVirtualShadowEngine_MapModeOpcUaStringNodeWithDots(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	channelID := "ch-opc"
	deviceID := "dev-opc"
	pointID := "ns=2;s=Some.Node.Name"
	ref := channelID + "." + deviceID + "." + pointID

	if err := vse.CreateVirtualDevice("v2", channelID, map[string]string{"tag": ref}); err != nil {
		t.Fatalf("CreateVirtualDevice failed: %v", err)
	}

	device, err := vse.GetVirtualDevice("v2")
	if err != nil {
		t.Fatalf("GetVirtualDevice failed: %v", err)
	}
	if len(device.Dependencies) != 1 || device.Dependencies[0] != ref {
		t.Fatalf("expected dependency %q, got %v", ref, device.Dependencies)
	}

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  deviceID,
		ChannelID: channelID,
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: pointID, Value: true, Quality: "good"},
		},
	})
	vse.RecomputeDevice("v2")

	device, _ = vse.GetVirtualDevice("v2")
	pt, ok := device.Points["tag"]
	if !ok {
		t.Fatal("expected mapped point tag")
	}
	if pt.Value != true {
		t.Errorf("expected value true, got %v", pt.Value)
	}
}

func TestParsePointRef(t *testing.T) {
	tests := []struct {
		ref     string
		wantCh  string
		wantDev string
		wantPt  string
		wantOK  bool
	}{
		{"mzp8f02lusxvk0da.modbus-slave-1.hr_0", "mzp8f02lusxvk0da", "modbus-slave-1", "hr_0", true},
		{"u3rellnz1jgz0ljg.3n39qx886h1qaxst.ns=3;i=1001", "u3rellnz1jgz0ljg", "3n39qx886h1qaxst", "ns=3;i=1001", true},
		{"ch.dev.ns=2;s=Some.Node", "ch", "dev", "ns=2;s=Some.Node", true},
		{"invalid", "", "", "", false},
		{"only.two", "", "", "", false},
	}
	for _, tt := range tests {
		ch, dev, pt, ok := parsePointRef(tt.ref)
		if ok != tt.wantOK {
			t.Fatalf("parsePointRef(%q) ok=%v, want %v", tt.ref, ok, tt.wantOK)
		}
		if !tt.wantOK {
			continue
		}
		if ch != tt.wantCh || dev != tt.wantDev || pt != tt.wantPt {
			t.Fatalf("parsePointRef(%q) = (%q,%q,%q), want (%q,%q,%q)", tt.ref, ch, dev, pt, tt.wantCh, tt.wantDev, tt.wantPt)
		}
	}
}

func TestVirtualShadowEngine_DeleteVirtualDevice(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"total": "device1.temp + device2.temp",
	}

	vse.CreateVirtualDevice("virtual-1", "ch1", formulaPoints)

	err = vse.DeleteVirtualDevice("virtual-1")
	if err != nil {
		t.Fatalf("DeleteVirtualDevice failed: %v", err)
	}

	_, err = vse.GetVirtualDevice("virtual-1")
	if err == nil {
		t.Errorf("Expected error after deletion, got nil")
	}
}

func TestVirtualShadowEngine_UpdateFormula(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"total": "device1.temp + device2.temp",
	}

	vse.CreateVirtualDevice("virtual-1", "ch1", formulaPoints)

	err = vse.UpdateFormula("virtual-1", "total", "device1.temp * 2")
	if err != nil {
		t.Fatalf("UpdateFormula failed: %v", err)
	}

	device, _ := vse.GetVirtualDevice("virtual-1")

	if device.FormulaPoints["total"] != "device1.temp * 2" {
		t.Errorf("Formula not updated correctly")
	}
}

func TestVirtualShadowEngine_GetDependencyGraph(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"total": "ch1.device1.temp + ch1.device2.temp",
	}

	vse.CreateVirtualDevice("virtual-1", "ch1", formulaPoints)

	graph := vse.GetDependencyGraph()

	if len(graph) == 0 {
		t.Errorf("Expected non-empty dependency graph")
	}
}

func TestVirtualShadowEngine_GetMetrics(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	formulaPoints := map[string]string{
		"total": "device1.temp + device2.temp",
	}

	vse.CreateVirtualDevice("virtual-1", "ch1", formulaPoints)

	metrics := vse.GetMetrics()

	if metrics["virtual_device_count"].(int) != 1 {
		t.Errorf("Expected 1 virtual device, got %d", metrics["virtual_device_count"])
	}

	if metrics["total_formulas"].(int) != 1 {
		t.Errorf("Expected 1 formula, got %d", metrics["total_formulas"])
	}
}

func TestVirtualShadowEngine_FormulaEvaluation(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 25.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	msg2 := model.ShadowIngressMessage{
		MessageID: "test-msg-2",
		DeviceID:  "dev2",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 30.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg2)

	formulaPoints := map[string]string{
		"sum": "ch1.dev1.temp + ch1.dev2.temp",
	}

	vse.CreateVirtualDevice("virtual-sum", "ch1", formulaPoints)

	time.Sleep(100 * time.Millisecond)

	device, err := vse.GetVirtualDevice("virtual-sum")
	if err != nil {
		t.Fatalf("GetVirtualDevice failed: %v", err)
	}

	t.Logf("Virtual device points: %+v", device.Points)

	sumPt, ok := device.Points["sum"]
	if !ok {
		t.Fatal("expected sum point")
	}
	if sumPt.Value != float64(55) {
		t.Errorf("expected sum 55, got %v", sumPt.Value)
	}
}

func TestVirtualShadowEngine_PipelineFanOut(t *testing.T) {
	sc := NewShadowCore()
	pipeline := NewDataPipeline(20)
	pipeline.Start()

	var mu sync.Mutex
	var received []model.Value
	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		received = append(received, v)
		mu.Unlock()
	})

	NewShadowBridge(pipeline).Attach(sc)
	vse := NewVirtualShadowEngine(sc)

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    []model.ShadowIngressPoint{{PointID: "temp", Value: 25.0, Quality: "good"}},
	})
	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev2",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    []model.ShadowIngressPoint{{PointID: "temp", Value: 30.0, Quality: "good"}},
	})

	err := vse.CreateVirtualDevice("virtual-sum", "ch1", map[string]string{
		"sum": "ch1.dev1.temp + ch1.dev2.temp",
	})
	if err != nil {
		t.Fatalf("CreateVirtualDevice: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var virtualSum *model.Value
	for time.Now().Before(deadline) {
		mu.Lock()
		for i := range received {
			if received[i].DeviceID == "virtual-sum" && received[i].PointID == "sum" {
				copy := received[i]
				virtualSum = &copy
				break
			}
		}
		mu.Unlock()
		if virtualSum != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if virtualSum == nil {
		t.Fatalf("pipeline did not receive virtual point, got %d values", len(received))
	}
	if virtualSum.ChannelID != "ch1" {
		t.Errorf("expected channel ch1, got %s", virtualSum.ChannelID)
	}
	if virtualSum.Value != float64(55) {
		t.Errorf("expected sum 55, got %v", virtualSum.Value)
	}

	vd, err := sc.GetVirtualShadowDevice("virtual-sum")
	if err != nil {
		t.Fatalf("GetVirtualShadowDevice: %v", err)
	}
	if len(vd.Points) == 0 {
		t.Error("expected virtual shadow in ShadowCore")
	}
}

func TestVirtualShadowEngine_BadSourceMarksVirtualBad(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 25.0, Quality: "Good"},
		},
	})
	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev2",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 30.0, Quality: "Bad"},
		},
	})

	if err := vse.CreateVirtualDevice("virtual-sum", "ch1", map[string]string{
		"sum": "ch1.dev1.temp + ch1.dev2.temp",
	}); err != nil {
		t.Fatalf("CreateVirtualDevice: %v", err)
	}

	vse.RecomputeDevice("virtual-sum")

	device, err := vse.GetVirtualDevice("virtual-sum")
	if err != nil {
		t.Fatalf("GetVirtualDevice: %v", err)
	}
	pt, ok := device.Points["sum"]
	if !ok {
		t.Fatal("expected sum point")
	}
	if !strings.EqualFold(pt.Quality, "bad") {
		t.Fatalf("expected bad quality when source is bad, got %q", pt.Quality)
	}
}

func TestVirtualShadowEngine_MapModeBadSource(t *testing.T) {
	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)

	ref := "ch1.dev1.temp"
	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 42.0, Quality: "Bad"},
		},
	})

	if err := vse.CreateVirtualDevice("v1", "ch1", map[string]string{"mapped": ref}); err != nil {
		t.Fatalf("CreateVirtualDevice: %v", err)
	}

	vse.RecomputeDevice("v1")

	device, err := vse.GetVirtualDevice("v1")
	if err != nil {
		t.Fatalf("GetVirtualDevice: %v", err)
	}
	pt, ok := device.Points["mapped"]
	if !ok {
		t.Fatal("expected mapped point")
	}
	if !strings.EqualFold(pt.Quality, "bad") {
		t.Fatalf("expected bad quality, got %q", pt.Quality)
	}
	if pt.Value != float64(42) {
		t.Fatalf("expected preserved value 42, got %v", pt.Value)
	}
}

func TestStress_VirtualShadow_10kRefresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping virtual shadow 10k refresh stress in short mode")
	}

	const (
		numDevices   = 100
		pointsPerDev = 100
		newValue     = 42.0
	)
	totalTags := numDevices * pointsPerDev

	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	vse := NewVirtualShadowEngine(sc)

	for i := 0; i < numDevices; i++ {
		devID := fmt.Sprintf("dev%03d", i)
		points := make([]model.ShadowIngressPoint, pointsPerDev)
		formulas := make(map[string]string, pointsPerDev)
		for j := 0; j < pointsPerDev; j++ {
			pointID := fmt.Sprintf("p%03d", j)
			points[j] = model.ShadowIngressPoint{PointID: pointID, Value: 0.0, Quality: "Good"}
			formulas[pointID] = fmt.Sprintf("ch1.%s.%s", devID, pointID)
		}
		if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
			DeviceID:  devID,
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points:    points,
		}); err != nil {
			t.Fatalf("seed shadow %s: %v", devID, err)
		}
		vdID := fmt.Sprintf("virt-%03d", i)
		if err := vse.CreateVirtualDevice(vdID, "ch1", formulas); err != nil {
			t.Fatalf("CreateVirtualDevice %s: %v", vdID, err)
		}
	}

	msgs := make([]model.ShadowIngressMessage, numDevices)
	for i := 0; i < numDevices; i++ {
		devID := fmt.Sprintf("dev%03d", i)
		points := make([]model.ShadowIngressPoint, pointsPerDev)
		for j := 0; j < pointsPerDev; j++ {
			points[j] = model.ShadowIngressPoint{
				PointID: fmt.Sprintf("p%03d", j),
				Value:   newValue,
				Quality: "Good",
			}
		}
		msgs[i] = model.ShadowIngressMessage{
			DeviceID:  devID,
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points:    points,
		}
	}

	start := time.Now()
	if err := sc.ApplyShadowWrites(msgs); err != nil {
		t.Fatalf("ApplyShadowWrites: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ready := 0
		for i := 0; i < numDevices; i++ {
			vdID := fmt.Sprintf("virt-%03d", i)
			vd, err := vse.GetVirtualDevice(vdID)
			if err != nil {
				break
			}
			if len(vd.Points) != pointsPerDev {
				break
			}
			ok := true
			for _, pt := range vd.Points {
				if pt.Value != newValue || !strings.EqualFold(pt.Quality, "good") {
					ok = false
					break
				}
			}
			if ok {
				ready++
			}
		}
		if ready == numDevices {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	elapsed := time.Since(start)
	ready := 0
	for i := 0; i < numDevices; i++ {
		vdID := fmt.Sprintf("virt-%03d", i)
		vd, err := vse.GetVirtualDevice(vdID)
		if err != nil {
			t.Fatalf("GetVirtualDevice %s: %v", vdID, err)
		}
		if len(vd.Points) != pointsPerDev {
			t.Fatalf("virtual %s: expected %d points, got %d", vdID, pointsPerDev, len(vd.Points))
		}
		for pid, pt := range vd.Points {
			if pt.Value != newValue {
				t.Fatalf("virtual %s.%s: expected %v, got %v", vdID, pid, newValue, pt.Value)
			}
		}
		ready++
	}

	tagsPerSec := float64(totalTags) / elapsed.Seconds()
	t.Logf("virtual shadow 10k refresh: %v (%.0f source tags/sec, %d virtual devices × %d points)",
		elapsed, tagsPerSec, numDevices, pointsPerDev)

	if ready != numDevices {
		t.Fatalf("only %d/%d virtual devices refreshed", ready, numDevices)
	}
	if elapsed > 5*time.Second {
		t.Errorf("virtual shadow 10k refresh took %v, want <= 5s", elapsed)
	}
}
