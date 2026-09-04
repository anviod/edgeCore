package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/config"
	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTAuth_NoToken(t *testing.T) {
	srv := newChannelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	srv := newChannelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestJWTAuth_QueryParamToken(t *testing.T) {
	srv := newChannelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/channels?token="+GenerateTestToken(), nil)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestJWT_CreateAndParseToken(t *testing.T) {
	j := NewJWT()
	claims := CustomClaims{
		Name:  "test-user",
		Email: "test@example.com",
	}
	token, err := j.CreateToken(claims)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	parsed, err := j.ParserToken(token)
	require.NoError(t, err)
	assert.Equal(t, "test-user", parsed.Name)
}

func TestGenerateNonce(t *testing.T) {
	nonce, err := GenerateNonce()
	require.NoError(t, err)
	assert.Len(t, nonce, 32)

	// Valid nonce should be consumable exactly once
	assert.True(t, ValidateAndConsumeNonce(nonce))
	assert.False(t, ValidateAndConsumeNonce(nonce))
}

func TestGenerateNonce_Expired(t *testing.T) {
	nonce, err := GenerateNonce()
	require.NoError(t, err)

	// Manually expire the nonce
	nonceStore.Store(nonce, time.Now().Add(-1*time.Minute))
	assert.False(t, ValidateAndConsumeNonce(nonce))
}

func TestLoginFailProtection(t *testing.T) {
	ip := "192.168.1.100"
	ClearLoginFail(ip)
	defer ClearLoginFail(ip)

	blocked, _ := IsIPBlocked(ip)
	assert.False(t, blocked)

	for i := 0; i < MaxLoginFailCount; i++ {
		AddLoginFail(ip)
	}

	blocked, wait := IsIPBlocked(ip)
	assert.True(t, blocked)
	assert.Greater(t, wait, time.Duration(0))

	ClearLoginFail(ip)
	blocked, _ = IsIPBlocked(ip)
	assert.False(t, blocked)
}

func TestHandleGetNonce(t *testing.T) {
	srv := newChannelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/nonce", nil)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, data["nonce"])
}

func TestHandleGetSystemInfo(t *testing.T) {
	tmpDir := testOutputDir(t)
	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	sm := core.NewSystemManager(cfg)

	srv := NewServer(nil, nil, nil, nil, nil, sm, nil, cfgManager, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/system-info", nil)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
}

func TestHandleLogin_LocalDebugMode(t *testing.T) {
	tmpDir := testOutputDir(t)
	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	cfg.Users = []model.UserConfig{
		{Username: "admin", Password: "Admin@12345", Role: "admin"},
	}
	sm := core.NewSystemManager(cfg)

	srv := NewServer(nil, nil, nil, nil, nil, sm, nil, cfgManager, nil)

	body, _ := json.Marshal(map[string]any{
		"loginFlag": true,
		"loginType": "local",
		"data": map[string]string{
			"username": "admin",
			"password": "Admin@12345",
			"nonce":    "",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "0", result["code"])
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	tmpDir := testOutputDir(t)
	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	cfg.Users = []model.UserConfig{
		{Username: "admin", Password: "Admin@12345", Role: "admin"},
	}
	sm := core.NewSystemManager(cfg)

	srv := NewServer(nil, nil, nil, nil, nil, sm, nil, cfgManager, nil)

	body, _ := json.Marshal(map[string]any{
		"loginFlag": true,
		"loginType": "local",
		"data": map[string]string{
			"username": "admin",
			"password": "wrong-password",
			"nonce":    "",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "1", result["code"])
}

func TestHandleLogout(t *testing.T) {
	srv := newChannelTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleChangePassword_InvalidNonce(t *testing.T) {
	tmpDir := testOutputDir(t)
	cfgManager := config.NewConfigManagerWithEmptyConfig(tmpDir)
	cfg := cfgManager.GetConfig()
	cfg.Users = []model.UserConfig{
		{Username: "admin", Password: "Admin@12345", Role: "admin"},
	}
	sm := core.NewSystemManager(cfg)

	srv := NewServer(nil, nil, nil, nil, nil, sm, nil, cfgManager, nil)

	body, _ := json.Marshal(map[string]any{
		"oldPassword": "old",
		"newPassword": "new",
		"nonce":       "invalid-nonce",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+GenerateTestToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "1", result["code"])
}
