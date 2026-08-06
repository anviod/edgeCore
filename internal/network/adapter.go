package network

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// NetworkAdapter defines the interface for OS-specific network operations
type NetworkAdapter interface {
	ApplyInterfaceConfig(iface model.NetworkInterface) error
	ApplyStaticRoute(route model.StaticRoute) error
	RemoveStaticRoute(route model.StaticRoute) error
	GetInterfaces() ([]model.NetworkInterface, error)
	GetRoutes() ([]model.StaticRoute, error)
	ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error)
}

// NewNetworkAdapter creates a platform-specific network adapter
func NewNetworkAdapter() NetworkAdapter {
	switch runtime.GOOS {
	case "windows":
		return &WindowsAdapter{}
	case "darwin":
		return &DarwinAdapter{}
	default:
		return &LinuxAdapter{}
	}
}

// WindowsAdapter implements NetworkAdapter for Windows
type WindowsAdapter struct{}

func (a *WindowsAdapter) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	var v4Configs, v6Configs []model.IPConfig
	for _, ip := range iface.IPConfigs {
		if ip.Version == "IPv6" || strings.Contains(ip.Address, ":") {
			v6Configs = append(v6Configs, ip)
		} else {
			v4Configs = append(v4Configs, ip)
		}
	}

	// --- IPv4 Handling ---
	v4DHCP := false
	for _, ip := range v4Configs {
		if ip.Source == "DHCP" {
			v4DHCP = true
			break
		}
	}

	if v4DHCP {
		// Set DHCP
		cmd := exec.Command("netsh", "interface", "ip", "set", "address", iface.Name, "dhcp")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to enable IPv4 DHCP: %s, output: %s", err, output)
		}
		// Set DNS DHCP
		exec.Command("netsh", "interface", "ip", "set", "dns", iface.Name, "dhcp").Run()
	} else if len(v4Configs) > 0 {
		// Set Static
		// 1. Set the primary IP (this overwrites existing)
		primary := v4Configs[0]
		mask := net.CIDRMask(primary.Prefix, 32)
		maskStr := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

		gateway := ""
		var gwMetric int
		// Find IPv4 Gateway
		for _, gw := range iface.Gateways {
			if !strings.Contains(gw.Gateway, ":") {
				gateway = gw.Gateway
				gwMetric = gw.Metric
				break
			}
		}

		// syntax: set address "Name" static IP Mask Gateway Metric
		args := []string{"interface", "ip", "set", "address", iface.Name, "static", primary.Address, maskStr}
		if gateway != "" {
			args = append(args, gateway)
			if gwMetric > 0 {
				args = append(args, strconv.Itoa(gwMetric))
			}
		}

		cmd := exec.Command("netsh", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set static IP: %s, output: %s", err, output)
		}

		// Set Interface Metric if specified
		if iface.InterfaceMetric > 0 {
			exec.Command("netsh", "interface", "ip", "set", "interface", iface.Name, "metric="+strconv.Itoa(iface.InterfaceMetric)).Run()
		}

		// 2. Add additional IPs
		for i := 1; i < len(v4Configs); i++ {
			cfg := v4Configs[i]
			mask = net.CIDRMask(cfg.Prefix, 32)
			maskStr = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
			// syntax: add address "Name" IP Mask
			addCmd := exec.Command("netsh", "interface", "ip", "add", "address", iface.Name, cfg.Address, maskStr)
			if out, err := addCmd.CombinedOutput(); err != nil {
				fmt.Printf("Warning: failed to add secondary IP %s: %s\n", cfg.Address, out)
			}
		}
	}

	// --- IPv6 Handling ---
	v6DHCP := false
	for _, ip := range v6Configs {
		if ip.Source == "DHCP" {
			v6DHCP = true
			break
		}
	}

	if v6DHCP {
		exec.Command("netsh", "interface", "ipv6", "set", "address", iface.Name, "dhcp").Run()
		exec.Command("netsh", "interface", "ipv6", "set", "dns", iface.Name, "dhcp").Run()
	} else if len(v6Configs) > 0 {
		// Set Primary IPv6
		primary := v6Configs[0]
		// netsh interface ipv6 set address interface="Name" address=IP/Prefix
		cmd := exec.Command("netsh", "interface", "ipv6", "set", "address",
			fmt.Sprintf("interface=%s", iface.Name),
			fmt.Sprintf("address=%s/%d", primary.Address, primary.Prefix))
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: failed to set primary IPv6 %s: %s\n", primary.Address, out)
		}

		// Additional IPv6
		for i := 1; i < len(v6Configs); i++ {
			cfg := v6Configs[i]
			cmd := exec.Command("netsh", "interface", "ipv6", "add", "address",
				fmt.Sprintf("interface=%s", iface.Name),
				fmt.Sprintf("address=%s/%d", cfg.Address, cfg.Prefix))
			cmd.Run()
		}

		// Note: Gateways for IPv6 are typically handled via routes (prefix ::/0)
		// We could add default route here if gateway is specified in Gateways
		for _, gw := range iface.Gateways {
			if strings.Contains(gw.Gateway, ":") {
				// netsh interface ipv6 add route prefix=::/0 interface="Name" nexthop=Gateway
				args := []string{"interface", "ipv6", "add", "route", "prefix=::/0",
					fmt.Sprintf("interface=%s", iface.Name),
					fmt.Sprintf("nexthop=%s", gw.Gateway)}
				if gw.Metric > 0 {
					args = append(args, fmt.Sprintf("metric=%d", gw.Metric))
				}
				exec.Command("netsh", args...).Run()
			}
		}
	}

	return nil
}

func (a *WindowsAdapter) ApplyStaticRoute(route model.StaticRoute) error {
	isIPv6 := strings.Contains(route.Destination, ":")

	// route add Dest mask Mask Gateway metric Metric if Interface
	// IPv6 syntax: add route prefix=Prefix interface=Interface nexthop=Gateway metric=Metric

	if isIPv6 {
		// netsh interface ipv6 add route prefix=... interface=... nexthop=... metric=...
		netshArgs := []string{"interface", "ipv6", "add", "route",
			fmt.Sprintf("prefix=%s/%d", route.Destination, route.Prefix),
			fmt.Sprintf("interface=%s", route.Interface),
		}
		if route.Gateway != "" {
			netshArgs = append(netshArgs, fmt.Sprintf("nexthop=%s", route.Gateway))
		}
		if route.Metric > 0 {
			netshArgs = append(netshArgs, fmt.Sprintf("metric=%d", route.Metric))
		}

		cmd := exec.Command("netsh", netshArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add ipv6 route: %s, output: %s", err, output)
		}
		return nil
	}

	// IPv4 Logic
	// Use netsh for better interface name support
	netshArgs := []string{"interface", "ip", "add", "route",
		fmt.Sprintf("%s/%d", route.Destination, route.Prefix),
		route.Interface,
		route.Gateway}
	if route.Metric > 0 {
		netshArgs = append(netshArgs, "metric="+strconv.Itoa(route.Metric))
	}

	cmd := exec.Command("netsh", netshArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return nil
}

func (a *WindowsAdapter) RemoveStaticRoute(route model.StaticRoute) error {
	route = normalizeStaticRoute(route)
	isIPv6 := strings.Contains(route.Destination, ":")

	if isIPv6 {
		netshArgs := []string{"interface", "ipv6", "delete", "route",
			fmt.Sprintf("prefix=%s/%d", route.Destination, route.Prefix),
			fmt.Sprintf("interface=%s", route.Interface),
		}
		if route.Gateway != "" {
			netshArgs = append(netshArgs, fmt.Sprintf("nexthop=%s", route.Gateway))
		}
		cmd := exec.Command("netsh", netshArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete ipv6 route: %s, output: %s", err, output)
		}
		return nil
	}

	netshArgs := []string{"interface", "ip", "delete", "route",
		fmt.Sprintf("%s/%d", route.Destination, route.Prefix),
		route.Interface,
		route.Gateway,
	}
	cmd := exec.Command("netsh", netshArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete route: %s, output: %s", err, output)
	}
	return nil
}

type netshInfo struct {
	DHCPEnabled     bool
	Gateways        []string
	InterfaceMetric int
}

func (a *WindowsAdapter) GetInterfaces() ([]model.NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Gather extra info from netsh
	nameMap := make(map[string]netshInfo) // Interface Name -> Info

	cmd := exec.Command("netsh", "interface", "ip", "show", "config")
	output, _ := cmd.Output()

	// Simple parser for netsh output
	scanner := bufio.NewScanner(bytes.NewReader(output))
	var currentName string
	var currentGateways []string
	var currentDHCP bool
	var currentMetric int

	// Regexes
	reName := regexp.MustCompile(`Configuration for interface "(.*)"`)
	reNameCN := regexp.MustCompile(`接口 "(.*)" 的配置`)
	reIP := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			// End of block, save to map
			if currentName != "" {
				nameMap[currentName] = netshInfo{
					DHCPEnabled:     currentDHCP,
					Gateways:        currentGateways,
					InterfaceMetric: currentMetric,
				}
			}
			currentName = ""
			currentGateways = []string{}
			currentDHCP = false
			currentMetric = 0
			continue
		}

		// Match Interface Name
		if matches := reName.FindStringSubmatch(line); len(matches) > 1 {
			currentName = matches[1]
		} else if matches := reNameCN.FindStringSubmatch(line); len(matches) > 1 {
			currentName = matches[1]
		}

		// Match DHCP
		if strings.Contains(line, "DHCP") && (strings.Contains(line, "Yes") || strings.Contains(line, "是")) {
			currentDHCP = true
		}

		// Match Gateway
		if strings.Contains(line, "Default Gateway") || strings.Contains(line, "默认网关") {
			matches := reIP.FindStringSubmatch(line)
			if len(matches) > 1 {
				currentGateways = append(currentGateways, matches[1])
			}
		}

		// Match InterfaceMetric
		if strings.Contains(line, "InterfaceMetric") || strings.Contains(line, "接口跃点数") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				val := strings.TrimSpace(parts[len(parts)-1])
				if m, err := strconv.Atoi(val); err == nil {
					currentMetric = m
				}
			}
		}
	}
	// Flush last block
	if currentName != "" {
		nameMap[currentName] = netshInfo{
			DHCPEnabled:     currentDHCP,
			Gateways:        currentGateways,
			InterfaceMetric: currentMetric,
		}
	}

	var result []model.NetworkInterface
	for _, i := range ifaces {
		if isLoopbackInterface(i.Name, i.Flags) {
			continue
		}

		info, hasInfo := nameMap[i.Name]

		ni := model.NetworkInterface{
			Name:            i.Name,
			MAC:             i.HardwareAddr.String(),
			Status:          "DOWN",
			InterfaceMetric: info.InterfaceMetric,
			Enabled:         true,
		}
		if i.Flags&net.FlagUp != 0 {
			ni.Status = "UP"
		}

		addrs, _ := i.Addrs()

		for _, a := range addrs {
			ipStr := ""
			prefix := 0
			version := "IPv4"

			if ipnet, ok := a.(*net.IPNet); ok {
				ipStr = ipnet.IP.String()
				ones, _ := ipnet.Mask.Size()
				prefix = ones
				if ipnet.IP.To4() == nil {
					version = "IPv6"
				}
			}

			// Determine Source based on interface-level DHCP setting
			source := "Static"
			if hasInfo && info.DHCPEnabled && version == "IPv4" {
				source = "DHCP"
			}
			// For IPv6, netsh info parsing above only covers IPv4 ("interface ip").
			// IPv6 DHCP check would require "interface ipv6 show config".
			// For now, assume Static for IPv6 or extend parser.

			ni.IPConfigs = append(ni.IPConfigs, model.IPConfig{
				Address: ipStr,
				Prefix:  prefix,
				Version: version,
				Source:  source,
				Enabled: true,
			})
		}

		// Add Gateways from netsh info
		if hasInfo {
			seenGw := make(map[string]bool)
			for _, gw := range info.Gateways {
				if !seenGw[gw] && gw != "0.0.0.0" {
					ni.Gateways = append(ni.Gateways, model.GatewayConfig{
						Gateway:   gw,
						Interface: i.Name,
						Enabled:   true,
						Scope:     "Global",
						Metric:    0, // Gateway metric not easily parsed from this view, assume 0 or auto
					})
					seenGw[gw] = true
				}
			}
		}

		result = append(result, ni)
	}

	return result, nil
}

func (a *WindowsAdapter) GetRoutes() ([]model.StaticRoute, error) {
	routesV4, err := a.getRoutesByContext("ip")
	if err != nil {
		return nil, err
	}
	routesV6, err := a.getRoutesByContext("ipv6")
	if err != nil {
		// Just log error or ignore if IPv6 not supported?
		// Better to return partial results if IPv6 fails (e.g. disabled)
		// But for now let's just append what we have.
	}
	return append(routesV4, routesV6...), nil
}

func (a *WindowsAdapter) getRoutesByContext(context string) ([]model.StaticRoute, error) {
	// Using netsh interface {context} show route
	cmd := exec.Command("netsh", "interface", context, "show", "route")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var routes []model.StaticRoute
	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Publish") || strings.HasPrefix(line, "-----") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		// Logic differs slightly for IPv6 output?
		// IPv4: No Manual 1 0.0.0.0/0 14 192.168.1.1
		// IPv6: No Manual 256 ::/0 14 fe80::1
		// It seems consistent.

		prefixStr := fields[3]
		parts := strings.Split(prefixStr, "/")
		if len(parts) != 2 {
			continue
		}

		dest := parts[0]
		prefixLen, _ := strconv.Atoi(parts[1])
		metric, _ := strconv.Atoi(fields[2])

		routeType := fields[1]
		if routeType != "Manual" {
			continue
		}

		gatewayOrIface := strings.Join(fields[5:], " ")

		route := model.StaticRoute{
			Destination: dest,
			Prefix:      prefixLen,
			Metric:      metric,
			Enabled:     true,
		}

		if net.ParseIP(gatewayOrIface) != nil {
			route.Gateway = gatewayOrIface
		} else {
			route.Interface = gatewayOrIface
		}

		routes = append(routes, route)
	}

	return routes, nil
}

func (a *WindowsAdapter) ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error) {
	report := model.ConnectivityReport{
		Success: true,
		Details: []model.ConnectivityResult{},
	}

	for _, target := range targets {
		result := model.ConnectivityResult{
			Target:  target.Target,
			Success: false,
		}

		switch target.Type {
		case "gateway", "ip":
			// Ping
			cmd := exec.Command("ping", "-n", "1", "-w", strconv.Itoa(target.Timeout*1000), target.Target)
			if err := cmd.Run(); err == nil {
				result.Success = true
				result.Message = "Ping successful"
			} else {
				result.Message = "Ping failed"
				report.Success = false
			}
		case "http":
			// HTTP GET
			client := &http.Client{
				Timeout: time.Duration(target.Timeout) * time.Second,
			}
			resp, err := client.Get(target.Target)
			if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 400 {
				result.Success = true
				result.Message = fmt.Sprintf("HTTP check successful: %s", resp.Status)
				resp.Body.Close()
			} else {
				if err != nil {
					result.Message = fmt.Sprintf("HTTP check failed: %v", err)
				} else {
					result.Message = fmt.Sprintf("HTTP check failed: %s", resp.Status)
					resp.Body.Close()
				}
				report.Success = false
			}
		default:
			result.Message = "Unknown target type"
			report.Success = false
		}

		report.Details = append(report.Details, result)
	}

	return report, nil
}
