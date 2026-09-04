package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestEdgeComputeManager_TypeConversion(t *testing.T) {
	// Setup
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	// Define Rule with t1 > 1 && t2 > 3
	rule := model.EdgeRule{
		ID:          "rule-type-test",
		Name:        "Type Test Rule",
		Type:        "threshold",
		Enable:      true,
		TriggerMode: "always",
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
			{Alias: "t2", ChannelID: "ch1", DeviceID: "dev1", PointID: "p2"},
		},
		Condition: "t1 > 1 && t2 > 3",
		Actions: []model.RuleAction{
			{Type: "log"},
		},
	}

	em.LoadRules([]model.EdgeRule{rule})

	// Helper to feed value
	feed := func(alias string, val any) {
		var pid string
		if alias == "t1" {
			pid = "p1"
		} else {
			pid = "p2"
		}
		v := model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   pid,
			Value:     val,
			TS:        time.Now(),
		}
		em.handleValue(v)
		time.Sleep(50 * time.Millisecond) // Wait for worker
	}

	// 1. Feed t1 = "2" (string)
	feed("t1", "2")

	// 2. Feed t2 = "4" (string) - This should trigger the rule
	feed("t2", "4")

	// Check State
	states := em.GetRuleStates()
	if states["rule-type-test"] == nil {
		t.Fatal("Rule state not created")
	}

	// Should be ALARM because "2" > 1 and "4" > 3
	if states["rule-type-test"].CurrentStatus != "ALARM" {
		t.Fatalf("Rule SHOULD trigger with string inputs. Status: %s, Error: %s",
			states["rule-type-test"].CurrentStatus,
			states["rule-type-test"].ErrorMessage)
	}

	// 3. Feed int32 input
	feed("t1", int32(5))
	feed("t2", int32(6))

	states = em.GetRuleStates()
	if states["rule-type-test"].CurrentStatus != "ALARM" {
		t.Fatalf("Rule SHOULD trigger with int32 inputs. Status: %s, Error: %s",
			states["rule-type-test"].CurrentStatus,
			states["rule-type-test"].ErrorMessage)
	}
}
