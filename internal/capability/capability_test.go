package capability_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"
	"github.com/stretchr/testify/require"
)

func TestNormalizeProtocolID(t *testing.T) {
	require.Equal(t, "modbus_tcp", capability.NormalizeProtocolID("modbus-tcp"))
	require.Equal(t, "iec60870_5_104", capability.NormalizeProtocolID("iec60870-5-104"))
}

func TestGenerateDefaultCapabilities(t *testing.T) {
	caps := capability.GenerateDefaultCapabilities("edgex-node-001", []string{"modbus-tcp"})
	require.GreaterOrEqual(t, len(caps), 5) // 4 device + system/ai

	ids := map[string]bool{}
	for _, c := range caps {
		ids[c.ID] = true
		require.Equal(t, "edgex-node-001", c.AgentID)
	}
	require.True(t, ids["modbus_tcp.read_point"])
	require.True(t, ids["modbus_tcp.write_point"])
	require.True(t, ids["system.diagnostics"])
	require.True(t, ids["ai.protocol_reverse"])
}

func TestGenerateUnifiedCapabilities(t *testing.T) {
	caps := capability.GenerateUnifiedCapabilities("edgex-node-001")
	require.Equal(t, 7, len(caps), "expected 7 unified capabilities")

	ids := map[string]bool{}
	for _, c := range caps {
		ids[c.ID] = true
		require.Equal(t, "edgex-node-001", c.AgentID)
		require.NotEmpty(t, c.Description)
		require.NotEmpty(t, c.Metadata["driver_command"])
	}
	require.True(t, ids["read_points"], "missing read_points")
	require.True(t, ids["write_points"], "missing write_points")
	require.True(t, ids["scan_devices"], "missing scan_devices")
	require.True(t, ids["list_points"], "missing list_points")
	require.True(t, ids["get_diagnostics"], "missing get_diagnostics")
	require.True(t, ids["ai_protocol_reverse"], "missing ai_protocol_reverse")
	require.True(t, ids["ai_doc_parse"], "missing ai_doc_parse")
}

func TestRegistryAndDispatcher(t *testing.T) {
	agent := capability.Agent{ID: "node-1", Kind: capability.AgentKindDevice, Version: "2.0.0"}
	reg := capability.NewRegistry(agent)
	require.NoError(t, reg.RegisterAll(capability.GenerateProtocolCapabilities("node-1", "modbus-tcp")))
	require.NoError(t, reg.RegisterAll(capability.GenerateSystemCapabilities("node-1")))

	disp := capability.NewDispatcher(reg)
	disp.SetMapper(execution.NewCapabilityMapper(nil))

	// Test legacy protocol-specific capability
	resp := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "modbus_tcp.read_point",
		Arguments: map[string]any{
			"device_id": "slave-1",
			"address":   "40001",
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)

	missing := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "unknown.capability",
	})
	require.Equal(t, capability.InvokeRejected, missing.Status)
	require.Equal(t, "E009", missing.Result.ErrorCode)
}

func TestRegistryAndDispatcherUnified(t *testing.T) {
	agent := capability.Agent{ID: "node-1", Kind: capability.AgentKindDevice, Version: "2.0.0"}
	reg := capability.NewRegistry(agent)
	require.NoError(t, reg.RegisterAll(capability.GenerateUnifiedCapabilities("node-1")))

	disp := capability.NewDispatcher(reg)
	disp.SetMapper(execution.NewCapabilityMapper(nil))

	// Test unified read_points
	resp := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "read_points",
		Arguments: map[string]any{
			"device_id": "slave-1",
			"address":   "40001",
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)

	// Test unified write_points
	resp = disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "write_points",
		Arguments: map[string]any{
			"device_id": "slave-1",
			"address":   "40001",
			"value":     25.5,
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)

	// Test unified get_diagnostics
	resp = disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "get_diagnostics",
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)
}

func TestRuntimeDiscoveryAndInvokeViaBus(t *testing.T) {
	bus := capability.NewMemoryBus()
	rt, err := capability.NewRuntime(capability.RuntimeConfig{
		AgentID:              "edgex-node-001",
		HeartbeatIntervalSec: 60,
		Protocols:            []string{"modbus-tcp"},
	}, bus)
	require.NoError(t, err)
	rt.SetMapper(execution.NewCapabilityMapper(nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, rt.Start(ctx))
	defer rt.Stop()

	published := bus.Published()
	topics := map[string]bool{}
	for _, m := range published {
		topics[m.Topic] = true
	}
	require.True(t, topics[capability.TopicDiscoveryAgent])
	require.True(t, topics[capability.TopicDiscoveryCapability])
	require.True(t, topics[capability.TopicHeartbeat("edgex-node-001")])

	// Invoke over bus
	req := capability.NewEnvelope("edgeos-planner", capability.MessageTypeInvokeCapability, capability.InvokeRequest{
		InvokeID:   "invoke-test-1",
		Target:     "edgex-node-001",
		Capability: "modbus_tcp.write_point",
		Arguments: map[string]any{
			"device_id": "slave-1",
			"address":   "40001",
			"value":     25.5,
		},
	})
	req.Header.CorrelationID = "corr-1"
	payload, err := json.Marshal(req)
	require.NoError(t, err)

	require.NoError(t, bus.Publish(capability.TopicInvoke("edgex-node-001"), payload, 1))

	// Allow handler to reply
	deadline := time.Now().Add(2 * time.Second)
	var replyFound bool
	for time.Now().Before(deadline) {
		for _, m := range bus.Published() {
			if m.Topic == capability.TopicReply("edgeos-planner") {
				var msg capability.Message
				require.NoError(t, json.Unmarshal(m.Payload, &msg))
				require.Equal(t, capability.MessageTypeInvokeResponse, msg.Header.MessageType)
				body, err := capability.DecodeBody[capability.InvokeResponse](msg.Body)
				require.NoError(t, err)
				require.Equal(t, "invoke-test-1", body.InvokeID)
				require.Equal(t, capability.InvokeCompleted, body.Status)
				replyFound = true
				break
			}
		}
		if replyFound {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, replyFound, "expected invoke reply on $edgeos/reply/edgeos-planner")
}

func TestV1CompatTopicsPreserved(t *testing.T) {
	require.Equal(t, "edgex/nodes/register", capability.V1.NodesRegister())
	require.Equal(t, "edgex/nodes/gw1/offline", capability.V1.NodeOffline("gw1"))
	require.Equal(t, "edgex/cmd/gw1/dev1/write", capability.V1.CmdWrite("gw1", "dev1"))
	require.Equal(t, "edgex/heartbeat/gw1", capability.V1.Heartbeat("gw1"))
}

func TestEventPublisher(t *testing.T) {
	bus := capability.NewMemoryBus()
	pub := capability.NewEventPublisher("edgex-node-001", bus)
	require.NoError(t, pub.PublishPointChanged("slave-1", "temperature", 45.2, 42.1, map[string]any{"quality": "good"}))

	msgs := bus.Published()
	require.NotEmpty(t, msgs)
	require.Equal(t, capability.TopicEvent("edgex-node-001"), msgs[0].Topic)

	var msg capability.Message
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &msg))
	evt, err := capability.DecodeBody[capability.Event](msg.Body)
	require.NoError(t, err)
	require.Equal(t, "temperature.changed", evt.EventType)
	require.Equal(t, "slave-1", evt.DeviceID)
}

func TestCapabilityMapper(t *testing.T) {
	mapper := execution.NewCapabilityMapper(nil)
	cap := capability.Capability{
		ID:       "modbus_tcp.read_point",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"protocol": "modbus-tcp", "driver_command": "ReadPoints"},
	}
	cmd, err := mapper.Map(capability.InvokeRequest{
		Capability: cap.ID,
		Arguments:  map[string]any{"device_id": "d1", "address": "40001"},
	}, cap)
	require.NoError(t, err)
	require.Equal(t, "ReadPoints", cmd.Command)
	require.Equal(t, "modbus-tcp", cmd.Protocol)
}
