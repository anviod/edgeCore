package omron

import (
	"context"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	finslib "github.com/anviod/fins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	mock := finslib.NewMockPLC()
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewOmronFinsDriver()
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

func TestScenario_ConnectRequiresIP(t *testing.T) {
	d := NewOmronFinsDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 9600},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.Connect(ctx)
	require.Error(t, err)
}

func TestScenario_ReconnectMetrics(t *testing.T) {
	mock := finslib.NewMockPLC()
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewOmronFinsDriver()
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

func TestScenario_ConcurrentReadPoints(t *testing.T) {
	mock := finslib.NewMockPLC()
	mock.SetWord(finslib.MemoryAreaDMWord, 300, 99)
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewOmronFinsDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": host, "port": port, "timeout": 2000},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	point := model.Point{ID: "p1", Address: "D300", DataType: "UINT16"}
	var wg sync.WaitGroup
	var ops int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := d.ReadPoints(ctx, []model.Point{point})
			if err == nil && results["p1"].Quality == "Good" {
				atomic.AddInt32(&ops, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(20), atomic.LoadInt32(&ops))
}

func TestScenario_ConnectRetryExhausted(t *testing.T) {
	d := NewOmronFinsDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": "127.0.0.1", "port": 59997, "timeout": 200},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := d.Connect(ctx)
	require.Error(t, err)
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	mock := finslib.NewMockPLC()
	mock.SetWord(finslib.MemoryAreaDMWord, 400, 11)
	addr, err := mock.Start()
	require.NoError(t, err)
	defer mock.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	d := NewOmronFinsDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"ip": host, "port": port, "timeout": 2000},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "good", Address: "D400", DataType: "UINT16"},
		{ID: "bad", Address: "INVALID", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["good"].Quality)
	assert.Equal(t, "Bad", results["bad"].Quality)
}
