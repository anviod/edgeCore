package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertSparkplugBConfig_Returns200WithWarningWhenBrokerUnreachable(t *testing.T) {
	var saved model.NorthboundConfig
	nbm := core.NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})
	srv := NewServer(nil, nil, nil, nbm, nil, nil, nil, nil, nil)

	body := model.SparkplugBConfig{
		ID:       "api-spb-test",
		Name:     "API Sparkplug Test",
		Enable:   true,
		Broker:   "127.0.0.1",
		Port:     18884,
		ClientID: "api-spb-client",
		GroupID:  "group1",
		NodeID:   "node1",
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	token := GenerateTestToken()
	req := httptest.NewRequest(http.MethodPost, "/api/northbound/sparkplugb", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "api-spb-test", out["id"])
	warning, _ := out["warning"].(string)
	assert.Contains(t, warning, "配置已保存")
	assert.Len(t, saved.SparkplugB, 1)
}
