package core

import (
	"sync"
	"time"
)

const (
	adaptiveThrottleMaxFactor = 4.0
	deviceRTTMinFactor        = 1.5
	deviceRTTMaxFactor        = 4.0
)

type deviceRTTState struct {
	mu         sync.Mutex
	baselineMs float64
	factor     float64
}

type AdaptiveThrottle struct {
	mu     sync.RWMutex
	factor float64

	deviceStates sync.Map // deviceKey -> *deviceRTTState

	metrics *ScanEngineMetrics
}

func NewAdaptiveThrottle(metrics *ScanEngineMetrics) *AdaptiveThrottle {
	return &AdaptiveThrottle{
		factor:  1.0,
		metrics: metrics,
	}
}

func (at *AdaptiveThrottle) Refresh(queueDepth, queueLimit int, failRate, avgRTTMs float64) float64 {
	factor := 1.0

	if queueLimit > 0 {
		ratio := float64(queueDepth) / float64(queueLimit)
		switch {
		case ratio >= 0.9:
			factor += 2.0
		case ratio >= 0.75:
			factor += 1.5
		case ratio >= 0.5:
			factor += 1.0
		case ratio >= 0.25:
			factor += 0.5
		}
	}

	if failRate > 0.05 {
		factor += failRate * 4.0
	}

	if avgRTTMs > 100 {
		rttBoost := (avgRTTMs - 100) / 100
		if rttBoost > 2.0 {
			rttBoost = 2.0
		}
		factor += rttBoost
	}

	if factor > adaptiveThrottleMaxFactor {
		factor = adaptiveThrottleMaxFactor
	}
	if factor < 1.0 {
		factor = 1.0
	}

	at.mu.Lock()
	at.factor = factor
	at.mu.Unlock()

	if at.metrics != nil {
		at.metrics.SetAdaptiveSlowdownFactor(factor)
	}

	return factor
}

func (at *AdaptiveThrottle) Factor() float64 {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.factor
}

func (at *AdaptiveThrottle) EffectiveInterval(base time.Duration) time.Duration {
	return at.effectiveIntervalForDevice("", base)
}

func (at *AdaptiveThrottle) effectiveIntervalForDevice(deviceKey string, base time.Duration) time.Duration {
	if base <= 0 {
		base = time.Millisecond
	}
	factor := at.Factor()
	if deviceKey != "" {
		factor *= at.deviceFactor(deviceKey)
	}
	if factor > adaptiveThrottleMaxFactor {
		factor = adaptiveThrottleMaxFactor
	}
	interval := time.Duration(float64(base) * factor)
	if interval < time.Millisecond {
		return time.Millisecond
	}
	return interval
}

// UpdateDeviceRTT records per-device RTT and raises interval factor when RTT > 2× baseline.
func (at *AdaptiveThrottle) UpdateDeviceRTT(deviceKey string, rttMs float64) {
	if at == nil || deviceKey == "" || rttMs <= 0 {
		return
	}

	raw, _ := at.deviceStates.LoadOrStore(deviceKey, &deviceRTTState{baselineMs: rttMs})
	st := raw.(*deviceRTTState)
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.baselineMs <= 0 {
		st.baselineMs = rttMs
	} else {
		st.baselineMs = st.baselineMs*0.9 + rttMs*0.1
	}

	st.factor = 1.0
	if st.baselineMs > 0 && rttMs > st.baselineMs*2 {
		ratio := rttMs / st.baselineMs
		st.factor = 1.5 + (ratio-2)*0.75
		if st.factor < deviceRTTMinFactor {
			st.factor = deviceRTTMinFactor
		}
		if st.factor > deviceRTTMaxFactor {
			st.factor = deviceRTTMaxFactor
		}
	}
}

func (at *AdaptiveThrottle) deviceFactor(deviceKey string) float64 {
	if at == nil || deviceKey == "" {
		return 1.0
	}
	raw, ok := at.deviceStates.Load(deviceKey)
	if !ok {
		return 1.0
	}
	st := raw.(*deviceRTTState)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.factor <= 0 {
		return 1.0
	}
	return st.factor
}

func (at *AdaptiveThrottle) DeviceFactor(deviceKey string) float64 {
	return at.deviceFactor(deviceKey)
}

func (at *AdaptiveThrottle) ApplyInterval(task *ScanTask) bool {
	if at == nil || task == nil {
		return false
	}

	task.mu.RLock()
	base, current := task.BaseInterval, task.Interval
	task.mu.RUnlock()
	if base <= 0 {
		base = current
	}
	if base <= 0 {
		return false
	}

	effective := at.effectiveIntervalForDevice(task.DeviceKey, base)
	task.mu.Lock()
	if effective < current {
		task.mu.Unlock()
		return false
	}
	changed := effective != current
	task.Interval = effective
	task.mu.Unlock()

	if changed && at.metrics != nil {
		at.metrics.RecordIntervalAdjusted()
	}
	return changed
}

func (at *AdaptiveThrottle) applyIntervalLocked(task *ScanTask) bool {
	if at == nil || task == nil {
		return false
	}

	base := task.BaseInterval
	if base <= 0 {
		base = task.Interval
	}
	if base <= 0 {
		return false
	}

	effective := at.effectiveIntervalForDevice(task.DeviceKey, base)
	if effective < task.Interval {
		return false
	}
	changed := effective != task.Interval
	task.Interval = effective

	if changed && at.metrics != nil {
		at.metrics.RecordIntervalAdjusted()
	}
	return changed
}
