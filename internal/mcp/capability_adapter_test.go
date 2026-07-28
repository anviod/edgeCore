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

func TestToolNameFromCapability(t *testing.T) {
	require.Equal(t, "ean_modbus_tcp_write_register",
		mcp.ToolNameFromCapability("modbus_tcp.write_register"))
}
