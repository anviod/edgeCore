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
	channelID, deviceID, err := e.resolveDevice(cmd.Args)
	if err != nil {
		return nil, err
	}
	pointIDs := collectPointIDs(cmd.Args)
	if len(pointIDs) == 0 {
		return nil, fmt.Errorf("address, addresses, point_id or point_ids is required")
	}

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

		if live && hasReader {
			val, err := reader.ReadPoint(channelID, deviceID, pointID)
			if err != nil {
				return nil, fmt.Errorf("read point %s: %w", pointID, err)
			}
			values = append(values, map[string]any{
				"point_id":  pointID,
				"address":   pointID,
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
			"address":   pointID,
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

func (e *DriverExecutor) writePoint(ctx context.Context, cmd DriverCommand) (any, error) {
	channelID, deviceID, err := e.resolveDevice(cmd.Args)
	if err != nil {
		return nil, err
	}
	pointID := firstString(cmd.Args, "point_id", "address")
	if pointID == "" {
		return nil, fmt.Errorf("address or point_id is required")
	}
	value, ok := cmd.Args["value"]
	if !ok {
		return nil, fmt.Errorf("value is required")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := e.sb.WritePoint(channelID, deviceID, pointID, value); err != nil {
		return nil, err
	}
	return map[string]any{
		"success":    true,
		"channel_id": channelID,
		"device_id":  deviceID,
		"point_id":   pointID,
		"address":    pointID,
		"value":      value,
		"timestamp":  time.Now().UnixMilli(),
		"source":     "driver",
	}, nil
}

func (e *DriverExecutor) getDevicePoints(cmd DriverCommand) (any, error) {
	channelID, deviceID, err := e.resolveDevice(cmd.Args)
	if err != nil {
		return nil, err
	}
	points, err := e.sb.GetDevicePoints(channelID, deviceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"channel_id": channelID,
		"device_id":  deviceID,
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
	params := map[string]any{}
	for k, v := range cmd.Args {
		if k == "channel_id" || k == "device_id" {
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
