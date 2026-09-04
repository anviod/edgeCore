package network

import (
	"net"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestIsLoopbackInterface(t *testing.T) {
	tests := []struct {
		name  string
		iface string
		flags net.Flags
		want  bool
	}{
		{"loopback flag", "eth0", net.FlagLoopback, true},
		{"lo device", "lo", 0, true},
		{"vlan on lo", "lo:1", 0, true},
		{"ethernet", "eth0", net.FlagUp, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLoopbackInterface(tt.iface, tt.flags); got != tt.want {
				t.Fatalf("isLoopbackInterface() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInterfaceStatus(t *testing.T) {
	if interfaceStatus(net.FlagUp) != "UP" {
		t.Fatal("expected UP")
	}
	if interfaceStatus(0) != "DOWN" {
		t.Fatal("expected DOWN")
	}
}

func TestParseDefaultGateways(t *testing.T) {
	input := []byte("default via 192.168.1.1 dev eth0 metric 100\n")
	gws := parseDefaultGateways(input)
	if len(gws["eth0"]) != 1 {
		t.Fatalf("expected 1 gateway, got %d", len(gws["eth0"]))
	}
	if gws["eth0"][0].Gateway != "192.168.1.1" {
		t.Fatalf("unexpected gateway: %s", gws["eth0"][0].Gateway)
	}
}

func TestDetectPersistBackendPriority(t *testing.T) {
	oldCommand := commandExistsFn
	oldSystemd := systemdActiveFn
	oldNetplan := netplanConfigExists
	oldIfupdown := ifupdownConfigExists
	t.Cleanup(func() {
		commandExistsFn = oldCommand
		systemdActiveFn = oldSystemd
		netplanConfigExists = oldNetplan
		ifupdownConfigExists = oldIfupdown
	})

	commandExistsFn = func(name string) bool { return name == "nmcli" || name == "netplan" }
	systemdActiveFn = func(unit string) bool { return unit == "NetworkManager" }
	netplanConfigExists = func() bool { return true }
	ifupdownConfigExists = func() bool { return true }

	backend := DetectPersistBackend()
	if backend.Type() != BackendNetworkManager {
		t.Fatalf("expected NetworkManager, got %s", backend.Type())
	}

	systemdActiveFn = func(unit string) bool { return unit == "systemd-networkd" }
	backend = DetectPersistBackend()
	if backend.Type() != BackendSystemdNetworkd {
		t.Fatalf("expected systemd-networkd, got %s", backend.Type())
	}

	systemdActiveFn = func(string) bool { return false }
	backend = DetectPersistBackend()
	if backend.Type() != BackendNetplan {
		t.Fatalf("expected netplan, got %s", backend.Type())
	}

	commandExistsFn = func(string) bool { return false }
	netplanConfigExists = func() bool { return false }
	backend = DetectPersistBackend()
	if backend.Type() != BackendIfupdown {
		t.Fatalf("expected ifupdown, got %s", backend.Type())
	}

	ifupdownConfigExists = func() bool { return false }
	backend = DetectPersistBackend()
	if backend.Type() != BackendIPRoute {
		t.Fatalf("expected iproute2, got %s", backend.Type())
	}
}

func TestDiscoverInterfacesExcludesLoopback(t *testing.T) {
	ifaces, err := discoverInterfacesFromNet(&iprouteBackend{})
	if err != nil {
		t.Skipf("skipping interface discovery in this environment: %v", err)
	}
	for _, iface := range ifaces {
		if isLoopbackInterface(iface.Name, 0) || iface.Name == "lo" {
			t.Fatalf("loopback interface should be excluded, got %s", iface.Name)
		}
		if iface.Status != "UP" && iface.Status != "DOWN" {
			t.Fatalf("unexpected status %s for %s", iface.Status, iface.Name)
		}
	}
}

func TestMergeConfiguredInterfaces(t *testing.T) {
	live := []model.NetworkInterface{
		{Name: "eth0", Status: "UP", IPConfigs: []model.IPConfig{{Address: "10.0.0.2", Prefix: 24, Version: "IPv4"}}},
	}
	configured := []model.NetworkInterface{
		{Name: "eth0", Enabled: true, Gateways: []model.GatewayConfig{{Gateway: "10.0.0.1", Enabled: true}}},
	}
	merged := MergeConfiguredInterfaces(live, configured)
	if len(merged[0].Gateways) != 1 || merged[0].Gateways[0].Gateway != "10.0.0.1" {
		t.Fatal("expected configured gateway to be merged")
	}
}
