package profinetio

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	d := NewProfinetIODriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config:    map[string]any{"simulation": true},
	}))
	require.NoError(t, d.Connect(context.Background()))
	defer d.Disconnect()
	d.SetDeviceConfig(map[string]any{"ip": "192.168.1.20"})

	results, err := d.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "DB1.DBD0", DataType: "int16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
}

func TestScenario_ReconnectMetrics(t *testing.T) {
	d := NewProfinetIODriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config:    map[string]any{"simulation": true},
	}))
	require.NoError(t, d.Connect(context.Background()))

	_, recon1, _, _, _ := d.GetConnectionMetrics()
	require.NoError(t, d.Disconnect())
	require.NoError(t, d.Connect(context.Background()))
	_, recon2, _, _, lastDisc := d.GetConnectionMetrics()
	assert.GreaterOrEqual(t, recon2, recon1)
	assert.False(t, lastDisc.IsZero())
}

func TestScenario_ConnectWithoutDeviceIPDeferred(t *testing.T) {
	transport := NewProfinetTransport(channelConfig{simulation: false, timeout: time.Second})
	require.NoError(t, transport.Connect(context.Background()))
	assert.True(t, transport.IsConnected())

	_, _, _, remote, _ := transport.GetConnectionMetrics()
	assert.Empty(t, remote)

	_, err := transport.ReadIO(context.Background(), 3, 1, 0, 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestScenario_ConcurrentSimulationReads(t *testing.T) {
	d := NewProfinetIODriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config:    map[string]any{"simulation": true},
	}))
	require.NoError(t, d.Connect(context.Background()))
	defer d.Disconnect()
	d.SetDeviceConfig(map[string]any{"ip": "192.168.1.20"})

	pt := model.Point{ID: "p1", Address: "3:1:0", DataType: "int16"}
	require.NoError(t, d.WritePoint(context.Background(), pt, int16(99)))

	var wg sync.WaitGroup
	var ops int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := d.ReadPoints(context.Background(), []model.Point{pt})
			if err == nil && results["p1"].Quality == "Good" {
				atomic.AddInt32(&ops, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(20), atomic.LoadInt32(&ops))
}

func TestScenario_ConnectionManagerBackoff(t *testing.T) {
	transport := NewProfinetTransport(channelConfig{
		simulation: false,
		timeout:    50 * time.Millisecond,
		maxRetries: 5,
	})
	defer transport.connMgr.Close()

	transport.connMgr.SetBackoffParams(100*time.Millisecond, 30*time.Second, 2.0)
	transport.connMgr.RecordSuccess()

	var backoffs []time.Duration
	for i := 0; i < 4; i++ {
		_, backoff := transport.connMgr.RecordFailure()
		backoffs = append(backoffs, backoff)
	}
	for i := 1; i < len(backoffs); i++ {
		assert.GreaterOrEqual(t, backoffs[i], backoffs[i-1])
	}
}

func TestScenario_MaxFailuresEnterDead(t *testing.T) {
	transport := NewProfinetTransport(channelConfig{
		simulation: false,
		timeout:    50 * time.Millisecond,
		maxRetries: 3,
	})
	defer transport.connMgr.Close()

	transport.connMgr.SetMaxRetries(3)
	transport.connMgr.RecordSuccess()
	for i := 0; i < 3; i++ {
		transport.connMgr.RecordFailure()
	}
	assert.Equal(t, driver.StateDead, transport.connMgr.GetState())
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	d := NewProfinetIODriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config:    map[string]any{"simulation": true},
	}))
	require.NoError(t, d.Connect(context.Background()))
	defer d.Disconnect()
	d.SetDeviceConfig(map[string]any{"ip": "192.168.1.20"})

	goodPt := model.Point{ID: "good", Address: "3:1:0", DataType: "int16"}
	require.NoError(t, d.WritePoint(context.Background(), goodPt, int16(42)))

	results, err := d.ReadPoints(context.Background(), []model.Point{
		goodPt,
		{ID: "bad", Address: "INVALID", DataType: "int16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["good"].Quality)
	assert.Equal(t, "Bad", results["bad"].Quality)
}
