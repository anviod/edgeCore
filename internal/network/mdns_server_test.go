package network

import (
	"net"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestCollectIPv4AddressesLoopbackSkipped(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("cannot list interfaces: %v", err)
	}

	var loName string
	for _, iface := range ifaces {
		if isLoopbackInterface(iface.Name, iface.Flags) {
			loName = iface.Name
			break
		}
	}
	if loName == "" {
		t.Skip("no loopback interface found")
	}

	ips, _, err := collectIPv4Addresses([]string{loName})
	if err == nil {
		t.Fatalf("expected error for loopback-only interface, got ips=%v", ips)
	}
}

func TestMDNSServerStartRequiresMDNSEnabled(t *testing.T) {
	server := NewMDNSServer()
	err := server.Start(model.HostnameConfig{EnableMDNS: false})
	if err != nil {
		t.Fatalf("expected nil when mDNS disabled, got %v", err)
	}
}

func TestMDNSServerStartRequiresIPs(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("cannot list interfaces: %v", err)
	}

	var loName string
	for _, iface := range ifaces {
		if isLoopbackInterface(iface.Name, iface.Flags) {
			loName = iface.Name
			break
		}
	}
	if loName == "" {
		t.Skip("no loopback interface found")
	}

	server := NewMDNSServer()
	err = server.Start(model.HostnameConfig{
		Name:       "edgeCore",
		EnableMDNS: true,
		HTTPPort:   8080,
		Interfaces: []string{loName},
	})
	if err == nil {
		server.Stop()
		t.Fatal("expected error when only loopback interface is configured")
	}
}
