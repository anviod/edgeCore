package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAiSettings_Default(t *testing.T) {
	srv := newChannelTestServer(t)

	resp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/settings", nil), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "remote", data["deployment_mode"])
	assert.Equal(t, "edgex-center", data["provider"])
}

func TestPutAiSettings_RemoteMode(t *testing.T) {
	srv := newChannelTestServer(t)

	payload, err := json.Marshal(map[string]any{
		"deployment_mode": "remote",
		"provider":        "edgex-center",
		"grpc_endpoint":   "192.168.1.50:50051",
		"tokens_limit":    80000,
		"tasks_limit":     120,
	})
	require.NoError(t, err)

	resp, err := srv.app.Test(aiAuthRequest(http.MethodPut, "/api/ai/settings", payload), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])

	// Status should reflect remote mode
	statusResp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/status", nil), -1)
	require.NoError(t, err)
	defer statusResp.Body.Close()

	var statusBody map[string]any
	require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&statusBody))
	statusData := statusBody["data"].(map[string]any)
	assert.Equal(t, "remote", statusData["mode"])
}

func TestPutAiSettings_CloudRequiresEnableCloud(t *testing.T) {
	srv := newChannelTestServer(t)

	payload, err := json.Marshal(map[string]any{
		"deployment_mode": "cloud",
		"provider":        "openai",
		"base_url":        "https://api.openai.com/v1",
		"auth_type":       "bearer",
		"enable_cloud":    false,
	})
	require.NoError(t, err)

	resp, err := srv.app.Test(aiAuthRequest(http.MethodPut, "/api/ai/settings", payload), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPutAiSettings_PreservesAPIKey(t *testing.T) {
	srv := newChannelTestServer(t)

	first, err := json.Marshal(map[string]any{
		"deployment_mode": "cloud",
		"provider":        "openai",
		"base_url":        "https://api.openai.com/v1",
		"auth_type":       "bearer",
		"api_key":         "sk-test-secret",
		"enable_cloud":    true,
	})
	require.NoError(t, err)

	resp1, err := srv.app.Test(aiAuthRequest(http.MethodPut, "/api/ai/settings", first), -1)
	require.NoError(t, err)
	resp1.Body.Close()

	second, err := json.Marshal(map[string]any{
		"deployment_mode": "cloud",
		"provider":        "openai",
		"base_url":        "https://api.openai.com/v1",
		"auth_type":       "bearer",
		"api_key":         "",
		"api_key_set":     true,
		"enable_cloud":    true,
		"model":           "gpt-4o",
	})
	require.NoError(t, err)

	resp2, err := srv.app.Test(aiAuthRequest(http.MethodPut, "/api/ai/settings", second), -1)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	getResp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/settings", nil), -1)
	require.NoError(t, err)
	defer getResp.Body.Close()

	var body map[string]any
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&body))
	data := body["data"].(map[string]any)
	assert.Equal(t, true, data["api_key_set"])
	assert.Equal(t, "gpt-4o", data["model"])
}
