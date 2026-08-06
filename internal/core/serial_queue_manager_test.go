package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestSerialQueueManager_SubmitAndDepth(t *testing.T) {
	sqm := NewSerialQueueManager()
	defer sqm.Stop()

	done := make(chan struct{})
	task := &DriverTask{
		Ctx:       context.Background(),
		DeviceKey: "dev-sq",
		Points:    []model.Point{{ID: "p1"}},
		ReadFunc: func(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
			close(done)
			return map[string]model.Value{"p1": {Quality: "Good", Value: 1}}, nil
		},
		Callback: func(_ map[string]model.Value, _ error) {},
	}

	if !sqm.Submit(task) {
		t.Fatal("Submit should succeed on empty queue")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task not processed")
	}

	depths := sqm.QueueDepths()
	if _, ok := depths["dev-sq"]; !ok {
		t.Fatalf("depths = %v", depths)
	}
}

func TestSerialQueueManager_SoftLimitReject(t *testing.T) {
	sqm := NewSerialQueueManager()
	defer sqm.Stop()

	block := make(chan struct{})
	sqm.RegisterDriver("dev-full", &execStubDriver{})

	for i := 0; i < serialQueueBufferSize; i++ {
		task := &DriverTask{
			Ctx:       context.Background(),
			DeviceKey: "dev-full",
			ReadFunc: func(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
				<-block
				return nil, nil
			},
			Callback: func(_ map[string]model.Value, _ error) {},
		}
		if !sqm.Submit(task) {
			break
		}
	}

	task := &DriverTask{
		Ctx:       context.Background(),
		DeviceKey: "dev-full",
		ReadFunc:  func(context.Context, []model.Point) (map[string]model.Value, error) { return nil, nil },
		Callback:  func(map[string]model.Value, error) {},
	}
	if sqm.Submit(task) {
		close(block)
		t.Fatal("Submit should reject when soft limit exceeded")
	}
	close(block)
}

func TestSerialQueueManager_RegisterAndRemove(t *testing.T) {
	sqm := NewSerialQueueManager()
	drv := &execStubDriver{}
	sqm.RegisterDriver("dev-reg", drv)
	sqm.RegisterDriver("dev-reg", drv)

	sqm.RemoveContext("dev-reg")
	if depths := sqm.QueueDepths(); len(depths) != 0 {
		t.Fatalf("depths after remove = %v", depths)
	}
	sqm.Stop()
}

func TestSerialQueueManager_ConcurrentSubmit(t *testing.T) {
	sqm := NewSerialQueueManager()
	defer sqm.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "dev-" + string(rune('a'+id))
			sqm.RegisterDriver(key, &execStubDriver{})
			sqm.Submit(&DriverTask{
				Ctx:       context.Background(),
				DeviceKey: key,
				ReadFunc:  func(context.Context, []model.Point) (map[string]model.Value, error) { return nil, nil },
				Callback:  func(map[string]model.Value, error) {},
			})
		}(i)
	}
	wg.Wait()
}
