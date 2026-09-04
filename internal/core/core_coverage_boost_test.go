package core

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

type scannerStubDriver struct {
	stubChannelDriver
	scanOut any
	scanErr error
}

func (s *scannerStubDriver) Scan(_ context.Context, _ map[string]any) (any, error) {
	if s.scanErr != nil {
		return nil, s.scanErr
	}
	return s.scanOut, nil
}

type objectScannerStubDriver struct {
	scannerStubDriver
}

func (s *objectScannerStubDriver) ScanObjects(_ context.Context, _ map[string]any) (any, error) {
	return s.scanOut, s.scanErr
}

func TestScanTaskStatus_String(t *testing.T) {
	cases := map[ScanTaskStatus]string{
		ScanTaskStatusIdle:     "Idle",
		ScanTaskStatusRunning:  "Running",
		ScanTaskStatusDegraded: "Degraded",
		ScanTaskStatusStopped:  "Stopped",
		ScanTaskStatus(99):     "Unknown",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Fatalf("ScanTaskStatus(%d).String() = %q, want %q", status, got, want)
		}
	}
}

func TestScanEngine_PriorityQueuePeekAndExecuteTask(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	pq := &PriorityQueue{}
	task := se.AddTask("dev-exec", "modbus-tcp", time.Second, 5, []string{"p1"}, nil)
	pq.Push(task)

	if pq.Peek() == nil {
		t.Fatal("Peek should return top task")
	}
	if empty := (&PriorityQueue{}).Peek(); empty != nil {
		t.Fatal("empty queue Peek should be nil")
	}

	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	el.RegisterDriver("dev-exec", &execStubDriver{})
	se.executionLayer = el

	result := se.ExecuteTask(task)
	if result == nil || !result.Success {
		t.Fatalf("ExecuteTask = %+v", result)
	}
	if se.ExecuteTask(nil) == nil || se.ExecuteTask(nil).Success {
		t.Fatal("ExecuteTask(nil) should fail")
	}
}

func TestScanEngine_UpdateTaskPriorityAndDriverConfig(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{})
	task := se.AddTask("dev-pri", "modbus-tcp", time.Second, 3, []string{"p1"}, map[string]any{
		"driverConfig": map[string]any{"timeout": 1},
	})

	se.UpdateTaskPriority("dev-pri", 9)
	if task.Priority != 9 {
		t.Fatalf("priority = %d, want 9", task.Priority)
	}
	se.UpdateTaskPriority("missing", 1)

	se.UpdateTaskDriverConfig("dev-pri", map[string]any{"timeout": 5, "retries": 2})
	cfg := task.Params["driverConfig"].(map[string]any)
	if cfg["timeout"] != 5 || cfg["retries"] != 2 {
		t.Fatalf("driverConfig = %+v", cfg)
	}
	se.UpdateTaskDriverConfig("dev-pri", nil)
}

func TestScanEngine_GetShadowCoreAndCollectFinalize(t *testing.T) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{})
	se.SetShadowCore(sc)
	if se.GetShadowCore() != sc {
		t.Fatal("GetShadowCore mismatch")
	}

	var finalized bool
	se.SetCollectFinalize(func(_ string, _ *ExecuteResult) { finalized = true })
	se.collectFinalize("dev-1", &ExecuteResult{Success: true})
	if !finalized {
		t.Fatal("collect finalize callback not invoked")
	}
}

func TestExecutionLayer_ReduceBackpressureRate(t *testing.T) {
	el := NewExecutionLayer()
	before := el.GetBackpressure().TokenRate()
	el.ReduceBackpressureRate(0.5)
	after := el.GetBackpressure().TokenRate()
	if after >= before {
		t.Fatalf("token rate should decrease: before=%v after=%v", before, after)
	}
}

func TestChannelManager_ShutdownAndBatchAddModbusSlaves(t *testing.T) {
	cm := NewChannelManager(nil, func(_ []model.Channel) error { return nil })
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-modbus-batch"
	cm.channels[channelID] = &model.Channel{
		ID: channelID, Name: "Modbus Batch", Protocol: "modbus-tcp",
		Devices: []model.Device{},
	}
	cm.drivers[channelID] = &stubChannelDriver{}
	cm.driverMus[channelID] = &sync.Mutex{}

	result, err := cm.BatchAddModbusSlaves(channelID, 1, 2, 0, 1, model.Duration(time.Second), true, "int16", "R", model.RegHolding, 3)
	if err != nil {
		t.Fatalf("BatchAddModbusSlaves: %v", err)
	}
	if len(result.Created) != 2 {
		t.Fatalf("created = %d, want 2", len(result.Created))
	}

	dup, err := cm.BatchAddModbusSlaves(channelID, 1, 2, 0, 1, model.Duration(time.Second), true, "int16", "R", model.RegHolding, 3)
	if err != nil {
		t.Fatalf("BatchAddModbusSlaves duplicate run: %v", err)
	}
	if len(dup.Skipped) != 2 {
		t.Fatalf("skipped = %d, want 2", len(dup.Skipped))
	}

	cm.Shutdown()
}

func TestChannelManager_RecordCircuitBreakerEvent(t *testing.T) {
	mc := model.NewMetricsCollector()
	model.SetGlobalMetricsCollector(mc)
	t.Cleanup(func() { model.SetGlobalMetricsCollector(nil) })

	cm := newTestChannelManager()
	cm.recordCircuitBreakerEvent("dev-1", "circuit_open", "test event")
	cm.recordCircuitBreakerEvent("missing-dev", "circuit_open", "ignored")
}

func TestChannelManager_FinalizeScanCollectPaths(t *testing.T) {
	cm := newTestChannelManager()
	cm.finalizeScanCollect("dev-1", &ExecuteResult{Success: true})
	cm.finalizeScanCollect("missing", &ExecuteResult{Success: false, Error: errors.New("timeout")})
	cm.finalizeScanCollect("dev-1", nil)
}

func TestChannelManager_ScanChannel(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-scan"
	cm.channels[channelID] = &model.Channel{
		ID: channelID, Name: "Scan", Protocol: addChannelMockProtocol,
	}
	cm.drivers[channelID] = &scannerStubDriver{scanOut: []map[string]any{{"id": "dev-x"}}}
	cm.driverMus[channelID] = &sync.Mutex{}

	out, err := cm.ScanChannel(context.Background(), channelID, nil)
	if err != nil {
		t.Fatalf("ScanChannel: %v", err)
	}
	if out == nil {
		t.Fatal("expected scan result")
	}

	cm.drivers[channelID] = &stubChannelDriver{}
	if _, err := cm.ScanChannel(context.Background(), channelID, nil); err == nil {
		t.Fatal("expected error when driver does not support scanning")
	}
}

func TestChannelManager_OnBACnetAddressDiscovered(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-bacnet"
	cm.channels[channelID] = &model.Channel{
		ID: channelID, Name: "BACnet", Protocol: "bacnet-ip",
		Devices: []model.Device{{
			ID: "dev-bacnet", Name: "BACnet Dev",
			Config: map[string]any{"ip": "192.168.1.1", "port": 47808},
		}},
	}
	cm.drivers[channelID] = &stubChannelDriver{}
	cm.driverMus[channelID] = &sync.Mutex{}

	cm.OnBACnetAddressDiscovered("dev-bacnet", "192.168.1.10", 47809)
	dev := cm.GetDevice(channelID, "dev-bacnet")
	if dev.Config["ip"] != "192.168.1.10" || dev.Config["port"] != 47809 {
		t.Fatalf("bacnet address not updated: %+v", dev.Config)
	}
	cm.OnBACnetAddressDiscovered("", "192.168.1.10", 47809)
}

// mockScanResult mimics bacnet.ScanResult without importing the bacnet package.
// Used to test autoUpdateBACnetAddresses reflection logic.
type mockScanResult struct {
	DeviceID   int    `json:"bacnet_device_id"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	DiffStatus string `json:"diff_status"`
}

// TestAutoUpdateBACnetAddresses_NoPanic verifies that the reflection-based
// address extraction does not panic when processing struct slices with int fields.
// Regression test for the "reflect.Value.FieldByName on int Value" panic.
func TestAutoUpdateBACnetAddresses_NoPanic(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-bacnet-auto"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "BACnet Auto",
		Protocol: "bacnet-ip",
		Devices: []model.Device{{
			ID:     "2228316",
			Name:   "RoomController 2228316",
			Config: map[string]any{"bacnet_device_id": 2228316, "ip": "192.168.3.104", "port": 53772},
		}},
	}
	cm.drivers[channelID] = &stubChannelDriver{}
	cm.driverMus[channelID] = &sync.Mutex{}

	// Simulate a scan result where the port has changed (device reboot).
	// This is the exact scenario that triggered the original panic.
	scanResult := []mockScanResult{
		{DeviceID: 2228316, IP: "192.168.3.104", Port: 99999, DiffStatus: "existing"},
	}

	// This must not panic.
	cm.autoUpdateBACnetAddresses(cm.channels[channelID], scanResult)

	// Verify the address was updated.
	dev := cm.GetDevice(channelID, "2228316")
	if dev == nil {
		t.Fatal("device not found after auto-update")
	}
	port := dev.Config["port"]
	if port != 99999 {
		t.Fatalf("port not updated: expected 99999, got %v", port)
	}
}

// TestAutoUpdateBACnetAddresses_NilAndNonSlice verifies graceful handling
// of nil and non-slice scan results.
func TestAutoUpdateBACnetAddresses_NilAndNonSlice(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-bacnet-nil"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Protocol: "bacnet-ip",
		Devices: []model.Device{{
			ID:     "dev1",
			Config: map[string]any{"bacnet_device_id": 1},
		}},
	}

	// nil result — must not panic.
	cm.autoUpdateBACnetAddresses(cm.channels[channelID], nil)

	// Non-slice result — must not panic.
	cm.autoUpdateBACnetAddresses(cm.channels[channelID], "not a slice")

	// Empty slice — must not panic.
	cm.autoUpdateBACnetAddresses(cm.channels[channelID], []mockScanResult{})

	// Slice of ints (non-struct elements) — must not panic.
	cm.autoUpdateBACnetAddresses(cm.channels[channelID], []int{1, 2, 3})
}

// TestExtractIntByJSONTag_NoPanicOnIntField directly tests the helper function
// that caused the original panic: calling FieldByName on an int value.
func TestExtractIntByJSONTag_NoPanicOnIntField(t *testing.T) {
	item := mockScanResult{DeviceID: 2228316, IP: "192.168.3.104", Port: 53772}
	v := reflect.ValueOf(item)

	// These calls previously panicked with "FieldByName on int Value".
	port := extractIntByJSONTag(v, "port")
	if port != 53772 {
		t.Fatalf("extractIntByJSONTag(port) = %d, want 53772", port)
	}

	id := extractIntByJSONTag(v, "bacnet_device_id")
	if id != 2228316 {
		t.Fatalf("extractIntByJSONTag(bacnet_device_id) = %d, want 2228316", id)
	}

	ip := extractStringByJSONTag(v, "ip")
	if ip != "192.168.3.104" {
		t.Fatalf("extractStringByJSONTag(ip) = %q, want 192.168.3.104", ip)
	}

	// Test with non-struct value — must return 0, not panic.
	intVal := reflect.ValueOf(42)
	if extractIntByJSONTag(intVal, "port") != 0 {
		t.Fatal("extractIntByJSONTag on int should return 0")
	}
	if extractStringByJSONTag(intVal, "ip") != "" {
		t.Fatal("extractStringByJSONTag on int should return empty string")
	}
	if extractIntField(intVal, "DeviceID") != 0 {
		t.Fatal("extractIntField on int should return 0")
	}
}

func TestNorthboundManager_SaveConfigAndUpdateConfig(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "m1", Name: "MQTT", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	nm.mu.Lock()
	if err := nm.saveConfig(); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	nm.mu.Unlock()
	if len(saved.MQTT) != 1 {
		t.Fatalf("saved = %+v", saved.MQTT)
	}

	nm.UpdateConfig(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "m2", Name: "MQTT 2", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
		HTTP: []model.HTTPConfig{{ID: "h1", Name: "HTTP", Enable: false, URL: "http://127.0.0.1"}},
	})
	if len(nm.config.MQTT) != 1 || nm.config.MQTT[0].ID != "m2" {
		t.Fatalf("UpdateConfig mqtt = %+v", nm.config.MQTT)
	}
}

func TestNorthboundManager_DeleteMQTTAndSetVirtualShadowManager(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "del-mqtt", Name: "Del", Enable: false, Broker: "tcp://127.0.0.1:1883"}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	sc := NewShadowCore()
	vse := NewVirtualShadowEngine(sc)
	vsm := NewVirtualShadowManager(vse, nil, sc, nil)
	nm.SetVirtualShadowManager(vsm)
	if nm.vsm == nil {
		t.Fatal("SetVirtualShadowManager failed")
	}

	if err := nm.DeleteMQTTConfig("del-mqtt"); err != nil {
		t.Fatalf("DeleteMQTTConfig: %v", err)
	}
	if len(saved.MQTT) != 0 {
		t.Fatalf("expected empty mqtt configs, got %d", len(saved.MQTT))
	}
}

func TestEdgeComputeManager_SetStorageRestoreState(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(filepath.Join(tmpDir, "edge-runtime"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	state := model.RuleRuntimeState{RuleID: "rule-restore", CurrentStatus: "ALARM"}
	if err := store.SaveData(storage.BucketRuleState, "rule-restore", state); err != nil {
		t.Fatalf("SaveData rule state: %v", err)
	}

	pipeline := NewDataPipeline(10)
	em := NewEdgeComputeManager(pipeline, store, nil)
	em.SetStorage(store)
	em.restoreState()

	states := em.GetRuleStates()
	if states["rule-restore"] == nil || states["rule-restore"].CurrentStatus != "ALARM" {
		t.Fatalf("restoreState = %+v", states["rule-restore"])
	}
}

func TestChannelManager_ScanDevice(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-scan-dev"
	cm.channels[channelID] = &model.Channel{
		ID: channelID, Name: "Scan Dev", Protocol: addChannelMockProtocol,
		Devices: []model.Device{{ID: "dev-scan", Name: "Dev", Config: map[string]any{"node_id": "ns=2;s=x"}}},
	}
	cm.drivers[channelID] = &objectScannerStubDriver{scannerStubDriver: scannerStubDriver{scanOut: []model.Point{{ID: "p1", Address: "0"}}}}
	cm.driverMus[channelID] = &sync.Mutex{}

	out, err := cm.ScanDevice(context.Background(), channelID, "dev-scan", map[string]any{"browse": true})
	if err != nil {
		t.Fatalf("ScanDevice: %v", err)
	}
	if out == nil {
		t.Fatal("expected scan device result")
	}
	if _, err := cm.ScanDevice(context.Background(), channelID, "missing", nil); err == nil {
		t.Fatal("expected error for missing device")
	}
}

func TestExecutionLayer_AllowThrottled(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	task := &ScanTask{DeviceKey: "dev-throttle", Protocol: "modbus-tcp"}
	if !el.allowThrottled(task, 10) {
		t.Fatal("allowThrottled should pass with positive device limit")
	}
}

func TestChannelManager_RestartDeviceLocked(t *testing.T) {
	cm, channelID := setupMockChannel(t, true)
	ch := cm.channels[channelID]
	if err := cm.restartDeviceLocked(ch, 0); err != nil {
		t.Fatalf("restartDeviceLocked: %v", err)
	}
}

// Ensure scanner stubs satisfy driver interfaces at compile time.
var (
	_ driver.Scanner       = (*scannerStubDriver)(nil)
	_ driver.ObjectScanner = (*objectScannerStubDriver)(nil)
)
