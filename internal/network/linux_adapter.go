package network

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

type LinuxAdapter struct {
	backend PersistBackend
}

func NewLinuxAdapter() *LinuxAdapter {
	return &LinuxAdapter{
		backend: DetectPersistBackend(),
	}
}

func (a *LinuxAdapter) GetInterfaces() ([]model.NetworkInterface, error) {
	return discoverInterfacesFromNet(a.backend)
}

func (a *LinuxAdapter) ApplyInterfaceConfig(iface model.NetworkInterface) error {
	if err := applyRuntimeInterfaceConfig(iface); err != nil {
		return err
	}
	if a.backend == nil {
		return nil
	}
	if err := a.backend.ApplyInterfaceConfig(iface); err != nil {
		return fmt.Errorf("persist interface %s via %s: %w", iface.Name, a.backend.Type().Label(), err)
	}
	return nil
}

func (a *LinuxAdapter) ApplyStaticRoute(route model.StaticRoute) error {
	if !route.Enabled {
		return nil
	}
	route = normalizeStaticRoute(route)
	if err := applyRuntimeStaticRoute(route); err != nil {
		return err
	}
	if a.backend == nil {
		return nil
	}
	if err := a.backend.ApplyStaticRoute(route); err != nil {
		return fmt.Errorf("persist route via %s: %w", a.backend.Type().Label(), err)
	}
	return nil
}

func (a *LinuxAdapter) RemoveStaticRoute(route model.StaticRoute) error {
	route = normalizeStaticRoute(route)
	if err := removeRuntimeStaticRoute(route); err != nil {
		return err
	}
	return nil
}

func (a *LinuxAdapter) GetRoutes() ([]model.StaticRoute, error) {
	if _, err := exec.LookPath("ip"); err != nil {
		return nil, nil
	}
	cmd := exec.Command("ip", "route", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var routes []model.StaticRoute
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}

		route := model.StaticRoute{Enabled: true}

		if fields[0] == "default" {
			route.Destination = "0.0.0.0"
			route.Prefix = 0
		} else {
			_, ipNet, err := net.ParseCIDR(fields[0])
			if err == nil {
				route.Destination = ipNet.IP.String()
				ones, _ := ipNet.Mask.Size()
				route.Prefix = ones
			} else {
				route.Destination = fields[0]
				route.Prefix = 32
			}
		}

		for i := 1; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				route.Gateway = fields[i+1]
			case "dev":
				route.Interface = fields[i+1]
			case "metric":
				m, _ := strconv.Atoi(fields[i+1])
				route.Metric = m
			}
		}

		if fields[0] != "default" && route.Gateway == "" && route.Interface == "" {
			continue
		}
		routes = append(routes, route)
	}

	return routes, nil
}

func (a *LinuxAdapter) ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error) {
	report := model.ConnectivityReport{
		Success: true,
		Details: []model.ConnectivityResult{},
	}

	for _, target := range targets {
		result := model.ConnectivityResult{
			Target:  target.Target,
			Success: false,
		}
		timeout := target.Timeout
		if timeout <= 0 {
			timeout = 2
		}

		switch target.Type {
		case "gateway", "ip":
			cmd := exec.Command("ping", "-c", "1", "-W", strconv.Itoa(timeout), target.Target)
			if err := cmd.Run(); err == nil {
				result.Success = true
				result.Message = "Ping successful"
			} else {
				result.Message = "Ping failed"
				report.Success = false
			}
		case "http":
			client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
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

func (a *LinuxAdapter) BackendInfo() BackendInfo {
	if a.backend == nil {
		return GetBackendInfo()
	}
	return BackendInfo{Type: a.backend.Type(), Label: a.backend.Type().Label()}
}
