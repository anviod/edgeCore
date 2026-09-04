package core

import (
	"context"
	"sync"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

const (
	serialQueueBufferSize   = 64
	serialQueueSoftLimitPct = 0.9
)

type DriverTask struct {
	Ctx       context.Context
	DeviceKey string
	Points    []model.Point
	ReadFunc  func(context.Context, []model.Point) (map[string]model.Value, error)
	Callback  func(map[string]model.Value, error)
}

type SerialQueueManager struct {
	contexts map[string]*ExecutionContext
	mu       sync.RWMutex
	wg       sync.WaitGroup
}

func NewSerialQueueManager() *SerialQueueManager {
	return &SerialQueueManager{
		contexts: make(map[string]*ExecutionContext),
	}
}

func (sqm *SerialQueueManager) Submit(task *DriverTask) bool {
	sqm.mu.Lock()
	ctx, ok := sqm.contexts[task.DeviceKey]
	if !ok {
		ctx = sqm.createContext(task.DeviceKey)
	}
	sqm.mu.Unlock()

	softLimit := int(float64(cap(ctx.Queue)) * serialQueueSoftLimitPct)
	if len(ctx.Queue) > softLimit {
		return false
	}

	select {
	case ctx.Queue <- task:
		return true
	default:
		return false
	}
}

func (sqm *SerialQueueManager) RegisterDriver(deviceKey string, d driver.Driver) {
	sqm.mu.Lock()
	defer sqm.mu.Unlock()

	if ctx, ok := sqm.contexts[deviceKey]; ok {
		ctx.mu.Lock()
		ctx.Driver = d
		ctx.mu.Unlock()
		return
	}

	ctx := sqm.createContext(deviceKey)
	ctx.mu.Lock()
	ctx.Driver = d
	ctx.mu.Unlock()
}

func (sqm *SerialQueueManager) createContext(deviceKey string) *ExecutionContext {
	ctx := &ExecutionContext{
		DeviceKey: deviceKey,
		Queue:     make(chan *DriverTask, serialQueueBufferSize),
	}

	worker := &SerialWorker{
		ctx:    ctx,
		stopCh: make(chan struct{}),
		wg:     &sqm.wg,
	}

	ctx.Worker = worker

	sqm.wg.Add(1)
	go worker.run()

	sqm.contexts[deviceKey] = ctx
	return ctx
}

func (sqm *SerialQueueManager) RemoveContext(deviceKey string) {
	sqm.mu.Lock()
	defer sqm.mu.Unlock()

	if ctx, ok := sqm.contexts[deviceKey]; ok {
		close(ctx.Worker.stopCh)
		delete(sqm.contexts, deviceKey)
	}
}

func (sqm *SerialQueueManager) Stop() {
	sqm.mu.Lock()
	defer sqm.mu.Unlock()

	for _, ctx := range sqm.contexts {
		close(ctx.Worker.stopCh)
		close(ctx.Queue)
	}

	sqm.wg.Wait()
	sqm.contexts = make(map[string]*ExecutionContext)
}

func (sqm *SerialQueueManager) QueueDepths() map[string]int {
	sqm.mu.RLock()
	defer sqm.mu.RUnlock()

	depths := make(map[string]int, len(sqm.contexts))
	for key, ctx := range sqm.contexts {
		if ctx == nil || ctx.Queue == nil {
			continue
		}
		depths[key] = len(ctx.Queue)
	}
	return depths
}
