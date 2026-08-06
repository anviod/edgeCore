package s7

import (
	"github.com/anviod/edgeCore/internal/driver"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestScenario_NormalLifecycle 测试正常生命周期：连接→成功→断开
func TestScenario_NormalLifecycle(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	// 1. 初始状态
	assert.Equal(t, StateDisconnected, cm.GetState())

	// 2. 进入连接状态
	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	// 3. 连接成功
	cm.RecordSuccess()
	state, retryCount, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateConnected, state)
	assert.Equal(t, 0, retryCount)

	// 4. 正常断开
	cm.SetState(StateDisconnected)
	assert.Equal(t, StateDisconnected, cm.GetState())
}

// TestScenario_ConsecutiveFailuresTriggerReconnect 测试连续失败触发重连
func TestScenario_ConsecutiveFailuresTriggerReconnect(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())

	// 第一次失败
	shouldRetry, backoff1 := cm.RecordFailure()
	assert.True(t, shouldRetry)
	assert.Equal(t, StateRetrying, cm.GetState())
	assert.Greater(t, backoff1, time.Duration(0))

	// 第二次失败（更长的退避时间）
	_, backoff2 := cm.RecordFailure()
	assert.GreaterOrEqual(t, backoff2, backoff1)
}

// TestScenario_MaxFailuresEnterDead 测试达到最大失败次数进入Dead状态
func TestScenario_MaxFailuresEnterDead(t *testing.T) {
	cm := driver.NewConnectionManager("s7-200smart")
	defer cm.Close()

	cm.SetMaxRetries(8)
	cm.RecordSuccess()

	for i := 0; i < 8; i++ {
		cm.RecordFailure()
	}

	assert.Equal(t, StateDead, cm.GetState())
	_, _, _, coolDown, _ := cm.GetStatus()
	assert.Greater(t, coolDown, time.Duration(0))
}

// TestScenario_ExponentialBackoffGrowth 测试指数退避增长
func TestScenario_ExponentialBackoffGrowth(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()
	cm.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	cm.SetMaxRetries(20)

	cm.RecordSuccess()

	var backoffs []time.Duration
	for i := 0; i < 5; i++ {
		_, backoff := cm.RecordFailure()
		backoffs = append(backoffs, backoff)
	}

	// 验证退避时间呈指数增长
	for i := 1; i < len(backoffs); i++ {
		assert.GreaterOrEqual(t, backoffs[i], backoffs[i-1],
			"backoff should not decrease: backoffs[%d]=%v should be >= backoffs[%d]=%v",
			i, backoffs[i], i-1, backoffs[i-1])
	}

	// 验证包含抖动（实际退避时间 > 理论值）
	// 基础值为100ms，第1次重试 base*2^1=200ms
	assert.GreaterOrEqual(t, backoffs[0], 200*time.Millisecond)
}

// TestScenario_CoolDownProgression 测试冷却期递增
func TestScenario_CoolDownProgression(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.RecordSuccess()

	// 触发多次冷却
	for cycle := 0; cycle < 3; cycle++ {
		cm.RecordFailure()
		cm.RecordFailure()
		// 重新连接
		cm.RecordSuccess()
	}

	// 验证状态恢复
	assert.Equal(t, StateConnected, cm.GetState())
}

// TestScenario_ConcurrentSafety 测试并发安全
func TestScenario_ConcurrentSafety(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	var wg sync.WaitGroup
	var ops int32

	// 并发执行成功和失败记录
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cm.RecordSuccess()
			atomic.AddInt32(&ops, 1)
		}()
		go func() {
			defer wg.Done()
			cm.RecordFailure()
			atomic.AddInt32(&ops, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(200), atomic.LoadInt32(&ops))
}

// TestScenario_DailyReset 测试每日重置
func TestScenario_DailyReset(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	// 累积重试和冷却
	for i := 0; i < 5; i++ {
		cm.RecordFailure()
	}

	_, retry1, _, _, _ := cm.GetStatus()
	assert.Equal(t, 5, retry1)

	// 手动触发每日重置
	cm.ResetDaily()

	// retryCount 应被重置为 0
	_, retry2, _, coolDown2, _ := cm.GetStatus()
	assert.Equal(t, 0, retry2)
	assert.Equal(t, time.Duration(0), coolDown2)
}

// TestScenario_HalfOpenProbe 测试半开探测
func TestScenario_HalfOpenProbe(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()
	cm.SetMaxRetries(3)

	cm.RecordSuccess()
	// 进入Dead状态
	for i := 0; i < 3; i++ {
		cm.RecordFailure()
	}
	assert.Equal(t, StateDead, cm.GetState())

	// 模拟冷却期结束

	// 半开探测成功
	cm.AttemptHalfOpen(true)
	assert.Equal(t, StateConnected, cm.GetState())
	_, retry, _, _, _ := cm.GetStatus()
	assert.Equal(t, 0, retry)
}

// TestScenario_PLC200SmartLimits 测试S7-200Smart的特殊限制
func TestScenario_PLC200SmartLimits(t *testing.T) {
	cm := driver.NewConnectionManager("s7-200smart")
	defer cm.Close()

	cm.SetMaxRetries(8)
	cm.SetMaxFailCount(3)

	cm.RecordSuccess()
	for i := 0; i < 7; i++ {
		shouldRetry, _ := cm.RecordFailure()
		assert.True(t, shouldRetry, "retry %d should allow retry", i+1)
	}

	// 第8次失败应进入Dead
	shouldRetry, _ := cm.RecordFailure()
	assert.False(t, shouldRetry)
	assert.Equal(t, StateDead, cm.GetState())
}

// TestScenario_PLC1500DefaultConfig 测试S7-1500默认配置
func TestScenario_PLC1500DefaultConfig(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1500")
	defer cm.Close()

	// S7-1500：max_retries=64, max_fail_count=5
}

// TestScenario_MaxBackoffCap 测试最大退避时间封顶
func TestScenario_MaxBackoffCap(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()
	cm.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	cm.SetMaxRetries(20)

	cm.RecordSuccess()

	// 大量失败，确保退避不超过maxDelay
	for i := 0; i < 15; i++ {
		_, backoff := cm.RecordFailure()
		// 实际退避 = 基础退避 + 抖动(0-49ms)
		assert.LessOrEqual(t, backoff, 30*time.Second+50*time.Millisecond,
			"backoff should be capped at maxDelay + jitter")
	}
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	cm := driver.NewConnectionManager("s7")
	defer cm.Close()
	cm.SetMaxRetries(5)
	cm.RecordSuccess()
	cm.RecordFailure()
	assert.NotEqual(t, StateDead, cm.GetState(), "single failure must not enter dead state immediately")
}
