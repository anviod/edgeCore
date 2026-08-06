package integration_test

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
)

// TestS7Protocol_SessionFramework is a skeleton for long-running S7 session stability (D-04).
func TestS7Protocol_SessionFramework(t *testing.T) {
	if testing.Short() {
		t.Skip("S7 long-session test skipped in short mode")
	}

	se := core.NewScanEngine(core.ScanEngineConfig{
		TickInterval: 5 * time.Millisecond,
		JitterBound:  50 * time.Millisecond,
	})
	se.RegisterProtocol("s7", core.ProtocolTypeLimited)

	// Framework mirrors modbus_protocol_test.go:
	// - shared connection manager session lock
	// - subscription/read jitter under ScanEngine EDF
	// Wire PLCSIM or hardware when available.

	_ = se
	t.Log("S7 session framework ready; connect snap7/plcsim to enable full soak")
}

func TestS7Protocol_EDFDeadlineInitialized(t *testing.T) {
	se := core.NewScanEngine(core.ScanEngineConfig{JitterBound: 30 * time.Millisecond})
	task := se.AddTask("s7-plc-1", "s7", 200*time.Millisecond, 5, []string{"db1.w0"}, nil)

	got := se.GetTask(task.ID)
	if got == nil {
		t.Fatal("task not found")
	}
	if got.DeadlineAt.IsZero() {
		t.Fatal("expected non-zero DeadlineAt for S7 task")
	}
	if !got.DeadlineAt.After(got.NextRun) {
		t.Fatalf("DeadlineAt %v must be after NextRun %v", got.DeadlineAt, got.NextRun)
	}
}
