package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/ai_agent"
	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/execution"
	"github.com/stretchr/testify/require"
)

func TestAIAdapterProtocolReverseAndDocParse(t *testing.T) {
	adapter := execution.NewAIAdapter(ai_agent.NewAgent("local"))
	exec := execution.NewDriverExecutor(nil)
	exec.SetAI(adapter)

	out, err := exec.Execute(context.Background(), execution.DriverCommand{
		Command: "AI.protocol_reverse",
		Args: map[string]any{
			"payload": map[string]any{
				"protocol_id": "modbus-tcp",
				"filename":    "sample.pcap",
			},
			"wait":             true,
			"wait_timeout_sec": 10,
		},
	})
	require.NoError(t, err)
	m, ok := out.(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, m["task_id"])
	require.Equal(t, "protocol-reverse", m["skill"])
	require.Equal(t, "waiting_confirm", m["status"])
	require.NotNil(t, m["deliverables"])

	out2, err := exec.Execute(context.Background(), execution.DriverCommand{
		Command: "AI.doc_parse",
		Args: map[string]any{
			"payload": map[string]any{
				"filename": "manual.pdf",
				"protocol": "modbus-tcp",
			},
			"wait":             true,
			"wait_timeout_sec": 10,
		},
	})
	require.NoError(t, err)
	m2 := out2.(map[string]any)
	require.Equal(t, "doc-parse", m2["skill"])
	require.Equal(t, "waiting_confirm", m2["status"])
}

func TestAIAdapterViaCapabilityMapper(t *testing.T) {
	agent := capability.Agent{ID: "node-1", Kind: capability.AgentKindDevice, Version: "2.0.0"}
	reg := capability.NewRegistry(agent)
	require.NoError(t, reg.RegisterAll(capability.GenerateDefaultCapabilities("node-1", nil)))

	exec := execution.NewWiredExecutor(nil)
	disp := capability.NewDispatcher(reg)
	disp.SetMapper(execution.NewCapabilityMapper(exec))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp := disp.Dispatch(ctx, capability.InvokeRequest{
		Capability: "ai.protocol_reverse",
		Arguments: map[string]any{
			"payload": map[string]any{
				"protocol_id": "s7",
				"filename":    "s7.pcap",
			},
			"wait":             true,
			"wait_timeout_sec": 10,
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)
	require.NotNil(t, resp.Result.Values)
}

func TestDriverExecutorAIStillErrorsWithoutAdapter(t *testing.T) {
	_, err := execution.NewDriverExecutor(nil).Execute(context.Background(), execution.DriverCommand{
		Command: "AI.doc_parse",
		Args:    map[string]any{"payload": map[string]any{"filename": "x.pdf"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "AI adapter")
}
