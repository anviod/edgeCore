package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_StartHandleValueAndStop(t *testing.T) {
	pipeline := NewDataPipeline(10)
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT:       []model.MQTTConfig{{ID: "m1", Name: "MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
		HTTP:       []model.HTTPConfig{{ID: "h1", Name: "HTTP", Enable: false, URL: "http://127.0.0.1"}},
		OPCUA:      []model.OPCUAConfig{{ID: "o1", Name: "OPC UA", Enable: false}},
		SparkplugB: []model.SparkplugBConfig{{ID: "s1", Name: "Sparkplug", Enable: false}},
		EdgeOSMQTT: []model.EdgeOSMQTTConfig{{ID: "e1", Name: "Edge MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
		EdgeOSNATS: []model.EdgeOSNATSConfig{{ID: "n1", Name: "Edge NATS", Enable: false, URL: "nats://127.0.0.1:4222"}},
	}, pipeline, nil, nil, nil)

	nm.Start()

	val := model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 42, TS: time.Now()}
	nm.handleValue(val)
	nm.PublishPointsMetadata()
	nm.PublishPointsSync("ch1", "dev1")

	nm.Stop()
}

func TestNorthboundManager_OnDeviceStatusChangeWithChannelManager(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()
	_ = cm.AddChannel(&model.Channel{
		ID: "ch-nb2", Name: "NB2", Protocol: addChannelMockProtocol,
		Devices: []model.Device{{
			ID: "dev-lifecycle", Name: "Lifecycle Dev",
			Config: map[string]any{"slave_id": 1},
		}},
	})

	pipeline := NewDataPipeline(10)
	nm := NewNorthboundManager(model.NorthboundConfig{
		EdgeOSMQTT: []model.EdgeOSMQTTConfig{{
			ID: "em1", Name: "Edge MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883",
			Devices: map[string]model.DevicePublishConfig{
				"dev-lifecycle": {Enable: true},
			},
		}},
	}, pipeline, nil, nil, nil)
	nm.SetChannelManager(cm)

	nm.OnDeviceStatusChange("dev-lifecycle", 0)
	nm.OnDeviceStatusChange("dev-lifecycle", 2)
	nm.OnDeviceStatusChange("dev-lifecycle", 3)
}

func TestNorthboundManager_DeleteOPCUAConfig(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		OPCUA: []model.OPCUAConfig{{ID: "opc-del", Name: "OPC UA", Enable: false}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	if err := nm.DeleteOPCUAConfig("opc-del"); err != nil {
		t.Fatalf("DeleteOPCUAConfig: %v", err)
	}
	if len(saved.OPCUA) != 0 {
		t.Fatalf("expected empty opcua configs, got %d", len(saved.OPCUA))
	}
}

func TestNorthboundManager_GetOPCUAStatsNotFound(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	if _, err := nm.GetOPCUAStats("missing"); err == nil {
		t.Fatal("expected error for missing OPC UA stats")
	}
}

func TestNorthboundManager_UpdateEdgeOSClientsDisabled(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	nm.mu.Lock()
	nm.updateEdgeOSMQTTClients(nil, []model.EdgeOSMQTTConfig{{ID: "e1", Name: "E", Enable: false}})
	nm.updateEdgeOSNATSClients(nil, []model.EdgeOSNATSConfig{{ID: "n1", Name: "N", Enable: false}})
	nm.mu.Unlock()
}

func TestEdgeComputeManager_QueryLogsWithoutStorage(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	_, err := em.QueryLogs(time.Now().Add(-time.Hour), time.Now(), "")
	if err == nil {
		t.Fatal("expected error without storage")
	}
}

func TestEdgeComputeManager_GetFailedActionsWithoutStorage(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	if len(em.GetFailedActions()) != 0 {
		t.Fatal("expected empty failed actions without storage")
	}
}

func TestEdgeComputeManager_ExecuteMqttWithoutNorthbound(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	err := em.executeMqtt(t.Context(), "rule1", model.RuleAction{
		Type:   "mqtt",
		Config: map[string]any{"topic": "t/test", "message": "hello"},
	}, model.Value{}, nil)
	if err == nil {
		t.Fatal("expected error without northbound manager")
	}
}

func TestEdgeComputeManager_ExecuteDatabaseWithoutStorage(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	err := em.executeDatabase(t.Context(), "rule1", model.RuleAction{
		Type:   "database",
		Config: map[string]any{"bucket": "test"},
	}, model.Value{}, nil)
	if err == nil {
		t.Fatal("expected error without storage")
	}
}

func TestScanEngine_IsRunningAndFindTask(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	if se.IsRunning() {
		t.Fatal("engine should not be running before Run")
	}
	task := se.AddTask("dev-find", "modbus-tcp", time.Second, 1, []string{"p1"}, nil)
	se.mu.Lock()
	found := se.findTaskLocked("dev-find")
	se.mu.Unlock()
	if found == nil || found.ID != task.ID {
		t.Fatalf("findTaskLocked = %+v", found)
	}
}

func TestScanEngine_LogSLAWarnings(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	se.logSLAWarnings()
}
