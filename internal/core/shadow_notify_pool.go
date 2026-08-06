package core

import (
	"hash/fnv"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

const defaultNotifyWorkers = 6

type notifyJob struct {
	deviceID string
	points   map[string]model.ShadowPoint
}

// shadowNotifyPool runs a fixed set of notify workers. Jobs for the same
// deviceID always route to the same worker (hash partition), giving
// best-effort per-device ordering; cross-device delivery is unordered.
type shadowNotifyPool struct {
	workers int
	queues  []chan notifyJob
	handler ShadowSubscriber
	wg      sync.WaitGroup
	stopCh  chan struct{}
	started bool
	mu      sync.Mutex
}

func newShadowNotifyPool(workers int, handler ShadowSubscriber) *shadowNotifyPool {
	if workers <= 0 {
		workers = defaultNotifyWorkers
	}
	return &shadowNotifyPool{
		workers: workers,
		queues:  make([]chan notifyJob, workers),
		handler: handler,
		stopCh:  make(chan struct{}),
	}
}

func (p *shadowNotifyPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	for i := 0; i < p.workers; i++ {
		q := make(chan notifyJob, 256)
		p.queues[i] = q
		p.wg.Add(1)
		go p.runWorker(q)
	}
	p.started = true
}

func (p *shadowNotifyPool) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	close(p.stopCh)
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *shadowNotifyPool) runWorker(q chan notifyJob) {
	defer p.wg.Done()
	for {
		select {
		case <-p.stopCh:
			p.drain(q)
			return
		case job, ok := <-q:
			if !ok {
				return
			}
			p.handler(job.deviceID, job.points)
		}
	}
}

func (p *shadowNotifyPool) drain(q chan notifyJob) {
	for {
		select {
		case job := <-q:
			p.handler(job.deviceID, job.points)
		default:
			return
		}
	}
}

func (p *shadowNotifyPool) Enqueue(deviceID string, points map[string]model.ShadowPoint) {
	if p.handler == nil || len(points) == 0 {
		return
	}
	p.mu.Lock()
	started := p.started
	p.mu.Unlock()
	if !started {
		go p.handler(deviceID, points)
		return
	}
	idx := p.workerIndex(deviceID)
	select {
	case p.queues[idx] <- notifyJob{deviceID: deviceID, points: points}:
	default:
		// Prefer freshest snapshot: drop one pending job and enqueue latest.
		// Never spawn unbounded goroutines under backpressure (causes GC storms).
		select {
		case <-p.queues[idx]:
		default:
		}
		select {
		case p.queues[idx] <- notifyJob{deviceID: deviceID, points: points}:
		default:
			// Still full — drop this notify.
		}
	}
}

func (p *shadowNotifyPool) workerIndex(deviceID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(deviceID))
	return int(h.Sum32() % uint32(p.workers))
}

func (p *shadowNotifyPool) WorkerCount() int {
	return p.workers
}
