package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"

	"github.com/expr-lang/expr"
)

var errWindowStepPending = errors.New("window aggregation step pending")

const (
	maxEdgeWindowSamples  = 10000
	maxEdgeValueCacheSize = 10000
	maxEdgeMinuteCacheAge = 2 * time.Hour
	maxEdgeActionRunKeys  = 64
)

type ruleTask struct {
	rule model.EdgeRule
	val  model.Value
}

type EdgeComputeManager struct {
	rules      map[string]model.EdgeRule
	pipeline   *DataPipeline
	nbm        *NorthboundManager
	cm         *ChannelManager
	store      *storage.Storage
	mu         sync.RWMutex
	saveFunc   func([]model.EdgeRule) error
	ruleStates map[string]*model.RuleRuntimeState
	windows    map[string][]model.Value
	stateMu    sync.RWMutex
	valueCache map[string]model.Value
	cacheMu    sync.RWMutex

	// Rule Index
	ruleIndex map[string][]string // Key: "ChannelID/DeviceID/PointID", Value: []RuleID
	indexMu   sync.RWMutex

	// Worker Pool
	workerPool  chan *ruleTask
	workerCount int
	wg          sync.WaitGroup
	scheduler   *edgeRuleScheduler
	events      *edgeEventRecorder

	// Metrics
	statsMu        sync.RWMutex
	rulesTriggered int64
	rulesExecuted  int64
	rulesDropped   int64
	rulesCoalesced int64
	rulesDebounced int64
	batchWindow    time.Duration

	stopOnce sync.Once

	// Minute-level bblot cache
	bblotMu     sync.Mutex
	minuteCache map[string]*model.RuleMinuteSnapshot

	// Test Hook
	actionHook func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error)

	// Dependency Injection for Testing
	writer DeviceIO

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

type DeviceIO interface {
	WritePoint(channelID, deviceID, pointID string, value any) error
	ReadPoint(channelID, deviceID, pointID string) (model.Value, error)
}

type EdgeComputeMetrics struct {
	WorkerPoolSize        int   `json:"worker_pool_size"`
	WorkerPoolUsage       int   `json:"worker_pool_usage"`
	RuleCount             int   `json:"rule_count"`
	CacheSize             int   `json:"cache_size"`
	WindowBufferTotal     int   `json:"window_buffer_total"`
	PendingSchedulerTasks int   `json:"pending_scheduler_tasks"`
	MinuteCacheSize       int   `json:"minute_cache_size"`
	EventBufferSize       int   `json:"event_buffer_size"`
	FailureBufferSize     int   `json:"failure_buffer_size"`
	BatchWindowMs         int64 `json:"batch_window_ms"`
	RulesTriggered        int64 `json:"rules_triggered"`
	RulesExecuted         int64 `json:"rules_executed"`
	RulesDropped          int64 `json:"rules_dropped"`
	RulesCoalesced        int64 `json:"rules_coalesced"`
	RulesDebounced        int64 `json:"rules_debounced"`
}

func NewEdgeComputeManager(pipeline *DataPipeline, store *storage.Storage, saveFunc func([]model.EdgeRule) error) *EdgeComputeManager {
	ctx, cancel := context.WithCancel(context.Background())
	em := &EdgeComputeManager{
		ctx:         ctx,
		cancel:      cancel,
		rules:       make(map[string]model.EdgeRule),
		pipeline:    pipeline,
		store:       store,
		saveFunc:    saveFunc,
		ruleStates:  make(map[string]*model.RuleRuntimeState),
		windows:     make(map[string][]model.Value),
		valueCache:  make(map[string]model.Value),
		minuteCache: make(map[string]*model.RuleMinuteSnapshot),
		ruleIndex:   make(map[string][]string),
		workerPool:  make(chan *ruleTask, defaultEdgeQueueSize),
		workerCount: defaultEdgeWorkerCount,
		batchWindow: defaultEdgeBatchWindow,
	}
	em.scheduler = newEdgeRuleScheduler(em, em.batchWindow)
	em.events = newEdgeEventRecorder(store)
	return em
}

func (em *EdgeComputeManager) GetMetrics() EdgeComputeMetrics {
	em.statsMu.RLock()
	defer em.statsMu.RUnlock()
	em.mu.RLock()
	ruleCount := len(em.rules)
	em.mu.RUnlock()
	em.cacheMu.RLock()
	cacheSize := len(em.valueCache)
	em.cacheMu.RUnlock()
	em.stateMu.RLock()
	windowTotal := 0
	for _, samples := range em.windows {
		windowTotal += len(samples)
	}
	em.stateMu.RUnlock()
	em.bblotMu.Lock()
	minuteCacheSize := len(em.minuteCache)
	em.bblotMu.Unlock()
	pending := 0
	if em.scheduler != nil {
		pending = em.scheduler.pendingCount()
	}
	eventCount, failureCount := 0, 0
	if em.events != nil {
		eventCount, failureCount = em.events.counts()
	}

	return EdgeComputeMetrics{
		WorkerPoolSize:        cap(em.workerPool),
		WorkerPoolUsage:       len(em.workerPool),
		RuleCount:             ruleCount,
		CacheSize:             cacheSize,
		WindowBufferTotal:     windowTotal,
		PendingSchedulerTasks: pending,
		MinuteCacheSize:       minuteCacheSize,
		EventBufferSize:       eventCount,
		FailureBufferSize:     failureCount,
		BatchWindowMs:         em.batchWindow.Milliseconds(),
		RulesTriggered:        em.rulesTriggered,
		RulesExecuted:         em.rulesExecuted,
		RulesDropped:          em.rulesDropped,
		RulesCoalesced:        em.rulesCoalesced,
		RulesDebounced:        em.rulesDebounced,
	}
}

func (em *EdgeComputeManager) incRulesCoalesced() {
	em.statsMu.Lock()
	em.rulesCoalesced++
	em.statsMu.Unlock()
}

func (em *EdgeComputeManager) incRulesDebounced(n int64) {
	em.statsMu.Lock()
	em.rulesDebounced += n
	em.statsMu.Unlock()
}

func (em *EdgeComputeManager) dispatchTask(task *ruleTask) {
	em.statsMu.Lock()
	em.rulesTriggered++
	em.statsMu.Unlock()

	select {
	case em.workerPool <- task:
	default:
		em.statsMu.Lock()
		em.rulesDropped++
		em.statsMu.Unlock()
		em.onSchedulerDrop(task, "worker_pool_full")
	}
}

func (em *EdgeComputeManager) onSchedulerDrop(task *ruleTask, reason string) {
	if task == nil {
		return
	}
	log.Printf("[EdgeCompute] Dropped rule execution rule=%s priority=%d reason=%s", task.rule.ID, task.rule.Priority, reason)
	tracker := em.startEvent(task.rule, task.val)
	if tracker != nil {
		tracker.beginPhase("dropped", map[string]any{"reason": reason})
		em.finishEventWithStats(tracker, "dropped", reason, 0, 0)
	}
	em.recordFailure(model.EdgeFailureRecord{
		RuleID:       task.rule.ID,
		RuleName:     task.rule.Name,
		Phase:        "dispatch",
		Error:        reason,
		TriggerValue: task.val.Value,
		Condition:    task.rule.Condition,
		Context: map[string]any{
			"priority":   task.rule.Priority,
			"channel_id": task.val.ChannelID,
			"device_id":  task.val.DeviceID,
			"point_id":   task.val.PointID,
		},
	}, task.val)
}

func (em *EdgeComputeManager) SetBatchWindow(d time.Duration) {
	em.batchWindow = d
	if em.scheduler != nil {
		em.scheduler.batchWindow = d
	}
}

func (em *EdgeComputeManager) SetNorthboundManager(nbm *NorthboundManager) {
	em.nbm = nbm
}

func (em *EdgeComputeManager) SetChannelManager(cm *ChannelManager) {
	em.cm = cm
	if em.writer == nil {
		em.writer = cm
	}
}

func (em *EdgeComputeManager) SetDeviceWriter(w DeviceIO) {
	em.writer = w
}

func (em *EdgeComputeManager) SetStorage(s *storage.Storage) {
	em.store = s
}

func (em *EdgeComputeManager) SetActionHook(hook func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error)) {
	em.actionHook = hook
}

func (em *EdgeComputeManager) LoadRules(rules []model.EdgeRule) {
	em.mu.Lock()
	defer em.mu.Unlock()
	for _, r := range rules {
		em.rules[r.ID] = r
	}
	em.rebuildIndex()
	//log.Printf("Loaded %d edge computing rules", len(rules))
}

func (em *EdgeComputeManager) Start() {
	// Restore state from DB
	em.restoreState()
	em.loadPersistedEvents()

	// Start Workers
	for i := 0; i < em.workerCount; i++ {
		em.wg.Add(1)
		go em.workerLoop(i)
	}

	// Register handler to data pipeline
	em.pipeline.AddHandler(em.handleValue)

	// Start retry loop
	go em.retryLoop()
	go em.maintenanceLoop()

	//log.Println("Edge Compute Manager started with", em.workerCount, "workers")
}

func (em *EdgeComputeManager) Stop() {
	em.stopOnce.Do(func() {
		em.cancel() // Cancel context
		if em.scheduler != nil {
			em.scheduler.stop()
		}
		close(em.workerPool)
		em.wg.Wait()
	})
}

func (em *EdgeComputeManager) workerLoop(id int) {
	defer em.wg.Done()
	for task := range em.workerPool {
		em.executeRule(task.rule, task.val)
		em.statsMu.Lock()
		em.rulesExecuted++
		em.statsMu.Unlock()
	}
}

func (em *EdgeComputeManager) rebuildIndex() {
	em.indexMu.Lock()
	defer em.indexMu.Unlock()

	em.ruleIndex = make(map[string][]string)
	for _, rule := range em.rules {
		em.indexRule(rule)
	}
}

func (em *EdgeComputeManager) indexRule(rule model.EdgeRule) {
	// Helper to add key
	addKey := func(cid, did, pid string) {
		key := fmt.Sprintf("%s/%s/%s", cid, did, pid)
		// Check duplicates
		for _, id := range em.ruleIndex[key] {
			if id == rule.ID {
				return
			}
		}
		em.ruleIndex[key] = append(em.ruleIndex[key], rule.ID)
	}

	if len(rule.Sources) > 0 {
		for _, src := range rule.Sources {
			addKey(src.ChannelID, src.DeviceID, src.PointID)
		}
	} else {
		// Legacy
		addKey(rule.Source.ChannelID, rule.Source.DeviceID, rule.Source.PointID)
	}
}

func (em *EdgeComputeManager) removeFromIndex(ruleID string) {
	em.indexMu.Lock()
	defer em.indexMu.Unlock()

	for key, ids := range em.ruleIndex {
		newIDs := make([]string, 0)
		for _, id := range ids {
			if id != ruleID {
				newIDs = append(newIDs, id)
			}
		}
		if len(newIDs) == 0 {
			delete(em.ruleIndex, key)
		} else {
			em.ruleIndex[key] = newIDs
		}
	}
}

func (em *EdgeComputeManager) handleValue(val model.Value) {
	// Update Cache (Thread Safe)
	cacheKey := fmt.Sprintf("%s/%s/%s", val.ChannelID, val.DeviceID, val.PointID)
	em.cacheMu.Lock()
	em.valueCache[cacheKey] = val
	em.trimValueCacheLocked()
	em.cacheMu.Unlock()

	// Find Matched Rules via Index (O(1) lookup)
	em.indexMu.RLock()
	ruleIDs, exists := em.ruleIndex[cacheKey]
	em.indexMu.RUnlock()

	if !exists || len(ruleIDs) == 0 {
		return
	}

	em.mu.RLock()
	var matchedRules []model.EdgeRule
	for _, id := range ruleIDs {
		if rule, ok := em.rules[id]; ok {
			if rule.Enable {
				// Check Interval Logic (Throttling)
				if rule.CheckInterval != "" {
					em.stateMu.RLock()
					state := em.ruleStates[rule.ID]
					em.stateMu.RUnlock()

					if state != nil {
						if interval, err := time.ParseDuration(rule.CheckInterval); err == nil {
							if time.Since(state.LastCheckTime) < interval {
								continue
							}
						}
					}
				}
				matchedRules = append(matchedRules, rule)
			}
		}
	}
	em.mu.RUnlock()

	// Sort by Priority (High to Low) before debounce scheduling.
	if len(matchedRules) > 1 {
		sort.Slice(matchedRules, func(i, j int) bool {
			return matchedRules[i].Priority > matchedRules[j].Priority
		})
	}

	for _, rule := range matchedRules {
		em.scheduler.schedule(&ruleTask{rule: rule, val: val})
	}
}

func matchRule(rule model.EdgeRule, val model.Value) bool {
	// New Multi-source match
	if len(rule.Sources) > 0 {
		for _, src := range rule.Sources {
			if matchSource(src, val) {
				return true
			}
		}
		return false
	}
	// Legacy match
	return matchSource(rule.Source, val)
}

func matchSource(src model.RuleSource, val model.Value) bool {
	// If ID is empty, it matches all (wildcard) - but usually we want specific match
	// For now, strict match if ID is provided
	if src.ChannelID != "" && src.ChannelID != val.ChannelID {
		return false
	}
	if src.DeviceID != "" && src.DeviceID != val.DeviceID {
		return false
	}
	if src.PointID != "" && src.PointID != val.PointID {
		return false
	}
	return true
}

func (em *EdgeComputeManager) setExecutionPhase(ruleID string, phase string, actionIndex int) {
	em.stateMu.Lock()
	defer em.stateMu.Unlock()
	state := em.ruleStates[ruleID]
	if state == nil {
		return
	}
	state.ExecutionPhase = phase
	state.ExecutionActionIndex = actionIndex
}

func (em *EdgeComputeManager) markActionRun(ruleID string, actionIndex int) {
	em.stateMu.Lock()
	defer em.stateMu.Unlock()
	state := em.ruleStates[ruleID]
	if state == nil {
		return
	}
	if state.ActionLastRuns == nil {
		state.ActionLastRuns = make(map[int]time.Time)
	}
	state.ActionLastRuns[actionIndex] = time.Now()
	em.trimActionLastRunsLocked(state)
}

func (em *EdgeComputeManager) shouldSkipActionInterval(ruleID string, actionIndex int, action model.RuleAction) bool {
	intervalStr, ok := action.Config["interval"].(string)
	if !ok || intervalStr == "" {
		return false
	}
	duration, err := time.ParseDuration(intervalStr)
	if err != nil {
		return false
	}
	em.stateMu.Lock()
	defer em.stateMu.Unlock()
	state := em.ruleStates[ruleID]
	if state == nil {
		return false
	}
	if state.ActionLastRuns == nil {
		state.ActionLastRuns = make(map[int]time.Time)
	}
	lastRun := state.ActionLastRuns[actionIndex]
	return !lastRun.IsZero() && time.Since(lastRun) < duration
}

func (em *EdgeComputeManager) executeRule(rule model.EdgeRule, val model.Value) {
	em.stateMu.Lock()
	state, exists := em.ruleStates[rule.ID]
	if !exists {
		state = &model.RuleRuntimeState{
			RuleID:         rule.ID,
			RuleName:       rule.Name,
			Enable:         rule.Enable,
			CurrentStatus:  "NORMAL",
			ExecutionPhase: "idle",
		}
		em.ruleStates[rule.ID] = state
	}
	if rule.Type == "window" {
		state.ExecutionPhase = "window"
	} else {
		state.ExecutionPhase = "evaluate"
	}
	state.LastCheckTime = time.Now()
	em.stateMu.Unlock()

	// Prepare Env for Expression
	env := make(map[string]any)
	// Try to convert trigger value to float for numeric comparisons
	triggerVal := val.Value
	if fVal, ok := toFloat(triggerVal); ok {
		triggerVal = fVal
	}
	env["value"] = triggerVal // Current triggering value

	// Populate env with aliases from cache
	if len(rule.Sources) > 0 {
		em.cacheMu.RLock()
		for _, src := range rule.Sources {
			if src.Alias == "" && src.PointID == "" {
				continue
			}

			var srcVal any
			found := false

			// Check if the triggering value belongs to this source
			// If so, use the triggering value as it is the most up-to-date
			if matchSource(src, val) {
				srcVal = val.Value
				found = true
			} else {
				key := fmt.Sprintf("%s/%s/%s", src.ChannelID, src.DeviceID, src.PointID)
				if v, ok := em.valueCache[key]; ok {
					srcVal = v.Value
					found = true
				}
			}

			if found {
				// Try to convert to float for better expression compatibility
				if fVal, ok := toFloat(srcVal); ok {
					srcVal = fVal
				}

				if src.Alias != "" {
					env[src.Alias] = srcVal
				}
				if src.PointID != "" {
					env[src.PointID] = srcVal
				}
			} else {
				// Missing value - use NaN to ensure comparisons fail safely (false) instead of crashing
				if src.Alias != "" {
					env[src.Alias] = math.NaN()
				}
				if src.PointID != "" {
					env[src.PointID] = math.NaN()
				}
			}
		}
		em.cacheMu.RUnlock()
	}

	var rawTriggered bool
	var err error
	var outputVal model.Value = val

	switch rule.Type {
	case "threshold", "state":
		rawTriggered, err = evaluateThreshold(rule.Condition, env)
	case "calculation":
		var res any
		res, err = evaluateCalculation(rule.Expression, env)
		if err == nil {
			outputVal.Value = res
			rawTriggered = true
		}
	case "window":
		rawTriggered, outputVal, err = em.evaluateWindow(rule, val, env)
	default:
		if rule.Condition != "" {
			rawTriggered, err = evaluateThreshold(rule.Condition, env)
		}
	}

	if errors.Is(err, errWindowStepPending) {
		return
	}

	em.stateMu.Lock()
	defer em.stateMu.Unlock()

	defer func() { go em.saveRuleState(rule.ID) }()

	if err != nil {
		state.ErrorMessage = err.Error()
		state.ExecutionPhase = "error"
		log.Printf("Rule %s evaluation error: %v", rule.Name, err)
		tracker := em.startEvent(rule, val)
		tracker.beginPhase("evaluate", map[string]any{"type": rule.Type})
		tracker.failPhase(err.Error())
		em.applyEventStats(state, "error", false, 0, 0)
		em.recordFinishedEvent(tracker, "error", err.Error())
		em.recordFailure(model.EdgeFailureRecord{
			RuleID:       rule.ID,
			RuleName:     rule.Name,
			Phase:        "evaluate",
			Error:        err.Error(),
			TriggerValue: val.Value,
			Condition:    rule.Condition,
		}, val)
		return
	}

	finalTriggered := false

	if !rawTriggered {
		state.ConditionStart = time.Time{}
		state.ConditionCount = 0
		state.CurrentStatus = "NORMAL"
		state.ExecutionPhase = "idle"
		state.ExecutionActionIndex = 0
	} else {
		if state.ConditionStart.IsZero() {
			state.ConditionStart = time.Now()
		}
		state.ConditionCount++

		constraintsMet := true
		if rule.State != nil {
			if rule.State.Duration != "" {
				if dur, err := time.ParseDuration(rule.State.Duration); err == nil {
					if time.Since(state.ConditionStart) < dur {
						constraintsMet = false
						state.CurrentStatus = "WARNING"
					}
				}
			}
			if rule.State.Count > 0 {
				if state.ConditionCount < rule.State.Count {
					constraintsMet = false
					if state.CurrentStatus != "WARNING" {
						state.CurrentStatus = "WARNING"
					}
				}
			}
		}

		if constraintsMet {
			finalTriggered = true
		} else {
			state.ExecutionPhase = "state_hold"
		}
	}

	if !finalTriggered {
		em.recordMinuteSnapshot(state)
		return
	}

	prevStatus := state.CurrentStatus

	state.LastTrigger = time.Now()
	state.TriggerCount++
	state.CurrentStatus = "ALARM"
	state.LastValue = outputVal.Value
	state.ErrorMessage = ""
	state.ExecutionPhase = "trigger"

	em.recordMinuteSnapshot(state)

	tracker := em.startEvent(rule, val)
	tracker.event.Triggered = true
	tracker.event.OutputValue = outputVal.Value
	tracker.beginPhase("trigger", map[string]any{
		"channel_id": val.ChannelID,
		"device_id":  val.DeviceID,
		"point_id":   val.PointID,
		"value":      val.Value,
		"output":     outputVal.Value,
	})

	shouldExecute := true
	if rule.TriggerMode == "on_change" {
		if prevStatus == "ALARM" {
			shouldExecute = false
		}
	}

	if shouldExecute && len(rule.Actions) > 0 {
		state.ExecutionPhase = "action"
		state.ExecutionActionIndex = 0
		go em.executeActions(rule, rule.Actions, outputVal, env, tracker)
		return
	}

	state.ExecutionPhase = "completed"
	em.applyEventStats(state, "completed", true, 0, 0)
	em.recordFinishedEvent(tracker, "completed", "")
}

func (em *EdgeComputeManager) recordMinuteSnapshot(state *model.RuleRuntimeState) {
	if em.store == nil || state == nil {
		return
	}
	if !hasEdgeErrorMessage(state.ErrorMessage) {
		return
	}

	minuteKey := time.Now().Format("2006-01-02 15:04")
	cacheKey := fmt.Sprintf("%s_%s", state.RuleID, minuteKey)
	errorType := ClassifyEdgeErrorType("evaluate", state.ErrorMessage)
	if state.ExecutionPhase == "action" {
		errorType = ClassifyEdgeErrorType("action", state.ErrorMessage)
	}

	em.bblotMu.Lock()
	snap, exists := em.minuteCache[cacheKey]
	if !exists {
		snap = &model.RuleMinuteSnapshot{
			RuleID:    state.RuleID,
			RuleName:  state.RuleName,
			Minute:    minuteKey,
			UpdatedAt: time.Now(),
		}
		em.minuteCache[cacheKey] = snap
	}

	snap.ErrorType = errorType
	snap.ErrorMessage = state.ErrorMessage
	snap.UpdatedAt = time.Now()

	snapCopy := *snap
	em.bblotMu.Unlock()

	// Persist asynchronously
	go func(snapshot model.RuleMinuteSnapshot) {
		// Use minuteKey as part of the key to ensure one record per minute per rule
		key := fmt.Sprintf("%s_%s", snapshot.RuleID, snapshot.Minute)
		if err := em.store.SaveData("bblot", key, snapshot); err != nil {
			log.Printf("Failed to save bblot snapshot: %v", err)
		}
	}(snapCopy)
}

func isEdgeErrorMinuteSnapshot(snap model.RuleMinuteSnapshot) bool {
	return hasEdgeErrorMessage(snap.ErrorMessage)
}

// IsEdgeErrorMinuteSnapshot reports whether a persisted minute snapshot is an error log row.
func IsEdgeErrorMinuteSnapshot(snap model.RuleMinuteSnapshot) bool {
	return isEdgeErrorMinuteSnapshot(snap)
}

// QueryLogs retrieves error logs based on time range and optional rule ID.
func (em *EdgeComputeManager) QueryLogs(start, end time.Time, ruleID string) ([]model.RuleMinuteSnapshot, error) {
	if em.store == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var logs []model.RuleMinuteSnapshot
	bucket := "bblot"

	startStr := start.Format("2006-01-02 15:04")
	endStr := end.Format("2006-01-02 15:04")

	if ruleID != "" {
		minKey := fmt.Sprintf("%s_%s", ruleID, startStr)
		maxKey := fmt.Sprintf("%s_%s", ruleID, endStr)

		err := em.store.LoadRange(bucket, minKey, maxKey, func(k, v []byte) error {
			var snap model.RuleMinuteSnapshot
			if err := json.Unmarshal(v, &snap); err != nil {
				return nil
			}
			if !isEdgeErrorMinuteSnapshot(snap) {
				return nil
			}
			logs = append(logs, snap)
			return nil
		})
		return logs, err
	}

	err := em.store.LoadAll(bucket, func(k, v []byte) error {
		var snap model.RuleMinuteSnapshot
		if err := json.Unmarshal(v, &snap); err != nil {
			return nil
		}
		if snap.Minute >= startStr && snap.Minute <= endStr && isEdgeErrorMinuteSnapshot(snap) {
			logs = append(logs, snap)
		}
		return nil
	})

	return logs, err
}

func (em *EdgeComputeManager) evaluateWindow(rule model.EdgeRule, val model.Value, baseEnv map[string]any) (bool, model.Value, error) {
	if rule.Window == nil {
		return false, val, fmt.Errorf("missing window config")
	}

	em.stateMu.Lock()
	history := em.windows[rule.ID]
	history = append(history, val)

	// Window Logic (Simplified: Count based or Time based)
	// For now, assume Size is duration "10s" or count "10"
	// Parse Size
	sizeDur, errDur := time.ParseDuration(rule.Window.Size)
	isTimeWindow := errDur == nil

	var filtered []model.Value
	if isTimeWindow {
		cutoff := val.TS.Add(-sizeDur)
		for _, v := range history {
			if v.TS.After(cutoff) || v.TS.Equal(cutoff) {
				filtered = append(filtered, v)
			}
		}
	} else {
		// Count window
		count := 10 // Default
		fmt.Sscanf(rule.Window.Size, "%d", &count)
		if len(history) > count {
			filtered = history[len(history)-count:]
		} else {
			filtered = history
		}
	}

	em.windows[rule.ID] = em.trimWindowSamples(filtered)

	// Sliding/tumbling step: buffer always, aggregate only on Interval tick.
	if rule.Window.Interval != "" {
		if step, err := time.ParseDuration(rule.Window.Interval); err == nil && step > 0 {
			state := em.ruleStates[rule.ID]
			lastEval := time.Time{}
			if state != nil {
				lastEval = state.LastWindowEval
			}
			if !lastEval.IsZero() && time.Since(lastEval) < step {
				em.stateMu.Unlock()
				go em.saveWindowData(rule.ID)
				return false, val, errWindowStepPending
			}
		}
	}
	em.stateMu.Unlock()

	// Persist window data asynchronously
	go em.saveWindowData(rule.ID)

	// Aggregation
	var result float64
	var count int
	var minVal, maxVal float64
	var firstVal, lastVal float64
	var firstTime, lastTime time.Time

	for i, v := range filtered {
		f, ok := toFloat(v.Value)
		if ok {
			if i == 0 {
				minVal = f
				maxVal = f
				firstVal = f
				firstTime = v.TS
			}
			lastVal = f
			lastTime = v.TS

			switch rule.Window.AggrFunc {
			case "sum", "avg":
				result += f
			case "max":
				if f > maxVal {
					maxVal = f
				}
			case "min":
				if f < minVal {
					minVal = f
				}
			}
			count++
		}
	}

	switch rule.Window.AggrFunc {
	case "max":
		result = maxVal
	case "min":
		result = minVal
	case "avg":
		if count > 0 {
			result = result / float64(count)
		}
	case "count":
		result = float64(count)
	case "rate":
		// (Last - First) / Duration (in seconds)
		if count > 1 {
			duration := lastTime.Sub(firstTime).Seconds()
			if duration > 0 {
				result = (lastVal - firstVal) / duration
			} else {
				result = 0
			}
		} else {
			result = 0
		}
	}

	// Evaluate Condition against Result
	env := make(map[string]any)
	for k, v := range baseEnv {
		env[k] = v
	}
	env["value"] = result

	triggered, err := evaluateThreshold(rule.Condition, env)

	outputVal := val
	outputVal.Value = result

	em.stateMu.Lock()
	if state := em.ruleStates[rule.ID]; state != nil {
		state.LastWindowEval = time.Now()
	}
	em.stateMu.Unlock()

	return triggered, outputVal, err
}

// evaluateState is removed as logic is merged into executeRule

func toFloat(v any) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case float32:
		return float64(i), true
	case int:
		return float64(i), true
	case int64:
		return float64(i), true
	case int32:
		return float64(i), true
	case uint:
		return float64(i), true
	case uint64:
		return float64(i), true
	case uint32:
		return float64(i), true
	case string:
		f, err := strconv.ParseFloat(i, 64)
		return f, err == nil
	case bool:
		if i {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func (em *EdgeComputeManager) GetRuleStates() map[string]*model.RuleRuntimeState {
	em.stateMu.RLock()
	defer em.stateMu.RUnlock()

	copy := make(map[string]*model.RuleRuntimeState)
	for k, v := range em.ruleStates {
		// Deep copy or shallow copy? Ptr is fine if we don't modify it outside
		c := *v
		copy[k] = &c
	}
	return copy
}

func (em *EdgeComputeManager) GetWindowData(ruleID string) []model.Value {
	em.stateMu.RLock()
	defer em.stateMu.RUnlock()

	if data, ok := em.windows[ruleID]; ok {
		// Return a copy
		res := make([]model.Value, len(data))
		copy(res, data)
		return res
	}
	return []model.Value{}
}

var bitAccessRegex = regexp.MustCompile(`\b([a-zA-Z_]\w*)\.(?:bit\.)?(\d+)\b`)
var bitMapRegex = regexp.MustCompile(`^bitget\(v,\s*(\d+)\)$`)
var bitSetRegex = regexp.MustCompile(`^bitset\((\d+),\s*(\d+)\)$`)
var bitSetValueRegex = regexp.MustCompile(`^bitset\((\d+),\s*value\)$`)

func preprocessExpression(input string) string {
	return bitAccessRegex.ReplaceAllStringFunc(input, func(match string) string {
		submatches := bitAccessRegex.FindStringSubmatch(match)
		if len(submatches) == 3 {
			// Parse N
			if n, err := strconv.Atoi(submatches[2]); err == nil {
				// Convert 1-based index to 0-based, but keep 0 as 0
				bitIndex := n
				if n > 0 {
					bitIndex = n - 1
				}
				return fmt.Sprintf("bitget(%s, %d)", submatches[1], bitIndex)
			}
			return fmt.Sprintf("bitget(%s, %s)", submatches[1], submatches[2])
		}
		return match
	})
}

func evaluateThreshold(condition string, env map[string]any) (bool, error) {
	env = prepareExprEnv(env)
	condition = preprocessExpression(condition)
	program, err := expr.Compile(condition, expr.Env(env))
	if err != nil {
		return false, err
	}
	output, err := expr.Run(program, env)
	if err != nil {
		return false, err
	}
	if res, ok := output.(bool); ok {
		return res, nil
	}
	return false, fmt.Errorf("condition must return boolean")
}

func evaluateCalculation(expression string, env map[string]any) (any, error) {
	env = prepareExprEnv(env)
	expression = preprocessExpression(expression)
	program, err := expr.Compile(expression, expr.Env(env))
	if err != nil {
		return nil, err
	}
	output, err := expr.Run(program, env)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func prepareExprEnv(env map[string]any) map[string]any {
	if env == nil {
		env = make(map[string]any)
	}
	// Check if already populated to avoid re-adding
	if _, ok := env["bitand"]; ok {
		return env
	}

	env["bitand"] = func(a, b any) (int64, error) { return bitwiseOp(a, b, func(x, y int64) int64 { return x & y }) }
	env["bitor"] = func(a, b any) (int64, error) { return bitwiseOp(a, b, func(x, y int64) int64 { return x | y }) }
	env["bitxor"] = func(a, b any) (int64, error) { return bitwiseOp(a, b, func(x, y int64) int64 { return x ^ y }) }
	env["bitnot"] = func(a any) (int64, error) { return bitwiseUnary(a, func(x int64) int64 { return ^x }) }
	env["bitshl"] = func(a, b any) (int64, error) { return bitwiseOp(a, b, func(x, y int64) int64 { return x << y }) }
	env["bitshr"] = func(a, b any) (int64, error) { return bitwiseOp(a, b, func(x, y int64) int64 { return x >> y }) }

	env["bitget"] = func(val, pos any) (int64, error) {
		v, err := toInt64(val)
		if err != nil {
			return 0, err
		}
		p, err := toInt64(pos)
		if err != nil {
			return 0, err
		}
		if p < 0 || p > 63 {
			return 0, fmt.Errorf("bit position out of range")
		}
		return (v >> p) & 1, nil
	}

	env["bitset"] = func(val, pos, newBit any) (int64, error) {
		v, err := toInt64(val)
		if err != nil {
			return 0, err
		}
		p, err := toInt64(pos)
		if err != nil {
			return 0, err
		}
		b, err := toInt64(newBit)
		if err != nil {
			return 0, err
		}
		if p < 0 || p > 63 {
			return 0, fmt.Errorf("bit position out of range")
		}
		if b != 0 {
			return v | (1 << p), nil
		}
		return v &^ (1 << p), nil
	}

	return env
}

func bitwiseOp(a, b any, op func(int64, int64) int64) (int64, error) {
	ia, err := toInt64(a)
	if err != nil {
		return 0, err
	}
	ib, err := toInt64(b)
	if err != nil {
		return 0, err
	}
	return op(ia, ib), nil
}

func bitwiseUnary(a any, op func(int64) int64) (int64, error) {
	ia, err := toInt64(a)
	if err != nil {
		return 0, err
	}
	return op(ia), nil
}

func toInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int:
		return int64(val), nil
	case int8:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int64:
		return val, nil
	case uint:
		return int64(val), nil
	case uint8:
		return int64(val), nil
	case uint16:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return int64(f), nil
		}
		// Try parsing as int directly if float parse fails (though float parse handles ints)
		i, err := strconv.ParseInt(val, 0, 64)
		if err == nil {
			return i, nil
		}
		return 0, fmt.Errorf("cannot convert string '%s' to int64", val)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func (em *EdgeComputeManager) executeActions(rule model.EdgeRule, actions []model.RuleAction, val model.Value, env map[string]any, tracker *edgeEventTracker) {
	if len(actions) == 0 {
		em.setExecutionPhase(rule.ID, "completed", 0)
		go em.saveRuleState(rule.ID)
		if tracker != nil {
			em.finishEventWithStats(tracker, "completed", "", 0, 0)
		}
		return
	}

	var wg sync.WaitGroup
	executed := 0
	var failMu sync.Mutex
	var hadFailure bool
	var lastErr string
	var actionSuccess, actionFailure int

	for i, action := range actions {
		if em.shouldSkipActionInterval(rule.ID, i, action) {
			if tracker != nil {
				tracker.recordAction(i, action.Type, "skipped", "", time.Now(), time.Now())
			}
			continue
		}
		executed++
		wg.Add(1)
		actionIndex := i
		go func(act model.RuleAction) {
			defer wg.Done()
			em.setExecutionPhase(rule.ID, "action", actionIndex)
			em.markActionRun(rule.ID, actionIndex)
			started := time.Now()
			err := em.executeSingleAction(em.ctx, rule.ID, act, val, env)
			ended := time.Now()
			if err != nil {
				failMu.Lock()
				hadFailure = true
				lastErr = err.Error()
				actionFailure++
				failMu.Unlock()
				if tracker != nil {
					tracker.recordAction(actionIndex, act.Type, "failed", err.Error(), started, ended)
				}
				em.saveFailedAction(rule.ID, act, val, env, err.Error())
				em.recordFailure(model.EdgeFailureRecord{
					RuleID:       rule.ID,
					RuleName:     rule.Name,
					Phase:        "action",
					Error:        err.Error(),
					TriggerValue: val.Value,
					Condition:    rule.Condition,
					ActionType:   act.Type,
					ActionIndex:  actionIndex,
				}, val)
			} else {
				failMu.Lock()
				actionSuccess++
				failMu.Unlock()
				if tracker != nil {
					tracker.recordAction(actionIndex, act.Type, "success", "", started, ended)
				}
			}
		}(action)
	}

	if executed == 0 {
		em.setExecutionPhase(rule.ID, "completed", 0)
		go em.saveRuleState(rule.ID)
		if tracker != nil {
			em.finishEventWithStats(tracker, "completed", "", 0, 0)
		}
		return
	}

	go func() {
		wg.Wait()
		em.setExecutionPhase(rule.ID, "completed", 0)
		go em.saveRuleState(rule.ID)
		if tracker != nil {
			status := "completed"
			if hadFailure {
				status = "error"
			}
			em.finishEventWithStats(tracker, status, lastErr, actionSuccess, actionFailure)
		}
	}()
}

func (em *EdgeComputeManager) resolveValueTemplate(val any, env map[string]any) any {
	strVal, ok := val.(string)
	if !ok {
		return val
	}
	// Check if it looks like a template
	if !strings.Contains(strVal, "${") {
		return val
	}

	// Resolve template
	resolved := os.Expand(strVal, func(k string) string {
		if v, ok := env[k]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	})

	return resolved
}

func (em *EdgeComputeManager) calculateRMW(cid, did, pid string, bitIdx int, bitValRes any, expression string) (any, error) {
	// Read current value
	currentVal, err := em.writer.ReadPoint(cid, did, pid)
	if err == nil {
		// Modify bit
		curInt, _ := toInt64(currentVal.Value)
		resInt, _ := toInt64(bitValRes)

		var newVal int64
		if resInt != 0 {
			newVal = curInt | (1 << bitIdx)
		} else {
			newVal = curInt &^ (1 << bitIdx)
		}
		log.Printf("[EdgeAction] RMW: %s/%s/%s | Expr: %s (Bit %d) | Cur: %d | BitVal: %d | New: %d", cid, did, pid, expression, bitIdx, curInt, resInt, newVal)
		return newVal, nil
	} else {
		log.Printf("[EdgeAction] RMW Warning: Failed to read point %s/%s/%s: %v. Fallback to writing bit value directly.", cid, did, pid, err)
		// Fallback: Write the bit value (shifted) directly? Or just the 0/1?
		// User expectation: If formula exists, use it.
		// If we can't read, we can't preserve other bits.
		// We return the best guess (bitmask only) or error?
		// Let's return the shifted value so at least that bit is correct.
		resInt, _ := toInt64(bitValRes)
		if resInt != 0 {
			return int64(1) << bitIdx, nil
		} else {
			return int64(0), nil
		}
	}
}

func (em *EdgeComputeManager) executeSingleAction(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) (err error) {
	// Test Hook
	defer func() {
		if em.actionHook != nil {
			em.actionHook(ruleID, action, val, env, err)
		}
	}()

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch action.Type {
	case "sequence":
		return em.executeSequence(ctx, ruleID, action, val, env)
	case "delay":
		return em.executeDelay(ctx, action)
	case "check":
		return em.executeCheck(ctx, ruleID, action, val, env)
	case "log":
		return em.executeLog(ctx, ruleID, action, val)
	case "device_control":
		return em.executeDeviceControl(ctx, ruleID, action, val, env)
	case "mqtt":
		return em.executeMqtt(ctx, ruleID, action, val, env)
	case "database":
		return em.executeDatabase(ctx, ruleID, action, val, env)
	case "http":
		return em.executeHttp(ctx, ruleID, action, val, env)
	default:
		return fmt.Errorf("unsupported action type: %s", action.Type)
	}
}

func (em *EdgeComputeManager) saveFailedAction(ruleID string, action model.RuleAction, val model.Value, env map[string]any, errStr string) {
	if em.store == nil {
		return
	}
	// Only retry idempotent actions or safe ones
	// For now support mqtt and device_control
	if action.Type != "mqtt" && action.Type != "device_control" {
		return
	}

	fa := model.FailedAction{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		RuleID:     ruleID,
		Action:     action,
		Value:      val,
		Timestamp:  time.Now(),
		RetryCount: 0,
		LastError:  errStr,
		Env:        env,
	}
	if err := em.store.SaveData("DataCache", fa.ID, fa); err != nil {
		//log.Printf("Failed to save failed action: %v", err)
	}
}

func (em *EdgeComputeManager) retryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-em.ctx.Done():
			return
		case <-ticker.C:
			em.processFailedActions()
		}
	}
}

func (em *EdgeComputeManager) processFailedActions() {
	if em.store == nil {
		return
	}
	em.store.LoadAll("DataCache", func(k, v []byte) error {
		var fa model.FailedAction
		if err := json.Unmarshal(v, &fa); err != nil {
			return nil
		}

		// Retry logic
		// Use manager context
		err := em.executeSingleAction(em.ctx, fa.RuleID, fa.Action, fa.Value, fa.Env)
		if err == nil {
			// Success, remove
			em.store.DeleteData("DataCache", fa.ID)
			log.Printf("Retry success for action %s", fa.ID)
		} else {
			// Fail, update count
			fa.RetryCount++
			fa.LastError = err.Error()
			if fa.RetryCount > 10 { // Max retries
				em.store.DeleteData("DataCache", fa.ID)
				log.Printf("Max retries reached for action %s, dropping", fa.ID)
			} else {
				em.store.SaveData("DataCache", fa.ID, fa)
			}
		}
		return nil
	})
}

func (em *EdgeComputeManager) GetFailedActions() []model.FailedAction {
	var result []model.FailedAction
	if em.store == nil {
		return result
	}
	em.store.LoadAll("DataCache", func(k, v []byte) error {
		var fa model.FailedAction
		if err := json.Unmarshal(v, &fa); err == nil {
			result = append(result, fa)
		}
		return nil
	})
	return result
}

// Extended Workflow Actions

func (em *EdgeComputeManager) executeSequence(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	stepsInterface, ok := action.Config["steps"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid steps format in sequence")
	}
	steps, err := em.convertToRuleActions(stepsInterface)
	if err != nil {
		return err
	}

	for _, step := range steps {
		if err := em.executeSingleAction(ctx, ruleID, step, val, env); err != nil {
			return err
		}
	}
	return nil
}

func (em *EdgeComputeManager) executeDelay(ctx context.Context, action model.RuleAction) error {
	durationStr, ok := action.Config["duration"].(string)
	if !ok {
		return fmt.Errorf("missing duration in delay")
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return err
	}
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (em *EdgeComputeManager) executeCheck(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	cid, _ := action.Config["channel_id"].(string)
	did, _ := action.Config["device_id"].(string)
	pid, _ := action.Config["point_id"].(string)
	exprStr, _ := action.Config["expression"].(string)

	retry := 1
	if r, ok := action.Config["retry"].(float64); ok {
		retry = int(r)
	} else if r, ok := action.Config["retry"].(int); ok {
		retry = r
	}
	if retry < 1 {
		retry = 1
	}

	interval := time.Second
	if iStr, ok := action.Config["interval"].(string); ok {
		if d, err := time.ParseDuration(iStr); err == nil {
			interval = d
		}
	}

	timeout := 0 * time.Second
	if tStr, ok := action.Config["timeout"].(string); ok {
		if d, err := time.ParseDuration(tStr); err == nil {
			timeout = d
		}
	}

	var checkErr error
	success := false

	startTime := time.Now()
	for i := 0; i < retry; i++ {
		if timeout > 0 && time.Since(startTime) > timeout {
			checkErr = fmt.Errorf("check timeout")
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentVal, err := em.writer.ReadPoint(cid, did, pid)
		if err != nil {
			checkErr = fmt.Errorf("read failed: %v", err)
		} else {
			checkEnv := map[string]any{"v": currentVal.Value}
			for k, v := range env {
				checkEnv[k] = v
			}

			res, err := evaluateCalculation(exprStr, checkEnv)
			if err != nil {
				checkErr = fmt.Errorf("eval failed: %v", err)
			} else {
				if b, ok := res.(bool); ok && b {
					success = true
					break
				}
				checkErr = fmt.Errorf("condition false")
			}
		}

		if i < retry-1 {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	if success {
		return nil
	}

	if onFailInterface, ok := action.Config["on_fail"].([]interface{}); ok {
		log.Printf("[EdgeAction] Check failed (%v), executing rollback...", checkErr)
		rollbackSteps, err := em.convertToRuleActions(onFailInterface)
		if err == nil {
			for _, step := range rollbackSteps {
				_ = em.executeSingleAction(ctx, ruleID, step, val, env)
			}
		}
	}

	return fmt.Errorf("check failed: %v", checkErr)
}

func (em *EdgeComputeManager) executeLog(ctx context.Context, ruleID string, action model.RuleAction, val model.Value) error {
	level, _ := action.Config["level"].(string)
	msg, _ := action.Config["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("Rule %s triggered", ruleID)
	}
	prefix := "[EdgeAction]"
	switch strings.ToLower(level) {
	case "warn":
		log.Printf("%s [WARN] %s", prefix, msg)
	case "error":
		log.Printf("%s [ERROR] %s", prefix, msg)
	default:
		log.Printf("%s [INFO] %s", prefix, msg)
	}
	return nil
}

func (em *EdgeComputeManager) convertToRuleActions(input []interface{}) ([]model.RuleAction, error) {
	var actions []model.RuleAction
	for _, item := range input {
		mapItem, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid action format")
		}
		act := model.RuleAction{}
		if t, ok := mapItem["type"].(string); ok {
			act.Type = t
		}
		if c, ok := mapItem["config"].(map[string]interface{}); ok {
			act.Config = c
		} else {
			act.Config = make(map[string]any)
		}
		actions = append(actions, act)
	}
	return actions, nil
}

func (em *EdgeComputeManager) executeMqtt(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	if em.nbm == nil {
		return fmt.Errorf("NorthboundManager not available")
	}
	topic, _ := action.Config["topic"].(string)
	clientID, _ := action.Config["client_id"].(string)
	configID, _ := action.Config["mqtt_config_id"].(string) // New reference
	strategy, _ := action.Config["send_strategy"].(string)

	// If using Config Reference, topic might be optional (use default)?
	// But usually Rule overrides topic.
	// If topic is empty, use default from config?
	// PublishMQTTClient uses client.PublishRaw which requires topic.
	// If topic is empty here, we should probably fetch it from config or error.
	// But PublishRaw just sends.
	// Let's assume topic is required in Rule Action OR we fetch from config.
	// We don't have easy access to config here.
	// Let's require topic in Rule Action for now, or if empty, pass empty and let client decide (client.PublishRaw might fail).
	// Actually, NorthboundManager's PublishMQTTClient calls client.PublishRaw.
	// client.PublishRaw checks topic? No, paho does.
	// Let's keep topic required if overrides are needed.

	var payload []byte
	var err error

	if strategy == "batch" {
		batchData := make(map[string]any)
		for k, v := range env {
			if k != "value" {
				batchData[k] = v
			}
		}
		if len(batchData) == 0 {
			payload, err = json.Marshal(val)
		} else {
			payload, err = json.Marshal(batchData)
		}
	} else {
		payload, err = json.Marshal(val)
	}
	if err != nil {
		return err
	}

	if msg, ok := action.Config["message"].(string); ok && msg != "" {
		resolvedMsg := os.Expand(msg, func(k string) string {
			if v, ok := env[k]; ok {
				return fmt.Sprintf("%v", v)
			}
			return ""
		})
		payload = []byte(resolvedMsg)
	}

	if configID != "" {
		if topic == "" {
			// Try to use default topic?
			// We can't access config here.
			// Let's assume Rule must provide topic if it wants to override,
			// OR we change PublishMQTTClient to accept empty topic and use default.
			// Let's update PublishMQTTClient in NorthboundManager?
			// No, let's just pass it.
		}
		return em.nbm.PublishMQTTClient(configID, topic, payload)
	}

	if topic == "" {
		return nil
	}

	return em.nbm.PublishMQTT(clientID, topic, payload)
}

func (em *EdgeComputeManager) executeDatabase(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	if em.store == nil {
		return fmt.Errorf("storage not available")
	}
	bucket, _ := action.Config["bucket"].(string)
	if bucket == "" {
		bucket = "rule_events"
	}
	key := fmt.Sprintf("%s_%d", ruleID, time.Now().UnixNano())

	data := map[string]interface{}{
		"rule_id": ruleID,
		"value":   val,
		"time":    time.Now(),
	}

	return em.store.SaveData(bucket, key, data)
}

func (em *EdgeComputeManager) executeHttp(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	strategy, _ := action.Config["send_strategy"].(string)
	var payload []byte
	var err error

	if strategy == "batch" {
		batchData := make(map[string]any)
		for k, v := range env {
			if k != "value" {
				batchData[k] = v
			}
		}
		if len(batchData) == 0 {
			payload, err = json.Marshal(val)
		} else {
			payload, err = json.Marshal(batchData)
		}
	} else {
		payload, err = json.Marshal(val)
	}
	if err != nil {
		return err
	}

	if msg, ok := action.Config["body"].(string); ok && msg != "" {
		resolvedMsg := os.Expand(msg, func(k string) string {
			if v, ok := env[k]; ok {
				return fmt.Sprintf("%v", v)
			}
			return ""
		})
		payload = []byte(resolvedMsg)
	}

	// Check for Northbound Config Reference
	configID, _ := action.Config["http_config_id"].(string)
	if configID != "" {
		if em.nbm == nil {
			return fmt.Errorf("NorthboundManager not available")
		}
		return em.nbm.PublishHTTP(configID, payload)
	}

	// Legacy Inline HTTP
	url, _ := action.Config["url"].(string)
	if url == "" {
		return nil
	}
	method, _ := action.Config["method"].(string)
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (em *EdgeComputeManager) executeDeviceControl(ctx context.Context, ruleID string, action model.RuleAction, val model.Value, env map[string]any) error {
	if em.writer == nil {
		return fmt.Errorf("DeviceWriter not available")
	}

	if targets, ok := action.Config["targets"].([]interface{}); ok && len(targets) > 0 {
		var errs []error
		for _, t := range targets {
			targetMap, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			cid, _ := targetMap["channel_id"].(string)
			did, _ := targetMap["device_id"].(string)
			pid, _ := targetMap["point_id"].(string)
			valToWrite := targetMap["value"]
			expression, _ := targetMap["expression"].(string)

			if cid != "" && did != "" && pid != "" {
				expressionHandled := false
				if expression == "" {
					//log.Printf("[EdgeAction] Info: Empty expression for %s/%s/%s, using direct value write", cid, did, pid)
				}

				if expression != "" {
					calcEnv := make(map[string]any)
					for k, v := range env {
						calcEnv[k] = v
					}
					if fVal, ok := toFloat(val.Value); ok {
						calcEnv["v"] = fVal
					} else {
						calcEnv["v"] = val.Value
					}

					preprocessed := preprocessExpression(expression)
					matches := bitMapRegex.FindStringSubmatch(preprocessed)
					if len(matches) == 2 {
						bitIdx, _ := strconv.Atoi(matches[1])
						res, err := evaluateCalculation(expression, calcEnv)
						if err == nil {
							if newVal, err := em.calculateRMW(cid, did, pid, bitIdx, res, expression); err == nil {
								valToWrite = newVal
								expressionHandled = true
							}
						} else {
							log.Printf("[EdgeAction] Failed to evaluate expression '%s': %v", expression, err)
						}
					} else {
						matchesSet := bitSetRegex.FindStringSubmatch(expression)
						matchesSetValue := bitSetValueRegex.FindStringSubmatch(expression)

						if len(matchesSetValue) == 2 {
							n, _ := strconv.Atoi(matchesSetValue[1])
							bitIdx := n - 1
							if bitIdx < 0 {
								bitIdx = 0
							}

							var resolvedVal int64
							rawVal := valToWrite
							if rawVal != nil {
								rawVal = em.resolveValueTemplate(rawVal, env)
							}
							if fVal, ok := toFloat(rawVal); ok {
								resolvedVal = int64(fVal)
							} else {
								if sVal, ok := rawVal.(string); ok {
									if f, err := strconv.ParseFloat(sVal, 64); err == nil {
										resolvedVal = int64(f)
									}
								}
							}

							if newVal, err := em.calculateRMW(cid, did, pid, bitIdx, resolvedVal, expression); err == nil {
								valToWrite = newVal
								expressionHandled = true
							}
						} else if len(matchesSet) == 3 {
							n, _ := strconv.Atoi(matchesSet[1])
							val, _ := strconv.Atoi(matchesSet[2])
							bitIdx := n - 1
							if bitIdx < 0 {
								bitIdx = 0
							}

							if newVal, err := em.calculateRMW(cid, did, pid, bitIdx, int64(val), expression); err == nil {
								valToWrite = newVal
								expressionHandled = true
							}
						} else {
							res, err := evaluateCalculation(expression, calcEnv)
							if err == nil {
								valToWrite = res
								expressionHandled = true
							} else {
								log.Printf("[EdgeAction] Failed to evaluate expression '%s': %v", expression, err)
							}
						}
					}
				}

				if !expressionHandled {
					if valToWrite != nil {
						valToWrite = em.resolveValueTemplate(valToWrite, env)
					} else {
						valToWrite = val.Value
					}
				}

				if err := em.writer.WritePoint(cid, did, pid, valToWrite); err != nil {
					errs = append(errs, fmt.Errorf("failed to write %s/%s/%s: %v", cid, did, pid, err))
				}
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("batch control errors: %v", errs)
		}
		return nil
	}

	channelID, _ := action.Config["channel_id"].(string)
	deviceID, _ := action.Config["device_id"].(string)
	pointID, _ := action.Config["point_id"].(string)
	valToWrite := action.Config["value"]

	if channelID == "" || deviceID == "" || pointID == "" {
		return fmt.Errorf("missing channel_id, device_id or point_id")
	}

	if valToWrite == nil {
		valToWrite = val.Value
	} else {
		valToWrite = em.resolveValueTemplate(valToWrite, env)
	}

	return em.writer.WritePoint(channelID, deviceID, pointID, valToWrite)
}

// CRUD Operations

func (em *EdgeComputeManager) UpsertRule(rule model.EdgeRule) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Sanitize rule configuration to remove redundant UI data
	em.sanitizeRule(&rule)

	// If update, remove old index entries first
	if _, exists := em.rules[rule.ID]; exists {
		// Note: We need to remove index entries for the OLD rule, not the new one
		// But here we are holding the lock and overwriting.
		// Ideally we should have removed it before.
		// Since we overwrite the map, let's just use removeFromIndex with ruleID
		// But removeFromIndex locks indexMu, so we need to be careful about deadlock if we hold mu.
		// Actually, removeFromIndex uses indexMu, UpsertRule uses mu. Separate locks.
		// BUT we need to call removeFromIndex OUTSIDE of mu if possible or just handle it carefully.
		// However, removeFromIndex iterates ruleIndex.

		// To be safe and clean:
		// 1. We have the old rule in em.rules[rule.ID]
		// 2. We can't call removeFromIndex while holding mu if removeFromIndex might need mu?
		//    No, removeFromIndex only needs indexMu. UpsertRule holds mu.
		//    So it is safe to call removeFromIndex inside UpsertRule.
		em.removeFromIndex(rule.ID)
	}

	em.rules[rule.ID] = rule

	// Add new index
	em.indexRule(rule)

	return em.persist()
}

func (em *EdgeComputeManager) DeleteRule(id string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if _, ok := em.rules[id]; !ok {
		return fmt.Errorf("rule not found")
	}

	em.removeFromIndex(id)
	delete(em.rules, id)

	return em.persist()
}

func (em *EdgeComputeManager) GetRules() []model.EdgeRule {
	em.mu.RLock()
	defer em.mu.RUnlock()

	rules := make([]model.EdgeRule, 0, len(em.rules))
	for _, r := range em.rules {
		rules = append(rules, r)
	}
	return rules
}

func (em *EdgeComputeManager) persist() error {
	if em.saveFunc == nil {
		return nil
	}
	rules := make([]model.EdgeRule, 0, len(em.rules))
	for _, r := range em.rules {
		rules = append(rules, r)
	}
	normalized, err := model.NormalizeEdgeRulesForSave(rules)
	if err != nil {
		return err
	}
	return em.saveFunc(normalized)
}

// ClearRuntimeState 清空边缘规则运行时状态（内存态，与 runtime.db 清理配合使用）。
func (em *EdgeComputeManager) ClearRuntimeState() {
	em.stateMu.Lock()
	em.ruleStates = make(map[string]*model.RuleRuntimeState)
	em.windows = make(map[string][]model.Value)
	em.stateMu.Unlock()

	em.cacheMu.Lock()
	em.valueCache = make(map[string]model.Value)
	em.cacheMu.Unlock()

	em.bblotMu.Lock()
	em.minuteCache = make(map[string]*model.RuleMinuteSnapshot)
	em.bblotMu.Unlock()
}

// RuntimeLogStats returns in-memory edge log buffer sizes for database stats.
func (em *EdgeComputeManager) RuntimeLogStats() (eventsMem, failuresMem, minuteCache int) {
	if em.events != nil {
		eventsMem, failuresMem = em.events.counts()
	}
	em.bblotMu.Lock()
	minuteCache = len(em.minuteCache)
	em.bblotMu.Unlock()
	return eventsMem, failuresMem, minuteCache
}

func (em *EdgeComputeManager) restoreState() {
	if em.store == nil {
		return
	}
	// Restore Rule States
	em.store.LoadAll(storage.BucketRuleState, func(k, v []byte) error {
		var state model.RuleRuntimeState
		if err := json.Unmarshal(v, &state); err == nil {
			em.stateMu.Lock()
			em.ruleStates[state.RuleID] = &state
			em.stateMu.Unlock()
		}
		return nil
	})

	// Restore Windows
	em.store.LoadAll(storage.BucketWindow, func(k, v []byte) error {
		var data []model.Value
		if err := json.Unmarshal(v, &data); err == nil {
			em.stateMu.Lock()
			em.windows[string(k)] = data
			em.stateMu.Unlock()
		}
		return nil
	})
	//log.Println("Edge Compute state restored from DB")
}

func (em *EdgeComputeManager) saveRuleState(ruleID string) {
	if em.store == nil {
		return
	}
	em.stateMu.RLock()
	statePtr, ok := em.ruleStates[ruleID]
	var stateCopy model.RuleRuntimeState
	if ok && statePtr != nil {
		stateCopy = *statePtr // Shallow copy to avoid race during marshal
	}
	em.stateMu.RUnlock()

	if ok && statePtr != nil {
		if err := em.store.SaveData(storage.BucketRuleState, ruleID, stateCopy); err != nil {
			log.Printf("Failed to save rule state for %s: %v", ruleID, err)
		}
	}
}

func (em *EdgeComputeManager) saveWindowData(ruleID string) {
	if em.store == nil {
		return
	}
	em.stateMu.RLock()
	data, ok := em.windows[ruleID]
	em.stateMu.RUnlock()

	if ok {
		em.store.SaveData(storage.BucketWindow, ruleID, data)
	}
}

func (em *EdgeComputeManager) sanitizeRule(rule *model.EdgeRule) {
	for i := range rule.Actions {
		if rule.Actions[i].Type == "device_control" {
			if targets, ok := rule.Actions[i].Config["targets"].([]interface{}); ok {
				for _, t := range targets {
					if targetMap, ok := t.(map[string]interface{}); ok {
						delete(targetMap, "_deviceList")
						delete(targetMap, "_pointList")
					} else if targetMap, ok := t.(map[string]any); ok {
						delete(targetMap, "_deviceList")
						delete(targetMap, "_pointList")
					}
				}
			}
		}
	}
}

func (em *EdgeComputeManager) maintenanceLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-em.ctx.Done():
			return
		case <-ticker.C:
			em.pruneMinuteCache()
		}
	}
}

func (em *EdgeComputeManager) pruneMinuteCache() {
	cutoff := time.Now().Add(-maxEdgeMinuteCacheAge)
	em.bblotMu.Lock()
	defer em.bblotMu.Unlock()
	for key, snap := range em.minuteCache {
		if snap.UpdatedAt.Before(cutoff) {
			delete(em.minuteCache, key)
		}
	}
}

func (em *EdgeComputeManager) trimValueCacheLocked() {
	if len(em.valueCache) <= maxEdgeValueCacheSize {
		return
	}
	overflow := len(em.valueCache) - maxEdgeValueCacheSize
	for key := range em.valueCache {
		delete(em.valueCache, key)
		overflow--
		if overflow <= 0 {
			break
		}
	}
}

func (em *EdgeComputeManager) trimWindowSamples(samples []model.Value) []model.Value {
	if len(samples) <= maxEdgeWindowSamples {
		return samples
	}
	return samples[len(samples)-maxEdgeWindowSamples:]
}

func (em *EdgeComputeManager) trimActionLastRunsLocked(state *model.RuleRuntimeState) {
	if len(state.ActionLastRuns) <= maxEdgeActionRunKeys {
		return
	}
	for idx := range state.ActionLastRuns {
		delete(state.ActionLastRuns, idx)
		if len(state.ActionLastRuns) <= maxEdgeActionRunKeys {
			break
		}
	}
}
