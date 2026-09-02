package model

import (
	"encoding/json"
	"testing"
)

func TestResolveOpcUaEndpoint_DeviceOverridesChannel(t *testing.T) {
	channel := map[string]any{"url": "opc.tcp://channel:4840"}
	device := map[string]any{"endpoint": "opc.tcp://device:4840"}
	got := ResolveOpcUaEndpoint(channel, device)
	if got != "opc.tcp://device:4840" {
		t.Fatalf("expected device endpoint, got %q", got)
	}
}

func TestResolveOpcUaEndpoint_InheritsChannelURL(t *testing.T) {
	channel := map[string]any{"url": "opc.tcp://channel:4840"}
	got := ResolveOpcUaEndpoint(channel, map[string]any{})
	if got != "opc.tcp://channel:4840" {
		t.Fatalf("expected channel url, got %q", got)
	}
}

func TestMergeOpcUaDeviceConfig_SetsEndpoint(t *testing.T) {
	channel := map[string]any{
		"url":             "opc.tcp://channel:4840",
		"security_policy": "None",
	}
	device := map[string]any{"username": "admin"}
	merged := MergeOpcUaDeviceConfig(channel, device)
	if merged["endpoint"] != "opc.tcp://channel:4840" {
		t.Fatalf("endpoint not inherited: %v", merged["endpoint"])
	}
	if merged["security_policy"] != "None" {
		t.Fatalf("security_policy not inherited")
	}
	if merged["username"] != "admin" {
		t.Fatalf("device field missing")
	}
}

func TestOpcUaDeviceMap_UnmarshalJSON_ObjectAndLegacyBool(t *testing.T) {
	raw := `{
		"dev1": true,
		"dev2": {"enable": true, "strategy": "periodic", "interval": "10s"},
		"dev3": {"enable": false}
	}`
	var devices OpcUaDeviceMap
	if err := json.Unmarshal([]byte(raw), &devices); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !devices["dev1"].Enable {
		t.Fatal("dev1 should be enabled from legacy bool")
	}
	if !devices["dev2"].Enable || devices["dev2"].Strategy != "periodic" {
		t.Fatalf("dev2 config mismatch: %+v", devices["dev2"])
	}
	if devices["dev3"].Enable {
		t.Fatal("dev3 should be disabled")
	}
}

func TestOPCUAConfig_UnmarshalJSON_FrontendPayload(t *testing.T) {
	raw := `{
		"name": "Factory OPC UA",
		"enable": true,
		"port": 4840,
		"endpoint": "/ipp/opcua/server",
		"auth_methods": ["Anonymous"],
		"devices": {
			"slave-1": {"enable": true, "strategy": "periodic", "interval": "10s"}
		}
	}`
	var cfg OPCUAConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !cfg.Devices.AllowsDevice("slave-1") {
		t.Fatal("slave-1 should be allowed")
	}
	if cfg.Devices.AllowsDevice("slave-2") {
		t.Fatal("slave-2 should not be allowed when not explicitly enabled")
	}
}
