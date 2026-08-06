package core

import (
	"fmt"
	"sync"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/network"
)

type SystemManager struct {
	config     *config.Config
	cfgManager *config.ConfigManager
	mu         sync.RWMutex
	mdnsServer *network.MDNSServer
	dnsProxy   *network.DNSProxy
	netManager *network.NetworkManager
}

// persist 持久化当前配置到数据库。
func (sm *SystemManager) persist() error {
	if sm.cfgManager == nil {
		return fmt.Errorf("config manager not attached")
	}
	return sm.cfgManager.SaveConfig(sm.config)
}

// SetConfigManager 注入配置管理器，使系统/用户配置持久化到数据库。
func (sm *SystemManager) SetConfigManager(cm *config.ConfigManager) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cfgManager = cm
}

func NewSystemManager(cfg *config.Config) *SystemManager {
	// Initialize with defaults if empty
	if cfg.System.Time.Mode == "" {
		cfg.System.Time.Mode = "manual"
		cfg.System.Time.Manual.Timezone = "Asia/Shanghai"
	}

	sm := &SystemManager{
		config:     cfg,
		mdnsServer: network.NewMDNSServer(),
		dnsProxy:   network.NewDNSProxy(),
		netManager: network.NewNetworkManager(),
	}
	sm.normalizeHostnameConfig()

	// Start network services (hostname discovery must run at startup, not only on settings save).
	sm.startHostnameServices()

	go sm.netManager.ApplyConfig(cfg.System.Network, cfg.System.Routes)

	return sm
}

func (sm *SystemManager) GetConfig() model.SystemConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	cfg := sm.config.System
	cfg.Hostname = sm.effectiveHostnameConfigLocked()
	return cfg
}

func (sm *SystemManager) UpdateConfig(newConfig model.SystemConfig) error {
	sm.mu.Lock()
	previousRoutes := append([]model.StaticRoute(nil), sm.config.System.Routes...)
	sm.config.System = newConfig
	if newConfig.Hostname.HTTPPort > 0 {
		sm.config.Server.Port = newConfig.Hostname.HTTPPort
	}

	if err := sm.persist(); err != nil {
		sm.mu.Unlock()
		return fmt.Errorf("failed to save system config: %v", err)
	}
	sm.mu.Unlock()

	go sm.applyConfig(newConfig, previousRoutes)

	return nil
}

func (sm *SystemManager) startHostnameServices() {
	sm.mu.RLock()
	hostnameCfg := sm.effectiveHostnameConfigLocked()
	sm.mu.RUnlock()

	if err := sm.mdnsServer.Start(hostnameCfg); err != nil {
		fmt.Printf("Error starting mDNS: %v\n", err)
	}
	if err := sm.dnsProxy.Start(hostnameCfg); err != nil {
		fmt.Printf("Error starting DNS proxy: %v\n", err)
	}
}

func (sm *SystemManager) applyConfig(cfg model.SystemConfig, previousRoutes []model.StaticRoute) {
	sm.startHostnameServices()

	if err := sm.netManager.ApplyConfigWithRouteSync(cfg.Network, cfg.Routes, previousRoutes, cfg.ConnectivityTargets); err != nil {
		fmt.Printf("Error updating network config: %v\n", err)
	}

	// TODO: Implement other system calls here
	// 1. Set System Time
	// 2. Configure HA/Keepalived

	fmt.Printf("System configuration applied: %+v\n", cfg)
}

func (sm *SystemManager) GetNetworkInterfaces() ([]model.NetworkInterface, error) {
	live, err := sm.netManager.GetInterfaces()
	if err != nil {
		return nil, err
	}
	sm.mu.RLock()
	configured := sm.config.System.Network
	sm.mu.RUnlock()
	return network.MergeConfiguredInterfaces(live, configured), nil
}

func (sm *SystemManager) GetNetworkBackendInfo() network.BackendInfo {
	return sm.netManager.GetBackendInfo()
}

func (sm *SystemManager) GetHostnameAccessStatus() network.HostnameAccessStatus {
	cfg := sm.effectiveHostnameConfig()
	return network.BuildHostnameAccessStatus(cfg, sm.mdnsServer.Status(), sm.dnsProxy.Status())
}

func (sm *SystemManager) ValidateConnectivity(targets []model.ConnectivityTarget) (model.ConnectivityReport, error) {
	if len(targets) == 0 {
		sm.mu.RLock()
		targets = sm.config.System.ConnectivityTargets
		sm.mu.RUnlock()
	}
	return sm.netManager.ValidateConnectivity(targets)
}

func (sm *SystemManager) GetUser(username string) (*model.UserConfig, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, u := range sm.config.Users {
		if u.Username == username {
			// Return a copy to prevent accidental modification
			userCopy := u
			return &userCopy, true
		}
	}
	return nil, false
}

func (sm *SystemManager) UpdateUserPassword(username, newPassword string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	found := false
	for i, u := range sm.config.Users {
		if u.Username == username {
			sm.config.Users[i].Password = newPassword
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user not found")
	}

	// Persist (DB-first, fallback to files)
	if err := sm.persist(); err != nil {
		return fmt.Errorf("failed to save system config: %v", err)
	}

	return nil
}

func (sm *SystemManager) GetRoutes() ([]model.StaticRoute, error) {
	return sm.netManager.GetRoutes()
}

func (sm *SystemManager) AddRoute(route model.StaticRoute) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	route = network.NormalizeStaticRoute(route)
	if route.Destination == "" {
		return fmt.Errorf("destination is required")
	}

	for _, existing := range sm.config.System.Routes {
		if network.RouteKey(existing) == network.RouteKey(route) {
			return fmt.Errorf("route already exists")
		}
	}

	if err := sm.netManager.ApplyStaticRoute(route); err != nil {
		return err
	}

	sm.config.System.Routes = append(sm.config.System.Routes, route)
	return sm.persist()
}

func (sm *SystemManager) DeleteRoute(route model.StaticRoute) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	route = network.NormalizeStaticRoute(route)
	key := network.RouteKey(route)
	if key == "" {
		return fmt.Errorf("invalid route")
	}

	var (
		found     bool
		removed   model.StaticRoute
		remaining []model.StaticRoute
	)
	for _, existing := range sm.config.System.Routes {
		if network.RouteKey(existing) == key {
			found = true
			removed = existing
			continue
		}
		remaining = append(remaining, existing)
	}
	if !found {
		if err := sm.netManager.RemoveStaticRoute(route); err != nil {
			return err
		}
		return nil
	}

	if err := sm.netManager.RemoveStaticRoute(removed); err != nil {
		return err
	}

	sm.config.System.Routes = remaining
	return sm.persist()
}

func (sm *SystemManager) normalizeHostnameConfig() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.normalizeHostnameConfigLocked()
}

func (sm *SystemManager) normalizeHostnameConfigLocked() {
	if sm.config.System.Hostname.Name == "" {
		sm.config.System.Hostname.Name = "edgeCore"
	}
	if sm.config.Server.Port > 0 {
		sm.config.System.Hostname.HTTPPort = sm.config.Server.Port
	} else if sm.config.System.Hostname.HTTPPort == 0 {
		sm.config.System.Hostname.HTTPPort = 8080
	}
	if sm.config.System.Hostname.HTTPSPort == 0 {
		sm.config.System.Hostname.HTTPSPort = 443
	}
}

func (sm *SystemManager) effectiveHostnameConfig() model.HostnameConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.effectiveHostnameConfigLocked()
}

func (sm *SystemManager) effectiveHostnameConfigLocked() model.HostnameConfig {
	cfg := sm.config.System.Hostname
	if cfg.Name == "" {
		cfg.Name = "edgeCore"
	}
	if sm.config.Server.Port > 0 {
		cfg.HTTPPort = sm.config.Server.Port
	} else if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 8080
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	return cfg
}
