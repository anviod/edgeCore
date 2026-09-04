package core

import (
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestClassifyEdgeErrorType(t *testing.T) {
	tests := []struct {
		phase string
		err   string
		want  string
	}{
		{"evaluate", "syntax error near >>>", model.EdgeErrorTypeFormula},
		{"action", "write point failed", model.EdgeErrorTypeExecution},
		{"dispatch", "worker_pool_full", model.EdgeErrorTypeDispatch},
		{"action", "check timeout", model.EdgeErrorTypeTimeout},
		{"evaluate", "读取超时", model.EdgeErrorTypeTimeout},
		{"unknown", "something else", model.EdgeErrorTypeOther},
	}
	for _, tc := range tests {
		if got := ClassifyEdgeErrorType(tc.phase, tc.err); got != tc.want {
			t.Fatalf("ClassifyEdgeErrorType(%q, %q) = %q, want %q", tc.phase, tc.err, got, tc.want)
		}
	}
}

func TestShouldPersistEdgeEvent(t *testing.T) {
	if shouldPersistEdgeEvent("completed", "boom") {
		t.Fatal("completed events must not persist")
	}
	if !shouldPersistEdgeEvent("error", "eval failed") || !shouldPersistEdgeEvent("dropped", "pool full") {
		t.Fatal("error/dropped events with message must persist")
	}
	if shouldPersistEdgeEvent("error", "") || shouldPersistEdgeEvent("error", "   ") {
		t.Fatal("events without error message must not persist")
	}
}

func TestIsEdgeErrorMinuteSnapshot(t *testing.T) {
	if IsEdgeErrorMinuteSnapshot(model.RuleMinuteSnapshot{Status: "NORMAL"}) {
		t.Fatal("NORMAL snapshot should be excluded")
	}
	if IsEdgeErrorMinuteSnapshot(model.RuleMinuteSnapshot{ErrorType: "other"}) {
		t.Fatal("error_type-only snapshot should be excluded")
	}
	if IsEdgeErrorMinuteSnapshot(model.RuleMinuteSnapshot{Status: "error"}) {
		t.Fatal("status-only snapshot should be excluded")
	}
	if !IsEdgeErrorMinuteSnapshot(model.RuleMinuteSnapshot{ErrorMessage: "bad expr"}) {
		t.Fatal("error_message snapshot should be included")
	}
}

func TestEmptyErrorNotStored(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error { return nil })
	pipeline.Start()
	em.Start()
	defer em.Stop()

	rule := model.EdgeRule{ID: "empty-err", Name: "Empty Error Rule"}

	em.recordFailure(model.EdgeFailureRecord{
		RuleID: rule.ID, RuleName: rule.Name, Phase: "evaluate", Error: "",
	}, model.Value{})
	em.recordFailure(model.EdgeFailureRecord{
		RuleID: rule.ID, RuleName: rule.Name, Phase: "evaluate", Error: "   ",
	}, model.Value{})
	if failures := em.GetFailures("", 10); len(failures) != 0 {
		t.Fatalf("expected no failure records for empty error, got %d", len(failures))
	}

	tracker := em.startEvent(rule, model.Value{Value: 1})
	em.recordFinishedEvent(tracker, "error", "")
	if events := em.GetEvents("", 10); len(events) != 0 {
		t.Fatalf("expected no events for empty error message, got %d", len(events))
	}

	em.recordMinuteSnapshot(&model.RuleRuntimeState{
		RuleID: rule.ID, RuleName: rule.Name, ExecutionPhase: "error", ErrorMessage: "",
	})
	em.recordMinuteSnapshot(&model.RuleRuntimeState{
		RuleID: rule.ID, RuleName: rule.Name, ExecutionPhase: "action", ErrorMessage: "  ",
	})

	time.Sleep(200 * time.Millisecond)
	found := false
	store.LoadAll("bblot", func(k, v []byte) error {
		if strings.HasPrefix(string(k), rule.ID+"_") {
			found = true
		}
		return nil
	})
	if found {
		t.Fatal("expected no bblot records for empty error message")
	}
}

func TestEdgeEventRecorder_NoEventOnSuccessfulTrigger(t *testing.T) {
	em := newTestEdgeComputeManager(t)

	rule := model.EdgeRule{
		ID: "evt-success", Name: "Success Rule", Type: "threshold", Enable: true,
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Condition: "t1 > 0",
		Actions:   []model.RuleAction{{Type: "log"}},
		State:     &model.StateConfig{Duration: "0s", Count: 1},
	}
	em.LoadRules([]model.EdgeRule{rule})

	em.handleValue(model.Value{
		ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now(),
	})

	time.Sleep(200 * time.Millisecond)
	if events := em.GetEvents("", 10); len(events) > 0 {
		t.Fatalf("expected no persisted events for successful trigger, got %d", len(events))
	}
	states := em.GetRuleStates()
	state := states["evt-success"]
	if state == nil || state.SuccessCount != 1 {
		t.Fatalf("expected success_count=1, got state=%v", state)
	}
}

func TestEdgeEventRecorder_ErrorEventPersisted(t *testing.T) {
	em := newTestEdgeComputeManager(t)

	rule := model.EdgeRule{
		ID: "evt-error", Name: "Error Rule", Type: "threshold", Enable: true,
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Condition: "t1 >>> 0",
	}
	em.LoadRules([]model.EdgeRule{rule})

	em.handleValue(model.Value{
		ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now(),
	})

	time.Sleep(200 * time.Millisecond)
	events := em.GetEvents("", 10)
	if len(events) == 0 {
		t.Fatal("expected error event to be persisted")
	}
	if events[0].Status != "error" {
		t.Fatalf("expected error status, got %q", events[0].Status)
	}
	failures := em.GetFailures("", 10)
	if len(failures) == 0 {
		t.Fatal("expected failure record")
	}
	if failures[0].ErrorType != model.EdgeErrorTypeFormula {
		t.Fatalf("expected formula_error, got %q", failures[0].ErrorType)
	}
	if failures[0].ChannelID != "ch1" || failures[0].DeviceID != "dev1" {
		t.Fatalf("expected channel/device scope on failure, got channel=%q device=%q", failures[0].ChannelID, failures[0].DeviceID)
	}
}
