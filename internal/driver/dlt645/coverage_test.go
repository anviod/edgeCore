package dlt645

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_DriverLifecycle(t *testing.T) {
	d := NewDLT645Driver().(*DLT645Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config: map[string]any{
			"connectionType": "tcp",
			"ip":             "127.0.0.1",
			"port":           8001,
		},
	}))

	assert.Equal(t, driver.HealthStatusBad, d.Health())
	assert.NoError(t, d.SetSlaveID(1))
	require.NoError(t, d.SetDeviceConfig(map[string]any{
		"station_address": "210220003011",
	}))

	_, _, _, remote, _ := d.GetConnectionMetrics()
	assert.Contains(t, remote, "127.0.0.1")

	m := d.GetMetrics()
	assert.Equal(t, "DLT645", m.Protocol)
	assert.Less(t, m.QualityScore, 85)

	ctx := context.Background()
	_, err := d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "02-01-01-00"}})
	require.Error(t, err)
}

func TestCoverage_DriverWithMockTransport(t *testing.T) {
	d := NewDLT645Driver().(*DLT645Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "mock",
		Config: map[string]any{
			"connectionType": "tcp",
			"ip":             "127.0.0.1",
			"port":           8002,
		},
	}))

	mock := &mockLink{}
	d.transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}

	ctx := context.Background()
	require.NoError(t, d.Connect(ctx))
	defer d.Disconnect()

	assert.Equal(t, driver.HealthStatusGood, d.Health())

	d.scheduler.totalRequests = 4
	d.scheduler.successCount = 3
	m := d.GetMetrics()
	assert.InDelta(t, 0.75, m.SuccessRate, 0.01)
}

func TestCoverage_DecoderErrorCode(t *testing.T) {
	frame := Frame{Control: CtrlRead | CtrlErrorMask, Data: []byte{0x33}}
	assert.True(t, frame.IsError())
	assert.Equal(t, byte(0x33), frame.ErrorCode())
}

func TestCoverage_ConfigNormalizeParity(t *testing.T) {
	cfg := parseTransportConfig(map[string]any{
		"connectionType": "serial",
		"port":           "/dev/ttyUSB0",
		"parity":         "EVEN",
	})
	assert.Equal(t, "E", cfg.parity)
}

func TestCoverage_DecoderStringValue(t *testing.T) {
	v := decodeStringValue([]byte{0x26, 0x07, 0x04, 0x12, 0x30, 0x45})
	assert.Contains(t, v, "2026")
	assert.Contains(t, v, ":")

	hexVal := decodeStringValue([]byte{'A', 'B', 'C'})
	assert.Equal(t, "414243", hexVal)
}
