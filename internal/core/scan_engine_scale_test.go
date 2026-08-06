package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestScanEngine_EventDrivenDispatch(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 50 * time.Millisecond,
		WorkerCount:  4,
		MaxQueueSize: 1000,
	})
	task := se.AddTask("dev-1", "modbus-tcp", 20*time.Millisecond, 5, []string{"p1"}, nil)
	se.RegisterDriver("dev-1", &mockStressDriver{})
	se.Run()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if task.ConsecutiveSuccess > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	se.Stop()

	if task.ConsecutiveSuccess == 0 {
		t.Fatal("expected at least one successful execution via event-driven dispatch")
	}
}

func TestScanEngine_ScanClassTasks(t *testing.T) {
	se := NewScanEngine(ScanEngineConfig{TickInterval: 10 * time.Millisecond, WorkerCount: 2, MaxQueueSize: 100})
	points := []model.Point{
		{ID: "fast1", ScanClass: model.ScanClassFast},
		{ID: "slow1", ScanClass: model.ScanClassSlow},
	}
	params := map[string]any{"points": points, "degradeOnFailure": false}
	se.AddTaskWithScanClass("dev-1", "modbus-tcp", model.ScanClassFast, 100*time.Millisecond, 5, []string{"fast1"}, params)
	se.AddTaskWithScanClass("dev-1", "modbus-tcp", model.ScanClassSlow, 10*time.Second, 5, []string{"slow1"}, params)

	tasks := se.GetTasksByDeviceKey("dev-1")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 scan class tasks, got %d", len(tasks))
	}
}
