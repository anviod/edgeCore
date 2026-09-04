package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestEdgeComputeManager_ClearEdgeLogs(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	rule := model.EdgeRule{
		ID: "clear-log-rule", Name: "Clear Log Rule", Type: "threshold", Enable: true,
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Condition: "t1 >>> 0",
		State:     &model.StateConfig{Duration: "0s", Count: 1},
	}
	em.LoadRules([]model.EdgeRule{rule})

	em.handleValue(model.Value{
		ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now(),
	})
	time.Sleep(100 * time.Millisecond)

	statesBefore := em.GetRuleStates()
	if len(statesBefore) == 0 {
		t.Fatal("expected rule state to exist before clear")
	}

	if err := store.SaveData(edgeEventsBucket, "evt-1", model.EdgeRuleEvent{ID: "evt-1", RuleID: rule.ID}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := store.SaveData(edgeFailuresBucket, "fail-1", model.EdgeFailureRecord{ID: "fail-1", RuleID: rule.ID}); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	if err := store.SaveData(edgeBblotBucket, rule.ID+"_2026-07-05 19:00", model.RuleMinuteSnapshot{RuleID: rule.ID}); err != nil {
		t.Fatalf("seed bblot: %v", err)
	}

	em.bblotMu.Lock()
	em.minuteCache["cached"] = &model.RuleMinuteSnapshot{RuleID: rule.ID}
	em.bblotMu.Unlock()

	result, err := em.ClearEdgeLogs()
	if err != nil {
		t.Fatalf("ClearEdgeLogs: %v", err)
	}
	if result.MinuteCache < 1 {
		t.Fatalf("expected minute cache count >= 1, got %d", result.MinuteCache)
	}
	if len(em.GetEvents("", 10)) != 0 {
		t.Fatal("expected in-memory events cleared")
	}
	if len(em.GetFailures("", 10)) != 0 {
		t.Fatal("expected in-memory failures cleared")
	}

	for _, bucket := range edgeLogBuckets {
		count := 0
		_ = store.LoadAll(bucket, func(k, v []byte) error {
			count++
			return nil
		})
		if count != 0 {
			t.Fatalf("expected bucket %s empty, got %d records", bucket, count)
		}
	}

	statesAfter := em.GetRuleStates()
	if len(statesAfter) == 0 {
		t.Fatal("rule states should be preserved after clear")
	}
	if statesAfter[rule.ID] == nil {
		t.Fatal("rule state should be preserved after clear")
	}

	em.bblotMu.Lock()
	cacheLen := len(em.minuteCache)
	em.bblotMu.Unlock()
	if cacheLen != 0 {
		t.Fatalf("expected minute cache cleared, got %d entries", cacheLen)
	}

	// Ensure persisted rule state bucket untouched.
	var foundState bool
	_ = store.LoadAll(storage.BucketRuleState, func(k, v []byte) error {
		if string(k) == rule.ID {
			foundState = true
			var state model.RuleRuntimeState
			if err := json.Unmarshal(v, &state); err != nil {
				t.Fatalf("unmarshal rule state: %v", err)
			}
		}
		return nil
	})
	if !foundState {
		t.Fatal("expected rule state bucket preserved")
	}
}

func TestClearAllRuntimeFlowClearsEdgeLogMemory(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	rule := model.EdgeRule{ID: "flow-rule", Name: "Flow Rule", Type: "threshold", Enable: true}
	em.LoadRules([]model.EdgeRule{rule})

	if err := store.SaveData(edgeEventsBucket, "evt-flow", model.EdgeRuleEvent{ID: "evt-flow", RuleID: rule.ID, Status: "error"}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	em.events.recordEvent(&model.EdgeRuleEvent{ID: "mem-evt", RuleID: rule.ID, Status: "error"})
	em.events.recordFailure(model.EdgeFailureRecord{ID: "mem-fail", RuleID: rule.ID, Error: "boom"})

	if _, err := store.ClearAllRuntimeBuckets(); err != nil {
		t.Fatalf("ClearAllRuntimeBuckets: %v", err)
	}
	if _, err := em.ClearEdgeLogs(); err != nil {
		t.Fatalf("ClearEdgeLogs: %v", err)
	}
	em.ClearRuntimeState()

	if len(em.GetEvents("", 10)) != 0 {
		t.Fatal("expected in-memory events cleared after clear-all-runtime flow")
	}
	if len(em.GetFailures("", 10)) != 0 {
		t.Fatal("expected in-memory failures cleared after clear-all-runtime flow")
	}
}
