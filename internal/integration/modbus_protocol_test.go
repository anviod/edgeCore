package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/driver/modbus"
	"github.com/anviod/edgeCore/internal/model"
	mbsim "github.com/anviod/edgeCore/internal/testutil/modbus"
)

func newModbusDriver(t *testing.T, sim *mbsim.Simulator, channelID, deviceID string, slaveID int) *modbus.ModbusDriver {
	t.Helper()
	d := modbus.NewModbusDriver().(*modbus.ModbusDriver)
	if err := d.Init(model.DriverConfig{
		ChannelID: channelID,
		Config: map[string]any{
			"url":      sim.URL(),
			"slave_id": slaveID,
			"timeout":  800,
		},
	}); err != nil {
		t.Fatalf("init modbus driver: %v", err)
	}
	if err := d.SetDeviceConfig(map[string]any{"slave_id": slaveID}); err != nil {
		t.Fatalf("set device config: %v", err)
	}
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("connect modbus driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect() })
	_ = deviceID
	return d
}

func TestModbusProtocol_ScanEngineRealDriver(t *testing.T) {
	sim := mbsim.StartSimulator(t)
	for unit := 1; unit <= 3; unit++ {
		sim.SeedHolding(uint8(unit), uint16(unit)*100)
	}

	channelID := "modbus-tcp-live"
	channelMu := &sync.Mutex{}
	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
		JitterBound:  0,
	})
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeSerial)

	for i := 1; i <= 3; i++ {
		devID := fmt.Sprintf("live-slave-%d", i)
		d := newModbusDriver(t, sim, channelID, devID, i)
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", 200*time.Millisecond, 5, []string{"p1"}, map[string]any{
			"channelID": channelID,
			"channelMu": channelMu,
			"slave_id":  i,
			"points": []model.Point{{
				ID:       "p1",
				Address:  "40001",
				DataType: "INT16",
				DeviceID: devID,
			}},
		})
	}

	se.Run()
	defer se.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if se.GetMetrics().TasksSucceeded.Load() >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if se.GetMetrics().TasksSucceeded.Load() < 3 {
		t.Fatalf("expected successful scans from real modbus driver, succeeded=%d",
			se.GetMetrics().TasksSucceeded.Load())
	}
}

func TestModbusProtocol_VariableLatencyIsolation(t *testing.T) {
	sim := mbsim.StartSimulator(t)
	sim.SeedHolding(1, 100)
	sim.SeedHolding(2, 200)
	sim.SetLatency(2, 3*time.Second)

	d1 := newModbusDriver(t, sim, "ch-latency", "dev-1", 1)
	d2 := newModbusDriver(t, sim, "ch-latency", "dev-2", 2)

	// Hot-path ReadPoints no longer dials; Connect must establish the link first.
	if err := d1.Connect(context.Background()); err != nil {
		t.Fatalf("connect fast slave: %v", err)
	}
	defer d1.Disconnect()
	if err := d2.Connect(context.Background()); err != nil {
		t.Fatalf("connect slow slave: %v", err)
	}
	defer d2.Disconnect()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	v1, err := d1.ReadPoints(ctx, []model.Point{{
		ID: "p1", Address: "40001", DataType: "INT16",
	}})
	if err != nil {
		t.Fatalf("fast slave read failed: %v", err)
	}
	if v1["p1"].Quality != "Good" {
		t.Fatalf("fast slave quality = %q", v1["p1"].Quality)
	}

	slowCtx, slowCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer slowCancel()
	start := time.Now()
	v2, err := d2.ReadPoints(slowCtx, []model.Point{{
		ID: "p2", Address: "40001", DataType: "INT16",
	}})
	elapsed := time.Since(start)
	// Scheduler returns Quality=Bad (often with err=nil) when the 3s slave latency
	// exceeds the short deadline; Good within the deadline would violate isolation.
	if err == nil && v2["p2"].Quality == "Good" {
		t.Fatalf("slow slave should not succeed under 3s latency within short deadline, elapsed=%s", elapsed)
	}
}

func TestModbusProtocol_SerialScanEngineFaultPropagation(t *testing.T) {
	sim := mbsim.StartSimulator(t)
	for unit := 1; unit <= 7; unit++ {
		sim.SeedHolding(uint8(unit), uint16(unit))
	}
	sim.BlockSlave(6, true)

	channelID := "modbus-tcp-serial-fault"
	channelMu := &sync.Mutex{}
	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
		JitterBound:  0,
	})
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeSerial)

	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("serial-slave-%d", i)
		d := newModbusDriver(t, sim, channelID, devID, i)
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", 300*time.Millisecond, 5, []string{"p1"}, map[string]any{
			"channelID": channelID,
			"channelMu": channelMu,
			"slave_id":  i,
			"points": []model.Point{{
				ID:       "p1",
				Address:  "40001",
				DataType: "INT16",
				DeviceID: devID,
			}},
		})
	}

	se.Run()
	defer se.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if se.GetMetrics().TasksSucceeded.Load() >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if se.GetMetrics().TasksSucceeded.Load() < 3 {
		t.Fatalf("healthy slaves should keep scanning on shared modbus link, succeeded=%d",
			se.GetMetrics().TasksSucceeded.Load())
	}

	faultTask := se.GetTaskByDeviceKey("serial-slave-6")
	if faultTask == nil {
		t.Fatal("missing faulted slave task")
	}
	faultRes := se.ExecuteTask(faultTask)
	if faultRes == nil || len(faultRes.Values) == 0 {
		t.Fatalf("faulted slave should return values, result=%+v", faultRes)
	}
	for id, v := range faultRes.Values {
		if v.Quality != "Bad" {
			t.Fatalf("faulted slave point %s quality = %q, want Bad", id, v.Quality)
		}
	}

	healthyTask := se.GetTaskByDeviceKey("serial-slave-1")
	healthyRes := se.ExecuteTask(healthyTask)
	if healthyRes == nil || !healthyRes.Success {
		t.Fatalf("healthy slave should succeed, result=%+v", healthyRes)
	}
	if v, ok := healthyRes.Values["p1"]; !ok || v.Quality != "Good" {
		t.Fatalf("healthy slave should return Good quality, value=%+v", healthyRes.Values["p1"])
	}
	if cb := se.GetCircuitBreaker(); cb.State("serial-slave-1") != core.CircuitClosed {
		t.Fatalf("healthy slave circuit should stay closed, state=%v", cb.State("serial-slave-1"))
	}
}

func TestModbusProtocol_CBRecoveryCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping modbus CB recovery soak variant in short mode")
	}

	sim := mbsim.StartSimulator(t)
	for unit := 1; unit <= 7; unit++ {
		sim.SeedHolding(uint8(unit), uint16(unit))
	}

	channelID := "modbus-cb-cycles"
	channelMu := &sync.Mutex{}
	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		MaxQueueSize: 1000,
		JitterBound:  0,
	})
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeSerial)

	for i := 1; i <= 7; i++ {
		devID := fmt.Sprintf("cycle-slave-%d", i)
		d := newModbusDriver(t, sim, channelID, devID, i)
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", 250*time.Millisecond, 5, []string{"p1"}, map[string]any{
			"channelID": channelID,
			"channelMu": channelMu,
			"slave_id":  i,
			"points": []model.Point{{
				ID: "p1", Address: "40001", DataType: "INT16", DeviceID: devID,
			}},
		})
	}

	se.Run()
	defer se.Stop()

	for cycle := 0; cycle < 3; cycle++ {
		sim.BlockSlave(6, true)
		time.Sleep(1500 * time.Millisecond)
		sim.BlockSlave(6, false)
		time.Sleep(1500 * time.Millisecond)
	}

	if se.GetMetrics().TasksSucceeded.Load() < 10 {
		t.Fatalf("expected healthy slaves to keep scanning after CB cycles, succeeded=%d",
			se.GetMetrics().TasksSucceeded.Load())
	}
}
