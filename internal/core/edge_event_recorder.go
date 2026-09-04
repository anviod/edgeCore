package core

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

const (
	defaultEdgeEventRingSize   = 500
	defaultEdgeFailureRingSize = 200
	edgeEventsBucket           = "edge_events"
	edgeFailuresBucket         = "edge_failures"
	edgeBblotBucket            = "bblot"
)

var edgeLogBuckets = []string{edgeEventsBucket, edgeFailuresBucket, edgeBblotBucket}

func hasEdgeErrorMessage(msg string) bool {
	return strings.TrimSpace(msg) != ""
}

func shouldPersistEdgeEvent(status, errMsg string) bool {
	if !hasEdgeErrorMessage(errMsg) {
		return false
	}
	return status == "error" || status == "dropped"
}

// ClassifyEdgeErrorType maps phase and message to a stable error category.
func ClassifyEdgeErrorType(phase, errMsg string) string {
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "timeout") || strings.Contains(errMsg, "超时") {
		return model.EdgeErrorTypeTimeout
	}
	switch phase {
	case "evaluate":
		return model.EdgeErrorTypeFormula
	case "action":
		return model.EdgeErrorTypeExecution
	case "dispatch":
		return model.EdgeErrorTypeDispatch
	default:
		return model.EdgeErrorTypeOther
	}
}

// EdgeLogsClearResult summarizes what was cleared by ClearEdgeLogs.
type EdgeLogsClearResult struct {
	EventsMemory   int      `json:"events_memory"`
	FailuresMemory int      `json:"failures_memory"`
	MinuteCache    int      `json:"minute_cache"`
	Buckets        []string `json:"buckets"`
}

type edgeEventTracker struct {
	event *model.EdgeRuleEvent
	mu    sync.Mutex
}

func (t *edgeEventTracker) beginPhase(phase string, detail map[string]any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.closeLastPhase(now)
	t.event.Phases = append(t.event.Phases, model.EdgeRuleEventPhase{
		Phase:     phase,
		StartedAt: now,
		Detail:    detail,
	})
}

func (t *edgeEventTracker) closeLastPhase(now time.Time) {
	if len(t.event.Phases) == 0 {
		return
	}
	last := &t.event.Phases[len(t.event.Phases)-1]
	if last.EndedAt.IsZero() {
		last.EndedAt = now
		last.DurationMs = now.Sub(last.StartedAt).Milliseconds()
	}
}

func (t *edgeEventTracker) failPhase(errMsg string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if len(t.event.Phases) > 0 {
		last := &t.event.Phases[len(t.event.Phases)-1]
		last.Error = errMsg
		if last.EndedAt.IsZero() {
			last.EndedAt = now
			last.DurationMs = now.Sub(last.StartedAt).Milliseconds()
		}
	}
}

func (t *edgeEventTracker) recordAction(index int, actionType, status, errMsg string, started, ended time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.event.Actions = append(t.event.Actions, model.EdgeRuleEventAction{
		Index:      index,
		Type:       actionType,
		Status:     status,
		Error:      errMsg,
		StartedAt:  started,
		EndedAt:    ended,
		DurationMs: ended.Sub(started).Milliseconds(),
	})
}

func (t *edgeEventTracker) finish(status, errMsg string) *model.EdgeRuleEvent {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.closeLastPhase(now)
	t.event.EndedAt = now
	t.event.DurationMs = now.Sub(t.event.StartedAt).Milliseconds()
	t.event.Status = status
	t.event.ErrorMessage = errMsg
	copy := *t.event
	copy.Phases = append([]model.EdgeRuleEventPhase(nil), t.event.Phases...)
	copy.Actions = append([]model.EdgeRuleEventAction(nil), t.event.Actions...)
	return &copy
}

type edgeEventRecorder struct {
	store *storage.Storage

	mu       sync.RWMutex
	events   []model.EdgeRuleEvent
	failures []model.EdgeFailureRecord
}

func newEdgeEventRecorder(store *storage.Storage) *edgeEventRecorder {
	return &edgeEventRecorder{
		store:    store,
		events:   make([]model.EdgeRuleEvent, 0, defaultEdgeEventRingSize),
		failures: make([]model.EdgeFailureRecord, 0, defaultEdgeFailureRingSize),
	}
}

func (r *edgeEventRecorder) startEvent(rule model.EdgeRule, val model.Value) *edgeEventTracker {
	return &edgeEventTracker{
		event: &model.EdgeRuleEvent{
			ID:            fmt.Sprintf("%d", time.Now().UnixNano()),
			RuleID:        rule.ID,
			RuleName:      rule.Name,
			StartedAt:     time.Now(),
			Status:        "running",
			TriggerSource: val,
			TriggerValue:  val.Value,
			Condition:     rule.Condition,
		},
	}
}

func (r *edgeEventRecorder) recordEvent(evt *model.EdgeRuleEvent) {
	if evt == nil || !hasEdgeErrorMessage(evt.ErrorMessage) {
		return
	}
	r.mu.Lock()
	r.events = appendRing(r.events, *evt, defaultEdgeEventRingSize)
	r.mu.Unlock()

	if r.store != nil {
		go func(snapshot model.EdgeRuleEvent) {
			if err := r.store.SaveData(edgeEventsBucket, snapshot.ID, snapshot); err != nil {
				log.Printf("[EdgeCompute] Failed to persist event %s: %v", snapshot.ID, err)
			}
		}(*evt)
	}
}

func (r *edgeEventRecorder) recordFailure(rec model.EdgeFailureRecord) {
	if !hasEdgeErrorMessage(rec.Error) {
		return
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("fail-%d", time.Now().UnixNano())
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	if rec.ErrorType == "" {
		rec.ErrorType = ClassifyEdgeErrorType(rec.Phase, rec.Error)
	}

	log.Printf("[EdgeCompute][FAILURE] rule_id=%s type=%s phase=%s error=%q action=%s idx=%d",
		rec.RuleID, rec.ErrorType, rec.Phase, rec.Error, rec.ActionType, rec.ActionIndex)

	r.mu.Lock()
	r.failures = appendRing(r.failures, rec, defaultEdgeFailureRingSize)
	r.mu.Unlock()

	if r.store != nil {
		go func(snapshot model.EdgeFailureRecord) {
			if err := r.store.SaveData(edgeFailuresBucket, snapshot.ID, snapshot); err != nil {
				log.Printf("[EdgeCompute] Failed to persist failure %s: %v", snapshot.ID, err)
			}
		}(rec)
	}
}

func (r *edgeEventRecorder) getEvents(ruleID string, limit int) []model.EdgeRuleEvent {
	if limit <= 0 {
		limit = 100
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.EdgeRuleEvent, 0, limit)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		evt := r.events[i]
		if !shouldPersistEdgeEvent(evt.Status, evt.ErrorMessage) {
			continue
		}
		if ruleID != "" && evt.RuleID != ruleID {
			continue
		}
		out = append(out, evt)
	}
	return out
}

func (r *edgeEventRecorder) getFailures(ruleID string, limit int) []model.EdgeFailureRecord {
	if limit <= 0 {
		limit = 100
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.EdgeFailureRecord, 0, limit)
	for i := len(r.failures) - 1; i >= 0 && len(out) < limit; i-- {
		if ruleID != "" && r.failures[i].RuleID != ruleID {
			continue
		}
		if !hasEdgeErrorMessage(r.failures[i].Error) {
			continue
		}
		out = append(out, r.failures[i])
	}
	return out
}

func (r *edgeEventRecorder) counts() (events, failures int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.events), len(r.failures)
}

func (r *edgeEventRecorder) clearBuffers() (events, failures int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	events = len(r.events)
	failures = len(r.failures)
	r.events = make([]model.EdgeRuleEvent, 0, defaultEdgeEventRingSize)
	r.failures = make([]model.EdgeFailureRecord, 0, defaultEdgeFailureRingSize)
	return events, failures
}

func appendRing[T any](buf []T, item T, cap int) []T {
	buf = append(buf, item)
	if len(buf) > cap {
		buf = buf[len(buf)-cap:]
	}
	return buf
}

func (em *EdgeComputeManager) startEvent(rule model.EdgeRule, val model.Value) *edgeEventTracker {
	if em.events == nil {
		return nil
	}
	return em.events.startEvent(rule, val)
}

func (em *EdgeComputeManager) applyEventStats(state *model.RuleRuntimeState, status string, triggered bool, actionSuccess, actionFailure int) {
	if state == nil {
		return
	}
	if actionSuccess > 0 {
		state.ActionSuccessCount += int64(actionSuccess)
	}
	if actionFailure > 0 {
		state.ActionFailureCount += int64(actionFailure)
	}
	switch status {
	case "error", "dropped":
		state.FailureCount++
	case "completed":
		if triggered {
			state.SuccessCount++
		}
	}
}

func (em *EdgeComputeManager) recordFinishedEvent(tracker *edgeEventTracker, status, errMsg string) *model.EdgeRuleEvent {
	if em.events == nil || tracker == nil {
		return nil
	}
	evt := tracker.finish(status, errMsg)
	if shouldPersistEdgeEvent(status, errMsg) {
		em.events.recordEvent(evt)
	}
	return evt
}

func (em *EdgeComputeManager) finishEvent(tracker *edgeEventTracker, status string, errMsg string) {
	em.finishEventWithStats(tracker, status, errMsg, 0, 0)
}

func (em *EdgeComputeManager) finishEventWithStats(tracker *edgeEventTracker, status string, errMsg string, actionSuccess, actionFailure int) {
	evt := em.recordFinishedEvent(tracker, status, errMsg)
	if evt == nil {
		return
	}
	em.stateMu.Lock()
	state := em.ruleStates[evt.RuleID]
	if state == nil {
		state = &model.RuleRuntimeState{
			RuleID:   evt.RuleID,
			RuleName: evt.RuleName,
		}
		em.ruleStates[evt.RuleID] = state
	}
	em.applyEventStats(state, status, evt.Triggered, actionSuccess, actionFailure)
	em.stateMu.Unlock()
}

func (em *EdgeComputeManager) recordFailure(rec model.EdgeFailureRecord, val model.Value) {
	if em.events == nil || !hasEdgeErrorMessage(rec.Error) {
		return
	}
	populateEdgeFailureScope(&rec, val)
	em.events.recordFailure(rec)
	em.recordFailureMinuteSnapshot(rec)
}

func (em *EdgeComputeManager) recordFailureMinuteSnapshot(rec model.EdgeFailureRecord) {
	store := em.storage()
	if store == nil || !hasEdgeErrorMessage(rec.Error) {
		return
	}
	if rec.ErrorType == "" {
		rec.ErrorType = ClassifyEdgeErrorType(rec.Phase, rec.Error)
	}

	minuteKey := rec.Timestamp.Format("2006-01-02 15:04")
	if minuteKey == "" || minuteKey == "0001-01-01 00:00" {
		minuteKey = time.Now().Format("2006-01-02 15:04")
	}
	cacheKey := fmt.Sprintf("%s_%s", rec.RuleID, minuteKey)

	em.bblotMu.Lock()
	snap, exists := em.minuteCache[cacheKey]
	if !exists {
		snap = &model.RuleMinuteSnapshot{
			RuleID:    rec.RuleID,
			RuleName:  rec.RuleName,
			Minute:    minuteKey,
			UpdatedAt: time.Now(),
		}
		em.minuteCache[cacheKey] = snap
	}
	snap.ErrorType = rec.ErrorType
	snap.ErrorMessage = rec.Error
	snap.Category = rec.Category
	snap.ChannelID = rec.ChannelID
	snap.DeviceID = rec.DeviceID
	snap.UpdatedAt = time.Now()
	snapCopy := *snap
	em.bblotMu.Unlock()

	go func(snapshot model.RuleMinuteSnapshot) {
		key := fmt.Sprintf("%s_%s", snapshot.RuleID, snapshot.Minute)
		if err := store.SaveData("bblot", key, snapshot); err != nil {
			log.Printf("Failed to save bblot error snapshot: %v", err)
		}
	}(snapCopy)
}

func (em *EdgeComputeManager) GetEvents(ruleID string, limit int) []model.EdgeRuleEvent {
	if em.events == nil {
		return nil
	}
	return em.events.getEvents(ruleID, limit)
}

func (em *EdgeComputeManager) GetFailures(ruleID string, limit int) []model.EdgeFailureRecord {
	if em.events == nil {
		return nil
	}
	return em.events.getFailures(ruleID, limit)
}

// ClearEdgeLogs removes historical edge events, failures, and minute-level logs.
// Rule definitions and runtime state (current_status, execution_phase, windows) are preserved.
func (em *EdgeComputeManager) ClearEdgeLogs() (EdgeLogsClearResult, error) {
	result := EdgeLogsClearResult{Buckets: append([]string(nil), edgeLogBuckets...)}

	if em.events != nil {
		result.EventsMemory, result.FailuresMemory = em.events.clearBuffers()
	}

	em.bblotMu.Lock()
	result.MinuteCache = len(em.minuteCache)
	em.minuteCache = make(map[string]*model.RuleMinuteSnapshot)
	em.bblotMu.Unlock()

	if store := em.storage(); store != nil {
		for _, bucket := range edgeLogBuckets {
			if err := store.ClearBucket(bucket); err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
				return result, fmt.Errorf("clear bucket %s: %w", bucket, err)
			}
		}
	}

	return result, nil
}

func (em *EdgeComputeManager) loadPersistedEvents() {
	store := em.storage()
	if em.events == nil || store == nil {
		return
	}
	var loaded []model.EdgeRuleEvent
	_ = store.LoadAll(edgeEventsBucket, func(k, v []byte) error {
		var evt model.EdgeRuleEvent
		if err := json.Unmarshal(v, &evt); err != nil {
			return nil
		}
		if shouldPersistEdgeEvent(evt.Status, evt.ErrorMessage) {
			loaded = append(loaded, evt)
		}
		return nil
	})
	if len(loaded) == 0 {
		em.loadPersistedFailures()
		return
	}
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].StartedAt.Before(loaded[j].StartedAt)
	})
	em.events.mu.Lock()
	for _, evt := range loaded {
		em.events.events = appendRing(em.events.events, evt, defaultEdgeEventRingSize)
	}
	em.events.mu.Unlock()
	em.loadPersistedFailures()
}

func (em *EdgeComputeManager) loadPersistedFailures() {
	store := em.storage()
	if em.events == nil || store == nil {
		return
	}
	var loaded []model.EdgeFailureRecord
	_ = store.LoadAll(edgeFailuresBucket, func(k, v []byte) error {
		var rec model.EdgeFailureRecord
		if err := json.Unmarshal(v, &rec); err != nil {
			return nil
		}
		if hasEdgeErrorMessage(rec.Error) {
			loaded = append(loaded, rec)
		}
		return nil
	})
	if len(loaded) == 0 {
		return
	}
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].Timestamp.Before(loaded[j].Timestamp)
	})
	em.events.mu.Lock()
	for _, rec := range loaded {
		em.events.failures = appendRing(em.events.failures, rec, defaultEdgeFailureRingSize)
	}
	em.events.mu.Unlock()
}
