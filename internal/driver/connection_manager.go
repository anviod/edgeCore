package driver

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const connectingMinBackoff = 200 * time.Millisecond

func ensureConnectingMinBackoff(state ConnState, wait time.Duration) time.Duration {
	if state == StateConnecting && wait < connectingMinBackoff {
		return connectingMinBackoff
	}
	return wait
}

// ConnectFunc performs a single connection attempt (dial/open). Retry/backoff is
// owned exclusively by ConnectionManager.EnsureConnected.
type ConnectFunc func(ctx context.Context) error

type ConnState int

const (
	StateDisconnected ConnState = iota
	StateConnecting
	StateConnected
	StateRetrying
	StateDead
)

func (s ConnState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateRetrying:
		return "Retrying"
	case StateDead:
		return "Dead"
	default:
		return "Unknown"
	}
}

type ConnectionManager struct {
	mu               sync.Mutex
	state            ConnState
	retryCount       int
	maxRetries       int
	lastRetryTime    time.Time
	lastSuccessTime  time.Time
	coolDownUntil    time.Time
	coolDownDuration time.Duration
	coolDownAttempts int
	coolDownBase     time.Duration

	baseDelay     time.Duration
	maxDelay      time.Duration
	backoffFactor float64

	maxFailCount int

	dailyResetTimer   *time.Timer
	dailyResetEnabled bool

	driverName string

	reconnectRunning atomic.Bool

	bgMu     sync.Mutex
	bgCancel context.CancelFunc
	bgDone   chan struct{}

	// linkMu is the shared-link channelMu (driverMus). Held only around dial/
	// install (not during backoff sleep) so reconnect cannot race I/O.
	linkMu *sync.Mutex
}

func NewConnectionManager(driverName string) *ConnectionManager {
	cm := &ConnectionManager{
		state:             StateDisconnected,
		baseDelay:         100 * time.Millisecond,
		maxDelay:          30 * time.Second,
		backoffFactor:     2.0,
		coolDownBase:      1 * time.Minute,
		coolDownDuration:  1 * time.Minute,
		maxRetries:        64,
		maxFailCount:      5,
		dailyResetEnabled: true,
		driverName:        driverName,
	}

	cm.startDailyReset()

	return cm
}

func (cm *ConnectionManager) startDailyReset() {
	if !cm.dailyResetEnabled {
		return
	}

	now := time.Now()
	nextMidnight := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	duration := nextMidnight.Sub(now)

	cm.dailyResetTimer = time.AfterFunc(duration, func() {
		cm.ResetDaily()
		cm.startDailyReset()
	})
}

func (cm *ConnectionManager) ResetDaily() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.retryCount = 0
	cm.coolDownAttempts = 0
	cm.coolDownDuration = 1 * time.Minute

	zap.L().Info("[ConnMgr] Daily reset",
		zap.String("driver", cm.driverName),
	)
}

func (cm *ConnectionManager) SetState(state ConnState) {
	cm.mu.Lock()
	oldState := cm.state
	cm.state = state
	cm.mu.Unlock()

	if oldState != state {
		zap.L().Info("[ConnMgr] Connection state changed",
			zap.String("driver", cm.driverName),
			zap.String("from", oldState.String()),
			zap.String("to", state.String()),
		)
	}
}

func (cm *ConnectionManager) GetState() ConnState {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.state
}

func (cm *ConnectionManager) RecordSuccess() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.retryCount = 0
	cm.lastSuccessTime = time.Now()
	cm.coolDownAttempts = 0
	cm.coolDownDuration = 1 * time.Minute

	if cm.state != StateConnected {
		cm.state = StateConnected
		zap.L().Info("[ConnMgr] Connection recovered",
			zap.String("driver", cm.driverName),
		)
	}
}

func (cm *ConnectionManager) RecordFailure() (shouldRetry bool, backoff time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.retryCount++
	cm.lastRetryTime = time.Now()

	if cm.state == StateConnected {
		cm.state = StateRetrying
	}

	if cm.retryCount >= cm.maxRetries {
		cm.state = StateDead
		cm.enterCoolDown()
		return false, 0
	}

	backoff = cm.calculateBackoff(cm.retryCount)
	return true, backoff
}

func (cm *ConnectionManager) calculateBackoff(attempt int) time.Duration {
	backoff := cm.baseDelay * time.Duration(math.Pow(cm.backoffFactor, float64(attempt)))
	if cm.maxDelay > 0 && backoff > cm.maxDelay {
		backoff = cm.maxDelay
	}

	jitter := time.Duration(rand.Intn(50)+1) * time.Millisecond
	return backoff + jitter
}

func (cm *ConnectionManager) enterCoolDown() {
	cm.coolDownAttempts++

	if cm.coolDownAttempts >= 5 {
		cm.coolDownDuration = 1 * time.Hour
	} else {
		cm.coolDownDuration = cm.coolDownBase * time.Duration(1<<(cm.coolDownAttempts-1))
	}

	cm.coolDownUntil = time.Now().Add(cm.coolDownDuration)

	zap.L().Error("[ConnMgr] Entering coolDown",
		zap.String("driver", cm.driverName),
		zap.Int("retryCount", cm.retryCount),
		zap.Int("maxRetries", cm.maxRetries),
		zap.Duration("coolDownDuration", cm.coolDownDuration),
		zap.Int("coolDownAttempts", cm.coolDownAttempts),
	)
}

func (cm *ConnectionManager) CanRetry() (canRetry bool, waitTime time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	switch cm.state {
	case StateDisconnected:
		if !tryAcquireGlobalReconnectSlot() {
			return true, 1 * time.Second
		}
		return true, 0
	case StateConnecting:
		if cm.retryCount > 0 {
			if !tryAcquireGlobalReconnectSlot() {
				return true, 1 * time.Second
			}
			return true, ensureConnectingMinBackoff(cm.state, cm.calculateBackoff(cm.retryCount))
		}
		if !tryAcquireGlobalReconnectSlot() {
			return true, 1 * time.Second
		}
		return true, ensureConnectingMinBackoff(cm.state, 0)
	case StateConnected:
		return false, 0
	case StateRetrying:
		if cm.retryCount >= cm.maxRetries {
			cm.state = StateDead
			cm.enterCoolDown()
			remaining := cm.coolDownUntil.Sub(time.Now())
			return true, remaining
		}
		if !tryAcquireGlobalReconnectSlot() {
			return true, 1 * time.Second
		}
		return true, cm.calculateBackoff(cm.retryCount)
	case StateDead:
		remaining := cm.coolDownUntil.Sub(time.Now())
		if remaining <= 0 {
			cm.state = StateRetrying
			cm.retryCount = 0 // 重置重试计数，给新一轮连接尝试机会
			if !tryAcquireGlobalReconnectSlot() {
				return true, 1 * time.Second
			}
			return true, 0
		}
		return true, remaining
	default:
		return false, 0
	}
}

func (cm *ConnectionManager) AttemptHalfOpen(success bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if success {
		cm.state = StateConnected
		cm.retryCount = 0
		cm.coolDownAttempts = 0
		cm.coolDownDuration = 1 * time.Minute
		cm.lastSuccessTime = time.Now()

		zap.L().Info("[ConnMgr] Half-Open probe succeeded",
			zap.String("driver", cm.driverName),
			zap.Int("coolDownAttempts", cm.coolDownAttempts),
		)
	} else {
		cm.enterCoolDown()

		zap.L().Warn("[ConnMgr] Half-Open probe failed",
			zap.String("driver", cm.driverName),
			zap.Int("coolDownAttempts", cm.coolDownAttempts),
			zap.Duration("coolDownDuration", cm.coolDownDuration),
		)
	}
}

func (cm *ConnectionManager) GetStatus() (state ConnState, retryCount int, maxRetries int, coolDownRemaining time.Duration, lastSuccess time.Time) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	remaining := cm.coolDownUntil.Sub(time.Now())
	if remaining < 0 {
		remaining = 0
	}

	return cm.state, cm.retryCount, cm.maxRetries, remaining, cm.lastSuccessTime
}

func (cm *ConnectionManager) Close() {
	cm.StopBackgroundLoop()
	if cm.dailyResetTimer != nil {
		cm.dailyResetTimer.Stop()
	}
}

// StartBackgroundLoop runs fn in a single managed goroutine until StopBackgroundLoop
// is called or the parent context passed to fn is cancelled via StopBackgroundLoop.
// Only one background loop may run per ConnectionManager.
func (cm *ConnectionManager) StartBackgroundLoop(fn func(context.Context)) {
	cm.bgMu.Lock()
	defer cm.bgMu.Unlock()
	cm.stopBackgroundLoopLocked()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	cm.bgCancel = cancel
	cm.bgDone = done

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("[ConnectionManager] background goroutine panic recovered",
					zap.Any("panic", r),
					zap.Stack("stack"))
			}
		}()
		fn(ctx)
	}()
}

// StopBackgroundLoop cancels and waits for the managed background goroutine to exit.
func (cm *ConnectionManager) StopBackgroundLoop() {
	cm.bgMu.Lock()
	defer cm.bgMu.Unlock()
	cm.stopBackgroundLoopLocked()
}

func (cm *ConnectionManager) stopBackgroundLoopLocked() {
	if cm.bgCancel == nil {
		return
	}
	cm.bgCancel()
	<-cm.bgDone
	cm.bgCancel = nil
	cm.bgDone = nil
}

func (cm *ConnectionManager) SetMaxRetries(max int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.maxRetries = max
}

func (cm *ConnectionManager) SetMaxFailCount(max int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.maxFailCount = max
}

func (cm *ConnectionManager) SetBackoffParams(base, max time.Duration, factor float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.baseDelay = base
	cm.maxDelay = max
	cm.backoffFactor = factor
}

// EnsureConnected is the single entry point for synchronous reconnect with
// backoff, cooldown, and global rate limiting.
func (cm *ConnectionManager) EnsureConnected(ctx context.Context, connect ConnectFunc) error {
	var lastErr error

	for {
		canRetry, waitTime := cm.CanRetry()
		if !canRetry {
			if lastErr != nil {
				return lastErr
			}
			return fmt.Errorf("[%s] connection not allowed to retry", cm.driverName)
		}

		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}

		cm.SetState(StateConnecting)

		err := cm.runConnect(ctx, connect)
		if err == nil {
			cm.RecordSuccess()
			return nil
		}

		lastErr = err
		shouldRetry, _ := cm.RecordFailure()
		if !shouldRetry {
			// CoolDown: only park inside EnsureConnected when the caller gave a
			// deadline (ScheduleReconnect). Background/unlimited ctx must return
			// immediately — otherwise offline config errors hang forever.
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				return lastErr
			}
			continue
		}
	}
}

// SetLinkMutex binds the shared-link channelMu so dial/install is serialized with I/O.
func (cm *ConnectionManager) SetLinkMutex(mu *sync.Mutex) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.linkMu = mu
}

func (cm *ConnectionManager) runConnect(ctx context.Context, connect ConnectFunc) error {
	cm.mu.Lock()
	linkMu := cm.linkMu
	cm.mu.Unlock()
	if linkMu != nil {
		linkMu.Lock()
		defer linkMu.Unlock()
	}
	return connect(ctx)
}

// ScheduleReconnect starts an asynchronous reconnect guarded by single-flight.
// Duplicate calls while a reconnect is in progress are ignored.
func (cm *ConnectionManager) ScheduleReconnect(parentCtx context.Context, timeout time.Duration, connect ConnectFunc) {
	if !cm.reconnectRunning.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer cm.reconnectRunning.Store(false)

		ctx := parentCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parentCtx, timeout)
			defer cancel()
		}

		if err := cm.EnsureConnected(ctx, connect); err != nil {
			zap.L().Error("[ConnMgr] Reconnection failed",
				zap.String("driver", cm.driverName),
				zap.Error(err),
			)
		}
	}()
}

// ScheduleAsyncTask runs a one-shot async operation under the same single-flight
// guard as ScheduleReconnect, without requiring a disconnected connection state.
// Use for device-level recovery probes while the channel remains connected.
func (cm *ConnectionManager) ScheduleAsyncTask(parentCtx context.Context, timeout time.Duration, task func(context.Context) error) {
	if !cm.reconnectRunning.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer cm.reconnectRunning.Store(false)

		ctx := parentCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parentCtx, timeout)
			defer cancel()
		}

		if err := task(ctx); err != nil {
			zap.L().Warn("[ConnMgr] Async task failed",
				zap.String("driver", cm.driverName),
				zap.Error(err),
			)
		}
	}()
}
