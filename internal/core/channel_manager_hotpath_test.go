package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestChannelManager_validateEtherCATPoint(t *testing.T) {
	cm := newTestChannelManager()
	t.Cleanup(func() { cm.cancel() })

	cases := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "pdo tx offset", address: "1:Tx:0", wantErr: false},
		{name: "pdo rx bit", address: "2:Rx:4.3", wantErr: false},
		{name: "pdo endian", address: "1:Tx:2#LE", wantErr: false},
		{name: "sdo index", address: "1:SDO:0x6041:0", wantErr: false},
		{name: "sdo endian", address: "1:SDO:0x6064:0#BE", wantErr: false},
		{name: "empty", address: "", wantErr: true},
		{name: "invalid", address: "bad-format", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cm.validateEtherCATPoint(&model.Point{Address: tc.address})
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestChannelManager_AddPoint_EtherCATValidation(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-ethercat"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "EtherCAT",
		Protocol: "ethercat",
		Devices: []model.Device{{
			ID: "dev-1", Name: "Slave", Points: []model.Point{},
		}},
	}
	cm.driverMus[channelID] = new(sync.Mutex)

	if err := cm.AddPoint(channelID, "dev-1", &model.Point{
		ID: "pt-1", Name: "Status", Address: "1:Tx:0", DataType: "uint16",
	}); err != nil {
		t.Fatalf("AddPoint valid: %v", err)
	}
	if err := cm.AddPoint(channelID, "dev-1", &model.Point{
		ID: "pt-bad", Name: "Bad", Address: "not-valid", DataType: "uint16",
	}); err == nil {
		t.Fatal("expected ethercat address validation error")
	}
}

func TestChannelManager_GetChannelScanEngineMetricsSnapshot(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  2,
		MaxQueueSize: 64,
	})
	se.Run()
	t.Cleanup(se.Stop)

	cm := newTestChannelManager()
	cm.scanEngineAdapter = NewScanEngineAdapter(se)
	t.Cleanup(func() { cm.cancel() })

	if snap := cm.GetChannelScanEngineMetricsSnapshot(""); len(snap) != 0 {
		t.Fatalf("empty channel id should return empty snapshot: %+v", snap)
	}

	se.GetMetrics().RecordDriftForChannel("ch-1", 25_000)
	se.GetMetrics().RecordExecuteForChannel("ch-1", true, 80_000)
	task := se.AddTask("dev-1", "modbus-tcp", time.Second, 5, []string{"hr_0"}, map[string]any{"channelID": "ch-1"})
	task.mu.Lock()
	task.NextRun = time.Now().Add(-50 * time.Millisecond)
	task.mu.Unlock()

	snap := cm.GetChannelScanEngineMetricsSnapshot("ch-1")
	if len(snap) == 0 {
		t.Fatal("expected non-empty metrics snapshot")
	}
	if _, ok := snap["circuit_breaker_open"]; !ok {
		t.Fatalf("missing circuit_breaker_open: %+v", snap)
	}
	if _, ok := snap["sla_warnings"]; !ok {
		t.Fatalf("missing sla_warnings: %+v", snap)
	}
}

func TestChannelManager_RemoveDevice_UnregistersScanTasks(t *testing.T) {
	cm := newTestChannelManager()
	t.Cleanup(func() { cm.cancel() })

	se := cm.scanEngineAdapter.scanEngine
	se.AddTask("dev-1", "modbus-tcp", time.Second, 5, []string{"p1"}, map[string]any{"channelID": "ch-1"})

	if err := cm.RemoveDevice("ch-1", "dev-1"); err != nil {
		t.Fatalf("RemoveDevice: %v", err)
	}
	if cm.GetDevice("ch-1", "dev-1") != nil {
		t.Fatal("device should be removed")
	}
	if tasks := se.GetTasksByDeviceKey("dev-1"); len(tasks) != 0 {
		t.Fatalf("expected scan tasks removed, got %d", len(tasks))
	}
}
