package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func writeShadowPoints(sc *ShadowCore, deviceID string, points map[string]any) {
	ingress := make([]model.ShadowIngressPoint, 0, len(points))
	for pid, val := range points {
		ingress = append(ingress, model.ShadowIngressPoint{
			PointID: pid,
			Value:   val,
			Quality: "good",
		})
	}
	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  deviceID,
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    ingress,
	})
}

func TestDeviceStorageManager_Interval(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	pipeline.Start()

	sc := NewShadowCore()
	dsm := NewDeviceStorageManager(store, pipeline)
	dsm.SetShadowCore(sc)

	deviceID := "dev1"
	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 5,
	})

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 100})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 100})
	time.Sleep(10 * time.Millisecond)

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 100, "p2": 200})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p2", Value: 200})

	time.Sleep(500 * time.Millisecond)

	history, err := dsm.GetHistory(deviceID, 10)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	t.Logf("History records: %d", len(history))

	if len(history) < 2 {
		t.Errorf("Expected at least 2 records, got %d", len(history))
	}

	lastRec := history[0]
	data := lastRec["data"].(map[string]interface{})
	if data["p1"].(float64) != 100 || data["p2"].(float64) != 200 {
		t.Errorf("Last record data mismatch: %v", data)
	}
}

func TestDeviceStorageManager_Prune(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	pipeline.Start()
	sc := NewShadowCore()
	dsm := NewDeviceStorageManager(store, pipeline)
	dsm.SetShadowCore(sc)

	deviceID := "dev_prune"
	maxRecords := 3
	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: maxRecords,
	})

	for i := 0; i < 5; i++ {
		writeShadowPoints(sc, deviceID, map[string]any{"p1": i})
		dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: i})
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	history, err := dsm.GetHistory(deviceID, 100)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	t.Logf("Prune History records: %d", len(history))

	if len(history) != maxRecords {
		t.Errorf("Expected %d records, got %d", maxRecords, len(history))
	}

	if len(history) == 0 {
		t.Fatal("History is empty")
	}

	lastRec := history[0]
	data := lastRec["data"].(map[string]interface{})
	if val, ok := data["p1"].(float64); !ok || int(val) != 4 {
		t.Errorf("Expected newest value 4, got %v", data["p1"])
	}
}

func TestDeviceStorageManager_SnapshotMerge(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	sc := NewShadowCore()
	dsm := NewDeviceStorageManager(store, pipeline)
	dsm.SetShadowCore(sc)

	deviceID := "dev_merge"
	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 10,
	})

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 1})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 1})
	time.Sleep(10 * time.Millisecond)

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 1, "p2": 2})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p2", Value: 2})
	time.Sleep(10 * time.Millisecond)

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 3, "p2": 2})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 3})
	time.Sleep(200 * time.Millisecond)

	history, _ := dsm.GetHistory(deviceID, 10)

	if len(history) != 3 {
		t.Errorf("Expected 3 records, got %d", len(history))
	}

	last := history[0]["data"].(map[string]interface{})
	if last["p1"].(float64) != 3 || last["p2"].(float64) != 2 {
		t.Errorf("Merge logic failed: %v", last)
	}
}

func TestDeviceStorageManager_StrategySwitch(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	sc := NewShadowCore()
	dsm := NewDeviceStorageManager(store, pipeline)
	dsm.SetShadowCore(sc)

	deviceID := "dev_switch"

	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 10,
	})

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 1})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 1})
	time.Sleep(200 * time.Millisecond)

	history, _ := dsm.GetHistory(deviceID, 10)
	if len(history) != 1 {
		t.Errorf("Expected 1 record in realtime mode, got %d", len(history))
	}

	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "interval",
		Interval:   1,
		MaxRecords: 10,
	})

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 2})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 2})
	time.Sleep(200 * time.Millisecond)

	history, _ = dsm.GetHistory(deviceID, 10)
	if len(history) != 1 {
		t.Errorf("Expected still 1 record after switching to interval (should not save immediately), got %d", len(history))
	}

	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 10,
	})

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 3})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "p1", Value: 3})
	time.Sleep(300 * time.Millisecond)

	history, _ = dsm.GetHistory(deviceID, 10)
	if len(history) != 2 {
		t.Errorf("Expected 2 records after switching back to realtime, got %d", len(history))
	}
}

func TestDeviceStorageManager_Interval_Execution(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	sc := NewShadowCore()
	dsm := NewDeviceStorageManager(store, pipeline)
	dsm.SetShadowCore(sc)
	dsm.intervalUnit = 100 * time.Millisecond

	deviceID := "dev_interval"

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 1})

	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "interval",
		Interval:   1,
		MaxRecords: 10,
	})

	time.Sleep(50 * time.Millisecond)
	history, _ := dsm.GetHistory(deviceID, 10)
	if len(history) != 0 {
		t.Errorf("Expected 0 records before interval, got %d", len(history))
	}

	// Wait past first tick (100ms) but before second (200ms).
	time.Sleep(120 * time.Millisecond)

	history, _ = dsm.GetHistory(deviceID, 10)
	if len(history) != 1 {
		t.Errorf("Expected 1 record after interval, got %d", len(history))
	}

	writeShadowPoints(sc, deviceID, map[string]any{"p1": 2})

	time.Sleep(150 * time.Millisecond)

	history, _ = dsm.GetHistory(deviceID, 10)
	if len(history) < 2 {
		t.Errorf("Expected at least 2 records after second interval, got %d", len(history))
	}

	dsm.RemoveDevice(deviceID)
}
