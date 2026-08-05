package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenDeviceKey_WithFingerprint(t *testing.T) {
	fp := &DeviceFingerprint{
		Vendor:  "Siemens",
		Model:   "S7-1200",
		SN:      "SN123456",
	}
	key := GenDeviceKey("192.168.1.10", "modbus-tcp", 502, fp)

	expected := fmt.Sprintf("%s:%s:%s:%d", fp.Vendor, fp.Model, fp.SN, 502)
	hash := sha256.Sum256([]byte(expected))
	assert.Equal(t, hex.EncodeToString(hash[:]), key)
	assert.Len(t, key, 64)
}

func TestGenDeviceKey_WithoutFingerprint(t *testing.T) {
	key := GenDeviceKey("192.168.1.10", "modbus-tcp", 502, nil)

	expected := fmt.Sprintf("%s:%s:%d", "192.168.1.10", "modbus-tcp", 502)
	hash := sha256.Sum256([]byte(expected))
	assert.Equal(t, hex.EncodeToString(hash[:]), key)
}

func TestGenDeviceKey_EmptyFingerprint(t *testing.T) {
	fp := &DeviceFingerprint{Vendor: "Vendor", Model: "Model", SN: ""}
	key := GenDeviceKey("10.0.0.1", "s7", 102, fp)

	expected := fmt.Sprintf("%s:%s:%d", "10.0.0.1", "s7", 102)
	hash := sha256.Sum256([]byte(expected))
	assert.Equal(t, hex.EncodeToString(hash[:]), key)
}

func TestGenBindingKey(t *testing.T) {
	key := GenBindingKey("192.168.1.10", "modbus-tcp", 502)

	expected := fmt.Sprintf("%s:%s:%d", "192.168.1.10", "modbus-tcp", 502)
	hash := sha256.Sum256([]byte(expected))
	assert.Equal(t, hex.EncodeToString(hash[:]), key)
}

func TestGenDeviceKey_DifferentInputsProduceDifferentHashes(t *testing.T) {
	key1 := GenDeviceKey("192.168.1.10", "modbus-tcp", 502, nil)
	key2 := GenDeviceKey("192.168.1.11", "modbus-tcp", 502, nil)
	key3 := GenDeviceKey("192.168.1.10", "modbus-rtu", 502, nil)
	key4 := GenDeviceKey("192.168.1.10", "modbus-tcp", 503, nil)

	assert.NotEqual(t, key1, key2)
	assert.NotEqual(t, key1, key3)
	assert.NotEqual(t, key1, key4)
}
