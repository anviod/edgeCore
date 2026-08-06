package knxnetip

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatGroupAddress(t *testing.T) {
	ga, err := parseGroupAddress("1/2/3")
	require.NoError(t, err)
	assert.Equal(t, "1/2/3", formatGroupAddress(ga))

	ga2, err := parseGroupAddress("15/7/255")
	require.NoError(t, err)
	assert.Contains(t, formatGroupAddress(ga2), "15")
}

func TestDecodeEncodeValueCoverage(t *testing.T) {
	t.Run("INT8 with scale", func(t *testing.T) {
		v, err := DecodeValue([]byte{0xFE}, "INT8", nil, 2, 1)
		require.NoError(t, err)
		assert.InDelta(t, -3.0, v.(float64), 0.001)
	})

	t.Run("UINT8", func(t *testing.T) {
		v, err := DecodeValue([]byte{0x7F}, "UINT8", nil, 1, 0)
		require.NoError(t, err)
		assert.Equal(t, float64(127), v)
	})

	t.Run("INT16", func(t *testing.T) {
		v, err := DecodeValue([]byte{0xFF, 0xF6}, "INT16", nil, 1, 0)
		require.NoError(t, err)
		assert.Equal(t, float64(-10), v)
	})

	t.Run("INT32", func(t *testing.T) {
		v, err := DecodeValue([]byte{0x00, 0x00, 0x03, 0xE8}, "INT32", nil, 1, 0)
		require.NoError(t, err)
		assert.Equal(t, float64(1000), v)
	})

	t.Run("UINT32", func(t *testing.T) {
		v, err := DecodeValue([]byte{0x00, 0x01, 0x86, 0xA0}, "UINT32", nil, 1, 0)
		require.NoError(t, err)
		assert.Equal(t, float64(100000), v)
	})

	t.Run("FLOAT 4-byte", func(t *testing.T) {
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, math.Float32bits(3.14))
		v, err := DecodeValue(buf, "FLOAT", nil, 1, 0)
		require.NoError(t, err)
		assert.InDelta(t, 3.14, v.(float64), 0.01)
	})

	t.Run("FLOAT DPT9 2-byte", func(t *testing.T) {
		// Verify DPT9 decode path is exercised (value depends on encoding)
		v, err := DecodeValue([]byte{0x0C, 0xAA}, "FLOAT", nil, 1, 0)
		require.NoError(t, err)
		assert.IsType(t, float64(0), v)
		assert.InDelta(t, decodeDPT9([]byte{0x0C, 0xAA}), v.(float64), 0.001)
	})

	t.Run("empty payload error", func(t *testing.T) {
		_, err := DecodeValue(nil, "BOOL", nil, 1, 0)
		require.Error(t, err)
	})

	t.Run("unsupported datatype", func(t *testing.T) {
		_, err := DecodeValue([]byte{0x01}, "UNKNOWN", nil, 1, 0)
		require.Error(t, err)
	})

	t.Run("EncodeValue all types", func(t *testing.T) {
		addr := &ParsedAddress{BitWidth: 2}
		b, err := EncodeValue(true, "BOOL", addr)
		require.NoError(t, err)
		assert.Equal(t, byte(0x40), b[0])

		b, err = EncodeValue(false, "BOOL", nil)
		require.NoError(t, err)
		assert.Equal(t, byte(0x00), b[0])

		b, err = EncodeValue(int64(42), "UINT16", nil)
		require.NoError(t, err)
		assert.Equal(t, uint16(42), binary.BigEndian.Uint16(b))

		b, err = EncodeValue(float64(1.5), "FLOAT", nil)
		require.NoError(t, err)
		assert.Equal(t, 4, len(b))

		_, err = EncodeValue("bad", "INT16", nil)
		require.Error(t, err)

		_, err = EncodeValue(true, "UNKNOWN", nil)
		require.Error(t, err)
	})
}

func TestToBoolIntFloatHelpers(t *testing.T) {
	b, err := toBool("true")
	require.NoError(t, err)
	assert.True(t, b)

	b, err = toBool("off")
	require.NoError(t, err)
	assert.False(t, b)

	_, err = toBool("maybe")
	require.Error(t, err)

	n, err := toInt64(float32(100))
	require.NoError(t, err)
	assert.Equal(t, int64(100), n)

	_, err = toInt64("x")
	require.Error(t, err)

	f, err := toFloat64(int(42))
	require.NoError(t, err)
	assert.Equal(t, 42.0, f)

	_, err = toFloat64("x")
	require.Error(t, err)

	assert.Equal(t, 0.0, decodeDPT9([]byte{0x00}))
}

func TestParseAddressErrors(t *testing.T) {
	_, err := ParseAddress("")
	require.Error(t, err)

	_, err = ParseAddress("99/99/99")
	require.Error(t, err)

	_, err = ParseAddress("1/2/3,invalid.ind")
	require.Error(t, err)

	_, err = ParseAddress("1/2/3,1.1.1,99")
	require.Error(t, err)
}

func TestDriverLifecycleAndMetrics(t *testing.T) {
	d := NewKNXnetIPDriver().(*KNXnetIPDriver)

	require.NoError(t, d.SetSlaveID(1))
	require.NoError(t, d.SetDeviceConfig(map[string]any{"foo": "bar"}))

	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "metrics-test",
		Config:    map[string]any{"ip": "192.168.1.50", "port": 3671},
	}))

	connSec, recon, local, remote, lastDisc := d.GetConnectionMetrics()
	assert.Equal(t, int64(0), connSec)
	assert.Equal(t, int64(0), recon)
	assert.Equal(t, "", local)
	assert.Contains(t, remote, "192.168.1.50")
	assert.True(t, lastDisc.IsZero())

	m := d.GetMetrics()
	assert.Equal(t, "KNXnet/IP", m.Protocol)
	assert.Less(t, m.QualityScore, 85) // not connected
}

func TestCalculateQualityScorePaths(t *testing.T) {
	d := NewKNXnetIPDriver().(*KNXnetIPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "score-test",
		Config:    map[string]any{"ip": "127.0.0.1", "port": 3671},
	}))

	// Simulate stats via scheduler
	d.scheduler.totalRequests = 10
	d.scheduler.successCount = 3
	d.scheduler.failureCount = 7
	m := d.GetMetrics()
	assert.Less(t, m.QualityScore, 60) // low success rate

	d.scheduler.successCount = 10
	d.scheduler.failureCount = 0
	m = d.GetMetrics()
	assert.Greater(t, m.SuccessRate, 0.9)
}

func TestGetCachedAndReadFallback(t *testing.T) {
	sim := NewSimulator()
	group, _ := parseGroupAddress("4/0/1")
	sim.SetGroupValue(group, []byte{0x00, 0x64})

	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	d := NewKNXnetIPDriver().(*KNXnetIPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cache-test",
		Config: map[string]any{
			"ip":      host,
			"port":    mustAtoi(portStr),
			"timeout": 2000,
		},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	// Prime cache via successful read
	_, err = d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "4/0/1", DataType: "UINT16"},
	})
	require.NoError(t, err)

	cached, ok := d.transport.GetCached(group)
	require.True(t, ok)
	assert.Equal(t, []byte{0x00, 0x64}, cached)

	total, _, _ := d.scheduler.GetStats()
	assert.Greater(t, total, int64(0))
}

func TestHPAIUDPAddr(t *testing.T) {
	h := hpai{ip: [4]byte{192, 168, 1, 1}, port: 3671}
	udp, err := hpaiUDPAddr(h)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", udp.IP.String())
	assert.Equal(t, 3671, udp.Port)

	_, err = hpaiUDPAddr(hpai{})
	require.Error(t, err)
}

func TestConfigParsingCoverage(t *testing.T) {
	cfg := parseTransportConfig(map[string]any{
		"ip":                 "10.0.0.1",
		"port":               float64(3671),
		"timeout":            1500,
		"heartbeat_interval": 1000,
		"max_retries":        5,
	})
	assert.Equal(t, "10.0.0.1", cfg.ip)
	assert.Equal(t, 1500*time.Millisecond, cfg.timeout)
	assert.Equal(t, 5, cfg.maxRetries)
}

func TestDriverNotConnectedErrors(t *testing.T) {
	d := NewKNXnetIPDriver().(*KNXnetIPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "nc",
		Config:    map[string]any{"ip": "127.0.0.1"},
	}))

	ctx := context.Background()
	_, err := d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "1/1/1"}})
	require.Error(t, err)

	err = d.WritePoint(ctx, model.Point{ID: "p1", Address: "1/1/1"}, true)
	require.Error(t, err)

	assert.Equal(t, driver.HealthStatusBad, d.Health())
}

func TestKNXDecoderWrapper(t *testing.T) {
	dec := NewKNXDecoder()
	parsed, err := dec.ParseAddress("1/2/3")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0A03), parsed.GroupAddr)
}

func TestSchedulerWriteWithSimulator(t *testing.T) {
	sim := NewSimulator()
	_, _ = parseGroupAddress("2/1/10")
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	d := NewKNXnetIPDriver().(*KNXnetIPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "write-test",
		Config: map[string]any{
			"ip": host, "port": mustAtoi(portStr), "timeout": 2000,
		},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	require.NoError(t, d.WritePoint(ctx, model.Point{
		ID: "w1", Address: "2/1/10", DataType: "BOOL",
	}, true))

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "r1", Address: "2/1/10", DataType: "BOOL"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["r1"].Quality)
	assert.True(t, results["r1"].Value.(bool))
}

func TestSchedulerInvalidAddressBatch(t *testing.T) {
	tr := NewKNXTransport(map[string]any{"ip": "127.0.0.1"})
	s := NewKNXScheduler(tr, NewKNXDecoder())

	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "invalid"},
		{ID: "bad2", Address: ""},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
	assert.Equal(t, "Bad", results["bad2"].Quality)

	total, _, failures := s.GetStats()
	assert.Equal(t, int64(2), failures)
	assert.Equal(t, int64(0), total)
}

func TestProtocolConnectResponse(t *testing.T) {
	body := make([]byte, 24)
	for i := 0; i < 16; i++ {
		body[i] = 0x08
	}
	body[16] = 8
	body[17] = 0x1A
	body[18] = 0x00
	binary.BigEndian.PutUint16(body[19:21], 0x1101)
	cr, err := parseConnectResponse(body)
	require.NoError(t, err)
	assert.Equal(t, byte(0x1A), cr.ChannelID)
	assert.Equal(t, uint16(0x1101), cr.KNXAddr)
}

func TestBuildConnectionStateRequest(t *testing.T) {
	req := buildConnectionStateRequest(5)
	require.NotEmpty(t, req)
	ch, status, err := parseConnectionStateResponse([]byte{4, 5, 0, 0})
	require.NoError(t, err)
	assert.Equal(t, byte(5), ch)
	assert.Equal(t, byte(0), status)
}
