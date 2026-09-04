package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anviod/edgeCore/internal/core"
	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDevice_API_BACnetScanPayload(t *testing.T) {
	pipeline := core.NewDataPipeline(100)
	cm := core.NewChannelManager(pipeline, nil)
	srv := NewServer(cm, nil, pipeline, nil, nil, nil, nil, nil, nil)

	ch := &model.Channel{
		ID:       "bacnet-ch",
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	payload := []map[string]any{{
		"id":       "bacnet-2228316",
		"name":     "RoomController.Simulator",
		"interval": "10s",
		"enable":   true,
		"config": map[string]any{
			"device_id":        2228316,
			"bacnet_device_id": 2228316,
			"ip":               "192.168.3.106",
			"port":             54103,
			"vendor_name":      "Test Vendor",
			"model_name":       "Room_FC_2014",
		},
		"points": []any{},
	}}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	token := GenerateTestToken()
	req := httptest.NewRequest(http.MethodPost, "/api/channels/bacnet-ch/devices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200, got %d", resp.StatusCode)
}

func TestAddDevice_API_BACnetScanPayload_DuplicateInstance(t *testing.T) {
	pipeline := core.NewDataPipeline(100)
	cm := core.NewChannelManager(pipeline, nil)
	srv := NewServer(cm, nil, pipeline, nil, nil, nil, nil, nil, nil)

	ch := &model.Channel{
		ID:       "bacnet-ch",
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	existing := map[string]any{
		"id":       "legacy-device-id",
		"name":     "Legacy Device",
		"interval": "10s",
		"enable":   true,
		"config": map[string]any{
			"bacnet_device_id": 2228316,
		},
		"points": []any{},
	}
	body1, _ := json.Marshal([]map[string]any{existing})
	token := GenerateTestToken()
	req1 := httptest.NewRequest(http.MethodPost, "/api/channels/bacnet-ch/devices", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token)
	resp1, err := srv.app.Test(req1, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	scanPayload := []map[string]any{{
		"id":       "bacnet-2228316",
		"name":     "Scanned Device",
		"interval": "10s",
		"enable":   true,
		"config": map[string]any{
			"bacnet_device_id": 2228316,
			"ip":               "192.168.3.106",
			"port":             54103,
		},
		"points": []any{},
	}}
	body2, _ := json.Marshal(scanPayload)
	req2 := httptest.NewRequest(http.MethodPost, "/api/channels/bacnet-ch/devices", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err := srv.app.Test(req2, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)
}
