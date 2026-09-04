package snmp

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriverLifecycleCoverage(t *testing.T) {
	d := NewSNMPDriver().(*SNMPDriver)
	require.NoError(t, d.SetSlaveID(1))
	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"ip": "127.0.0.1", "community": "private",
	}))

	assert.Equal(t, driver.HealthStatusBad, d.Health())

	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config:    map[string]any{"ip": "127.0.0.1", "port": 161, "community": "public"},
	}))

	_, _, _, remote, _ := d.GetConnectionMetrics()
	assert.Contains(t, remote, "127.0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.Connect(ctx)
	require.Error(t, err) // no real agent

	_, err = d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "public|1.3.6.1.2.1.1.1.0"}})
	require.Error(t, err)
}

func TestDriverReadWriteWithMockHooks(t *testing.T) {
	d := NewSNMPDriver().(*SNMPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "rw",
		Config:    map[string]any{"ip": "127.0.0.1", "community": "public"},
	}))

	d.transport.getHook = func(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
		return []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.Integer, Value: 42}}, nil
	}
	d.transport.setHook = func(oid string, value interface{}, asnType gosnmp.Asn1BER, community string) error {
		return nil
	}
	d.transport.connected.Store(true)
	d.transport.client = &gosnmp.GoSNMP{}

	ctx := context.Background()
	assert.Equal(t, driver.HealthStatusGood, d.Health())

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "public|1.3.6.1.2.1.1.1.0", DataType: "INT32"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)

	require.NoError(t, d.WritePoint(ctx, model.Point{
		Address: "public|1.3.6.1.4.1.1.0", DataType: "INT32",
	}, int32(99)))

	total, success, _ := d.scheduler.GetStats()
	assert.Equal(t, int64(2), total)
	assert.Equal(t, int64(2), success)
}

func TestDecoderFloatAndInt64(t *testing.T) {
	dec := NewSNMPDecoder()
	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.Gauge32, Value: uint(12345)}, "UINT32")
	assert.Equal(t, "Good", quality)
	assert.NotNil(t, val)

	val, quality = dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: []byte("hello")}, "STRING")
	assert.Equal(t, "Good", quality)
	assert.Equal(t, "hello", val)

	_, ok := toInt64(gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: "bad"})
	assert.False(t, ok)

	v, ok := toUint64(gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: 100})
	assert.True(t, ok)
	assert.Equal(t, uint64(100), v)
}

func TestDriverDisconnectCoverage(t *testing.T) {
	d := NewSNMPDriver().(*SNMPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"ip": "127.0.0.1"},
	}))
	d.transport.connected.Store(true)
	require.NoError(t, d.Disconnect())
	assert.Equal(t, driver.HealthStatusBad, d.Health())
}

func TestScanObjectsWithWalkHook(t *testing.T) {
	d := NewSNMPDriver().(*SNMPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "scan",
		Config:    map[string]any{"ip": "127.0.0.1", "community": "public"},
	}))

	d.transport.connected.Store(true)
	d.transport.client = &gosnmp.GoSNMP{}
	d.transport.walkHook = func(rootOID, community string, walkFn func(pdu gosnmp.SnmpPDU) error) error {
		assert.Equal(t, "1.3.6.1.2.1", rootOID)
		assert.Equal(t, "public", community)
		return walkFn(gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("test")})
	}

	entries, err := d.ScanObjects(context.Background(), map[string]any{"rootOID": "1.3.6.1.2.1"})
	require.NoError(t, err)
	require.NotNil(t, entries)
}

func TestBuildClientV3Coverage(t *testing.T) {
	t.Run("valid authPriv", func(t *testing.T) {
		tr := NewSNMPTransport(map[string]any{
			"snmpVersion":   "v3",
			"securityName":  "admin",
			"authProtocol":  "SHA",
			"authPassword":  "authpass123",
			"privProtocol":  "AES",
			"privPassword":  "privpass123",
			"securityLevel": "authPriv",
			"ip":            "127.0.0.1",
		})
		client, err := tr.buildClient()
		require.NoError(t, err)
		assert.Equal(t, gosnmp.Version3, client.Version)
	})

	t.Run("invalid v3 config", func(t *testing.T) {
		tr := NewSNMPTransport(map[string]any{
			"snmpVersion":   "v3",
			"securityName":  "admin",
			"securityLevel": "authPriv",
			"authProtocol":  "INVALID",
			"authPassword":  "authpass123",
			"privPassword":  "privpass123",
			"ip":            "127.0.0.1",
		})
		_, err := tr.buildClient()
		require.Error(t, err)
	})
}

func TestDecoderEncodeCoverage(t *testing.T) {
	dec := NewSNMPDecoder()
	val, asn, err := dec.EncodeValue(42, "INT32")
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Integer, asn)
	assert.Equal(t, 42, val)

	_, _, err = dec.EncodeValue("bad", "INT32")
	require.Error(t, err)

	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: 7}, "INT32")
	assert.Equal(t, "Good", quality)
	assert.Equal(t, int32(7), val)

	val, quality = dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.IPAddress, Value: []byte{192, 168, 1, 1}}, "IP")
	assert.Equal(t, "Good", quality)
	assert.NotNil(t, val)
}

func TestTransportGetBulkAndNextHooks(t *testing.T) {
	tr := NewSNMPTransport(map[string]any{"ip": "127.0.0.1"})
	tr.connected.Store(true)
	tr.client = &gosnmp.GoSNMP{}

	tr.getHook = func(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
		return []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.Integer, Value: 1}}, nil
	}
	pdus, err := tr.Get([]string{"1.3.6.1.2.1.1.1.0"}, "public")
	require.NoError(t, err)
	require.Len(t, pdus, 1)

	tr.getHook = nil
	_, err = tr.GetBulk("1.3.6.1.2.1", "public", 10)
	require.Error(t, err)
}

func TestParseDeviceConfigCoverage(t *testing.T) {
	cfg := parseDeviceConfig(map[string]any{
		"ip":          "10.0.0.5",
		"port":        162,
		"timeout":     3000,
		"retries":     2,
		"version":     "v2c",
		"community":   "public",
		"maxBulkSize": 25,
	})
	assert.Equal(t, "10.0.0.5", cfg.TargetIP)
	assert.Equal(t, 25, cfg.MaxBulkSize)
	assert.False(t, cfg.isV3())
}

func TestCoverage_ParseAddress(t *testing.T) {
	dec := NewSNMPDecoder()
	v2cfg := parseDeviceConfig(map[string]any{"snmpVersion": "v2c"})

	addr, err := dec.ParseAddress("public|1.3.6.1.2.1.1.1.0", v2cfg)
	require.NoError(t, err)
	assert.Equal(t, "public", addr.Community)
	assert.Equal(t, "1.3.6.1.2.1.1.1.0", addr.OID)

	v3cfg := parseDeviceConfig(map[string]any{"snmpVersion": "v3"})
	addr, err = dec.ParseAddress("admin|1.3.6.1.4.1.1.0", v3cfg)
	require.NoError(t, err)
	assert.Equal(t, "admin", addr.SecurityName)

	invalid := []string{"", "only-oid", "public|", "public|not.oid"}
	for _, raw := range invalid {
		_, err = dec.ParseAddress(raw, v2cfg)
		require.Error(t, err, raw)
	}
}

func TestCoverage_ConfigHelperFunctions(t *testing.T) {
	assert.Equal(t, 42, intFromAny(float64(42), 0))
	assert.Equal(t, 7, intFromAny("7", 0))
	assert.Equal(t, 99, intFromAny(nil, 99))
	assert.Equal(t, "hello", stringFromAny(" hello "))
	assert.Equal(t, "10.0.0.1", firstNonEmpty("", "10.0.0.1"))
	assert.Equal(t, 200*time.Millisecond, millisFromAny(200, time.Second))
	assert.Equal(t, "127.0.0.1:161", parseDeviceConfig(nil).remoteAddr())
}

func TestCoverage_DecoderFloatAndCounter(t *testing.T) {
	dec := NewSNMPDecoder()

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, math.Float32bits(3.14))
	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: buf}, "FLOAT")
	assert.Equal(t, "Good", quality)
	assert.InDelta(t, 3.14, val.(float64), 0.01)

	val, quality = dec.DecodePDU(gosnmp.SnmpPDU{Type: gosnmp.Counter64, Value: uint64(999)}, "UINT64")
	assert.Equal(t, "Good", quality)
	assert.Equal(t, uint64(999), val)

	modelVal := dec.ToModelValue(model.Point{ID: "p1", DataType: "INT32"},
		gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: 5})
	assert.Equal(t, "p1", modelVal.PointID)
	assert.Equal(t, "Good", modelVal.Quality)
}

func TestCoverage_SchedulerStatsAfterRead(t *testing.T) {
	d := NewSNMPDriver().(*SNMPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"ip": "127.0.0.1", "community": "public"},
	}))
	d.transport.getHook = func(oids []string, _ string) ([]gosnmp.SnmpPDU, error) {
		return []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.Integer, Value: 1}}, nil
	}
	d.transport.connected.Store(true)
	d.transport.client = &gosnmp.GoSNMP{}

	ctx := context.Background()
	_, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "public|1.3.6.1.2.1.1.1.0", DataType: "INT32"},
	})
	require.NoError(t, err)

	total, success, failure := d.scheduler.GetStats()
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), success)
	assert.Equal(t, int64(0), failure)
}
