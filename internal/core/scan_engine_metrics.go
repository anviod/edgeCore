package core

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	scanLagSampleCap      = 2048
	scanSLAWindowDuration = 5 * time.Minute
)

// SLA 告警阈值（运维对齐用，可通过 diagnostics API 的 sla_warnings 查看）。
const (
	SLAScanLagP95MsThreshold   = 100.0
	SLAScanDriftAvgMsThreshold = 50.0
	SLAScanMissDeadlineMax     = 0
	SLACircuitBreakerRejectMax = 0
)

type timedDriftSample struct {
	at     time.Time
	micros int64
}

// ScanEngineMetrics 调度引擎可观测指标（对标 Kepware Diagnostics）。
type ScanEngineMetrics struct {
	TasksExecuted         atomic.Uint64
	TasksSucceeded        atomic.Uint64
	TasksFailed           atomic.Uint64
	StarvationRescues     atomic.Uint64
	TaskOverdueTotal      atomic.Uint64
	ScanMissDeadlineTotal atomic.Uint64
	ScanDriftMicrosTotal  atomic.Uint64
	ScanDriftSamples      atomic.Uint64
	TotalScanLagMicros    atomic.Uint64
	ScanLagSamples        atomic.Uint64
	MaxScanLagMicros      atomic.Uint64
	IntervalAdjustedTotal atomic.Uint64

	// 恢复能力指标（对标 Kepware 诊断）：采集过程中被 recover 的 panic 次数
	// 与成功重新入队的次数。 nonzero TaskPanicsTotal 是必须告警的严重信号。
	TaskPanicsTotal     atomic.Uint64
	TaskRecoveriesTotal atomic.Uint64

	adaptiveFactorMu sync.RWMutex
	adaptiveFactor   float64

	lagMu      sync.Mutex
	lagSamples []int64

	driftWindowMu sync.Mutex
	driftWindow   []timedDriftSample

	missWindowMu sync.Mutex
	missWindow   []time.Time

	channelMu sync.RWMutex
	channels  map[string]*ScanEngineMetrics
}

func (m *ScanEngineMetrics) RecordExecute(success bool, lagMicros int64) {
	if m == nil {
		return
	}
	m.recordExecuteInternal(success, lagMicros)
}

func (m *ScanEngineMetrics) RecordExecuteForChannel(channelID string, success bool, lagMicros int64) {
	if m == nil {
		return
	}
	m.recordExecuteInternal(success, lagMicros)
	if cm := m.ensureChannelMetrics(channelID); cm != nil {
		cm.recordExecuteInternal(success, lagMicros)
	}
}

func (m *ScanEngineMetrics) recordExecuteInternal(success bool, lagMicros int64) {
	m.TasksExecuted.Add(1)
	if success {
		m.TasksSucceeded.Add(1)
	} else {
		m.TasksFailed.Add(1)
	}
	if lagMicros > 0 {
		m.TotalScanLagMicros.Add(uint64(lagMicros))
		m.ScanLagSamples.Add(1)
		for {
			prev := m.MaxScanLagMicros.Load()
			if uint64(lagMicros) <= prev {
				break
			}
			if m.MaxScanLagMicros.CompareAndSwap(prev, uint64(lagMicros)) {
				break
			}
		}
		m.recordLagSample(lagMicros)
	}
}

func (m *ScanEngineMetrics) recordLagSample(lagMicros int64) {
	m.lagMu.Lock()
	defer m.lagMu.Unlock()
	if len(m.lagSamples) >= scanLagSampleCap {
		copy(m.lagSamples, m.lagSamples[1:])
		m.lagSamples[len(m.lagSamples)-1] = lagMicros
		return
	}
	m.lagSamples = append(m.lagSamples, lagMicros)
}

func (m *ScanEngineMetrics) RecordStarvationRescue() {
	if m != nil {
		m.StarvationRescues.Add(1)
	}
}

func (m *ScanEngineMetrics) RecordOverdue() {
	if m != nil {
		m.TaskOverdueTotal.Add(1)
	}
}

func (m *ScanEngineMetrics) RecordMissDeadline() {
	if m == nil {
		return
	}
	m.recordMissDeadlineInternal()
}

func (m *ScanEngineMetrics) RecordMissDeadlineForChannel(channelID string) {
	if m == nil {
		return
	}
	m.recordMissDeadlineInternal()
	if cm := m.ensureChannelMetrics(channelID); cm != nil {
		cm.recordMissDeadlineInternal()
	}
}

func (m *ScanEngineMetrics) recordMissDeadlineInternal() {
	m.ScanMissDeadlineTotal.Add(1)
	now := time.Now()
	m.missWindowMu.Lock()
	m.missWindow = append(m.missWindow, now)
	cutoff := now.Add(-scanSLAWindowDuration)
	start := 0
	for start < len(m.missWindow) && !m.missWindow[start].After(cutoff) {
		start++
	}
	if start > 0 {
		m.missWindow = append([]time.Time(nil), m.missWindow[start:]...)
	}
	m.missWindowMu.Unlock()
}

func (m *ScanEngineMetrics) RecordDrift(driftMicros int64) {
	if m == nil || driftMicros <= 0 {
		return
	}
	m.recordDriftInternal(driftMicros)
}

func (m *ScanEngineMetrics) RecordDriftForChannel(channelID string, driftMicros int64) {
	if m == nil || driftMicros <= 0 {
		return
	}
	m.recordDriftInternal(driftMicros)
	if cm := m.ensureChannelMetrics(channelID); cm != nil {
		cm.recordDriftInternal(driftMicros)
	}
}

func (m *ScanEngineMetrics) recordDriftInternal(driftMicros int64) {
	// #nosec G115 -- 调用方已保证 driftMicros > 0
	m.ScanDriftMicrosTotal.Add(uint64(driftMicros))
	m.ScanDriftSamples.Add(1)
	now := time.Now()
	m.driftWindowMu.Lock()
	m.driftWindow = append(m.driftWindow, timedDriftSample{at: now, micros: driftMicros})
	cutoff := now.Add(-scanSLAWindowDuration)
	start := 0
	for start < len(m.driftWindow) && !m.driftWindow[start].at.After(cutoff) {
		start++
	}
	if start > 0 {
		m.driftWindow = append([]timedDriftSample(nil), m.driftWindow[start:]...)
	}
	m.driftWindowMu.Unlock()
}

func (m *ScanEngineMetrics) ensureChannelMetrics(channelID string) *ScanEngineMetrics {
	if channelID == "" {
		return nil
	}
	m.channelMu.RLock()
	if cm, ok := m.channels[channelID]; ok {
		m.channelMu.RUnlock()
		return cm
	}
	m.channelMu.RUnlock()

	m.channelMu.Lock()
	defer m.channelMu.Unlock()
	if m.channels == nil {
		m.channels = make(map[string]*ScanEngineMetrics)
	}
	if cm, ok := m.channels[channelID]; ok {
		return cm
	}
	cm := &ScanEngineMetrics{}
	m.channels[channelID] = cm
	return cm
}

func (m *ScanEngineMetrics) channelMetrics(channelID string) *ScanEngineMetrics {
	if m == nil || channelID == "" {
		return nil
	}
	m.channelMu.RLock()
	defer m.channelMu.RUnlock()
	if m.channels == nil {
		return nil
	}
	return m.channels[channelID]
}

func (m *ScanEngineMetrics) SetAdaptiveSlowdownFactor(factor float64) {
	if m == nil {
		return
	}
	m.adaptiveFactorMu.Lock()
	m.adaptiveFactor = factor
	m.adaptiveFactorMu.Unlock()
}

func (m *ScanEngineMetrics) AdaptiveSlowdownFactor() float64 {
	if m == nil {
		return 1.0
	}
	m.adaptiveFactorMu.RLock()
	defer m.adaptiveFactorMu.RUnlock()
	if m.adaptiveFactor <= 0 {
		return 1.0
	}
	return m.adaptiveFactor
}

func (m *ScanEngineMetrics) RecordIntervalAdjusted() {
	if m != nil {
		m.IntervalAdjustedTotal.Add(1)
	}
}

func (m *ScanEngineMetrics) GlobalFailRate() float64 {
	if m == nil {
		return 0
	}
	executed := m.TasksExecuted.Load()
	if executed == 0 {
		return 0
	}
	return float64(m.TasksFailed.Load()) / float64(executed)
}

func (m *ScanEngineMetrics) AvgLagMs() float64 {
	if m == nil {
		return 0
	}
	samples := m.ScanLagSamples.Load()
	if samples == 0 {
		return 0
	}
	return float64(m.TotalScanLagMicros.Load()) / float64(samples) / 1000.0
}

func (m *ScanEngineMetrics) scanLagP95Ms() float64 {
	m.lagMu.Lock()
	samples := append([]int64(nil), m.lagSamples...)
	m.lagMu.Unlock()
	if len(samples) == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	idx := (len(samples)*95 + 99) / 100
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	return float64(samples[idx]) / 1000.0
}

func (m *ScanEngineMetrics) driftAvgMsWindow() float64 {
	if m == nil {
		return 0
	}
	now := time.Now()
	cutoff := now.Add(-scanSLAWindowDuration)
	m.driftWindowMu.Lock()
	samples := m.driftWindow
	start := 0
	for start < len(samples) && !samples[start].at.After(cutoff) {
		start++
	}
	if start > 0 {
		samples = append([]timedDriftSample(nil), samples[start:]...)
		m.driftWindow = samples
	}
	var total int64
	for _, s := range samples {
		total += s.micros
	}
	m.driftWindowMu.Unlock()
	if len(samples) == 0 {
		return 0
	}
	return float64(total) / float64(len(samples)) / 1000.0
}

func (m *ScanEngineMetrics) missDeadlineWindow() uint64 {
	if m == nil {
		return 0
	}
	now := time.Now()
	cutoff := now.Add(-scanSLAWindowDuration)
	m.missWindowMu.Lock()
	events := m.missWindow
	start := 0
	for start < len(events) && !events[start].After(cutoff) {
		start++
	}
	if start > 0 {
		events = append([]time.Time(nil), events[start:]...)
		m.missWindow = events
	}
	count := uint64(len(events))
	m.missWindowMu.Unlock()
	return count
}

// ResetWindow clears cumulative counters and lag samples for steady-state measurement windows (benchmarks/soak).
func (m *ScanEngineMetrics) ResetWindow() {
	if m == nil {
		return
	}
	m.TasksExecuted.Store(0)
	m.TasksSucceeded.Store(0)
	m.TasksFailed.Store(0)
	m.StarvationRescues.Store(0)
	m.TaskOverdueTotal.Store(0)
	m.ScanMissDeadlineTotal.Store(0)
	m.ScanDriftMicrosTotal.Store(0)
	m.ScanDriftSamples.Store(0)
	m.TotalScanLagMicros.Store(0)
	m.ScanLagSamples.Store(0)
	m.MaxScanLagMicros.Store(0)
	m.IntervalAdjustedTotal.Store(0)
	m.lagMu.Lock()
	m.lagSamples = m.lagSamples[:0]
	m.lagMu.Unlock()
	m.driftWindowMu.Lock()
	m.driftWindow = m.driftWindow[:0]
	m.driftWindowMu.Unlock()
	m.missWindowMu.Lock()
	m.missWindow = m.missWindow[:0]
	m.missWindowMu.Unlock()
	m.channelMu.Lock()
	m.channels = nil
	m.channelMu.Unlock()
}

func (m *ScanEngineMetrics) snapshotFields() map[string]any {
	samples := m.ScanLagSamples.Load()
	avgLagMs := float64(0)
	if samples > 0 {
		avgLagMs = float64(m.TotalScanLagMicros.Load()) / float64(samples) / 1000.0
	}
	driftSamples := m.ScanDriftSamples.Load()
	avgDriftMs := float64(0)
	if driftSamples > 0 {
		avgDriftMs = float64(m.ScanDriftMicrosTotal.Load()) / float64(driftSamples) / 1000.0
	}
	return map[string]any{
		"tasks_executed":               m.TasksExecuted.Load(),
		"tasks_succeeded":              m.TasksSucceeded.Load(),
		"tasks_failed":                 m.TasksFailed.Load(),
		"starvation_rescue_total":      m.StarvationRescues.Load(),
		"task_overdue_total":           m.TaskOverdueTotal.Load(),
		"scan_miss_deadline_total":     m.ScanMissDeadlineTotal.Load(),
		"scan_miss_deadline_window":    m.missDeadlineWindow(),
		"scan_drift_avg_ms":            avgDriftMs,
		"scan_drift_avg_ms_window":     m.driftAvgMsWindow(),
		"scan_drift_samples":           driftSamples,
		"scan_lag_avg_ms":              avgLagMs,
		"scan_lag_p95_ms":              m.scanLagP95Ms(),
		"scan_lag_max_ms":              float64(m.MaxScanLagMicros.Load()) / 1000.0,
		"scan_lag_samples":             samples,
		"adaptive_slowdown_factor":     m.AdaptiveSlowdownFactor(),
		"scan_interval_adjusted_total": m.IntervalAdjustedTotal.Load(),
		"task_panics_total":            m.TaskPanicsTotal.Load(),
		"task_recoveries_total":        m.TaskRecoveriesTotal.Load(),
	}
}

func (m *ScanEngineMetrics) Snapshot() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m.snapshotFields()
}

func (m *ScanEngineMetrics) ChannelSnapshot(channelID string) map[string]any {
	cm := m.channelMetrics(channelID)
	if cm == nil {
		return map[string]any{
			"scan_lag_p95_ms":           float64(0),
			"scan_drift_avg_ms":         float64(0),
			"scan_drift_avg_ms_window":  float64(0),
			"scan_miss_deadline_total":  uint64(0),
			"scan_miss_deadline_window": uint64(0),
			"scan_lag_samples":          uint64(0),
			"scan_drift_samples":        uint64(0),
		}
	}
	return cm.snapshotFields()
}

func (m *ScanEngineMetrics) slaWarningsFromSnapshot(snap map[string]any, cb *DriverCircuitBreaker, deviceKeys []string) []map[string]any {
	if m == nil {
		return nil
	}
	var warnings []map[string]any

	if p95, ok := snap["scan_lag_p95_ms"].(float64); ok && p95 > SLAScanLagP95MsThreshold {
		warnings = append(warnings, map[string]any{
			"code":      "scan_lag_p95_exceeded",
			"metric":    "scan_lag_p95_ms",
			"value":     p95,
			"threshold": SLAScanLagP95MsThreshold,
			"message":   fmt.Sprintf("scan lag P95 %.2fms exceeds %.0fms", p95, SLAScanLagP95MsThreshold),
		})
	}
	if drift, ok := snap["scan_drift_avg_ms_window"].(float64); ok && drift > SLAScanDriftAvgMsThreshold {
		warnings = append(warnings, map[string]any{
			"code":      "scan_drift_avg_exceeded",
			"metric":    "scan_drift_avg_ms_window",
			"value":     drift,
			"threshold": SLAScanDriftAvgMsThreshold,
			"message":   fmt.Sprintf("scan drift avg (5m window) %.2fms exceeds %.0fms", drift, SLAScanDriftAvgMsThreshold),
		})
	}
	if missed, ok := snap["scan_miss_deadline_window"].(uint64); ok && missed > SLAScanMissDeadlineMax {
		warnings = append(warnings, map[string]any{
			"code":      "scan_miss_deadline_exceeded",
			"metric":    "scan_miss_deadline_window",
			"value":     missed,
			"threshold": SLAScanMissDeadlineMax,
			"message":   fmt.Sprintf("scan miss deadline (5m window) %d exceeds %d", missed, SLAScanMissDeadlineMax),
		})
	}

	if len(deviceKeys) > 0 && cb != nil {
		openCount := 0
		for _, key := range deviceKeys {
			if cb.State(key) == CircuitOpen {
				openCount++
			}
		}
		if openCount > 0 {
			warnings = append(warnings, map[string]any{
				"code":      "circuit_breaker_open",
				"metric":    "circuit_breaker_open_devices",
				"value":     openCount,
				"threshold": 0,
				"message":   fmt.Sprintf("circuit breaker open on %d device(s) in channel", openCount),
			})
		}
	} else if cb != nil && len(deviceKeys) == 0 {
		if rejects := cb.RejectTotal(); rejects > SLACircuitBreakerRejectMax {
			warnings = append(warnings, map[string]any{
				"code":      "circuit_breaker_rejects",
				"metric":    "circuit_breaker_reject_total",
				"value":     rejects,
				"threshold": SLACircuitBreakerRejectMax,
				"message":   fmt.Sprintf("circuit breaker rejected %d requests", rejects),
			})
		}
	}
	return warnings
}

func (m *ScanEngineMetrics) SLAWarnings(cb *DriverCircuitBreaker) []map[string]any {
	return m.slaWarningsFromSnapshot(m.Snapshot(), cb, nil)
}

func (m *ScanEngineMetrics) ChannelSLAWarnings(channelID string, cb *DriverCircuitBreaker, deviceKeys []string) []map[string]any {
	if m == nil || channelID == "" {
		return nil
	}
	return m.slaWarningsFromSnapshot(m.ChannelSnapshot(channelID), cb, deviceKeys)
}
