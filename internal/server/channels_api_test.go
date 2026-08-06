package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anviod/edgeCore/internal/core"
	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	_ "github.com/anviod/edgeCore/internal/driver/dlt645"
	_ "github.com/anviod/edgeCore/internal/driver/ethernetip"
	_ "github.com/anviod/edgeCore/internal/driver/ice104"
	_ "github.com/anviod/edgeCore/internal/driver/knxnetip"
	_ "github.com/anviod/edgeCore/internal/driver/mitsubishi"
	_ "github.com/anviod/edgeCore/internal/driver/modbus"
	_ "github.com/anviod/edgeCore/internal/driver/omron"
	_ "github.com/anviod/edgeCore/internal/driver/opcua"
	_ "github.com/anviod/edgeCore/internal/driver/s7"
	_ "github.com/anviod/edgeCore/internal/driver/snmp"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newChannelTestServer(t *testing.T) *Server {
	t.Helper()
	pipeline := core.NewDataPipeline(100)
	cm := core.NewChannelManager(pipeline, nil)
	return NewServer(cm, nil, pipeline, nil, nil, nil, nil, nil, nil)
}

func postChannel(t *testing.T, srv *Server, body any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	token := GenerateTestToken()
	req := httptest.NewRequest(http.MethodPost, "/api/channels", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	return resp
}

func TestAddChannel_API_ValidationErrors(t *testing.T) {
	srv := newChannelTestServer(t)

	t.Run("invalid json", func(t *testing.T) {
		token := GenerateTestToken()
		req := httptest.NewRequest(http.MethodPost, "/api/channels", bytes.NewReader([]byte("{")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := srv.app.Test(req, -1)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("missing id and name", func(t *testing.T) {
		resp := postChannel(t, srv, map[string]any{
			"protocol": "modbus-tcp",
			"config":   map[string]any{},
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("device with empty id", func(t *testing.T) {
		resp := postChannel(t, srv, map[string]any{
			"id":       "ch-bad-device",
			"name":     "Bad Device Channel",
			"protocol": "modbus-tcp",
			"config":   map[string]any{"url": "tcp://127.0.0.1:502"},
			"devices": []map[string]any{
				{"id": "", "name": ""},
			},
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestAddChannel_API_ProtocolBoundaries(t *testing.T) {
	srv := newChannelTestServer(t)

	cases := []struct {
		name     string
		protocol string
		config   map[string]any
		wantCode int
	}{
		{
			name:     "modbus-tcp empty config",
			protocol: "modbus-tcp",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "modbus-tcp with url",
			protocol: "modbus-tcp",
			config:   map[string]any{"url": "tcp://127.0.0.1:502"},
			wantCode: http.StatusOK,
		},
		{
			name:     "bacnet-ip empty config",
			protocol: "bacnet-ip",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "opc-ua empty config",
			protocol: "opc-ua",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "s7 empty config",
			protocol: "s7",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "ethernet-ip empty config",
			protocol: "ethernet-ip",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "omron-fins empty config",
			protocol: "omron-fins",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "knxnet-ip without ip",
			protocol: "knxnet-ip",
			config:   map[string]any{"port": 3671},
			wantCode: http.StatusOK,
		},
		{
			name:     "knxnet-ip with ip",
			protocol: "knxnet-ip",
			config:   map[string]any{"ip": "192.168.1.50", "port": 3671},
			wantCode: http.StatusOK,
		},
		{
			name:     "knxnet-ip discovery only",
			protocol: "knxnet-ip",
			config:   map[string]any{"discovery": true},
			wantCode: http.StatusOK,
		},
		{
			name:     "snmp empty config",
			protocol: "snmp",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "iec60870-5-104 empty config",
			protocol: "iec60870-5-104",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "dlt645 empty config",
			protocol: "dlt645",
			config:   map[string]any{},
			wantCode: http.StatusOK,
		},
		{
			name:     "mitsubishi without ip",
			protocol: "mitsubishi-slmp",
			config:   map[string]any{"port": 5000},
			wantCode: http.StatusOK,
		},
		{
			name:     "unknown protocol",
			protocol: "unknown-protocol",
			config:   map[string]any{},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "ch-" + tc.name
			resp := postChannel(t, srv, map[string]any{
				"id":       id,
				"name":     tc.name,
				"protocol": tc.protocol,
				"enable":   false,
				"config":   tc.config,
			})
			assert.Equal(t, tc.wantCode, resp.StatusCode, "protocol=%s", tc.protocol)
		})
	}
}

func TestAddChannel_API_DuplicateChannel(t *testing.T) {
	srv := newChannelTestServer(t)

	body := map[string]any{
		"id":       "ch-dup-api",
		"name":     "Duplicate API",
		"protocol": "modbus-tcp",
		"enable":   false,
		"config":   map[string]any{"url": "tcp://127.0.0.1:502"},
	}

	resp := postChannel(t, srv, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = postChannel(t, srv, body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestAddChannel_API_ModbusAutoPoints(t *testing.T) {
	srv := newChannelTestServer(t)

	resp := postChannel(t, srv, map[string]any{
		"id":       "ch-modbus-auto-api",
		"name":     "Modbus Auto Points",
		"protocol": "modbus-tcp",
		"enable":   false,
		"config":   map[string]any{"url": "tcp://127.0.0.1:502"},
		"devices": []map[string]any{
			{
				"id":     "dev-auto",
				"name":   "Auto Device",
				"config": map[string]any{"slave_id": 1, "auto_points_range": "0-5"},
				"points": []any{},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var ch model.Channel
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	require.Len(t, ch.Devices, 1)
	assert.NotEmpty(t, ch.Devices[0].Points)
}

func TestAddChannel_API_NilConfig(t *testing.T) {
	srv := newChannelTestServer(t)

	resp := postChannel(t, srv, map[string]any{
		"id":       "ch-nil-config-api",
		"name":     "Nil Config",
		"protocol": "knxnet-ip",
		"enable":   false,
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
