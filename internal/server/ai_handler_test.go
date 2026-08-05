package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func aiPOST(t *testing.T, srv *Server, body any) *http.Response {
	t.Helper()
	token := GenerateTestToken()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	return resp
}

func TestGetAiStatus(t *testing.T) {
	srv := newChannelTestServer(t)

	token := GenerateTestToken()
	req := httptest.NewRequest(http.MethodGet, "/api/ai/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "remote", data["mode"])
	assert.Equal(t, true, data["enabled"])
}

func TestPostAiChat_ChannelQuery(t *testing.T) {
	srv := newChannelTestServer(t)

	resp := aiPOST(t, srv, map[string]any{
		"message": "查看通道状态",
		"context": map[string]any{"route": "/channels"},
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	reply, _ := data["reply"].(string)
	assert.Contains(t, reply, "通道")
}

func TestPostAiChat_EmptyMessage(t *testing.T) {
	srv := newChannelTestServer(t)

	resp := aiPOST(t, srv, map[string]any{"message": "  "})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
