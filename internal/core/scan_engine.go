package core

import (
	"container/heap"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

type ScanEngineConfig struct {
	TickInterval      time.Duration
	WorkerCount       int
	MaxQueueSize      int
	AntiStarvationSec int
	PriorityLevels    int
	GoroutineLimit    int
	ConnectionLimit   int
	JitterBound       time.Duration
}

type ScanTaskStatus int

const (
	ScanTaskStatusIdle ScanTaskStatus = iota
	ScanTaskStatusRunning
	ScanTaskStatusDegraded
	ScanTaskStatusStopped
)

func (s ScanTaskStatus) String() string {
	switch s {
	case ScanTaskStatusIdle:
		return "Idle"
	case ScanTaskStatusRunning:
		return "Running"
	case ScanTaskStatusDegraded:
		return "Degraded"
	case ScanTaskStatusStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

type ScanTask struct {
	ID                  string
	DeviceKey           string
	ScanClass           string
	Protocol            string
	Interval            time.Duration
	BaseInterval        time.Duration
	NextRun             time.Time
	LastScheduledAt     time.Time
	DeadlineAt          time.Time
	PhaseOffset         time.Duration
	Priority            int
	FailRate            float64
	Status              ScanTaskStatus
	ConsecutiveFailures int
	ConsecutiveSuccess  int
	LastSuccess         time.Time
	LastFailure         time.Time
	PointIDs            []string
	Points              []model.Point
	pointsScratch       []model.Point
	Params              map[string]any
	mu                  sync.RWMutex
}

func (t *ScanTask) GetStatus() ScanTaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status
}

func (t *ScanTask) SetStatus(status ScanTaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
}

func (t *ScanTask) UpdateNextRun(interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.LastScheduledAt = now
	t.NextRun = now.Add(interval)
}

func taskJitterBound(bound time.Duration) time.Duration {
	if bound <= 0 {
		return 50 * time.Millisecond
	}
	return bound
}

func taskDeterministicJitter(taskID string, bound time.Duration) time.Duration {
	bound = taskJitterBound(bound)
	if bound <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskID))
	return time.Duration(h.Sum32() % uint32(bound.Nanoseconds()))
}

type PriorityQueue []*ScanTask

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	pq[i].mu.RLock()
	iNext, iDeadline, iPriority := pq[i].NextRun, pq[i].DeadlineAt, pq[i].Priority
	pq[i].mu.RUnlock()
	pq[j].mu.RLock()
	jNext, jDeadline, jPriority := pq[j].NextRun, pq[j].DeadlineAt, pq[j].Priority
	pq[j].mu.RUnlock()
	if iNext.Before(jNext) {
		return true
	}
	if jNext.Before(iNext) {
		return false
	}
	// EDF tie-break: earliest DeadlineAt among same NextRun.
	if !iDeadline.IsZero() && !jDeadline.IsZero() {
		if iDeadline.Before(jDeadline) {
			return true
		}
		if jDeadline.Before(iDeadline) {
			return false
		}
	}
	return iPriority > jPriority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*ScanTask))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Peek() *ScanTask {
	if len(*pq) == 0 {
		return nil
	}
	return (*pq)[0]
}

// CollectFinalizeFunc 采集完成后回写设备通信状态。
type CollectFinalizeFunc func(deviceID string, result *ExecuteResult)

const antiStarvationWarnInterval = 60 * time.Second

type ScanEngine struct {
	tasks             map[string]*ScanTask
	priorityQueue     *PriorityQueue
	executionLayer    *ExecutionLayer
	resourceCtrl      *ResourceController
	shadowCore        *ShadowCore
	shadowIngress     *ShadowIngress
	pointDegrade      *PointDegradationManager
	collectFinalize   CollectFinalizeFunc
	metrics           *ScanEngineMetrics
	adaptiveThrottle  *AdaptiveThrottle
	gcMonitor         *GCMonitor
	feedbackAgg       *FeedbackAggregator
	feedbackPending   map[string]*ScanTask
	feedbackPendingMu sync.Mutex
	config            ScanEngineConfig
	ticker            *time.Ticker
	running           bool
	stopCh            chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
	taskIDCounter     int
	overdueWarnAt     map[string]time.Time
}

func NewScanEngine(config ScanEngineConfig) *ScanEngine {
	if config.TickInterval == 0 {
		config.TickInterval = 10 * time.Millisecond
	}
	if config.WorkerCount == 0 {
		config.WorkerCount = 4
	}
	if config.MaxQueueSize == 0 {
		config.MaxQueueSize = 10000
	}
	if config.AntiStarvationSec == 0 {
		config.AntiStarvationSec = 300
	}
	if config.PriorityLevels == 0 {
		config.PriorityLevels = 10
	}
	if config.GoroutineLimit == 0 {
		config.GoroutineLimit = 2048
	}
	if config.ConnectionLimit == 0 {
		config.ConnectionLimit = 500
	}
	config.JitterBound = taskJitterBound(config.JitterBound)

	se := &ScanEngine{
		tasks:           make(map[string]*ScanTask),
		feedbackPending: make(map[string]*ScanTask),
		overdueWarnAt:   make(map[string]time.Time),
		priorityQueue:   &PriorityQueue{},
		executionLayer:  NewExecutionLayer(),
		resourceCtrl: NewResourceController(ResourceLimits{
			GoroutineLimit:  config.GoroutineLimit,
			ConnectionLimit: config.ConnectionLimit,
			QueueLimit:      config.MaxQueueSize,
		}),
		shadowCore: nil,
		metrics: &ScanEngineMetrics{
			lagSamples: make([]int64, 0, scanLagSampleCap),
		},
		config: config,
		stopCh: make(chan struct{}),
	}

	se.adaptiveThrottle = NewAdaptiveThrottle(se.metrics)
	se.feedbackAgg = NewFeedbackAggregator(2*time.Second, func(deviceKey string, stats AggregatedStats) {
		se.applyAggregatedFeedback(deviceKey, stats)
	})
	se.gcMonitor = NewGCMonitor(func(pauseMaxMs float64) {
		if se.executionLayer != nil {
			se.executionLayer.ReduceBackpressureRate(gcBackpressureRateFactor)
		}
	})

	heap.Init(se.priorityQueue)

	return se
}

func (se *ScanEngine) Run() {
	se.mu.Lock()
	if se.running {
		se.mu.Unlock()
		return
	}
	se.running = true
	se.mu.Unlock()

	se.wg.Add(1)
	go se.dispatchLoop()

	se.wg.Add(1)
	go se.resourceCtrl.Monitor(&se.wg)

	if se.executionLayer != nil {
		se.executionLayer.Start()
	}

	if se.feedbackAgg != nil {
		se.feedbackAgg.Start()
	}

	if se.gcMonitor != nil {
		se.gcMonitor.Start()
	}

	se.wg.Add(1)
	go se.slaWarningLoop()

	zap.L().Info("[ScanEngine] 调度引擎已启动",
		zap.String("tickInterval", se.config.TickInterval.String()),
		zap.Int("workerCount", se.config.WorkerCount),
		zap.Int("maxQueueSize", se.config.MaxQueueSize),
		zap.Int("antiStarvationSec", se.config.AntiStarvationSec),
		zap.Int("goroutineLimit", se.config.GoroutineLimit),
		zap.Int("connectionLimit", se.config.ConnectionLimit),
	)
}

func (se *ScanEngine) Stop() {
	se.mu.Lock()
	if !se.running {
		se.mu.Unlock()
		return
	}
	se.running = false
	se.mu.Unlock()

	close(se.stopCh)

	if se.ticker != nil {
		se.ticker.Stop()
	}

	if se.gcMonitor != nil {
		se.gcMonitor.Stop()
	}

	if se.feedbackAgg != nil {
		se.feedbackAgg.Stop()
	}

	se.resourceCtrl.Stop()
	if se.executionLayer != nil {
		se.executionLayer.Stop()
	}

	se.wg.Wait()

	zap.L().Info("[ScanEngine] 调度引擎已停止")
}

func (se *ScanEngine) fallbackTickInterval() time.Duration {
	tick := se.config.TickInterval
	if se.config.MaxQueueSize <= 0 {
		return tick
	}
	loadRatio := float64(se.GetPendingTaskCount()) / float64(se.config.MaxQueueSize)
	if loadRatio > 0.7 {
		return 50 * time.Millisecond
	}
	return tick
}

func (se *ScanEngine) dispatchLoop() {
	defer se.wg.Done()

	fallbackTick := se.fallbackTickInterval()
	fallback := time.NewTicker(fallbackTick)
	defer fallback.Stop()

	var wakeTimer *time.Timer
	var wakeCh <-chan time.Time

	scheduleWake := func() {
		next := se.nextReadyTime()
		if next.IsZero() {
			if wakeTimer != nil {
				if !wakeTimer.Stop() {
					select {
					case <-wakeTimer.C:
					default:
					}
				}
				wakeTimer = nil
				wakeCh = nil
			}
			return
		}

		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}

		if wakeTimer == nil {
			wakeTimer = time.NewTimer(delay)
			wakeCh = wakeTimer.C
			return
		}

		if !wakeTimer.Stop() {
			select {
			case <-wakeTimer.C:
			default:
			}
		}
		wakeTimer.Reset(delay)
		wakeCh = wakeTimer.C
	}

	scheduleWake()

	for {
		select {
		case <-se.stopCh:
			if wakeTimer != nil {
				wakeTimer.Stop()
			}
			return
		case <-wakeCh:
			se.processReadyTasks()
			scheduleWake()
		case <-fallback.C:
			se.processReadyTasks()
			scheduleWake()
			if newTick := se.fallbackTickInterval(); newTick != fallbackTick {
				fallbackTick = newTick
				fallback.Reset(fallbackTick)
			}
		}
	}
}

func (se *ScanEngine) nextReadyTime() time.Time {
	se.mu.RLock()
	defer se.mu.RUnlock()
	if se.priorityQueue.Len() == 0 {
		return time.Time{}
	}
	task := (*se.priorityQueue)[0]
	if task == nil {
		return time.Time{}
	}
	task.mu.RLock()
	next := task.NextRun
	task.mu.RUnlock()
	return next
}

func (se *ScanEngine) processReadyTasks() {
	now := time.Now()

	if se.adaptiveThrottle != nil {
		se.adaptiveThrottle.Refresh(
			se.GetPendingTaskCount(),
			se.config.MaxQueueSize,
			se.metrics.GlobalFailRate(),
			se.metrics.AvgLagMs(),
		)
	}

	for {
		task := se.popReadyTaskEDF(now)
		if task == nil {
			break
		}

		if !se.resourceCtrl.CanExecute() {
			se.mu.Lock()
			heap.Push(se.priorityQueue, task)
			se.mu.Unlock()
			break
		}

		if task.GetStatus() == ScanTaskStatusStopped {
			continue
		}

		se.resourceCtrl.Acquire()
		go se.executeTaskAsync(task)
	}

	// Clamp only tasks still queued after dispatch; avoids counting a miss
	// for work dispatched in the same tick.
	se.enforceHardJitterClamp(time.Now())

	se.enforceAntiStarvation(now)
}

// popReadyTaskEDF removes the ready task with the earliest DeadlineAt (EDF).
func (se *ScanEngine) popReadyTaskEDF(now time.Time) *ScanTask {
	se.mu.Lock()
	defer se.mu.Unlock()

	pq := se.priorityQueue
	if pq.Len() == 0 {
		return nil
	}

	bestIdx := -1
	for i, task := range *pq {
		task.mu.RLock()
		nextRun, deadline, priority := task.NextRun, task.DeadlineAt, task.Priority
		task.mu.RUnlock()
		if now.Before(nextRun) {
			continue
		}
		if bestIdx < 0 {
			bestIdx = i
			continue
		}
		best := (*pq)[bestIdx]
		best.mu.RLock()
		bestDeadline := best.DeadlineAt
		bestPriority := best.Priority
		best.mu.RUnlock()
		if deadline.Before(bestDeadline) {
			bestIdx = i
		} else if deadline.Equal(bestDeadline) && priority > bestPriority {
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	return heap.Remove(pq, bestIdx).(*ScanTask)
}

// enforceHardJitterClamp forces immediate dispatch when now exceeds DeadlineAt.
func (se *ScanEngine) enforceHardJitterClamp(now time.Time) {
	se.mu.Lock()
	defer se.mu.Unlock()

	pq := se.priorityQueue
	for i := 0; i < pq.Len(); i++ {
		task := (*pq)[i]
		task.mu.RLock()
		deadline := task.DeadlineAt
		task.mu.RUnlock()
		if deadline.IsZero() || !now.After(deadline) {
			continue
		}
		if task.GetStatus() != ScanTaskStatusIdle {
			continue
		}
		se.metrics.RecordMissDeadlineForChannel(taskShadowChannelID(task))
		task.mu.Lock()
		se.boostPriorityOnMiss(task)
		task.NextRun = now
		task.LastScheduledAt = now
		task.DeadlineAt = now
		task.mu.Unlock()
		heap.Fix(pq, i)
	}
}

func (se *ScanEngine) boostPriorityOnMiss(task *ScanTask) {
	if task == nil {
		return
	}
	boost := task.Priority + 2
	if boost > se.config.PriorityLevels {
		boost = se.config.PriorityLevels
	}
	if boost < 1 {
		boost = 1
	}
	task.Priority = boost
}

func (se *ScanEngine) enforceAntiStarvation(now time.Time) {
	antiStarvationDuration := time.Duration(se.config.AntiStarvationSec) * time.Second

	se.mu.Lock()
	defer se.mu.Unlock()

	for _, task := range se.tasks {
		task.mu.Lock()
		if task.NextRun.IsZero() {
			task.mu.Unlock()
			continue
		}
		if now.Sub(task.NextRun) > antiStarvationDuration {
			se.metrics.RecordOverdue()
			lastWarn, warned := se.overdueWarnAt[task.ID]
			if !warned || now.Sub(lastWarn) >= antiStarvationWarnInterval {
				se.overdueWarnAt[task.ID] = now
				zap.L().Warn("[防饿死] 任务超过预期执行时间",
					zap.String("taskID", task.ID),
					zap.String("deviceKey", task.DeviceKey),
					zap.Duration("overdue", now.Sub(task.NextRun)),
				)
			}
			if task.Status == ScanTaskStatusIdle {
				se.metrics.RecordStarvationRescue()
				task.Priority = 10
				task.LastScheduledAt = now
				task.NextRun = now
				task.DeadlineAt = now.Add(taskJitterBound(se.config.JitterBound))
				task.mu.Unlock()
				heap.Push(se.priorityQueue, task)
				continue
			}
		}
		task.mu.Unlock()
	}
}

func (se *ScanEngine) executeTaskAsync(task *ScanTask) {
	defer se.resourceCtrl.Release()

	task.SetStatus(ScanTaskStatusRunning)

	task.mu.RLock()
	scheduledAt := task.NextRun
	task.mu.RUnlock()
	start := time.Now()
	lagMicros := start.Sub(scheduledAt).Microseconds()
	if lagMicros < 0 {
		lagMicros = 0
	}

	var result *ExecuteResult
	if se.executionLayer != nil {
		result = se.executionLayer.Execute(task)
	} else {
		result = &ExecuteResult{Success: false, Error: ErrDriverNotFound}
	}

	if result.Success && se.shadowCore != nil {
		rttMicros := time.Since(start).Microseconds()
		se.shadowCore.UpdateDeviceRTT(task.DeviceKey, rttMicros)
		if se.adaptiveThrottle != nil {
			se.adaptiveThrottle.UpdateDeviceRTT(task.DeviceKey, float64(rttMicros)/1000.0)
		}
	}
	se.metrics.RecordExecuteForChannel(taskShadowChannelID(task), result != nil && result.Success, lagMicros)

	se.applyCollectToShadow(task, result)

	if result != nil && (result.Success || errors.Is(result.Error, ErrCircuitOpen)) {
		se.updateTaskState(task, result)
	} else if se.feedbackAgg != nil {
		se.feedbackPendingMu.Lock()
		se.feedbackPending[task.DeviceKey] = task
		se.feedbackPendingMu.Unlock()
		var failErr error
		if result != nil {
			failErr = result.Error
		}
		se.feedbackAgg.Submit(FeedbackEvent{
			DeviceKey: task.DeviceKey,
			TaskID:    task.ID,
			Success:   false,
			Err:       failErr,
			At:        time.Now(),
			LagMicros: lagMicros,
		})
	} else {
		se.updateTaskState(task, result)
	}

	if se.collectFinalize != nil {
		se.collectFinalize(task.DeviceKey, result)
	}

	task.SetStatus(ScanTaskStatusIdle)

	se.mu.RLock()
	running := se.running
	se.mu.RUnlock()

	if !running {
		return
	}

	se.rescheduleTask(task, time.Now())

	se.mu.Lock()
	if se.running {
		heap.Push(se.priorityQueue, task)
	}
	se.mu.Unlock()
}

func (se *ScanEngine) rescheduleTask(task *ScanTask, completedAt time.Time) {
	task.mu.Lock()
	defer task.mu.Unlock()

	interval := task.Interval
	if interval <= 0 {
		interval = time.Millisecond
	}

	anchor := task.LastScheduledAt
	if anchor.IsZero() {
		anchor = task.NextRun
	}
	if anchor.IsZero() {
		anchor = completedAt
	}

	next := anchor.Add(interval)
	jitterBound := taskJitterBound(se.config.JitterBound)

	channelID := taskShadowChannelID(task)
	for next.Before(completedAt) {
		if !task.DeadlineAt.IsZero() && completedAt.After(task.DeadlineAt) {
			se.metrics.RecordMissDeadlineForChannel(channelID)
			se.boostPriorityOnMiss(task)
		}
		drift := completedAt.Sub(next)
		if drift > 0 {
			se.metrics.RecordDriftForChannel(channelID, drift.Microseconds())
		}
		next = next.Add(interval)
	}

	jitter := taskDeterministicJitter(task.ID, jitterBound)

	task.LastScheduledAt = next
	task.NextRun = next.Add(jitter)
	task.DeadlineAt = task.NextRun.Add(jitterBound)
}

func taskCollectPointIDs(task *ScanTask) []string {
	if len(task.Points) > 0 {
		ids := make([]string, len(task.Points))
		for i, p := range task.Points {
			ids[i] = p.ID
		}
		return ids
	}
	return task.PointIDs
}

func taskShadowChannelID(task *ScanTask) string {
	if task.Params != nil {
		if id, ok := task.Params["channelID"].(string); ok {
			return id
		}
	}
	return ""
}

func resolveCollectQuality(v model.Value) string {
	if v.Quality != "" {
		return v.Quality
	}
	if v.Value == nil {
		return "Bad"
	}
	return "Good"
}

func preservedShadowValue(existing *model.ShadowDevice, pointID string, newVal any, quality string) any {
	if newVal != nil {
		return newVal
	}
	if quality == "Good" {
		return nil
	}
	if existing != nil {
		if sp, ok := existing.Points[pointID]; ok && sp.Value != nil {
			return sp.Value
		}
	}
	return nil
}

func (se *ScanEngine) writeShadowMessage(msg model.ShadowIngressMessage) {
	if se.shadowIngress != nil {
		se.shadowIngress.IngestDirect(msg)
	} else if se.shadowCore != nil {
		se.shadowCore.WriteShadowDevice(msg)
	}
}

// applyCollectToShadow writes scan results to shadow, including Bad quality on
// failed reads so stale Good values are not left behind when collection fails.
func (se *ScanEngine) applyCollectToShadow(task *ScanTask, result *ExecuteResult) {
	if se.shadowCore == nil && se.shadowIngress == nil {
		return
	}

	pointIDs := taskCollectPointIDs(task)
	if len(pointIDs) == 0 {
		return
	}

	now := time.Now()
	shadowID := fmt.Sprintf("shadow-%s", task.DeviceKey)
	existing, _ := se.shadowCore.GetShadowDevice(shadowID)

	resultValues := map[string]model.Value{}
	if result != nil && len(result.Values) > 0 {
		resultValues = result.Values
	}

	samplePeriodMs := int(task.Interval.Milliseconds())
	raw := borrowShadowIngressPointSlice(len(pointIDs))
	points := (*raw)[:0]

	for _, pointID := range pointIDs {
		value, inResult := resultValues[pointID]
		if !inResult {
			// Missing entries on both failed and successful collects must not
			// leave prior Good values untouched (e.g. SNMP parse skips, Modbus cooldown).
			value = model.Value{Quality: "Bad"}
		}

		quality := resolveCollectQuality(value)
		collectedAt := value.TS
		if collectedAt.IsZero() {
			collectedAt = now
		}
		val := preservedShadowValue(existing, pointID, value.Value, quality)

		degraded := false
		if se.pointDegrade != nil {
			degraded = se.pointDegrade.IsDegraded(task.DeviceKey, pointID)
		}
		points = append(points, model.ShadowIngressPoint{
			PointID:        pointID,
			Value:          val,
			Quality:        quality,
			SamplePeriodMs: samplePeriodMs,
			CollectedAt:    collectedAt,
			Degraded:       degraded,
		})
	}

	if len(points) == 0 {
		returnShadowIngressPointSlice(raw)
		return
	}

	msgPoints := make([]model.ShadowIngressPoint, len(points))
	copy(msgPoints, points)
	returnShadowIngressPointSlice(raw)

	se.writeShadowMessage(model.ShadowIngressMessage{
		DeviceID:  task.DeviceKey,
		ChannelID: taskShadowChannelID(task),
		Timestamp: now,
		Points:    msgPoints,
		Meta: model.ShadowIngressMeta{
			Source: "scan_engine",
		},
	})
}

// markDeviceShadowBad marks all known shadow points Bad when a device goes offline
// (e.g. channel connect failure) so stale Good values are not left behind.
func (se *ScanEngine) markDeviceShadowBad(deviceKey, channelID string) {
	if (se.shadowCore == nil && se.shadowIngress == nil) || deviceKey == "" {
		return
	}

	pointIDs := make(map[string]struct{})
	for _, task := range se.GetTasksByDeviceKey(deviceKey) {
		for _, pid := range taskCollectPointIDs(task) {
			pointIDs[pid] = struct{}{}
		}
	}

	shadowID := fmt.Sprintf("shadow-%s", deviceKey)
	existing, _ := se.shadowCore.GetShadowDevice(shadowID)
	if len(pointIDs) == 0 && existing != nil {
		for pid := range existing.Points {
			pointIDs[pid] = struct{}{}
		}
	}
	if len(pointIDs) == 0 {
		return
	}

	now := time.Now()
	ingress := make([]model.ShadowIngressPoint, 0, len(pointIDs))
	for pid := range pointIDs {
		ingress = append(ingress, model.ShadowIngressPoint{
			PointID:     pid,
			Value:       preservedShadowValue(existing, pid, nil, "Bad"),
			Quality:     "Bad",
			CollectedAt: now,
		})
	}

	se.writeShadowMessage(model.ShadowIngressMessage{
		DeviceID:  deviceKey,
		ChannelID: channelID,
		Timestamp: now,
		Points:    ingress,
		Meta: model.ShadowIngressMeta{
			Source: "channel_offline",
		},
	})
}

func (se *ScanEngine) applyAggregatedFeedback(deviceKey string, stats AggregatedStats) {
	se.feedbackPendingMu.Lock()
	task := se.feedbackPending[deviceKey]
	delete(se.feedbackPending, deviceKey)
	se.feedbackPendingMu.Unlock()
	if task == nil {
		return
	}
	se.updateTaskStateAggregated(task, stats)
}

func (se *ScanEngine) updateTaskStateAggregated(task *ScanTask, stats AggregatedStats) {
	task.mu.Lock()

	if stats.FailCount == 0 {
		task.ConsecutiveSuccess += stats.SuccessCount
		task.ConsecutiveFailures = 0
		task.LastSuccess = time.Now()
		task.FailRate = 0
		task.Status = ScanTaskStatusIdle
		if task.BaseInterval > 0 && task.Interval != task.BaseInterval {
			task.Interval = task.BaseInterval
		}
		if stats.SuccessCount > 0 && task.Priority < 10 {
			task.Priority++
		}
	} else {
		task.ConsecutiveFailures += stats.FailCount
		task.ConsecutiveSuccess = 0
		task.LastFailure = time.Now()
		task.FailRate = stats.FailRate

		if stats.FailCount >= 3 && se.taskDegradeOnFailure(task) {
			shift := task.ConsecutiveFailures - 3
			if shift > 6 {
				shift = 6
			}
			newInterval := task.Interval * (1 << shift)
			if newInterval > 64*time.Second {
				newInterval = 64 * time.Second
			}
			if task.BaseInterval > 0 && newInterval < task.BaseInterval {
				newInterval = task.BaseInterval
			}
			if newInterval < time.Millisecond {
				newInterval = time.Millisecond
			}
			if newInterval != task.Interval {
				zap.L().Warn("[降级] 窗口内失败率过高，调整采集间隔",
					zap.String("taskID", task.ID),
					zap.String("deviceKey", task.DeviceKey),
					zap.Int("failCount", stats.FailCount),
					zap.Float64("failRate", stats.FailRate),
					zap.Duration("oldInterval", task.Interval),
					zap.Duration("newInterval", newInterval),
				)
				task.Interval = newInterval
				task.Status = ScanTaskStatusDegraded
			}
		}
		if task.Priority > 1 {
			task.Priority--
		}
	}
	applyAdaptive := se.adaptiveThrottle != nil
	task.mu.Unlock()

	if applyAdaptive {
		se.adaptiveThrottle.ApplyInterval(task)
	}
}

func (se *ScanEngine) updateTaskState(task *ScanTask, result *ExecuteResult) {
	task.mu.Lock()

	if result.Success {
		task.ConsecutiveSuccess++
		task.ConsecutiveFailures = 0
		task.LastSuccess = time.Now()
		task.FailRate = 0
		task.Status = ScanTaskStatusIdle
		if task.BaseInterval > 0 && task.Interval != task.BaseInterval {
			task.Interval = task.BaseInterval
		}
		if task.Priority < 10 {
			task.Priority++
		}
	} else if result != nil && errors.Is(result.Error, ErrCircuitOpen) {
		// Fast-fail while CB open: keep scan cadence for HalfOpen probes.
		task.Status = ScanTaskStatusIdle
		if task.BaseInterval > 0 {
			task.Interval = task.BaseInterval
		}
	} else {
		task.ConsecutiveFailures++
		task.ConsecutiveSuccess = 0
		task.LastFailure = time.Now()
		task.FailRate = (task.FailRate*0.8 + 1.0*0.2)

		if task.ConsecutiveFailures >= 3 && se.taskDegradeOnFailure(task) {
			shift := task.ConsecutiveFailures - 3
			if shift > 6 {
				shift = 6
			}
			newInterval := task.Interval * (1 << shift)
			if newInterval > 64*time.Second {
				newInterval = 64 * time.Second
			}
			if task.BaseInterval > 0 && newInterval < task.BaseInterval {
				newInterval = task.BaseInterval
			}
			if newInterval < time.Millisecond {
				newInterval = time.Millisecond
			}
			if newInterval != task.Interval {
				zap.L().Warn("[降级] 任务失败率过高，调整采集间隔",
					zap.String("taskID", task.ID),
					zap.String("deviceKey", task.DeviceKey),
					zap.Int("failures", task.ConsecutiveFailures),
					zap.Duration("oldInterval", task.Interval),
					zap.Duration("newInterval", newInterval),
				)
				task.Interval = newInterval
				task.Status = ScanTaskStatusDegraded
			}
		}
		if task.Priority > 1 {
			task.Priority--
		}
	}
	applyAdaptive := se.adaptiveThrottle != nil
	task.mu.Unlock()

	if applyAdaptive {
		se.adaptiveThrottle.ApplyInterval(task)
	}
}

func (se *ScanEngine) taskDegradeOnFailure(task *ScanTask) bool {
	if task.Params == nil {
		return true
	}
	if v, ok := task.Params["degradeOnFailure"].(bool); ok {
		return v
	}
	return true
}

func (se *ScanEngine) AddTask(deviceKey, protocol string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	return se.addTask(deviceKey, protocol, "", interval, priority, pointIDs, params)
}

func (se *ScanEngine) AddTaskWithScanClass(deviceKey, protocol, scanClass string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	return se.addTask(deviceKey, protocol, scanClass, interval, priority, pointIDs, params)
}

func (se *ScanEngine) addTask(deviceKey, protocol, scanClass string, interval time.Duration, priority int, pointIDs []string, params map[string]any) *ScanTask {
	se.mu.Lock()
	defer se.mu.Unlock()

	taskID := fmt.Sprintf("task_%d_%s", se.taskIDCounter, deviceKey)
	if scanClass != "" {
		taskID = fmt.Sprintf("task_%d_%s_%s", se.taskIDCounter, deviceKey, scanClass)
	}
	se.taskIDCounter++

	var points []model.Point
	if params != nil {
		if pts, ok := params["points"].([]model.Point); ok {
			points = pts
		}
	}

	now := time.Now()
	jitterBound := taskJitterBound(se.config.JitterBound)
	jitter := taskDeterministicJitter(taskID, jitterBound)
	phaseOffset := time.Duration(0)
	if params != nil {
		switch v := params["phaseOffset"].(type) {
		case time.Duration:
			phaseOffset = v
		case int64:
			phaseOffset = time.Duration(v)
		}
	}
	if phaseOffset == 0 && interval > 0 {
		h := fnv.New32a()
		_, _ = h.Write([]byte(deviceKey))
		phaseOffset = time.Duration(uint64(h.Sum32()) % uint64(interval.Nanoseconds()))
	}
	base := now.Add(phaseOffset)
	nextRun := base.Add(jitter)

	task := &ScanTask{
		ID:              taskID,
		DeviceKey:       deviceKey,
		ScanClass:       scanClass,
		Protocol:        protocol,
		Interval:        interval,
		BaseInterval:    interval,
		LastScheduledAt: base,
		NextRun:         nextRun,
		DeadlineAt:      nextRun.Add(jitterBound),
		PhaseOffset:     phaseOffset,
		Priority:        priority,
		FailRate:        0,
		Status:          ScanTaskStatusIdle,
		PointIDs:        pointIDs,
		Points:          points,
		Params:          params,
		LastSuccess:     time.Time{},
		LastFailure:     time.Time{},
	}

	se.tasks[taskID] = task
	heap.Push(se.priorityQueue, task)

	zap.L().Info("[ScanEngine] 添加任务",
		zap.String("taskID", taskID),
		zap.String("deviceKey", deviceKey),
		zap.String("scanClass", scanClass),
		zap.String("protocol", protocol),
		zap.Duration("interval", interval),
		zap.Int("priority", priority),
		zap.Int("pointsCount", len(pointIDs)),
	)

	return task
}

func (se *ScanEngine) RemoveTask(taskID string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if task, exists := se.tasks[taskID]; exists {
		task.SetStatus(ScanTaskStatusStopped)
		delete(se.tasks, taskID)
		delete(se.overdueWarnAt, taskID)
		zap.L().Info("[ScanEngine] 移除任务",
			zap.String("taskID", taskID),
			zap.String("deviceKey", task.DeviceKey),
		)
	}
}

func (se *ScanEngine) RemoveTasksByDeviceKey(deviceKey string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	for taskID, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			task.SetStatus(ScanTaskStatusStopped)
			delete(se.tasks, taskID)
			delete(se.overdueWarnAt, taskID)
			zap.L().Info("[ScanEngine] 移除任务",
				zap.String("taskID", taskID),
				zap.String("deviceKey", deviceKey),
			)
		}
	}
}

func (se *ScanEngine) GetTask(taskID string) *ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.tasks[taskID]
}

func (se *ScanEngine) GetTaskByDeviceKey(deviceKey string) *ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	for _, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			return task
		}
	}
	return nil
}

func (se *ScanEngine) GetTasksByDeviceKey(deviceKey string) []*ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	var tasks []*ScanTask
	for _, task := range se.tasks {
		if task.DeviceKey == deviceKey {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (se *ScanEngine) GetTasks() []*ScanTask {
	se.mu.RLock()
	defer se.mu.RUnlock()
	tasks := make([]*ScanTask, 0, len(se.tasks))
	for _, task := range se.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (se *ScanEngine) UpdateTaskInterval(deviceKey string, interval time.Duration) {
	se.mu.Lock()
	defer se.mu.Unlock()

	task := se.findTaskLocked(deviceKey)
	if task != nil {
		task.mu.Lock()
		task.Interval = interval
		task.BaseInterval = interval
		task.mu.Unlock()
		se.rescheduleTask(task, time.Now())
		zap.L().Info("[ScanEngine] 更新任务间隔",
			zap.String("taskID", task.ID),
			zap.Duration("interval", interval),
		)
	}
}

func (se *ScanEngine) UpdateTaskDriverConfig(deviceKey string, updates map[string]any) {
	if len(updates) == 0 {
		return
	}
	se.mu.Lock()
	defer se.mu.Unlock()

	for _, task := range se.tasks {
		if task.DeviceKey != deviceKey || task.Params == nil {
			continue
		}
		base, ok := task.Params["driverConfig"].(map[string]any)
		if !ok || base == nil {
			base = map[string]any{}
		} else {
			clone := make(map[string]any, len(base)+len(updates))
			for k, v := range base {
				clone[k] = v
			}
			base = clone
		}
		for k, v := range updates {
			base[k] = v
		}
		task.Params["driverConfig"] = base
	}
}

func (se *ScanEngine) findTaskLocked(key string) *ScanTask {
	if task, exists := se.tasks[key]; exists {
		return task
	}
	for _, task := range se.tasks {
		if task.DeviceKey == key {
			return task
		}
	}
	return nil
}

func (se *ScanEngine) UpdateTaskPriority(deviceKey string, priority int) {
	se.mu.Lock()
	defer se.mu.Unlock()

	task := se.findTaskLocked(deviceKey)
	if task != nil {
		task.mu.Lock()
		task.Priority = priority
		task.mu.Unlock()
		zap.L().Info("[ScanEngine] 更新任务优先级",
			zap.String("taskID", task.ID),
			zap.Int("priority", priority),
		)
	}
}

func (se *ScanEngine) IsRunning() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.running
}

func (se *ScanEngine) GetActiveTaskCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	count := 0
	for _, task := range se.tasks {
		if task.GetStatus() == ScanTaskStatusRunning {
			count++
		}
	}
	return count
}

func (se *ScanEngine) GetPendingTaskCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return len(*se.priorityQueue)
}

func (se *ScanEngine) RegisterProtocol(protocol string, pType ProtocolType) {
	se.executionLayer.RegisterProtocol(protocol, pType)
}

func (se *ScanEngine) RegisterDriver(deviceKey string, d driver.Driver) {
	se.executionLayer.RegisterDriver(deviceKey, d)
}

func (se *ScanEngine) UnregisterDriver(deviceKey string) {
	se.executionLayer.UnregisterDriver(deviceKey)
}

func (se *ScanEngine) SetShadowCore(sc *ShadowCore) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.shadowCore = sc
}

func (se *ScanEngine) SetShadowIngress(si *ShadowIngress) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.shadowIngress = si
	if si != nil {
		se.shadowCore = si.shadowCore
	}
}

func (se *ScanEngine) SetCollectFinalize(fn CollectFinalizeFunc) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.collectFinalize = fn
}

func (se *ScanEngine) GetShadowCore() *ShadowCore {
	return se.shadowCore
}

func (se *ScanEngine) SetPointDegradation(m *PointDegradationManager) {
	se.mu.Lock()
	se.pointDegrade = m
	se.mu.Unlock()
	if se.executionLayer != nil {
		se.executionLayer.SetPointDegradation(m)
	}
}

func (se *ScanEngine) SetIOProfileProvider(fn IOProfileProvider) {
	if se.executionLayer != nil {
		se.executionLayer.SetIOProfileProvider(fn)
	}
}

func (se *ScanEngine) GetMetrics() *ScanEngineMetrics {
	return se.metrics
}

func (se *ScanEngine) GetGCMonitor() *GCMonitor {
	return se.gcMonitor
}

func (se *ScanEngine) GetCircuitBreaker() *DriverCircuitBreaker {
	if se == nil || se.executionLayer == nil {
		return nil
	}
	return se.executionLayer.GetCircuitBreaker()
}

func (se *ScanEngine) SetCircuitBreakerEventHandler(fn CircuitBreakerEventHandler) {
	if se.executionLayer != nil {
		se.executionLayer.SetCircuitBreakerEventHandler(fn)
	}
}

func (se *ScanEngine) OperationalSnapshot() map[string]any {
	out := map[string]any{}
	if se == nil || se.executionLayer == nil {
		return out
	}
	out["serial_queue_depth"] = se.executionLayer.GetSerialQueueDepths()
	if bp := se.executionLayer.GetBackpressure(); bp != nil {
		out["backpressure_reject_total"] = bp.RejectTotal()
		out["throttle_reject_by_reason"] = bp.RejectByReason()
	}
	return out
}

func (se *ScanEngine) slaWarningLoop() {
	defer se.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-se.stopCh:
			return
		case <-ticker.C:
			se.logSLAWarnings()
		}
	}
}

func (se *ScanEngine) logSLAWarnings() {
	if se.metrics == nil {
		return
	}
	cb := se.GetCircuitBreaker()
	warnings := se.metrics.SLAWarnings(cb)
	for _, w := range warnings {
		zap.L().Warn("[SLA] scan engine threshold exceeded",
			zap.Any("code", w["code"]),
			zap.Any("metric", w["metric"]),
			zap.Any("value", w["value"]),
			zap.Any("threshold", w["threshold"]),
			zap.Any("message", w["message"]),
		)
	}
}

func (se *ScanEngine) ExecuteTask(task *ScanTask) *ExecuteResult {
	if se == nil || se.executionLayer == nil || task == nil {
		return &ExecuteResult{Success: false, Error: ErrDriverNotFound}
	}
	return se.executionLayer.Execute(task)
}
