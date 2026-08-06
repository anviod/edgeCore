package core

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

const (
	SoakSampleInterval         = 15 * time.Second
	SoakMaxTrendSamples        = 480
	SoakBacklogExcessThreshold = 10 // max allowed backlog above registered task count
	SoakPointSuccessRateGate   = 0.99
)

type soakTrendSample struct {
	TotalBacklog       int `json:"total_backlog"`
	CircuitBreakerOpen int `json:"circuit_breaker_open"`
	GlobalQueue        int `json:"global_queue"`
	ScanClassLate      int `json:"scan_class_late"`
}

type soakReleaseGateItem struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Passed  bool   `json:"passed"`
	Detail  string `json:"detail"`
	Value   any    `json:"value,omitempty"`
	Limit   any    `json:"limit,omitempty"`
	Warning bool   `json:"warning,omitempty"`
}

type soakScanClassRow struct {
	Class   string  `json:"class"`
	Tasks   int     `json:"tasks"`
	Backlog int     `json:"backlog"`
	Queue   int     `json:"queue"`
	Late    int     `json:"late"`
	Success float64 `json:"success"`
}

type soakInstantMetrics struct {
	Running              bool
	TaskCount            int
	TotalBacklog         int
	SerialQueueDepth     int
	CircuitBreakerOpen   int
	Throttled            bool
	ThrottleStatus       string
	ThrottleFactor       float64
	GlobalQueue          int
	GlobalQueueLimit     int
	ScanClassLate        int
	ScanClasses          []soakScanClassRow
	MinPointSuccessRate  float64
	MinPointSuccessLabel string
}

// SoakMonitor tracks in-process ScanEngine SLA samples for dashboard soak views.
type SoakMonitor struct {
	cm *ChannelManager

	mu sync.RWMutex

	startedAt time.Time
	samples   []soakTrendSample

	maxBacklog            int
	maxExcessBacklog      int
	maxCircuitBreakerOpen int
	everThrottled         bool
	minPointSuccessRate   float64
	minPointSuccessLabel  string

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewSoakMonitor(cm *ChannelManager) *SoakMonitor {
	return &SoakMonitor{
		cm:                   cm,
		minPointSuccessRate:  1.0,
		minPointSuccessLabel: "",
		stopCh:               make(chan struct{}),
	}
}

func (sm *SoakMonitor) Start() {
	sm.mu.Lock()
	if !sm.startedAt.IsZero() {
		sm.mu.Unlock()
		return
	}
	sm.startedAt = time.Now()
	sm.mu.Unlock()

	sm.wg.Add(1)
	go sm.loop()
}

func (sm *SoakMonitor) Stop() {
	select {
	case <-sm.stopCh:
	default:
		close(sm.stopCh)
	}
	sm.wg.Wait()
}

func (sm *SoakMonitor) loop() {
	defer sm.wg.Done()

	ticker := time.NewTicker(SoakSampleInterval)
	defer ticker.Stop()

	sm.recordSample()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.recordSample()
		}
	}
}

func (sm *SoakMonitor) recordSample() {
	if sm.cm == nil {
		return
	}
	instant := sm.cm.collectSoakInstantMetrics()
	sample := soakTrendSample{
		TotalBacklog:       instant.TotalBacklog,
		CircuitBreakerOpen: instant.CircuitBreakerOpen,
		GlobalQueue:        instant.GlobalQueue,
		ScanClassLate:      instant.ScanClassLate,
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.samples = append(sm.samples, sample)
	if len(sm.samples) > SoakMaxTrendSamples {
		sm.samples = sm.samples[len(sm.samples)-SoakMaxTrendSamples:]
	}

	if instant.TotalBacklog > sm.maxBacklog {
		sm.maxBacklog = instant.TotalBacklog
	}
	excess := soakExcessBacklog(instant.TotalBacklog, instant.TaskCount)
	if excess > sm.maxExcessBacklog {
		sm.maxExcessBacklog = excess
	}
	if instant.CircuitBreakerOpen > sm.maxCircuitBreakerOpen {
		sm.maxCircuitBreakerOpen = instant.CircuitBreakerOpen
	}
	if instant.Throttled {
		sm.everThrottled = true
	}
	if instant.MinPointSuccessLabel != "" && instant.MinPointSuccessRate < sm.minPointSuccessRate {
		sm.minPointSuccessRate = instant.MinPointSuccessRate
		sm.minPointSuccessLabel = instant.MinPointSuccessLabel
	}
}

func (sm *SoakMonitor) Snapshot() map[string]any {
	instant := soakInstantMetrics{}
	if sm.cm != nil {
		instant = sm.cm.collectSoakInstantMetrics()
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	elapsed := time.Duration(0)
	if !sm.startedAt.IsZero() {
		elapsed = time.Since(sm.startedAt)
	}

	gates := sm.buildReleaseGateItems(instant)
	allPassed := true
	partialFailed := false
	for _, g := range gates {
		if !g.Passed {
			allPassed = false
			partialFailed = true
		}
	}

	trends := sm.trendSeriesLocked()

	minRate := sm.minPointSuccessRate
	minLabel := sm.minPointSuccessLabel
	if instant.MinPointSuccessLabel != "" && instant.MinPointSuccessRate < minRate {
		minRate = instant.MinPointSuccessRate
		minLabel = instant.MinPointSuccessLabel
	}

	return map[string]any{
		"session": map[string]any{
			"sample_count":        len(sm.samples),
			"elapsed_sec":         int(elapsed.Seconds()),
			"elapsed_display":     formatSoakDuration(elapsed),
			"sample_interval_sec": int(SoakSampleInterval.Seconds()),
			"started_at":          sm.startedAt,
		},
		"release_gate": map[string]any{
			"all_passed":     allPassed,
			"partial_failed": partialFailed && !allPassed,
			"items":          gates,
		},
		"snapshot": map[string]any{
			"task_count":           instant.TaskCount,
			"total_backlog":        instant.TotalBacklog,
			"circuit_breaker_open": instant.CircuitBreakerOpen,
			"throttled":            instant.Throttled,
			"throttle_status":      instant.ThrottleStatus,
			"throttle_factor":      instant.ThrottleFactor,
			"global_queue":         instant.GlobalQueue,
			"global_queue_limit":   instant.GlobalQueueLimit,
			"scan_class_late":      instant.ScanClassLate,
		},
		"session_summary": map[string]any{
			"max_backlog":              sm.maxBacklog,
			"max_excess_backlog":       sm.maxExcessBacklog,
			"max_circuit_breaker_open": sm.maxCircuitBreakerOpen,
			"ever_throttled":           sm.everThrottled,
			"min_point_success_rate":   minRate,
			"min_point_success_label":  minLabel,
		},
		"trends":       trends,
		"scan_classes": instant.ScanClasses,
	}
}

func (sm *SoakMonitor) trendSeriesLocked() map[string][]int {
	out := map[string][]int{
		"total_backlog":        make([]int, 0, len(sm.samples)),
		"circuit_breaker_open": make([]int, 0, len(sm.samples)),
		"global_queue":         make([]int, 0, len(sm.samples)),
		"scan_class_late":      make([]int, 0, len(sm.samples)),
	}
	for _, s := range sm.samples {
		out["total_backlog"] = append(out["total_backlog"], s.TotalBacklog)
		out["circuit_breaker_open"] = append(out["circuit_breaker_open"], s.CircuitBreakerOpen)
		out["global_queue"] = append(out["global_queue"], s.GlobalQueue)
		out["scan_class_late"] = append(out["scan_class_late"], s.ScanClassLate)
	}
	return out
}

func soakExcessBacklog(totalBacklog, taskCount int) int {
	excess := totalBacklog - taskCount
	if excess < 0 {
		return 0
	}
	return excess
}

func soakBacklogGatePassed(instant soakInstantMetrics, sessionMaxExcess int) bool {
	excess := soakExcessBacklog(instant.TotalBacklog, instant.TaskCount)
	maxExcess := sessionMaxExcess
	if excess > maxExcess {
		maxExcess = excess
	}
	return excess <= SoakBacklogExcessThreshold && maxExcess <= SoakBacklogExcessThreshold
}

func soakBacklogGateDetail(instant soakInstantMetrics, sessionMaxExcess int) string {
	excess := soakExcessBacklog(instant.TotalBacklog, instant.TaskCount)
	maxExcess := sessionMaxExcess
	if excess > maxExcess {
		maxExcess = excess
	}
	return fmt.Sprintf(
		"超基线 %d/峰 %d ≤%d · 串行 %d",
		excess, maxExcess, SoakBacklogExcessThreshold, instant.SerialQueueDepth,
	)
}

func (sm *SoakMonitor) buildReleaseGateItems(instant soakInstantMetrics) []soakReleaseGateItem {
	sm.mu.RLock()
	maxExcessBacklog := sm.maxExcessBacklog
	maxCBOpen := sm.maxCircuitBreakerOpen
	everThrottled := sm.everThrottled
	minRate := sm.minPointSuccessRate
	minLabel := sm.minPointSuccessLabel
	sm.mu.RUnlock()

	if instant.MinPointSuccessLabel != "" && instant.MinPointSuccessRate < minRate {
		minRate = instant.MinPointSuccessRate
		minLabel = instant.MinPointSuccessLabel
	}

	runningDetail := "running=false"
	if instant.Running {
		runningDetail = "running=true"
	}

	throttleDetail := "throttled=true"
	if !instant.Throttled && !everThrottled {
		throttleDetail = "throttled=false"
	} else if instant.Throttled {
		throttleDetail = fmt.Sprintf("throttled=true (factor=%.2f)", instant.ThrottleFactor)
	} else {
		throttleDetail = "throttled=false（会话内曾出现）"
	}

	pointDetail := fmt.Sprintf("最低 %.1f%%", minRate*100)
	if minLabel != "" {
		pointDetail += fmt.Sprintf("（%s）", minLabel)
	}

	return []soakReleaseGateItem{
		{
			ID:     "scan_engine_running",
			Label:  "ScanEngine 运行中",
			Passed: instant.Running,
			Detail: runningDetail,
		},
		{
			ID:     "circuit_breaker_closed",
			Label:  "断路器关闭",
			Passed: instant.CircuitBreakerOpen == 0 && maxCBOpen == 0,
			Detail: fmt.Sprintf("当前 %d/峰 %d", instant.CircuitBreakerOpen, maxCBOpen),
			Value:  instant.CircuitBreakerOpen,
			Limit:  0,
		},
		{
			ID:     "no_throttle",
			Label:  "无节流",
			Passed: !instant.Throttled && !everThrottled,
			Detail: throttleDetail,
		},
		{
			ID:     "backlog_stable",
			Label:  "积压稳定",
			Passed: soakBacklogGatePassed(instant, maxExcessBacklog),
			Detail: soakBacklogGateDetail(instant, maxExcessBacklog),
			Value:  soakExcessBacklog(instant.TotalBacklog, instant.TaskCount),
			Limit:  SoakBacklogExcessThreshold,
		},
		{
			ID:     "scan_class_on_time",
			Label:  "Scan Class 无迟到",
			Passed: instant.ScanClassLate == 0,
			Detail: fmt.Sprintf("合计迟到 %d", instant.ScanClassLate),
			Value:  instant.ScanClassLate,
			Limit:  0,
		},
		{
			ID:      "point_success_rate",
			Label:   "点位成功率 ≥ 99%",
			Passed:  minRate >= SoakPointSuccessRateGate,
			Detail:  pointDetail,
			Value:   minRate,
			Limit:   SoakPointSuccessRateGate,
			Warning: minRate < SoakPointSuccessRateGate,
		},
	}
}

func (cm *ChannelManager) collectSoakInstantMetrics() soakInstantMetrics {
	out := soakInstantMetrics{
		ThrottleStatus:      "正常",
		MinPointSuccessRate: 1.0,
		GlobalQueueLimit:    10000,
	}
	if cm == nil {
		return out
	}

	se := cm.scanEngineAdapter.scanEngine
	if se == nil {
		return out
	}

	out.Running = se.IsRunning()
	out.GlobalQueueLimit = se.config.MaxQueueSize
	if out.GlobalQueueLimit <= 0 {
		out.GlobalQueueLimit = 10000
	}

	out.GlobalQueue = se.GetPendingTaskCount()
	out.TaskCount = len(se.GetTasks())

	serialDepth := 0
	if se.executionLayer != nil {
		for _, depth := range se.executionLayer.GetSerialQueueDepths() {
			serialDepth += depth
		}
	}
	out.SerialQueueDepth = serialDepth
	out.TotalBacklog = out.GlobalQueue + se.GetActiveTaskCount() + serialDepth

	if cb := se.GetCircuitBreaker(); cb != nil {
		out.CircuitBreakerOpen = countOpenCircuits(cb)
	}

	factor := 1.0
	if se.metrics != nil {
		factor = se.metrics.AdaptiveSlowdownFactor()
	}
	out.ThrottleFactor = factor
	out.Throttled = factor > 1.0
	if out.Throttled {
		out.ThrottleStatus = fmt.Sprintf("节流 (×%.2f)", factor)
	}

	now := time.Now()
	classStats := map[string]*soakScanClassRow{}
	lateTotal := 0

	for _, task := range se.GetTasks() {
		if task == nil {
			continue
		}
		label := soakScanClassLabel(task)
		row, ok := classStats[label]
		if !ok {
			row = &soakScanClassRow{Class: label}
			classStats[label] = row
		}
		row.Tasks++

		task.mu.RLock()
		isLate := !task.NextRun.IsZero() && now.After(task.NextRun) && task.Status == ScanTaskStatusIdle
		isRunning := task.Status == ScanTaskStatusRunning
		failRate := task.FailRate
		task.mu.RUnlock()

		if isLate {
			row.Late++
			lateTotal++
		}
		if isRunning {
			row.Backlog++
		}
		row.Queue++

		success := 1.0 - failRate
		if success < 0 {
			success = 0
		}
		if row.Tasks == 1 {
			row.Success = success
		} else {
			row.Success = ((row.Success * float64(row.Tasks-1)) + success) / float64(row.Tasks)
		}
	}

	out.ScanClassLate = lateTotal
	out.ScanClasses = make([]soakScanClassRow, 0, len(classStats))
	for _, row := range classStats {
		out.ScanClasses = append(out.ScanClasses, *row)
	}
	sort.Slice(out.ScanClasses, func(i, j int) bool {
		return out.ScanClasses[i].Class < out.ScanClasses[j].Class
	})

	minRate, minLabel := cm.minChannelPointSuccessRate()
	out.MinPointSuccessRate = minRate
	out.MinPointSuccessLabel = minLabel

	return out
}

func (cm *ChannelManager) minChannelPointSuccessRate() (float64, string) {
	mc := model.GetGlobalMetricsCollector()
	if mc == nil || cm == nil {
		return 1.0, ""
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	minRate := 1.0
	minLabel := ""
	found := false

	for id, ch := range cm.channels {
		if !ch.Enable {
			continue
		}
		metrics := mc.GetChannelMetrics(id)
		if metrics == nil {
			continue
		}
		rate := metrics.SuccessRate
		if metrics.TotalRequests == 0 && metrics.SuccessCount == 0 && metrics.FailureCount == 0 {
			continue
		}
		found = true
		if rate < minRate {
			minRate = rate
			minLabel = ch.Name
		}
	}

	if !found {
		return 1.0, ""
	}
	return minRate, minLabel
}

func (cm *ChannelManager) GetSoakMonitorSnapshot() map[string]any {
	if cm == nil || cm.soakMonitor == nil {
		return map[string]any{}
	}
	return cm.soakMonitor.Snapshot()
}

func countOpenCircuits(cb *DriverCircuitBreaker) int {
	snap := cb.Snapshot()
	devices, _ := snap["devices"].(map[string]any)
	if len(devices) == 0 {
		return 0
	}
	open := 0
	for _, raw := range devices {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		state, _ := entry["state"].(string)
		if state == CircuitOpen.String() {
			open++
		}
	}
	return open
}

func soakScanClassLabel(task *ScanTask) string {
	if task == nil {
		return "unknown"
	}
	task.mu.RLock()
	interval := task.Interval
	scanClass := task.ScanClass
	task.mu.RUnlock()

	if interval > 0 {
		return formatSoakInterval(interval)
	}
	if scanClass != "" {
		return scanClass
	}
	return "normal"
}

func formatSoakInterval(d time.Duration) string {
	switch {
	case d >= time.Second && d%time.Second == 0:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d >= time.Millisecond && d%time.Millisecond == 0:
		return fmt.Sprintf("%dms", int(d/time.Millisecond))
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return d.String()
	}
}

func formatSoakDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
