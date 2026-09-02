package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLookupNorthboundPublishConfig(t *testing.T) {
	real := OpcUaDeviceMap{
		"dev-1": {Enable: true, Strategy: "periodic", Interval: Duration(5 * time.Second)},
	}
	virtual := OpcUaDeviceMap{
		"vdev-1": {Enable: true, Strategy: "periodic", Interval: Duration(10 * time.Second)},
		"vdev-2": {Enable: false},
	}

	if cfg, ok := LookupNorthboundPublishConfig("dev-1", real, virtual); !ok || cfg.Strategy != "periodic" {
		t.Fatalf("expected real device config, got %+v ok=%v", cfg, ok)
	}
	if cfg, ok := LookupNorthboundPublishConfig("vdev-1", real, virtual); !ok || cfg.Interval != Duration(10*time.Second) {
		t.Fatalf("expected virtual device config, got %+v ok=%v", cfg, ok)
	}
	if _, ok := LookupNorthboundPublishConfig("vdev-2", real, virtual); ok {
		t.Fatal("expected disabled virtual device to be rejected")
	}
	if _, ok := LookupNorthboundPublishConfig("unknown", real, virtual); ok {
		t.Fatal("expected unknown device to be rejected when maps are non-empty")
	}
	if _, ok := LookupNorthboundPublishConfig("any", nil, nil); ok {
		t.Fatal("expected no implicit allow-all when both maps empty")
	}
}

func TestLookupNorthboundPublishConfigLegacyBoolDevices(t *testing.T) {
	var devices OpcUaDeviceMap
	if err := json.Unmarshal([]byte(`{"dev-1":true,"dev-2":false}`), &devices); err != nil {
		t.Fatalf("unmarshal legacy bool devices: %v", err)
	}
	if cfg, ok := LookupNorthboundPublishConfig("dev-1", devices, nil); !ok || !cfg.Enable || cfg.Strategy != "realtime" {
		t.Fatalf("expected enabled legacy device, got %+v ok=%v", cfg, ok)
	}
	if _, ok := LookupNorthboundPublishConfig("dev-2", devices, nil); ok {
		t.Fatal("expected disabled legacy device to be rejected")
	}
}

func TestIsNorthboundVirtualDevice(t *testing.T) {
	virtual := OpcUaDeviceMap{"vdev-1": {Enable: true}}
	if !IsNorthboundVirtualDevice("vdev-1", virtual) {
		t.Fatal("expected virtual device")
	}
	if IsNorthboundVirtualDevice("dev-1", virtual) {
		t.Fatal("expected physical device to not be virtual")
	}
}
