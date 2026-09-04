package opcua

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/stretchr/testify/assert"
)

func TestScenario_ConnectionManagerNormalLifecycle(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
	defer cm.Close()

	assert.Equal(t, StateDisconnected, cm.GetState())

	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	cm.RecordSuccess()
	state, _, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateConnected, state)
}

func TestScenario_ExponentialBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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
}

func TestScenario_CoolDownProgression(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_DailyReset(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_HalfOpenProbe(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_ConcurrentAccess(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_StateTransitions(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
	defer cm.Close()

	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())

	cm.RecordFailure()
	assert.Equal(t, StateRetrying, cm.GetState())

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())
}

func TestScenario_CanRetryDuringCooldown(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_BackoffUpperBound(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
	defer cm.Close()
	cm.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	cm.SetMaxRetries(20)

	cm.RecordSuccess()

	for i := 0; i < 15; i++ {
		_, backoff := cm.RecordFailure()
		assert.LessOrEqual(t, backoff, 30*time.Second+50*time.Millisecond)
	}
}

func TestScenario_RecoveryAfterCoolDown(t *testing.T) {
	t.Skip("skips 2-minute Dead cooldown; covered by driver.TestConnectionManager_CanRetryAllStates/DeadAfterCoolDownExpires")
	cm := driver.NewConnectionManager("opcua")
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

func TestScenario_SuccessResetsAllCounters(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
	defer cm.Close()
	cm.SetMaxRetries(10)

	cm.RecordSuccess()
	for i := 0; i < 5; i++ {
		cm.RecordFailure()
	}

	cm.RecordSuccess()

	_, retry, _, coolDown, _ := cm.GetStatus()
	assert.Equal(t, 0, retry)
	assert.Equal(t, time.Duration(0), coolDown)
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	cm := driver.NewConnectionManager("opcua")
	defer cm.Close()
	cm.RecordSuccess()
	cm.RecordFailure()
	assert.NotEqual(t, StateDead, cm.GetState(), "single failure must not enter dead state immediately")
}
