package integration_test

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
)

// TestOpcuaProtocol_SessionFramework is a skeleton for long-running OPC UA session
// stability (D-04). Full test requires a live OPC UA server or embedded simulator.
func TestOpcuaProtocol_SessionFramework(t *testing.T) {
	if testing.Short() {
		t.Skip("OPC UA long-session test skipped in short mode")
	}

	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		JitterBound:  50 * time.Millisecond,
	})
	se.RegisterProtocol("opc-ua", core.ProtocolTypeParallel)

	// Framework: register driver + task when simulator/server is available.
	// se.RegisterDriver("opcua-dev-1", driver)
	// se.AddTask("opcua-dev-1", "opc-ua", 500*time.Millisecond, 5, pointIDs, params)
	// se.Run(); defer se.Stop()
	// Assert: peer modbus tasks unaffected during opcua session reconnect.

	_ = se
	t.Log("OPC UA session lock / subscription jitter framework ready; wire live server to enable")
}

func TestOpcuaProtocol_SubscriptionJitterBound(t *testing.T) {
	jitterBound := 50 * time.Millisecond
	se := core.NewScanEngine(core.ScanEngineConfig{JitterBound: jitterBound})
	task := se.AddTask("opcua-mock", "opc-ua", time.Second, 5, []string{"p1"}, nil)

	got := se.GetTask(task.ID)
	if got == nil {
		t.Fatal("task not found")
	}
	if got.DeadlineAt.IsZero() {
		t.Fatal("expected non-zero DeadlineAt")
	}
	gap := got.DeadlineAt.Sub(got.NextRun)
	if gap != jitterBound {
		t.Fatalf("deadline gap = %v, want jitter bound %v", gap, jitterBound)
	}
}
