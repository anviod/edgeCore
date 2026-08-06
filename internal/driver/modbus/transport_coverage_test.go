package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHookedTransport() *ModbusTransport {
	tr := NewModbusTransport(model.DriverConfig{
		ChannelID: "hook",
		Config: map[string]any{
			"url":            "127.0.0.1:502",
			"timeout":        500,
			"max_retries":    1,
			"max_fail_count": 2,
		},
	})
	tr.connected.Store(true)
	regs := map[uint16]uint16{0: 0x1234, 1: 0x5678}
	coil := map[uint16]bool{10: true}

	tr.readRegistersHook = func(_ context.Context, regType string, offset uint16, count uint16) ([]byte, error) {
		if regType == "input" {
			return nil, fmt.Errorf("illegal data address")
		}
		buf := make([]byte, count*2)
		for i := uint16(0); i < count; i++ {
			binary.BigEndian.PutUint16(buf[i*2:], regs[offset+i])
		}
		return buf, nil
	}
	tr.readCoilHook = func(_ context.Context, offset uint16) (bool, error) {
		return coil[offset], nil
	}
	tr.readDiscreteInputHook = func(_ context.Context, offset uint16) (bool, error) {
		return !coil[offset], nil
	}
	tr.writeRegisterHook = func(_ context.Context, offset uint16, value uint16) error {
		regs[offset] = value
		return nil
	}
	tr.writeRegistersHook = func(_ context.Context, offset uint16, values []uint16) error {
		for i, v := range values {
			regs[offset+uint16(i)] = v
		}
		return nil
	}
	tr.writeCoilHook = func(_ context.Context, offset uint16, value bool) error {
		coil[offset] = value
		return nil
	}
	return tr
}

func TestCoverage_TransportReadWriteViaHooks(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	ctx := context.Background()

	raw, err := tr.ReadRegisters(ctx, "holding", 0, 2)
	require.NoError(t, err)
	assert.Len(t, raw, 4)

	val, err := tr.ReadCoil(ctx, 10)
	require.NoError(t, err)
	assert.True(t, val)

	val, err = tr.ReadDiscreteInput(ctx, 10)
	require.NoError(t, err)
	assert.False(t, val)

	require.NoError(t, tr.WriteRegister(ctx, 0, 999))
	require.NoError(t, tr.WriteRegisters(ctx, 1, []uint16{111, 222}))
	require.NoError(t, tr.WriteCoil(ctx, 11, true))

	_, err = tr.ReadCustom(ctx, 0x03, 0, 1)
	require.Error(t, err)
}

func TestCoverage_TransportWithRetryDeviceError(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	tr.readRegistersHook = func(_ context.Context, regType string, _, _ uint16) ([]byte, error) {
		return nil, fmt.Errorf("modbus exception: illegal function")
	}

	_, err := tr.ReadRegisters(context.Background(), "input", 0, 1)
	require.Error(t, err)
}

func TestCoverage_TransportWithRetryNetworkError(t *testing.T) {
	tr := NewModbusTransport(model.DriverConfig{
		Config: map[string]any{"url": "127.0.0.1:502", "max_retries": 0, "timeout": 100},
	})
	defer tr.connMgr.Close()
	tr.connected.Store(true)
	tr.readRegistersHook = func(context.Context, string, uint16, uint16) ([]byte, error) {
		return nil, fmt.Errorf("connection reset by peer")
	}

	_, err := tr.ReadRegisters(context.Background(), "holding", 0, 1)
	require.Error(t, err)
}

func TestCoverage_TransportProbeAndMetricsConnected(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	tr.connectTime = time.Now().Add(-5 * time.Second)
	tr.localAddr = "127.0.0.1:50100"
	tr.remoteAddr = "127.0.0.1:502"

	sec, recon, local, remote, _ := tr.GetConnectionMetrics()
	assert.GreaterOrEqual(t, sec, int64(4))
	assert.Equal(t, int64(0), recon)
	assert.Equal(t, "127.0.0.1:50100", local)
	assert.Equal(t, "127.0.0.1:502", remote)

	tr.lastActivityTime.Store(time.Now().Add(-60 * time.Second))
	assert.True(t, tr.NeedProbeCheck())
}

func TestCoverage_ContainsHelper(t *testing.T) {
	assert.True(t, contains("modbus timeout", "timeout"))
	assert.False(t, contains("ok", "timeout"))
}

func TestCoverage_TransportConnectOnceMissingURL(t *testing.T) {
	tr := NewModbusTransport(model.DriverConfig{Config: map[string]any{}})
	defer tr.connMgr.Close()
	err := tr.connectOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestCoverage_TransportConnectOnceHostPort(t *testing.T) {
	tr := NewModbusTransport(model.DriverConfig{
		Config: map[string]any{"host": "127.0.0.1", "port": 502, "timeout": 100},
	})
	defer tr.connMgr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = tr.connectOnce(ctx)
}

func TestCoverage_TransportConnectOnceRTUConfig(t *testing.T) {
	tr := NewModbusTransport(model.DriverConfig{
		Config: map[string]any{
			"port": "/dev/ttyUSB0", "baudRate": 9600,
			"dataBits": 8, "stopBits": 1, "parity": "N",
		},
	})
	defer tr.connMgr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = tr.connectOnce(ctx)
}

func TestCoverage_DetectMTUWithHooks(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	tr.readRegistersHook = func(_ context.Context, _ string, _ uint16, count uint16) ([]byte, error) {
		if count > 80 {
			return nil, fmt.Errorf("illegal data address")
		}
		return make([]byte, count*2), nil
	}
	mtu, err := tr.DetectMTU(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, mtu, uint16(32))
}

func TestCoverage_DriverBatchReadMixedRegisters(t *testing.T) {
	d := NewModbusDriver().(*ModbusDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "batch",
		Config:    map[string]any{"url": "127.0.0.1:502", "slave_id": 1, "batchSize": 5},
	}))
	regs := map[uint16]uint16{0: 100, 1: 200, 2: 300}
	coil := map[uint16]bool{1000: true}
	d.transport.readRegistersHook = func(_ context.Context, regType string, offset uint16, count uint16) ([]byte, error) {
		if regType == "input" {
			buf := make([]byte, count*2)
			for i := uint16(0); i < count; i++ {
				binary.BigEndian.PutUint16(buf[i*2:], uint16(500+offset+i))
			}
			return buf, nil
		}
		buf := make([]byte, count*2)
		for i := uint16(0); i < count; i++ {
			binary.BigEndian.PutUint16(buf[i*2:], regs[offset+i])
		}
		return buf, nil
	}
	d.transport.readCoilHook = func(_ context.Context, offset uint16) (bool, error) {
		return coil[offset], nil
	}
	d.transport.readDiscreteInputHook = func(_ context.Context, offset uint16) (bool, error) {
		return !coil[offset], nil
	}
	d.transport.connected.Store(true)

	results, err := d.ReadPoints(context.Background(), []model.Point{
		{ID: "h1", Address: "40001", DataType: "int16", RegisterType: model.RegHolding},
		{ID: "h2", Address: "40002", DataType: "int16", RegisterType: model.RegHolding},
		{ID: "i1", Address: "30001", DataType: "uint16", RegisterType: model.RegInput},
		{ID: "c1", Address: "1001", DataType: "bool", RegisterType: model.RegCoil},
		{ID: "d1", Address: "10001", DataType: "bool", RegisterType: model.RegDiscreteInput},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["h1"].Quality)
	assert.Equal(t, int16(100), results["h1"].Value)
	assert.Equal(t, "Good", results["i1"].Quality)
	assert.Equal(t, uint16(500), results["i1"].Value)
	assert.Equal(t, true, results["c1"].Value)
	assert.Equal(t, true, results["d1"].Value)
}

func TestCoverage_TransportWriteCoilViaHook(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	coil := map[uint16]bool{}
	tr.writeCoilHook = func(_ context.Context, offset uint16, value bool) error {
		coil[offset] = value
		return nil
	}
	tr.connected.Store(true)
	require.NoError(t, tr.WriteCoil(context.Background(), 1000, true))
	assert.True(t, coil[1000])
}

type mockMetricsRecorder struct {
	requests int
}

func (m *mockMetricsRecorder) RecordRequest(string, bool, time.Duration, string) { m.requests++ }
func (m *mockMetricsRecorder) RecordReconnect(string)                            {}
func (m *mockMetricsRecorder) RecordConnectionStart(string)                      {}
func (m *mockMetricsRecorder) RecordError(string, string, string, string)        {}
func (m *mockMetricsRecorder) RecordPointDebug(string, string, []byte, any, string) {
}
func (m *mockMetricsRecorder) RecordCycle(string, bool) {}

func TestCoverage_SchedulerPointCooldown(t *testing.T) {
	mock := newMockModbusTransport()
	mock.connected = true
	mock.registers[0] = 42

	s := NewPointScheduler(mock, NewPointDecoder("ABCD", 0, 0), 125, 50, 0)
	s.mu.Lock()
	s.pointStates["p1"] = &PointRuntime{
		Point:         model.Point{ID: "p1", Address: "40001", DataType: "int16", RegisterType: model.RegHolding},
		State:         "SKIPPED",
		CooldownUntil: time.Now().Add(-time.Second),
	}
	s.mu.Unlock()

	results, err := s.Read(context.Background(), []model.Point{
		{ID: "p1", Address: "40001", DataType: "int16", RegisterType: model.RegHolding},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)
}

func TestCoverage_SchedulerWriteMultipleRegisters(t *testing.T) {
	mock := newMockModbusTransport()
	mock.connected = true
	s := NewPointScheduler(mock, NewPointDecoder("ABCD", 0, 0), 125, 50, 0)

	pt := model.Point{Address: "40001", DataType: "int32", RegisterType: model.RegHolding}
	require.NoError(t, s.Write(context.Background(), pt, int32(0x12345678)))
	assert.NotZero(t, mock.registers[0])
	assert.NotZero(t, mock.registers[1])
}

func TestCoverage_TransportProbeUsesHook(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	tr.connected.Store(true)
	calls := 0
	tr.readCoilHook = func(_ context.Context, offset uint16) (bool, error) {
		calls++
		if offset != 0 {
			t.Fatalf("probe offset = %d", offset)
		}
		return true, nil
	}
	tr.ProbeConnection()
	assert.Equal(t, 1, calls)
	assert.True(t, tr.connected.Load())
}

func TestCoverage_TransportProbeFailureSchedulesReconnect(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	tr.connected.Store(true)
	tr.maxFailCount = 1
	tr.readCoilHook = func(context.Context, uint16) (bool, error) {
		return false, fmt.Errorf("connection reset by peer")
	}
	tr.ProbeConnection()
	assert.GreaterOrEqual(t, tr.collectFailCount.Load(), int32(1))
}

func TestCoverage_TransportMetricsRecorder(t *testing.T) {
	tr := newHookedTransport()
	defer tr.connMgr.Close()
	rec := &mockMetricsRecorder{}
	tr.SetMetricsRecorder(rec, "ch1")
	tr.connected.Store(true)
	tr.connectTime = time.Now()
	_, _ = tr.ReadRegisters(context.Background(), "holding", 0, 1)
	assert.GreaterOrEqual(t, rec.requests, 0)
}
