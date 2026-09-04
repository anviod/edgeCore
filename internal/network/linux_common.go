package network

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

func isLoopbackInterface(name string, flags net.Flags) bool {
	if flags&net.FlagLoopback != 0 {
		return true
	}
	return name == "lo" || strings.HasPrefix(name, "lo:")
}

func interfaceStatus(flags net.Flags) string {
	if flags&net.FlagUp != 0 {
		return "UP"
	}
	return "DOWN"
}

func parseIPConfigs(addrs []net.Addr) []model.IPConfig {
	var configs []model.IPConfig
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip == nil {
			continue
		}
		ones, _ := ipNet.Mask.Size()
		version := "IPv4"
		if ip.To4() == nil {
			version = "IPv6"
		}
		configs = append(configs, model.IPConfig{
			Address: ip.String(),
			Prefix:  ones,
			Version: version,
			Source:  "Static",
			Enabled: true,
		})
	}
	return configs
}

func parseDefaultGateways(output []byte) map[string][]model.GatewayConfig {
	gateways := make(map[string][]model.GatewayConfig)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "default" {
			continue
		}
		gw := model.GatewayConfig{Enabled: true, Scope: "Global"}
		for i := 1; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				gw.Gateway = fields[i+1]
			case "dev":
				gw.Interface = fields[i+1]
			case "metric":
				if m, err := strconv.Atoi(fields[i+1]); err == nil {
					gw.Metric = m
				}
			}
		}
		if gw.Interface != "" && gw.Gateway != "" {
			gateways[gw.Interface] = append(gateways[gw.Interface], gw)
		}
	}
	return gateways
}

func loadDefaultGateways() map[string][]model.GatewayConfig {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}
	output, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return nil
	}
	return parseDefaultGateways(output)
}

func detectDHCPSource(ifaceName string, backend PersistBackend) string {
	if backend == nil || backend.Type() != BackendNetworkManager {
		return "Static"
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return "Static"
	}
	output, err := exec.Command("nmcli", "-g", "IP4.METHOD,IP6.METHOD", "device", "show", ifaceName).Output()
	if err != nil {
		return "Static"
	}
	methods := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, method := range methods {
		if strings.EqualFold(strings.TrimSpace(method), "auto") {
			return "DHCP"
		}
	}
	return "Static"
}

func applyRuntimeInterfaceConfig(iface model.NetworkInterface) error {
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}

	_ = exec.Command("ip", "addr", "flush", "dev", iface.Name).Run()

	for _, ip := range iface.IPConfigs {
		if !ip.Enabled {
			continue
		}
		if ip.Source == "DHCP" {
			if ip.Version == "IPv6" || strings.Contains(ip.Address, ":") {
				go exec.Command("dhclient", "-6", iface.Name).Run()
			} else {
				go exec.Command("dhclient", iface.Name).Run()
			}
			continue
		}
		if ip.Address == "" {
			continue
		}
		cidr := fmt.Sprintf("%s/%d", ip.Address, ip.Prefix)
		if err := exec.Command("ip", "addr", "add", cidr, "dev", iface.Name).Run(); err != nil {
			return fmt.Errorf("failed to add address %s: %w", cidr, err)
		}
	}

	if err := exec.Command("ip", "link", "set", iface.Name, "up").Run(); err != nil {
		return fmt.Errorf("failed to bring up %s: %w", iface.Name, err)
	}

	for _, gw := range iface.Gateways {
		if !gw.Enabled || gw.Gateway == "" {
			continue
		}
		args := []string{"route", "replace", "default", "via", gw.Gateway, "dev", iface.Name}
		if gw.Metric > 0 {
			args = append(args, "metric", strconv.Itoa(gw.Metric))
		}
		_ = exec.Command("ip", args...).Run()
	}
	return nil
}

func applyRuntimeStaticRoute(route model.StaticRoute) error {
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}
	args := []string{"route", "replace"}
	dest := route.Destination
	if route.Prefix > 0 || dest == "0.0.0.0" {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	args = append(args, dest)
	if route.Gateway != "" {
		args = append(args, "via", route.Gateway)
	}
	if route.Interface != "" {
		args = append(args, "dev", route.Interface)
	}
	if route.Metric > 0 {
		args = append(args, "metric", strconv.Itoa(route.Metric))
	}
	return exec.Command("ip", args...).Run()
}

func removeRuntimeStaticRoute(route model.StaticRoute) error {
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}
	args := []string{"route", "del"}
	dest := route.Destination
	if route.Prefix > 0 || dest == "0.0.0.0" {
		dest = fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
	}
	args = append(args, dest)
	if route.Gateway != "" {
		args = append(args, "via", route.Gateway)
	}
	if route.Interface != "" {
		args = append(args, "dev", route.Interface)
	}
	return exec.Command("ip", args...).Run()
}

func discoverInterfacesFromNet(backend PersistBackend) ([]model.NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	gatewayMap := loadDefaultGateways()
	dhcpDefault := backend != nil && backend.Type() == BackendNetworkManager

	var result []model.NetworkInterface
	for _, iface := range ifaces {
		if isLoopbackInterface(iface.Name, iface.Flags) {
			continue
		}

		ni := model.NetworkInterface{
			Name:    iface.Name,
			MAC:     iface.HardwareAddr.String(),
			Status:  interfaceStatus(iface.Flags),
			Enabled: true,
		}

		addrs, err := iface.Addrs()
		if err == nil {
			ni.IPConfigs = parseIPConfigs(addrs)
		}

		if gws, ok := gatewayMap[iface.Name]; ok {
			ni.Gateways = gws
		}

		if dhcpDefault {
			source := detectDHCPSource(iface.Name, backend)
			if source == "DHCP" {
				for i := range ni.IPConfigs {
					if ni.IPConfigs[i].Version == "IPv4" {
						ni.IPConfigs[i].Source = "DHCP"
					}
				}
			}
		}

		result = append(result, ni)
	}
	return result, nil
}
