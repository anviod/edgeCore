package ice104

import (
	"fmt"
	"strconv"
	"time"
)

type deviceConfig struct {
	IP                  string
	Port                int
	CommonAddress       uint16
	GeneralCallInterval time.Duration
	ClockSyncInterval   time.Duration
	PulseInterval       time.Duration
	T0                  time.Duration
	T1                  time.Duration
	T2                  time.Duration
	T3                  time.Duration
	W                   int
	MaxRetries          int
	RetryInterval       time.Duration
}

func parseDeviceConfig(raw map[string]any) deviceConfig {
	cfg := deviceConfig{
		IP:                  "127.0.0.1",
		Port:                2404,
		CommonAddress:       1,
		GeneralCallInterval: 300 * time.Second,
		ClockSyncInterval:   600 * time.Second,
		PulseInterval:       300 * time.Second,
		T0:                  10 * time.Second,
		T1:                  15 * time.Second,
		T2:                  10 * time.Second,
		T3:                  20 * time.Second,
		W:                   7,
		MaxRetries:          3,
		RetryInterval:       time.Second,
	}
	if raw == nil {
		return cfg
	}
	if v, ok := raw["ip"].(string); ok && v != "" {
		cfg.IP = v
	}
	cfg.Port = intFromAny(raw["port"], cfg.Port)
	commonAddress := intFromAny(raw["commonAddress"], int(cfg.CommonAddress))
	if commonAddress < 0 {
		commonAddress = 0
	} else if commonAddress > 65535 {
		commonAddress = 65535
	}
	cfg.CommonAddress = uint16(commonAddress)
	cfg.GeneralCallInterval = secondsFromAny(raw["generalCallInterval"], cfg.GeneralCallInterval)
	cfg.ClockSyncInterval = secondsFromAny(raw["clockSyncInterval"], cfg.ClockSyncInterval)
	cfg.PulseInterval = secondsFromAny(raw["pulseInterval"], cfg.PulseInterval)
	cfg.T0 = secondsFromAny(raw["t0"], cfg.T0)
	cfg.T1 = secondsFromAny(raw["t1"], cfg.T1)
	cfg.T2 = secondsFromAny(raw["t2"], cfg.T2)
	cfg.T3 = secondsFromAny(raw["t3"], cfg.T3)
	cfg.W = intFromAny(raw["w"], cfg.W)
	cfg.MaxRetries = intFromAny(raw["maxRetries"], cfg.MaxRetries)
	cfg.RetryInterval = millisFromAny(raw["retryInterval"], cfg.RetryInterval)
	return cfg
}

func (c deviceConfig) remoteAddr() string {
	return fmt.Sprintf("%s:%d", c.IP, c.Port)
}

func intFromAny(v any, fallback int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if parsed, err := strconv.Atoi(n); err == nil {
			return parsed
		}
	}
	return fallback
}

func secondsFromAny(v any, fallback time.Duration) time.Duration {
	n := intFromAny(v, -1)
	if n < 0 {
		return fallback
	}
	if n == 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func millisFromAny(v any, fallback time.Duration) time.Duration {
	n := intFromAny(v, -1)
	if n < 0 {
		return fallback
	}
	return time.Duration(n) * time.Millisecond
}
