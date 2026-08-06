package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestEdgeComputeManager_UpsertRule_PersistViaSaveFunc(t *testing.T) {
	var saved []model.EdgeRule
	em := NewEdgeComputeManager(nil, nil, func(rules []model.EdgeRule) error {
		saved = rules
		return nil
	})

	rule := model.EdgeRule{
		ID:          "rule-1",
		Name:        "High Temperature Alert",
		Type:        "threshold",
		Enable:      true,
		Condition:   "t1 > 80",
		TriggerMode: "on_change",
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch-1", DeviceID: "dev-1", PointID: "pt-1"},
		},
		Actions: []model.RuleAction{
			{Type: "log", Config: map[string]any{"message": "alert"}},
		},
	}
	if err := em.UpsertRule(rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	if len(saved) != 1 {
		t.Fatalf("expected 1 rule saved, got %d", len(saved))
	}
	if saved[0].ID != "rule-1" || saved[0].Name != "High Temperature Alert" {
		t.Errorf("rule not saved correctly: %+v", saved[0])
	}
}

func TestEdgeComputeManager_DeleteRule_PersistViaSaveFunc(t *testing.T) {
	var saved []model.EdgeRule
	em := NewEdgeComputeManager(nil, nil, func(rules []model.EdgeRule) error {
		saved = rules
		return nil
	})

	em.rules["rule-1"] = model.EdgeRule{ID: "rule-1", Name: "Rule 1", Type: "threshold", Enable: true}

	if err := em.DeleteRule("rule-1"); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	if len(saved) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(saved))
	}
}

func TestEdgeComputeManager_UpsertRule_RejectsEmptyID(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, func(rules []model.EdgeRule) error {
		return nil
	})

	err := em.UpsertRule(model.EdgeRule{ID: "", Name: "", Type: "threshold"})
	if err == nil {
		t.Fatal("expected error for empty rule ID")
	}
}
