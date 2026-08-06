package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_UpsertMQTT_PersistViaSaveFunc(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.MQTTConfig{
		ID:     "nb-mqtt-1",
		Name:   "Test MQTT",
		Enable: false,
		Broker: "tcp://127.0.0.1:1883",
		Topic:  "test/topic",
	}
	if _, err := nm.UpsertMQTTConfig(cfg); err != nil {
		t.Fatalf("UpsertMQTTConfig: %v", err)
	}

	if len(saved.MQTT) != 1 {
		t.Fatalf("expected 1 MQTT config saved, got %d", len(saved.MQTT))
	}
	if saved.MQTT[0].ID != "nb-mqtt-1" {
		t.Errorf("expected id nb-mqtt-1, got %s", saved.MQTT[0].ID)
	}
	if saved.Status != nil {
		t.Errorf("runtime Status should not be persisted, got %v", saved.Status)
	}
}

func TestNorthboundManager_DeleteMQTT_PersistViaSaveFunc(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{
			{ID: "nb-mqtt-1", Name: "Test MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"},
		},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	if err := nm.DeleteMQTTConfig("nb-mqtt-1"); err != nil {
		t.Fatalf("DeleteMQTTConfig: %v", err)
	}

	if len(saved.MQTT) != 0 {
		t.Fatalf("expected 0 MQTT configs after delete, got %d", len(saved.MQTT))
	}
}

func TestNorthboundManager_UpsertHTTP_PersistViaSaveFunc(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.HTTPConfig{
		ID:     "nb-http-1",
		Name:   "Test HTTP",
		Enable: false,
		URL:    "http://127.0.0.1:8080",
		Method: "POST",
	}
	if err := nm.UpsertHTTPConfig(cfg); err != nil {
		t.Fatalf("UpsertHTTPConfig: %v", err)
	}

	if len(saved.HTTP) != 1 || saved.HTTP[0].ID != "nb-http-1" {
		t.Fatalf("HTTP config not saved correctly: %+v", saved.HTTP)
	}
}

func TestNorthboundManager_UpsertOPCUA_PersistDeviceMapping(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.OPCUAConfig{
		ID:       "nb-opcua-1",
		Name:     "Test OPC UA",
		Enable:   false,
		Port:     4840,
		Endpoint: "/ipp/opcua/server",
		Devices: model.OpcUaDeviceMap{
			"dev1": {Enable: true, Strategy: "periodic", Interval: model.Duration(10 * 1e9)},
		},
	}
	if _, _, err := nm.UpsertOPCUAConfig(cfg); err != nil {
		t.Fatalf("UpsertOPCUAConfig: %v", err)
	}

	if len(saved.OPCUA) != 1 {
		t.Fatalf("expected 1 OPC UA config saved, got %d", len(saved.OPCUA))
	}
	if !saved.OPCUA[0].Devices["dev1"].Enable {
		t.Fatalf("device mapping not persisted: %+v", saved.OPCUA[0].Devices)
	}
}
