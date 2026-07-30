package execution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anviod/edgex/internal/model"
)

// PointReader is the optional live Driver read path (ChannelManager.ReadPoint).
type PointReader interface {
	ReadPoint(channelID, deviceID, pointID string) (model.Value, error)
}

// DeviceScanner is the optional ScanEngine browse path.
type DeviceScanner interface {
	ScanDevice(ctx context.Context, channelID, deviceID string, params map[string]any) (any, error)
	ScanChannel(ctx context.Context, channelID string, params map[string]any) (any, error)
}

// DiagnosticsProvider is the optional diagnostics path.
type DiagnosticsProvider interface {
	GetDeviceDiagnostics(deviceID string) map[string]any
}

// DriverExecutor executes mapped DriverCommands against ShadowCore / ScanEngine / Driver
// via SouthboundManager (typically *core.ChannelManager). AI.* commands delegate to AIAdapter.
type DriverExecutor struct {
	sb model.SouthboundManager
	ai AICommandExecutor
}

// NewDriverExecutor wires a real southbound backend. sb may be nil (Execute errors).
// AI commands require SetAI / NewWiredExecutor.
func NewDriverExecutor(sb model.SouthboundManager) *DriverExecutor {
	return &DriverExecutor{sb: sb}
}

// NewWiredExecutor builds a DriverExecutor with SharedAIAdapter attached.
func NewWiredExecutor(sb model.SouthboundManager) *DriverExecutor {
	e := NewDriverExecutor(sb)
	e.SetAI(SharedAIAdapter())
	return e
}

// SetAI attaches an AI command executor (protocol_reverse / doc_parse).
func (e *DriverExecutor) SetAI(ai AICommandExecutor) {
	if e == nil {
		return
	}
	e.ai = ai
}

// Execute runs a DriverCommand on the real I/O path.
func (e *DriverExecutor) Execute(ctx context.Context, cmd DriverCommand) (any, error) {
	if e == nil {
		return nil, fmt.Errorf("driver executor not wired")
	}
	switch cmd.Command {
	case "AI.protocol_reverse", "AI.doc_parse":
		if e.ai == nil {
			return nil, fmt.Errorf("AI command %s requires AI adapter (not wired on driver executor)", cmd.Command)
		}
		return e.ai.Execute(ctx, cmd)
	}
	if e.sb == nil {
		return nil, fmt.Errorf("driver executor not wired")
	}
	switch cmd.Command {
	case "ReadPoints":
		return e.readPoints(ctx, cmd)
	case "WritePoint":
		return e.writePoint(ctx, cmd)
	case "GetDevicePoints":
		return e.getDevicePoints(cmd)
	case "ScanDevices":
		return e.scanDevices(ctx, cmd)
	case "Diagnostics":
		return e.diagnostics(cmd)
	default:
		return nil, fmt.Errorf("unsupported driver command: %s", cmd.Command)
	}
}

func (e *DriverExecutor) readPoints(ctx context.Context, cmd DriverCommand) (any, error) {
	channelID, deviceID, protocol, err := e.resolveDeviceWithProtocol(cmd.Args)
	if err != nil {
		return nil, err
	}
	if cmd.Protocol == "" {
		cmd.Protocol = protocol
	}
	rawIDs := collectPointIDs(cmd.Args)
	if len(rawIDs) == 0 {
		return nil, fmt.Errorf("address, addresses, point_id or point_ids is required")
	}

	// Resolve raw identifiers (address, name, or point_id) to actual point IDs.
	// list_points returns PDU offsets as "address" (e.g. "0", "2"), but ReadPoint
	// expects the internal point ID (e.g. "pt_0723121000"). This resolution step
	// bridges the semantic gap so users can pass list_points output directly.
	// 将用户输入（可能是地址、名称或 point_id）解析为内部 point_id，
	// 使 list_points 的输出可直接作为 read_points 的输入。
	pointIDs, resolveMap := e.resolvePointIDs(channelID, deviceID, rawIDs)

	live := true
	if v, ok := cmd.Args["live"].(bool); ok {
		live = v
	}
	if v, ok := cmd.Args["prefer_shadow"].(bool); ok && v {
		live = false
	}

	values := make([]map[string]any, 0, len(pointIDs))
	reader, hasReader := e.sb.(PointReader)

	for _, pointID := range pointIDs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Preserve the original user-supplied identifier in the response
		// so the caller can correlate input with output.
		// 保留用户原始输入标识，便于调用方关联输入与输出。
		originalID := pointID
		if orig, ok := resolveMap[pointID]; ok && orig != "" {
			originalID = orig
		}

		if live && hasReader {
			val, err := reader.ReadPoint(channelID, deviceID, pointID)
			if err != nil {
				return nil, fmt.Errorf("read point %s: %w", pointID, err)
			}
			values = append(values, map[string]any{
				"point_id":  pointID,
				"address":   originalID,
				"value":     val.Value,
				"quality":   val.Quality,
				"timestamp": val.TS.UnixMilli(),
				"source":    "driver",
			})
			continue
		}

		sp, err := e.sb.GetShadowPoint(channelID, deviceID, pointID)
		if err != nil {
			return nil, fmt.Errorf("shadow read %s: %w", pointID, err)
		}
		ts := sp.CollectedAt
		if ts.IsZero() {
			ts = sp.Timestamp
		}
		values = append(values, map[string]any{
			"point_id":  pointID,
			"address":   originalID,
			"value":     sp.Value,
			"quality":   sp.Quality,
			"timestamp": ts.UnixMilli(),
			"source":    "shadow",
		})
	}

	return map[string]any{
		"channel_id": channelID,
		"device_id":  deviceID,
		"protocol":   cmd.Protocol,
		"values":     values,
		"timestamp":  time.Now().UnixMilli(),
	}, nil
}

// writePoint handles both single-point and batch writes.
// Batch mode is activated when args["writes"] (array of {address,value}) is present.
func (e *DriverExecutor) writePoint(ctx context.Context, cmd DriverCommand) (any, error) {
	channelID, deviceID, protocol, err := e.resolveDeviceWithProtocol(cmd.Args)
	if err != nil {
		return nil, err
	}
	if cmd.Protocol == "" {
		cmd.Protocol = protocol
	}

	// Batch write path | 批量写入路径
	if writes, ok := cmd.Args["writes"].([]any); ok && len(writes) > 0 {
		results := make([]map[string]any, 0, len(writes))
		successCount := 0
		for i, w := range writes {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			entry, ok := w.(map[string]any)
			if !ok {
				results = append(results, map[string]any{
					"index":   i,
					"success": false,
					"error":   "invalid write entry (expecting object with point_id/address+value)",
				})
				continue
			}
			// Prefer point_id, fall back to address.
			// 优先使用 point_id，回退到 address。
			rawID, _ := entry["point_id"].(string)
			if rawID == "" {
				rawID, _ = entry["address"].(string)
			}
			if rawID == "" {
				results = append(results, map[string]any{
					"index":   i,
					"success": false,
					"error":   "point_id or address is required in batch write entry",
				})
				continue
			}
			val, vok := entry["value"]
			if !vok {
				results = append(results, map[string]any{
					"index":   i,
					"success": false,
					"error":   "value is required in batch write entry",
				})
				continue
			}
			// Resolve each entry independently to handle duplicate point_ids correctly.
			// 独立解析每条写入，正确处理多个输入解析到同一 point_id 的情况。
			resolved, _ := e.resolvePointIDs(channelID, deviceID, []string{rawID})
			writeID := rawID
			if len(resolved) > 0 {
				writeID = resolved[0]
			}
			if werr := e.sb.WritePoint(channelID, deviceID, writeID, val); werr != nil {
				results = append(results, map[string]any{
					"index":    i,
					"address":  rawID,
					"point_id": writeID,
					"success":  false,
					"error":    werr.Error(),
				})
				continue
			}
			successCount++
			results = append(results, map[string]any{
				"index":    i,
				"address":  rawID,
				"point_id": writeID,
				"value":    val,
				"success":  true,
			})
		}
		return map[string]any{
			"success":    successCount == len(writes),
			"channel_id": channelID,
			"device_id":  deviceID,
			"protocol":   cmd.Protocol,
			"count":      len(writes),
			"success_count": successCount,
			"results":    results,
			"timestamp":  time.Now().UnixMilli(),
			"source":     "driver",
		}, nil
	}

	// Single write path | 单点写入路径
	rawPointID := firstString(cmd.Args, "point_id", "address")
	if rawPointID == "" {
		return nil, fmt.Errorf("address or point_id is required")
	}
	value, ok := cmd.Args["value"]
	if !ok {
		return nil, fmt.Errorf("value is required")
	}

	// Resolve address/name to point_id (same as read path).
	// 将地址/名称解析为 point_id（与读取路径一致）。
	resolvedIDs, resolveMap := e.resolvePointIDs(channelID, deviceID, []string{rawPointID})
	pointID := rawPointID
	if len(resolvedIDs) > 0 {
		pointID = resolvedIDs[0]
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := e.sb.WritePoint(channelID, deviceID, pointID, value); err != nil {
		return nil, err
	}
	originalID := rawPointID
	if orig, ok := resolveMap[pointID]; ok && orig != "" {
		originalID = orig
	}
	return map[string]any{
		"success":    true,
		"channel_id": channelID,
		"device_id":  deviceID,
		"protocol":   cmd.Protocol,
		"point_id":   pointID,
		"address":    originalID,
		"value":      value,
		"timestamp":  time.Now().UnixMilli(),
		"source":     "driver",
	}, nil
}

func (e *DriverExecutor) getDevicePoints(cmd DriverCommand) (any, error) {
	channelID, deviceID, protocol, err := e.resolveDeviceWithProtocol(cmd.Args)
	if err != nil {
		return nil, err
	}
	if cmd.Protocol == "" {
		cmd.Protocol = protocol
	}
	points, err := e.sb.GetDevicePoints(channelID, deviceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"channel_id": channelID,
		"device_id":  deviceID,
		"protocol":   cmd.Protocol,
		"points":     points,
		"count":      len(points),
	}, nil
}

func (e *DriverExecutor) scanDevices(ctx context.Context, cmd DriverCommand) (any, error) {
	scanner, ok := e.sb.(DeviceScanner)
	if !ok {
		return nil, fmt.Errorf("scan devices not supported by southbound backend")
	}
	channelID, _ := cmd.Args["channel_id"].(string)
	deviceID, _ := cmd.Args["device_id"].(string)
	if channelID == "" {
		return nil, fmt.Errorf("channel_id is required for ScanDevices")
	}
	// Auto-resolve protocol from channel for unified capabilities
	if cmd.Protocol == "" {
		for _, ch := range e.sb.GetChannels() {
			if ch.ID == channelID {
				cmd.Protocol = ch.Protocol
				break
			}
		}
	}
	params := map[string]any{}
	for k, v := range cmd.Args {
		if k == "channel_id" || k == "device_id" || k == "protocol" {
			continue
		}
		params[k] = v
	}
	if deviceID != "" {
		return scanner.ScanDevice(ctx, channelID, deviceID, params)
	}
	return scanner.ScanChannel(ctx, channelID, params)
}

func (e *DriverExecutor) diagnostics(cmd DriverCommand) (any, error) {
	deviceID, _ := cmd.Args["device_id"].(string)
	if provider, ok := e.sb.(DiagnosticsProvider); ok && deviceID != "" {
		return map[string]any{
			"device_id":    deviceID,
			"diagnostics": provider.GetDeviceDiagnostics(deviceID),
		}, nil
	}
	channels := e.sb.GetChannels()
	summaries := make([]map[string]any, 0, len(channels))
	for _, ch := range channels {
		summaries = append(summaries, map[string]any{
			"channel_id": ch.ID,
			"name":       ch.Name,
			"protocol":   ch.Protocol,
			"enable":     ch.Enable,
			"devices":    len(ch.Devices),
		})
	}
	return map[string]any{
		"diagnostics": map[string]any{
			"channels": summaries,
			"count":    len(summaries),
		},
		"timestamp": time.Now().UnixMilli(),
	}, nil
}

// resolveDevice finds the channel and device for the given args.
// Returns channelID, deviceID, and the channel's protocol.
func (e *DriverExecutor) resolveDevice(args map[string]any) (channelID, deviceID string, err error) {
	deviceID, _ = args["device_id"].(string)
	if deviceID == "" {
		return "", "", fmt.Errorf("device_id is required")
	}
	channelID, _ = args["channel_id"].(string)
	if channelID != "" {
		if e.sb.GetDevice(channelID, deviceID) == nil {
			return "", "", fmt.Errorf("device %s not found in channel %s", deviceID, channelID)
		}
		return channelID, deviceID, nil
	}
	for _, ch := range e.sb.GetChannels() {
		if e.sb.GetDevice(ch.ID, deviceID) != nil {
			return ch.ID, deviceID, nil
		}
	}
	return "", "", fmt.Errorf("device not found: %s", deviceID)
}

// resolveDeviceWithProtocol finds the channel, device, and protocol for the given args.
// Used by unified capabilities where protocol is not embedded in the Capability ID.
// Protocol resolution order: explicit args["protocol"] > channel.Protocol > empty.
func (e *DriverExecutor) resolveDeviceWithProtocol(args map[string]any) (channelID, deviceID, protocol string, err error) {
	channelID, deviceID, err = e.resolveDevice(args)
	if err != nil {
		return "", "", "", err
	}
	// Check if explicit protocol was passed in args
	if p, ok := args["protocol"].(string); ok && p != "" {
		return channelID, deviceID, p, nil
	}
	// Auto-resolve from channel
	for _, ch := range e.sb.GetChannels() {
		if ch.ID == channelID {
			return channelID, deviceID, ch.Protocol, nil
		}
	}
	return channelID, deviceID, "", nil
}

func collectPointIDs(args map[string]any) []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if v := firstString(args, "point_id", "address"); v != "" {
		add(v)
	}
	for _, key := range []string{"point_ids", "addresses"} {
		switch arr := args[key].(type) {
		case []string:
			for _, s := range arr {
				add(s)
			}
		case []any:
			for _, item := range arr {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		}
	}
	return out
}

func firstString(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// resolvePointIDs resolves raw user identifiers (point_id, address, or name)
// to internal point IDs by looking up the device's configured points.
//
// This bridges the semantic gap between list_points (which returns PDU offsets
// as "address", e.g. "0", "2") and read_points/write_points (which require
// the internal point ID, e.g. "pt_0723121000").
//
// Returns:
//   - resolvedIDs: the resolved point IDs (same order as input, deduplicated)
//   - resolveMap:  maps resolved point_id → original user input (for response correlation)
//
// resolvePointIDs 将用户输入（point_id、地址或名称）解析为内部 point_id。
// 解决 list_points 返回 PDU 偏移（如 "0"）而 read/write 需要 point_id 的语义不一致问题。
func (e *DriverExecutor) resolvePointIDs(channelID, deviceID string, rawIDs []string) (resolvedIDs []string, resolveMap map[string]string) {
	resolvedIDs = make([]string, 0, len(rawIDs))
	resolveMap = make(map[string]string) // point_id → original_input
	seen := make(map[string]bool)

	if len(rawIDs) == 0 {
		return
	}

	// Fetch device points to build lookup maps.
	// 获取设备点位列表，构建查找映射。
	points, err := e.sb.GetDevicePoints(channelID, deviceID)
	if err != nil || len(points) == 0 {
		// Cannot resolve — fall back to raw IDs (backward compatible).
		// 无法解析时回退到原始输入（向后兼容）。
		for _, id := range rawIDs {
			if !seen[id] {
				seen[id] = true
				resolvedIDs = append(resolvedIDs, id)
				resolveMap[id] = id
			}
		}
		return
	}

	// Build lookup: address → point_id, name → point_id, id → point_id
	// 构建查找映射：地址→point_id, 名称→point_id, id→point_id
	byAddr := make(map[string]string, len(points))
	byName := make(map[string]string, len(points))
	byID := make(map[string]bool, len(points))
	for _, p := range points {
		byID[p.ID] = true
		if p.Address != "" {
			byAddr[p.Address] = p.ID
		}
		if p.Name != "" {
			byName[strings.ToLower(p.Name)] = p.ID
		}
	}

	for _, raw := range rawIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		var pointID string
		switch {
		case byID[raw]:
			// Direct point ID match.
			pointID = raw
		case byAddr[raw] != "":
			// Address match (e.g., PDU offset "0", PLC address "40001").
			pointID = byAddr[raw]
		case byName[strings.ToLower(raw)] != "":
			// Name match (case-insensitive).
			pointID = byName[strings.ToLower(raw)]
		default:
			// No match found — use raw value (backward compatible, may fail at I/O layer).
			pointID = raw
		}

		if !seen[pointID] {
			seen[pointID] = true
			resolvedIDs = append(resolvedIDs, pointID)
			resolveMap[pointID] = raw
		}
	}
	return
}
