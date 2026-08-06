package s7

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/gos7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_DriverLifecycleAndReadWrite(t *testing.T) {
	d := NewS7Driver().(*S7Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config:    map[string]any{"ip": "127.0.0.1", "rack": 0, "slot": 1, "timeout": 1000},
	}))

	assert.Equal(t, driver.HealthStatusBad, d.Health())
	assert.NoError(t, d.SetSlaveID(1))
	assert.NoError(t, d.SetDeviceConfig(nil))

	m := d.GetMetrics()
	assert.Equal(t, "S7", m.Protocol)

	mockHandler := &mockS7ClientHandler{}
	mockCli := &mockClient{
		agReadMultiFunc: func(dataItems []gos7.S7DataItem, itemsCount int) error {
			for i := range dataItems {
				if len(dataItems[i].Data) >= 2 {
					binary.BigEndian.PutUint16(dataItems[i].Data, 4321)
				}
			}
			return nil
		},
	}
	d.transport.handlerFactory = func(address string, rack, slot, connType int) S7ClientHandler {
		return mockHandler
	}
	d.transport.clientFactory = func(handler S7ClientHandler) Client {
		return mockCli
	}

	ctx := context.Background()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	assert.Equal(t, driver.HealthStatusGood, d.Health())

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "DB1.DBW0", DataType: "int16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)

	require.NoError(t, d.WritePoint(ctx, model.Point{Address: "DB1.DBW0", DataType: "int16"}, int16(100)))

	d.scheduler.totalRequests = 5
	d.scheduler.successCount = 4
	m = d.GetMetrics()
	assert.InDelta(t, 0.8, m.SuccessRate, 0.01)
	assert.Greater(t, m.QualityScore, 0)
}

func TestCoverage_DriverNotConnected(t *testing.T) {
	d := NewS7Driver().(*S7Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"ip": "127.0.0.1"},
	}))

	ctx := context.Background()
	_, err := d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "DB1.DBW0"}})
	require.Error(t, err)
	err = d.WritePoint(ctx, model.Point{Address: "DB1.DBW0"}, int16(1))
	require.Error(t, err)
}

func TestCoverage_DecoderEncodeDecode(t *testing.T) {
	dec := NewS7Decoder()
	area, err := dec.ParseAddress("DB1.DBD0")
	require.NoError(t, err)

	raw := make([]byte, 4)
	binary.BigEndian.PutUint32(raw, 1065353216)
	val, err := dec.DecodeValue(raw, area, "float32")
	require.NoError(t, err)
	assert.InDelta(t, float32(1.0), val.(float32), 0.01)

	buf := make([]byte, 2)
	require.NoError(t, dec.EncodeValue(buf, area, "int16", int16(500)))
	assert.NotZero(t, binary.BigEndian.Uint16(buf))
}

func TestCoverage_SchedulerInvalidAddress(t *testing.T) {
	tr := NewS7Transport(map[string]any{"ip": "127.0.0.1"})
	tr.connected.Store(true)
	s := NewS7Scheduler(tr, NewS7Decoder(), nil)

	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "INVALID", DataType: "int16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
}

func TestCoverage_ParseConfigFloat(t *testing.T) {
	cfg := map[string]any{"ratio": 1.5, "count": 10, "label": "x"}
	assert.InDelta(t, 1.5, parseConfigFloat(cfg, "ratio", 0), 0.001)
	assert.InDelta(t, 10.0, parseConfigFloat(cfg, "count", 0), 0.001)
	assert.InDelta(t, 99.0, parseConfigFloat(cfg, "missing", 99), 0.001)
}

func TestCoverage_TransportMetricsAndProbe(t *testing.T) {
	tr := NewS7Transport(map[string]any{
		"ip": "10.0.0.8", "rack": 0, "slot": 1, "timeout": 500,
	})
	defer tr.connMgr.Close()

	connSec, recon, _, remote, _ := tr.GetConnectionMetrics()
	assert.Equal(t, int64(0), connSec)
	assert.Equal(t, int64(0), recon)
	assert.Contains(t, remote, "10.0.0.8")

	tr.RecordFailure(assert.AnError)
	tr.RecordSuccess()
	assert.False(t, tr.NeedProbeCheck())

	tr.connected.Store(true)
	tr.connectTime = time.Now()
	tr.lastSuccessTime.Store(time.Now().Add(-31 * time.Second))
	assert.True(t, tr.NeedProbeCheck())
	assert.False(t, tr.ProbeConnection())
}

func TestCoverage_SchedulerMerkerRead(t *testing.T) {
	tr := NewS7Transport(map[string]any{"ip": "127.0.0.1"})
	tr.connected.Store(true)

	mockCli := &mockClient{
		agReadMultiFunc: func(dataItems []gos7.S7DataItem, itemsCount int) error {
			for i := range dataItems {
				if len(dataItems[i].Data) >= 2 {
					binary.BigEndian.PutUint16(dataItems[i].Data, 100)
				}
			}
			return nil
		},
	}
	tr.client = mockCli

	s := NewS7Scheduler(tr, NewS7Decoder(), nil)
	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "m2", Address: "MW10", DataType: "int16"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["m2"].Quality)
}

func TestCoverage_QualityScoreDisconnected(t *testing.T) {
	d := NewS7Driver().(*S7Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		Config: map[string]any{"ip": "127.0.0.1"},
	}))
	assert.Equal(t, 45, d.calculateQualityScore(1.0))
}
