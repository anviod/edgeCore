package network

import (
	"fmt"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// NetworkManager manages network configurations and operations
type NetworkManager struct {
	adapter NetworkAdapter
	mu      sync.RWMutex
}

// NewNetworkManager creates a new NetworkManager
func NewNetworkManager() *NetworkManager {
	return &NetworkManager{
		adapter: NewNetworkAdapter(),
	}
}

// ApplyConfig applies the given network configuration
func (nm *NetworkManager) ApplyConfig(interfaces []model.NetworkInterface, routes []model.StaticRoute) error {
	return nm.ApplyConfigWithRouteSync(interfaces, routes, nil, nil)
}

// ApplyConfigWithRouteSync applies network config and removes routes dropped from the previous config.
func (nm *NetworkManager) ApplyConfigWithRouteSync(interfaces []model.NetworkInterface, routes, previousRoutes []model.StaticRoute, validationTargets []model.ConnectivityTarget) error {
	if validationTargets != nil {
		return nm.applyConfigWithTransaction(interfaces, routes, previousRoutes, validationTargets)
	}
	nm.mu.Lock()
	defer nm.mu.Unlock()
	return nm.applyConfigLocked(interfaces, routes, previousRoutes)
}

// ApplyConfigWithTransaction applies the given network configuration with transaction support (rollback on failure)
func (nm *NetworkManager) ApplyConfigWithTransaction(interfaces []model.NetworkInterface, routes []model.StaticRoute, validationTargets []model.ConnectivityTarget) error {
	return nm.ApplyConfigWithRouteSync(interfaces, routes, nil, validationTargets)
}

func (nm *NetworkManager) applyConfigWithTransaction(interfaces []model.NetworkInterface, routes, previousRoutes []model.StaticRoute, validationTargets []model.ConnectivityTarget) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	oldInterfaces, err := nm.adapter.GetInterfaces()
	if err != nil {
		return fmt.Errorf("failed to snapshot interfaces: %v", err)
	}
	oldRoutes, err := nm.adapter.GetRoutes()
	if err != nil {
		return fmt.Errorf("failed to snapshot routes: %v", err)
	}

	applyErr := nm.applyConfigLocked(interfaces, routes, previousRoutes)

	if applyErr == nil && len(validationTargets) > 0 {
		report, err := nm.adapter.ValidateConnectivity(validationTargets)
		if err != nil {
			applyErr = fmt.Errorf("connectivity validation error: %v", err)
		} else if !report.Success {
			applyErr = fmt.Errorf("connectivity validation failed: %v", report)
		}
	}

	if applyErr != nil {
		fmt.Printf("Network transaction failed: %v. Rolling back...\n", applyErr)
		for _, iface := range oldInterfaces {
			if err := nm.adapter.ApplyInterfaceConfig(iface); err != nil {
				fmt.Printf("Rollback failed for interface %s: %v\n", iface.Name, err)
			}
		}
		for _, route := range oldRoutes {
			if err := nm.adapter.ApplyStaticRoute(route); err != nil {
				fmt.Printf("Rollback failed for route %s: %v\n", route.Destination, err)
			}
		}
		return applyErr
	}

	return nil
}

func (nm *NetworkManager) applyConfigLocked(interfaces []model.NetworkInterface, routes, previousRoutes []model.StaticRoute) error {
	for _, route := range routesToRemove(previousRoutes, routes) {
		if err := nm.adapter.RemoveStaticRoute(route); err != nil {
			return fmt.Errorf("failed to remove static route %s: %v", route.Destination, err)
		}
	}

	for _, iface := range interfaces {
		if err := nm.adapter.ApplyInterfaceConfig(iface); err != nil {
			return fmt.Errorf("failed to apply config for interface %s: %v", iface.Name, err)
		}
	}

	for _, route := range routes {
		if err := nm.adapter.ApplyStaticRoute(route); err != nil {
			return fmt.Errorf("failed to apply static route %s: %v", route.Destination, err)
		}
	}

	return nil
}

// GetInterfaces returns the current status of all interfaces
func (nm *NetworkManager) GetInterfaces() ([]model.NetworkInterface, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.adapter.GetInterfaces()
}

// ApplyStaticRoute applies a single static route immediately.
func (nm *NetworkManager) ApplyStaticRoute(route model.StaticRoute) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	return nm.adapter.ApplyStaticRoute(route)
}

// RemoveStaticRoute removes a single static route immediately.
func (nm *NetworkManager) RemoveStaticRoute(route model.StaticRoute) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	return nm.adapter.RemoveStaticRoute(route)
}

// GetRoutes returns the current static routes
func (nm *NetworkManager) GetRoutes() ([]model.StaticRoute, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.adapter.GetRoutes()
}

// GetBackendInfo returns the detected OS network backend.
func (nm *NetworkManager) GetBackendInfo() BackendInfo {
	switch adapter := nm.adapter.(type) {
	case *LinuxAdapter:
		return adapter.BackendInfo()
	case *DarwinAdapter:
		return adapter.BackendInfo()
	default:
		return BackendInfo{Type: "windows", Label: "Windows netsh"}
	}
}

// ValidateConnectivity runs connectivity checks against the given targets.
func (nm *NetworkManager) ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.adapter.ValidateConnectivity(targets)
}
