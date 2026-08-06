package network

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

type netplanBackend struct{}

func (b *netplanBackend) Type() BackendType { return BackendNetplan }

func (b *netplanBackend) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	path := "/etc/netplan/99-edgeCore.yaml"
	var builder strings.Builder
	builder.WriteString("network:\n  version: 2\n  ethernets:\n")
	builder.WriteString(fmt.Sprintf("    %s:\n", iface.Name))

	v4DHCP := false
	v6DHCP := false
	addresses := []string{}
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
		if ip.Address != "" {
			addresses = append(addresses, fmt.Sprintf("%s/%d", ip.Address, ip.Prefix))
		}
	}

	if len(addresses) > 0 {
		builder.WriteString("      addresses:\n")
		for _, addr := range addresses {
			builder.WriteString(fmt.Sprintf("        - %s\n", addr))
		}
	}
	if v4DHCP {
		builder.WriteString("      dhcp4: true\n")
	}
	if v6DHCP {
		builder.WriteString("      dhcp6: true\n")
	}

	for _, gw := range iface.Gateways {
		if gw.Enabled && gw.Gateway != "" {
			if strings.Contains(gw.Gateway, ":") {
				builder.WriteString(fmt.Sprintf("      gateway6: %s\n", gw.Gateway))
			} else {
				builder.WriteString(fmt.Sprintf("      gateway4: %s\n", gw.Gateway))
			}
		}
	}

	if err := writeSystemConfigFile(path, builder.String()); err != nil {
		return err
	}
	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply failed: %v, output: %s", err, out)
	}
	return nil
}

func (b *netplanBackend) ApplyStaticRoute(route model.StaticRoute) error {
	path := "/etc/netplan/98-edgeCore-routes.yaml"
	dest := route.Destination
	if route.Prefix > 0 {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	content := fmt.Sprintf(`network:
  version: 2
  routes:
    - to: %s
`, dest)
	if route.Gateway != "" {
		content += fmt.Sprintf("      via: %s\n", route.Gateway)
	}
	if route.Interface != "" {
		content += fmt.Sprintf("      interface: %s\n", route.Interface)
	}
	if route.Metric > 0 {
		content += fmt.Sprintf("      metric: %d\n", route.Metric)
	}
	if err := writeSystemConfigFile(path, content); err != nil {
		return err
	}
	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply failed: %v, output: %s", err, out)
	}
	return nil
}
