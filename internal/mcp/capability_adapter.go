package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/capability"
)

// CapabilityToolPrefix namespaces auto-generated EAN tools so they do not collide
// with hand-written JSON-RPC tools (list_channels, create_device, etc.).
const CapabilityToolPrefix = "ean_"

// ToolNameFromCapability converts a Capability ID to an MCP tool name.
// Example: modbus_tcp.read_point → ean_modbus_tcp_read_point
func ToolNameFromCapability(capabilityID string) string {
	return CapabilityToolPrefix + strings.ReplaceAll(capabilityID, ".", "_")
}

// CapabilityToTool maps an EAN Capability Descriptor to an MCP Tool definition.
func CapabilityToTool(cap capability.Capability) Tool {
	return Tool{
		Name:        ToolNameFromCapability(cap.ID),
		Description: fmt.Sprintf("[EAN Capability] %s", firstNonEmpty(cap.Description, cap.ID)),
		InputSchema: schemaToInputSchema(cap.InputSchema),
	}
}

// RegisterCapabilityTools registers Capability→Tool handlers on an MCP server.
// Tool calls are dispatched through capability.Runtime.Invoke (not raw JSON-RPC
// business handlers), keeping MCP protocol concerns in this package.
// writeGate, when non-nil, is invoked before write/admin capabilities; a non-nil
// result short-circuits the call (used for MCP full-access confirmation).
func RegisterCapabilityTools(srv *MCPServer, rt *capability.Runtime, writeGate func() *CallToolResult) int {
	if srv == nil || rt == nil {
		return 0
	}
	caps := rt.Registry().List()
	for _, cap := range caps {
		cap := cap
		tool := CapabilityToTool(cap)
		capabilityID := cap.ID
		writeLike := cap.Permission == capability.PermissionWrite ||
			cap.Permission == capability.PermissionReadWrite ||
			cap.Permission == capability.PermissionAdmin

		srv.RegisterTool(tool, func(args json.RawMessage) (*CallToolResult, error) {
			if writeLike && writeGate != nil {
				if blocked := writeGate(); blocked != nil {
					return blocked, nil
				}
			}
			arguments := map[string]any{}
			if len(args) > 0 && string(args) != "null" {
				if err := json.Unmarshal(args, &arguments); err != nil {
					return NewErrorResult("参数解析失败: " + err.Error()), nil
				}
			}
			resp := rt.Invoke(context.Background(), capability.InvokeRequest{
				Target:     rt.Registry().AgentID(),
				Capability: capabilityID,
				Arguments:  arguments,
			})
			payload, _ := json.MarshalIndent(resp, "", "  ")
			if resp.Status != capability.InvokeCompleted || !resp.Result.Success {
				return NewErrorResult(fmt.Sprintf("Capability invoke %s: %s\n```json\n%s\n```",
					resp.Status, firstNonEmpty(resp.Result.Error, string(resp.Status)), string(payload))), nil
			}
			return NewSuccessResult("```json\n" + string(payload) + "\n```"), nil
		})
	}
	return len(caps)
}

func schemaToInputSchema(schema map[string]any) InputSchema {
	out := InputSchema{
		Type:       "object",
		Properties: map[string]PropertyDef{},
	}
	if schema == nil {
		return out
	}
	if t, ok := schema["type"].(string); ok && t != "" {
		out.Type = t
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name, raw := range props {
			out.Properties[name] = propertyFromAny(raw)
		}
	}
	switch req := schema["required"].(type) {
	case []string:
		out.Required = append([]string{}, req...)
	case []any:
		for _, item := range req {
			if s, ok := item.(string); ok {
				out.Required = append(out.Required, s)
			}
		}
	}
	return out
}

func propertyFromAny(raw any) PropertyDef {
	def := PropertyDef{Type: "string"}
	m, ok := raw.(map[string]any)
	if !ok {
		return def
	}
	if t, ok := m["type"].(string); ok {
		def.Type = t
	}
	if d, ok := m["description"].(string); ok {
		def.Description = d
	}
	if d, ok := m["default"]; ok {
		def.Default = d
	}
	switch enums := m["enum"].(type) {
	case []string:
		def.Enum = append([]string{}, enums...)
	case []any:
		for _, item := range enums {
			if s, ok := item.(string); ok {
				def.Enum = append(def.Enum, s)
			}
		}
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
