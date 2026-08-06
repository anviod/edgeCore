package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

func TestTaskCollectPointIDs(t *testing.T) {
	withPoints := &ScanTask{
		Points: []model.Point{{ID: "a"}, {ID: "b"}},
	}
	ids := taskCollectPointIDs(withPoints)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("from Points = %v", ids)
	}

	withIDs := &ScanTask{PointIDs: []string{"x", "y"}}
	ids = taskCollectPointIDs(withIDs)
	if len(ids) != 2 || ids[0] != "x" {
		t.Fatalf("from PointIDs = %v", ids)
	}
}

func TestTaskShadowChannelID(t *testing.T) {
	task := &ScanTask{Params: map[string]any{"channelID": "ch-42"}}
	if got := taskShadowChannelID(task); got != "ch-42" {
		t.Fatalf("taskShadowChannelID = %q, want ch-42", got)
	}
	if got := taskShadowChannelID(&ScanTask{}); got != "" {
		t.Fatalf("empty params = %q, want empty", got)
	}
}

func TestResolveCollectQuality(t *testing.T) {
	if q := resolveCollectQuality(model.Value{Quality: "Uncertain"}); q != "Uncertain" {
		t.Fatalf("explicit quality = %q", q)
	}
	if q := resolveCollectQuality(model.Value{Value: nil}); q != "Bad" {
		t.Fatalf("nil value = %q, want Bad", q)
	}
	if q := resolveCollectQuality(model.Value{Value: 1}); q != "Good" {
		t.Fatalf("non-nil value = %q, want Good", q)
	}
}

func TestPreservedShadowValue(t *testing.T) {
	existing := &model.ShadowDevice{
		Points: map[string]model.ShadowPoint{
			"p1": {Value: 99},
		},
	}
	if v := preservedShadowValue(existing, "p1", 42, "Good"); v != 42 {
		t.Fatalf("new Good value = %v, want 42", v)
	}
	if v := preservedShadowValue(existing, "p1", nil, "Good"); v != nil {
		t.Fatalf("Good nil = %v, want nil", v)
	}
	if v := preservedShadowValue(existing, "p1", nil, "Bad"); v != 99 {
		t.Fatalf("Bad nil should preserve stale = %v, want 99", v)
	}
	if v := preservedShadowValue(nil, "p1", nil, "Bad"); v != nil {
		t.Fatalf("no existing Bad nil = %v, want nil", v)
	}
}

func TestScanPointSlicePool(t *testing.T) {
	raw := borrowPointSlice(8)
	*raw = append(*raw, model.Point{ID: "p1"}, model.Point{ID: "p2"})
	if len(*raw) != 2 {
		t.Fatalf("borrowed slice len = %d", len(*raw))
	}
	returnPointSlice(raw)

	raw2 := borrowPointSlice(4)
	if cap(*raw2) < 4 {
		t.Fatalf("reused slice cap = %d, want ≥ 4", cap(*raw2))
	}
	returnPointSlice(nil)

	large := borrowPointSlice(600)
	for i := 0; i < 600; i++ {
		*large = append(*large, model.Point{ID: "x"})
	}
	returnPointSlice(large)

	sraw := borrowShadowIngressPointSlice(4)
	*sraw = append(*sraw, model.ShadowIngressPoint{PointID: "p"})
	returnShadowIngressPointSlice(sraw)
	returnShadowIngressPointSlice(nil)
}

func TestCircuitStateString(t *testing.T) {
	cases := map[CircuitState]string{
		CircuitClosed:    "Closed",
		CircuitOpen:      "Open",
		CircuitHalfOpen:  "HalfOpen",
		CircuitState(99): "Unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Fatalf("CircuitState(%d).String() = %q, want %q", state, got, want)
		}
	}
}

func TestConnStateString(t *testing.T) {
	states := []ConnState{
		ConnStateDisconnected, ConnStateConnecting, ConnStateConnected,
		ConnStateRetrying, ConnStateDead, ConnStateHealthy, ConnStateDegraded,
	}
	for _, s := range states {
		if s.String() == "" || s.String() == "Unknown" {
			t.Fatalf("ConnState(%d).String() = %q", s, s.String())
		}
	}
	if ConnState(99).String() != "Unknown" {
		t.Fatal("unknown ConnState should return Unknown")
	}
}

func TestScanEngine_RemoveTasksByDeviceKey(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: 10 * time.Millisecond})
	se.AddTask("dev1", "modbus-tcp", time.Second, 5, []string{"p1"}, nil)
	se.AddTask("dev2", "modbus-tcp", time.Second, 5, []string{"p2"}, nil)

	se.RemoveTasksByDeviceKey("dev1")
	if se.GetTaskByDeviceKey("dev1") != nil {
		t.Fatal("dev1 task should be removed")
	}
	if se.GetTaskByDeviceKey("dev2") == nil {
		t.Fatal("dev2 task should remain")
	}
}

func TestExecutionLayer_FilterAndRecordPoints(t *testing.T) {
	el := NewExecutionLayer()
	pd := NewPointDegradationManager()
	el.SetPointDegradation(pd)

	task := &ScanTask{DeviceKey: "dev1"}
	points := []model.Point{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}

	for i := 0; i < pointDegradeThreshold; i++ {
		pd.RecordResults("dev1", map[string]string{"p2": "Bad"})
	}

	filtered := el.filterPoints(task, points)
	if len(filtered) != 2 {
		t.Fatalf("filterPoints = %d points, want 2 (p2 degraded)", len(filtered))
	}

	el.recordPointResults(task, map[string]model.Value{
		"p1": {Quality: "Good"},
		"p3": {Quality: "Bad"},
	})

	el.recordPointResults(task, nil)
	el.filterPoints(task, nil)
}

func TestDeepCloneValue(t *testing.T) {
	src := map[string]any{
		"a": 1,
		"b": []any{float64(2), map[string]any{"c": 3}},
	}
	cloned := deepCloneValue(src).(map[string]any)
	cloned["a"] = 99
	if src["a"] != 1 {
		t.Fatal("deepCloneValue should not alias nested maps")
	}
	if deepCloneValue(nil) != nil {
		t.Fatal("nil clone should be nil")
	}
}

func TestCircuitBreakerEventEmit(t *testing.T) {
	var called bool
	e := circuitBreakerEvent{
		fn:        func(_, _, _ string) { called = true },
		deviceKey: "dev",
		eventType: "open",
		message:   "test",
	}
	e.emit()
	if !called {
		t.Fatal("emit should invoke handler")
	}
	circuitBreakerEvent{}.emit()
}

func TestValidatePointProtocols(t *testing.T) {
	cm := newTestChannelManager()

	cases := []struct {
		protocol string
		point    model.Point
		wantErr  bool
	}{
		{"bacnet-ip", model.Point{Address: "AnalogInput:1"}, false},
		{"bacnet-ip", model.Point{Address: "invalid"}, true},
		{"bacnet-ip", model.Point{Address: "BadType:1"}, true},
		{"s7", model.Point{Address: "DB1.DBD0"}, false},
		{"s7", model.Point{Address: ""}, true},
		{"dlt645", model.Point{Address: "123456789012#00010000"}, false},
		{"dlt645", model.Point{Address: "bad"}, true},
		{"ethernet-ip", model.Point{Address: "Tag1", DataType: "int16"}, false},
		{"mitsubishi-slmp", model.Point{Address: "D0", DataType: "int16"}, false},
		{"omron-fins", model.Point{Address: "D100"}, false},
		{"knxnet-ip", model.Point{Address: "1/2/3"}, false},
		{"profinet-io", model.Point{Address: "3:1:0"}, false},
		{"ethercat", model.Point{Address: "1:Tx:0"}, false},
		{"ethercat", model.Point{Address: "1:SDO:0x6041:0"}, false},
		{"ethercat", model.Point{Address: "invalid"}, true},
		{"modbus-tcp", model.Point{Address: "40001", DataType: "int16"}, false},
	}
	for _, tc := range cases {
		ch := &model.Channel{Protocol: tc.protocol}
		err := cm.validatePoint(ch, &tc.point)
		if tc.wantErr && err == nil {
			t.Fatalf("%s point %+v should fail validation", tc.protocol, tc.point)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%s point %+v validation failed: %v", tc.protocol, tc.point, err)
		}
	}
}

func TestRegisterProtocolToScanEngine(t *testing.T) {
	cm := newTestChannelManager()
	protocols := []string{
		"modbus-tcp", "opc-ua", "s7", "bacnet-ip", "custom-proto",
	}
	for _, p := range protocols {
		cm.registerProtocolToScanEngine(p)
	}
}

func TestVirtualShadowHelpers(t *testing.T) {
	if !IsVirtualShadowID("virtual-dev1") {
		t.Fatal("virtual prefix should be detected")
	}
	if IsVirtualShadowID("shadow-dev1") {
		t.Fatal("non-virtual id should not match")
	}
	if got := VirtualShadowID("dev1"); got != "virtual-dev1" {
		t.Fatalf("VirtualShadowID = %q", got)
	}

	dev, pt := parseDepRef("ch1.dev1.temp")
	if dev != "dev1" || pt != "temp" {
		t.Fatalf("parseDepRef dotted = (%s, %s)", dev, pt)
	}
	dev, pt = parseDepRef("dev1.temp")
	if dev != "dev1" || pt != "temp" {
		t.Fatalf("parseDepRef simple = (%s, %s)", dev, pt)
	}
	if key := depToEnvKey("ch1.dev1.temp"); key == "ch1.dev1.temp" {
		t.Fatal("depToEnvKey should replace separators")
	}
	rewritten := rewriteFormulaDeps("ch1.dev1.temp + 1", []string{"ch1.dev1.temp"})
	if rewritten == "ch1.dev1.temp + 1" {
		t.Fatal("rewriteFormulaDeps should substitute deps")
	}
}

func TestShadowNotifyPool(t *testing.T) {
	var called atomic.Int32
	handler := func(_ string, _ map[string]model.ShadowPoint) {
		called.Add(1)
	}

	pool := newShadowNotifyPool(2, handler)
	if pool.WorkerCount() != 2 {
		t.Fatalf("WorkerCount = %d, want 2", pool.WorkerCount())
	}

	pool.Enqueue("dev1", map[string]model.ShadowPoint{"p1": {Value: 1}})
	time.Sleep(30 * time.Millisecond)
	if called.Load() != 1 {
		t.Fatalf("pre-start inline enqueue = %d, want 1", called.Load())
	}

	pool.Start()
	pool.Enqueue("dev1", map[string]model.ShadowPoint{"p2": {Value: 2}})
	time.Sleep(50 * time.Millisecond)
	pool.Stop()

	if called.Load() < 2 {
		t.Fatalf("handler calls = %d, want ≥ 2", called.Load())
	}
}

func TestScanEngineAdapter_RegisterDevice(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: 10 * time.Millisecond})
	adapter := NewScanEngineAdapter(se)

	degradeFalse := false
	ch := &model.Channel{ID: "ch1", Protocol: "modbus-tcp", Config: map[string]any{}}
	dev := &model.Device{
		ID: "dev1",
		Points: []model.Point{
			{ID: "p1", ScanClass: "fast"},
			{ID: "p2", ScanClass: "slow"},
		},
		Config:           map[string]any{"slave_id": 1, "degrade_on_failure": false},
		DegradeOnFailure: &degradeFalse,
	}

	err := adapter.RegisterDevice("dev1", "modbus-tcp", &stubChannelDriver{}, &sync.Mutex{}, ch, dev, time.Second, 5)
	if err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if len(adapter.GetTasks()) == 0 {
		t.Fatal("expected tasks after register")
	}

	adapter.UnregisterDevice("dev1")
	if len(adapter.GetTasks()) != 0 {
		t.Fatalf("tasks after unregister = %d, want 0", len(adapter.GetTasks()))
	}

	if err := adapter.RegisterDevice("dev-nil", "modbus-tcp", nil, nil, ch, dev, time.Second, 5); err != nil {
		t.Fatalf("nil driver register: %v", err)
	}
}

func TestScanEngineAdapter_ResetDeviceCollection(t *testing.T) {
	resetter := &resettingStubDriver{}
	adapter := &ScanEngineAdapter{
		driverManager: map[string]driver.Driver{
			"dev1": resetter,
		},
	}
	adapter.resetDeviceCollection("dev1")
	if !resetter.resetCalled {
		t.Fatal("ResetDeviceCollection should be invoked")
	}
	adapter.resetDeviceCollection("missing")
}

type resettingStubDriver struct {
	stubChannelDriver
	resetCalled bool
}

func (r *resettingStubDriver) ResetDeviceCollection(deviceID string) {
	r.resetCalled = true
}

func TestScanEngine_UpdateTaskStateAggregated(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev1", "modbus-tcp", time.Second, 5, []string{"p1"}, nil)
	task.BaseInterval = time.Second

	se.updateTaskStateAggregated(task, AggregatedStats{SuccessCount: 2, FailCount: 0})
	if task.ConsecutiveSuccess != 2 {
		t.Fatalf("ConsecutiveSuccess = %d, want 2", task.ConsecutiveSuccess)
	}

	se.updateTaskStateAggregated(task, AggregatedStats{FailCount: 3, FailRate: 0.5})
	if task.ConsecutiveFailures < 3 {
		t.Fatalf("ConsecutiveFailures = %d, want ≥ 3", task.ConsecutiveFailures)
	}
}

func TestScanEngine_ApplyAggregatedFeedback(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev-feed", "modbus-tcp", time.Second, 5, []string{"p1"}, nil)

	se.feedbackPendingMu.Lock()
	se.feedbackPending["dev-feed"] = task
	se.feedbackPendingMu.Unlock()

	se.applyAggregatedFeedback("dev-feed", AggregatedStats{SuccessCount: 1, FailCount: 0})
	if task.ConsecutiveSuccess != 1 {
		t.Fatalf("feedback apply ConsecutiveSuccess = %d, want 1", task.ConsecutiveSuccess)
	}
	se.applyAggregatedFeedback("missing", AggregatedStats{SuccessCount: 1})
}

func TestValidateDeviceInterval(t *testing.T) {
	cm := newTestChannelManager()

	if _, ok := cm.validateDeviceInterval(nil); ok {
		t.Fatal("nil device should fail")
	}
	if _, ok := cm.validateDeviceInterval(&model.Device{Name: "", Interval: model.Duration(time.Second)}); ok {
		t.Fatal("empty name should fail")
	}
	if _, ok := cm.validateDeviceInterval(&model.Device{Name: "d", Interval: 0}); ok {
		t.Fatal("zero interval should fail")
	}
	dur, ok := cm.validateDeviceInterval(&model.Device{Name: "d", Interval: model.Duration(500 * time.Millisecond)})
	if !ok || dur != 500*time.Millisecond {
		t.Fatalf("valid interval = (%v, %v)", dur, ok)
	}
}

func TestCoerceConfigInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{int(7), 7, true},
		{int64(8), 8, true},
		{float64(9), 9, true},
		{float32(10), 10, true},
		{"x", 0, false},
	}
	for _, tc := range cases {
		got, ok := coerceConfigInt(tc.in)
		if ok != tc.ok || (tc.ok && got != tc.want) {
			t.Fatalf("coerceConfigInt(%v) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBuildDriverDeviceConfig(t *testing.T) {
	ch := &model.Channel{
		Protocol: "opc-ua",
		Config:   map[string]any{"endpoint": "opc.tcp://127.0.0.1:4840"},
	}
	cfg := buildDriverDeviceConfig(ch, map[string]any{"node_id": "ns=2;s=Tag"}, map[string]any{"timeout": 5})
	if cfg["node_id"] == nil {
		t.Fatal("device config should be preserved")
	}
	if cfg["timeout"] == nil {
		t.Fatal("extra config should be merged")
	}

	modbusCfg := buildDriverDeviceConfig(&model.Channel{Protocol: "modbus-tcp"}, map[string]any{"slave_id": 1}, nil)
	if modbusCfg["slave_id"] != 1 {
		t.Fatalf("modbus cfg = %v", modbusCfg)
	}
}

func TestScanEngineMetrics_RecordLagSampleCap(t *testing.T) {
	m := &ScanEngineMetrics{lagSamples: make([]int64, 0, scanLagSampleCap)}
	for i := 0; i < scanLagSampleCap+5; i++ {
		m.recordLagSample(int64(i))
	}
	if len(m.lagSamples) != scanLagSampleCap {
		t.Fatalf("lagSamples len = %d, want %d", len(m.lagSamples), scanLagSampleCap)
	}
	if m.lagSamples[len(m.lagSamples)-1] != int64(scanLagSampleCap+4) {
		t.Fatalf("last lag sample = %d", m.lagSamples[len(m.lagSamples)-1])
	}

	m.SetAdaptiveSlowdownFactor(0.5)
	if m.AdaptiveSlowdownFactor() != 0.5 {
		t.Fatalf("adaptive factor = %v", m.AdaptiveSlowdownFactor())
	}
	m.TasksExecuted.Store(10)
	m.TasksFailed.Store(2)
	if rate := m.GlobalFailRate(); rate != 0.2 {
		t.Fatalf("GlobalFailRate = %v, want 0.2", rate)
	}
}

func TestChannelManager_MinPointSuccessRate(t *testing.T) {
	mc := model.NewMetricsCollector()
	model.SetGlobalMetricsCollector(mc)
	t.Cleanup(func() { model.SetGlobalMetricsCollector(nil) })

	cm := newTestChannelManager()
	mc.RecordRequest("ch-1", true, 0, "")
	mc.RecordRequest("ch-1", false, 0, "timeout")

	rate, label := cm.minChannelPointSuccessRate()
	if rate <= 0 || rate > 1 {
		t.Fatalf("min rate = %v", rate)
	}
	if label == "" {
		t.Fatal("expected channel label")
	}
}

func TestResourceController_ConnectionLimit(t *testing.T) {
	rc := NewResourceController(ResourceLimits{GoroutineLimit: 10, ConnectionLimit: 1})
	if !rc.AcquireConnection() {
		t.Fatal("first connection should succeed")
	}
	if rc.AcquireConnection() {
		t.Fatal("second connection should be rejected")
	}
	rc.ReleaseConnection()
	if !rc.AcquireConnection() {
		t.Fatal("connection after release should succeed")
	}
	rc.ReleaseConnection()
}

func TestScanEngine_GetTaskHelpers(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev-x", "modbus-tcp", time.Second, 3, []string{"p1"}, nil)
	if se.GetTask(task.ID) == nil {
		t.Fatal("GetTask should return registered task")
	}
	if got := se.GetTaskByDeviceKey("dev-x"); got == nil || got.ID != task.ID {
		t.Fatal("GetTaskByDeviceKey mismatch")
	}
	se.RegisterProtocol("custom", ProtocolTypeSerial)
	se.UnregisterDriver("dev-x")
}

func TestScanEngine_RemoveTask(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev-rm", "modbus-tcp", time.Second, 5, []string{"p1"}, nil)
	se.RemoveTask(task.ID)
	if se.GetTask(task.ID) != nil {
		t.Fatal("RemoveTask should delete task")
	}
	se.RemoveTask("missing-id")
}

func TestAdaptiveThrottle_UpdateDeviceRTT(t *testing.T) {
	at := NewAdaptiveThrottle(nil)
	at.UpdateDeviceRTT("dev1", 500)
	if eff := at.effectiveIntervalForDevice("dev1", 100*time.Millisecond); eff < 100*time.Millisecond {
		t.Fatalf("effective interval = %v", eff)
	}
	var nilAt *AdaptiveThrottle
	nilAt.UpdateDeviceRTT("dev1", 100)
}

func TestGetDeviceID(t *testing.T) {
	id, ok := getDeviceID(map[string]any{"bacnet_device_id": float64(42)})
	if !ok || id != 42 {
		t.Fatalf("getDeviceID = (%d, %v)", id, ok)
	}
	if _, ok := getDeviceID(nil); ok {
		t.Fatal("nil config should miss")
	}
}

func TestScanEngineAdapter_GetDriverAndUpdateInterval(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: 10 * time.Millisecond})
	adapter := NewScanEngineAdapter(se)
	if adapter.IsStarted() {
		t.Fatal("adapter should not be started initially")
	}

	drv := &stubChannelDriver{}
	ch := &model.Channel{ID: "ch1", Protocol: "modbus-tcp"}
	dev := &model.Device{
		ID:     "dev1",
		Points: []model.Point{{ID: "p1"}},
	}
	if err := adapter.RegisterDevice("dev1", "modbus-tcp", drv, &sync.Mutex{}, ch, dev, time.Second, 5); err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	if adapter.GetDriver("dev1") != drv {
		t.Fatal("GetDriver should return registered driver")
	}
	adapter.UpdateDeviceInterval("dev1", 2*time.Second)
	adapter.Stop()
	if adapter.GetPendingTaskCount() < 0 {
		t.Fatal("pending count should be non-negative")
	}
}

func TestNewFeedbackAggregator_DefaultOnFlush(t *testing.T) {
	fa := NewFeedbackAggregator(0, nil)
	if fa.Window() != 2*time.Second {
		t.Fatalf("default window = %v", fa.Window())
	}
}
