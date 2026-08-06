package profinetio

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    *ParsedAddress
		wantErr bool
	}{
		{
			name: "int16 at index 0",
			addr: "3:1:0",
			want: &ParsedAddress{Slot: 3, SubSlot: 1, Index: 0, Bit: -1, Endian: EndianBig},
		},
		{
			name: "uint16 at index 1",
			addr: "3:1:1",
			want: &ParsedAddress{Slot: 3, SubSlot: 1, Index: 1, Bit: -1, Endian: EndianBig},
		},
		{
			name: "bit address",
			addr: "3:2:5.3",
			want: &ParsedAddress{Slot: 3, SubSlot: 2, Index: 5, Bit: 3, Endian: EndianBig, IsBit: true},
		},
		{
			name: "little endian suffix",
			addr: "3:2:10#LE",
			want: &ParsedAddress{Slot: 3, SubSlot: 2, Index: 10, Bit: -1, Endian: EndianLittle},
		},
		{
			name:    "empty",
			addr:    "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			addr:    "DB1.DBD0",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAddress(tc.addr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeEncodeRoundTrip(t *testing.T) {
	decoder := NewProfinetDecoder()

	cases := []struct {
		dataType string
		value    any
		addr     *ParsedAddress
	}{
		{"int16", int16(-1234), &ParsedAddress{Endian: EndianBig}},
		{"uint16", uint16(65000), &ParsedAddress{Endian: EndianBig}},
		{"int32", int32(-99999), &ParsedAddress{Endian: EndianLittle}},
		{"float", float32(3.14), &ParsedAddress{Endian: EndianBig}},
		{"double", float64(2.71828), &ParsedAddress{Endian: EndianLittle}},
		{"bit", true, &ParsedAddress{Bit: 2, IsBit: true}},
	}

	for _, tc := range cases {
		t.Run(tc.dataType, func(t *testing.T) {
			encoded, err := decoder.EncodeValue(tc.value, tc.dataType, tc.addr)
			require.NoError(t, err)
			decoded, err := decoder.DecodeValue(encoded, tc.dataType, tc.addr)
			require.NoError(t, err)
			switch tc.dataType {
			case "float":
				assert.InDelta(t, tc.value, decoded, 0.001)
			case "double":
				assert.InDelta(t, tc.value, decoded, 0.00001)
			default:
				assert.Equal(t, tc.value, decoded)
			}
		})
	}
}

func TestParseDeviceConfig(t *testing.T) {
	cfg := map[string]any{
		"device_name":   "io-device-1",
		"ip":            "192.168.1.20",
		"port":          34964,
		"slot":          3,
		"subslot":       1,
		"input_length":  64,
		"output_length": 32,
	}
	dc := parseDeviceConfig(cfg)
	assert.Equal(t, "io-device-1", dc.deviceName)
	assert.Equal(t, "192.168.1.20", dc.ip)
	assert.Equal(t, 34964, dc.port)
	assert.Equal(t, 3, dc.slot)
	assert.Equal(t, 1, dc.subslot)
	assert.Equal(t, 64, dc.inputLength)
	assert.Equal(t, 32, dc.outputLength)
	assert.Equal(t, "192.168.1.20:34964", dc.remoteAddr())
}

func TestSimulationReadWrite(t *testing.T) {
	store := newSimulationStore()
	data := []byte{0x01, 0x02, 0x03, 0x04}
	store.write(3, 1, 0, data)

	got := store.read(3, 1, 0, 4)
	assert.Equal(t, data, got)

	decoder := NewProfinetDecoder()
	val, err := decoder.DecodeValue(got, "uint32", &ParsedAddress{Endian: EndianBig})
	require.NoError(t, err)
	assert.Equal(t, uint32(0x01020304), val)
}

func TestDriverSimulationMode(t *testing.T) {
	d := NewProfinetIODriver()
	err := d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config: map[string]any{
			"simulation":      true,
			"local_interface": "eth0",
		},
	})
	require.NoError(t, err)

	err = d.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, driver.HealthStatusGood, d.Health())

	d.SetDeviceConfig(map[string]any{
		"device_name": "test-device",
		"ip":          "192.168.1.20",
	})

	results, err := d.ReadPoints(context.Background(), []model.Point{
		{ID: "pt1", Name: "temp", Address: "3:1:0", DataType: "int16"},
	})
	require.NoError(t, err)
	require.Contains(t, results, "pt1")
	assert.Equal(t, "Good", results["pt1"].Quality)
}

func TestDriverWriteSimulationMode(t *testing.T) {
	d := NewProfinetIODriver()
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ch-test",
		Config:    map[string]any{"simulation": true},
	}))
	require.NoError(t, d.Connect(context.Background()))
	d.SetDeviceConfig(map[string]any{"ip": "192.168.1.20"})

	pt := model.Point{ID: "pt1", Name: "out", Address: "3:1:0", DataType: "int16"}
	require.NoError(t, d.WritePoint(context.Background(), pt, int16(42)))

	results, err := d.ReadPoints(context.Background(), []model.Point{pt})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["pt1"].Quality)
}
