package bacnet

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
)

// TestScenario_CalculateBackoff 测试指数退避计算
func TestScenario_CalculateBackoff(t *testing.T) {
	d := &BACnetDriver{}

	testCases := []struct {
		name     string
		attempts int
		expected time.Duration
	}{
		{"first", 1, 1 * time.Minute},
		{"second", 2, 2 * time.Minute},
		{"third", 3, 4 * time.Minute},
		{"fourth", 4, 8 * time.Minute},
		{"fifth", 5, 16 * time.Minute},
		{"sixth", 6, 32 * time.Minute},
		{"seventh_capped", 7, 1 * time.Hour},
		{"tenth_capped", 10, 1 * time.Hour},
		{"zero", 0, 1 * time.Minute},
		{"negative", -1, 1 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backoff := d.calculateBackoff(tc.attempts)
			assert.Equal(t, tc.expected, backoff)
		})
	}
}

// TestScenario_CheckDailyReset 测试每日重置
func TestScenario_CheckDailyReset(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		IsolationCount: 5,
	}

	// 第一次调用会初始化lastReset
	d.checkDailyReset(devCtx)
	assert.False(t, devCtx.lastReset.IsZero())
	assert.Equal(t, 5, devCtx.IsolationCount)

	// 模拟刚过24小时
	devCtx.lastReset = time.Now().Add(-25 * time.Hour)
	d.checkDailyReset(devCtx)
	assert.Equal(t, 0, devCtx.IsolationCount, "isolation count should be reset after 24h")
}

// TestScenario_CheckDailyResetNoOpWhenRecent 测试24小时内不重置
func TestScenario_CheckDailyResetNoOpWhenRecent(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		IsolationCount: 3,
		lastReset:      time.Now(),
	}

	// 23小时内不重置
	devCtx.lastReset = time.Now().Add(-23 * time.Hour)
	d.checkDailyReset(devCtx)
	assert.Equal(t, 3, devCtx.IsolationCount, "should not reset within 24h")
}

// TestScenario_HandleReadFailureIsolation 测试失败触发隔离
func TestScenario_HandleReadFailureIsolation(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		State:               DeviceStateOnline,
		ConsecutiveFailures: 0,
		IsolationCount:      0,
		LastValues:          make(map[string]model.Value),
	}

	// 3次失败触发隔离
	d.handleReadFailure(devCtx, 100, assert.AnError)
	d.handleReadFailure(devCtx, 100, assert.AnError)
	d.handleReadFailure(devCtx, 100, assert.AnError)

	assert.Equal(t, DeviceStateIsolated, devCtx.State)
	assert.Equal(t, 1, devCtx.IsolationCount)
	assert.True(t, devCtx.IsolationUntil.After(time.Now()))
}

// TestScenario_HandleReadFailureNoDoubleIsolation 测试重复触发不重复隔离
func TestScenario_HandleReadFailureNoDoubleIsolation(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		State:               DeviceStateIsolated,
		ConsecutiveFailures: 3,
		IsolationCount:      1,
		IsolationUntil:      time.Now().Add(1 * time.Minute),
		LastValues:          make(map[string]model.Value),
	}

	// 已经在隔离状态
	d.handleReadFailure(devCtx, 100, assert.AnError)
	assert.Equal(t, 1, devCtx.IsolationCount, "should not increment when already isolated")
}

// TestScenario_BackoffWithJitter 测试退避+抖动
func TestScenario_BackoffWithJitter(t *testing.T) {
	d := &BACnetDriver{}

	// 基础退避1分钟，抖动0-5秒
	baseBackoff := d.calculateBackoff(1)
	// jitter: rand.Intn(5000) ms

	// 由于是随机的，最小值 >= 1min，最大值 <= 1min+5s
	assert.GreaterOrEqual(t, baseBackoff, 1*time.Minute)

	// 模拟10次，确认抖动范围
	for i := 0; i < 10; i++ {
		b := d.calculateBackoff(1)
		assert.LessOrEqual(t, b, 1*time.Minute+5*time.Second)
	}
}

// TestScenario_CoolDownProgression 测试冷却期递增
func TestScenario_CoolDownProgression(t *testing.T) {
	d := &BACnetDriver{}

	// 第1次：1分钟
	backoff1 := d.calculateBackoff(1)
	assert.Equal(t, 1*time.Minute, backoff1)

	// 第2次：2分钟
	backoff2 := d.calculateBackoff(2)
	assert.Equal(t, 2*time.Minute, backoff2)

	// 第3次：4分钟
	backoff3 := d.calculateBackoff(3)
	assert.Equal(t, 4*time.Minute, backoff3)
}

// TestScenario_MaxBackoffCap 测试最大冷却时间封顶
func TestScenario_MaxBackoffCap(t *testing.T) {
	d := &BACnetDriver{}

	// 第7次及以上封顶为1小时
	for attempts := 7; attempts <= 20; attempts++ {
		backoff := d.calculateBackoff(attempts)
		assert.LessOrEqual(t, backoff, 1*time.Hour+5*time.Second,
			"backoff for %d attempts should be capped", attempts)
	}
}

// TestScenario_DeviceStateTransitions 测试设备状态转换
func TestScenario_DeviceStateTransitions(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		State:               DeviceStateOnline,
		ConsecutiveFailures: 0,
		IsolationCount:      0,
		LastValues:          make(map[string]model.Value),
	}

	// Online → Isolated (3次失败)
	d.handleReadFailure(devCtx, 100, assert.AnError)
	d.handleReadFailure(devCtx, 100, assert.AnError)
	d.handleReadFailure(devCtx, 100, assert.AnError)
	assert.Equal(t, DeviceStateIsolated, devCtx.State)
}

// TestScenario_DailyResetAfterLongIdle 测试长期空闲后的重置
func TestScenario_DailyResetAfterLongIdle(t *testing.T) {
	d := &BACnetDriver{}
	devCtx := &DeviceContext{
		IsolationCount: 5,
		lastReset:      time.Now().Add(-100 * time.Hour),
	}

	d.checkDailyReset(devCtx)
	assert.Equal(t, 0, devCtx.IsolationCount)
}

func TestScenario_ConcurrentAccess(t *testing.T) {
	d := &BACnetDriver{}
	var wg sync.WaitGroup
	var ops int32

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.mu.RLock()
			_ = len(d.deviceContexts)
			d.mu.RUnlock()
			atomic.AddInt32(&ops, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(30), atomic.LoadInt32(&ops))
}
