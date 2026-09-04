package core

import (
	"path/filepath"
	"testing"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func attachTestConfigManager(t *testing.T, sm *SystemManager, cfg *config.Config) {
	t.Helper()
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(filepath.Join(tmpDir, "data"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cfgMgr, err := config.NewConfigManagerWithDB(tmpDir, store.GetConfigDB())
	if err != nil {
		t.Fatalf("config manager: %v", err)
	}
	cfgMgr.Config = cfg
	sm.SetConfigManager(cfgMgr)
}

func TestNewSystemManager_Defaults(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)

	got := sm.GetConfig()
	if got.Time.Mode != "manual" {
		t.Fatalf("time mode = %q, want manual", got.Time.Mode)
	}
	if got.Hostname.Name != "edgeCore" {
		t.Fatalf("hostname = %q, want edgeCore", got.Hostname.Name)
	}
	if got.Hostname.HTTPPort != 8080 {
		t.Fatalf("http port = %d, want 8080", got.Hostname.HTTPPort)
	}
}

func TestSystemManager_GetUser(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Users = []model.UserConfig{
		{Username: "admin", Password: "old"},
	}
	sm := NewSystemManager(cfg)

	user, ok := sm.GetUser("admin")
	if !ok || user.Password != "old" {
		t.Fatalf("GetUser = %+v, ok=%v", user, ok)
	}
	if _, ok := sm.GetUser("missing"); ok {
		t.Fatal("missing user should not be found")
	}
}

func TestSystemManager_UpdateUserPassword(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Users = []model.UserConfig{{Username: "admin", Password: "old"}}
	sm := NewSystemManager(cfg)
	attachTestConfigManager(t, sm, cfg)

	if err := sm.UpdateUserPassword("missing", "x"); err == nil {
		t.Fatal("expected error for missing user")
	}
	if err := sm.UpdateUserPassword("admin", "new"); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	user, _ := sm.GetUser("admin")
	if user.Password != "new" {
		t.Fatalf("password = %q, want new", user.Password)
	}
}

func TestSystemManager_RoutesValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)
	attachTestConfigManager(t, sm, cfg)

	if err := sm.AddRoute(model.StaticRoute{}); err == nil {
		t.Fatal("empty destination should fail")
	}
	if err := sm.DeleteRoute(model.StaticRoute{}); err == nil {
		t.Fatal("invalid route delete should fail")
	}
}

func TestSystemManager_HostnameConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 9090
	sm := NewSystemManager(cfg)

	eff := sm.effectiveHostnameConfig()
	if eff.HTTPPort != 9090 {
		t.Fatalf("effective HTTPPort = %d, want 9090", eff.HTTPPort)
	}
	if eff.HTTPSPort != 443 {
		t.Fatalf("effective HTTPSPort = %d, want 443", eff.HTTPSPort)
	}
}

func TestSystemManager_GetBackendAndHostnameStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)

	info := sm.GetNetworkBackendInfo()
	if info.Label == "" {
		t.Fatal("backend info label should not be empty")
	}
	status := sm.GetHostnameAccessStatus()
	if status.Name == "" {
		t.Fatal("hostname status should include name")
	}
}

func TestSystemManager_PersistWithoutManager(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)
	if err := sm.persist(); err == nil {
		t.Fatal("persist without config manager should fail")
	}
}

func TestSystemManager_UpdateConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)
	attachTestConfigManager(t, sm, cfg)

	newCfg := sm.GetConfig()
	newCfg.Time.Mode = "ntp"
	newCfg.Hostname.Name = "edgeCore-test"
	if err := sm.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	got := sm.GetConfig()
	if got.Time.Mode != "ntp" || got.Hostname.Name != "edgeCore-test" {
		t.Fatalf("config not updated: %+v", got)
	}
}

func TestSystemManager_AddRoute_Duplicate(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.System.Routes = []model.StaticRoute{{Destination: "10.0.0.0/24", Gateway: "192.168.1.1"}}
	sm := NewSystemManager(cfg)
	attachTestConfigManager(t, sm, cfg)

	dup := model.StaticRoute{Destination: "10.0.0.0/24", Gateway: "192.168.1.1"}
	if err := sm.AddRoute(dup); err == nil {
		t.Fatal("expected duplicate route error")
	}
}

func TestSystemManager_ValidateConnectivity(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewSystemManager(cfg)

	report, err := sm.ValidateConnectivity(nil)
	if err != nil {
		t.Fatalf("ValidateConnectivity with defaults: %v", err)
	}
	if report.Details == nil {
		t.Fatal("report should include details slice")
	}
}
