package s7

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/stretchr/testify/assert"
)

func TestNewConnectionManager(t *testing.T) {
	tests := []struct {
		name         string
		driverName   string
		maxRetries   int
		maxFailCount int
	}{
		{"S7-200Smart", "s7-200smart", 8, 3},
		{"S7-1200", "s7-1200", 64, 5},
		{"S7-1500", "s7-1500", 64, 5},
		{"Unknown", "", 64, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := driver.NewConnectionManager(tt.driverName)
			cm.SetMaxRetries(tt.maxRetries)
			cm.SetMaxFailCount(tt.maxFailCount)
			defer cm.Close()

			assert.Equal(t, StateDisconnected, cm.GetState())
			state, retryCount, maxRetries, _, _ := cm.GetStatus()
			assert.Equal(t, StateDisconnected, state)
			assert.Equal(t, 0, retryCount)
			assert.Equal(t, tt.maxRetries, maxRetries)
		})
	}
}

func TestStateTransitions(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	assert.Equal(t, StateDisconnected, cm.GetState())

	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())

	shouldRetry, _ := cm.RecordFailure()
	assert.True(t, shouldRetry)
	assert.Equal(t, StateRetrying, cm.GetState())

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())
}

func TestRecordFailure_EntersDead(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(2)

	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())

	shouldRetry, _ := cm.RecordFailure()
	assert.True(t, shouldRetry)
	assert.Equal(t, StateRetrying, cm.GetState())

	shouldRetry, _ = cm.RecordFailure()
	assert.False(t, shouldRetry)
	assert.Equal(t, StateDead, cm.GetState())

	state, retryCount, maxRetries, coolDownRemaining, _ := cm.GetStatus()
	assert.Equal(t, StateDead, state)
	assert.Equal(t, 2, retryCount)
	assert.Equal(t, 2, maxRetries)
	assert.True(t, coolDownRemaining > 0)
}

func TestCanRetry(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(3)

	canRetry, wait := cm.CanRetry()
	assert.True(t, canRetry)
	assert.Equal(t, 0*time.Millisecond, wait)

	cm.SetState(StateConnecting)
	canRetry, wait = cm.CanRetry()
	assert.True(t, canRetry)
	assert.Equal(t, 200*time.Millisecond, wait)

	cm.RecordSuccess()
	canRetry, wait = cm.CanRetry()
	assert.False(t, canRetry)
	assert.Equal(t, 0*time.Millisecond, wait)

	cm.RecordFailure()
	canRetry, wait = cm.CanRetry()
	assert.True(t, canRetry)
	assert.True(t, wait > 0)

	cm.RecordFailure()
	cm.RecordFailure()

	canRetry, wait = cm.CanRetry()
	assert.True(t, canRetry)
	assert.True(t, wait > 0)
	assert.Equal(t, StateDead, cm.GetState())
}

func TestAttemptHalfOpen(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(1)
	cm.RecordSuccess()
	cm.RecordFailure()

	assert.Equal(t, StateDead, cm.GetState())

	cm.AttemptHalfOpen(true)
	assert.Equal(t, StateConnected, cm.GetState())

	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	cm.AttemptHalfOpen(false)
	assert.Equal(t, StateDead, cm.GetState())
}

func TestExponentialBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetBackoffParams(100*time.Millisecond, 1000*time.Millisecond, 2.0)

	cm.RecordSuccess()
	cm.RecordFailure()
	_, backoff1 := cm.CanRetry()
	assert.True(t, backoff1 >= 200*time.Millisecond && backoff1 <= 250*time.Millisecond)

	cm.RecordFailure()
	_, backoff2 := cm.CanRetry()
	assert.True(t, backoff2 >= 400*time.Millisecond && backoff2 <= 450*time.Millisecond)
}

func TestCoolDownCycle(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(1)
	cm.RecordSuccess()

	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)

	cm.AttemptHalfOpen(false)
}

func TestResetDaily(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(2)
	cm.RecordSuccess()

	cm.RecordFailure()
	cm.RecordFailure()

	cm.ResetDaily()

	state, retryCount, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateDead, state)
	assert.Equal(t, 0, retryCount)
}

func TestBackoffEdgeCases(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetBackoffParams(0, 100*time.Millisecond, 2.0)
	cm.RecordSuccess()
	cm.RecordFailure()
	_, backoff := cm.CanRetry()
	assert.True(t, backoff > 0)

	cm.SetBackoffParams(100*time.Millisecond, 0, 2.0)
	cm.RecordSuccess()
	cm.RecordFailure()
	_, backoff = cm.CanRetry()
	assert.True(t, backoff > 0)
}

func TestStateEdgeCases(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetState(StateConnected)
	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())

	cm.SetState(StateDead)
	cm.RecordSuccess()
	assert.Equal(t, StateConnected, cm.GetState())
}

func TestCanRetry_DeadStateWithCoolDown(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(1)
	cm.RecordSuccess()

	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	canRetry, wait := cm.CanRetry()
	assert.True(t, canRetry)
	assert.True(t, wait > 0)
}

func TestGetStatus(t *testing.T) {
	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(10)
	cm.RecordSuccess()

	state, retryCount, maxRetries, coolDownRemaining, lastSuccess := cm.GetStatus()
	assert.Equal(t, StateConnected, state)
	assert.Equal(t, 0, retryCount)
	assert.Equal(t, 10, maxRetries)
	assert.Equal(t, 0*time.Millisecond, coolDownRemaining)
	assert.True(t, lastSuccess.After(time.Now().Add(-1*time.Second)))
}

func TestCanRetry_CoolDownExpired(t *testing.T) {
	t.Skip("skips 2-minute Dead cooldown sleep; covered by driver.TestConnectionManager_CanRetryAllStates/DeadAfterCoolDownExpires")

	cm := driver.NewConnectionManager("s7-1200")
	defer cm.Close()

	cm.SetMaxRetries(1)
	cm.RecordSuccess()

	cm.RecordFailure()
	assert.Equal(t, StateDead, cm.GetState())

	time.Sleep(2 * time.Minute)

	canRetry, _ := cm.CanRetry()
	assert.True(t, canRetry)
}
