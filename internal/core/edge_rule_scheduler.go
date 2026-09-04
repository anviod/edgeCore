package core

import (
	"sort"
	"sync"
	"time"
)

const (
	defaultEdgeBatchWindow = 250 * time.Millisecond
	defaultEdgeWorkerCount = 10
	defaultEdgeQueueSize   = 1000
	defaultEdgePendingCap  = 5000
)

// edgeRuleScheduler coalesces per-rule triggers within a batch window and
// dispatches by priority within batch_window_ms.
type edgeRuleScheduler struct {
	em          *EdgeComputeManager
	batchWindow time.Duration
	pending     map[string]*ruleTask
	mu          sync.Mutex
	flushTimer  *time.Timer
	stopped     bool
}

func newEdgeRuleScheduler(em *EdgeComputeManager, batchWindow time.Duration) *edgeRuleScheduler {
	if batchWindow < 0 {
		batchWindow = defaultEdgeBatchWindow
	}
	return &edgeRuleScheduler{
		em:          em,
		batchWindow: batchWindow,
		pending:     make(map[string]*ruleTask),
	}
}

// setBatchWindow updates the batch window in a thread-safe manner.
func (s *edgeRuleScheduler) setBatchWindow(d time.Duration) {
	s.mu.Lock()
	s.batchWindow = d
	s.mu.Unlock()
}

func (s *edgeRuleScheduler) schedule(task *ruleTask) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	if _, exists := s.pending[task.rule.ID]; !exists && len(s.pending) >= defaultEdgePendingCap {
		if dropped := s.evictLowestPriorityLocked(); dropped != nil {
			s.mu.Unlock()
			s.em.onSchedulerDrop(dropped, "coalesce_pending_cap")
			s.mu.Lock()
		}
	}

	if _, exists := s.pending[task.rule.ID]; exists {
		s.em.incRulesCoalesced()
	}
	s.pending[task.rule.ID] = task

	if s.batchWindow <= 0 {
		task := s.pending[task.rule.ID]
		delete(s.pending, task.rule.ID)
		s.mu.Unlock()
		s.em.dispatchTask(task)
		return
	}

	if s.flushTimer == nil {
		s.flushTimer = time.AfterFunc(s.batchWindow, s.onFlush)
	} else {
		s.flushTimer.Reset(s.batchWindow)
	}
	s.mu.Unlock()
}

func (s *edgeRuleScheduler) onFlush() {
	s.mu.Lock()
	if s.stopped || len(s.pending) == 0 {
		s.flushTimer = nil
		s.mu.Unlock()
		return
	}
	tasks := make([]*ruleTask, 0, len(s.pending))
	for _, task := range s.pending {
		tasks = append(tasks, task)
	}
	s.pending = make(map[string]*ruleTask)
	s.flushTimer = nil
	s.mu.Unlock()

	sort.Slice(tasks, func(i, j int) bool {
		pi, pj := tasks[i].rule.Priority, tasks[j].rule.Priority
		if pi != pj {
			return pi > pj
		}
		return tasks[i].rule.ID < tasks[j].rule.ID
	})

	s.em.incRulesDebounced(int64(len(tasks)))
	for _, task := range tasks {
		s.em.dispatchTask(task)
	}
}

func (s *edgeRuleScheduler) pendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func (s *edgeRuleScheduler) evictLowestPriorityLocked() *ruleTask {
	if len(s.pending) == 0 {
		return nil
	}
	var victimID string
	var victimPri int
	first := true
	for id, task := range s.pending {
		if first || task.rule.Priority < victimPri || (task.rule.Priority == victimPri && id > victimID) {
			victimID = id
			victimPri = task.rule.Priority
			first = false
		}
	}
	if victimID == "" {
		return nil
	}
	victim := s.pending[victimID]
	delete(s.pending, victimID)
	return victim
}

func (s *edgeRuleScheduler) stop() {
	s.mu.Lock()
	s.stopped = true
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	pending := s.pending
	s.pending = make(map[string]*ruleTask)
	s.mu.Unlock()

	if len(pending) == 0 {
		return
	}
	tasks := make([]*ruleTask, 0, len(pending))
	for _, task := range pending {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].rule.Priority > tasks[j].rule.Priority
	})
	for _, task := range tasks {
		s.em.dispatchTask(task)
	}
}
