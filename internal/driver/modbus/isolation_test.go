package modbus

import (
	"fmt"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
)

// TestScenario_DeviceFaultIsolation verifies single-slave errors do not
// trigger channel-level reconnect or TCP disconnect on a shared Modbus link.
func TestScenario_DeviceFaultIsolation(t *testing.T) {
	cfg := model.DriverConfig{
		Config: map[string]any{
			"url": "127.0.0.1:5020",
		},
	}
	mt := NewModbusTransport(cfg)
	defer mt.connMgr.Close()
	mt.connected.Store(true)

	deviceErrors := []string{
		"i/o timeout",
		"request timed out",
		"modbus exception 2: illegal data address",
		"gateway target device failed to respond",
		"read timeout on slave 3",
		"bad unit id",
		"server device busy",
	}
	for _, msg := range deviceErrors {
		before := mt.collectFailCount.Load()
		mt.RecordFailure(fmt.Errorf("%s", msg))
		assert.Equal(t, before, mt.collectFailCount.Load(), "device error %q must not increment collectFailCount", msg)
		assert.True(t, mt.connected.Load(), "device error %q must not disconnect shared TCP", msg)
	}

	mt.RecordFailure(fmt.Errorf("connection reset by peer"))
	assert.Equal(t, int32(1), mt.collectFailCount.Load(), "link error must increment collectFailCount")
}
