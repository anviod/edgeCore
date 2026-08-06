package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/core"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// EAN 2.0 Capability Runtime REST Handler
// 包装 capability.Runtime 的 Registry / Dispatcher / Discovery / Event 组件，
// 为 UI 提供 REST 端点，降级为 MCP JSON-RPC 时的备用通道。

const eanEventBufferCap = 200 // 事件环形缓冲区容量

// eanEventRing 是线程安全的事件环形缓冲区
type eanEventRing struct {
	mu     sync.RWMutex
	events []capability.Event
	idx    int
	cap    int
}

func newEanEventRing(capacity int) *eanEventRing {
	return &eanEventRing{
		events: make([]capability.Event, 0, capacity),
		cap:    capacity,
	}
}

// Push 追加一条事件，超出容量时淘汰最旧记录
func (r *eanEventRing) Push(evt capability.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) < r.cap {
		r.events = append(r.events, evt)
	} else {
		r.events[r.idx] = evt
		r.idx = (r.idx + 1) % r.cap
	}
}

// Recent 返回最近 n 条事件（按时间倒序）
func (r *eanEventRing) Recent(n int) []capability.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := len(r.events)
	if n <= 0 || n > total {
		n = total
	}
	out := make([]capability.Event, n)
	for i := 0; i < n; i++ {
		srcIdx := (r.idx - 1 - i + r.cap) % r.cap
		if total < r.cap {
			srcIdx = total - 1 - i
		}
		out[i] = r.events[srcIdx]
	}
	return out
}

// Clear 清空缓冲区
func (r *eanEventRing) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = r.events[:0]
	r.idx = 0
}

// ── REST Handlers ──

// handleEanAgentStatus GET /api/capability/agent/status
func (s *Server) handleEanAgentStatus(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data":    nil,
		})
	}
	agent := rt.Registry().GetAgent()
	snap := rt.Registry().Snapshot()
	metrics := rt.Metrics()

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"id":                     agent.ID,
			"kind":                   string(agent.Kind),
			"version":                agent.Version,
			"status":                 string(agent.Status),
			"transport":              string(agent.Transport),
			"heartbeat_interval_sec": agent.HeartbeatIntervalSec,
			"capabilities_count":     snap.CapabilitiesCount,
			"last_seen":              snap.LastSeen,
			"invoke_metrics":         metrics,
		},
	})
}

// handleEanCapabilityList GET /api/capability/list
func (s *Server) handleEanCapabilityList(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data":    fiber.Map{"capabilities": []any{}},
		})
	}

	category := c.Query("category")
	keyword := c.Query("keyword")

	var caps []capability.Capability
	if category != "" {
		caps = rt.Registry().ListByCategory(capability.CapabilityCategory(category))
	} else {
		caps = rt.Registry().List()
	}

	// 关键词过滤
	if keyword != "" {
		filtered := caps[:0]
		for _, cap := range caps {
			if containsFold(cap.ID, keyword) || containsFold(cap.Description, keyword) {
				filtered = append(filtered, cap)
			}
		}
		caps = filtered
	}

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"capabilities": caps,
			"total":        len(caps),
		},
	})
}

// handleEanCapabilityDetail GET /api/capability/list/:id
func (s *Server) handleEanCapabilityDetail(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.Status(404).JSON(fiber.Map{"code": "E009", "message": "capability runtime not initialized"})
	}
	id := c.Params("id")
	cap, ok := rt.Registry().Get(id)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"code": "E009", "message": "capability not found: " + id})
	}
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data":    cap,
	})
}

// handleEanInvoke POST /api/capability/invoke
func (s *Server) handleEanInvoke(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.Status(503).JSON(fiber.Map{"code": "E500", "message": "capability runtime not initialized"})
	}

	var req capability.InvokeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": "E400", "message": "invalid request body: " + err.Error()})
	}

	if req.Capability == "" {
		return c.Status(400).JSON(fiber.Map{"code": "E012", "message": "capability is required"})
	}

	// 同步调用（API 超时 ≤ 5s，capability 默认超时 10s 覆盖）
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	resp := rt.Invoke(ctx, req)

	// 捕获事件到环形缓冲区
	s.captureInvokeEvent(rt, req, resp)

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data":    resp,
	})
}

// handleEanInvokeStatus GET /api/capability/invoke/:id/status
func (s *Server) handleEanInvokeStatus(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.Status(503).JSON(fiber.Map{"code": "E500", "message": "capability runtime not initialized"})
	}
	invokeID := c.Params("id")
	resp, ok := rt.Dispatcher().GetStatus(invokeID)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"code": "E009", "message": "invoke not found: " + invokeID})
	}
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data":    resp,
	})
}

// handleEanDiscoveryAgents GET /api/capability/discovery/agents
func (s *Server) handleEanDiscoveryAgents(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data":    fiber.Map{"agents": []any{}},
		})
	}

	// 本地 Agent 信息（EdgeOS 未连接时仅返回自身）
	agent := rt.Registry().GetAgent()
	caps := rt.Registry().List()
	snap := rt.Registry().Snapshot()

	localAgent := fiber.Map{
		"id":                    agent.ID,
		"kind":                  string(agent.Kind),
		"version":               agent.Version,
		"status":                string(agent.Status),
		"transport":             string(agent.Transport),
		"capabilities_count":    snap.CapabilitiesCount,
		"last_seen_seconds_ago": maxInt(0, int((time.Now().UnixMilli()-snap.LastSeen)/1000)),
		"capabilities":          caps,
	}

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"agents": []fiber.Map{localAgent},
			"total":  1,
		},
	})
}

// handleEanEventHistory GET /api/capability/events/history
func (s *Server) handleEanEventHistory(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 50)
	if limit <= 0 || limit > eanEventBufferCap {
		limit = 50
	}

	ring := s.ensureEanEventRing()
	events := ring.Recent(limit)

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"events": events,
			"total":  len(events),
		},
	})
}

// handleEanEventClear DELETE /api/capability/events/history
func (s *Server) handleEanEventClear(c *fiber.Ctx) error {
	ring := s.ensureEanEventRing()
	ring.Clear()
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "events cleared",
	})
}

// handleEanSettings GET /api/capability/settings
func (s *Server) handleEanSettings(c *fiber.Ctx) error {
	rt := s.ensureCapabilityRuntime()

	// 查询北向 edgeOS 通道配置，EAN 复用北向通道传输层
	var nbChannel fiber.Map
	nbAvailable := false
	if s.nbm != nil {
		cfg := s.nbm.GetConfig()
		stats := s.nbm.GetNorthboundStats()
		statMap := make(map[string]core.NorthboundStatus)
		for _, st := range stats {
			statMap[st.ID] = st
		}

		// 优先查找 edgeOS(MQTT)，其次 edgeOS(NATS)
		for _, ch := range cfg.EdgeOSMQTT {
			st, ok := statMap[ch.ID]
			status := "stopped"
			if ok {
				status = st.Status
			}
			if !nbAvailable && ch.Enable {
				nbAvailable = true
				nbChannel = fiber.Map{
					"id":                     ch.ID,
					"name":                   ch.Name,
					"type":                   "edgeOS(MQTT)",
					"broker":                 ch.Broker,
					"node_id":                ch.NodeID,
					"enabled":                ch.Enable,
					"status":                 status,
					"ean_enabled":            ch.EANEnabled,
					"ean_heartbeat_sec":      ch.EANHeartbeatSec,
					"ean_event_auto_publish": ch.EANEventAutoPublish,
					"v1_command_enabled":     ch.V1CommandEnabled,
				}
			}
		}
		for _, ch := range cfg.EdgeOSNATS {
			if nbAvailable {
				break
			}
			st, ok := statMap[ch.ID]
			status := "stopped"
			if ok {
				status = st.Status
			}
			if ch.Enable {
				nbAvailable = true
				nbChannel = fiber.Map{
					"id":                     ch.ID,
					"name":                   ch.Name,
					"type":                   "edgeOS(NATS)",
					"url":                    ch.URL,
					"node_id":                ch.NodeID,
					"enabled":                ch.Enable,
					"status":                 status,
					"ean_enabled":            ch.EANEnabled,
					"ean_heartbeat_sec":      ch.EANHeartbeatSec,
					"ean_event_auto_publish": ch.EANEventAutoPublish,
					"v1_command_enabled":     ch.V1CommandEnabled,
				}
			}
		}
	}

	if rt == nil {
		return c.JSON(fiber.Map{
			"code":    "0",
			"message": "success",
			"data": fiber.Map{
				"enabled":              false,
				"transport":            "mqtt",
				"heartbeat_sec":        60,
				"event_auto_publish":   true,
				"northbound_available": nbAvailable,
				"northbound_channel":   nbChannel,
			},
		})
	}

	agent := rt.Registry().GetAgent()
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": fiber.Map{
			"enabled":              true,
			"agent_id":             agent.ID,
			"transport":            string(agent.Transport),
			"heartbeat_sec":        agent.HeartbeatIntervalSec,
			"event_auto_publish":   true,
			"capabilities_count":   rt.Registry().Count(),
			"northbound_available": nbAvailable,
			"northbound_channel":   nbChannel,
		},
	})
}

// ── 内部辅助 ──

// eanEventRingOnce / eanEventRingInstance 确保环形缓冲区单例
var (
	eanEventRingOnce     sync.Once
	eanEventRingInstance *eanEventRing
)

func (s *Server) ensureEanEventRing() *eanEventRing {
	eanEventRingOnce.Do(func() {
		eanEventRingInstance = newEanEventRing(eanEventBufferCap)
	})
	return eanEventRingInstance
}

// captureInvokeEvent 将 Invoke 结果转化为 EAN Event 并存入环形缓冲区
func (s *Server) captureInvokeEvent(rt *capability.Runtime, req capability.InvokeRequest, resp capability.InvokeResponse) {
	ring := s.ensureEanEventRing()
	severity := capability.SeverityInfo
	if resp.Status == capability.InvokeFailed || resp.Status == capability.InvokeTimeout {
		severity = capability.SeverityError
	} else if resp.Status == capability.InvokeRejected {
		severity = capability.SeverityWarning
	}

	evt := capability.Event{
		EventID:   "evt-" + resp.InvokeID,
		EventType: "capability.invoked",
		AgentID:   rt.Registry().AgentID(),
		Timestamp: time.Now().UnixMilli(),
		Severity:  severity,
		Metadata: map[string]any{
			"capability": req.Capability,
			"invoke_id":  resp.InvokeID,
			"status":     string(resp.Status),
			"latency_ms": resp.LatencyMs,
		},
	}
	ring.Push(evt)
}

// captureShadowEvent 将 Shadow 变化事件存入环形缓冲区（供 ShadowEventBridge 回调）
func (s *Server) captureShadowEvent(deviceID, pointID string, value, previous any) {
	ring := s.ensureEanEventRing()
	rt := s.ensureCapabilityRuntime()
	agentID := "edgeCore"
	if rt != nil {
		agentID = rt.Registry().AgentID()
	}
	evt := capability.Event{
		EventID:       "evt-shadow-" + pointID + "-" + time.Now().Format("150405.000"),
		EventType:     pointID + ".changed",
		AgentID:       agentID,
		DeviceID:      deviceID,
		PointID:       pointID,
		Value:         value,
		PreviousValue: previous,
		Timestamp:     time.Now().UnixMilli(),
		Severity:      capability.SeverityInfo,
	}
	ring.Push(evt)
}

// containsFold 大小写不敏感的子串匹配
func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// handleEanInvokeFromMCP 允许 MCP handler 调用 EAN invoke（内部复用）
func (s *Server) handleEanInvokeFromMCP(ctx context.Context, req capability.InvokeRequest) capability.InvokeResponse {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return capability.InvokeResponse{
			InvokeID: req.InvokeID,
			Status:   capability.InvokeRejected,
			Result: capability.InvokeResult{
				Success:   false,
				Error:     "capability runtime not initialized",
				ErrorCode: "E500",
				Timestamp: time.Now().UnixMilli(),
			},
		}
	}
	resp := rt.Invoke(ctx, req)
	s.captureInvokeEvent(rt, req, resp)
	s.logger.Debug("EAN invoke via MCP",
		zap.String("capability", req.Capability),
		zap.String("status", string(resp.Status)),
		zap.Int64("latency_ms", resp.LatencyMs),
	)
	return resp
}

// handleEanCapabilitiesJSON 返回 Capability 列表 JSON（供 MCP handler 复用）
func (s *Server) handleEanCapabilitiesJSON() []capability.Capability {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return nil
	}
	return rt.Registry().List()
}

// handleEanAgentDescriptorJSON 返回 Agent 描述符 JSON（供 MCP handler 复用）
func (s *Server) handleEanAgentDescriptorJSON() *capability.Agent {
	rt := s.ensureCapabilityRuntime()
	if rt == nil {
		return nil
	}
	agent := rt.Registry().GetAgent()
	return &agent
}
