package integration_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/testutil/fault"
	mbsim "github.com/anviod/edgeCore/internal/testutil/modbus"
)

/*
Modbus PLC pressure test (Sprint 3 — simulator-backed, no real hardware):

	go test ./internal/integration/ -run TestModbusPLC_PressureMultiSlaveIsolation -count=1 -timeout=5m

Validates 12 simulated slaves × 100 points, variable latency via fault injector,
one hung slave, P95 lag ≤ 200ms, healthy slave isolation, metrics snapshot JSON.
*/

const (
	plcPressureSlaveCount     = 12
	plcPressurePointsPerSlave = 100
	plcPressureHungSlave      = 12
)

func makePLCHoldingPoints(deviceID string, count int) []model.Point {
	points := make([]model.Point, count)
	for i := range points {
		points[i] = model.Point{
			ID:       fmt.Sprintf("p%03d", i),
			Address:  fmt.Sprintf("%d", 40001+i),
			DataType: "INT16",
			DeviceID: deviceID,
		}
	}
	return points
}

func seedPLCSimulatorHolding(sim *mbsim.Simulator, unitID uint8, count int) {
	regs := make([]uint16, count)
	for i := range regs {
		regs[i] = uint16(i + 1)
	}
	sim.SeedHolding(unitID, regs...)
}

func TestModbusPLC_PressureMultiSlaveIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping modbus PLC pressure test in short mode")
	}

	sim := mbsim.StartSimulator(t)
	for unit := 1; unit <= plcPressureSlaveCount; unit++ {
		seedPLCSimulatorHolding(sim, uint8(unit), plcPressurePointsPerSlave)
	}
	sim.SetLatency(plcPressureHungSlave, 30*time.Second)

	channelID := "plc-pressure"
	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       16,
		MaxQueueSize:      20000,
		AntiStarvationSec: 300,
		GoroutineLimit:    256,
		ConnectionLimit:   64,
		JitterBound:       0,
	})
	se.RegisterProtocol("modbus-tcp", core.ProtocolTypeParallel)

	for i := 1; i <= plcPressureSlaveCount; i++ {
		devID := fmt.Sprintf("plc-slave-%02d", i)
		points := makePLCHoldingPoints(devID, plcPressurePointsPerSlave)
		pointIDs := make([]string, len(points))
		for j, p := range points {
			pointIDs[j] = p.ID
		}

		d := driver.Driver(newModbusDriver(t, sim, channelID, devID, i))
		if i != plcPressureHungSlave && i%3 == 0 {
			inj := fault.Wrap(d)
			inj.Latency = time.Duration(20+i*5) * time.Millisecond
			d = inj
		}
		se.RegisterDriver(devID, d)
		se.AddTask(devID, "modbus-tcp", time.Second, 5, pointIDs, map[string]any{
			"channelID": channelID,
			"slave_id":  i,
			"points":    points,
		})
	}

	se.Run()
	defer se.Stop()

	const warmup = 10 * time.Second
	time.Sleep(warmup)

	runFor := 15 * time.Second
	time.Sleep(runFor)

	snap := se.GetMetrics().Snapshot()
	p95, _ := snap["scan_lag_p95_ms"].(float64)
	executed, _ := snap["tasks_executed"].(uint64)
	failed, _ := snap["tasks_failed"].(uint64)

	failRate := 0.0
	if executed > 0 {
		failRate = float64(failed) / float64(executed)
	}

	var healthyMu sync.Mutex
	healthyOK := make(map[string]bool, plcPressureSlaveCount-1)
	for i := 1; i <= plcPressureSlaveCount; i++ {
		if i == plcPressureHungSlave {
			continue
		}
		devID := fmt.Sprintf("plc-slave-%02d", i)
		healthyOK[devID] = false
	}

	for i := 1; i <= plcPressureSlaveCount; i++ {
		if i == plcPressureHungSlave {
			continue
		}
		devID := fmt.Sprintf("plc-slave-%02d", i)
		task := se.GetTaskByDeviceKey(devID)
		if task == nil {
			t.Fatalf("missing task for healthy slave %s", devID)
		}
		res := se.ExecuteTask(task)
		if res != nil && res.Success {
			healthyMu.Lock()
			healthyOK[devID] = true
			healthyMu.Unlock()
		}
	}

	unhealthyHealthy := 0
	for devID, ok := range healthyOK {
		if !ok {
			unhealthyHealthy++
			t.Errorf("healthy slave %s should remain scannable while slave %d hangs", devID, plcPressureHungSlave)
		}
	}

	gates := []productionGate{
		{
			Name:   "scan_lag_p95_under_200ms",
			Passed: p95 <= plcPressureLagP95Limit,
			Value:  p95,
			Limit:  plcPressureLagP95Limit,
			Detail: fmt.Sprintf("P95 lag %.2fms", p95),
		},
		{
			Name:   "healthy_slaves_unaffected_by_hung_slave",
			Passed: unhealthyHealthy == 0,
			Value:  unhealthyHealthy,
			Limit:  0,
			Detail: fmt.Sprintf("hung slave=%d unaffected count=%d", plcPressureHungSlave, unhealthyHealthy),
		},
		{
			Name:   "fault_injection_fail_rate_bounded",
			Passed: failRate < 0.05,
			Value:  failRate,
			Limit:  0.05,
			Detail: "expected elevated failures from hung slave only",
		},
	}

	summary := buildProductionReadinessSummary(
		t.Name(),
		warmup+runFor,
		gates,
		snap,
	)
	assertProductionGates(t, summary)
}
