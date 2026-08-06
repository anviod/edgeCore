// Package capability implements the edgeCore Capability Runtime for EAN 2.0.
// It defines the shared Agent / Capability / Discovery / Invoke / Event models
// and local runtime components (registry, dispatcher, discovery/event publishers).
package capability

import "encoding/json"

const (
	ProtocolVersion = "2.0"
	RuntimeVersion  = "2.0.0"
)

// AgentKind classifies an Agent in the EAN network.
type AgentKind string

const (
	AgentKindDevice   AgentKind = "device"
	AgentKindAI       AgentKind = "ai"
	AgentKindWorkflow AgentKind = "workflow"
	AgentKindService  AgentKind = "service"
	AgentKindCloud    AgentKind = "cloud"
)

// AgentStatus is the lifecycle status of an Agent.
type AgentStatus string

const (
	AgentStatusOnline   AgentStatus = "online"
	AgentStatusOffline  AgentStatus = "offline"
	AgentStatusDegraded AgentStatus = "degraded"
	AgentStatusError    AgentStatus = "error"
)

// TransportType is how the Agent is reachable.
type TransportType string

const (
	TransportMQTT TransportType = "mqtt"
	TransportNATS TransportType = "nats"
	TransportHTTP TransportType = "http"
	TransportSDK  TransportType = "sdk"
)

// CapabilityCategory groups capabilities for dispatch routing.
type CapabilityCategory string

const (
	CategoryDevice   CapabilityCategory = "device"
	CategoryAI       CapabilityCategory = "ai"
	CategoryWorkflow CapabilityCategory = "workflow"
	CategorySystem   CapabilityCategory = "system"
)

// Permission is the Capability access mode.
type Permission string

const (
	PermissionRead      Permission = "read"
	PermissionWrite     Permission = "write"
	PermissionReadWrite Permission = "readwrite"
	PermissionAdmin     Permission = "admin"
)

// InvokeStatus is the Invoke state machine status.
type InvokeStatus string

const (
	InvokeQueued    InvokeStatus = "queued"
	InvokeRunning   InvokeStatus = "running"
	InvokeCompleted InvokeStatus = "completed"
	InvokeFailed    InvokeStatus = "failed"
	InvokeTimeout   InvokeStatus = "timeout"
	InvokeRejected  InvokeStatus = "rejected"
)

// EventSeverity classifies event urgency.
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// MessageType values used in EAN 2.0 envelopes.
const (
	MessageTypeAgentDescriptor      = "agent_descriptor"
	MessageTypeAgentOffline         = "agent_offline"
	MessageTypeCapabilityDescriptor = "capability_descriptor"
	MessageTypeDiscoveryQuery       = "discovery_query"
	MessageTypeDiscoveryResponse    = "discovery_response"
	MessageTypeInvokeCapability     = "invoke_capability"
	MessageTypeInvokeResponse       = "invoke_response"
	MessageTypeInvokeStatusQuery    = "invoke_status_query"
	MessageTypeEvent                = "event"
	MessageTypeHeartbeat            = "agent_heartbeat"
)

// MessageHeader is the common EAN 2.0 envelope header.
type MessageHeader struct {
	MessageID     string `json:"message_id"`
	Timestamp     int64  `json:"timestamp"`
	Source        string `json:"source"`
	Destination   string `json:"destination,omitempty"`
	MessageType   string `json:"message_type"`
	Version       string `json:"version"`
	CorrelationID string `json:"correlation_id,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

// Message is the standard EAN 2.0 envelope.
type Message struct {
	Header MessageHeader `json:"header"`
	Body   any           `json:"body"`
}

// AgentEndpoint describes how peers reach the Agent.
type AgentEndpoint struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}

// Agent is the EAN 2.0 unified Agent model.
type Agent struct {
	ID                   string         `json:"id"`
	Kind                 AgentKind      `json:"kind"`
	Version              string         `json:"version"`
	Status               AgentStatus    `json:"status"`
	Transport            TransportType  `json:"transport"`
	HeartbeatIntervalSec int            `json:"heartbeat_interval_sec"`
	Endpoint             *AgentEndpoint `json:"endpoint,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	Capabilities         []Capability   `json:"capabilities,omitempty"`
}

// AgentDescriptorBody is published on discovery/agent.
type AgentDescriptorBody struct {
	Agent Agent `json:"agent"`
}

// AgentOfflineBody is published on discovery/agent/offline.
type AgentOfflineBody struct {
	AgentID string `json:"agent_id"`
	Reason  string `json:"reason,omitempty"`
}

// Capability is the EAN 2.0 unified capability model.
type Capability struct {
	ID           string             `json:"id"`
	AgentID      string             `json:"agent_id"`
	Description  string             `json:"description"`
	Category     CapabilityCategory `json:"category"`
	InputSchema  map[string]any     `json:"input_schema,omitempty"`
	OutputSchema map[string]any     `json:"output_schema,omitempty"`
	TimeoutSec   int                `json:"timeout_sec"`
	Permission   Permission         `json:"permission"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
}

// CapabilityDescriptorBody is published on discovery/capability.
type CapabilityDescriptorBody struct {
	Capabilities []Capability `json:"capabilities"`
}

// RegistrySnapshot is the local/remote registry view.
type RegistrySnapshot struct {
	AgentID           string   `json:"agent_id"`
	LastSeen          int64    `json:"last_seen"`
	CapabilitiesCount int      `json:"capabilities_count"`
	Capabilities      []string `json:"capabilities"`
	Status            string   `json:"status"`
	Version           string   `json:"version"`
}

// InvokeOptions controls Invoke execution.
type InvokeOptions struct {
	TimeoutSec int    `json:"timeout_sec,omitempty"`
	Priority   string `json:"priority,omitempty"`
	Retry      int    `json:"retry,omitempty"`
	Async      bool   `json:"async,omitempty"`
}

// InvokeRequest is the body of an invoke_capability message.
type InvokeRequest struct {
	InvokeID   string         `json:"invoke_id"`
	Target     string         `json:"target"`
	Capability string         `json:"capability"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Options    InvokeOptions  `json:"options,omitempty"`
}

// InvokeResult is the successful or failed execution payload.
type InvokeResult struct {
	Success   bool           `json:"success"`
	Values    any            `json:"values,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Error     string         `json:"error,omitempty"`
	ErrorCode string         `json:"error_code,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// InvokeResponse is the body of an invoke_response message.
type InvokeResponse struct {
	InvokeID  string       `json:"invoke_id"`
	Status    InvokeStatus `json:"status"`
	Result    InvokeResult `json:"result"`
	LatencyMs int64        `json:"latency_ms,omitempty"`
}

// InvokeStatusQuery requests async invoke status.
type InvokeStatusQuery struct {
	InvokeID string `json:"invoke_id"`
}

// InvokeMetrics tracks EAN Invoke runtime statistics for monitoring.
// Thread-safe; designed for concurrent Invoke calls from HTTP/MCP/bus adapters.
type InvokeMetrics struct {
	TotalInvokes  int64 `json:"total_invokes"`
	SuccessCount  int64 `json:"success_count"`
	FailedCount   int64 `json:"failed_count"`
	TimeoutCount  int64 `json:"timeout_count"`
	RejectedCount int64 `json:"rejected_count"`

	// 延迟统计（毫秒）| Latency statistics in milliseconds
	TotalLatencyMs int64 `json:"total_latency_ms"`
	MinLatencyMs   int64 `json:"min_latency_ms"`
	MaxLatencyMs   int64 `json:"max_latency_ms"`
}

// InvokeMetricsSnapshot is a point-in-time copy of InvokeMetrics for API exposure.
type InvokeMetricsSnapshot struct {
	TotalInvokes  int64          `json:"total_invokes"`
	SuccessCount  int64          `json:"success_count"`
	FailedCount   int64          `json:"failed_count"`
	TimeoutCount  int64          `json:"timeout_count"`
	RejectedCount int64          `json:"rejected_count"`
	SuccessRate   float64        `json:"success_rate"`
	AvgLatencyMs  float64        `json:"avg_latency_ms"`
	P50LatencyMs  int64          `json:"p50_latency_ms"`
	P99LatencyMs  int64          `json:"p99_latency_ms"`
	MinLatencyMs  int64          `json:"min_latency_ms"`
	MaxLatencyMs  int64          `json:"max_latency_ms"`
	TopErrors     []ErrorCounter `json:"top_errors,omitempty"`
}

// ErrorCounter tracks failure error code frequency.
type ErrorCounter struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

// Event is the EAN 2.0 event model.
type Event struct {
	EventID       string         `json:"event_id"`
	EventType     string         `json:"event_type"`
	AgentID       string         `json:"agent_id"`
	DeviceID      string         `json:"device_id,omitempty"`
	PointID       string         `json:"point_id,omitempty"`
	Value         any            `json:"value,omitempty"`
	PreviousValue any            `json:"previous_value,omitempty"`
	Timestamp     int64          `json:"timestamp"`
	Severity      EventSeverity  `json:"severity"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// DiscoveryQuery is sent on discovery/query.
type DiscoveryQuery struct {
	QueryType    string `json:"query_type"` // agent | capability | all
	AgentID      string `json:"agent_id,omitempty"`
	CapabilityID string `json:"capability_id,omitempty"`
	Category     string `json:"category,omitempty"`
	Keyword      string `json:"keyword,omitempty"`
}

// DiscoveryResponseBody is published on discovery/response.
type DiscoveryResponseBody struct {
	Agent        *Agent       `json:"agent,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// HeartbeatBody is published on heartbeat/{agent_id}.
type HeartbeatBody struct {
	AgentID   string      `json:"agent_id"`
	Status    AgentStatus `json:"status"`
	Timestamp int64       `json:"timestamp"`
	Sequence  int64       `json:"sequence"`
}

// Envelope helpers for typed decoding.

func DecodeBody[T any](body any) (T, error) {
	var zero T
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(raw, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}
