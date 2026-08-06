package opcua

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_AllVariantCreators(t *testing.T) {
	assert.NotNil(t, createInt32Variant(int32(1)))
	assert.NotNil(t, createInt32Variant("42"))
	assert.Nil(t, createInt32Variant(struct{}{}))

	assert.NotNil(t, createInt16Variant(int16(-5)))
	assert.NotNil(t, createInt16Variant("10"))

	assert.NotNil(t, createUint32Variant(uint32(99)))
	assert.NotNil(t, createUint16Variant(uint16(500)))
	assert.NotNil(t, createInt64Variant(int64(-999)))
	assert.NotNil(t, createUint64Variant(uint64(999)))

	assert.NotNil(t, createFloat32Variant(float32(1.5)))
	assert.NotNil(t, createFloat32Variant(float64(2.5)))
	assert.NotNil(t, createFloat64Variant(float64(3.14)))

	assert.NotNil(t, createBooleanVariant(true))
	assert.NotNil(t, createBooleanVariant("true"))
	assert.NotNil(t, createSByteVariant(int8(-1)))
	assert.NotNil(t, createByteVariant(uint8(255)))

	assert.NotNil(t, createDateTimeVariant(time.Now()))
	assert.NotNil(t, createDateTimeVariant("2024-01-01T00:00:00Z"))

	assert.NotNil(t, createGuidVariant("550e8400-e29b-41d4-a716-446655440000"))
	assert.NotNil(t, createByteStringVariant([]byte{0x01}))
	assert.NotNil(t, createNodeIDVariant("ns=2;i=100"))
	assert.NotNil(t, createStatusCodeVariant("good"))
	assert.NotNil(t, createQualifiedNameVariant("1:Tag"))
	assert.NotNil(t, createLocalizedTextVariant(map[string]any{"text": "hi"}))
	assert.NotNil(t, createExtensionObjectVariant([]byte{0xAA}))
}

func TestCoverage_CreateArrayVariant(t *testing.T) {
	v := createArrayVariant("array:int32", []any{int32(1), int32(2)})
	require.NotNil(t, v)

	v = createArrayVariant("[]float", []float64{1.1, 2.2})
	require.NotNil(t, v)

	v = createArrayVariant("array:bool", []bool{true, false})
	require.NotNil(t, v)
}

func TestCoverage_LookupHelpers(t *testing.T) {
	assert.Contains(t, lookupAccessLevel(0x03), "CurrentRead")
	assert.Contains(t, lookupAccessLevel(0x03), "CurrentWrite")
	assert.Contains(t, lookupAccessLevel(0xFF), "TimestampWrite")

	assert.Equal(t, "Boolean", lookupDataType(ua.NewNumericNodeID(0, 1)))
	assert.Equal(t, "Double", lookupDataType(ua.NewNumericNodeID(0, 11)))
	assert.Contains(t, lookupDataType(ua.NewNumericNodeID(2, 99)), "ns=2")

	alts := getAlternativeTypes("int32")
	assert.NotEmpty(t, alts)
	alts = getAlternativeTypes("unknown")
	assert.Empty(t, alts)
}

func TestCoverage_ConnErrorClassifiers(t *testing.T) {
	assert.True(t, isOPCUAConnError(io.EOF))
	assert.True(t, isOpcUaSessionError(context.DeadlineExceeded))
	assert.True(t, isOpcUaSessionError(fmt.Errorf("BadSessionIDInvalid")))
	assert.False(t, isOPCUAConnError(nil))
}

func TestCoverage_NormalizeNodeID(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	id := ua.NewStringNodeID(2, "TagName")
	out := d.normalizeNodeID("ns=2;s=TagName", id)
	require.NotNil(t, out)
	assert.Equal(t, uint16(2), out.Namespace())
}

func TestCoverage_StampCollectionTime(t *testing.T) {
	now := time.Now()
	results := map[string]model.Value{
		"p1": {PointID: "p1", Value: 1, Quality: "Good"},
	}
	stampCollectionTime(results)
	assert.False(t, results["p1"].TS.Before(now))
}

func TestCoverage_ResetDeviceSubscription(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	require.NoError(t, d.Init(model.DriverConfig{Config: map[string]any{"url": "opc.tcp://127.0.0.1:4840"}}))

	subCtx, cancel := context.WithCancel(context.Background())
	w := &ClientWrapper{
		Subscriptions: map[string]*DeviceSubscription{
			"dev-1": {Ctx: subCtx, Cancel: cancel},
		},
	}
	d.activeClient = w
	d.ResetDeviceCollection("dev-1")
}

func TestCoverage_CancelDeviceSubscription(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	subCtx, cancel := context.WithCancel(context.Background())
	w := &ClientWrapper{
		Subscriptions: map[string]*DeviceSubscription{
			"dev-1": {Ctx: subCtx, Cancel: cancel},
		},
	}
	d.cancelDeviceSubscription(w, "dev-1")
	assert.Empty(t, w.Subscriptions)
}

func TestCoverage_CreateWriteVariantNumericIDs(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	vals := map[string]any{
		"i=6": 1, "i=4": 1, "i=7": uint32(1), "i=5": uint16(1),
		"i=10": float32(1), "i=11": float64(1), "i=1": true, "i=2": int8(1), "i=3": uint8(1),
		"i=12": "s", "i=13": time.Now(), "i=14": "550e8400-e29b-41d4-a716-446655440000",
		"i=15": []byte{1}, "i=17": "ns=2;i=1", "i=19": "good", "i=20": "1:T", "i=21": "txt", "i=22": []byte{1},
	}
	for dt, val := range vals {
		v := d.createWriteVariant(dt, val)
		require.NotNil(t, v, dt)
	}
	assert.NotNil(t, d.createWriteVariant("array:int32", []int32{1, 2}))
}

func TestCoverage_ParseWriteValueAllBranches(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)

	val, err := d.parseWriteValue("ns=2;i=100", "nodeid")
	require.NoError(t, err)
	assert.NotNil(t, val)

	val, err = d.parseWriteValue([]byte{0x01, 0x02}, "bytestring")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02}, val)

	val, err = d.parseWriteValue(map[string]any{"text": "ok", "locale": "en"}, "localizedtext")
	require.NoError(t, err)
	assert.NotNil(t, val)

	_, err = d.parseWriteValue("", "guid")
	require.NoError(t, err)
}

func TestCoverage_ParseWriteValueExhaustive(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)

	val, err := d.parseWriteValue("AQID", "bytestring")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, val)

	val, err = d.parseWriteValue(map[string]any{"encoding": "hex", "value": "0xFF"}, "bytestring")
	require.NoError(t, err)
	assert.Equal(t, []byte{0xFF}, val)

	val, err = d.parseWriteValue(int64(1700000000), "datetime")
	require.NoError(t, err)
	assert.IsType(t, time.Time{}, val)

	val, err = d.parseWriteValue(uint32(0), "statuscode")
	require.NoError(t, err)
	assert.Equal(t, ua.StatusCode(0), val)

	val, err = d.parseWriteValue(map[string]any{"namespace": float64(2), "name": "Tag"}, "qualifiedname")
	require.NoError(t, err)
	assert.Equal(t, uint16(2), val.(ua.QualifiedName).NamespaceIndex)

	_, err = d.parseWriteValue(struct{}{}, "nodeid")
	require.Error(t, err)

	val, err = d.parseWriteValue("raw", "extensionobject")
	require.NoError(t, err)
	assert.Equal(t, []byte("raw"), val)
}

func TestCoverage_CastValueAllTypes(t *testing.T) {
	v, err := castValue("255", "byte")
	require.NoError(t, err)
	assert.Equal(t, uint8(255), v)

	v, err = castValue(float64(100), "uint16")
	require.NoError(t, err)
	assert.Equal(t, uint16(100), v)

	v, err = castValue(float64(-50), "int16")
	require.NoError(t, err)
	assert.Equal(t, int16(-50), v)

	v, err = castValue(float64(3.14), "float")
	require.NoError(t, err)
	assert.InDelta(t, float32(3.14), v, 0.01)

	v, err = castValue("1", "boolean")
	require.NoError(t, err)
	assert.True(t, v.(bool))

	assert.Equal(t, "int32", normalizeOpcUaDataType("i=6"))
	assert.Equal(t, "boolean", normalizeOpcUaDataType("1"))
	assert.Equal(t, "custom", normalizeOpcUaDataType("custom"))
}

func TestCoverage_ClientWrapperProbe(t *testing.T) {
	w := &ClientWrapper{
		Connected:    true,
		connMgr:      driver.NewConnectionManager("probe"),
		collectCycle: time.Second,
	}
	w.lastActivityTime.Store(time.Now().Add(-10 * time.Minute))
	assert.True(t, w.NeedProbeCheck())
	w.RecordSuccess()
	assert.False(t, w.NeedProbeCheck())
	w.RecordFailure(fmt.Errorf("connection reset"))
}
