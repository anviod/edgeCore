package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestProtocolAdapters_AdjustValidateDefaults(t *testing.T) {
	cases := []struct {
		name    string
		adapter ProtocolAdapter
		wantKey string
		wantVal any
	}{
		{"modbus", NewModbusProtocolAdapter(), "batch_size", 100},
		{"tcp", NewTCPProtocolAdapter(), "buffer_size", 4096},
		{"bacnet", NewBACnetProtocolAdapter(), "apdu_timeout", 2000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{"timeout": 9999}
			adjusted := tc.adapter.AdjustParameters("dev-1", params)
			if adjusted[tc.wantKey] != tc.wantVal {
				t.Fatalf("AdjustParameters[%q] = %v, want %v", tc.wantKey, adjusted[tc.wantKey], tc.wantVal)
			}
			if err := tc.adapter.ValidateParameters(adjusted); err != nil {
				t.Fatalf("ValidateParameters: %v", err)
			}
			defaults := tc.adapter.GetDefaultParameters()
			if defaults[tc.wantKey] != tc.wantVal {
				t.Fatalf("GetDefaultParameters[%q] = %v, want %v", tc.wantKey, defaults[tc.wantKey], tc.wantVal)
			}
		})
	}
}

func TestVirtualShadowManager_CRUDAndRuntime(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	vse := NewVirtualShadowEngine(sc)
	cm := NewChannelManager(nil, nil)
	cm.channels["ch1"] = &model.Channel{
		ID:   "ch1",
		Name: "Ch1",
		Devices: []model.Device{{
			ID: "dev1", Name: "Dev1",
			Points: []model.Point{{ID: "temp", Name: "Temp"}},
		}},
	}

	var saved []model.VirtualShadowDeviceConfig
	mgr := NewVirtualShadowManager(vse, cm, sc, func(cfgs []model.VirtualShadowDeviceConfig) error {
		saved = append([]model.VirtualShadowDeviceConfig(nil), cfgs...)
		return nil
	})

	disabled := model.VirtualShadowDeviceConfig{
		ID:     "virt-a",
		Enable: false,
		Points: []model.VirtualShadowPointDef{
			{PointID: "out1", Mode: "formula", Formula: "1 + 1"},
		},
	}
	if err := mgr.Create(disabled); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("persist calls = %d, want 1", len(saved))
	}

	mgr.Load([]model.VirtualShadowDeviceConfig{disabled})
	if len(mgr.List()) != 1 {
		t.Fatalf("List after Load = %d", len(mgr.List()))
	}

	got, err := mgr.Get("virt-a")
	if err != nil || got.ID != "virt-a" {
		t.Fatalf("Get: (%+v, %v)", got, err)
	}

	enabled := model.VirtualShadowDeviceConfig{
		ID:     "virt-b",
		Enable: true,
		Points: []model.VirtualShadowPointDef{
			{PointID: "mapped", Mode: "map", SourceRef: "ch1.dev1.temp"},
		},
	}
	if err := mgr.Create(enabled); err != nil {
		t.Fatalf("Create enabled: %v", err)
	}

	sources := mgr.ListPointSources()
	if len(sources) != 1 || sources[0].Ref != "ch1.dev1.temp" {
		t.Fatalf("ListPointSources = %+v", sources)
	}

	if mgr.Engine() != vse {
		t.Fatal("Engine should return underlying VSE")
	}

	time.Sleep(30 * time.Millisecond)
	if _, _, err := mgr.GetRuntime("virt-b"); err != nil {
		t.Fatalf("GetRuntime enabled device: %v", err)
	}

	if err := mgr.Update("virt-b", model.VirtualShadowDeviceConfig{
		Enable: false,
		Points: []model.VirtualShadowPointDef{
			{PointID: "mapped", Mode: "map", SourceRef: "ch1.dev1.temp"},
		},
	}); err != nil {
		t.Fatalf("Update disable: %v", err)
	}
	if _, err := vse.GetVirtualDevice("virt-b"); err == nil {
		t.Fatal("disabled device should be removed from engine")
	}

	if _, _, err := mgr.RefreshRuntime("virt-a"); err == nil {
		t.Log("RefreshRuntime for disabled-only config may succeed if shadow exists")
	}

	if err := mgr.Delete("virt-a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("List after delete = %d", len(mgr.List()))
	}
	if err := mgr.Create(enabled); err == nil {
		t.Fatal("expected duplicate create error for virt-b")
	}
}

func TestCollectContext_MarkCounters(t *testing.T) {
	ctx := &CollectContext{}
	ctx.MarkSuccess()
	ctx.MarkFail()
	ctx.MarkSuccess()
	if ctx.SuccessCmd != 2 || ctx.FailCmd != 1 {
		t.Fatalf("counters = success %d fail %d", ctx.SuccessCmd, ctx.FailCmd)
	}
}

func TestDriverCircuitBreaker_Reset(t *testing.T) {
	cb := NewDriverCircuitBreaker()
	key := "dev-reset"
	for i := 0; i < circuitBreakerConsecutiveTimeoutThreshold; i++ {
		cb.Record(key, false, true)
	}
	if cb.State(key) != CircuitOpen {
		t.Fatal("expected open circuit before reset")
	}
	cb.Reset(key)
	if cb.State(key) != CircuitClosed {
		t.Fatalf("after Reset state = %v", cb.State(key))
	}
}

func TestPointDegradationManager_SnapshotDevice(t *testing.T) {
	pd := NewPointDegradationManager()
	for i := 0; i < pointDegradeThreshold; i++ {
		pd.RecordResults("dev1", map[string]string{"p1": "Bad"})
	}

	snap := pd.SnapshotDevice("dev1", []string{"p1", "p2"})
	if len(snap) != 1 {
		t.Fatalf("snapshot keys = %d, want 1", len(snap))
	}
	entry := snap["p1"].(map[string]any)
	if entry["degraded"] != true {
		t.Fatalf("p1 degraded = %v", entry["degraded"])
	}

	var nilPD *PointDegradationManager
	if len(nilPD.SnapshotDevice("dev1", []string{"p1"})) != 0 {
		t.Fatal("nil manager should return empty snapshot")
	}
}

func TestScanTask_UpdateNextRun(t *testing.T) {
	task := &ScanTask{ID: "t1"}
	before := time.Now()
	task.UpdateNextRun(2 * time.Second)
	next := task.NextRun
	if next.Before(before.Add(1500 * time.Millisecond)) {
		t.Fatalf("NextRun too early: %v", next)
	}
	if task.LastScheduledAt.IsZero() {
		t.Fatal("LastScheduledAt should be set")
	}
}

func TestScanEngine_GetActiveTaskCount(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev-active", "modbus-tcp", time.Second, 3, []string{"p1"}, nil)
	task.SetStatus(ScanTaskStatusRunning)
	if se.GetActiveTaskCount() != 1 {
		t.Fatalf("active = %d, want 1", se.GetActiveTaskCount())
	}

	adapter := NewScanEngineAdapter(se)
	if adapter.GetActiveTaskCount() != 1 {
		t.Fatalf("adapter active = %d, want 1", adapter.GetActiveTaskCount())
	}
}

func TestDeviceStorageManager_SetStorageClearAndTimeRange(t *testing.T) {
	tmpDir := testOutputDir(t)
	store1, err := storage.NewStorage(filepath.Join(tmpDir, "store1"))
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer store1.Close()

	pipeline := NewDataPipeline(10)
	dsm := NewDeviceStorageManager(store1, pipeline)
	sc := NewShadowCore()
	dsm.SetShadowCore(sc)

	store2, err := storage.NewStorage(filepath.Join(tmpDir, "store2"))
	if err != nil {
		t.Fatalf("NewStorage store2: %v", err)
	}
	defer store2.Close()
	dsm.SetStorage(store2)

	deviceID := "dev-range"
	dsm.UpdateDeviceConfig(deviceID, model.DeviceStorage{
		Enable:     true,
		Strategy:   "realtime",
		MaxRecords: 10,
	})

	t0 := time.Now().Add(-time.Minute)
	writeShadowPoints(sc, deviceID, map[string]any{"p1": 10})
	dsm.saveSnapshot(deviceID, t0)
	writeShadowPoints(sc, deviceID, map[string]any{"p1": 20})
	dsm.saveSnapshot(deviceID, time.Now())

	records, err := dsm.GetHistoryByTimeRange(deviceID, t0.Add(-time.Second), time.Now().Add(time.Second), 10)
	if err != nil {
		t.Fatalf("GetHistoryByTimeRange: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("time range records = %d, want >= 2", len(records))
	}

	dsm.ClearAllHistory()
	history, err := dsm.GetHistory(deviceID, 10)
	if err != nil {
		t.Fatalf("GetHistory after clear: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("history after clear = %d", len(history))
	}
}

func TestConnectionController_SetLimits(t *testing.T) {
	cc := NewConnectionController("modbus", "dev-lim", "modbus-tcp")
	cc.SetMaxRetries(7)
	cc.SetMaxFailCount(4)

	_, _, maxRetries, _, _, _, _ := cc.GetStatus()
	if maxRetries != 7 {
		t.Fatalf("maxRetries = %d, want 7", maxRetries)
	}

	cc.SetState(ConnStateHealthy)
	for i := 0; i < 4; i++ {
		cc.RecordReadFailure()
	}
	if cc.GetState() != ConnStateDegraded {
		t.Fatalf("state = %s, want Degraded", cc.GetState())
	}
}

func TestToInt64_AllNumericTypes(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int(5), 5},
		{int8(6), 6},
		{int16(7), 7},
		{int32(8), 8},
		{int64(9), 9},
		{uint(10), 10},
		{uint8(11), 11},
		{uint16(12), 12},
		{uint32(13), 13},
		{uint64(14), 14},
		{float32(15.9), 15},
		{float64(16.1), 16},
		{"17", 17},
		{"18.5", 18},
	}
	for _, tc := range cases {
		got, err := toInt64(tc.in)
		if err != nil {
			t.Fatalf("toInt64(%v): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("toInt64(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
	if _, err := toInt64("not-a-number"); err == nil {
		t.Fatal("expected error for invalid string")
	}
}

func TestSystemManager_EffectiveHostnameConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 9090
	cfg.System.Hostname = model.HostnameConfig{}
	sm := NewSystemManager(cfg)

	got := sm.effectiveHostnameConfig()
	if got.Name != "edgeCore" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.HTTPPort != 9090 || got.HTTPSPort != 443 {
		t.Fatalf("ports = http %d https %d", got.HTTPPort, got.HTTPSPort)
	}
}

func TestChannelManager_GetDevicePoints_ViaDriver(t *testing.T) {
	cm, channelID := setupMockChannel(t, true)
	if err := cm.StartChannel(channelID); err != nil {
		t.Fatalf("StartChannel: %v", err)
	}

	points, err := cm.GetDevicePoints(channelID, "dev-1")
	if err != nil {
		t.Fatalf("GetDevicePoints: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
}

func TestNorthboundManager_UpdateClientsDisablePaths(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, nil)

	oldMQTT := []model.MQTTConfig{{ID: "m1", Name: "M1", Enable: true, Broker: "tcp://127.0.0.1:1883"}}
	newMQTT := []model.MQTTConfig{{ID: "m1", Name: "M1", Enable: false, Broker: "tcp://127.0.0.1:1883"}}
	nm.mu.Lock()
	nm.updateMQTTClients(oldMQTT, newMQTT)
	nm.updateHTTPClients(
		[]model.HTTPConfig{{ID: "h1", Name: "H1", Enable: true, URL: "http://127.0.0.1"}},
		[]model.HTTPConfig{},
	)
	nm.updateSparkplugBClients(
		[]model.SparkplugBConfig{{ID: "s1", Name: "S1", Enable: true, Broker: "tcp://127.0.0.1:1883"}},
		[]model.SparkplugBConfig{{ID: "s1", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	)
	nm.updateOPCUAServers(
		[]model.OPCUAConfig{{ID: "o1", Name: "O1", Enable: true, Port: 4840}},
		[]model.OPCUAConfig{{ID: "o1", Enable: false, Port: 4840}},
	)
	nm.updateEdgeOSMQTTClients(
		[]model.EdgeOSMQTTConfig{{ID: "e1", Enable: true, Broker: "tcp://127.0.0.1:1883"}},
		[]model.EdgeOSMQTTConfig{{ID: "e1", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	)
	nm.updateEdgeOSNATSClients(
		[]model.EdgeOSNATSConfig{{ID: "n1", Enable: true, URL: "nats://127.0.0.1:4222"}},
		[]model.EdgeOSNATSConfig{{ID: "n1", Enable: false, URL: "nats://127.0.0.1:4222"}},
	)
	nm.mu.Unlock()
}

func TestEdgeComputeManager_LoadRulesThreshold(t *testing.T) {
	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, nil, nil)
	em.SetBatchWindow(0)
	em.Start()
	defer em.Stop()

	em.LoadRules([]model.EdgeRule{{
		ID: "thresh-load", Type: "threshold", Enable: true, TriggerMode: "always",
		Condition: "t1 >= 5",
		Sources:   []model.RuleSource{{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"}},
		Actions:   []model.RuleAction{{Type: "log", Config: map[string]any{"message": "high"}}},
	}})

	em.handleValue(model.Value{ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 10, TS: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if len(em.GetRules()) != 1 {
		t.Fatal("LoadRules should register one rule")
	}
}

func TestAdaptiveThrottle_DeviceRTTAndApplyInterval(t *testing.T) {
	metrics := &ScanEngineMetrics{}
	at := NewAdaptiveThrottle(metrics)
	at.Refresh(800, 1000, 0.2, 250)
	at.UpdateDeviceRTT("dev-th", 100)
	at.UpdateDeviceRTT("dev-th", 500)

	task := &ScanTask{
		DeviceKey:    "dev-th",
		BaseInterval: 100 * time.Millisecond,
		Interval:     100 * time.Millisecond,
	}
	if !at.ApplyInterval(task) {
		t.Fatal("expected interval adjustment after RTT spike")
	}
	if at.DeviceFactor("dev-th") < deviceRTTMinFactor {
		t.Fatalf("DeviceFactor = %v, want >= %v", at.DeviceFactor("dev-th"), deviceRTTMinFactor)
	}
}

func TestExecutionLayer_SerialWorkerReadFunc(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := &ExecutionContext{
		DeviceKey: "dev-readfunc",
		Queue:     make(chan *DriverTask, 1),
	}
	worker := &SerialWorker{ctx: ctx, stopCh: make(chan struct{}), wg: &wg}
	go worker.run()

	done := make(chan struct{})
	ctx.Queue <- &DriverTask{
		ReadFunc: func(_ context.Context, pts []model.Point) (map[string]model.Value, error) {
			return map[string]model.Value{pts[0].ID: {PointID: pts[0].ID, Value: 42}}, nil
		},
		Points: []model.Point{{ID: "p1"}},
		Callback: func(values map[string]model.Value, err error) {
			if err != nil {
				t.Errorf("callback error: %v", err)
			}
			if values["p1"].Value != 42 {
				t.Errorf("value = %v", values["p1"].Value)
			}
			close(done)
		},
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker timeout")
	}
	close(ctx.Queue)
	close(worker.stopCh)
	wg.Wait()
}

func TestExecutionLayer_SerialWorkerDriverNotFound(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := &ExecutionContext{
		DeviceKey: "dev-no-driver",
		Queue:     make(chan *DriverTask, 1),
		Driver:    nil,
	}
	worker := &SerialWorker{ctx: ctx, stopCh: make(chan struct{}), wg: &wg}
	go worker.run()

	done := make(chan struct{})
	ctx.Queue <- &DriverTask{
		Points: []model.Point{{ID: "p1"}},
		Callback: func(_ map[string]model.Value, err error) {
			if !errors.Is(err, ErrDriverNotFound) {
				t.Errorf("err = %v, want ErrDriverNotFound", err)
			}
			close(done)
		},
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker timeout")
	}
	close(ctx.Queue)
	close(worker.stopCh)
	wg.Wait()
}
