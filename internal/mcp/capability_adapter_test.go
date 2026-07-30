package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"
	"github.com/anviod/edgex/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestCapabilityToToolAndRegister(t *testing.T) {
	rt, err := capability.NewRuntime(capability.RuntimeConfig{
		AgentID:   "node-mcp",
		Protocols: []string{"modbus-tcp"},
	}, capability.NoopBus{})
	require.NoError(t, err)
	rt.SetMapper(execution.NewCapabilityMapper(nil))

	srv := mcp.NewMCPServer("test", "1.0.0")
	n := mcp.RegisterCapabilityTools(srv, rt, nil)
	require.GreaterOrEqual(t, n, 5)

	tools := srv.GetTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	require.True(t, names["ean_modbus_tcp_read_holding_register"])
	require.True(t, names["ean_system_diagnostics"])

	// tools/call via JSON-RPC path
	params, _ := json.Marshal(map[string]any{
		"name": "ean_modbus_tcp_read_holding_register",
		"arguments": map[string]any{
			"device_id": "d1",
			"address":   "40001",
		},
	})
	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  json.RawMessage(params),
	})
	resp := srv.HandleMessage(req)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)

	// Local invoke still works
	out := rt.Invoke(context.Background(), capability.InvokeRequest{
		Capability: "system.diagnostics",
	})
	require.Equal(t, capability.InvokeCompleted, out.Status)
}

func TestUnifiedCapabilityTools(t *testing.T) {
	rt, err := capability.NewRuntime(capability.RuntimeConfig{
		AgentID: "node-mcp",
		Unified: true,
	}, capability.NoopBus{})
	require.NoError(t, err)
	rt.SetMapper(execution.NewCapabilityMapper(nil))

	// Unified runtime generates 7 capabilities (not 63)
	require.Equal(t, 7, rt.Registry().Count())

	srv := mcp.NewMCPServer("test", "1.0.0")
	n := mcp.RegisterCapabilityTools(srv, rt, nil)
	require.Equal(t, 7, n)

	tools := srv.GetTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}

	// Unified tools have ean_ prefix (converted from capability ID)
	require.True(t, names["ean_read_points"], "missing ean_read_points")
	require.True(t, names["ean_write_points"], "missing ean_write_points")
	require.True(t, names["ean_scan_devices"], "missing ean_scan_devices")
	require.True(t, names["ean_list_points"], "missing ean_list_points")
	require.True(t, names["ean_get_diagnostics"], "missing ean_get_diagnostics")
	require.True(t, names["ean_ai_protocol_reverse"], "missing ean_ai_protocol_reverse")
	require.True(t, names["ean_ai_doc_parse"], "missing ean_ai_doc_parse")

	// Legacy protocol-specific tools should NOT exist
	require.False(t, names["ean_modbus_tcp_read_holding_register"], "legacy tool should not exist in unified mode")

	// tools/call via JSON-RPC path for unified tool
	params, _ := json.Marshal(map[string]any{
		"name": "ean_read_points",
		"arguments": map[string]any{
			"device_id": "d1",
			"address":   "40001",
		},
	})
	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  json.RawMessage(params),
	})
	resp := srv.HandleMessage(req)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)
}

func TestToolNameFromCapability(t *testing.T) {
	require.Equal(t, "ean_modbus_tcp_write_register",
		mcp.ToolNameFromCapability("modbus_tcp.write_register"))
}
