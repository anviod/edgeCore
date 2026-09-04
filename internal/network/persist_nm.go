package network

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

type networkManagerBackend struct{}

func (b *networkManagerBackend) Type() BackendType { return BackendNetworkManager }

func (b *networkManagerBackend) connectionName(iface string) (string, error) {
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,DEVICE", "con", "show")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[1]) == iface {
			return strings.TrimSpace(parts[0]), nil
		}
	}
	return iface, nil
}

func (b *networkManagerBackend) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	conn, err := b.connectionName(iface.Name)
	if err != nil {
		return err
	}

	v4Static := []string{}
	v4DHCP := false
	v6Static := []string{}
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
		entry := fmt.Sprintf("%s/%d", ip.Address, ip.Prefix)
		if ip.Version == "IPv6" || strings.Contains(ip.Address, ":") {
			v6Static = append(v6Static, entry)
		} else {
			v4Static = append(v4Static, entry)
		}
	}

	args := []string{"con", "mod", conn}
	if v4DHCP {
		args = append(args, "ipv4.method", "auto", "ipv4.addresses", "")
	} else if len(v4Static) > 0 {
		args = append(args, "ipv4.method", "manual", "ipv4.addresses", strings.Join(v4Static, ","))
	} else {
		args = append(args, "ipv4.method", "disabled")
	}

	if v6DHCP {
		args = append(args, "ipv6.method", "auto")
	} else if len(v6Static) > 0 {
		args = append(args, "ipv6.method", "manual", "ipv6.addresses", strings.Join(v6Static, ","))
	} else {
		args = append(args, "ipv6.method", "ignore")
	}

	for _, gw := range iface.Gateways {
		if !gw.Enabled || gw.Gateway == "" {
			continue
		}
		if strings.Contains(gw.Gateway, ":") {
			args = append(args, "ipv6.gateway", gw.Gateway)
		} else {
			args = append(args, "ipv4.gateway", gw.Gateway)
		}
	}

	if out, err := exec.Command("nmcli", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli con mod failed: %v, output: %s", err, out)
	}
	if out, err := exec.Command("nmcli", "con", "up", conn).CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli con up failed: %v, output: %s", err, out)
	}
	return nil
}

func (b *networkManagerBackend) ApplyStaticRoute(route model.StaticRoute) error {
	dest := route.Destination
	if route.Prefix >= 0 {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	args := []string{"connection", "modify", route.Interface, "+ipv4.routes", dest}
	if route.Gateway != "" {
		args[len(args)-1] = fmt.Sprintf("%s %s", dest, route.Gateway)
	}
	if out, err := exec.Command("nmcli", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli route failed: %v, output: %s", err, out)
	}
	return nil
}
