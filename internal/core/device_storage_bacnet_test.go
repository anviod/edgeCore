package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestDeviceStorageManager_SkipsNilShadowValues(t *testing.T) {
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

	deviceID := "bacnet-dev"
	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 10,
	})

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  deviceID,
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "Temperature.Indoor", Value: nil, Quality: "good"},
		},
	})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "Temperature.Indoor", Value: nil})
	time.Sleep(200 * time.Millisecond)

	history, err := dsm.GetHistory(deviceID, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected no history when shadow values are nil, got %d records", len(history))
	}

	sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  deviceID,
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "Temperature.Indoor", Value: 23.5, Quality: "good"},
		},
	})
	dsm.handleValue(model.Value{DeviceID: deviceID, PointID: "Temperature.Indoor", Value: 23.5})
	time.Sleep(200 * time.Millisecond)

	history, err = dsm.GetHistory(deviceID, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history record after valid value, got %d", len(history))
	}
	data := history[0]["data"].(map[string]interface{})
	if data["Temperature.Indoor"].(float64) != 23.5 {
		t.Fatalf("unexpected stored value: %v", data["Temperature.Indoor"])
	}
}
