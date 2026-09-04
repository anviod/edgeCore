package dlt645

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLink struct {
	readBuf  *bytes.Buffer
	writeBuf bytes.Buffer
	closed   bool
	mu       sync.Mutex
}

func (m *mockLink) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readBuf == nil {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockLink) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuf.Write(p)
}

func (m *mockLink) Close() error {
	m.closed = true
	return nil
}

func (m *mockLink) written() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.writeBuf.Bytes()...)
}

func TestTransportConnectMock(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"port":           8001,
	})

	mock := &mockLink{}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}

	err := transport.Connect(context.Background())
	require.NoError(t, err)
	assert.True(t, transport.IsConnected())

	connSec, reconCount, _, remoteAddr, _ := transport.GetConnectionMetrics()
	assert.Equal(t, int64(0), connSec)
	assert.Equal(t, int64(1), reconCount)
	assert.Equal(t, "127.0.0.1:8001", remoteAddr)

	err = transport.Disconnect()
	require.NoError(t, err)
	assert.False(t, transport.IsConnected())
	assert.True(t, mock.closed)
}

func TestTransportReadDataMock(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")

	value := []byte{0x20, 0x02}
	respBody := append(di[:], value...)
	respFrame := buildFrame(addr, CtrlReadResp, encode033(respBody))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"port":           8001,
		"timeout":        float64(1000),
		"preambleBytes":  0,
	})
	mock := &mockLink{readBuf: bytes.NewBuffer(respFrame)}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}
	require.NoError(t, transport.Connect(context.Background()))

	got, err := transport.ReadData(context.Background(), addr, di)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	written := mock.written()
	assert.Equal(t, byte(FrameStart), written[0])
	assert.Equal(t, byte(CtrlRead), written[8])
}

func TestTransportSerialPreamble(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")
	respFrame := buildFrame(addr, CtrlReadResp, encode033(append(di[:], 0x20, 0x02)))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "serial",
		"port":           "/dev/ttyS0",
		"preambleBytes":  4,
		"timeout":        float64(1000),
	})
	mock := &mockLink{readBuf: bytes.NewBuffer(respFrame)}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}
	require.NoError(t, transport.Connect(context.Background()))

	_, err := transport.ReadData(context.Background(), addr, di)
	require.NoError(t, err)

	written := mock.written()
	require.GreaterOrEqual(t, len(written), 4)
	for i := 0; i < 4; i++ {
		assert.Equal(t, byte(PreambleByte), written[i])
	}
}

func TestTransportMissingIP(t *testing.T) {
	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
	})
	transport.linkFactory = defaultLinkFactory
	err := transport.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IP address")
}

func TestReadFrameFromBuffer(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")
	frame := BuildReadFrame(addr, di)

	// Leading preamble bytes should be skipped.
	buf := append([]byte{PreambleByte, PreambleByte, PreambleByte}, frame...)
	got, err := readFrame(bytes.NewReader(buf), time.Now().Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, frame, got)
}

func TestDLT645DriverInitMetrics(t *testing.T) {
	d := NewDLT645Driver().(*DLT645Driver)
	err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config: map[string]any{
			"connectionType": "tcp",
			"ip":             "127.0.0.1",
			"port":           float64(10000),
		},
	})
	require.NoError(t, err)

	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	assert.Equal(t, int64(0), connSec)
	assert.Equal(t, int64(0), reconCount)
	assert.Empty(t, localAddr)
	assert.Equal(t, "127.0.0.1:10000", remoteAddr)
	assert.True(t, lastDisc.IsZero())

	metrics := d.GetMetrics()
	assert.Equal(t, "DLT645", metrics.Protocol)
}
