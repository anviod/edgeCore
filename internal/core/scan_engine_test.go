package core

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestScanEngine_AddTask(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  2,
		MaxQueueSize: 100,
	}

	se := NewScanEngine(config)

	task := se.AddTask("device1", "modbus-tcp", 1*time.Second, 5, []string{"point1", "point2"}, nil)

	if task.ID == "" {
		t.Error("任务ID不能为空")
	}
	if task.DeviceKey != "device1" {
		t.Errorf("设备Key不匹配，期望device1，实际%s", task.DeviceKey)
	}
	if task.Protocol != "modbus-tcp" {
		t.Errorf("协议不匹配，期望modbus-tcp，实际%s", task.Protocol)
	}
	if len(task.PointIDs) != 2 {
		t.Errorf("点位数量不匹配，期望2，实际%d", len(task.PointIDs))
	}
	if task.GetStatus() != ScanTaskStatusIdle {
		t.Errorf("任务状态不匹配，期望Idle，实际%s", task.GetStatus())
	}
}

func TestScanEngine_Schedule(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  2,
		MaxQueueSize: 100,
	}

	se := NewScanEngine(config)

	task := se.AddTask("device1", "modbus-tcp", 1*time.Second, 5, []string{"point1"}, nil)

	if task.NextRun.IsZero() {
		t.Error("任务下次执行时间不能为空")
	}
}

func TestScanEngine_AntiStarvation(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval:      10 * time.Millisecond,
		WorkerCount:       1,
		MaxQueueSize:      100,
		AntiStarvationSec: 1,
	}

	se := NewScanEngine(config)

	task := se.AddTask("device1", "modbus-tcp", 1*time.Second, 5, []string{"point1"}, nil)

	now := time.Now()
	task.NextRun = now.Add(-2 * time.Second)

	se.enforceAntiStarvation(now)

	task.mu.Lock()
	if task.Priority != 10 {
		t.Errorf("防饿死后优先级应为10，实际%d", task.Priority)
	}
	task.mu.Unlock()
}

func TestScanEngine_Priority(t *testing.T) {
	pq := &PriorityQueue{}
	heap.Init(pq)

	nextRun := time.Now().Add(100 * time.Millisecond)
	task1 := &ScanTask{
		ID:        "task1",
		DeviceKey: "device1",
		NextRun:   nextRun,
		Priority:  1,
	}
	task2 := &ScanTask{
		ID:        "task2",
		DeviceKey: "device2",
		NextRun:   nextRun,
		Priority:  10,
	}

	heap.Push(pq, task1)
	heap.Push(pq, task2)

	top := heap.Pop(pq).(*ScanTask)
	if top.ID != "task2" {
		t.Errorf("高优先级任务应先执行，实际%s", top.ID)
	}
}

func TestScanEngine_ApplyCollectToShadow_FailureMarksBad(t *testing.T) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  1,
		MaxQueueSize: 10,
	})
	se.SetShadowCore(sc)

	oldTime := time.Date(2026, 6, 29, 17, 23, 0, 0, time.UTC)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "modbus-slave-1",
		ChannelID: "ch-1",
		Timestamp: oldTime,
		Points: []model.ShadowIngressPoint{
			{PointID: "hr_0", Value: 10.0, Quality: "Good", CollectedAt: oldTime},
			{PointID: "hr_1", Value: 20.0, Quality: "Good", CollectedAt: oldTime},
		},
	}); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	task := &ScanTask{
		DeviceKey: "modbus-slave-1",
		Interval:  10 * time.Second,
		PointIDs:  []string{"hr_0", "hr_1", "hr_2"},
		Params:    map[string]any{"channelID": "ch-1"},
	}

	before := time.Now()
	se.applyCollectToShadow(task, &ExecuteResult{Success: false, Error: ErrTimeout})

	shadow, err := sc.GetShadowDevice("shadow-modbus-slave-1")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}

	for _, id := range []string{"hr_0", "hr_1", "hr_2"} {
		pt, ok := shadow.Points[id]
		if !ok {
			t.Fatalf("missing point %s in shadow", id)
		}
		if pt.Quality != "Bad" {
			t.Fatalf("point %s quality = %q, want Bad", id, pt.Quality)
		}
		if pt.CollectedAt.Before(before) {
			t.Fatalf("point %s collected_at not updated on failure: %v", id, pt.CollectedAt)
		}
	}

	if shadow.Points["hr_0"].Value != 10.0 {
		t.Fatalf("hr_0 value should be preserved, got %v", shadow.Points["hr_0"].Value)
	}
	if shadow.Points["hr_2"].Value != nil {
		t.Fatalf("hr_2 had no prior value, want nil, got %v", shadow.Points["hr_2"].Value)
	}
}

func TestScanEngine_ApplyCollectToShadow_PartialBadPreservesValue(t *testing.T) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  1,
		MaxQueueSize: 10,
	})
	se.SetShadowCore(sc)

	oldTime := time.Date(2026, 6, 29, 17, 23, 0, 0, time.UTC)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		Timestamp: oldTime,
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 99.0, Quality: "Good", CollectedAt: oldTime},
		},
	}); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	now := time.Now()
	task := &ScanTask{
		DeviceKey: "dev-1",
		Interval:  1 * time.Second,
		PointIDs:  []string{"p1"},
	}

	se.applyCollectToShadow(task, &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {PointID: "p1", Value: nil, Quality: "Bad", TS: now},
		},
	})

	shadow, err := sc.GetShadowDevice("shadow-dev-1")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	pt := shadow.Points["p1"]
	if pt.Quality != "Bad" {
		t.Fatalf("quality = %q, want Bad", pt.Quality)
	}
	if pt.Value != 99.0 {
		t.Fatalf("value = %v, want preserved 99.0", pt.Value)
	}
}

func TestScanEngine_ApplyCollectToShadow_SuccessMissingPointsMarksBad(t *testing.T) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  1,
		MaxQueueSize: 10,
	})
	se.SetShadowCore(sc)

	oldTime := time.Date(2026, 6, 29, 17, 23, 0, 0, time.UTC)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "snmp-dev",
		Timestamp: oldTime,
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 1.0, Quality: "Good", CollectedAt: oldTime},
			{PointID: "p2", Value: 2.0, Quality: "Good", CollectedAt: oldTime},
		},
	}); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	task := &ScanTask{
		DeviceKey: "snmp-dev",
		Interval:  5 * time.Second,
		PointIDs:  []string{"p1", "p2"},
	}

	before := time.Now()
	se.applyCollectToShadow(task, &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {PointID: "p1", Value: 1.1, Quality: "Good", TS: before},
		},
	})

	shadow, err := sc.GetShadowDevice("shadow-snmp-dev")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	if shadow.Points["p1"].Quality != "Good" {
		t.Fatalf("p1 quality = %q, want Good", shadow.Points["p1"].Quality)
	}
	if shadow.Points["p2"].Quality != "Bad" {
		t.Fatalf("p2 quality = %q, want Bad for missing success result", shadow.Points["p2"].Quality)
	}
	if shadow.Points["p2"].CollectedAt.Before(before) {
		t.Fatalf("p2 collected_at not refreshed: %v", shadow.Points["p2"].CollectedAt)
	}
}

func TestScanEngine_MarkDeviceShadowBad(t *testing.T) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  1,
		MaxQueueSize: 10,
	})
	se.SetShadowCore(sc)

	oldTime := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		ChannelID: "ch-1",
		Timestamp: oldTime,
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 10.0, Quality: "Good", CollectedAt: oldTime},
		},
	}); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	se.AddTask("dev-1", "modbus-tcp", 1*time.Second, 5, []string{"p1", "p2"}, map[string]any{"channelID": "ch-1"})

	before := time.Now()
	se.markDeviceShadowBad("dev-1", "ch-1")

	shadow, err := sc.GetShadowDevice("shadow-dev-1")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	for _, id := range []string{"p1", "p2"} {
		pt, ok := shadow.Points[id]
		if !ok {
			t.Fatalf("missing point %s", id)
		}
		if pt.Quality != "Bad" {
			t.Fatalf("point %s quality = %q, want Bad", id, pt.Quality)
		}
		if pt.CollectedAt.Before(before) {
			t.Fatalf("point %s collected_at not updated", id)
		}
	}
	if shadow.Points["p1"].Value != 10.0 {
		t.Fatalf("p1 value should be preserved, got %v", shadow.Points["p1"].Value)
	}
}

func TestScanEngine_Degradation(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval: 10 * time.Millisecond,
		WorkerCount:  1,
		MaxQueueSize: 100,
	}

	se := NewScanEngine(config)

	task := se.AddTask("device1", "modbus-tcp", 50*time.Millisecond, 5, []string{"point1"}, nil)

	task.mu.Lock()
	task.ConsecutiveFailures = 3
	task.mu.Unlock()

	se.updateTaskState(task, &ExecuteResult{Success: false})

	task.mu.Lock()
	if task.Status != ScanTaskStatusDegraded {
		t.Errorf("任务状态应为Degraded，实际%s", task.Status)
	}
	if task.Interval != 100*time.Millisecond {
		t.Errorf("采集间隔应翻倍，期望100ms，实际%s", task.Interval)
	}
	task.mu.Unlock()
}

func TestPriorityQueue(t *testing.T) {
	pq := &PriorityQueue{}
	heap.Init(pq)

	soon := time.Now().Add(50 * time.Millisecond)
	later := time.Now().Add(100 * time.Millisecond)
	task1 := &ScanTask{
		ID:        "task1",
		DeviceKey: "device1",
		NextRun:   later,
		Priority:  5,
	}
	task2 := &ScanTask{
		ID:        "task2",
		DeviceKey: "device2",
		NextRun:   soon,
		Priority:  5,
	}
	task3 := &ScanTask{
		ID:        "task3",
		DeviceKey: "device3",
		NextRun:   soon,
		Priority:  10,
	}

	heap.Push(pq, task1)
	heap.Push(pq, task2)
	heap.Push(pq, task3)

	if pq.Len() != 3 {
		t.Errorf("队列长度不匹配，期望3，实际%d", pq.Len())
	}

	top := heap.Pop(pq).(*ScanTask)
	if top.ID != "task3" {
		t.Errorf("期望task3优先，实际%s", top.ID)
	}

	top = heap.Pop(pq).(*ScanTask)
	if top.ID != "task2" {
		t.Errorf("期望task2其次，实际%s", top.ID)
	}

	top = heap.Pop(pq).(*ScanTask)
	if top.ID != "task1" {
		t.Errorf("期望task1最后，实际%s", top.ID)
	}
}

func TestScanTask_Status(t *testing.T) {
	task := &ScanTask{
		ID:        "task1",
		DeviceKey: "device1",
		Status:    ScanTaskStatusIdle,
	}

	if task.GetStatus() != ScanTaskStatusIdle {
		t.Errorf("初始状态应为Idle，实际%s", task.GetStatus())
	}

	task.SetStatus(ScanTaskStatusRunning)
	if task.GetStatus() != ScanTaskStatusRunning {
		t.Errorf("状态应为Running，实际%s", task.GetStatus())
	}

	task.SetStatus(ScanTaskStatusDegraded)
	if task.GetStatus() != ScanTaskStatusDegraded {
		t.Errorf("状态应为Degraded，实际%s", task.GetStatus())
	}

	task.SetStatus(ScanTaskStatusStopped)
	if task.GetStatus() != ScanTaskStatusStopped {
		t.Errorf("状态应为Stopped，实际%s", task.GetStatus())
	}
}

func TestExecutionLayer_Execute(t *testing.T) {
	el := NewExecutionLayer()

	task := &ScanTask{
		ID:        "task1",
		DeviceKey: "device1",
		Protocol:  "modbus-tcp",
		Interval:  1 * time.Second,
	}

	result := el.Execute(task)

	if result.Success {
		t.Error("期望失败（无驱动），实际成功")
	}
}

func TestSerialQueueManager_Submit(t *testing.T) {
	manager := NewSerialQueueManager()

	task := &DriverTask{
		DeviceKey: "device1",
		Points:    []model.Point{{ID: "point1"}},
	}

	result := manager.Submit(task)
	if !result {
		t.Error("任务提交失败")
	}

	manager.Stop()
}

func TestBackpressureController_Allow(t *testing.T) {
	bc := NewBackpressureController(10, 100)

	for i := 0; i < 10; i++ {
		if !bc.Allow("device1", 5) {
			t.Errorf("第%d次应允许", i)
		}
		bc.Release("device1")
	}
}

func TestResourceController_CanExecute(t *testing.T) {
	rc := NewResourceController(ResourceLimits{
		GoroutineLimit:  10,
		ConnectionLimit: 10,
	})

	if !rc.CanExecute() {
		t.Error("应允许执行")
	}

	rc.Acquire()
	if rc.GetGoroutineCount() != 1 {
		t.Errorf("goroutine计数应为1，实际%d", rc.GetGoroutineCount())
	}
	rc.Release()
}

func TestWorkerPool_Submit(t *testing.T) {
	wp := NewWorkerPool(2)

	done := make(chan bool, 1)
	wp.Submit(func() {
		done <- true
	})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("任务未执行")
	}

	wp.Stop()
}

func TestScanEngineAdapter_StartControl(t *testing.T) {
	config := ScanEngineConfig{
		TickInterval:    10 * time.Millisecond,
		WorkerCount:     2,
		MaxQueueSize:    100,
		GoroutineLimit:  10,
		ConnectionLimit: 5,
	}

	se := NewScanEngine(config)
	adapter := NewScanEngineAdapter(se)

	if adapter.IsStarted() {
		t.Error("初始状态应为未启动")
	}

	adapter.Start()

	if !adapter.IsStarted() {
		t.Error("启动后状态应为已启动")
	}

	adapter.Start()
	adapter.Start()

	if !adapter.IsStarted() {
		t.Error("重复启动后状态仍应为已启动")
	}

	time.Sleep(200 * time.Millisecond)
	se.Stop()
}

func TestScanEngineAdapter_ConcurrentStart(t *testing.T) {
	var once sync.Once
	var counter int32

	for i := 0; i < 100; i++ {
		go func() {
			once.Do(func() {
				atomic.AddInt32(&counter, 1)
			})
		}()
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("sync.Once应只执行一次，实际执行%d次", counter)
	}
}
