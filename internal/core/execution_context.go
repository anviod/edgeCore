package core

import (
	"sync"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type ExecutionContext struct {
	DeviceKey string
	Queue     chan *DriverTask
	Worker    *SerialWorker
	Driver    driver.Driver
	Running   bool
	mu        sync.Mutex
}

type SerialWorker struct {
	ctx    *ExecutionContext
	stopCh chan struct{}
	wg     *sync.WaitGroup
}

func (w *SerialWorker) run() {
	defer w.wg.Done()

	for {
		select {
		case task, ok := <-w.ctx.Queue:
			if !ok {
				return
			}

			w.ctx.mu.Lock()
			w.ctx.Running = true
			w.ctx.mu.Unlock()

			var values map[string]model.Value
			var err error
			if task.ReadFunc != nil {
				values, err = task.ReadFunc(task.Ctx, task.Points)
			} else if w.ctx.Driver != nil {
				values, err = w.ctx.Driver.ReadPoints(task.Ctx, task.Points)
			} else {
				err = ErrDriverNotFound
			}

			if task.Callback != nil {
				task.Callback(values, err)
			}

			w.ctx.mu.Lock()
			w.ctx.Running = false
			w.ctx.mu.Unlock()
		case <-w.stopCh:
			return
		}
	}
}
