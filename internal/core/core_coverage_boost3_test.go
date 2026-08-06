package core

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestEdgeComputeManager_ResolveValueTemplate(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	env := map[string]any{"name": "sensor", "val": 42}

	if got := em.resolveValueTemplate("${name}-${val}", env); got != "sensor-42" {
		t.Fatalf("template = %v", got)
	}
	if got := em.resolveValueTemplate(99, env); got != 99 {
		t.Fatalf("non-string = %v", got)
	}
	if got := em.resolveValueTemplate("plain", env); got != "plain" {
		t.Fatalf("plain string = %v", got)
	}
}

func TestEdgeComputeManager_CalculateRMW(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	em.SetDeviceWriter(&MockDeviceIO{
		ReadFunc: func(_, _, _ string) (model.Value, error) {
			return model.Value{Value: int64(0b1010)}, nil
		},
	})

	newVal, err := em.calculateRMW("ch1", "dev1", "p1", 1, int64(1), "bit set")
	if err != nil {
		t.Fatalf("calculateRMW set: %v", err)
	}
	if newVal.(int64) != 0b1010|0b10 {
		t.Fatalf("set bit 1 = %b", newVal.(int64))
	}

	newVal, err = em.calculateRMW("ch1", "dev1", "p1", 1, int64(0), "bit clear")
	if err != nil {
		t.Fatalf("calculateRMW clear: %v", err)
	}
	if newVal.(int64) != 0b1000 {
		t.Fatalf("clear bit 1 = %b", newVal.(int64))
	}
}

func TestEdgeComputeManager_SaveFailedAction(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(filepath.Join(tmpDir, "failed-actions"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	em := NewEdgeComputeManager(nil, store, nil)
	em.saveFailedAction("rule1", model.RuleAction{Type: "mqtt", Config: map[string]any{"topic": "t"}}, model.Value{}, nil, "broker down")

	if actions := em.GetFailedActions(); len(actions) != 1 {
		t.Fatalf("GetFailedActions = %d, want 1", len(actions))
	}
}

func TestScanEngineAdapter_AdapterMethods(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: time.Hour})
	adapter := NewScanEngineAdapter(se)
	ch := &model.Channel{ID: "ch1", Protocol: "modbus-tcp"}
	dev := &model.Device{
		ID: "dev-adapt", Enable: true, Interval: model.Duration(time.Second),
		Points: []model.Point{{ID: "p1", ScanClass: "fast", Address: "40001", DataType: "int16"}},
		Config: map[string]any{"slave_id": 1},
	}
	if err := adapter.RegisterDevice("dev-adapt", "modbus-tcp", &execStubDriver{}, &sync.Mutex{}, ch, dev, time.Second, 3); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}

	adapter.UpdateDeviceInterval("dev-adapt", 2*time.Second)
	adapter.UpdateDevicePriority("dev-adapt", 8)
	adapter.UpdateDeviceDriverConfig("dev-adapt", map[string]any{"timeout": 3})
	if adapter.GetTaskStatus("dev-adapt") == ScanTaskStatusStopped {
		t.Fatal("registered device should have active task status")
	}
	if adapter.GetPendingTaskCount() < 0 {
		t.Fatal("pending count should be non-negative")
	}

	adapter.UnregisterDevice("dev-adapt")
	if adapter.GetTaskStatus("dev-adapt") != ScanTaskStatusStopped {
		t.Fatal("unregistered device should be stopped")
	}
}

func TestPipeline_SetShadowIngressIngest(t *testing.T) {
	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, time.Millisecond)
	si.Start()
	defer si.Stop()

	dp := NewDataPipeline(10)
	dp.SetShadowIngress(si)
	dp.Start()

	var ingested atomic.Int32
	dp.AddHandler(func(v model.Value) { ingested.Add(1) })

	dp.Push(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1, TS: time.Now()})
	time.Sleep(100 * time.Millisecond)

	if ingested.Load() == 0 {
		t.Fatal("handler should receive pushed value")
	}
}

func TestCommunicationManageTemplate_MarkOfflineCallback(t *testing.T) {
	mgr := NewCommunicationManageTemplate()
	node := mgr.RegisterNode("dev-off", "Offline Dev")
	var called bool
	mgr.OnStateChange = func(_ string, _, _ NodeState) { called = true }

	mgr.MarkOffline("dev-off")
	if node.Runtime.State != NodeStateOffline {
		t.Fatalf("state = %v", node.Runtime.State)
	}
	if !called {
		t.Fatal("OnStateChange should fire on MarkOffline")
	}
}

func TestEdgeComputeManager_ExecuteHttpWithoutNorthbound(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	err := em.executeHttp(context.Background(), "rule1", model.RuleAction{
		Type:   "http",
		Config: map[string]any{"url": "http://127.0.0.1"},
	}, model.Value{}, nil)
	if err == nil {
		t.Fatal("expected error without northbound manager")
	}
}

func TestScanEngine_RunAndStop(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: 50 * time.Millisecond})
	se.AddTask("dev-run", "modbus-tcp", time.Second, 1, []string{"p1"}, nil)
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	el.RegisterDriver("dev-run", &execStubDriver{})
	se.executionLayer = el

	go se.Run()
	time.Sleep(80 * time.Millisecond)
	se.Stop()
	if se.IsRunning() {
		t.Fatal("engine should stop after Stop()")
	}
}

func TestNorthboundManager_UpdateMQTTClientsDisabled(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	nm.mu.Lock()
	nm.updateMQTTClients(
		[]model.MQTTConfig{{ID: "old", Name: "Old", Enable: true, Broker: "tcp://127.0.0.1:1883"}},
		[]model.MQTTConfig{{ID: "new", Name: "New", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	)
	nm.updateHTTPClients(nil, []model.HTTPConfig{{ID: "h1", Name: "H", Enable: false, URL: "http://127.0.0.1"}})
	nm.mu.Unlock()
}

func TestStoreForwardManager_SetStorage(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	mgr := NewStoreForwardManager(nil, StoreForwardPolicy{})
	mgr.SetStorage(store)
	mgr.HandleBatch([]model.Value{{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1, Quality: "Good"}})

	count := 0
	_ = store.LoadAll(storage.BucketDataCache, func(k, _ []byte) error {
		if len(k) > len(storeForwardSouthKey) && string(k[:len(storeForwardSouthKey)]) == storeForwardSouthKey {
			count++
		}
		return nil
	})
	if count != 1 {
		t.Fatalf("stored records = %d, want 1", count)
	}
}

func TestScanEngineMetrics_FullSnapshotAndSLA(t *testing.T) {
	m := &ScanEngineMetrics{lagSamples: make([]int64, 0, 8)}
	m.RecordExecute(true, 150_000)
	m.RecordExecute(false, 200_000)
	m.RecordStarvationRescue()
	m.RecordOverdue()
	m.RecordMissDeadline()
	m.RecordDrift(60_000)
	m.RecordIntervalAdjusted()
	m.SetAdaptiveSlowdownFactor(0.5)
	m.ResetWindow()

	m.RecordExecute(true, 250_000)
	snap := m.Snapshot()
	if snap["tasks_executed"].(uint64) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}

	cb := NewDriverCircuitBreaker()
	warnings := m.SLAWarnings(cb)
	if len(warnings) == 0 {
		t.Fatal("expected SLA warnings for high lag")
	}
}

func TestResourceController_GetConnectionCountAndMonitor(t *testing.T) {
	rc := NewResourceController(ResourceLimits{GoroutineLimit: 100, ConnectionLimit: 2})
	rc.AcquireConnection()
	if rc.GetConnectionCount() != 1 {
		t.Fatalf("connection count = %d", rc.GetConnectionCount())
	}
	rc.ReleaseConnection()

	var wg sync.WaitGroup
	wg.Add(1)
	go rc.Monitor(&wg)
	rc.Stop()
	wg.Wait()
}

func TestEdgeComputeManager_LegacySourceIndex(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	em.LoadRules([]model.EdgeRule{{
		ID: "legacy", Enable: true, Type: "threshold", Condition: "v > 0",
		Source: model.RuleSource{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Alias: "v"},
	}})
	if !matchRule(em.rules["legacy"], model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1}) {
		t.Fatal("legacy rule should match")
	}
}

func TestNorthboundManager_RebuildOPCUAServersEmpty(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	nm.RebuildOPCUAServers()
}

func TestChannelManager_GetChannelStats(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	stats := cm.GetChannelStats()
	if len(stats) != 1 || stats[0].ID != channelID {
		t.Fatalf("GetChannelStats = %+v", stats)
	}
}

func TestEdgeComputeManager_ExecuteHttpEmptyURL(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	if err := em.executeHttp(context.Background(), "r1", model.RuleAction{
		Type:   "http",
		Config: map[string]any{"url": ""},
	}, model.Value{}, nil); err != nil {
		t.Fatalf("empty url should noop: %v", err)
	}
}

func TestToFloat_AllTypes(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{float64(1.5), 1.5, true},
		{float32(2.5), 2.5, true},
		{int(3), 3, true},
		{int64(4), 4, true},
		{uint(5), 5, true},
		{"6.5", 6.5, true},
		{true, 1, true},
		{false, 0, true},
		{"bad", 0, false},
		{struct{}{}, 0, false},
	}
	for _, tc := range cases {
		got, ok := toFloat(tc.in)
		if ok != tc.ok || (tc.ok && got != tc.want) {
			t.Fatalf("toFloat(%v) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBitwiseUnary(t *testing.T) {
	got, err := bitwiseUnary(int64(0), func(x int64) int64 { return ^x })
	if err != nil || got != -1 {
		t.Fatalf("bitwiseUnary = (%v, %v)", got, err)
	}
}

func TestEdgeComputeManager_StateRuleOnChange(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, nil)
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{{
		ID: "state-on-change", Type: "state", Enable: true, TriggerMode: "on_change",
		Condition: "t1 > 0",
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		State:     &model.StateConfig{Duration: "1ms", Count: 1},
		Actions:   []model.RuleAction{{Type: "log"}},
	}})

	em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 1, TS: time.Now()})
	time.Sleep(80 * time.Millisecond)
	if states := em.GetRuleStates(); len(states) == 0 {
		t.Fatal("expected rule state")
	}
}

func TestNorthboundManager_PublishMQTTNotFound(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)
	if err := nm.PublishMQTT("missing", "topic", []byte("x")); err == nil {
		t.Fatal("expected publish error")
	}
}
