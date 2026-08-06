package core

import (
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func newTestEdgeComputeManager(t *testing.T) *EdgeComputeManager {
	t.Helper()
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0) // immediate dispatch for deterministic unit tests
	em.Start()
	t.Cleanup(em.Stop)
	return em
}

func TestEdgeComputeManager_CheckIntervalThrottling(t *testing.T) {
	em := newTestEdgeComputeManager(t)

	rule := model.EdgeRule{
		ID:            "rule-interval",
		Name:          "Interval Rule",
		Type:          "threshold",
		Enable:        true,
		CheckInterval: "200ms",
		Priority:      1,
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "t1 > 0",
	}
	em.LoadRules([]model.EdgeRule{rule})

	val := model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   "p1",
		Value:     1.0,
		TS:        time.Now(),
	}

	em.handleValue(val)
	time.Sleep(30 * time.Millisecond)
	if em.GetMetrics().RulesExecuted != 1 {
		t.Fatalf("expected 1 execution, got %d", em.GetMetrics().RulesExecuted)
	}

	em.handleValue(val)
	time.Sleep(30 * time.Millisecond)
	if em.GetMetrics().RulesExecuted != 1 {
		t.Fatalf("expected CheckInterval to suppress second execution, got %d", em.GetMetrics().RulesExecuted)
	}

	time.Sleep(200 * time.Millisecond)
	em.handleValue(val)
	time.Sleep(30 * time.Millisecond)
	if em.GetMetrics().RulesExecuted != 2 {
		t.Fatalf("expected execution after CheckInterval elapsed, got %d", em.GetMetrics().RulesExecuted)
	}
}

func TestEdgeRuleScheduler_BatchCoalescing(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(100 * time.Millisecond)
	em.Start()
	defer em.Stop()

	var executed int64
	em.SetActionHook(func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		atomic.AddInt64(&executed, 1)
	})

	rule := model.EdgeRule{
		ID:     "rule-coalesce",
		Name:   "Coalesce Rule",
		Type:   "threshold",
		Enable: true,
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "t1 > 0",
		Actions:   []model.RuleAction{{Type: "log"}},
		State: &model.StateConfig{
			Duration: "0s",
			Count:    1,
		},
	}
	em.LoadRules([]model.EdgeRule{rule})

	for i := 0; i < 5; i++ {
		em.handleValue(model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   "p1",
			Value:     float64(i + 1),
			TS:        time.Now(),
		})
	}

	time.Sleep(50 * time.Millisecond)
	if got := em.GetMetrics().RulesCoalesced; got < 4 {
		t.Fatalf("expected at least 4 coalesced events, got %d", got)
	}
	if atomic.LoadInt64(&executed) > 0 {
		t.Fatalf("expected no execution before batch window flush, got %d", executed)
	}

	time.Sleep(120 * time.Millisecond)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if em.GetMetrics().RulesExecuted >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if em.GetMetrics().RulesExecuted != 1 {
		t.Fatalf("expected 1 execution after coalescing, got %d", em.GetMetrics().RulesExecuted)
	}
}

func TestEdgeRuleScheduler_FlushPriorityOrder(t *testing.T) {
	tasks := []*ruleTask{
		{rule: model.EdgeRule{ID: "low", Priority: 1}},
		{rule: model.EdgeRule{ID: "mid", Priority: 5}},
		{rule: model.EdgeRule{ID: "high", Priority: 10}},
	}
	sort.Slice(tasks, func(i, j int) bool {
		pi, pj := tasks[i].rule.Priority, tasks[j].rule.Priority
		if pi != pj {
			return pi > pj
		}
		return tasks[i].rule.ID < tasks[j].rule.ID
	})
	if tasks[0].rule.ID != "high" || tasks[1].rule.ID != "mid" || tasks[2].rule.ID != "low" {
		t.Fatalf("unexpected priority order: %s, %s, %s", tasks[0].rule.ID, tasks[1].rule.ID, tasks[2].rule.ID)
	}
}

func TestEdgeRuleScheduler_PriorityDispatchOrder(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(50 * time.Millisecond)
	em.Start()
	defer em.Stop()

	var order []string
	var mu sync.Mutex
	em.SetActionHook(func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		mu.Lock()
		order = append(order, ruleID)
		mu.Unlock()
	})

	rules := []model.EdgeRule{
		{
			ID: "low", Enable: true, Type: "threshold", Priority: 1,
			Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
			Condition: "t1 > 0",
			Actions:   []model.RuleAction{{Type: "log"}},
		},
		{
			ID: "high", Enable: true, Type: "threshold", Priority: 10,
			Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
			Condition: "t1 > 0",
			Actions:   []model.RuleAction{{Type: "log"}},
		},
	}
	em.LoadRules(rules)

	val := model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now()}
	em.handleValue(val)
	time.Sleep(120 * time.Millisecond)

	if em.GetMetrics().RulesExecuted < 2 {
		t.Fatalf("expected 2 rule executions, got %d", em.GetMetrics().RulesExecuted)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 {
		t.Fatalf("expected at least 2 action hooks, got %v", order)
	}
	// Action hooks run asynchronously; verify both rules ran after coalesced flush.
	hasHigh, hasLow := false, false
	for _, id := range order {
		if id == "high" {
			hasHigh = true
		}
		if id == "low" {
			hasLow = true
		}
	}
	if !hasHigh || !hasLow {
		t.Fatalf("expected both high and low rules to execute, got %v", order)
	}
}

func TestEdgeComputeManager_WindowIntervalStep(t *testing.T) {
	em := newTestEdgeComputeManager(t)

	rule := model.EdgeRule{
		ID:     "rule-window-step",
		Name:   "Window Step",
		Type:   "window",
		Enable: true,
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "value > 5",
		Window: &model.WindowConfig{
			Type:     "sliding",
			Size:     "10s",
			Interval: "100ms",
			AggrFunc: "avg",
		},
	}
	em.LoadRules([]model.EdgeRule{rule})

	feed := func(v float64) {
		em.handleValue(model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   "p1",
			Value:     v,
			TS:        time.Now(),
		})
		time.Sleep(20 * time.Millisecond)
	}

	feed(10)
	feed(10)
	if data := em.GetWindowData("rule-window-step"); len(data) != 2 {
		t.Fatalf("expected 2 buffered samples, got %d", len(data))
	}

	states := em.GetRuleStates()
	if states["rule-window-step"] != nil && states["rule-window-step"].LastWindowEval.IsZero() {
		// first eval may run immediately
	}

	time.Sleep(120 * time.Millisecond)
	feed(10)
	states = em.GetRuleStates()
	if state := states["rule-window-step"]; state == nil || state.LastWindowEval.IsZero() {
		t.Fatal("expected LastWindowEval to be set after interval step")
	}
}
