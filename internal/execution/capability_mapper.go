// Package execution provides Capability → Driver Command mapping for EAN 2.0.
package execution

import (
	"context"
	"fmt"
	"strings"

	"github.com/anviod/edgex/internal/capability"
)

// DriverCommand is the lower-level command produced by Capability mapping.
type DriverCommand struct {
	Protocol string
	Command  string // ReadPoints | WritePoint | ScanDevices | GetDevicePoints | Diagnostics | AI.*
	Args     map[string]any
}

// Executor executes a mapped DriverCommand against ScanEngine / Driver / AI adapters.
// Wiring to concrete southbound managers is intentionally deferred so the Capability
// Runtime can be brought up without changing ScanEngine hot paths.
type Executor interface {
	Execute(ctx context.Context, cmd DriverCommand) (any, error)
}

// CapabilityMapper maps EAN Capability Invokes to Driver Commands.
type CapabilityMapper struct {
	executor Executor
}

// NewCapabilityMapper creates a mapper. executor may be nil (returns mapping only errors on execute).
func NewCapabilityMapper(executor Executor) *CapabilityMapper {
	return &CapabilityMapper{executor: executor}
}

// Map converts a Capability Invoke into a DriverCommand without executing it.
// For unified capabilities (unified=true), protocol may be empty — the
// DriverExecutor will auto-resolve it from the channel's Protocol field.
func (m *CapabilityMapper) Map(req capability.InvokeRequest, cap capability.Capability) (DriverCommand, error) {
	protocol, _ := cap.Metadata["protocol"].(string)
	driverCmd, _ := cap.Metadata["driver_command"].(string)
	if driverCmd == "" {
		driverCmd = inferDriverCommand(cap.ID)
	}
	if driverCmd == "" {
		return DriverCommand{}, fmt.Errorf("cannot map capability %s to driver command", cap.ID)
	}
	args := map[string]any{}
	for k, v := range req.Arguments {
		args[k] = v
	}
	// For unified capabilities, allow explicit protocol override from args.
	// If not specified, DriverExecutor.resolveDeviceWithProtocol auto-resolves
	// from the channel's Protocol field via SouthboundManager.
	if protocol == "" {
		if p, ok := args["protocol"].(string); ok && p != "" {
			protocol = p
		}
	}
	return DriverCommand{
		Protocol: protocol,
		Command:  driverCmd,
		Args:     args,
	}, nil
}

// MapAndExecute implements capability.Mapper.
func (m *CapabilityMapper) MapAndExecute(ctx context.Context, req capability.InvokeRequest, cap capability.Capability) (any, error) {
	cmd, err := m.Map(req, cap)
	if err != nil {
		return nil, err
	}
	if m.executor == nil {
		return map[string]any{
			"mapped":   true,
			"protocol": cmd.Protocol,
			"command":  cmd.Command,
			"args":     cmd.Args,
			"note":     "executor not wired; mapping-only mode",
		}, nil
	}
	return m.executor.Execute(ctx, cmd)
}

func inferDriverCommand(capabilityID string) string {
	switch {
	// 协议特定能力（中性命名，v2.26）：read_point/write_point/scan_devices/list_points
	case strings.HasSuffix(capabilityID, ".read_point"):
		return "ReadPoints"
	case strings.HasSuffix(capabilityID, ".write_point"):
		return "WritePoint"
	case strings.HasSuffix(capabilityID, ".scan_devices"):
		return "ScanDevices"
	case strings.HasSuffix(capabilityID, ".list_points"):
		return "GetDevicePoints"
	// 兼容旧命名（过渡期，v2.26 前生成的 capability 仍可调用）
	case strings.HasSuffix(capabilityID, ".read_holding_register"):
		return "ReadPoints"
	case strings.HasSuffix(capabilityID, ".write_register"):
		return "WritePoint"
	case capabilityID == "system.diagnostics":
		return "Diagnostics"
	case capabilityID == "ai.protocol_reverse":
		return "AI.protocol_reverse"
	case capabilityID == "ai.doc_parse":
		return "AI.doc_parse"
	// Unified MCP tools (merged from 63 protocol-specific capabilities)
	case capabilityID == "read_points":
		return "ReadPoints"
	case capabilityID == "write_points":
		return "WritePoint"
	case capabilityID == "scan_devices":
		return "ScanDevices"
	case capabilityID == "list_points":
		return "GetDevicePoints"
	case capabilityID == "get_diagnostics":
		return "Diagnostics"
	case capabilityID == "ai_protocol_reverse":
		return "AI.protocol_reverse"
	case capabilityID == "ai_doc_parse":
		return "AI.doc_parse"
	default:
		return ""
	}
}
