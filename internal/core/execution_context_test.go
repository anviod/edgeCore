package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestSerialWorker_ReadViaDriver(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := &ExecutionContext{
		DeviceKey: "dev-serial",
		Queue:     make(chan *DriverTask, 1),
		Driver:    &execStubDriver{},
	}
	worker := &SerialWorker{ctx: ctx, stopCh: make(chan struct{}), wg: &wg}
	go worker.run()

	done := make(chan struct{})
	ctx.Queue <- &DriverTask{
		Ctx:    context.Background(),
		Points: []model.Point{{ID: "p1"}},
		Callback: func(values map[string]model.Value, err error) {
			if err != nil {
				t.Errorf("read error: %v", err)
			}
			if len(values) != 1 {
				t.Errorf("values = %d, want 1", len(values))
			}
			close(done)
		},
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("serial worker did not complete read")
	}

	close(ctx.Queue)
	close(worker.stopCh)
	wg.Wait()
}
