package dlt645

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type connMode int

const (
	connSerial connMode = iota
	connTCP
)

type transportConfig struct {
	mode          connMode
	ip            string
	port          int
	serialPort    string
	baudRate      int
	dataBits      int
	stopBits      int
	parity        string
	timeout       time.Duration
	sendInterval  time.Duration
	maxRetries    int
	maxFailCount  int32
	preambleBytes int
}

func parseTransportConfig(cfg map[string]any) transportConfig {
	tc := transportConfig{
		port:          8001,
		baudRate:      9600,
		dataBits:      8,
		stopBits:      1,
		parity:        "N",
		timeout:       2 * time.Second,
		sendInterval:  200 * time.Millisecond,
		maxRetries:    3,
		maxFailCount:  5,
		preambleBytes: 4,
	}

	mode, _ := cfg["connectionType"].(string)
	if mode == "" {
		mode, _ = cfg["connection_type"].(string)
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "tcp", "ethernet":
		tc.mode = connTCP
	default:
		tc.mode = connSerial
	}

	if v, ok := cfg["ip"].(string); ok {
		tc.ip = v
	}
	tc.port = getCfgInt(cfg, "port", tc.port)
	if p, ok := cfg["port"].(string); ok && tc.mode == connSerial {
		tc.serialPort = p
	} else if p, ok := cfg["serialPort"].(string); ok {
		tc.serialPort = p
	}
	if tc.serialPort == "" {
		if p, ok := cfg["port"].(string); ok {
			tc.serialPort = p
		}
	}

	tc.baudRate = getCfgInt(cfg, "baudRate", tc.baudRate)
	tc.dataBits = getCfgInt(cfg, "dataBits", tc.dataBits)
	tc.stopBits = getCfgInt(cfg, "stopBits", tc.stopBits)
	if v, ok := cfg["parity"].(string); ok && v != "" {
		tc.parity = normalizeParity(v)
	}

	if v, ok := cfg["timeout"]; ok {
		if d := parseDurationMs(v); d > 0 {
			tc.timeout = d
		}
	}
	if v, ok := cfg["responseTimeout"]; ok {
		if d := parseDurationMs(v); d > 0 {
			tc.timeout = d
		}
	}
	if v, ok := cfg["sendInterval"]; ok {
		if d := parseDurationMs(v); d > 0 {
			tc.sendInterval = d
		}
	}
	tc.maxRetries = getCfgInt(cfg, "maxRetries", tc.maxRetries)
	if tc.maxRetries <= 0 {
		tc.maxRetries = getCfgInt(cfg, "max_retries", tc.maxRetries)
	}
	// 配置值为 int，饱和钳制到 int32 域防止计数器溢出
	if mf := getCfgInt(cfg, "max_fail_count", int(tc.maxFailCount)); mf < math.MinInt32 {
		tc.maxFailCount = math.MinInt32
	} else if mf > math.MaxInt32 {
		tc.maxFailCount = math.MaxInt32
	} else {
		tc.maxFailCount = int32(mf)
	}
	tc.preambleBytes = getCfgInt(cfg, "preambleBytes", tc.preambleBytes)

	return tc
}

func (tc transportConfig) remoteAddr() string {
	if tc.mode == connTCP && tc.ip != "" {
		return fmt.Sprintf("%s:%d", tc.ip, tc.port)
	}
	if tc.mode == connSerial && tc.serialPort != "" {
		return tc.serialPort
	}
	return ""
}

func normalizeParity(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "none", "n":
		return "N"
	case "odd", "o":
		return "O"
	case "even", "e":
		return "E"
	default:
		if len(p) > 0 {
			return strings.ToUpper(p[:1])
		}
		return "N"
	}
}

func parseDurationMs(v any) time.Duration {
	switch val := v.(type) {
	case float64:
		return time.Duration(val) * time.Millisecond
	case int:
		return time.Duration(val) * time.Millisecond
	case int64:
		return time.Duration(val) * time.Millisecond
	case string:
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return 0
}

func getCfgInt(cfg map[string]any, key string, defaultVal int) int {
	if v, ok := cfg[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int64:
			return int(val)
		case string:
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
				return n
			}
		}
	}
	return defaultVal
}
