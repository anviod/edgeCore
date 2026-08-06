package core

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func benchEdgeRule(prefix string, pointID string) model.EdgeRule {
	return model.EdgeRule{
		ID:     prefix + "-" + pointID,
		Name:   prefix + " " + pointID,
		Type:   "threshold",
		Enable: true,
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: pointID},
		},
		Condition: "t1 > 0",
		Actions:   []model.RuleAction{{Type: "log"}},
		State:     &model.StateConfig{Duration: "0s", Count: 1},
	}
}

func feedValue(em *EdgeComputeManager, pointID string, value float64) {
	em.handleValue(model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   pointID,
		Value:     value,
		TS:        time.Now(),
	})
}

func waitExecutions(t *testing.T, em *EdgeComputeManager, min int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if em.GetMetrics().RulesExecuted >= min {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for >= %d executions, got %d", min, em.GetMetrics().RulesExecuted)
}

func TestEdgeComputeStress_HighFrequencyCoalesce(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped in -short mode")
	}

	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(100 * time.Millisecond)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{benchEdgeRule("coalesce", "p1")})

	start := time.Now()
	const updates = 1000
	for i := 0; i < updates; i++ {
		feedValue(em, "p1", float64(i+1))
	}
	time.Sleep(200 * time.Millisecond)
	waitExecutions(t, em, 1, 2*time.Second)

	metrics := em.GetMetrics()
	elapsed := time.Since(start)
	t.Logf("coalesce: updates=%d executed=%d coalesced=%d dropped=%d elapsed=%v pending=%d",
		updates, metrics.RulesExecuted, metrics.RulesCoalesced, metrics.RulesDropped, elapsed, metrics.PendingSchedulerTasks)

	if metrics.RulesExecuted > 5 {
		t.Fatalf("expected heavy coalescing (<=5 executions), got %d", metrics.RulesExecuted)
	}
	if metrics.RulesCoalesced < updates-5 {
		t.Fatalf("expected most updates coalesced, coalesced=%d updates=%d", metrics.RulesCoalesced, updates)
	}
}

func TestEdgeComputeStress_MultiRuleBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped in -short mode")
	}

	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(50 * time.Millisecond)
	em.Start()
	defer em.Stop()

	const ruleCount = 100
	rules := make([]model.EdgeRule, 0, ruleCount)
	for i := 0; i < ruleCount; i++ {
		r := benchEdgeRule("burst", "p1")
		r.ID = fmt.Sprintf("burst-%d", i)
		r.Priority = i % 10
		rules = append(rules, r)
	}
	em.LoadRules(rules)

	start := time.Now()
	for round := 0; round < 20; round++ {
		feedValue(em, "p1", float64(round+1))
	}
	time.Sleep(150 * time.Millisecond)
	waitExecutions(t, em, int64(ruleCount), 5*time.Second)

	metrics := em.GetMetrics()
	elapsed := time.Since(start)
	t.Logf("burst: rules=%d executed=%d coalesced=%d dropped=%d elapsed=%v queue_usage=%d/%d",
		ruleCount, metrics.RulesExecuted, metrics.RulesCoalesced, metrics.RulesDropped, elapsed,
		metrics.WorkerPoolUsage, metrics.WorkerPoolSize)

	if metrics.RulesDropped > 0 {
		t.Logf("warning: %d rules dropped under burst load", metrics.RulesDropped)
	}
}

func TestEdgeComputeStress_MemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped in -short mode")
	}

	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{
		{
			ID: "mem-window", Enable: true, Type: "window",
			Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
			Condition: "value > 0",
			Window:    &model.WindowConfig{Type: "sliding", Size: "10s", AggrFunc: "avg"},
		},
	})

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	const iterations = 5000
	for i := 0; i < iterations; i++ {
		feedValue(em, "p1", float64(i%100))
	}

	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	metrics := em.GetMetrics()
	windowData := em.GetWindowData("mem-window")
	t.Logf("memory: alloc_before=%dKB alloc_after=%dKB delta=%dKB window_samples=%d cache=%d minute_cache=%d",
		before.Alloc/1024, after.Alloc/1024, (after.Alloc-before.Alloc)/1024,
		len(windowData), metrics.CacheSize, metrics.MinuteCacheSize)

	if len(windowData) > maxEdgeWindowSamples {
		t.Fatalf("window buffer exceeded cap: %d > %d", len(windowData), maxEdgeWindowSamples)
	}
	if metrics.CacheSize > maxEdgeValueCacheSize {
		t.Fatalf("value cache exceeded cap: %d > %d", metrics.CacheSize, maxEdgeValueCacheSize)
	}
}

func TestEdgeComputeStress_LatencyPercentiles(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped in -short mode")
	}

	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	var latencies []int64
	var mu sync.Mutex
	var pending []time.Time
	em.SetActionHook(func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		mu.Lock()
		defer mu.Unlock()
		if len(pending) > 0 {
			start := pending[0]
			pending = pending[1:]
			latencies = append(latencies, time.Since(start).Milliseconds())
		}
	})

	em.LoadRules([]model.EdgeRule{benchEdgeRule("latency", "p1")})

	const samples = 100
	for i := 0; i < samples; i++ {
		mu.Lock()
		pending = append(pending, time.Now())
		mu.Unlock()
		feedValue(em, "p1", float64(i+1))
		time.Sleep(3 * time.Millisecond)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(latencies)
		mu.Unlock()
		if n >= samples {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	n := len(latencies)
	mu.Unlock()
	if n == 0 {
		t.Fatal("no latency samples collected")
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p99 := latencies[len(latencies)*99/100]
	t.Logf("latency: samples=%d p50=%dms p99=%dms max=%dms", len(latencies), p50, p99, latencies[len(latencies)-1])

	if p99 > 100 {
		t.Fatalf("p99 latency %dms exceeds 100ms SLA", p99)
	}
}

func TestEdgeComputeStress_QueueDropRecordsFailure(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(0)
	em.workerPool = make(chan *ruleTask, 1)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{benchEdgeRule("drop", "p1"), benchEdgeRule("drop", "p2")})

	block := make(chan struct{})
	em.SetActionHook(func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		<-block
	})

	for i := 0; i < 20; i++ {
		feedValue(em, "p1", 1)
		feedValue(em, "p2", 1)
	}
	close(block)
	time.Sleep(200 * time.Millisecond)

	failures := em.GetFailures("", 50)
	drops := em.GetMetrics().RulesDropped
	t.Logf("drops=%d failures=%d", drops, len(failures))
	if drops > 0 && len(failures) == 0 {
		t.Fatal("expected failure records when rules are dropped")
	}
}

func BenchmarkEdgeCompute_HandleValue(b *testing.B) {
	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(250 * time.Millisecond)
	em.Start()
	b.Cleanup(em.Stop)

	em.LoadRules([]model.EdgeRule{benchEdgeRule("bench", "p1")})
	val := model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		em.handleValue(val)
	}
}

func BenchmarkEdgeCompute_DebounceFlush(b *testing.B) {
	pipeline := NewDataPipeline(100)
	em := NewEdgeComputeManager(pipeline, nil, func(rules []model.EdgeRule) error { return nil })
	em.SetBatchWindow(10 * time.Millisecond)
	em.workerPool = make(chan *ruleTask, 10000)
	em.Start()
	b.Cleanup(em.Stop)

	em.LoadRules([]model.EdgeRule{benchEdgeRule("flush", "p1")})
	val := model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1.0, TS: time.Now()}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		em.handleValue(val)
	}
}
