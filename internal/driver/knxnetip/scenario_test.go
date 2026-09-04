package knxnetip

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	sim := NewSimulator()
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewKNXnetIPDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": host, "port": port, "timeout": 2000},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "bad", Address: "INVALID", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
}

func TestScenario_ReconnectMetrics(t *testing.T) {
	sim := NewSimulator()
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewKNXnetIPDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": host, "port": port, "timeout": 2000},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))

	_, recon1, _, _, _ := d.GetConnectionMetrics()
	require.NoError(t, d.Disconnect())
	require.NoError(t, d.Connect(ctx))
	_, recon2, _, _, lastDisc := d.GetConnectionMetrics()
	assert.GreaterOrEqual(t, recon2, recon1)
	assert.False(t, lastDisc.IsZero())
}

func TestScenario_TimeoutConfig(t *testing.T) {
	cfg := parseTransportConfig(map[string]any{
		"ip":      "127.0.0.1",
		"timeout": 500,
	})
	assert.Equal(t, 500*time.Millisecond, cfg.timeout)
}

func TestScenario_MaxFailuresEnterDead(t *testing.T) {
	transport := NewKNXTransport(map[string]any{
		"ip":         "127.0.0.1",
		"timeout":    500,
		"maxRetries": 3,
	})
	defer transport.connMgr.Close()

	transport.connMgr.SetMaxRetries(3)
	transport.connMgr.RecordSuccess()
	for i := 0; i < 3; i++ {
		transport.connMgr.RecordFailure()
	}
	assert.Equal(t, driver.StateDead, transport.connMgr.GetState())
}

func TestScenario_HalfOpenProbe(t *testing.T) {
	transport := NewKNXTransport(map[string]any{"maxRetries": 2})
	defer transport.connMgr.Close()

	transport.connMgr.SetMaxRetries(2)
	transport.connMgr.RecordSuccess()
	transport.connMgr.RecordFailure()
	transport.connMgr.RecordFailure()
	assert.Equal(t, driver.StateDead, transport.connMgr.GetState())

	transport.connMgr.AttemptHalfOpen(true)
	assert.Equal(t, driver.StateConnected, transport.connMgr.GetState())
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	sim := NewSimulator()
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewKNXnetIPDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": host, "port": port, "timeout": 2000},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "bad", Address: "INVALID", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
}
