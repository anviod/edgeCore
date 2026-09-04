package modbus

import (
	"fmt"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestIsDeviceLevelModbusError(t *testing.T) {
	assert.True(t, isDeviceLevelModbusError(fmt.Errorf("i/o timeout")))
	assert.True(t, isDeviceLevelModbusError(fmt.Errorf("modbus exception 2: illegal data address")))
	assert.True(t, isDeviceLevelModbusError(fmt.Errorf("request timed out")))
	assert.True(t, isDeviceLevelModbusError(fmt.Errorf("bad unit id")))
	assert.False(t, isDeviceLevelModbusError(fmt.Errorf("connection refused")))
	assert.False(t, isDeviceLevelModbusError(fmt.Errorf("dial tcp 127.0.0.1:502: connect: connection refused")))
}

func TestRecordFailure_DeviceLevelDoesNotIncrement(t *testing.T) {
	cfg := model.DriverConfig{
		Config: map[string]any{
			"url": "127.0.0.1:5020",
		},
	}
	mt := NewModbusTransport(cfg)
	defer mt.connMgr.Close()

	mt.RecordFailure(fmt.Errorf("i/o timeout"))
	assert.Equal(t, int32(0), mt.collectFailCount.Load())

	mt.RecordFailure(fmt.Errorf("connection reset by peer"))
	assert.Equal(t, int32(1), mt.collectFailCount.Load())
}
