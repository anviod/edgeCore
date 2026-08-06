package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestEdgeComputeManager_StateLogic(t *testing.T) {
	// Setup
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	// Define Rule
	rule := model.EdgeRule{
		ID:          "rule-state-test",
		Name:        "Test Rule",
		Type:        "state",
		Enable:      true,
		TriggerMode: "always",
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "t1 > 10",
		State: &model.StateConfig{
			Duration: "100ms",
			Count:    3,
		},
		Actions: []model.RuleAction{
			{Type: "log"},
		},
	}

	em.LoadRules([]model.EdgeRule{rule})

	// Helper to feed value
	feed := func(val float64) {
		v := model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   "p1",
			Value:     val,
			TS:        time.Now(),
		}
		// Directly call handleValue to avoid pipeline async delay in test (though handleValue also dispatches to worker pool)
		// We can use the public method, but need to wait for worker.
		// For deterministic test, we might need to wait a bit.
		em.handleValue(v)
		time.Sleep(10 * time.Millisecond) // Wait for worker
	}

	// 1. Initial State
	states := em.GetRuleStates()
	if len(states) != 0 {
		// Rule state is created on first execution
	}

	// 2. Feed value that meets condition (1st time)
	feed(20.0)
	states = em.GetRuleStates()
	if states["rule-state-test"] == nil {
		t.Fatal("Rule state not created")
	}
	if states["rule-state-test"].CurrentStatus == "ALARM" {
		t.Fatal("Rule should not trigger yet (Count 1, Dur < 100ms)")
	}

	// 3. Feed value (2nd time)
	feed(20.0)
	states = em.GetRuleStates()
	if states["rule-state-test"].CurrentStatus == "ALARM" {
		t.Fatal("Rule should not trigger yet (Count 2, Dur < 100ms)")
	}

	// 4. Feed value (3rd time) - Count met, Duration not met
	feed(20.0)
	states = em.GetRuleStates()
	if states["rule-state-test"].CurrentStatus == "ALARM" {
		t.Fatal("Rule should not trigger yet (Count 3, Dur < 100ms)")
	}

	// 5. Wait for Duration
	time.Sleep(150 * time.Millisecond)

	// 6. Feed value (4th time) - Both met
	feed(20.0)
	states = em.GetRuleStates()
	if states["rule-state-test"].CurrentStatus != "ALARM" {
		t.Fatalf("Rule SHOULD trigger now (Count 4, Dur > 100ms). Status: %s", states["rule-state-test"].CurrentStatus)
	}

	// 7. Feed value that fails condition -> Reset
	feed(5.0)
	states = em.GetRuleStates()
	if states["rule-state-test"].CurrentStatus == "ALARM" {
		// Depending on implementation, ALARM might stick if not "on_change" or if logic keeps it.
		// But evaluateState resets ConditionStart/Count when condition fails.
		// And executeRule sets status to NORMAL if triggered=false.
		t.Fatal("Rule should reset to NORMAL")
	}

	// 8. Feed value meeting condition again (1st time after reset)
	feed(20.0)
	states = em.GetRuleStates()
	if states["rule-state-test"].ConditionCount != 1 {
		t.Fatalf("ConditionCount should be 1, got %d", states["rule-state-test"].ConditionCount)
	}
}

func TestEdgeComputeManager_ExecutionPhase(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	rule := model.EdgeRule{
		ID:          "rule-phase-test",
		Name:        "Phase Rule",
		Type:        "state",
		Enable:      true,
		TriggerMode: "always",
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "t1 > 10",
		State: &model.StateConfig{
			Duration: "50ms",
			Count:    1,
		},
		Actions: []model.RuleAction{
			{Type: "log", Config: map[string]any{"message": "ok"}},
		},
	}
	em.LoadRules([]model.EdgeRule{rule})

	feed := func(val float64) {
		em.handleValue(model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   "p1",
			Value:     val,
			TS:        time.Now(),
		})
		time.Sleep(15 * time.Millisecond)
	}

	feed(20.0)
	states := em.GetRuleStates()
	state := states["rule-phase-test"]
	if state == nil {
		t.Fatal("expected rule state")
	}
	if state.ExecutionPhase != "state_hold" {
		t.Fatalf("expected state_hold phase during WARNING, got %q", state.ExecutionPhase)
	}
	if state.CurrentStatus != "WARNING" {
		t.Fatalf("expected WARNING, got %q", state.CurrentStatus)
	}

	time.Sleep(60 * time.Millisecond)
	feed(20.0)
	states = em.GetRuleStates()
	state = states["rule-phase-test"]
	if state.CurrentStatus != "ALARM" {
		t.Fatalf("expected ALARM, got %q", state.CurrentStatus)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		states = em.GetRuleStates()
		state = states["rule-phase-test"]
		if state.ExecutionPhase == "completed" && len(state.ActionLastRuns) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected completed phase with action_last_runs, got phase=%q runs=%v", state.ExecutionPhase, state.ActionLastRuns)
}
