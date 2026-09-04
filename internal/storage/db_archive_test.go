package storage

import (
	"path/filepath"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	bolt "go.etcd.io/bbolt"
)

func TestExportImportConfigDBArchive(t *testing.T) {
	tempDir := testOutputDir(t)

	sourcePath := filepath.Join(tempDir, "source-config.db")
	sourceDB, err := bolt.Open(sourcePath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open source db: %v", err)
	}

	sourceStore, err := NewConfigStore(sourceDB)
	if err != nil {
		t.Fatalf("failed to create source config store: %v", err)
	}

	err = sourceStore.SaveServerConfig(model.ServerConfig{Port: 9090, LogLevel: "debug"})
	if err != nil {
		t.Fatalf("failed to save server config: %v", err)
	}
	err = sourceStore.SaveUsers([]model.UserConfig{
		{Username: "imported-user", Password: "imported-hash"},
	})
	if err != nil {
		t.Fatalf("failed to save users: %v", err)
	}
	err = sourceStore.SaveChannels([]model.Channel{
		{ID: "ch-1", Name: "Channel 1", Protocol: "modbus-tcp", Enable: true},
	})
	if err != nil {
		t.Fatalf("failed to save channels: %v", err)
	}
	err = sourceStore.SaveDevice(model.Device{ID: "dev-1", Name: "Device 1", Enable: true})
	if err != nil {
		t.Fatalf("failed to save devices: %v", err)
	}
	sourceDB.Close()

	archiveData, filename, err := ExportDBAsTarGz(sourcePath, ConfigDBEntryName, "config")
	if err != nil {
		t.Fatalf("failed to export archive: %v", err)
	}
	if filename == "" {
		t.Fatal("expected archive filename")
	}
	if len(archiveData) == 0 {
		t.Fatal("expected archive data")
	}

	targetStorage, err := NewStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create target storage: %v", err)
	}
	defer targetStorage.Close()

	targetStore, err := NewConfigStore(targetStorage.GetConfigDB())
	if err != nil {
		t.Fatalf("failed to create target config store: %v", err)
	}
	err = targetStore.SaveServerConfig(model.ServerConfig{Port: 8080, LogLevel: "info"})
	if err != nil {
		t.Fatalf("failed to save target server config: %v", err)
	}
	err = targetStore.SaveUsers([]model.UserConfig{
		{Username: "local-user", Password: "local-hash"},
	})
	if err != nil {
		t.Fatalf("failed to save target users: %v", err)
	}
	err = targetStore.SaveChannels([]model.Channel{
		{ID: "old-ch", Name: "Old Channel", Protocol: "modbus-tcp", Enable: true},
	})
	if err != nil {
		t.Fatalf("failed to save target channels: %v", err)
	}

	result, err := targetStorage.ImportConfigDBArchive(archiveData, ImportArchiveOptions{})
	if err != nil {
		t.Fatalf("failed to import archive: %v", err)
	}
	if result.PreservedPort != 8080 {
		t.Fatalf("expected preserved port 8080, got %d", result.PreservedPort)
	}
	if result.ChannelCount != 1 || result.DeviceCount != 1 {
		t.Fatalf("unexpected import counts: channels=%d devices=%d", result.ChannelCount, result.DeviceCount)
	}

	loadedStore, err := NewConfigStore(targetStorage.GetConfigDB())
	if err != nil {
		t.Fatalf("failed to reload target config store: %v", err)
	}

	serverConfig, err := loadedStore.LoadServerConfig()
	if err != nil || serverConfig == nil {
		t.Fatalf("failed to load server config: %v", err)
	}
	if serverConfig.Port != 8080 {
		t.Fatalf("expected preserved port 8080, got %d", serverConfig.Port)
	}
	if serverConfig.LogLevel != "debug" {
		t.Fatalf("expected imported log level debug, got %s", serverConfig.LogLevel)
	}

	users, err := loadedStore.LoadUsers()
	if err != nil {
		t.Fatalf("failed to load users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "local-user" || users[0].Password != "local-hash" {
		t.Fatalf("expected local users to be preserved, got %+v", users)
	}

	channels, err := loadedStore.LoadChannels()
	if err != nil {
		t.Fatalf("failed to load channels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "ch-1" {
		t.Fatalf("expected imported channel ch-1, got %+v", channels)
	}
}

func TestExportRuntimeDBArchive(t *testing.T) {
	tempDir := testOutputDir(t)

	storage, err := NewStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	data, filename, err := storage.ExportRuntimeDBArchive()
	if err != nil {
		storage.Close()
		t.Fatalf("failed to export runtime archive: %v", err)
	}
	storage.Close()
	if filename == "" || len(data) == 0 {
		t.Fatal("expected runtime archive output")
	}

	files, err := extractTarGz(data)
	if err != nil {
		t.Fatalf("failed to extract runtime archive: %v", err)
	}
	if _, err := findArchiveEntry(files, RuntimeDBEntryName); err != nil {
		t.Fatalf("runtime archive missing runtime.db: %v", err)
	}
}
