package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestBackpressureController_RejectTotal(t *testing.T) {
	bc := NewBackpressureController(1, 1000)

	if !bc.Allow("dev-1", 1) {
		t.Fatal("first request should be allowed")
	}
	if bc.Allow("dev-2", 1) {
		t.Fatal("second request should be rejected by global limit")
	}
	bc.Release("dev-1")

	if bc.RejectTotal() == 0 {
		t.Fatal("expected backpressure rejects to be counted")
	}
}

func TestSerialQueueManager_QueueDepths(t *testing.T) {
	sqm := NewSerialQueueManager()
	sqm.createContext("shared:ch-1")

	depths := sqm.QueueDepths()
	if depths["shared:ch-1"] != 0 {
		t.Fatalf("expected empty queue depth, got %v", depths["shared:ch-1"])
	}
}

func TestExecutionLayer_LoadPointsZeroAlloc(t *testing.T) {
	el := NewExecutionLayer()
	points := make([]model.Point, 100)
	for i := range points {
		points[i] = model.Point{ID: "p", Address: "0", DataType: "int16", DeviceID: "dev-1"}
	}
	task := &ScanTask{
		DeviceKey: "dev-1",
		Points:    points,
	}

	allocs := testing.AllocsPerRun(10, func() {
		got := el.loadPoints(task)
		if len(got) != len(points) {
			t.Fatalf("got %d points, want %d", len(got), len(points))
		}
	})
	if allocs != 0 {
		t.Fatalf("loadPoints allocs/op = %v, want 0", allocs)
	}
}
