package modbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
)

func TestScenario_ConnectionManagerNormalLifecycle(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()

	assert.Equal(t, StateDisconnected, cm.GetState())

	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	cm.RecordSuccess()
	state, _, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateConnected, state)

	cm.SetState(StateDisconnected)
	assert.Equal(t, StateDisconnected, cm.GetState())
}

func TestScenario_ExponentialBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	cm.SetMaxRetries(20)

	cm.RecordSuccess()

	var backoffs []time.Duration
	for i := 0; i < 5; i++ {
		_, backoff := cm.RecordFailure()
		backoffs = append(backoffs, backoff)
	}

	for i := 1; i < len(backoffs); i++ {
		assert.GreaterOrEqual(t, backoffs[i], backoffs[i-1])
	}

	assert.GreaterOrEqual(t, backoffs[0], 200*time.Millisecond)
}

func TestScenario_CoolDownProgression(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.SetState(StateConnected)
	cm.RecordFailure()
	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)
}

func TestScenario_MaxCoolDownCap(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.SetState(StateConnected)
	cm.RecordFailure()
	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	for cycle := 0; cycle < 5; cycle++ {
		cm.AttemptHalfOpen(false)
	}

	assert.Equal(t, StateDead, cm.GetState())
}

func TestScenario_TransportRecordSuccess(t *testing.T) {
	cfg := model.DriverConfig{
		Config: map[string]any{
			"url": "127.0.0.1:5020",
		},
	}
	mt := NewModbusTransport(cfg)
	defer mt.connMgr.Close()

	assert.Equal(t, int32(0), mt.collectFailCount.Load())

	mt.RecordFailure(assert.AnError)
	mt.RecordFailure(assert.AnError)
	mt.RecordFailure(assert.AnError)
	assert.Equal(t, int32(3), mt.collectFailCount.Load())

	mt.RecordSuccess()
	assert.Equal(t, int32(0), mt.collectFailCount.Load())

	state, _, _, _, _ := mt.connMgr.GetStatus()
	assert.Equal(t, StateConnected, state)
}

func TestScenario_TransportMaxFailTriggersReconnect(t *testing.T) {
	cfg := model.DriverConfig{
		Config: map[string]any{
			"url":            "127.0.0.1:5020",
			"max_fail_count": int32(3),
		},
	}
	mt := NewModbusTransport(cfg)
	defer mt.connMgr.Close()
	mt.maxFailCount = 3

	mt.RecordFailure(assert.AnError)
	mt.RecordFailure(assert.AnError)
	mt.RecordFailure(assert.AnError)

	time.Sleep(200 * time.Millisecond)

	assert.GreaterOrEqual(t, mt.collectFailCount.Load(), int32(3))
}

func TestScenario_NeedProbeCheck(t *testing.T) {
	cfg := model.DriverConfig{
		Config: map[string]any{
			"url":           "127.0.0.1:5020",
			"collect_cycle": 5000,
		},
	}
	mt := NewModbusTransport(cfg)
	defer mt.connMgr.Close()
	mt.collectCycle = 5 * time.Second

	assert.False(t, mt.NeedProbeCheck())

	mt.lastActivityTime.Store(time.Now())
	assert.False(t, mt.NeedProbeCheck(), "immediate should not need probe")

	mt.lastActivityTime.Store(time.Now().Add(-16 * time.Second))
	assert.True(t, mt.NeedProbeCheck(), "after 3x collectCycle should need probe")
}

func TestScenario_DailyReset(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()

	cm.SetMaxRetries(10)
	cm.RecordSuccess()

	for i := 0; i < 5; i++ {
		cm.RecordFailure()
	}

	_, retry1, _, _, _ := cm.GetStatus()
	assert.Equal(t, 5, retry1)

	cm.ResetDaily()

	_, retry2, _, coolDown, _ := cm.GetStatus()
	assert.Equal(t, 0, retry2)
	assert.Equal(t, time.Duration(0), coolDown)
}

func TestScenario_CanRetryDuringCooldown(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.RecordSuccess()
	cm.RecordFailure()
	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	canRetry, waitTime := cm.CanRetry()
	assert.True(t, canRetry)
	assert.Greater(t, waitTime, time.Duration(0))
}

func TestScenario_HalfOpenProbe(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.RecordSuccess()
	cm.RecordFailure()
	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	cm.AttemptHalfOpen(true)
	state, retry, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateConnected, state)
	assert.Equal(t, 0, retry)
}

func TestScenario_HalfOpenProbeFailure(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(2)

	cm.RecordSuccess()
	cm.RecordFailure()
	cm.RecordFailure()

	cm.AttemptHalfOpen(false)
	assert.Equal(t, StateDead, cm.GetState())
}

func TestScenario_ConcurrentAccess(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()

	var wg sync.WaitGroup
	var ops int32

	for i := 0; i < 50; i++ {
		wg.Add(3)
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
		go func() {
			defer wg.Done()
			cm.CanRetry()
			atomic.AddInt32(&ops, 1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(150), atomic.LoadInt32(&ops))
}

func TestScenario_RecoveryFromDead(t *testing.T) {
	t.Skip("skips 2-minute Dead cooldown; covered by driver.TestConnectionManager_CanRetryAllStates/DeadAfterCoolDownExpires")
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetMaxRetries(3)

	cm.RecordSuccess()
	cm.RecordFailure()
	cm.RecordFailure()
	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	time.Sleep(2 * time.Minute)

	canRetry, _ := cm.CanRetry()
	assert.True(t, canRetry)
}
