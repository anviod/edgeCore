package ethernetip

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_SchedulerWriteDisconnected(t *testing.T) {
	tr := NewENIPTransport(map[string]any{"ip": "192.168.1.10"})
	defer tr.connMgr.Close()
	s := NewENIPScheduler(tr, NewENIPDecoder(), nil)

	err := s.WritePoint(context.Background(), model.Point{Address: "MyTag", DataType: "DINT"}, int32(1))
	require.Error(t, err)
}

func TestCoverage_SchedulerLogixClass2Paths(t *testing.T) {
	s := &ENIPScheduler{batchReadMax: 1, transport: NewENIPTransport(map[string]any{"connection_type": "logix"})}
	defer s.transport.connMgr.Close()

	assert.Equal(t, "DintTag", s.resolveLogixTagName("Program:Main.DintTag"))
	id, ok := s.getLogixClass2AttrID("RealTag")
	assert.True(t, ok)
	assert.Equal(t, 10, id)
}

func TestCoverage_DecoderAllLogixTypes(t *testing.T) {
	dec := NewENIPDecoder()
	types := []struct {
		dt  string
		val any
	}{
		{"BOOL", true},
		{"SINT", int8(-1)},
		{"USINT", uint8(255)},
		{"UINT", uint16(1000)},
		{"UDINT", uint32(100000)},
		{"ULINT", uint64(9999999999)},
		{"LINT", int64(-1234567890)},
		{"LREAL", float64(2.718281828)},
	}
	for _, tc := range types {
		raw, err := dec.EncodeValue(tc.val, tc.dt)
		require.NoError(t, err, tc.dt)
		got, err := dec.DecodeValue(raw, tc.dt)
		require.NoError(t, err, tc.dt)
		assert.NotNil(t, got, tc.dt)
	}
}

func TestCoverage_DriverInitAndMetricsExtended(t *testing.T) {
	d := NewEtherNetIPDriver().(*EtherNetIPDriver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "ext",
		Config: map[string]any{
			"ip": "10.0.0.1", "port": 44818, "slot": 1,
			"connection_type": "logix", "batch_read_max": 10,
		},
	}))
	assert.NotNil(t, d.scheduler)
	assert.Equal(t, 10, d.scheduler.batchReadMax)

	d.scheduler.totalRequests = 5
	d.scheduler.successCount = 4
	d.transport.connected.Store(true)
	m := d.GetMetrics()
	assert.InDelta(t, 0.8, m.SuccessRate, 0.01)
}
