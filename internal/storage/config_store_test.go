package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	bolt "go.etcd.io/bbolt"
)

func TestConfigStoreExportImport(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "config.db")
	db, err := bolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	configStore, err := NewConfigStore(db)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	testChannels := []model.Channel{
		{
			ID:       "test-channel",
			Name:     "Test Channel",
			Protocol: "modbus-tcp",
			Enable:   true,
			Config:   map[string]any{"url": "tcp://127.0.0.1:502"},
			Devices: []model.Device{
				{
					ID:       "test-device",
					Name:     "Test Device",
					Enable:   true,
					Interval: model.Duration(time.Duration(10 * time.Second)),
					Config:   map[string]any{"slave_id": 1},
					Points: []model.Point{
						{
							ID:       "test-point",
							Name:     "Test Point",
							Address:  "0",
							DataType: "int16",
						},
					},
				},
			},
		},
	}

	err = configStore.SaveChannels(testChannels)
	if err != nil {
		t.Fatalf("Failed to save channels: %v", err)
	}

	testNorthbound := model.NorthboundConfig{
		HTTP: []model.HTTPConfig{
			{
				ID:     "test-northbound",
				Name:   "Test Northbound",
				Enable: true,
				URL:    "http://localhost:8080/api",
			},
		},
	}

	err = configStore.SaveNorthbound(testNorthbound)
	if err != nil {
		t.Fatalf("Failed to save northbound: %v", err)
	}

	testServer := model.ServerConfig{
		Port:     8080,
		LogLevel: "info",
	}

	err = configStore.SaveServerConfig(testServer)
	if err != nil {
		t.Fatalf("Failed to save server: %v", err)
	}

	exportData, err := configStore.ExportAllConfig()
	if err != nil {
		t.Fatalf("Failed to export config: %v", err)
	}

	if len(exportData.Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(exportData.Channels))
	}

	if exportData.Channels[0].ID != "test-channel" {
		t.Errorf("Expected channel ID 'test-channel', got '%s'", exportData.Channels[0].ID)
	}

	if len(exportData.Channels[0].Devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(exportData.Channels[0].Devices))
	}

	if exportData.Channels[0].Devices[0].Name != "Test Device" {
		t.Errorf("Expected device name 'Test Device', got '%s'", exportData.Channels[0].Devices[0].Name)
	}

	if len(exportData.Channels[0].Devices[0].Points) != 1 {
		t.Errorf("Expected 1 point, got %d", len(exportData.Channels[0].Devices[0].Points))
	}

	if len(exportData.Northbound.HTTP) != 1 {
		t.Errorf("Expected 1 HTTP northbound, got %d", len(exportData.Northbound.HTTP))
	}

	if exportData.Northbound.HTTP[0].ID != "test-northbound" {
		t.Errorf("Expected northbound ID 'test-northbound', got '%s'", exportData.Northbound.HTTP[0].ID)
	}

	if exportData.Server.Port != 8080 {
		t.Errorf("Expected server port 8080, got %d", exportData.Server.Port)
	}

	newDBPath := filepath.Join(tempDir, "new_config.db")
	newDB, err := bolt.Open(newDBPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open new database: %v", err)
	}
	defer newDB.Close()

	newConfigStore, err := NewConfigStore(newDB)
	if err != nil {
		t.Fatalf("Failed to create new config store: %v", err)
	}

	err = newConfigStore.ImportConfig(exportData)
	if err != nil {
		t.Fatalf("Failed to import config: %v", err)
	}

	reExportData, err := newConfigStore.ExportAllConfig()
	if err != nil {
		t.Fatalf("Failed to re-export config: %v", err)
	}

	if len(reExportData.Channels) != 1 {
		t.Errorf("After import, expected 1 channel, got %d", len(reExportData.Channels))
	}

	if reExportData.Channels[0].ID != "test-channel" {
		t.Errorf("After import, expected channel ID 'test-channel', got '%s'", reExportData.Channels[0].ID)
	}

	if len(reExportData.Northbound.HTTP) != 1 {
		t.Errorf("After import, expected 1 HTTP northbound, got %d", len(reExportData.Northbound.HTTP))
	}

	if reExportData.Server.Port != 8080 {
		t.Errorf("After import, expected server port 8080, got %d", reExportData.Server.Port)
	}

	t.Log("Config store export and import test passed!")
}

func TestHasUsersAndSystemInitialized(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "config.db")
	db, err := bolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	configStore, err := NewConfigStore(db)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	t.Run("empty db has no users", func(t *testing.T) {
		hasUsers, err := configStore.HasUsers()
		if err != nil {
			t.Fatalf("HasUsers failed: %v", err)
		}
		if hasUsers {
			t.Error("Expected no users in empty DB")
		}
	})

	t.Run("empty db is not initialized", func(t *testing.T) {
		isInit, err := configStore.IsSystemInitialized()
		if err != nil {
			t.Fatalf("IsSystemInitialized failed: %v", err)
		}
		if isInit {
			t.Error("Expected empty DB to not be initialized")
		}
	})

	t.Run("after saving users, has users", func(t *testing.T) {
		users := []model.UserConfig{
			{
				Username: "admin",
				Password: "hashedpassword123",
				Role:     "admin",
			},
		}
		err := configStore.SaveUsers(users)
		if err != nil {
			t.Fatalf("SaveUsers failed: %v", err)
		}

		hasUsers, err := configStore.HasUsers()
		if err != nil {
			t.Fatalf("HasUsers failed: %v", err)
		}
		if !hasUsers {
			t.Error("Expected to have users after saving")
		}
	})

	t.Run("with users, system is initialized", func(t *testing.T) {
		isInit, err := configStore.IsSystemInitialized()
		if err != nil {
			t.Fatalf("IsSystemInitialized failed: %v", err)
		}
		if !isInit {
			t.Error("Expected system to be initialized with users")
		}
	})

	t.Run("empty users array means not initialized", func(t *testing.T) {
		err := configStore.SaveUsers([]model.UserConfig{})
		if err != nil {
			t.Fatalf("SaveUsers failed: %v", err)
		}

		hasUsers, err := configStore.HasUsers()
		if err != nil {
			t.Fatalf("HasUsers failed: %v", err)
		}
		if hasUsers {
			t.Error("Expected no users with empty array")
		}

		isInit, err := configStore.IsSystemInitialized()
		if err != nil {
			t.Fatalf("IsSystemInitialized failed: %v", err)
		}
		if isInit {
			t.Error("Expected system not initialized with empty users")
		}
	})

	t.Log("HasUsers and IsSystemInitialized test passed!")
}

func TestSaveAllConfigRejectsEmptyDeviceID(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "config.db")
	db, err := bolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	configStore, err := NewConfigStore(db)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	channels := []model.Channel{
		{
			ID:       "ch-1",
			Name:     "Channel 1",
			Protocol: "modbus-tcp",
			Devices: []model.Device{
				{Name: "", ID: "", Points: []model.Point{{ID: "p1", Name: "p1"}}},
			},
		},
	}

	err = configStore.SaveAllConfig(
		model.ServerConfig{Port: 8080, LogLevel: "info"},
		channels,
		channels[0].Devices,
		model.NorthboundConfig{},
		nil,
		model.SystemConfig{},
		nil,
	)
	if err == nil {
		t.Fatal("expected error when saving device with empty ID")
	}
	if !strings.Contains(err.Error(), "device ID or name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHasConfigData_NorthboundBucket(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "config.db")
	db, err := bolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	configStore, err := NewConfigStore(db)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	hasData, err := configStore.HasConfigData()
	if err != nil {
		t.Fatalf("HasConfigData failed: %v", err)
	}
	if hasData {
		t.Fatal("expected no config data in empty DB")
	}

	err = configStore.SaveNorthbound(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{
			{ID: "nb-1", Name: "MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"},
		},
	})
	if err != nil {
		t.Fatalf("SaveNorthbound failed: %v", err)
	}

	hasData, err = configStore.HasConfigData()
	if err != nil {
		t.Fatalf("HasConfigData failed: %v", err)
	}
	if !hasData {
		t.Fatal("expected HasConfigData true when northbound bucket has data")
	}
}

func TestHasConfigData_EdgeRulesBucket(t *testing.T) {
	tempDir := testOutputDir(t)

	dbPath := filepath.Join(tempDir, "config.db")
	db, err := bolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	configStore, err := NewConfigStore(db)
	if err != nil {
		t.Fatalf("Failed to create config store: %v", err)
	}

	err = configStore.SaveEdgeRules([]model.EdgeRule{
		{ID: "rule-1", Name: "Rule 1", Type: "threshold", Enable: true, Condition: "t1 > 1"},
	})
	if err != nil {
		t.Fatalf("SaveEdgeRules failed: %v", err)
	}

	hasData, err := configStore.HasConfigData()
	if err != nil {
		t.Fatalf("HasConfigData failed: %v", err)
	}
	if !hasData {
		t.Fatal("expected HasConfigData true when edge rules bucket has data")
	}
}
