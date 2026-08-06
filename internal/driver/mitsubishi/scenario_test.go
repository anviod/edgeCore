package mitsubishi

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_ConnectRetryBackoff(t *testing.T) {
	attempts := 0
	transport := NewMCTransport(driverConfig{
		ip:         "127.0.0.1",
		port:       59999,
		timeout:    50 * time.Millisecond,
		maxRetries: 2,
	})
	transport.dialFn = func(network, address string, timeout time.Duration) (net.Conn, error) {
		attempts++
		return nil, errors.New("connection refused")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := transport.Connect(ctx)
	require.Error(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestScenario_TransactionFailureMarksDisconnected(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = server.Close()

	transport := NewMCTransport(driverConfig{
		ip:      "127.0.0.1",
		port:    5000,
		timeout: 100 * time.Millisecond,
	})
	transport.conn = client
	transport.connected.Store(true)

	addr, err := ParseAddress("D100")
	require.NoError(t, err)
	_, err = transport.ReadRaw(addr, 2, false)
	require.Error(t, err)
	assert.False(t, transport.IsConnected())
}

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	mock := NewMockPLC()
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	transport := NewMCTransport(driverConfig{ip: host, port: port, timeout: time.Second, maxRetries: 1})
	require.NoError(t, transport.Connect(context.Background()))
	defer transport.Disconnect()

	scheduler := NewMCScheduler(transport, NewMCDecoder(), 8)
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "INVALID", DataType: "INT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
}

func TestScenario_ReconnectCountOnConnect(t *testing.T) {
	mock := NewMockPLC()
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	transport := NewMCTransport(driverConfig{ip: host, port: port, timeout: time.Second})
	require.NoError(t, transport.Connect(context.Background()))
	_, reconCount, _, _, _ := transport.GetConnectionMetrics()
	assert.Equal(t, int64(1), reconCount)

	require.NoError(t, transport.Disconnect())
	require.NoError(t, transport.Connect(context.Background()))
	_, reconCount, _, _, _ = transport.GetConnectionMetrics()
	assert.Equal(t, int64(2), reconCount)
}

func TestScenario_SchedulerStatsAccumulation(t *testing.T) {
	mock := NewMockPLC()
	mock.SetWord("D", 200, 42)
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	transport := NewMCTransport(driverConfig{ip: host, port: port, timeout: time.Second})
	require.NoError(t, transport.Connect(context.Background()))
	defer transport.Disconnect()

	scheduler := NewMCScheduler(transport, NewMCDecoder(), 8)
	point := model.Point{ID: "p1", Address: "D200", DataType: "INT16"}

	// MC 为串行协议；同 Transport 并发 I/O 无 channelMu 保护，与 ScanEngine 生产路径不一致。
	for i := 0; i < 20; i++ {
		_, err := scheduler.ReadPoints(context.Background(), []model.Point{point})
		require.NoError(t, err)
	}

	total, success, _ := scheduler.GetStats()
	assert.Equal(t, int64(20), total)
	assert.Equal(t, int64(20), success)
}

func TestScenario_MaxConnectRetriesExhausted(t *testing.T) {
	transport := NewMCTransport(driverConfig{
		ip:         "127.0.0.1",
		port:       59998,
		timeout:    20 * time.Millisecond,
		maxRetries: 2,
	})
	transport.dialFn = func(network, address string, timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := transport.Connect(ctx)
	require.Error(t, err)
	assert.False(t, transport.IsConnected())
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	mock := NewMockPLC()
	mock.SetWord("D", 100, 7)
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	transport := NewMCTransport(driverConfig{ip: host, port: port, timeout: time.Second})
	require.NoError(t, transport.Connect(context.Background()))
	defer transport.Disconnect()

	scheduler := NewMCScheduler(transport, NewMCDecoder(), 8)
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "good", Address: "D100", DataType: "INT16"},
		{ID: "bad", Address: "INVALID", DataType: "INT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["good"].Quality)
	assert.Equal(t, "Bad", results["bad"].Quality)
	assert.True(t, transport.IsConnected())
}
