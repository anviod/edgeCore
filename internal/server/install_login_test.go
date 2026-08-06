package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInstallTestServer(t *testing.T) (*Server, *core.SystemManager, string) {
	t.Helper()

	tmpDir := testOutputDir(t)
	dbPath := filepath.Join(tmpDir, "data")

	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	sm := core.NewSystemManager(cfg)

	store, err := storage.NewStorage(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	pipeline := core.NewDataPipeline(10)
	cm := core.NewChannelManager(pipeline, nil)
	srv := NewServer(cm, store, pipeline, nil, nil, sm, nil, cfgManager, nil, nil)

	return srv, sm, tmpDir
}

func TestInstallStorageAttachHook(t *testing.T) {
	tmpDir := testOutputDir(t)
	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	sm := core.NewSystemManager(cfg)

	pipeline := core.NewDataPipeline(10)
	cm := core.NewChannelManager(pipeline, nil)
	srv := NewServer(cm, nil, pipeline, nil, nil, sm, nil, cfgManager, nil, nil)

	var attached *storage.Storage
	srv.SetStorageAttachHook(func(st *storage.Storage) {
		attached = st
	})

	installCfg := &model.InstallConfig{
		Port:            8080,
		Username:        "admin",
		Password:        "Admin@12345",
		StoragePath:     "data",
		GatewayName:     "test-gateway",
		GatewayLocation: "lab",
	}
	srv.executeInstall(installCfg)

	require.NotNil(t, attached, "storage attach hook should run after install")
	require.NotNil(t, srv.storage)
}

func TestInstallSyncsUserForLogin(t *testing.T) {
	srv, sm, _ := newInstallTestServer(t)

	cfg := &model.InstallConfig{
		Port:            8082,
		Username:        "admin",
		Password:        "Admin@12345",
		StoragePath:     "data",
		GatewayName:     "test-gateway",
		GatewayLocation: "lab",
	}

	srv.executeInstall(cfg)

	user, found := sm.GetUser("admin")
	require.True(t, found, "admin user should be available in SystemManager after install")
	assert.Equal(t, "Admin@12345", user.Password)
	assert.Equal(t, "admin", user.Role)

	installed, err := srv.checkIfInstalled()
	require.NoError(t, err)
	assert.True(t, installed)
}

func TestLoginAfterInstall(t *testing.T) {
	srv, _, _ := newInstallTestServer(t)

	installCfg := &model.InstallConfig{
		Port:            8080,
		Username:        "admin",
		Password:        "Admin@12345",
		StoragePath:     "data",
		GatewayName:     "test-gateway",
		GatewayLocation: "lab",
	}
	srv.executeInstall(installCfg)

	app := fiber.New()
	app.Post("/api/auth/login", srv.handleLogin)

	body, err := json.Marshal(LoginRequest{
		LoginFlag: true,
		LoginType: "local",
	})
	require.NoError(t, err)

	// fill nested data field
	var reqMap map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &reqMap))
	reqMap["data"] = map[string]string{
		"username": "admin",
		"password": "Admin@12345",
		"nonce":    "",
	}
	body, err = json.Marshal(reqMap)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "0", result["code"], "login should succeed after install, got: %s", string(respBody))
}

func TestCheckPortCurrentListenPortAvailable(t *testing.T) {
	srv, _, _ := newInstallTestServer(t)
	srv.listenAddr = ":8080"

	app := fiber.New()
	app.Get("/api/install/check-port", srv.checkPort)

	req := httptest.NewRequest("GET", "/api/install/check-port?port=8080", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		Code string `json:"code"`
		Data struct {
			Available bool `json:"available"`
			Port      int  `json:"port"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "0", result.Code)
	assert.True(t, result.Data.Available)
	assert.Equal(t, 8080, result.Data.Port)
}

func TestCheckInstallStatusAfterInstall(t *testing.T) {
	srv, _, _ := newInstallTestServer(t)

	cfg := &model.InstallConfig{
		Port:            8080,
		Username:        "admin",
		Password:        "Admin@12345",
		StoragePath:     "data",
		GatewayName:     "test-gateway",
		GatewayLocation: "lab",
	}
	srv.executeInstall(cfg)

	app := fiber.New()
	app.Get("/api/install/status", srv.checkInstallStatus)

	req := httptest.NewRequest("GET", "/api/install/status", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		Code string `json:"code"`
		Data struct {
			IsInstalled bool `json:"isInstalled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "0", result.Code)
	assert.True(t, result.Data.IsInstalled)
}
