package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestEdgeComputeManager_ActionHookAndMetrics(t *testing.T) {
	var saved []model.EdgeRule
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error {
		saved = rules
		return nil
	})
	em.SetBatchWindow(0)

	var hookMu sync.Mutex
	var hookCalls int
	em.SetActionHook(func(ruleID string, action model.RuleAction, _ model.Value, _ map[string]any, err error) {
		hookMu.Lock()
		defer hookMu.Unlock()
		if ruleID == "rule-hook" && action.Type == "log" && err == nil {
			hookCalls++
		}
	})

	cm := NewChannelManager(nil, nil)
	defer cm.cancel()
	em.SetChannelManager(cm)
	em.SetNorthboundManager(NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil))

	rule := model.EdgeRule{
		ID: "rule-hook", Name: "Hook Rule", Type: "threshold", Enable: true,
		TriggerMode: "always", Condition: "t1 > 0",
		Sources: []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Actions: []model.RuleAction{
			{Type: "log", Config: map[string]any{"message": "triggered", "level": "warn"}},
		},
	}
	if err := em.UpsertRule(rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	rules := em.GetRules()
	if len(rules) != 1 {
		t.Fatalf("GetRules = %d, want 1", len(rules))
	}

	em.Start()
	defer em.Stop()

	em.handleValue(model.Value{
		ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 10, TS: time.Now(),
	})
	time.Sleep(50 * time.Millisecond)

	hookMu.Lock()
	calls := hookCalls
	hookMu.Unlock()
	if calls == 0 {
		t.Fatal("actionHook should be invoked for log action")
	}

	metrics := em.GetMetrics()
	if metrics.RuleCount != 1 {
		t.Fatalf("GetMetrics rule count = %+v", metrics)
	}

	em.stateMu.Lock()
	em.ruleStates["rule-hook"] = &model.RuleRuntimeState{RuleID: "rule-hook", CurrentStatus: "ALARM"}
	em.stateMu.Unlock()
	em.ClearRuntimeState()
	states := em.GetRuleStates()
	if len(states) != 0 {
		t.Fatalf("ClearRuntimeState should reset states, got %+v", states)
	}

	if err := em.DeleteRule("rule-hook"); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if len(saved) != 0 {
		t.Fatalf("expected 0 saved rules after delete, got %d", len(saved))
	}
}

func TestEdgeComputeManager_SanitizeRuleOnUpsert(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, func(rules []model.EdgeRule) error { return nil })

	rule := model.EdgeRule{
		ID: "rule-sanitize", Name: "Sanitize", Type: "threshold", Enable: true,
		Condition: "t1 > 0",
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "d1", PointID: "p1"}},
		Actions: []model.RuleAction{{
			Type: "device_control",
			Config: map[string]any{
				"targets": []interface{}{
					map[string]interface{}{
						"channel_id":  "ch1",
						"device_id":   "d1",
						"point_id":    "p1",
						"_deviceList": []string{"should-remove"},
						"_pointList":  []string{"should-remove"},
					},
				},
			},
		}},
	}
	if err := em.UpsertRule(rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	got := em.rules["rule-sanitize"]
	targets, ok := got.Actions[0].Config["targets"].([]interface{})
	if !ok || len(targets) != 1 {
		t.Fatalf("targets = %+v", got.Actions[0].Config["targets"])
	}
	targetMap, ok := targets[0].(map[string]interface{})
	if !ok {
		t.Fatalf("target type = %T", targets[0])
	}
	if _, exists := targetMap["_deviceList"]; exists {
		t.Fatal("_deviceList should be sanitized")
	}
	if _, exists := targetMap["_pointList"]; exists {
		t.Fatal("_pointList should be sanitized")
	}
}

func TestEdgeComputeManager_ExecuteSingleAction_Unsupported(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	var hookErr error
	em.SetActionHook(func(_ string, _ model.RuleAction, _ model.Value, _ map[string]any, err error) {
		hookErr = err
	})

	err := em.executeSingleAction(context.Background(), "r1", model.RuleAction{Type: "unknown"}, model.Value{}, nil)
	if err == nil {
		t.Fatal("expected unsupported action error")
	}
	if hookErr == nil {
		t.Fatal("actionHook should receive error from defer")
	}
}

func TestEdgeComputeManager_ExecuteLogLevels(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	levels := []string{"info", "warn", "error", ""}
	for _, level := range levels {
		action := model.RuleAction{
			Type:   "log",
			Config: map[string]any{"level": level, "message": "test"},
		}
		if err := em.executeLog(context.Background(), "rule-log", action, model.Value{}); err != nil {
			t.Fatalf("executeLog level=%q: %v", level, err)
		}
	}
}
