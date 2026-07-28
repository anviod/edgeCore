package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgex/internal/ai_agent"
	"github.com/anviod/edgex/internal/ai_agent/aitypes"
)

// AICommandExecutor runs AI.* DriverCommands (protocol reverse / doc parse).
type AICommandExecutor interface {
	Execute(ctx context.Context, cmd DriverCommand) (any, error)
}

// AIAdapter bridges EAN Capability Invoke → ai_agent.Agent task pipeline.
// Create is always real (quota + mock/remote pipeline); optional wait polls until
// waiting_confirm / failed / cancelled or ctx deadline.
type AIAdapter struct {
	agent *ai_agent.Agent
}

// NewAIAdapter wraps an existing Agent. agent must be non-nil.
func NewAIAdapter(agent *ai_agent.Agent) *AIAdapter {
	if agent == nil {
		agent = ai_agent.NewAgent("local")
	}
	return &AIAdapter{agent: agent}
}

var (
	sharedAIOnce sync.Once
	sharedAI     *AIAdapter
)

// SharedAIAdapter returns a process-wide default adapter (local mode Agent).
// Northbound MQTT/NATS runtimes use this when Server has not injected a custom Agent.
func SharedAIAdapter() *AIAdapter {
	sharedAIOnce.Do(func() {
		sharedAI = NewAIAdapter(ai_agent.NewAgent("local"))
	})
	return sharedAI
}

// Agent exposes the underlying Agent (for Server reuse / settings Apply).
func (a *AIAdapter) Agent() *ai_agent.Agent {
	if a == nil {
		return nil
	}
	return a.agent
}

// Execute implements AICommandExecutor.
func (a *AIAdapter) Execute(ctx context.Context, cmd DriverCommand) (any, error) {
	if a == nil || a.agent == nil {
		return nil, fmt.Errorf("AI adapter not wired")
	}
	skill, err := skillFromCommand(cmd.Command)
	if err != nil {
		return nil, err
	}
	req, wait, waitTimeout, err := buildCreateRequest(skill, cmd.Args)
	if err != nil {
		return nil, err
	}

	rec, err := a.agent.Create(req)
	if err != nil {
		return nil, err
	}

	if wait {
		rec, err = a.waitTask(ctx, rec.ID, waitTimeout)
		if err != nil {
			return nil, err
		}
	}

	return taskResult(rec, wait), nil
}

func skillFromCommand(command string) (aitypes.Skill, error) {
	switch command {
	case "AI.protocol_reverse":
		return aitypes.SkillProtocolReverse, nil
	case "AI.doc_parse":
		return aitypes.SkillDocParse, nil
	default:
		return "", fmt.Errorf("unsupported AI command: %s", command)
	}
}

func buildCreateRequest(skill aitypes.Skill, args map[string]any) (aitypes.CreateRequest, bool, time.Duration, error) {
	payload := extractPayload(args)
	if len(payload) == 0 && len(args) > 0 {
		// Allow flat arguments (without nested payload) for Invoke convenience.
		payload = args
	}
	if len(payload) == 0 {
		return aitypes.CreateRequest{}, false, 0, fmt.Errorf("payload is required for AI capability")
	}

	req := aitypes.CreateRequest{
		Skill:      skill,
		ProtocolID: firstStringAny(payload, "protocol_id", "protocol"),
		Filename:   firstStringAny(payload, "filename", "file", "document"),
		Scenario:   firstStringAny(payload, "scenario"),
		Description: firstStringAny(payload, "description", "prompt"),
	}
	if meta, ok := payload["meta"].(map[string]string); ok {
		req.Meta = meta
	} else if metaAny, ok := payload["meta"].(map[string]any); ok {
		req.Meta = stringMap(metaAny)
	}
	if obs, ok := payload["observations"].([]aitypes.Observation); ok {
		req.Observations = obs
	} else if raw, ok := payload["observations"]; ok {
		req.Observations = parseObservations(raw)
	}

	wait := boolFromAny(args["wait"]) || boolFromAny(payload["wait"])
	timeout := 30 * time.Second
	if v, ok := durationFromAny(args["wait_timeout_sec"]); ok {
		timeout = v
	} else if v, ok := durationFromAny(payload["wait_timeout_sec"]); ok {
		timeout = v
	}
	return req, wait, timeout, nil
}

func (a *AIAdapter) waitTask(ctx context.Context, taskID string, timeout time.Duration) (*aitypes.TaskRecord, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		rec, ok := a.agent.Get(taskID)
		if !ok {
			return nil, fmt.Errorf("AI task %s not found", taskID)
		}
		switch rec.Status {
		case aitypes.StatusWaitingConfirm, aitypes.StatusApplied, aitypes.StatusFailed, aitypes.StatusCancelled:
			return rec, nil
		}
		if time.Now().After(deadline) {
			return rec, fmt.Errorf("AI task %s timed out waiting for completion (status=%s)", taskID, rec.Status)
		}
		select {
		case <-ctx.Done():
			return rec, ctx.Err()
		case <-ticker.C:
		}
	}
}

func taskResult(rec *aitypes.TaskRecord, waited bool) map[string]any {
	out := map[string]any{
		"task_id":   rec.ID,
		"skill":     string(rec.Skill),
		"status":    string(rec.Status),
		"mode":      rec.Mode,
		"waited":    waited,
		"tokens":    rec.TokensUsed,
		"created_at": rec.CreatedAt.UnixMilli(),
		"updated_at": rec.UpdatedAt.UnixMilli(),
		"message":   "AI task accepted via Capability Runtime; Human-in-the-loop confirm still required before config apply",
	}
	if rec.ProtocolID != "" {
		out["protocol_id"] = rec.ProtocolID
	}
	if rec.ErrorMessage != "" {
		out["error"] = rec.ErrorMessage
	}
	if rec.Deliverables != nil {
		out["deliverables"] = rec.Deliverables
		// Convenience aliases matching Capability output schemas.
		if rec.Deliverables.PointDefinition != nil {
			out["points"] = rec.Deliverables.PointDefinition.Points
			out["candidates"] = rec.Deliverables.PointDefinition.Points
		} else if rec.Deliverables.ProtocolModel != nil {
			out["candidates"] = []any{rec.Deliverables.ProtocolModel}
		}
	}
	if rec.Validation != nil {
		out["validation"] = rec.Validation
	}
	return out
}

func extractPayload(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	raw, ok := args["payload"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err == nil {
			return m
		}
	}
	// Re-encode structured types (e.g. map[string]string) via JSON.
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

func firstStringAny(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stringMap(in map[string]any) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s, ok := v.(string); ok {
			out[k] = s
		} else {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

func parseObservations(raw any) []aitypes.Observation {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var obs []aitypes.Observation
	if err := json.Unmarshal(b, &obs); err != nil {
		return nil
	}
	return obs
}

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func durationFromAny(v any) (time.Duration, bool) {
	switch t := v.(type) {
	case float64:
		if t > 0 {
			return time.Duration(t) * time.Second, true
		}
	case int:
		if t > 0 {
			return time.Duration(t) * time.Second, true
		}
	case int64:
		if t > 0 {
			return time.Duration(t) * time.Second, true
		}
	case string:
		if d, err := time.ParseDuration(t); err == nil && d > 0 {
			return d, true
		}
	}
	return 0, false
}
