package network

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

type ifupdownBackend struct{}

func (b *ifupdownBackend) Type() BackendType { return BackendIfupdown }

func (b *ifupdownBackend) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	path := fmt.Sprintf("/etc/network/interfaces.d/edgeCore-%s", iface.Name)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("auto %s\n", iface.Name))
	builder.WriteString(fmt.Sprintf("iface %s inet ", iface.Name))

	v4DHCP := false
	for _, ip := range iface.IPConfigs {
		if !ip.Enabled {
			continue
		}
		if ip.Source == "DHCP" && ip.Version != "IPv6" && !strings.Contains(ip.Address, ":") {
			v4DHCP = true
			break
		}
	}
	if v4DHCP {
		builder.WriteString("dhcp\n")
	} else {
		builder.WriteString("static\n")
		for _, ip := range iface.IPConfigs {
			if !ip.Enabled || ip.Address == "" || ip.Version == "IPv6" || strings.Contains(ip.Address, ":") {
				continue
			}
			builder.WriteString(fmt.Sprintf("    address %s\n", ip.Address))
			builder.WriteString(fmt.Sprintf("    netmask %s\n", prefixToNetmask(ip.Prefix)))
		}
		for _, gw := range iface.Gateways {
			if gw.Enabled && gw.Gateway != "" && !strings.Contains(gw.Gateway, ":") {
				builder.WriteString(fmt.Sprintf("    gateway %s\n", gw.Gateway))
			}
		}
	}

	if err := writeSystemConfigFile(path, builder.String()); err != nil {
		return err
	}
	return nil
}

func (b *ifupdownBackend) ApplyStaticRoute(route model.StaticRoute) error {
	path := fmt.Sprintf("/etc/network/interfaces.d/edgeCore-route-%s", strings.ReplaceAll(route.Destination, ".", "-"))
	dest := route.Destination
	if route.Prefix > 0 {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	content := fmt.Sprintf("up ip route add %s", dest)
	if route.Gateway != "" {
		content += fmt.Sprintf(" via %s", route.Gateway)
	}
	if route.Interface != "" {
		content += fmt.Sprintf(" dev %s", route.Interface)
	}
	content += "\n"
	return writeSystemConfigFile(path, content)
}
