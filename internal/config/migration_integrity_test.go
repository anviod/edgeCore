package config

import (
	"path/filepath"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestDBDataIntegrity_CRUD(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "test.db")
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	configStore, err := storage.NewConfigStore(store.GetConfigDB())
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}

	t.Run("save and load channels", func(t *testing.T) {
		channels := []model.Channel{
			{
				ID:       "ch-crud-1",
				Name:     "CRUD Channel 1",
				Protocol: "modbus-tcp",
				Enable:   true,
				Devices: []model.Device{
					{
						ID:     "dev-crud-1",
						Name:   "CRUD Device 1",
						Enable: true,
						Points: []model.Point{
							{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"},
						},
					},
				},
			},
		}

		err := configStore.SaveChannels(channels)
		if err != nil {
			t.Fatalf("SaveChannels failed: %v", err)
		}

		loaded, err := configStore.LoadChannels()
		if err != nil {
			t.Fatalf("LoadChannels failed: %v", err)
		}

		if len(loaded) != 1 {
			t.Errorf("Expected 1 channel, got %d", len(loaded))
		}

		if loaded[0].ID != "ch-crud-1" {
			t.Errorf("Expected ch-crud-1, got %s", loaded[0].ID)
		}

		if len(loaded[0].Devices) != 1 {
			t.Errorf("Expected 1 device, got %d", len(loaded[0].Devices))
		}
	})

	t.Run("save and load devices individually", func(t *testing.T) {
		device := model.Device{
			ID:     "dev-individual",
			Name:   "Individual Device",
			Enable: true,
			Points: []model.Point{
				{ID: "pt-a", Name: "Point A", Address: "100", DataType: "float32"},
				{ID: "pt-b", Name: "Point B", Address: "101", DataType: "int32"},
			},
		}

		err := configStore.SaveDevice(device)
		if err != nil {
			t.Fatalf("SaveDevice failed: %v", err)
		}

		loaded, err := configStore.LoadDevice("dev-individual")
		if err != nil {
			t.Fatalf("LoadDevice failed: %v", err)
		}

		if loaded == nil {
			t.Fatal("Loaded device is nil")
		}

		if loaded.Name != "Individual Device" {
			t.Errorf("Expected 'Individual Device', got %s", loaded.Name)
		}

		if len(loaded.Points) != 2 {
			t.Errorf("Expected 2 points, got %d", len(loaded.Points))
		}

		allDevices, err := configStore.LoadAllDevices()
		if err != nil {
			t.Fatalf("LoadAllDevices failed: %v", err)
		}

		if len(allDevices) != 1 {
			t.Errorf("Expected 1 device in map, got %d", len(allDevices))
		}
	})

	t.Run("update device", func(t *testing.T) {
		device, err := configStore.LoadDevice("dev-individual")
		if err != nil {
			t.Fatalf("LoadDevice failed: %v", err)
		}

		device.Name = "Updated Device Name"
		err = configStore.SaveDevice(*device)
		if err != nil {
			t.Fatalf("SaveDevice (update) failed: %v", err)
		}

		updated, err := configStore.LoadDevice("dev-individual")
		if err != nil {
			t.Fatalf("LoadDevice failed: %v", err)
		}

		if updated.Name != "Updated Device Name" {
			t.Errorf("Expected 'Updated Device Name', got %s", updated.Name)
		}
	})

	t.Run("delete device", func(t *testing.T) {
		err := configStore.DeleteDevice("dev-individual")
		if err != nil {
			t.Fatalf("DeleteDevice failed: %v", err)
		}

		allDevices, err := configStore.LoadAllDevices()
		if err != nil {
			t.Fatalf("LoadAllDevices failed: %v", err)
		}

		if _, ok := allDevices["dev-individual"]; ok {
			t.Error("Device should have been deleted")
		}
	})
}
