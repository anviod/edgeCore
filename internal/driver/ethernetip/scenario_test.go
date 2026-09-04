package ethernetip

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/stretchr/testify/assert"
)

func TestScenario_ConnectionManagerNormalLifecycle(t *testing.T) {
	cm := driver.NewConnectionManager("ethernetip")
	defer cm.Close()

	assert.Equal(t, StateDisconnected, cm.GetState())

	cm.SetState(StateConnecting)
	assert.Equal(t, StateConnecting, cm.GetState())

	cm.RecordSuccess()
	state, _, _, _, _ := cm.GetStatus()
	assert.Equal(t, StateConnected, state)
}

func TestScenario_ExponentialBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("ethernetip")
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
	cm := driver.NewConnectionManager("ethernetip")
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
	cm := driver.NewConnectionManager("ethernetip")
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
	cfg := map[string]any{
		"ip":   "127.0.0.1",
		"port": 44818,
	}
	transport := NewENIPTransport(cfg)
	defer transport.connMgr.Close()

	assert.Equal(t, int32(0), transport.collectFailCount.Load())

	transport.RecordFailure(assert.AnError)
	transport.RecordFailure(assert.AnError)
	assert.Equal(t, int32(2), transport.collectFailCount.Load())

	transport.RecordSuccess()
	assert.Equal(t, int32(0), transport.collectFailCount.Load())

	state, _, _, _, _ := transport.connMgr.GetStatus()
	assert.Equal(t, StateConnected, state)
}

func TestScenario_MaxFailTriggersReconnect(t *testing.T) {
	cfg := map[string]any{
		"ip":             "127.0.0.1",
		"port":           44818,
		"max_fail_count": 3,
	}
	transport := NewENIPTransport(cfg)
	defer transport.connMgr.Close()
	transport.maxFailCount = 3

	transport.connected.Store(true)
	transport.tcp = nil

	transport.RecordFailure(assert.AnError)
	transport.RecordFailure(assert.AnError)
	transport.RecordFailure(assert.AnError)

	time.Sleep(100 * time.Millisecond)

	assert.GreaterOrEqual(t, transport.collectFailCount.Load(), int32(3))
}

func TestScenario_NeedProbeCheck(t *testing.T) {
	cfg := map[string]any{
		"ip":            "127.0.0.1",
		"port":          44818,
		"collect_cycle": 5000,
	}
	transport := NewENIPTransport(cfg)
	defer transport.connMgr.Close()
	transport.collectCycle = 5 * time.Second

	transport.lastActivityTime.Store(time.Time{})
	assert.False(t, transport.NeedProbeCheck())

	transport.lastActivityTime.Store(time.Now())
	assert.False(t, transport.NeedProbeCheck())

	transport.lastActivityTime.Store(time.Now().Add(-16 * time.Second))
	assert.True(t, transport.NeedProbeCheck())
}

func TestScenario_DailyReset(t *testing.T) {
	cm := driver.NewConnectionManager("ethernetip")
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
	cm := driver.NewConnectionManager("ethernetip")
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
	cm := driver.NewConnectionManager("ethernetip")
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
	cm := driver.NewConnectionManager("ethernetip")
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

func TestScenario_MaxRetries64(t *testing.T) {
	cm := driver.NewConnectionManager("ethernetip")
	defer cm.Close()

	cm.SetMaxRetries(64)
	cm.RecordSuccess()
	for i := 0; i < 63; i++ {
		shouldRetry, _ := cm.RecordFailure()
		assert.True(t, shouldRetry, "retry %d should allow", i+1)
		assert.Equal(t, StateRetrying, cm.GetState())
	}

	shouldRetry, _ := cm.RecordFailure()
	assert.False(t, shouldRetry)
	assert.Equal(t, StateDead, cm.GetState())
}

func TestScenario_BackoffUpperBound(t *testing.T) {
	cm := driver.NewConnectionManager("ethernetip")
	defer cm.Close()
	cm.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	cm.SetMaxRetries(20)

	cm.RecordSuccess()

	for i := 0; i < 15; i++ {
		_, backoff := cm.RecordFailure()
		assert.LessOrEqual(t, backoff, 30*time.Second+50*time.Millisecond)
	}
}

func TestScenario_TransportDisconnected(t *testing.T) {
	cfg := map[string]any{
		"ip":   "127.0.0.1",
		"port": 44818,
	}
	transport := NewENIPTransport(cfg)
	defer transport.connMgr.Close()

	transport.connected.Store(true)
	transport.connMgr.SetState(StateConnected)

	err := transport.Disconnect()
	assert.NoError(t, err)
	assert.False(t, transport.IsConnected())
	assert.Equal(t, StateDisconnected, transport.connMgr.GetState())
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	cfg := map[string]any{
		"ip":   "127.0.0.1",
		"port": 44818,
	}
	transport := NewENIPTransport(cfg)
	defer transport.connMgr.Close()
	transport.connected.Store(true)
	transport.connMgr.SetState(StateConnected)

	transport.RecordFailure(fmt.Errorf("i/o timeout"))
	assert.True(t, transport.connected.Load(), "device-level timeout must not disconnect shared transport")
}
