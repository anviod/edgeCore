package capability

import (
	"sort"
	"sync"
	"sync/atomic"
)

// metricsRingCap is the max number of latency samples retained for percentile calculation.
const metricsRingCap = 100

// invokeMetricsCollector is a thread-safe collector for EAN Invoke metrics.
// 使用原子计数器 + 环形缓冲区实现无锁读、低争用写。
// | Lock-free reads via atomic counters; low-contention writes via ring buffer.
type invokeMetricsCollector struct {
	totalInvokes   int64
	successCount   int64
	failedCount    int64
	timeoutCount   int64
	rejectedCount  int64
	totalLatencyMs int64
	minLatencyMs   int64
	maxLatencyMs   int64

	// 延迟环形缓冲区 | Latency ring buffer
	latMu      sync.Mutex
	latencies  []int64
	latIdx     int
	latFilled  bool

	// 错误码计数 | Error code counters
	errMu     sync.Mutex
	errCounts map[string]int64
}

func newInvokeMetricsCollector() *invokeMetricsCollector {
	return &invokeMetricsCollector{
		latencies:  make([]int64, metricsRingCap),
		errCounts:  make(map[string]int64),
		minLatencyMs: -1, // sentinel: -1 means unset
	}
}

// Record captures a single Invoke result into the metrics collector.
// 在 Runtime.Invoke 返回后调用。
// | Called after Runtime.Invoke returns.
func (m *invokeMetricsCollector) Record(resp InvokeResponse) {
	atomic.AddInt64(&m.totalInvokes, 1)

	lat := resp.LatencyMs
	if lat < 0 {
		lat = 0
	}
	atomic.AddInt64(&m.totalLatencyMs, lat)

	// 更新 min/max（无锁 CAS 循环）| Update min/max via CAS loop
	for {
		oldMin := atomic.LoadInt64(&m.minLatencyMs)
		if oldMin >= 0 && lat >= oldMin {
			break
		}
		if atomic.CompareAndSwapInt64(&m.minLatencyMs, oldMin, lat) {
			break
		}
	}
	for {
		oldMax := atomic.LoadInt64(&m.maxLatencyMs)
		if lat <= oldMax {
			break
		}
		if atomic.CompareAndSwapInt64(&m.maxLatencyMs, oldMax, lat) {
			break
		}
	}

	// 按状态分类计数 | Count by status
	switch resp.Status {
	case InvokeCompleted:
		atomic.AddInt64(&m.successCount, 1)
	case InvokeFailed:
		atomic.AddInt64(&m.failedCount, 1)
		m.recordError(resp.Result.ErrorCode)
	case InvokeTimeout:
		atomic.AddInt64(&m.timeoutCount, 1)
		m.recordError(resp.Result.ErrorCode)
	case InvokeRejected:
		atomic.AddInt64(&m.rejectedCount, 1)
		m.recordError(resp.Result.ErrorCode)
	}

	// 写入延迟环形缓冲区 | Push to latency ring buffer
	m.latMu.Lock()
	m.latencies[m.latIdx] = lat
	m.latIdx = (m.latIdx + 1) % metricsRingCap
	if m.latIdx == 0 {
		m.latFilled = true
	}
	m.latMu.Unlock()
}

func (m *invokeMetricsCollector) recordError(code string) {
	if code == "" {
		code = "UNKNOWN"
	}
	m.errMu.Lock()
	m.errCounts[code]++
	m.errMu.Unlock()
}

// Snapshot returns a point-in-time copy of the collected metrics.
func (m *invokeMetricsCollector) Snapshot() InvokeMetricsSnapshot {
	total := atomic.LoadInt64(&m.totalInvokes)
	success := atomic.LoadInt64(&m.successCount)
	failed := atomic.LoadInt64(&m.failedCount)
	timeout := atomic.LoadInt64(&m.timeoutCount)
	rejected := atomic.LoadInt64(&m.rejectedCount)
	totalLat := atomic.LoadInt64(&m.totalLatencyMs)
	minLat := atomic.LoadInt64(&m.minLatencyMs)
	maxLat := atomic.LoadInt64(&m.maxLatencyMs)

	snap := InvokeMetricsSnapshot{
		TotalInvokes:  total,
		SuccessCount:  success,
		FailedCount:   failed,
		TimeoutCount:  timeout,
		RejectedCount: rejected,
		MinLatencyMs:  minLat,
		MaxLatencyMs:  maxLat,
	}

	if total > 0 {
		snap.SuccessRate = float64(success) / float64(total) * 100
		snap.AvgLatencyMs = float64(totalLat) / float64(total)
	}

	// 计算百分位数 | Compute percentiles from ring buffer
	m.latMu.Lock()
	var sorted []int64
	if m.latFilled {
		sorted = make([]int64, metricsRingCap)
		copy(sorted, m.latencies)
	} else if m.latIdx > 0 {
		sorted = make([]int64, m.latIdx)
		copy(sorted, m.latencies[:m.latIdx])
	}
	m.latMu.Unlock()

	if len(sorted) > 0 {
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		snap.P50LatencyMs = percentile(sorted, 50)
		snap.P99LatencyMs = percentile(sorted, 99)
	}

	// Top 5 错误码 | Top 5 error codes
	m.errMu.Lock()
	if len(m.errCounts) > 0 {
		errs := make([]ErrorCounter, 0, len(m.errCounts))
		for code, cnt := range m.errCounts {
			errs = append(errs, ErrorCounter{Code: code, Count: cnt})
		}
		sort.Slice(errs, func(i, j int) bool { return errs[i].Count > errs[j].Count })
		if len(errs) > 5 {
			errs = errs[:5]
		}
		snap.TopErrors = errs
	}
	m.errMu.Unlock()

	return snap
}

// percentile returns the p-th percentile from a sorted slice.
// Uses nearest-rank method.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted) + 99) / 100 // ceil(p/100 * n)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
