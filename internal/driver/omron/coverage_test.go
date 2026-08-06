package omron

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	finslib "github.com/anviod/fins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_DriverBeforeInit(t *testing.T) {
	d := NewOmronFinsDriver().(*OmronFinsDriver)

	assert.Equal(t, driver.HealthStatusBad, d.Health())
	assert.NoError(t, d.SetSlaveID(1))
	assert.NoError(t, d.SetDeviceConfig(map[string]any{"ip": "10.0.0.1"}))

	connSec, recon, local, _, lastDisc := d.GetConnectionMetrics()
	assert.Equal(t, int64(0), connSec)
	assert.Equal(t, int64(0), recon)
	assert.Empty(t, local)
	assert.True(t, lastDisc.IsZero())

	m := d.GetMetrics()
	assert.Equal(t, "Omron FINS", m.Protocol)

	ctx := context.Background()
	_, err := d.ReadPoints(ctx, nil)
	require.Error(t, err)
	err = d.WritePoint(ctx, model.Point{Address: "D0"}, int16(1))
	require.Error(t, err)
}

func TestCoverage_ConfigHelpers(t *testing.T) {
	assert.Equal(t, "192.168.1.20:9600", remoteAddrFromConfig(map[string]any{
		"ip": "192.168.1.20", "port": 9600,
	}))

	v := configInt(map[string]any{"x": float64(42)}, "x")
	assert.Equal(t, 42, v)

	b := configByte(map[string]any{"n": 255}, "n")
	assert.Equal(t, byte(255), b)

	cfg := toFinsLibConfig(map[string]any{
		"src_node_addr": 2, "dst_node_addr": 1,
		"max_retries": 5, "retry_interval": 100,
	})
	assert.Equal(t, 2, cfg["srcNodeAddr"])
	assert.Equal(t, 5, cfg["maxRetries"])
}

func TestCoverage_UDPBackendInit(t *testing.T) {
	d := NewOmronFinsDriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "udp",
		Config:    map[string]any{"mode": "UDP", "port": 9600},
	}))

	err := d.Connect(context.Background())
	require.Error(t, err) // no IP
}

func TestCoverage_ConvertHelpers(t *testing.T) {
	cases := []struct {
		dt   string
		want finslib.DataType
	}{
		{"BOOL", finslib.DataTypeBIT},
		{"int16", finslib.DataTypeINT16},
		{"FLOAT32", finslib.DataTypeFLOAT},
		{"DOUBLE", finslib.DataTypeDOUBLE},
		{"CUSTOM", finslib.DataType("CUSTOM")},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, toFinsDataType(tc.dt), tc.dt)
	}

	finsVals := map[string]finslib.Value{
		"p1": {Value: int16(42), Quality: finslib.QualityGood},
	}
	modelVals := fromFinsValues(finsVals)
	assert.Equal(t, int16(42), modelVals["p1"].Value)
	assert.Equal(t, "Good", modelVals["p1"].Quality)

	assert.Equal(t, driver.HealthStatusGood, toDriverHealth(finslib.HealthStatusUp))
	assert.Equal(t, driver.HealthStatusBad, toDriverHealth(finslib.HealthStatusDown))
	assert.Equal(t, driver.HealthStatusUnknown, toDriverHealth(finslib.HealthStatus("unknown")))

	now := time.Now().Add(-5 * time.Second)
	sec, recon, local, remote, lastDisc := connectionMetricsTuple(finslib.ConnectionMetrics{
		Connected: true, ConnectTime: now, ReconnectCount: 2,
		LocalAddr: "0.0.0.0:9600", RemoteAddr: "10.0.0.1:9600",
	})
	assert.GreaterOrEqual(t, sec, int64(4))
	assert.Equal(t, int64(2), recon)
	assert.Equal(t, "0.0.0.0:9600", local)
	assert.Equal(t, "10.0.0.1:9600", remote)
	assert.True(t, lastDisc.IsZero())
}

func TestCoverage_TCPBackendLifecycle(t *testing.T) {
	b := newTCPBackend()
	require.NoError(t, b.Init(finslib.DriverConfig{Config: map[string]interface{}{"ip": "10.0.0.2"}}))
	require.Error(t, b.Init(finslib.DriverConfig{}))

	ctx := context.Background()
	_, err := b.ReadPoints(ctx, nil)
	require.Error(t, err)
	require.Error(t, b.WritePoint(ctx, finslib.Point{Address: "D0"}, int16(1)))
	assert.Equal(t, finslib.HealthStatusDown, b.Health())

	require.NoError(t, b.SetDeviceConfig(map[string]interface{}{"ip": "10.0.0.3"}))
	metrics := b.GetConnectionMetrics()
	assert.Contains(t, metrics.RemoteAddr, "10.0.0")
	assert.Equal(t, finslib.SchedulerStats{}, b.GetSchedulerStats())
}

func TestCoverage_DriverMetricsAndQuality(t *testing.T) {
	d := NewOmronFinsDriver().(*OmronFinsDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "metrics",
		Config:    map[string]any{"ip": "192.168.1.50", "port": 9600},
	}))

	m := d.GetMetrics()
	assert.Equal(t, "Omron FINS", m.Protocol)
	assert.Equal(t, 15, d.calculateQualityScore(0.3)) // low success + backend down

	_, recon, _, remote, _ := d.GetConnectionMetrics()
	assert.Contains(t, remote, "192.168.1.50")
	assert.Equal(t, int64(0), recon)
}

type mockUDPClient struct {
	words map[uint16]uint16
}

func (m *mockUDPClient) Close()              {}
func (m *mockUDPClient) SetTimeoutMs(_ uint) {}
func (m *mockUDPClient) ReadWords(_ byte, address uint16, readCount uint16) ([]uint16, error) {
	out := make([]uint16, readCount)
	for i := uint16(0); i < readCount; i++ {
		out[i] = m.words[address+i]
	}
	return out, nil
}
func (m *mockUDPClient) ReadBytes(_ byte, address uint16, readCount uint16) ([]byte, error) {
	words, err := m.ReadWords(0, address, readCount)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, len(words)*2)
	for i, w := range words {
		buf[i*2] = byte(w >> 8)
		buf[i*2+1] = byte(w)
	}
	return buf, nil
}
func (m *mockUDPClient) ReadBits(_ byte, _ uint16, _ byte, readCount uint16) ([]bool, error) {
	out := make([]bool, readCount)
	return out, nil
}
func (m *mockUDPClient) WriteWords(_ byte, address uint16, data []uint16) error {
	for i, w := range data {
		m.words[address+uint16(i)] = w
	}
	return nil
}
func (m *mockUDPClient) WriteBytes(_ byte, address uint16, b []byte) error {
	for i := 0; i+1 < len(b); i += 2 {
		m.words[address+uint16(i/2)] = uint16(b[i])<<8 | uint16(b[i+1])
	}
	return nil
}
func (m *mockUDPClient) WriteBits(_ byte, _ uint16, _ byte, _ []bool) error { return nil }

func TestCoverage_UDPBackendWithMockClient(t *testing.T) {
	d := NewOmronFinsDriver().(*OmronFinsDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "udp-mock",
		Config: map[string]any{
			"mode": "UDP", "ip": "127.0.0.1", "port": 9600,
		},
	}))

	backend := d.backend.(*udpBackend)
	mock := &mockUDPClient{words: map[uint16]uint16{100: 777}}
	backend.connected.Store(true)
	backend.client = mock
	backend.scheduler.setClient(mock)

	ctx := context.Background()
	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "D100", DataType: "INT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)
	assert.Equal(t, int16(777), results["p1"].Value)

	require.NoError(t, d.WritePoint(ctx, model.Point{Address: "D100", DataType: "INT16"}, int16(888)))
	assert.Equal(t, uint16(888), mock.words[100])
	assert.Equal(t, driver.HealthStatusGood, d.Health())
}
