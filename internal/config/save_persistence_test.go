package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

// runWithTimeout 执行 fn，若超过 d 仍未结束则判定为死锁并使测试失败。
// 该守卫用于捕获 ConfigManager.SaveConfig 的 saveMu 重入死锁回归。
func runWithTimeout(t *testing.T, d time.Duration, name string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s returned error: %v", name, err)
		}
	case <-time.After(d):
		t.Fatalf("%s did not return within %s (deadlock regression)", name, d)
	}
}

func newDBConfigManager(t *testing.T) (*ConfigManager, func()) {
	t.Helper()
	tempDir := testOutputDir(t)
	dbPath := filepath.Join(tempDir, "data")
	store, err := storage.NewStorage(dbPath)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	confDir := filepath.Join(tempDir, "conf")
	cm, err := NewConfigManagerWithDB(confDir, store.GetConfigDB())
	if err != nil {
		store.Close()
		t.Fatalf("NewConfigManagerWithDB failed: %v", err)
	}
	cleanup := func() {
		store.Close()
	}
	return cm, cleanup
}

// TestSaveConfigNoDeadlock 验证 ConfigManager.SaveConfig 在数据库模式下不会因
// saveMu 重入而死锁（修复前会永久阻塞，导致设备/点位无法持久化）。
func TestSaveConfigNoDeadlock(t *testing.T) {
	cm, cleanup := newDBConfigManager(t)
	defer cleanup()

	cfg := cm.GetConfig()

	runWithTimeout(t, 5*time.Second, "first SaveConfig", func() error {
		return cm.SaveConfig(cfg)
	})

	// 多次调用，确保锁正确释放、可重复保存。
	runWithTimeout(t, 5*time.Second, "repeated SaveConfig", func() error {
		for i := 0; i < 5; i++ {
			if err := cm.SaveConfig(cfg); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestAddDeviceAndPointPersistThroughSaveConfig 模拟 UI 端添加设备 / 点位的完整
// 持久化链路：修改内存配置 -> ConfigManager.SaveConfig(DB) -> 从 DB 重新加载。
func TestAddDeviceAndPointPersistThroughSaveConfig(t *testing.T) {
	cm, cleanup := newDBConfigManager(t)
	defer cleanup()

	cfg := cm.GetConfig()

	// 1. 添加采集通道
	cfg.Channels = []model.Channel{
		{
			ID:       "ch-1",
			Name:     "Modbus Channel",
			Protocol: "modbus-tcp",
			Enable:   true,
			Config:   map[string]any{"url": "tcp://127.0.0.1:502"},
			Devices:  []model.Device{},
		},
	}
	runWithTimeout(t, 5*time.Second, "save channel", func() error {
		return cm.SaveConfig(cfg)
	})

	// 2. 添加设备
	cfg.Channels[0].Devices = append(cfg.Channels[0].Devices, model.Device{
		ID:       "dev-1",
		Name:     "Slave 1",
		Enable:   true,
		Interval: model.Duration(1000 * time.Millisecond),
		Config:   map[string]any{"slave_id": 1},
		Points:   []model.Point{},
	})
	runWithTimeout(t, 5*time.Second, "save device", func() error {
		return cm.SaveConfig(cfg)
	})

	// 3. 添加点位
	cfg.Channels[0].Devices[0].Points = append(cfg.Channels[0].Devices[0].Points, model.Point{
		ID:       "pt-1",
		Name:     "Temperature",
		Address:  "0",
		DataType: "int16",
	})
	runWithTimeout(t, 5*time.Second, "save point", func() error {
		return cm.SaveConfig(cfg)
	})

	// 4. 从数据库重新加载，验证全部已持久化
	reloaded, err := LoadConfigFromDB(cm.db)
	if err != nil {
		t.Fatalf("LoadConfigFromDB failed: %v", err)
	}

	if len(reloaded.Channels) != 1 {
		t.Fatalf("expected 1 channel after reload, got %d", len(reloaded.Channels))
	}
	if len(reloaded.Channels[0].Devices) != 1 {
		t.Fatalf("expected 1 device after reload, got %d", len(reloaded.Channels[0].Devices))
	}
	dev := reloaded.Channels[0].Devices[0]
	if dev.ID != "dev-1" || dev.Name != "Slave 1" {
		t.Errorf("device not persisted correctly: %+v", dev)
	}
	if len(dev.Points) != 1 {
		t.Fatalf("expected 1 point after reload, got %d", len(dev.Points))
	}
	if dev.Points[0].ID != "pt-1" || dev.Points[0].Address != "0" || dev.Points[0].DataType != "int16" {
		t.Errorf("point not persisted correctly: %+v", dev.Points[0])
	}
}

// TestNorthboundPersistThroughSaveConfig 模拟 UI 端添加/删除北向通道的完整持久化链路。
func TestNorthboundPersistThroughSaveConfig(t *testing.T) {
	cm, cleanup := newDBConfigManager(t)
	defer cleanup()

	cfg := cm.GetConfig()
	cfg.Northbound = model.NorthboundConfig{
		MQTT: []model.MQTTConfig{
			{
				ID:     "nb-mqtt-1",
				Name:   "Cloud MQTT",
				Enable: false,
				Broker: "tcp://127.0.0.1:1883",
				Topic:  "edge/data",
			},
		},
		HTTP: []model.HTTPConfig{
			{
				ID:     "nb-http-1",
				Name:   "Cloud HTTP",
				Enable: false,
				URL:    "http://127.0.0.1:9000",
				Method: "POST",
			},
		},
	}
	runWithTimeout(t, 5*time.Second, "save northbound", func() error {
		return cm.SaveConfig(cfg)
	})

	reloaded, err := LoadConfigFromDB(cm.db)
	if err != nil {
		t.Fatalf("LoadConfigFromDB failed: %v", err)
	}
	if len(reloaded.Northbound.MQTT) != 1 {
		t.Fatalf("expected 1 MQTT northbound, got %d", len(reloaded.Northbound.MQTT))
	}
	if reloaded.Northbound.MQTT[0].ID != "nb-mqtt-1" || reloaded.Northbound.MQTT[0].Broker != "tcp://127.0.0.1:1883" {
		t.Errorf("MQTT northbound not persisted correctly: %+v", reloaded.Northbound.MQTT[0])
	}
	if len(reloaded.Northbound.HTTP) != 1 || reloaded.Northbound.HTTP[0].ID != "nb-http-1" {
		t.Errorf("HTTP northbound not persisted correctly: %+v", reloaded.Northbound.HTTP)
	}

	cfg.Northbound.MQTT = nil
	runWithTimeout(t, 5*time.Second, "save northbound after delete", func() error {
		return cm.SaveConfig(cfg)
	})

	reloaded, err = LoadConfigFromDB(cm.db)
	if err != nil {
		t.Fatalf("LoadConfigFromDB after delete failed: %v", err)
	}
	if len(reloaded.Northbound.MQTT) != 0 {
		t.Fatalf("expected 0 MQTT northbound after delete, got %d", len(reloaded.Northbound.MQTT))
	}
	if len(reloaded.Northbound.HTTP) != 1 {
		t.Fatalf("HTTP northbound should remain, got %d", len(reloaded.Northbound.HTTP))
	}
}

// TestEdgeRulesPersistThroughSaveConfig 模拟 UI 端添加/删除边缘规则的完整持久化链路。
func TestEdgeRulesPersistThroughSaveConfig(t *testing.T) {
	cm, cleanup := newDBConfigManager(t)
	defer cleanup()

	cfg := cm.GetConfig()
	cfg.EdgeRules = []model.EdgeRule{
		{
			ID:          "rule-1",
			Name:        "Threshold Rule",
			Type:        "threshold",
			Enable:      true,
			Condition:   "t1 > 100",
			TriggerMode: "on_change",
			Sources: []model.RuleSource{
				{Alias: "t1", ChannelID: "ch-1", DeviceID: "dev-1", PointID: "pt-1"},
			},
			Actions: []model.RuleAction{
				{Type: "log", Config: map[string]any{"message": "threshold exceeded"}},
			},
		},
	}
	runWithTimeout(t, 5*time.Second, "save edge rules", func() error {
		return cm.SaveConfig(cfg)
	})

	reloaded, err := LoadConfigFromDB(cm.db)
	if err != nil {
		t.Fatalf("LoadConfigFromDB failed: %v", err)
	}
	if len(reloaded.EdgeRules) != 1 {
		t.Fatalf("expected 1 edge rule, got %d", len(reloaded.EdgeRules))
	}
	if reloaded.EdgeRules[0].ID != "rule-1" || reloaded.EdgeRules[0].Condition != "t1 > 100" {
		t.Errorf("edge rule not persisted correctly: %+v", reloaded.EdgeRules[0])
	}

	cfg.EdgeRules = nil
	runWithTimeout(t, 5*time.Second, "save after delete", func() error {
		return cm.SaveConfig(cfg)
	})

	reloaded, err = LoadConfigFromDB(cm.db)
	if err != nil {
		t.Fatalf("LoadConfigFromDB after delete failed: %v", err)
	}
	if len(reloaded.EdgeRules) != 0 {
		t.Fatalf("expected 0 edge rules after delete, got %d", len(reloaded.EdgeRules))
	}
}

// TestSaveConfigRejectsEmptyDeviceID 空设备 ID 应在保存前被拒绝，避免 bbolt key required。
func TestSaveConfigRejectsEmptyDeviceID(t *testing.T) {
	cm, cleanup := newDBConfigManager(t)
	defer cleanup()

	cfg := cm.GetConfig()
	cfg.Channels = []model.Channel{
		{
			ID:       "ch-bad",
			Name:     "Bad Channel",
			Protocol: "modbus-tcp",
			Devices: []model.Device{
				{ID: "", Name: "", Points: []model.Point{{ID: "pt-1", Name: "pt-1"}}},
			},
		},
	}

	err := cm.SaveConfig(cfg)
	if err == nil {
		t.Fatal("expected SaveConfig to fail for empty device ID")
	}
	if !strings.Contains(err.Error(), "device ID or name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
