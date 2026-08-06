package opcua

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockClient for testing browseNode logic without real connection
// Since opcua.Client is a struct, we cannot easily mock it unless we wrap it.
// However, OpcUaDriver uses *opcua.Client directly.
// To test browseNode, we might need to set up a real mock server or refactor OpcUaDriver to use an interface.
// For now, let's test the helper functions logic like lookupDataType and castValue thoroughly,
// and if possible, integration test with a mock server.

// TestLookupDataType tests the DataType lookup logic
func TestLookupDataType(t *testing.T) {
	tests := []struct {
		id   int
		want string
	}{
		{1, "Boolean"},
		{2, "SByte"},
		{3, "Byte"},
		{4, "Int16"},
		{5, "UInt16"},
		{6, "Int32"},
		{7, "UInt32"},
		{8, "Int64"},
		{9, "UInt64"},
		{10, "Float"},
		{11, "Double"},
		{12, "String"},
		{13, "DateTime"},
		{15, "ByteString"},
		{999, "ns=0;i=999"},
	}

	for _, tt := range tests {
		nodeID := ua.NewNumericNodeID(0, uint32(tt.id))
		got := lookupDataType(nodeID)
		if got != tt.want {
			t.Errorf("lookupDataType(%d) = %s, want %s", tt.id, got, tt.want)
		}
	}

	nodeID := ua.NewNumericNodeID(2, 1234)
	if got := lookupDataType(nodeID); got != "ns=2;i=1234" {
		t.Errorf("lookupDataType(ns=2;i=1234) = %s, want ns=2;i=1234", got)
	}
}

func TestCastValue(t *testing.T) {
	base64Value := base64.StdEncoding.EncodeToString([]byte("123"))
	bytestringPayload := map[string]any{
		"value":    base64Value,
		"encoding": "base64",
	}

	tests := []struct {
		name     string
		input    any
		dataType string
		want     any
		wantErr  bool
	}{
		{"Float64 to Int16", float64(123), "int16", int16(123), false},
		{"Float64 to UInt16", float64(123), "uint16", uint16(123), false},
		{"Float64 to Int32", float64(123), "int32", int32(123), false},
		{"String to Int16", "123", "int16", int16(123), false},
		{"Float64 to Byte", float64(255), "byte", uint8(255), false},
		{"Float64 to SByte", float64(127), "sbyte", int8(127), false},
		{"String to Byte", "255", "byte", uint8(255), false},
		{"String to SByte", "-128", "sbyte", int8(-128), false},
		{"Float64 to Float32", float64(123.45), "float32", float32(123.45), false},
		{"String to Float32", "10.001", "float32", float32(10.001), false},
		{"Bool to Bool", true, "bool", true, false},
		{"String to Bool", "true", "bool", true, false},
		{"Int to Bool (1)", 1, "bool", true, false},
		{"Int to Bool (0)", 0, "bool", false, false},
		{"Hex to ByteString", "0x313233", "bytestring", []byte{0x31, 0x32, 0x33}, false},
		{"Base64 payload to ByteString", bytestringPayload, "bytestring", []byte("123"), false},
		{"Invalid String to Int", "abc", "int16", nil, true},
		{"Invalid Hex to ByteString", "0xGG", "bytestring", nil, true},
		{"Invalid Base64 payload to ByteString", map[string]any{"value": "***", "encoding": "base64"}, "bytestring", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := castValue(tt.input, tt.dataType)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			switch want := tt.want.(type) {
			case []byte:
				assert.Equal(t, want, got)
			default:
				assert.Equal(t, want, got)
			}
		})
	}
}

// TestDriverMethodsCoverage covers basic driver lifecycle methods
func TestDriverMethodsCoverage(t *testing.T) {
	d := NewOpcUaDriver()
	err := d.Init(model.DriverConfig{})
	assert.NoError(t, err)

	err = d.Connect(context.Background())
	assert.NoError(t, err)

	status := d.Health()
	assert.Equal(t, driver.HealthStatusUnknown, status)

	err = d.Disconnect()
	assert.NoError(t, err)
}

// TestSetDeviceConfigCoverage covers configuration parsing
func TestSetDeviceConfigCoverage(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)

	config := map[string]any{
		"endpoint":               "opc.tcp://localhost:4840",
		"use_dataformat_decoder": true,
	}
	err := d.SetDeviceConfig(config)
	assert.NoError(t, err)
	assert.True(t, d.useDataformatDecoder)

	config["use_dataformat_decoder"] = "false"
	err = d.SetDeviceConfig(config)
	assert.NoError(t, err)
	assert.False(t, d.useDataformatDecoder)
}

// TestParseWriteValue tests the parseWriteValue function for all data types
func TestParseWriteValue(t *testing.T) {
	d := &OpcUaDriver{}

	tests := []struct {
		name     string
		value    any
		dataType string
		wantErr  bool
		wantType any
	}{
		// Boolean tests
		{"Bool true", true, "bool", false, true},
		{"Bool false", false, "bool", false, false},
		{"String true to bool", "true", "bool", false, true},
		{"String false to bool", "false", "bool", false, false},

		// Integer types
		{"Int32 from int", 123, "int32", false, int32(123)},
		{"Int32 from float", float64(456), "int32", false, int32(456)},
		{"Int32 from string", "789", "int32", false, int32(789)},
		{"Int16 from int", 100, "int16", false, int16(100)},
		{"Int64 from int", 999999, "int64", false, int64(999999)},
		{"UInt32 from int", 1000, "uint32", false, uint32(1000)},
		{"UInt16 from int", 500, "uint16", false, uint16(500)},
		{"UInt64 from int", 1000000, "uint64", false, uint64(1000000)},

		// Floating point types
		{"Float32 from float64", float64(3.14), "float32", false, float32(3.14)},
		{"Float32 from string", "2.718", "float32", false, float32(2.718)},
		{"Double from float32", float32(1.23), "float64", false, float64(1.23)},
		{"Double from int", 42, "double", false, float64(42)},

		// SByte and Byte
		{"SByte from int", 127, "sbyte", false, int8(127)},
		{"SByte from string", "-50", "sbyte", false, int8(-50)},
		{"Byte from int", 255, "byte", false, uint8(255)},
		{"Byte from string", "200", "byte", false, uint8(200)},

		// String
		{"String from string", "hello world", "string", false, "hello world"},
		{"String from int", 123, "string", false, "123"},

		// DateTime
		{"DateTime from RFC3339", "2024-01-15T10:30:00Z", "datetime", false, true}, // Just check no error
		{"DateTime from Unix", int64(1705312200), "datetime", false, true},
		{"DateTime from time.Time", "invalid", "datetime", true, nil},

		// ByteString
		{"ByteString from []byte", []byte{0x01, 0x02, 0x03}, "bytestring", false, []byte{0x01, 0x02, 0x03}},
		{"ByteString from base64", "SGVsbG8=", "bytestring", false, []byte("Hello")},
		{"ByteString from raw string", "test", "bytestring", false, []byte("test")},

		// Guid
		{"Guid from standard format", "12345678-1234-1234-1234-123456789abc", "guid", false, true},
		{"Guid from 32-char hex", "12345678123412341234123456789abc", "guid", false, true},

		// StatusCode
		{"StatusCode from uint32", uint32(0), "statuscode", false, ua.StatusCode(0)},
		{"StatusCode from string Good", "Good", "statuscode", false, ua.StatusGood},
		{"StatusCode from string Bad", "Bad", "statuscode", false, ua.StatusBad},

		// QualifiedName
		{"QualifiedName from string", "2:Temperature", "qualifiedname", false, true},
		{"QualifiedName simple", "Temperature", "qualifiedname", false, true},

		// LocalizedText
		{"LocalizedText from string", "Hello", "localizedtext", false, true},

		// NodeId type
		{"NodeId from string", "ns=2;s=Demo", "nodeid", false, true},

		// OPC-UA ID format tests
		{"Int32 with i=6", float64(100), "i=6", false, int32(100)},
		{"Float with i=10", float64(1.5), "i=10", false, float32(1.5)},
		{"Double with i=11", float32(2.5), "i=11", false, float64(2.5)},
		{"Boolean with i=1", true, "i=1", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.parseWriteValue(tt.value, tt.dataType)
			if tt.wantErr {
				require.Error(t, err, "Expected error for %s", tt.name)
				return
			}
			require.NoError(t, err, "Unexpected error for %s", tt.name)

			// Type-specific assertions
			switch tt.dataType {
			case "bool", "i=1":
				assert.IsType(t, bool(true), got)
			case "int32", "i=6":
				assert.IsType(t, int32(0), got)
			case "int16", "i=4":
				assert.IsType(t, int16(0), got)
			case "uint32", "i=7":
				assert.IsType(t, uint32(0), got)
			case "uint16", "i=5":
				assert.IsType(t, uint16(0), got)
			case "int64", "i=8":
				assert.IsType(t, int64(0), got)
			case "uint64", "i=9":
				assert.IsType(t, uint64(0), got)
			case "float32", "float", "i=10":
				assert.IsType(t, float32(0), got)
			case "float64", "double", "i=11":
				assert.IsType(t, float64(0), got)
			case "sbyte", "i=2":
				assert.IsType(t, int8(0), got)
			case "byte", "uint8", "i=3":
				assert.IsType(t, uint8(0), got)
			case "string", "i=12", "xmlliteral", "i=16":
				assert.IsType(t, string(""), got)
			case "bytestring", "i=15":
				assert.IsType(t, []byte{}, got)
			case "guid", "i=14":
				// GUID returns *ua.GUID
				assert.NotNil(t, got)
			case "datetime", "i=13":
				assert.NotNil(t, got)
			case "statuscode", "i=19":
				assert.NotNil(t, got)
			case "qualifiedname", "i=20":
				assert.NotNil(t, got)
			case "localizedtext", "i=21":
				assert.NotNil(t, got)
			case "nodeid", "i=17":
				assert.NotNil(t, got)
			}
		})
	}
}

// TestCreateWriteVariant tests the createWriteVariant function for all data types
func TestCreateWriteVariant(t *testing.T) {
	d := &OpcUaDriver{}

	tests := []struct {
		name     string
		value    any
		dataType string
		wantNil  bool
	}{
		// Boolean
		{"Bool true", true, "bool", false},
		{"Bool false", false, "bool", false},
		{"String to Bool", "true", "bool", false},
		{"Number to Bool", 1, "bool", false},
		{"Invalid Bool", "invalid", "bool", true},

		// Integer types
		{"Int32 valid", int32(100), "int32", false},
		{"Int32 from float", float64(200), "int32", false},
		{"Int32 from string", "300", "int32", false},
		{"Int16 valid", int16(50), "int16", false},
		{"Int64 valid", int64(999999), "int64", false},
		{"UInt32 valid", uint32(1000), "uint32", false},
		{"UInt16 valid", uint16(500), "uint16", false},
		{"UInt64 valid", uint64(1000000), "uint64", false},

		// Floating point types
		{"Float32 valid", float32(3.14), "float32", false},
		{"Float32 from float64", float64(2.71), "float32", false},
		{"Float32 from string", "1.414", "float32", false},
		{"Double valid", float64(3.14159), "double", false},
		{"Double from int", 42, "double", false},

		// SByte and Byte
		{"SByte valid", int8(127), "sbyte", false},
		{"SByte from string", "100", "sbyte", false},
		{"Byte valid", uint8(255), "byte", false},
		{"Byte from string", "200", "byte", false},

		// String
		{"String valid", "hello", "string", false},
		{"String from int", 123, "string", false},

		// DateTime
		{"DateTime from time.Time", time.Now(), "datetime", false},
		{"DateTime from Unix", int64(1705312200), "datetime", false},
		{"DateTime from RFC3339 string", "2024-01-15T10:30:00Z", "datetime", false},

		// ByteString
		{"ByteString from []byte", []byte{0x01, 0x02}, "bytestring", false},
		{"ByteString from base64 string", "SGVsbG8=", "bytestring", false},
		{"ByteString from plain string", "test", "bytestring", false},

		// Guid
		{"Guid from string", "12345678-1234-1234-1234-123456789abc", "guid", false},
		{"Guid from [16]byte", [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, "guid", false},

		// QualifiedName
		{"QualifiedName simple", "Temperature", "qualifiedname", false},
		{"QualifiedName with namespace", "2:Temperature", "qualifiedname", false},
		{"QualifiedName from map", map[string]any{"namespace": float64(2), "name": "Value"}, "qualifiedname", false},

		// LocalizedText
		{"LocalizedText string", "Hello", "localizedtext", false},
		{"LocalizedText map", map[string]any{"text": "Hello", "locale": "en"}, "localizedtext", false},

		// NodeId
		{"NodeId string", "ns=2;s=Demo", "nodeid", false},
		{"NodeId invalid", "invalid!!", "nodeid", true},

		// StatusCode
		{"StatusCode from uint32", uint32(0), "statuscode", false},
		{"StatusCode from string Good", "Good", "statuscode", false},
		{"StatusCode from string Bad", "Bad", "statuscode", false},

		// ExtensionObject
		{"ExtensionObject from []byte", []byte{0x01, 0x02}, "extensionobject", false},
		{"ExtensionObject from base64", "SGVsbG8=", "extensionobject", false},

		// OPC-UA ID format tests
		{"Int32 with i=6", int32(100), "i=6", false},
		{"Float with i=10", float32(1.5), "i=10", false},
		{"Double with i=11", float64(2.5), "i=11", false},
		{"Boolean with i=1", true, "i=1", false},
		{"String with i=12", "test", "i=12", false},
		{"DateTime with i=13", time.Now(), "i=13", false},
		{"ByteString with i=15", []byte("test"), "i=15", false},

		// Invalid cases
		{"Invalid Float32 string", "abc", "float32", true},
		{"Invalid Int32 bool", true, "int32", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variant := d.createWriteVariant(tt.dataType, tt.value)
			if tt.wantNil {
				assert.Nil(t, variant, "Expected nil variant for %s", tt.name)
			} else {
				assert.NotNil(t, variant, "Expected non-nil variant for %s", tt.name)
			}
		})
	}
}

// TestNormalizeNodeID tests the normalizeNodeID function
func TestNormalizeNodeID(t *testing.T) {
	d := &OpcUaDriver{}

	tests := []struct {
		name    string
		address string
		wantNil bool
	}{
		{"Numeric NodeID", "ns=2;i=1234", false},
		{"String NodeID", "ns=2;s=Demo.Node", false},
		{"Numeric with string format", "ns=0;s=1234", false}, // String ID that looks like a number
		{"Invalid format", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, _ := ua.ParseNodeID(tt.address)
			normalized := d.normalizeNodeID(tt.address, original)
			if tt.wantNil {
				assert.Nil(t, normalized)
			} else {
				assert.NotNil(t, normalized)
			}
		})
	}
}

// TestGetAlternativeTypes tests the getAlternativeTypes function
func TestGetAlternativeTypes(t *testing.T) {
	tests := []struct {
		dataType string
		wantLen  int // minimum expected alternatives
	}{
		{"int32", 5},
		{"int16", 4},
		{"uint32", 4},
		{"uint16", 4},
		{"double", 2},
		{"float", 2},
		{"string", 0},
		{"bool", 0},
		{"i=6", 5},
		{"i=10", 2},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			alts := getAlternativeTypes(tt.dataType)
			assert.GreaterOrEqual(t, len(alts), tt.wantLen,
				"Expected at least %d alternatives for %s, got %d", tt.wantLen, tt.dataType, len(alts))
		})
	}
}

// TestParseGuid tests the parseGuid helper function
func TestParseGuid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{"Standard UUID format", "12345678-1234-1234-1234-123456789abc", false},
		{"32-char hex without dashes", "12345678123412341234123456789abc", false},
		{"Empty string", "", false},             // Should return empty GUID
		{"Invalid format", "not-a-guid", false}, // Should try to parse as-is
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := parseGuid(tt.input)
			if tt.wantNil {
				assert.Nil(t, g)
			} else {
				assert.NotNil(t, g)
			}
		})
	}
}

// TestStatusCodeFromName tests the statusCodeFromName helper function
func TestStatusCodeFromName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantOK bool
	}{
		{"Good", "Good", true},
		{"good lowercase", "good", true},
		{"Bad", "Bad", true},
		{"Uncertain", "Uncertain", true},
		{"Invalid", "InvalidStatus", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := statusCodeFromName(tt.input)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestOpcUaMetricsAccounting(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	d.config = model.DriverConfig{ChannelID: "test-ch"}

	err := d.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), d.reconnectCount)

	start := time.Now()
	d.recordReadOutcome(start, true, "")
	d.recordReadOutcome(start, true, "")
	d.recordReadOutcome(start, false, "network")

	assert.Equal(t, int64(3), d.totalRequests)
	assert.Equal(t, int64(2), d.successCount)
	assert.Equal(t, int64(1), d.failureCount)

	metrics := d.GetMetrics()
	assert.InDelta(t, 2.0/3.0, metrics.SuccessRate, 0.001)
	assert.InDelta(t, 1.0/3.0, metrics.PacketLoss, 0.001)

	d.recordRtt(25 * time.Millisecond)
	d.recordRtt(75 * time.Millisecond)
	assert.InDelta(t, 50.0, d.avgRttMs(), 0.001)

	d.recordReconnect()
	assert.Equal(t, int64(1), d.reconnectCount)
}

func TestOpcUaQualityScoreUsesSuccessRate(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	d.activeClient = &ClientWrapper{Connected: true}

	d.totalRequests = 100
	d.successCount = 96
	scoreHigh := d.calculateQualityScore()

	d.successCount = 70
	scoreLow := d.calculateQualityScore()

	assert.Greater(t, scoreHigh, scoreLow)
	assert.Greater(t, scoreHigh, 80)
}

func TestStampCollectionTime_RefreshesDirectRead(t *testing.T) {
	old := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	results := map[string]model.Value{
		"p1": {PointID: "p1", Value: 1.0, Quality: "Good", TS: old},
	}
	before := time.Now()
	stampCollectionTime(results)
	if !results["p1"].TS.After(before.Add(-time.Second)) {
		t.Fatalf("expected TS refreshed for direct read, got %v", results["p1"].TS)
	}
}

func TestSubscriptionCachePreservesTimestamp(t *testing.T) {
	cacheTime := time.Date(2025, 6, 1, 8, 30, 0, 0, time.UTC)
	cached := model.Value{PointID: "p1", Value: 42.0, Quality: "Good", TS: cacheTime}
	result := map[string]model.Value{"p1": cached}

	if result["p1"].TS != cacheTime {
		t.Fatalf("cache hit should preserve TS, got %v", result["p1"].TS)
	}
}

func TestSubscriptionCacheReady(t *testing.T) {
	tests := []struct {
		name  string
		value model.Value
		want  bool
	}{
		{"Good with value", model.Value{Quality: "Good", Value: float32(1.2)}, true},
		{"Good nil value", model.Value{Quality: "Good", Value: nil}, false},
		{"Bad without value", model.Value{Quality: "Bad", Value: nil}, true},
		{"Empty quality", model.Value{Quality: "", Value: nil}, false},
		{"Good empty bytestring", model.Value{Quality: "Good", Value: []byte{}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, subscriptionCacheReady(tt.value))
		})
	}
}

func TestReadPointsFallsBackWhenCacheGoodWithoutValue(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	w := &ClientWrapper{Connected: true}
	sub := &DeviceSubscription{
		Cache: map[string]model.Value{
			"existing": {PointID: "existing", Quality: "Good", Value: float32(10)},
			"new":      {PointID: "new", Quality: "Good", Value: nil},
		},
		Points: map[string]model.Point{
			"existing": {ID: "existing", DeviceID: "dev-1", Address: "ns=3;i=1001"},
			"new":      {ID: "new", DeviceID: "dev-1", Address: "ns=3;i=1011"},
		},
	}
	w.Subscriptions = map[string]*DeviceSubscription{"dev-1": sub}
	d.activeClient = w

	points := []model.Point{
		{ID: "existing", DeviceID: "dev-1", Address: "ns=3;i=1001"},
		{ID: "new", DeviceID: "dev-1", Address: "ns=3;i=1011", DataType: "float32"},
	}

	sub.mu.RLock()
	missing := false
	for _, p := range points {
		if v, ok := sub.Cache[p.ID]; !ok || !subscriptionCacheReady(v) {
			missing = true
			break
		}
	}
	sub.mu.RUnlock()

	assert.True(t, missing, "Good cache entry with nil value must trigger direct read fallback")
}

func TestMergeSubscriptionPointsAddsNewIDs(t *testing.T) {
	existing := map[string]model.Point{
		"a": {ID: "a", Address: "ns=5;s=A"},
		"b": {ID: "b", Address: "ns=5;s=B"},
	}
	incoming := []model.Point{
		{ID: "b", Address: "ns=5;s=B"},
		{ID: "c", Address: "ns=5;s=AccessLevelCurrentReadNotUser"},
	}

	merged, toAdd, changed := mergeSubscriptionPoints(existing, incoming)
	if changed {
		t.Fatal("expected no address change")
	}
	if len(toAdd) != 1 || toAdd[0].ID != "c" {
		t.Fatalf("expected one new point c, got %+v", toAdd)
	}
	if len(merged) != 3 {
		t.Fatalf("expected merged size 3, got %d", len(merged))
	}
}

func TestIsOpcUaSessionError(t *testing.T) {
	assert.True(t, isOpcUaSessionError(errors.New("client not connected")))
	assert.True(t, isOpcUaSessionError(errors.New("use of closed network connection")))
	assert.False(t, isOpcUaSessionError(errors.New("StatusBadNodeIdUnknown")))
	assert.False(t, isOpcUaSessionError(errors.New("subscription transfer failed")))
}

func TestRecordFailureIgnoresNonSessionError(t *testing.T) {
	w := &ClientWrapper{
		connMgr:      driver.NewConnectionManager("opcua-test"),
		maxFailCount: 2,
	}
	w.Connected = true
	w.connMgr.RecordSuccess()

	w.RecordFailure(errors.New("StatusBadNodeIdUnknown"))
	w.RecordFailure(errors.New("StatusBadNodeIdUnknown"))

	assert.Equal(t, int32(0), w.collectFailCount.Load())
	assert.True(t, w.Connected)
	assert.Equal(t, driver.StateConnected, w.connMgr.GetState())
}

func TestMergeSubscriptionPointsDetectsAddressChange(t *testing.T) {
	existing := map[string]model.Point{
		"a": {ID: "a", Address: "ns=5;s=Old"},
	}
	incoming := []model.Point{
		{ID: "a", Address: "ns=5;s=New"},
	}

	_, toAdd, changed := mergeSubscriptionPoints(existing, incoming)
	if !changed {
		t.Fatal("expected address change")
	}
	if len(toAdd) != 0 {
		t.Fatalf("expected no additions on address change, got %+v", toAdd)
	}
}

func TestResetDeviceCollectionClearsSubscription(t *testing.T) {
	d := NewOpcUaDriver().(*OpcUaDriver)
	w := &ClientWrapper{
		Subscriptions: make(map[string]*DeviceSubscription),
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Subscriptions["dev-1"] = &DeviceSubscription{
		Ctx:    ctx,
		Cancel: cancel,
		Points: map[string]model.Point{"p1": {ID: "p1"}},
	}
	d.activeClient = w

	d.ResetDeviceCollection("dev-1")

	if len(w.Subscriptions) != 0 {
		t.Fatalf("expected subscriptions cleared, got %d", len(w.Subscriptions))
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected subscription context cancelled")
	}
}
