package core

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type WorkerPool struct {
	workers      []*Worker
	taskQueue    chan func()
	wg           sync.WaitGroup
	stopCh       chan struct{}
	maxQueueSize int
	mu           sync.Mutex
	stopOnce     sync.Once
	stopped      atomic.Bool
	activeCount  int
}

type Worker struct {
	id        int
	taskQueue chan func()
	stopCh    chan struct{}
	wg        *sync.WaitGroup
}

func NewWorkerPool(workerCount int) *WorkerPool {
	if workerCount <= 0 {
		workerCount = 4
	}

	wp := &WorkerPool{
		taskQueue:    make(chan func(), 1000),
		stopCh:       make(chan struct{}),
		maxQueueSize: 1000,
		workers:      make([]*Worker, 0, workerCount),
	}

	for i := 0; i < workerCount; i++ {
		worker := NewWorker(i, wp.taskQueue, wp.stopCh, &wp.wg)
		wp.workers = append(wp.workers, worker)
	}

	return wp
}

func NewWorker(id int, taskQueue chan func(), stopCh chan struct{}, wg *sync.WaitGroup) *Worker {
	w := &Worker{
		id:        id,
		taskQueue: taskQueue,
		stopCh:    stopCh,
		wg:        wg,
	}

	w.wg.Add(1)
	go w.run()

	return w
}

func (w *Worker) run() {
	defer w.wg.Done()

	for {
		select {
		case task, ok := <-w.taskQueue:
			if !ok {
				return
			}
			if task != nil {
				task()
			}
		case <-w.stopCh:
			return
		}
	}
}

func (wp *WorkerPool) Start() {
	zap.L().Info("[WorkerPool] Worker池已启动",
		zap.Int("workerCount", len(wp.workers)),
		zap.Int("maxQueueSize", wp.maxQueueSize),
	)
}

func (wp *WorkerPool) Stop() {
	wp.stopOnce.Do(func() {
		wp.stopped.Store(true)
		close(wp.stopCh)
		wp.wg.Wait()

		wp.mu.Lock()
		close(wp.taskQueue)
		wp.mu.Unlock()

		zap.L().Info("[WorkerPool] Worker池已停止")
	})
}

func (wp *WorkerPool) Submit(task func()) bool {
	if wp.stopped.Load() {
		return false
	}

	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.stopped.Load() {
		return false
	}

	select {
	case wp.taskQueue <- func() {
		defer func() {
			wp.mu.Lock()
			wp.activeCount--
			wp.mu.Unlock()
		}()
		task()
	}:
		wp.activeCount++
		return true
	default:
		zap.L().Warn("[WorkerPool] 任务队列已满")
		return false
	}
}

func (wp *WorkerPool) PendingCount() int {
	return len(wp.taskQueue)
}

func (wp *WorkerPool) ActiveCount() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.activeCount
}

func (wp *WorkerPool) SetWorkerCount(count int) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	currentCount := len(wp.workers)

	if count == currentCount {
		return
	}

	if count > currentCount {
		for i := currentCount; i < count; i++ {
			worker := NewWorker(i, wp.taskQueue, wp.stopCh, &wp.wg)
			wp.workers = append(wp.workers, worker)
		}
	} else {
		// Workers share stopCh; extra workers exit on pool Stop().
		wp.workers = wp.workers[:count]
	}

	zap.L().Info("[WorkerPool] Worker数量已更新",
		zap.Int("workerCount", len(wp.workers)),
	)
}

func (wp *WorkerPool) WaitForIdle(timeout time.Duration) bool {
	start := time.Now()

	for {
		wp.mu.Lock()
		idle := len(wp.taskQueue) == 0 && wp.activeCount == 0
		wp.mu.Unlock()
		if idle {
			return true
		}

		if time.Since(start) >= timeout {
			return false
		}

		time.Sleep(10 * time.Millisecond)
	}
}
