package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_GetConfigAndStop(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "m1", Name: "MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	}, nil, nil, nil, nil)

	cfg := nm.GetConfig()
	if len(cfg.MQTT) != 1 {
		t.Fatalf("GetConfig = %+v", cfg.MQTT)
	}

	nm.Stop()
}

func TestNorthboundManager_DeleteSparkplugBConfig(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		SparkplugB: []model.SparkplugBConfig{{ID: "sp-del", Name: "Sparkplug", Enable: false}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	if err := nm.DeleteSparkplugBConfig("sp-del"); err != nil {
		t.Fatalf("DeleteSparkplugBConfig: %v", err)
	}
	if len(saved.SparkplugB) != 0 {
		t.Fatalf("expected empty sparkplug configs, got %d", len(saved.SparkplugB))
	}
}

func TestNorthboundManager_UpsertSparkplugB_Disabled(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.SparkplugBConfig{ID: "sp-1", Name: "Sparkplug", Enable: false}
	if _, err := nm.UpsertSparkplugBConfig(cfg); err != nil {
		t.Fatalf("UpsertSparkplugBConfig: %v", err)
	}
	if len(saved.SparkplugB) != 1 {
		t.Fatalf("saved sparkplug = %+v", saved.SparkplugB)
	}
}

func TestNorthboundManager_GetMQTTStatsNotFound(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	if _, err := nm.GetMQTTStats("missing"); err == nil {
		t.Fatal("expected error for missing MQTT stats")
	}
}

func TestEdgeComputeManager_ThresholdRuleEvaluation(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, nil)
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{{
		ID: "rule-threshold", Name: "Threshold", Type: "threshold", Enable: true,
		TriggerMode: "always", Condition: "t1 > 50",
		Sources: []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Actions: []model.RuleAction{{Type: "log", Config: map[string]any{"message": "high"}}},
	}})

	em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 60, TS: time.Now()})
	time.Sleep(50 * time.Millisecond)

	metrics := em.GetMetrics()
	if metrics.RulesExecuted == 0 {
		t.Fatalf("expected rule execution, metrics=%+v", metrics)
	}
}

func TestEdgeComputeManager_WindowRuleEvaluation(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, nil)
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{{
		ID: "rule-window", Name: "Window", Type: "window", Enable: true,
		TriggerMode: "always", Condition: "value > 5",
		Sources: []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Window:  &model.WindowConfig{Size: "10", AggrFunc: "avg"},
		Actions: []model.RuleAction{{Type: "log"}},
	}})

	for i := 0; i < 3; i++ {
		em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: float64(10 + i), TS: time.Now()})
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	if data := em.GetWindowData("rule-window"); len(data) == 0 {
		t.Fatal("window data should be populated")
	}
}

func TestEdgeComputeManager_CalculationRuleEvaluation(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, nil)
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{{
		ID: "rule-calc", Name: "Calc", Type: "calculation", Enable: true,
		TriggerMode: "always",
		Sources: []model.RuleSource{
			{Alias: "a", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
			{Alias: "b", ChannelID: "ch1", DeviceID: "dev1", PointID: "p2"},
		},
		Expression: "a + b",
		Actions:    []model.RuleAction{{Type: "log"}},
	}})

	em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 3, TS: time.Now()})
	em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p2", Value: 4, TS: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if em.GetMetrics().RulesExecuted == 0 {
		t.Fatal("calculation rule should execute")
	}
}

func TestChannelManager_UpdateDevice(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	updated := &model.Device{
		ID: "dev-1", Name: "Updated Device", Enable: true,
		Interval: model.Duration(time.Second),
		Points:   []model.Point{{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"}},
		Config:   map[string]any{"slave_id": 2},
	}
	if err := cm.UpdateDevice(channelID, updated); err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	dev := cm.GetDevice(channelID, "dev-1")
	if dev.Name != "Updated Device" {
		t.Fatalf("device name = %q", dev.Name)
	}
}
