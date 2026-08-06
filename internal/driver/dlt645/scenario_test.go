package dlt645

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_ConnectionManagerBackoff(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{"connectionType": "tcp", "ip": "127.0.0.1"})
	defer transport.connMgr.Close()

	transport.connMgr.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	transport.connMgr.SetMaxRetries(20)
	transport.connMgr.RecordSuccess()

	var backoffs []time.Duration
	for i := 0; i < 5; i++ {
		_, backoff := transport.connMgr.RecordFailure()
		backoffs = append(backoffs, backoff)
	}
	for i := 1; i < len(backoffs); i++ {
		assert.GreaterOrEqual(t, backoffs[i], backoffs[i-1])
	}
}

func TestScenario_MaxFailuresEnterDead(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{"connectionType": "tcp", "ip": "127.0.0.1"})
	defer transport.connMgr.Close()

	transport.connMgr.SetMaxRetries(3)
	transport.connMgr.RecordSuccess()
	for i := 0; i < 3; i++ {
		transport.connMgr.RecordFailure()
	}
	assert.Equal(t, driver.StateDead, transport.connMgr.GetState())
}

func TestScenario_TransportMaxFailTriggersDisconnect(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"max_fail_count": 2,
	})
	transport.maxFailCount = 2
	mock := &mockLink{}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) { return mock, nil }
	require.NoError(t, transport.Connect(context.Background()))
	assert.True(t, transport.IsConnected())

	transport.RecordFailure(assert.AnError)
	transport.RecordFailure(assert.AnError)
	assert.False(t, transport.IsConnected())
}

func TestScenario_HalfOpenProbe(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{"connectionType": "tcp", "ip": "127.0.0.1"})
	defer transport.connMgr.Close()

	transport.connMgr.SetMaxRetries(2)
	transport.connMgr.RecordSuccess()
	transport.connMgr.RecordFailure()
	transport.connMgr.RecordFailure()
	assert.Equal(t, driver.StateDead, transport.connMgr.GetState())

	transport.connMgr.AttemptHalfOpen(true)
	assert.Equal(t, driver.StateConnected, transport.connMgr.GetState())
}

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{"connectionType": "tcp", "ip": "127.0.0.1"})
	mock := &mockLink{}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) { return mock, nil }
	require.NoError(t, transport.Connect(context.Background()))

	scheduler := NewDLT645Scheduler(transport, NewDLT645Decoder())
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "not-a-valid-address", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)

	_, success, failure := scheduler.GetStats()
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(1), failure)
}

type responseQueueLink struct {
	frame    []byte
	pending  []byte
	mu       sync.Mutex
	writeBuf bytes.Buffer
}

func (m *responseQueueLink) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, m.pending)
	m.pending = m.pending[n:]
	return n, nil
}

func (m *responseQueueLink) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = append([]byte(nil), m.frame...)
	return m.writeBuf.Write(p)
}

func (m *responseQueueLink) Close() error { return nil }

func TestScenario_ConcurrentSchedulerStats(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")
	respFrame := buildFrame(addr, CtrlReadResp, encode033(append(di[:], 0x20, 0x02)))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"timeout":        float64(1000),
	})
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return &responseQueueLink{frame: respFrame}, nil
	}
	require.NoError(t, transport.Connect(context.Background()))

	scheduler := NewDLT645Scheduler(transport, NewDLT645Decoder())
	point := model.Point{
		ID: "p1", Address: "210220003011#02-01-01-00", DataType: "UINT16",
	}

	var wg sync.WaitGroup
	var ops int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = scheduler.ReadPoints(context.Background(), []model.Point{point})
			atomic.AddInt32(&ops, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(20), atomic.LoadInt32(&ops))

	total, success, _ := scheduler.GetStats()
	assert.Equal(t, int64(20), total)
	assert.Equal(t, int64(20), success)
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")
	respFrame := buildFrame(addr, CtrlReadResp, encode033(append(di[:], 0x20, 0x02)))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"timeout":        float64(1000),
	})
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return &responseQueueLink{frame: respFrame}, nil
	}
	require.NoError(t, transport.Connect(context.Background()))
	defer transport.Disconnect()

	scheduler := NewDLT645Scheduler(transport, NewDLT645Decoder())
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "good", Address: "210220003011#02-01-01-00", DataType: "UINT16"},
		{ID: "bad", Address: "INVALID", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["good"].Quality)
	assert.Equal(t, "Bad", results["bad"].Quality)
}
