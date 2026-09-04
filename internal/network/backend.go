package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// BackendType identifies the detected Linux network management tool.
type BackendType string

const (
	BackendNetworkManager  BackendType = "networkmanager"
	BackendSystemdNetworkd BackendType = "systemd-networkd"
	BackendNetplan         BackendType = "netplan"
	BackendIfupdown        BackendType = "ifupdown"
	BackendIPRoute         BackendType = "iproute2"
)

// BackendInfo describes the active network persistence backend.
type BackendInfo struct {
	Type  BackendType `json:"type"`
	Label string      `json:"label"`
}

func (t BackendType) Label() string {
	switch t {
	case BackendNetworkManager:
		return "NetworkManager"
	case BackendSystemdNetworkd:
		return "systemd-networkd"
	case BackendNetplan:
		return "netplan"
	case BackendIfupdown:
		return "ifupdown"
	default:
		return "iproute2 (runtime only)"
	}
}

// PersistBackend persists network configuration through OS-specific tools.
type PersistBackend interface {
	Type() BackendType
	ApplyInterfaceConfig(iface model.NetworkInterface) error
	ApplyStaticRoute(route model.StaticRoute) error
}

var (
	commandExistsFn      = commandExists
	systemdActiveFn      = systemdServiceActive
	netplanConfigExists  = hasNetplanConfig
	ifupdownConfigExists = hasIfupdownConfig
)

// DetectPersistBackend selects the best available persistence backend.
func DetectPersistBackend() PersistBackend {
	if commandExistsFn("nmcli") && systemdActiveFn("NetworkManager") {
		return &networkManagerBackend{}
	}
	if systemdActiveFn("systemd-networkd") {
		return &systemdNetworkdBackend{}
	}
	if commandExistsFn("netplan") && netplanConfigExists() {
		return &netplanBackend{}
	}
	if ifupdownConfigExists() {
		return &ifupdownBackend{}
	}
	return &iprouteBackend{}
}

// GetBackendInfo returns metadata about the detected backend.
func GetBackendInfo() BackendInfo {
	b := DetectPersistBackend()
	return BackendInfo{
		Type:  b.Type(),
		Label: b.Type().Label(),
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func systemdServiceActive(unit string) bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil
}

func hasNetplanConfig() bool {
	matches, err := filepath.Glob("/etc/netplan/*.yaml")
	if err != nil {
		return false
	}
	for _, path := range matches {
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			return true
		}
	}
	return false
}

func hasIfupdownConfig() bool {
	if _, err := os.Stat("/etc/network/interfaces"); err == nil {
		return true
	}
	matches, _ := filepath.Glob("/etc/network/interfaces.d/*")
	return len(matches) > 0
}

func prefixToNetmask(prefix int) string {
	if prefix <= 0 || prefix > 32 {
		return "255.255.255.0"
	}
	mask := uint32(0xFFFFFFFF << (32 - prefix))
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask))
}
