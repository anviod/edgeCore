package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestEdgeRulePersistence(t *testing.T) {
	// 1. Setup Storage
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// 2. Setup EdgeComputeManager
	pipeline := NewDataPipeline(10)
	ecm := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error {
		return nil // Mock save config
	})
	ecm.SetBatchWindow(0)

	pipeline.Start()
	ecm.Start()
	defer ecm.Stop()

	// 3. Define Rule
	ruleID := "rule-persist-1"
	rule := model.EdgeRule{
		ID:          ruleID,
		Name:        "TestPersistence",
		Type:        "state",
		Enable:      true,
		TriggerMode: "always",
		Sources: []model.RuleSource{
			{Alias: "t1", PointID: "p1"},
		},
		Condition: "t1 > 10",
		State: &model.StateConfig{
			Duration: "1s",
			Count:    2,
		},
	}

	ecm.LoadRules([]model.EdgeRule{rule})

	// 4. Simulate Data Trigger
	// Send p1 = 11 (True)
	pipeline.Push(model.Value{
		PointID: "p1",
		Value:   11,
		TS:      time.Now(),
	})

	time.Sleep(100 * time.Millisecond) // Wait for processing

	// Check State (Should be in progress)
	states := ecm.GetRuleStates()
	if s, ok := states[ruleID]; !ok {
		t.Fatalf("Rule state not found")
	} else {
		if s.ConditionCount != 1 {
			t.Errorf("Expected ConditionCount 1, got %d", s.ConditionCount)
		}
	}

	// Wait a bit and check persistence
	time.Sleep(200 * time.Millisecond)

	// Verify DB has the state
	store.LoadAll(storage.BucketRuleState, func(k, v []byte) error {
		if string(k) == ruleID {
			t.Logf("Found persisted state for %s", ruleID)
			return nil
		}
		return nil
	})

	// 5. Restart ECM to test Restore
	ecm.Stop()

	// New ECM instance with same store
	ecm2 := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error {
		return nil
	})
	ecm2.SetBatchWindow(0)
	// Load same rules
	ecm2.LoadRules([]model.EdgeRule{rule})

	// Start (should restore)
	ecm2.Start()
	defer ecm2.Stop()

	// Check Restored State
	states2 := ecm2.GetRuleStates()
	if s, ok := states2[ruleID]; !ok {
		t.Fatalf("Restored rule state not found")
	} else {
		t.Logf("Restored State: %+v", s)
		if s.ConditionCount != 1 {
			t.Errorf("Expected restored ConditionCount 1, got %d", s.ConditionCount)
		}
		if s.ConditionStart.IsZero() {
			t.Error("Expected restored ConditionStart to be set")
		}
	}
}
