package opcua

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestRequiresServerRestart(t *testing.T) {
	base := model.OPCUAConfig{
		Port:           4840,
		Endpoint:       "/",
		SecurityPolicy: "None",
		SecurityMode:   "None",
		AuthMethods:    []string{"Anonymous"},
	}

	same := base
	same.Devices = model.OpcUaDeviceMap{"d1": {Enable: true}}
	if RequiresServerRestart(base, same) {
		t.Fatal("device mapping change must not require restart")
	}

	portChange := base
	portChange.Port = 4841
	if !RequiresServerRestart(base, portChange) {
		t.Fatal("port change must require restart")
	}

	secChange := base
	secChange.SecurityPolicy = "Basic256Sha256"
	if !RequiresServerRestart(base, secChange) {
		t.Fatal("security policy change must require restart")
	}

	authChange := base
	authChange.AuthMethods = []string{"Anonymous", "UserName"}
	if !RequiresServerRestart(base, authChange) {
		t.Fatal("auth methods change must require restart")
	}
}

func TestServerSyncAddressSpaceInPlace(t *testing.T) {
	sb := NewMockSouthboundManager()
	cfg := model.OPCUAConfig{
		Name:        "Sync Test",
		Enable:      true,
		Port:        4851,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}
	srv := NewServer(cfg, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	before := srv.srv
	if err := srv.SyncAddressSpace(); err != nil {
		t.Fatalf("SyncAddressSpace: %v", err)
	}
	if srv.srv != before {
		t.Fatal("SyncAddressSpace must keep the same listener server instance")
	}
}
