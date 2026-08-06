package network

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// DarwinAdapter implements NetworkAdapter for macOS.
type DarwinAdapter struct{}

func (a *DarwinAdapter) GetInterfaces() ([]model.NetworkInterface, error) {
	return discoverInterfacesFromNet(nil)
}

func (a *DarwinAdapter) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	return applyRuntimeInterfaceConfig(iface)
}

func (a *DarwinAdapter) ApplyStaticRoute(route model.StaticRoute) error {
	if !route.Enabled {
		return nil
	}
	return applyDarwinStaticRoute(normalizeStaticRoute(route))
}

func (a *DarwinAdapter) RemoveStaticRoute(route model.StaticRoute) error {
	return removeDarwinStaticRoute(normalizeStaticRoute(route))
}

func (a *DarwinAdapter) GetRoutes() ([]model.StaticRoute, error) {
	cmd := exec.Command("netstat", "-rn", "-f", "inet")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseDarwinRoutes(output)
}

func (a *DarwinAdapter) ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error) {
	la := &LinuxAdapter{}
	return la.ValidateConnectivity(targets)
}

func (a *DarwinAdapter) BackendInfo() BackendInfo {
	return BackendInfo{Type: BackendIPRoute, Label: "macOS route"}
}

func parseDarwinRoutes(output []byte) ([]model.StaticRoute, error) {
	var routes []model.StaticRoute
	scanner := bufio.NewScanner(bytes.NewReader(output))
	headerSeen := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Routing tables") {
			continue
		}
		if strings.HasPrefix(line, "Destination") {
			headerSeen = true
			continue
		}
		if !headerSeen {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		destField := fields[0]
		gateway := fields[1]
		iface := fields[len(fields)-1]

		if shouldSkipDarwinRoute(destField, gateway) {
			continue
		}

		route := model.StaticRoute{Enabled: true}
		if destField == "default" {
			route.Destination = "0.0.0.0"
			route.Prefix = 0
		} else if strings.Contains(destField, "/") {
			_, ipNet, err := net.ParseCIDR(destField)
			if err != nil {
				continue
			}
			route.Destination = ipNet.IP.String()
			ones, _ := ipNet.Mask.Size()
			route.Prefix = ones
		} else if ip := net.ParseIP(destField); ip != nil {
			route.Destination = ip.String()
			route.Prefix = 32
		} else {
			dest, prefix := parseDarwinNetworkDestination(destField)
			if dest == "" {
				continue
			}
			route.Destination = dest
			route.Prefix = prefix
		}

		if net.ParseIP(gateway) != nil {
			route.Gateway = gateway
		} else if strings.HasPrefix(gateway, "link#") {
			continue
		} else {
			continue
		}

		if route.Interface == "" && iface != "" && !strings.HasPrefix(iface, "link#") {
			route.Interface = iface
		}

		if route.Gateway == "" && route.Interface == "" {
			continue
		}

		routes = append(routes, route)
	}

	return routes, scanner.Err()
}

func shouldSkipDarwinRoute(destField, gateway string) bool {
	if destField == "127" || strings.HasPrefix(destField, "127.") {
		return true
	}
	if destField == "169.254" || strings.HasPrefix(destField, "169.254.") {
		return true
	}
	if destField == "224.0.0/4" || destField == "224.0.0.0/4" || destField == "255.255.255.255" {
		return true
	}
	if strings.HasPrefix(gateway, "ff:") {
		return true
	}
	return false
}

func parseDarwinNetworkDestination(destField string) (string, int) {
	parts := strings.Split(destField, ".")
	switch len(parts) {
	case 1:
		return destField + ".0.0.0", 8
	case 2:
		return parts[0] + "." + parts[1] + ".0.0", 16
	case 3:
		return parts[0] + "." + parts[1] + "." + parts[2] + ".0", 24
	default:
		if ip := net.ParseIP(destField); ip != nil {
			return ip.String(), 32
		}
		return "", 0
	}
}

func applyDarwinStaticRoute(route model.StaticRoute) error {
	args := []string{"-n", "add"}
	scope := "-host"
	if route.Prefix < 32 {
		scope = "-net"
	}
	args = append(args, scope, darwinRouteDestination(route))
	if route.Gateway != "" {
		args = append(args, route.Gateway)
	}
	if route.Interface != "" {
		args = append(args, "-interface", route.Interface)
	}
	return exec.Command("route", args...).Run()
}

func removeDarwinStaticRoute(route model.StaticRoute) error {
	args := []string{"-n", "delete"}
	scope := "-host"
	if route.Prefix < 32 {
		scope = "-net"
	}
	args = append(args, scope, darwinRouteDestination(route))
	if route.Gateway != "" {
		args = append(args, route.Gateway)
	}
	if route.Interface != "" {
		args = append(args, "-interface", route.Interface)
	}
	return exec.Command("route", args...).Run()
}

func darwinRouteDestination(route model.StaticRoute) string {
	if route.Prefix == 0 || (route.Destination == "0.0.0.0" && route.Prefix == 0) {
		return "default"
	}
	if route.Prefix == 32 {
		return route.Destination
	}
	return fmt.Sprintf("%s/%d", route.Destination, route.Prefix)
}
