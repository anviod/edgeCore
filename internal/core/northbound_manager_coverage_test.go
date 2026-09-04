package core

import (
	"strings"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_ConnectorStartWarningMessage(t *testing.T) {
	if got := connectorStartWarning("MQTT", "test", nil); got != "" {
		t.Fatalf("nil err should return empty, got %q", got)
	}
	warn := connectorStartWarning("MQTT Broker", "broker-1", errTestBroker)
	if warn == "" || !strings.Contains(warn, "MQTT Broker") {
		t.Fatalf("unexpected warning: %q", warn)
	}
}

var errTestBroker = &testBrokerError{msg: "connection refused"}

type testBrokerError struct{ msg string }

func (e *testBrokerError) Error() string { return e.msg }

func TestNorthboundManager_GetNorthboundStats(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT:       []model.MQTTConfig{{ID: "m1", Name: "MQTT", Enable: false}},
		HTTP:       []model.HTTPConfig{{ID: "h1", Name: "HTTP", Enable: true}},
		OPCUA:      []model.OPCUAConfig{{ID: "o1", Name: "OPC UA", Enable: false}},
		SparkplugB: []model.SparkplugBConfig{{ID: "s1", Name: "Sparkplug", Enable: true}},
		EdgeOSMQTT: []model.EdgeOSMQTTConfig{{ID: "e1", Name: "EdgeOS MQTT", Enable: false}},
		EdgeOSNATS: []model.EdgeOSNATSConfig{{ID: "n1", Name: "EdgeOS NATS", Enable: true}},
	}, nil, nil, nil, nil)

	stats := nm.GetNorthboundStats()
	if len(stats) != 5 {
		t.Fatalf("expected 5 stats entries (HTTP not included), got %d", len(stats))
	}
	statusByType := map[string]string{}
	for _, s := range stats {
		statusByType[s.Type] = s.Status
	}
	if statusByType["MQTT"] != "Disabled" {
		t.Fatalf("MQTT status = %q", statusByType["MQTT"])
	}
	if statusByType["SparkplugB"] != "Stopped" {
		t.Fatalf("SparkplugB status = %q", statusByType["SparkplugB"])
	}
	if _, ok := statusByType["HTTP"]; ok {
		t.Fatal("HTTP should not appear in GetNorthboundStats")
	}
}

func TestNorthboundManager_EdgeOSMQTT_CRUD_Disabled(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.EdgeOSMQTTConfig{
		ID: "edgos-mqtt-1", Name: "Edge MQTT", Enable: false,
		Broker: "tcp://127.0.0.1:1883",
	}
	if _, err := nm.UpsertEdgeOSMQTTConfig(cfg); err != nil {
		t.Fatalf("UpsertEdgeOSMQTTConfig: %v", err)
	}
	if len(saved.EdgeOSMQTT) != 1 || saved.EdgeOSMQTT[0].ID != "edgos-mqtt-1" {
		t.Fatalf("saved EdgeOS MQTT = %+v", saved.EdgeOSMQTT)
	}

	if _, err := nm.GetEdgeOSMQTTStats("edgos-mqtt-1"); err == nil {
		t.Fatal("expected error for disabled client stats")
	}
	if err := nm.PublishEdgeOSMQTT("edgos-mqtt-1", "t", []byte("x")); err == nil {
		t.Fatal("expected publish error for missing client")
	}

	if err := nm.DeleteEdgeOSMQTTConfig("edgos-mqtt-1"); err != nil {
		t.Fatalf("DeleteEdgeOSMQTTConfig: %v", err)
	}
	if len(saved.EdgeOSMQTT) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(saved.EdgeOSMQTT))
	}
}

func TestNorthboundManager_EdgeOSNATS_CRUD_Disabled(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.EdgeOSNATSConfig{
		ID: "edgos-nats-1", Name: "Edge NATS", Enable: false,
		URL: "nats://127.0.0.1:4222",
	}
	if _, err := nm.UpsertEdgeOSNATSConfig(cfg); err != nil {
		t.Fatalf("UpsertEdgeOSNATSConfig: %v", err)
	}
	if len(saved.EdgeOSNATS) != 1 {
		t.Fatalf("saved EdgeOS NATS = %+v", saved.EdgeOSNATS)
	}

	if _, err := nm.GetEdgeOSNATSStats("edgos-nats-1"); err == nil {
		t.Fatal("expected error for disabled NATS client stats")
	}
	if err := nm.PublishEdgeOSNATS("edgos-nats-1", "subj", []byte("x")); err == nil {
		t.Fatal("expected publish error for missing client")
	}

	if err := nm.DeleteEdgeOSNATSConfig("edgos-nats-1"); err != nil {
		t.Fatalf("DeleteEdgeOSNATSConfig: %v", err)
	}
}

func TestNorthboundManager_DeleteHTTPConfig(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		HTTP: []model.HTTPConfig{{ID: "http-del", Name: "To Delete", Enable: false, URL: "http://127.0.0.1"}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	if err := nm.DeleteHTTPConfig("http-del"); err != nil {
		t.Fatalf("DeleteHTTPConfig: %v", err)
	}
	if len(saved.HTTP) != 0 {
		t.Fatalf("expected 0 HTTP configs, got %d", len(saved.HTTP))
	}
}

func TestNorthboundManager_FindDeviceViaChannelManager(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()
	_ = cm.AddChannel(&model.Channel{
		ID: "ch-nb", Name: "NB Channel", Protocol: addChannelMockProtocol,
		Devices: []model.Device{{ID: "dev-nb", Name: "NB Device"}},
	})

	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	nm.SetChannelManager(cm)

	dev := nm.findDevice("dev-nb")
	if dev == nil {
		t.Fatal("findDevice should locate device")
	}
	if nm.findDevice("missing") != nil {
		t.Fatal("missing device should return nil")
	}

	nm2 := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	if nm2.findDevice("dev-nb") != nil {
		t.Fatal("findDevice without cm should return nil")
	}
}

func TestNorthboundManager_ValidateChannelName_AllProtocols(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		OPCUA:      []model.OPCUAConfig{{ID: "opc-1", Name: "OPC UA Server"}},
		SparkplugB: []model.SparkplugBConfig{{ID: "sp-1", Name: "Sparkplug Client"}},
		EdgeOSMQTT: []model.EdgeOSMQTTConfig{{ID: "em-1", Name: "Edge MQTT"}},
		EdgeOSNATS: []model.EdgeOSNATSConfig{{ID: "en-1", Name: "Edge NATS"}},
	}, nil, nil, nil, nil)

	dupCases := []struct {
		excludeID string
		name      string
	}{
		{"", "OPC UA Server"},
		{"", "Sparkplug Client"},
		{"", "Edge MQTT"},
		{"", "Edge NATS"},
	}
	for _, tc := range dupCases {
		nm.mu.Lock()
		err := nm.validateNorthboundChannelName(tc.excludeID, tc.name)
		nm.mu.Unlock()
		if err == nil || !strings.Contains(err.Error(), "已存在") {
			t.Fatalf("validate(%q) = %v, want duplicate error", tc.name, err)
		}
	}
}

func TestNorthboundManager_UpsertMQTT_DisableStopsClient(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "mqtt-off", Name: "MQTT Off", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	disabled := model.MQTTConfig{ID: "mqtt-off", Name: "MQTT Off", Enable: false, Broker: "tcp://127.0.0.1:1883"}
	if _, err := nm.UpsertMQTTConfig(disabled); err != nil {
		t.Fatalf("UpsertMQTTConfig disable: %v", err)
	}
	if len(saved.MQTT) != 1 || saved.MQTT[0].Enable {
		t.Fatalf("saved config = %+v", saved.MQTT)
	}
}

func TestNorthboundManager_PublishNotFound(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)

	if err := nm.PublishHTTP("missing", []byte("x")); err == nil {
		t.Fatal("expected HTTP publish error")
	}
	if err := nm.PublishMQTTClient("missing", "t", []byte("x")); err == nil {
		t.Fatal("expected MQTT publish error")
	}
}
