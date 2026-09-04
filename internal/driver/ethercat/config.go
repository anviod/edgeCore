// Package ethercat implements EtherCAT master driver for edgeCore gateway.
// Provides PDO periodic snapshot read/write, CoE SDO parameter access,
// bus topology scanning, and slave enumeration via anviod/EtherCAT library.
package ethercat

import (
	"fmt"
	"strconv"
	"time"
)

// channelConfig holds channel-level EtherCAT master configuration.
// One channel binds one physical network interface for the entire bus.
type channelConfig struct {
	localInterface string        // bind network interface, e.g. "eth0"
	cycleTime      time.Duration // PDO exchange cycle period, default 1ms
	timeout        time.Duration // SDO / state transition timeout, default 3s
	maxRetries     int           // link error retry count, default 3
	simulation     bool          // simulation mode (no real NIC required)
}

// defaultChannelConfig returns sensible defaults.
func defaultChannelConfig() channelConfig {
	return channelConfig{
		cycleTime:  1 * time.Millisecond,
		timeout:    3 * time.Second,
		maxRetries: 3,
	}
}

// parseChannelConfig parses channel-level config from map[string]any.
// Supports both snake_case and camelCase key aliases.
func parseChannelConfig(cfg map[string]any) (channelConfig, error) {
	c := defaultChannelConfig()

	if v := firstString(cfg, "local_interface", "localInterface"); v != "" {
		c.localInterface = v
	}
	if v := firstInt(cfg, "cycle_time_us", "cycleTimeUs"); v > 0 {
		c.cycleTime = time.Duration(v) * time.Microsecond
	}
	if v := firstInt(cfg, "timeout"); v > 0 {
		c.timeout = time.Duration(v) * time.Millisecond
	}
	if v := firstInt(cfg, "max_retries", "maxRetries"); v > 0 {
		c.maxRetries = v
	}
	if v := firstBool(cfg, "simulation"); v {
		c.simulation = true
	}

	if !c.simulation && c.localInterface == "" {
		return c, fmt.Errorf("ethercat channel: local_interface is required (or enable simulation mode)")
	}

	return c, nil
}

// deviceConfig holds per-device (slave) EtherCAT configuration.
type deviceConfig struct {
	position    int    // slave position on bus (1..N)
	alias       int    // optional alias address
	vendorID    string // vendor ID, e.g. "0x00000002"
	productCode string // product code, e.g. "0x044c2c52"
	revision    string // optional revision
	txPDOSize   int    // TxPDO (input) image size in bytes
	rxPDOSize   int    // RxPDO (output) image size in bytes
	runMode     string // "pdo" (default) or "sdo"
}

// defaultDeviceConfig returns sensible defaults.
func defaultDeviceConfig() deviceConfig {
	return deviceConfig{
		runMode: "pdo",
	}
}

// parseDeviceConfig parses per-device config from map[string]any.
// Supports both snake_case and camelCase key aliases.
func parseDeviceConfig(cfg map[string]any) (deviceConfig, error) {
	c := defaultDeviceConfig()

	if v := firstInt(cfg, "position"); v > 0 {
		c.position = v
	}
	if v := firstInt(cfg, "alias"); v > 0 {
		c.alias = v
	}
	if v := firstString(cfg, "vendor_id", "vendorId"); v != "" {
		c.vendorID = v
	}
	if v := firstString(cfg, "product_code", "productCode"); v != "" {
		c.productCode = v
	}
	if v := firstString(cfg, "revision"); v != "" {
		c.revision = v
	}
	if v := firstInt(cfg, "tx_pdo_size", "txPdoSize"); v > 0 {
		c.txPDOSize = v
	}
	if v := firstInt(cfg, "rx_pdo_size", "rxPdoSize"); v > 0 {
		c.rxPDOSize = v
	}
	if v := firstString(cfg, "run_mode", "runMode"); v != "" {
		if v != "pdo" && v != "sdo" {
			return c, fmt.Errorf("ethercat device: run_mode must be 'pdo' or 'sdo', got %q", v)
		}
		c.runMode = v
	}

	if c.position <= 0 {
		return c, fmt.Errorf("ethercat device: position is required (slave position on bus, 1..N)")
	}

	return c, nil
}

// --- config helper functions (aligned with profinetio/config.go) ---

// firstString returns the first non-empty string value from config map
// matching any of the given keys (tried in order).
func firstString(cfg map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// firstInt returns the first non-zero integer value from config map
// matching any of the given keys (tried in order).
// Supports int, int64, float64 (JSON), and string representations.
func firstInt(cfg map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := cfg[k]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case int:
			if val != 0 {
				return val
			}
		case int64:
			if val != 0 {
				return int(val)
			}
		case float64:
			if val != 0 {
				return int(val)
			}
		case string:
			if i, err := strconv.Atoi(val); err == nil && i != 0 {
				return i
			}
		}
	}
	return 0
}

// firstBool returns the first boolean value from config map
// matching any of the given keys.
func firstBool(cfg map[string]any, keys ...string) bool {
	for _, k := range keys {
		v, ok := cfg[k]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case bool:
			return val
		case string:
			if b, err := strconv.ParseBool(val); err == nil {
				return b
			}
		}
	}
	return false
}
