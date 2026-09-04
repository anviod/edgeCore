package network

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

type systemdNetworkdBackend struct{}

func (b *systemdNetworkdBackend) Type() BackendType { return BackendSystemdNetworkd }

func (b *systemdNetworkdBackend) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	path := fmt.Sprintf("/etc/systemd/network/10-edgeCore-%s.network", iface.Name)
	var builder strings.Builder
	builder.WriteString("[Match]\n")
	builder.WriteString(fmt.Sprintf("Name=%s\n\n", iface.Name))
	builder.WriteString("[Network]\n")

	v4DHCP := false
	v6DHCP := false
	for _, ip := range iface.IPConfigs {
		if !ip.Enabled {
			continue
		}
		if ip.Source == "DHCP" {
			if ip.Version == "IPv6" || strings.Contains(ip.Address, ":") {
				v6DHCP = true
			} else {
				v4DHCP = true
			}
			continue
		}
		if ip.Address == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf("Address=%s/%d\n", ip.Address, ip.Prefix))
	}
	if v4DHCP {
		builder.WriteString("DHCP=ipv4\n")
	}
	if v6DHCP {
		builder.WriteString("DHCP=ipv6\n")
	}

	for _, gw := range iface.Gateways {
		if gw.Enabled && gw.Gateway != "" {
			builder.WriteString(fmt.Sprintf("Gateway=%s\n", gw.Gateway))
		}
	}

	if err := writeSystemConfigFile(path, builder.String()); err != nil {
		return err
	}
	if out, err := exec.Command("networkctl", "reload").CombinedOutput(); err != nil {
		if out2, err2 := exec.Command("systemctl", "restart", "systemd-networkd").CombinedOutput(); err2 != nil {
			return fmt.Errorf("reload networkd failed: %v (%s), restart failed: %v (%s)", err, out, err2, out2)
		}
	}
	return nil
}

func (b *systemdNetworkdBackend) ApplyStaticRoute(route model.StaticRoute) error {
	path := fmt.Sprintf("/etc/systemd/network/20-edgeCore-route-%s-%s.network", route.Interface, strings.ReplaceAll(route.Destination, ":", ""))
	dest := route.Destination
	if route.Prefix > 0 {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	content := fmt.Sprintf("[Match]\nName=%s\n\n[Route]\nDestination=%s\n", route.Interface, dest)
	if route.Gateway != "" {
		content += fmt.Sprintf("Gateway=%s\n", route.Gateway)
	}
	if route.Metric > 0 {
		content += fmt.Sprintf("Metric=%d\n", route.Metric)
	}
	if err := writeSystemConfigFile(path, content); err != nil {
		return err
	}
	_ = exec.Command("networkctl", "reload").Run()
	return nil
}
