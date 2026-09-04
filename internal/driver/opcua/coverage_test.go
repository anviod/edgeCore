package opcua

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_ResolveEndpoint(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config:    map[string]any{"url": "opc.tcp://127.0.0.1:4840"},
	}))

	ep, err := d.resolveEndpointInConfig(map[string]any{})
	require.NoError(t, err)
	assert.Contains(t, ep, "4840")

	d2 := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d2.Init(model.DriverConfig{Config: map[string]any{}}))
	_, err = d2.resolveEndpointInConfig(map[string]any{})
	require.Error(t, err)
}

func TestCoverage_BuildClientOptions(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	opts, err := d.buildClientOptions(map[string]any{})
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestCoverage_DriverLifecycle(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config:    map[string]any{"url": "opc.tcp://127.0.0.1:4840"},
	}))

	assert.NoError(t, d.SetSlaveID(1))
	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"endpoint": "opc.tcp://127.0.0.1:4840",
	}))

	ctx := context.Background()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	health := d.Health()
	assert.NotEqual(t, driver.HealthStatus(-1), health)

	_, _, _, remote, _ := d.GetConnectionMetrics()
	assert.Contains(t, remote, "4840")

	results, err := d.ReadPoints(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, results)

	m := d.GetMetrics()
	assert.Equal(t, "OPC-UA", m.Protocol)
}

func TestCoverage_RTTAndQualityScore(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))

	d.recordRtt(10 * time.Millisecond)
	d.recordRtt(20 * time.Millisecond)
	avg, min, max := d.rttSnapshot()
	assert.Greater(t, avg, 0.0)
	assert.Greater(t, min, 0.0)
	assert.Greater(t, max, 0.0)

	// calculateQualityScore requires connected activeClient
	d.activeClient = &ClientWrapper{Connected: true, Endpoint: "opc.tcp://127.0.0.1:4840"}
	d.totalRequests = 10
	d.successCount = 9
	d.failureCount = 1
	score := d.calculateQualityScore()
	assert.Greater(t, score, 0)
	assert.LessOrEqual(t, score, 100)

	d.recordReadOutcome(time.Now(), true, "")
	d.recordReadOutcome(time.Now(), false, "timeout")
	d.recordReconnect()
}

func TestCoverage_ScanDefaultEndpoints(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{}}))
	result, err := d.Scan(context.Background(), nil)
	require.NoError(t, err)
	endpoints, ok := result.([]map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, endpoints)
}

func TestCoverage_ClassifyReadError(t *testing.T) {
	assert.Equal(t, "timeout", classifyOpcUaReadError(context.DeadlineExceeded))
	assert.Equal(t, "network", classifyOpcUaReadError(assert.AnError))
}

func TestCoverage_PureHelpers(t *testing.T) {
	assert.True(t, subscriptionCacheReady(model.Value{Quality: "Good", Value: 1}))
	assert.False(t, subscriptionCacheReady(model.Value{Quality: "Good", Value: nil}))
	assert.True(t, subscriptionCacheReady(model.Value{Quality: "Bad", Value: nil}))

	merged, toAdd, changed := mergeSubscriptionPoints(
		map[string]model.Point{"p1": {ID: "p1", Address: "ns=2;i=1"}},
		[]model.Point{
			{ID: "p1", Address: "ns=2;i=2"},
			{ID: "p2", Address: "ns=2;i=3"},
		},
	)
	assert.True(t, changed)
	assert.Len(t, merged, 2)
	assert.Len(t, toAdd, 1)

	ids := sortedPointIDs([]model.Point{{ID: "b"}, {ID: "a"}, {ID: "c"}})
	assert.Equal(t, []string{"a", "b", "c"}, ids)

	code, ok := statusCodeFromName("badtimeout")
	assert.True(t, ok)
	assert.Equal(t, ua.StatusBadTimeout, code)
	_, ok = statusCodeFromName("not-a-code")
	assert.False(t, ok)

	g := parseGuid("550e8400-e29b-41d4-a716-446655440000")
	require.NotNil(t, g)
}

func TestCoverage_ByteStringParsing(t *testing.T) {
	raw, err := decodeHexString("0x0102FF")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0xFF}, raw)

	raw, err = decodeHexString("ABC")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0A, 0xBC}, raw)

	raw, err = decodeBase64String("AQID")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, raw)

	raw, err = parseByteStringValue([]byte{0xAA})
	require.NoError(t, err)
	assert.Equal(t, []byte{0xAA}, raw)

	raw, err = parseByteStringValue(map[string]any{"encoding": "hex", "value": "0xFF"})
	require.NoError(t, err)
	assert.Equal(t, []byte{0xFF}, raw)

	_, err = parseByteStringValue(123)
	require.Error(t, err)
}

func TestCoverage_SetDeviceConfigDecoderFlag(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"},
	}))
	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"endpoint":               "opc.tcp://127.0.0.1:4840",
		"use_dataformat_decoder": "true",
	}))
	assert.True(t, d.useDataformatDecoder)

	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"endpoint":               "opc.tcp://127.0.0.1:4840",
		"use_dataformat_decoder": float64(0),
	}))
	assert.False(t, d.useDataformatDecoder)
}

func TestCoverage_DisconnectClearsClients(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))
	d.clients["opc.tcp://127.0.0.1:4840"] = &ClientWrapper{Endpoint: "opc.tcp://127.0.0.1:4840"}
	require.NoError(t, d.Disconnect())
	assert.Empty(t, d.clients)
}

func TestCoverage_ReadPointsSubscriptionCacheHit(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &ClientWrapper{
		Connected: true,
		connMgr:   driver.NewConnectionManager("cov-opcua"),
		Subscriptions: map[string]*DeviceSubscription{
			"dev-1": {
				Cache: map[string]model.Value{
					"p1": {PointID: "p1", Value: float32(3.14), Quality: "Good"},
					"p2": {PointID: "p2", Value: int32(42), Quality: "Good"},
				},
				Points: map[string]model.Point{
					"p1": {ID: "p1", DeviceID: "dev-1", Address: "ns=2;i=1"},
					"p2": {ID: "p2", DeviceID: "dev-1", Address: "ns=2;i=2"},
				},
				Ctx: subCtx,
			},
		},
	}
	w.connMgr.RecordSuccess()
	d.activeClient = w

	results, err := d.ReadPoints(context.Background(), []model.Point{
		{ID: "p1", DeviceID: "dev-1", Address: "ns=2;i=1", DataType: "float32"},
		{ID: "p2", DeviceID: "dev-1", Address: "ns=2;i=2", DataType: "int32"},
	})
	require.NoError(t, err)
	assert.Equal(t, float32(3.14), results["p1"].Value)
	assert.Equal(t, int32(42), results["p2"].Value)
	assert.Equal(t, "Good", results["p1"].Quality)
}

func TestCoverage_ReadPointsNoActiveClient(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))
	_, err := d.ReadPoints(context.Background(), []model.Point{{ID: "p1", DeviceID: "d1", Address: "ns=2;i=1"}})
	require.Error(t, err)
}

func TestCoverage_PointsMapHelpers(t *testing.T) {
	merged := map[string]model.Point{
		"b": {ID: "b", Address: "ns=1;i=2"},
		"a": {ID: "a", Address: "ns=1;i=1"},
	}
	pts := pointsFromMap(merged)
	require.Len(t, pts, 2)
	assert.Equal(t, "a", pts[0].ID)
	assert.Equal(t, []string{"a", "b"}, sortedPointIDsFromMap(merged))
}

func TestCoverage_ParseWriteValueExtended(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)

	dt := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	val, err := d.parseWriteValue("2024-06-01T12:00:00Z", "datetime")
	require.NoError(t, err)
	assert.Equal(t, dt, val.(time.Time))

	val, err = d.parseWriteValue("good", "statuscode")
	require.NoError(t, err)
	assert.Equal(t, ua.StatusGood, val)

	val, err = d.parseWriteValue("2:TagName", "qualifiedname")
	require.NoError(t, err)
	qn := val.(ua.QualifiedName)
	assert.Equal(t, uint16(2), qn.NamespaceIndex)
	assert.Equal(t, "TagName", qn.Name)

	val, err = d.parseWriteValue("hello", "localizedtext")
	require.NoError(t, err)
	lt := val.(ua.LocalizedText)
	assert.Equal(t, "hello", lt.Text)

	val, err = d.parseWriteValue("AQID", "extensionobject")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, val)

	val, err = d.parseWriteValue(float64(170), "byte")
	require.NoError(t, err)
	assert.Equal(t, uint8(170), val)

	_, err = d.parseWriteValue("not-a-date", "datetime")
	require.Error(t, err)
}

func TestCoverage_CreateWriteVariantExtended(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)

	cases := []struct {
		dt  string
		val any
	}{
		{"int16", int16(-100)},
		{"uint16", uint16(65000)},
		{"int64", int64(-999999)},
		{"uint64", uint64(999999)},
		{"double", float64(2.718)},
		{"sbyte", int8(-3)},
		{"boolean", true},
		{"guid", "550e8400-e29b-41d4-a716-446655440000"},
		{"statuscode", "badtimeout"},
		{"qualifiedname", "1:MyTag"},
		{"localizedtext", map[string]any{"text": "hi", "locale": "zh"}},
		{"nodeid", "ns=2;i=100"},
		{"bytestring", []byte{0xAA, 0xBB}},
	}
	for _, tc := range cases {
		v := d.createWriteVariant(tc.dt, tc.val)
		require.NotNil(t, v, tc.dt)
	}

	assert.NotNil(t, d.createWriteVariant("unknown-type", "fallback"))
}

func TestCoverage_GetConnectionMetricsConnected(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "metrics",
		Config:    map[string]any{"url": "opc.tcp://127.0.0.1:4840"},
	}))

	w := &ClientWrapper{
		Connected: true,
		Endpoint:  "opc.tcp://127.0.0.1:4840",
		connMgr:   driver.NewConnectionManager("metrics"),
	}
	w.connMgr.RecordSuccess()
	d.activeClient = w
	d.connectionStartTime = time.Now().Add(-5 * time.Second)
	d.reconnectCount = 2

	sec, recon, _, remote, _ := d.GetConnectionMetrics()
	assert.GreaterOrEqual(t, sec, int64(4))
	assert.Equal(t, int64(2), recon)
	assert.Contains(t, remote, "4840")
}

func TestCoverage_SetDeviceConfigFull(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840", "security_mode": "None"},
	}))
	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"endpoint":               "opc.tcp://192.168.1.50:4840",
		"security_mode":          "Sign",
		"use_dataformat_decoder": true,
		"subscription_interval":  float64(500),
		"max_nodes_per_read":     float64(50),
	}))
	assert.True(t, d.useDataformatDecoder)
}

func TestCoverage_CastValueNumericTypes(t *testing.T) {
	v, err := castValue(float64(100), "uint32")
	require.NoError(t, err)
	assert.Equal(t, uint32(100), v)

	v, err = castValue("1", "bool")
	require.NoError(t, err)
	assert.True(t, v.(bool))

	v, err = castValue(int8(-5), "sbyte")
	require.NoError(t, err)
	assert.Equal(t, int8(-5), v)

	_, err = castValue("xyz", "int16")
	require.Error(t, err)
}

func TestCoverage_WritePointNoClient(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))
	err := d.WritePoint(context.Background(), model.Point{Address: "ns=2;i=1", DataType: "int32"}, int32(1))
	require.Error(t, err)
}

func TestCoverage_ScanWithBadEndpoint(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))
	_, err := d.Scan(context.Background(), map[string]any{"endpoint": "opc.tcp://192.168.255.255:4840"})
	require.Error(t, err)
}

func TestCoverage_RecordReconnect(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{ChannelID: "recon", Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))
	d.recordReconnect()
	assert.Equal(t, int64(1), d.reconnectCount)
}
