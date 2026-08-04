package edgos_mqtt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/northbound/reconnect"
	"github.com/anviod/edgex/internal/storage"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"
)

const (
	StatusDisconnected = 0
	StatusConnected    = 1
	StatusReconnecting = 2
	StatusError        = 3
)

// MessageHeader represents the standard edgeOS message header
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

// Message represents a standard edgeOS message
type Message struct {
	Header MessageHeader `json:"header"`
	Body   any           `json:"body"`
}

// HeartbeatMessage represents the enriched heartbeat message body
type HeartbeatMessage struct {
	NodeID          string                 `json:"node_id"`
	Status          string                 `json:"status"`
	Timestamp       int64                  `json:"timestamp"`
	Sequence        int64                  `json:"sequence"`
	UptimeSeconds   int64                  `json:"uptime_seconds"`
	Version         string                 `json:"version"`
	SystemMetrics   SystemMetrics          `json:"system_metrics"`
	DeviceSummary   DeviceSummary          `json:"device_summary"`
	ChannelSummary  ChannelSummary         `json:"channel_summary"`
	TaskSummary     TaskSummary            `json:"task_summary"`
	ConnectionStats ConnectionStats        `json:"connection_stats"`
	CustomMetrics   map[string]interface{} `json:"custom_metrics,omitempty"`
}

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    float64 `json:"memory_usage"`
	MemoryTotal    int64   `json:"memory_total"`
	MemoryUsed     int64   `json:"memory_used"`
	DiskUsage      float64 `json:"disk_usage"`
	DiskTotal      int64   `json:"disk_total"`
	DiskUsed       int64   `json:"disk_used"`
	LoadAverage    float64 `json:"load_average"`
	NetworkRXBytes int64   `json:"network_rx_bytes"`
	NetworkTXBytes int64   `json:"network_tx_bytes"`
	ProcessCount   int     `json:"process_count"`
	ThreadCount    int     `json:"thread_count"`
}

// DeviceSummary represents device statistics summary
type DeviceSummary struct {
	TotalCount      int `json:"total_count"`
	OnlineCount     int `json:"online_count"`
	OfflineCount    int `json:"offline_count"`
	ErrorCount      int `json:"error_count"`
	DegradedCount   int `json:"degraded_count"`
	RecoveringCount int `json:"recovering_count"`
}

// ChannelSummary represents channel statistics summary
type ChannelSummary struct {
	TotalCount     int     `json:"total_count"`
	ConnectedCount int     `json:"connected_count"`
	ErrorCount     int     `json:"error_count"`
	AvgSuccessRate float64 `json:"avg_success_rate"`
}

// TaskSummary represents task statistics summary
type TaskSummary struct {
	TotalCount   int `json:"total_count"`
	RunningCount int `json:"running_count"`
	PausedCount  int `json:"paused_count"`
	ErrorCount   int `json:"error_count"`
}

// ConnectionStats represents MQTT connection statistics
type ConnectionStats struct {
	ReconnectCount  int64  `json:"reconnect_count"`
	LastOnlineTime  int64  `json:"last_online_time"`
	LastOfflineTime int64  `json:"last_offline_time"`
	ConnectedSince  int64  `json:"connected_since"`
	PublishCount    int64  `json:"publish_count"`
	ProtocolVersion string `json:"protocol_version"`
}

// Client implements edgeOS(MQTT) northbound channel
type Client struct {
	config   model.EdgeOSMQTTConfig
	configMu sync.RWMutex
	client   mqtt.Client
	nodeID   string

	status   int
	statusMu sync.RWMutex
	stopChan chan struct{}

	sb      model.SouthboundManager
	storage *storage.Storage
	logger  *zap.Logger

	// Stats
	successCount    int64
	failCount       int64
	reconnectCount  int64
	lastOfflineTime int64
	lastOnlineTime  int64
	publishCount    int64

	// Command handlers
	cmdSubscriptions map[string]mqtt.Token
	cmdMu            sync.Mutex

	// Device aggregation for periodic push
	deviceAggregators map[string]*deviceAggregator
	aggregatorMu      sync.RWMutex
	periodicTicker    *time.Ticker

	// Heartbeat ticker
	heartbeatTicker   *time.Ticker
	heartbeatInterval time.Duration

	reconnectSched reconnect.Scheduler

	// EAN 2.0 Capability Runtime (optional; V1.0 edgex/* topics remain unchanged)
	eanMu               sync.RWMutex
	eanRuntime          *capability.Runtime
	onEANRuntimeChanged func()

	// deviceReportGen bumps on each connect; fallback timer ignores stale gens.
	// deviceReportOK is set after a successful publishDeviceReport in this generation.
	deviceReportGen atomic.Uint64
	deviceReportOK  atomic.Bool
}

// NewClient creates a new edgeOS(MQTT) client
func NewClient(cfg model.EdgeOSMQTTConfig, sb model.SouthboundManager, s *storage.Storage) *Client {
	logger := zap.L().With(
		zap.String("component", "edgos-mqtt-client"),
		zap.String("client_id", cfg.ID),
		zap.String("name", cfg.Name),
	)
	return &Client{
		config:            cfg,
		sb:                sb,
		storage:           s,
		nodeID:            cfg.NodeID,
		logger:            logger,
		stopChan:          make(chan struct{}),
		cmdSubscriptions:  make(map[string]mqtt.Token),
		deviceAggregators: make(map[string]*deviceAggregator),
	}
}

// GetStatus returns the current connection status
func (c *Client) GetStatus() int {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

// GetStats returns client statistics (including EAN Runtime metrics if enabled)
func (c *Client) GetStats() EdgeOSMQTTStats {
	stats := EdgeOSMQTTStats{
		SuccessCount:    atomic.LoadInt64(&c.successCount),
		FailCount:       atomic.LoadInt64(&c.failCount),
		ReconnectCount:  atomic.LoadInt64(&c.reconnectCount),
		PublishCount:    atomic.LoadInt64(&c.publishCount),
		LastOfflineTime: atomic.LoadInt64(&c.lastOfflineTime),
		LastOnlineTime:  atomic.LoadInt64(&c.lastOnlineTime),
	}
	// 附加 EAN Runtime 指标（如果已启用）
	if rt := c.CapabilityRuntime(); rt != nil {
		snap := rt.Metrics()
		stats.EANMetrics = &snap
	}
	return stats
}

// EdgeOSMQTTStats represents client statistics
type EdgeOSMQTTStats struct {
	SuccessCount    int64                          `json:"success_count"`
	FailCount       int64                          `json:"fail_count"`
	ReconnectCount  int64                          `json:"reconnect_count"`
	PublishCount    int64                          `json:"publish_count"`
	LastOfflineTime int64                          `json:"last_offline_time"`
	LastOnlineTime  int64                          `json:"last_online_time"`
	EANMetrics      *capability.InvokeMetricsSnapshot `json:"ean_metrics,omitempty"`
}

// deviceAggregator aggregates points for periodic device-level push
type deviceAggregator struct {
	points       map[string]model.Value // pointID -> Value
	lastPushTS   time.Time
	pushInterval time.Duration
	mu           sync.RWMutex
}

// UpdateConfig updates the client configuration.
// 连接级字段（Broker/ClientID/Username/Password/NodeID）变化触发全量重连；
// EAN 字段（EANEnabled/EANHeartbeatSec）变化走热更新路径，不中断北向连接。
// | Connection-level fields trigger full restart;
// | EAN fields are hot-applied without dropping the northbound connection.
func (c *Client) UpdateConfig(cfg model.EdgeOSMQTTConfig) error {
	// 在锁内捕获旧 EAN 状态和重连决策，锁外执行重连/热更新以避免死锁
	// | Capture old EAN state and restart decision under lock;
	// | execute restart/hot-update outside lock to avoid deadlock.
	c.configMu.Lock()
	oldEANEnabled := c.config.EANEnabled
	oldHeartbeat := c.config.EANHeartbeatSec
	oldAutoPublish := c.config.EANEventAutoPublish
	oldV1Cmd := c.config.V1CommandEnabled
	oldDevices := c.config.Devices

	needRestart := c.config.Broker != cfg.Broker ||
		c.config.ClientID != cfg.ClientID ||
		c.config.Username != cfg.Username ||
		c.config.Password != cfg.Password ||
		c.config.NodeID != cfg.NodeID ||
		// Phase 4 (EX-P4): V1 命令面开关变化 → 全量重连以重新订阅/下线 V1 命令 Topic
		// | V1 command plane switch change → full reconnect to (re)subscribe V1 command topics
		oldV1Cmd != cfg.V1CommandEnabled

	c.config = cfg
	c.nodeID = cfg.NodeID
	c.configMu.Unlock()

	if needRestart {
		// 全量重连 — OnConnect 回调会根据新配置条件性启动 EAN Runtime
		// | Full restart — OnConnect will conditionally start EAN Runtime based on new config
		c.Stop()
		c.stopChan = make(chan struct{})
		return c.Start()
	}

	// EAN 热更新：无需重连，仅检测 EAN 字段变化
	// | EAN hot update: apply EAN field changes without reconnection
	c.applyEANConfigChange(oldEANEnabled, cfg.EANEnabled, oldHeartbeat, cfg.EANHeartbeatSec, oldAutoPublish, cfg.EANEventAutoPublish)

	// 设备映射变化：重新上报设备清单（edgex/devices/report），确保 EdgeOS 设备列表同步。
	// | Devices mapping changed: re-publish device inventory so EdgeOS device list stays in sync.
	if !reflect.DeepEqual(oldDevices, cfg.Devices) && c.GetStatus() == StatusConnected {
		zap.L().Info("Northbound Devices mapping changed; re-publishing device report",
			zap.String("node_id", cfg.NodeID))
		c.publishDeviceReport()
	}
	return nil
}

// applyEANConfigChange 处理 EAN 配置字段变化，无需重连北向通道。
// | Handles EAN config field changes without reconnecting the northbound channel.
// false→true: 启动 Runtime；true→false: 停止 Runtime；心跳变化: 重启 Runtime；
// EANEventAutoPublish 变化: 刷新 Shadow→Event publisher 列表。
func (c *Client) applyEANConfigChange(oldEnabled, newEnabled bool, oldHeartbeat, newHeartbeat int, oldAutoPublish, newAutoPublish bool) {
	if !oldEnabled && newEnabled {
		// false → true：启动 EAN Runtime
		if c.GetStatus() == StatusConnected {
			rt, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion)
			if err != nil {
				zap.L().Warn("EAN hot-start failed", zap.Error(err))
			} else if rt != nil {
				c.StartEAN(context.Background())
				zap.L().Info("EAN Runtime hot-started (EANEnabled false→true)")
			}
		}
		return
	}

	if oldEnabled && !newEnabled {
		// true → false：停止 EAN Runtime
		c.StopCapabilityRuntime()
		return
	}

	// 心跳间隔变化且 EAN 仍启用：重启 Runtime 以应用新参数
	// | Heartbeat interval changed while EAN enabled: restart Runtime to apply new parameter
	if newEnabled && oldHeartbeat != newHeartbeat {
		c.StopCapabilityRuntime()
		if c.GetStatus() == StatusConnected {
			rt, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion)
			if err != nil {
				zap.L().Warn("EAN hot-restart failed (heartbeat change)", zap.Error(err))
			} else if rt != nil {
				c.StartEAN(context.Background())
				zap.L().Info("EAN Runtime hot-restarted (heartbeat changed)",
					zap.Int("old_sec", oldHeartbeat),
					zap.Int("new_sec", newHeartbeat))
			}
		}
	}

	// EANEventAutoPublish 变化：刷新 Shadow→Event publisher（EAN 仍启用时）
	// | EANEventAutoPublish changed while EAN enabled: refresh Shadow→Event publishers
	if newEnabled && oldAutoPublish != newAutoPublish {
		c.notifyEANRuntimeChanged()
		zap.L().Info("EAN event auto-publish toggled; Shadow→Event publishers refreshed",
			zap.Bool("old", oldAutoPublish),
			zap.Bool("new", newAutoPublish))
	}
}

// Start starts the edgeOS(MQTT) client
func (c *Client) Start() error {
	go c.connectLoop()
	go c.retryLoop()
	go c.periodicPushLoop()
	go c.heartbeatLoop()
	return nil
}

// connectLoop manages the MQTT connection
func (c *Client) connectLoop() {
	c.setStatus(StatusReconnecting)
	c.logger.Info("Starting edgeOS(MQTT) client connection")

	c.configMu.RLock()
	broker := c.config.Broker
	clientID := c.config.ClientID
	username := c.config.Username
	password := c.config.Password
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(false)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectTimeout(30 * time.Second)

	// Set LWT (Last Will Testament) for node status
	lwtTopic := fmt.Sprintf("edgex/nodes/%s/offline", nodeID)
	opts.SetWill(lwtTopic, "", 1, true)

	// Set connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		zap.L().Info("edgeOS(MQTT) Connected",
			zap.String("broker", broker),
			zap.String("node_id", nodeID),
			zap.String("component", "edgos-mqtt-client"),
		)
		c.setStatus(StatusConnected)
		atomic.StoreInt64(&c.lastOnlineTime, time.Now().UnixMilli())

		// Subscribe before publishing registration so we do not miss
		// register_response (which also triggers publishDeviceReport).
		c.subscribeToCommands()

		// Publish node online status
		c.publishNodeOnline()

		// Inventory on connect (same as re-register): EdgeOS may skip
		// register_response when the node is already known.
		c.deviceReportOK.Store(false)
		c.publishDeviceReport()
		c.scheduleDeviceReportFallback()

		// Publish points metadata
		if err := c.PublishPointsMetadata(); err != nil {
			zap.L().Warn("Failed to publish points metadata on connect",
				zap.Error(err),
				zap.String("node_id", nodeID),
			)
		}

		// EAN 2.0 Capability Runtime: auto-ensure + publish $edgeos/* descriptors.
		// V1.0 edgex/* compatibility paths above are unchanged.
		// EnsureCapabilityRuntime returns (nil, nil) when EANEnabled=false — skip start in that case.
		rt, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion)
		if err != nil {
			zap.L().Warn("Failed to ensure EAN Capability Runtime",
				zap.Error(err),
				zap.String("node_id", nodeID),
			)
		} else if rt != nil {
			c.startEANLocked(context.Background())
		} else {
			zap.L().Info("EAN capability layer disabled, skipping Runtime start",
				zap.String("node_id", nodeID))
		}
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		zap.L().Warn("edgeOS(MQTT) Connection Lost",
			zap.Error(err),
			zap.String("node_id", nodeID),
			zap.String("component", "edgos-mqtt-client"),
		)
		c.setStatus(StatusDisconnected)
		atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
		c.scheduleReconnect()
	})

	c.client = mqtt.NewClient(opts)

	// Initial connection attempt
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		zap.L().Error("Initial edgeOS(MQTT) connection failed",
			zap.Error(token.Error()),
			zap.String("node_id", nodeID),
			zap.String("component", "edgos-mqtt-client"),
		)
		c.setStatus(StatusDisconnected)
		atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
		c.scheduleReconnect()
	} else {
		atomic.StoreInt64(&c.lastOnlineTime, time.Now().UnixMilli())
	}
}

// publishNodeOnline publishes node registration and online status
func (c *Client) publishNodeOnline() {
	c.configMu.RLock()
	nodeID := c.config.NodeID
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-02/03): V1 命令面开关——false 时不再发布 V1 节点注册/状态（EAN Discovery 已覆盖）。
	// | V1 command plane switch: when false, skip V1 node register/status publish (EAN Discovery covers it).
	if !v1CmdEnabled {
		zap.L().Info("V1 command plane disabled; skipping V1 node registration publish",
			zap.String("node_id", nodeID))
		return
	}

	// Phase 4 (EX-P4-02): V1 节点注册/心跳上报标记 deprecated——EAN Discovery($edgeos/discovery/agent)
	// + Heartbeat($edgeos/heartbeat/{agent}) 已完全替代；此处仅保留 V1 兼容，不再维护。
	// | V1 node register/heartbeat marked DEPRECATED; superseded by EAN Discovery + Heartbeat.
	zap.L().Warn("DEPRECATED: publishing V1 node registration (edgex/nodes/*); use EAN $edgeos/discovery/agent",
		zap.String("node_id", nodeID))

	// Publish node registration
	regMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "node_register",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":      nodeID,
			"node_name":    "EdgeX Gateway Node",
			"model":        "edgex",
			"version":      "1.0.0",
			"api_version":  "v1",
			"capabilities": []string{"shadow-sync", "heartbeat", "device-control", "task-execution"},
			"protocol":     "edgeOS(MQTT)",
			"endpoint": map[string]string{
				"host": "127.0.0.1",
				"port": "8082",
			},
		},
	}

	if err := c.publishMessage("edgex/nodes/register", regMessage, 1); err != nil {
		zap.L().Error("Failed to publish node registration",
			zap.Error(err),
			zap.String("node_id", nodeID),
		)
	}

	// Publish online status
	topic := fmt.Sprintf("edgex/nodes/%s/online", nodeID)
	payload := `{"status":"online","timestamp":` + fmt.Sprintf("%d", time.Now().UnixMilli()) + `}`
	token := c.client.Publish(topic, 1, true, payload)
	token.Wait()
}

// subscribeToCommands subscribes to edgeOS command topics
// Phase 4 (EX-P4-01/03): V1 command plane is DEPRECATED — subscriptions retained only
// for backward compatibility; all command traffic must migrate to EAN Capability Invoke.
// After the transition window passes, these subscriptions are removed (V1 command plane off).
func (c *Client) subscribeToCommands() {
	c.configMu.RLock()
	nodeID := c.config.NodeID
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-03): V1 命令面开关——false 时全面下线 V1 命令 Topic 订阅。
	// | V1 command plane switch: when false, decommission V1 command topic subscriptions.
	if !v1CmdEnabled {
		zap.L().Info("V1 command plane disabled (V1CommandEnabled=false); skipping V1 command topic subscriptions",
			zap.String("node_id", nodeID))
		return
	}

	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	// V1 命令面（edgex/cmd/*）Phase 4 标记 deprecated，仅保留兼容订阅，不再维护。
	// | V1 command plane (edgex/cmd/*) marked DEPRECATED in Phase 4; retained only for compat.
	zap.L().Warn("DEPRECATED: subscribing V1 command topics (edgex/cmd/*); migrate to EAN Capability Invoke",
		zap.String("node_id", nodeID))

	// Subscribe to device discovery command
	discoverTopic := fmt.Sprintf("edgex/cmd/%s/discover", nodeID)
	if token := c.client.Subscribe(discoverTopic, 0, c.handleDiscoverCommand); token.Wait() && token.Error() != nil {
		zap.L().Error("Failed to subscribe to discover topic",
			zap.Error(token.Error()),
			zap.String("topic", discoverTopic),
		)
	} else {
		c.cmdSubscriptions[discoverTopic] = token
		zap.L().Warn("DEPRECATED: subscribed to V1 discover topic (retained for compat)",
			zap.String("topic", discoverTopic))
	}

	// Subscribe to write commands for all devices
	writeTopic := fmt.Sprintf("edgex/cmd/%s/+/write", nodeID)
	if token := c.client.Subscribe(writeTopic, 1, c.handleWriteCommand); token.Wait() && token.Error() != nil {
		zap.L().Error("Failed to subscribe to write topic",
			zap.Error(token.Error()),
			zap.String("topic", writeTopic),
		)
	} else {
		c.cmdSubscriptions[writeTopic] = token
		zap.L().Warn("DEPRECATED: subscribed to V1 write topic (retained for compat)",
			zap.String("topic", writeTopic))
	}

	// Subscribe to task control commands
	taskTopic := fmt.Sprintf("edgex/cmd/%s/task/+/+", nodeID)
	if token := c.client.Subscribe(taskTopic, 1, c.handleTaskCommand); token.Wait() && token.Error() != nil {
		zap.L().Error("Failed to subscribe to task topic",
			zap.Error(token.Error()),
			zap.String("topic", taskTopic),
		)
	} else {
		c.cmdSubscriptions[taskTopic] = token
		zap.L().Warn("DEPRECATED: subscribed to V1 task topic (retained for compat)",
			zap.String("topic", taskTopic))
	}

	// Subscribe to global node register command (triggered by EdgeOS for proactive re-registration)
	registerTopic := "edgex/cmd/nodes/register"
	if token := c.client.Subscribe(registerTopic, 1, c.handleNodeRegisterCommand); token.Wait() && token.Error() != nil {
		zap.L().Error("Failed to subscribe to register topic",
			zap.Error(token.Error()),
			zap.String("topic", registerTopic),
		)
	} else {
		c.cmdSubscriptions[registerTopic] = token
		zap.L().Warn("DEPRECATED: subscribed to V1 node register topic (retained for compat)",
			zap.String("topic", registerTopic))
	}

	// Subscribe to node registration response (EdgeOS responds to our registration request)
	responseTopic := fmt.Sprintf("edgex/nodes/%s/response", nodeID)
	if token := c.client.Subscribe(responseTopic, 0, c.handleRegisterResponseCommand); token.Wait() && token.Error() != nil {
		zap.L().Error("Failed to subscribe to response topic",
			zap.Error(token.Error()),
			zap.String("topic", responseTopic),
		)
	} else {
		c.cmdSubscriptions[responseTopic] = token
		zap.L().Warn("DEPRECATED: subscribed to V1 node register-response topic (retained for compat)",
			zap.String("topic", responseTopic))
	}
}

// handleDiscoverCommand handles device discovery commands
func (c *Client) handleDiscoverCommand(client mqtt.Client, msg mqtt.Message) {
	zap.L().Warn("DEPRECATED: V1 discover command received, migrate to EAN *.scan_devices Capability Invoke",
		zap.String("topic", msg.Topic()))
	var message Message
	if err := json.Unmarshal(msg.Payload(), &message); err != nil {
		zap.L().Error("Failed to unmarshal discover command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received discover command",
		zap.String("message_id", message.Header.MessageID),
	)

	// Send response
	c.sendCommandResponse(message.Header, "discover_response", true, "Discovery triggered", nil, "")
}

// handleWriteCommand handles write commands for devices
func (c *Client) handleWriteCommand(client mqtt.Client, msg mqtt.Message) {
	zap.L().Warn("DEPRECATED: V1 write command received, migrate to EAN *.write_point Capability Invoke",
		zap.String("topic", msg.Topic()))
	var message Message
	if err := json.Unmarshal(msg.Payload(), &message); err != nil {
		zap.L().Error("Failed to unmarshal write command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received write command",
		zap.String("message_id", message.Header.MessageID),
	)

	// Extract device ID from topic
	topicParts := strings.Split(msg.Topic(), "/")
	if len(topicParts) < 5 {
		zap.L().Error("Invalid write topic", zap.String("topic", msg.Topic()))
		c.sendCommandResponse(message.Header, "write_response", false, "Invalid topic", map[string]interface{}{"request_id": message.Header.MessageID}, "")
		return
	}
	deviceID := topicParts[3]

	c.configMu.RLock()
	virtualDevices := c.config.VirtualDevices
	c.configMu.RUnlock()
	if model.IsNorthboundVirtualDevice(deviceID, virtualDevices) {
		zap.L().Warn("Write rejected for virtual device", zap.String("device", deviceID))
		c.sendCommandResponse(message.Header, "write_response", false, "Virtual device is read-only", map[string]interface{}{"request_id": message.Header.MessageID}, deviceID)
		return
	}

	// Parse write command body
	body, ok := message.Body.(map[string]interface{})
	if !ok {
		zap.L().Error("Invalid write command body")
		c.sendCommandResponse(message.Header, "write_response", false, "Invalid body", map[string]interface{}{"request_id": message.Header.MessageID}, deviceID)
		return
	}

	// Extract request_id if available
	requestID := message.Header.MessageID
	// Check header for request_id first
	if message.Header.RequestID != "" {
		requestID = message.Header.RequestID
	} else if rid, exists := body["request_id"]; exists {
		// Then check body for request_id
		if ridStr, ok := rid.(string); ok {
			requestID = ridStr
		}
	}

	// Handle different message formats
	var points map[string]interface{}
	if p, ok := body["points"].(map[string]interface{}); ok {
		// Standard format with points map
		points = p
	} else if pointID, ok := body["point_id"].(string); ok {
		// New format with direct point_id and value
		value := body["value"]
		points = map[string]interface{}{
			pointID: value,
		}
	} else {
		zap.L().Error("Invalid points in write command")
		c.sendCommandResponse(message.Header, "write_response", false, "Invalid points", map[string]interface{}{"request_id": requestID}, deviceID)
		return
	}

	// Execute writes through southbound manager
	if c.sb == nil {
		zap.L().Error("Southbound manager not initialized")
		c.sendCommandResponse(message.Header, "write_response", false, "Southbound not available", map[string]interface{}{"request_id": requestID}, deviceID)
		return
	}

	// Check if device is enabled in configuration
	c.configMu.RLock()
	deviceConfig, deviceExists := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	c.configMu.RUnlock()

	if !deviceExists {
		zap.L().Error("Device not found in configuration", zap.String("device", deviceID))
		c.sendCommandResponse(message.Header, "write_response", false, "Device not found in configuration", map[string]interface{}{"request_id": requestID}, deviceID)
		return
	}

	if !deviceConfig.Enable {
		zap.L().Error("Device is disabled in configuration", zap.String("device", deviceID))
		c.sendCommandResponse(message.Header, "write_response", false, "Device is disabled", map[string]interface{}{"request_id": requestID}, deviceID)
		return
	}

	// Find channel that contains the device
	var targetChannelID string
	found := false
	channels := c.sb.GetChannels()
	zap.L().Info("Searching for device in channels",
		zap.String("device", deviceID),
		zap.Int("channel_count", len(channels)),
	)

	// First try direct lookup by channel ID from URL if available
	// Extract channel ID from topic if possible
	topicParts = strings.Split(msg.Topic(), "/")
	if len(topicParts) >= 3 {
		channelIDFromTopic := topicParts[2]
		zap.L().Info("Trying channel from topic",
			zap.String("channel_id", channelIDFromTopic),
		)
		if dev := c.sb.GetDevice(channelIDFromTopic, deviceID); dev != nil {
			zap.L().Info("Device found using channel from topic",
				zap.String("device", deviceID),
				zap.String("channel_id", channelIDFromTopic),
			)
			targetChannelID = channelIDFromTopic
			found = true
		}
	}

	// If not found, search all channels
	if !found {
		for _, channel := range channels {
			zap.L().Info("Checking channel",
				zap.String("channel_id", channel.ID),
				zap.Int("device_count", len(channel.Devices)),
			)
			// Log device IDs in this channel
			deviceIDs := make([]string, 0, len(channel.Devices))
			for _, dev := range channel.Devices {
				deviceIDs = append(deviceIDs, dev.ID)
			}
			zap.L().Info("Devices in channel",
				zap.String("channel_id", channel.ID),
				zap.Strings("device_ids", deviceIDs),
			)
			if dev := c.sb.GetDevice(channel.ID, deviceID); dev != nil {
				zap.L().Info("Device found",
					zap.String("device", deviceID),
					zap.String("channel_id", channel.ID),
				)
				targetChannelID = channel.ID
				found = true
				break
			}
		}
	}

	if !found {
		zap.L().Error("Device not found in any channel", zap.String("device", deviceID))
		c.sendCommandResponse(message.Header, "write_response", false, "Device not found in any channel", map[string]interface{}{"request_id": requestID}, deviceID)
		return
	}

	var errors []string
	for pointID, value := range points {
		if err := c.sb.WritePoint(targetChannelID, deviceID, pointID, value); err != nil {
			zap.L().Error("Failed to write point",
				zap.String("device", deviceID),
				zap.String("point", pointID),
				zap.Error(err),
			)
			errors = append(errors, pointID+": "+err.Error())
		} else {
			zap.L().Info("Write point success",
				zap.String("device", deviceID),
				zap.String("point", pointID),
				zap.Any("value", value),
			)
		}
	}

	success := len(errors) == 0
	errorMsg := ""
	if len(errors) > 0 {
		errorMsg = strings.Join(errors, "; ")
	}
	// Add device info to response data
	responseData := map[string]interface{}{
		"request_id": requestID,
		"device_id":  deviceID,
		"node_id":    c.config.NodeID,
	}
	// Add point info if available
	if len(points) == 1 {
		for pointID, value := range points {
			responseData["point_id"] = pointID
			responseData["value"] = value
			break
		}
	}
	c.sendCommandResponse(message.Header, "write_response", success, errorMsg, responseData, deviceID)
}

// handleTaskCommand handles task control commands
func (c *Client) handleTaskCommand(client mqtt.Client, msg mqtt.Message) {
	zap.L().Warn("DEPRECATED: V1 task command received, migrate to EAN Capability Invoke",
		zap.String("topic", msg.Topic()))
	var message Message
	if err := json.Unmarshal(msg.Payload(), &message); err != nil {
		zap.L().Error("Failed to unmarshal task command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received task command",
		zap.String("message_id", message.Header.MessageID),
		zap.String("type", message.Header.MessageType),
	)

	// For now, just acknowledge the command
	// Task execution can be implemented later
	c.sendCommandResponse(message.Header, "task_response", true, "Task command received", nil, "")
}

// handleNodeRegisterCommand handles proactive node re-registration command from EdgeOS
func (c *Client) handleNodeRegisterCommand(client mqtt.Client, msg mqtt.Message) {
	var message Message
	if err := json.Unmarshal(msg.Payload(), &message); err != nil {
		zap.L().Error("Failed to unmarshal node register command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received node register command",
		zap.String("message_id", message.Header.MessageID),
		zap.String("source", message.Header.Source),
	)

	// Trigger node re-registration by publishing node online status again
	c.publishNodeOnline()
	// Re-register may not yield another register_response (node already known);
	// publish device inventory immediately so EdgeOS stays in sync.
	c.publishDeviceReport()

	// Send response back to EdgeOS
	c.sendCommandResponse(message.Header, "node_register_response", true, "Node re-registered successfully", nil, "")
}

// handleRegisterResponseCommand handles registration response from EdgeOS
func (c *Client) handleRegisterResponseCommand(client mqtt.Client, msg mqtt.Message) {
	zap.L().Info("MQTT handleRegisterResponseCommand called",
		zap.String("topic", msg.Topic()),
	)

	var message Message
	if err := json.Unmarshal(msg.Payload(), &message); err != nil {
		zap.L().Error("Failed to unmarshal register response",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received register response",
		zap.String("topic", msg.Topic()),
		zap.String("message_id", message.Header.MessageID),
		zap.String("message_type", message.Header.MessageType),
	)

	// Check if registration was successful
	body, ok := message.Body.(map[string]any)
	if !ok {
		zap.L().Error("Invalid register response body")
		return
	}

	status, hasStatus := body["status"].(string)
	zap.L().Info("Registration status check",
		zap.Bool("hasStatus", hasStatus),
		zap.String("status", status),
	)

	if hasStatus && status == "success" {
		zap.L().Info("Node registration successful, publishing device report",
			zap.String("node_id", message.Header.Source),
		)
		// Publish device report after successful registration
		c.publishDeviceReport()
	} else {
		zap.L().Warn("Node registration not successful",
			zap.String("status", status),
		)
	}
}

// scheduleDeviceReportFallback republishes inventory if the connect-time
// publish failed/was lost and no later successful report arrived within 5s.
func (c *Client) scheduleDeviceReportFallback() {
	gen := c.deviceReportGen.Add(1)
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-c.stopChan:
			return
		case <-timer.C:
		}
		if c.deviceReportGen.Load() != gen {
			return
		}
		if c.deviceReportOK.Load() {
			return
		}
		if c.GetStatus() != StatusConnected {
			return
		}
		zap.L().Info("device_report fallback: republishing inventory after connect",
			zap.String("node_id", c.nodeID),
		)
		c.publishDeviceReport()
	}()
}

// publishDeviceReport publishes all device information to EdgeOS
func (c *Client) publishDeviceReport() {
	zap.L().Info("publishDeviceReport called")

	if c.sb == nil {
		zap.L().Error("Southbound manager not initialized, cannot publish device report")
		return
	}

	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	zap.L().Info("Publishing device report",
		zap.String("node_id", nodeID),
	)

	// Get all channels and devices
	channels := c.sb.GetChannels()
	var devices []map[string]any

	for _, channel := range channels {
		for _, device := range channel.Devices {
			// Map device state to operating_state
			operatingState := "ENABLED"
			switch device.State {
			case 2: // Offline
				operatingState = "DISABLED"
			case 1: // Unstable
				operatingState = "UNSTABLE"
			case 3: // Quarantine
				operatingState = "QUARANTINE"
			}

			deviceInfo := map[string]any{
				"device_id":       device.ID,
				"device_name":     device.Name,
				"device_profile":  channel.Protocol, // Use protocol as profile
				"service_name":    channel.Name,     // Use channel name as service
				"labels":          []string{},
				"description":     "",
				"admin_state":     "ENABLED",
				"operating_state": operatingState,
				"properties": map[string]any{
					"protocol":   channel.Protocol,
					"channel_id": channel.ID,
				},
			}

			// Add config properties
			if device.Config != nil {
				for k, v := range device.Config {
					deviceInfo["properties"].(map[string]any)[k] = v
				}
			}

			devices = append(devices, deviceInfo)
		}
	}

	reportMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_report",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id": nodeID,
			"devices": devices,
		},
	}

	if err := c.publishMessage("edgex/devices/report", reportMessage, 1); err != nil {
		zap.L().Error("Failed to publish device report",
			zap.Error(err),
			zap.String("node_id", nodeID),
		)
	} else {
		c.deviceReportOK.Store(true)
		zap.L().Info("Device report published successfully",
			zap.String("node_id", nodeID),
			zap.Int("device_count", len(devices)),
		)
		// Immediately publish points metadata after device report
		if err := c.PublishPointsMetadata(); err != nil {
			zap.L().Error("Failed to publish points metadata after device report",
				zap.Error(err),
				zap.String("node_id", nodeID),
			)
		}
	}
}

// PublishDeviceReport republishes the device inventory (edgex/devices/report) to EdgeOS.
// Used when channels/devices are added/updated or the northbound Devices mapping changes,
// so EdgeOS stays in sync without requiring a re-connect.
func (c *Client) PublishDeviceReport() {
	c.publishDeviceReport()
}

// sendCommandResponse sends a response to a command
func (c *Client) sendCommandResponse(header MessageHeader, msgType string, success bool, message string, data interface{}, deviceID string) {
	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	// Extract request_id from data if available
	requestID := header.MessageID
	if dataMap, ok := data.(map[string]interface{}); ok {
		if rid, exists := dataMap["request_id"]; exists {
			if ridStr, ok := rid.(string); ok {
				requestID = ridStr
			}
		}
	}

	response := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			Destination: header.Source,
			MessageType: msgType,
			Version:     "1.0",
			RequestID:   requestID,
		},
		Body: map[string]any{
			"success": success,
			"message": message,
			"data":    map[string]any{},
		},
	}

	if data != nil {
		response.Body.(map[string]any)["data"] = data
	}

	// Use single topic for both success and error responses
	topic := fmt.Sprintf("edgex/cmd/responses/%s/%s", nodeID, deviceID)

	if err := c.publishMessage(topic, response, 1); err != nil {
		zap.L().Error("Failed to send command response",
			zap.Error(err),
			zap.String("message_id", header.MessageID),
			zap.String("request_id", requestID),
			zap.String("topic", topic),
		)
	}
}

// Publish publishes a value to edgeOS
func (c *Client) Publish(v model.Value) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	deviceConfig, ok := model.LookupNorthboundPublishConfig(v.DeviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	c.configMu.RUnlock()

	if !ok {
		return
	}

	// Handle based on strategy
	if deviceConfig.Strategy == "periodic" {
		// Periodic mode: aggregate points
		c.aggregatePoint(v.DeviceID, v, deviceConfig.Interval)
	} else {
		// Realtime mode (default): push immediately with device-level aggregation
		c.publishDeviceData(v.DeviceID, map[string]any{v.PointID: v.Value}, v.Quality, v.TS)
	}
}

// aggregatePoint aggregates a point for periodic push
func (c *Client) aggregatePoint(deviceID string, v model.Value, interval model.Duration) {
	c.aggregatorMu.Lock()

	// Get or create aggregator
	agg, exists := c.deviceAggregators[deviceID]
	if !exists {
		agg = &deviceAggregator{
			points:       make(map[string]model.Value),
			lastPushTS:   time.Now(),
			pushInterval: time.Duration(interval),
		}
		c.deviceAggregators[deviceID] = agg
	}

	// Update point value
	agg.mu.Lock()
	agg.points[v.PointID] = v
	agg.mu.Unlock()
	c.aggregatorMu.Unlock()
}

// periodicPushLoop triggers periodic device-level pushes
func (c *Client) periodicPushLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkAndPushPeriodicDevices()
		}
	}
}

// heartbeatLoop sends heartbeat messages periodically
func (c *Client) heartbeatLoop() {
	c.configMu.RLock()
	intervalStr := c.config.HeartbeatInterval
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-02/03): V1 命令面开关——false 时不再运行 V1 心跳循环（EAN Heartbeat 已覆盖）。
	// | V1 command plane switch: when false, stop V1 heartbeat loop (EAN heartbeat covers it).
	if !v1CmdEnabled {
		c.logger.Info("V1 command plane disabled; V1 heartbeat loop skipped (EAN $edgeos/heartbeat/{agent} active)")
		return
	}

	interval := 30 * time.Second
	if intervalStr != "" {
		var err error
		interval, err = time.ParseDuration(intervalStr)
		if err != nil {
			c.logger.Warn("Failed to parse heartbeat interval, using default 30s",
				zap.String("interval_str", intervalStr),
				zap.Error(err),
			)
			interval = 30 * time.Second
		}
	}

	c.heartbeatInterval = interval
	c.heartbeatTicker = time.NewTicker(interval)
	defer c.heartbeatTicker.Stop()

	c.logger.Info("Heartbeat loop started",
		zap.Duration("interval", interval),
	)

	// Phase 4 (EX-P4-02): V1 心跳上报标记 deprecated——EAN $edgeos/heartbeat/{agent} 已替代。
	// | V1 heartbeat publish marked DEPRECATED; superseded by EAN heartbeat.
	c.logger.Warn("DEPRECATED: V1 heartbeat loop (edgex/heartbeat/{node}) retained for compat; use EAN $edgeos/heartbeat/{agent}",
		zap.Duration("interval", interval))

	for {
		select {
		case <-c.stopChan:
			c.logger.Info("Heartbeat loop stopped")
			return
		case <-c.heartbeatTicker.C:
			c.PublishHeartbeat(nil)
		}
	}
}

// checkAndPushPeriodicDevices checks which devices need periodic push
func (c *Client) checkAndPushPeriodicDevices() {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.aggregatorMu.RLock()
	defer c.aggregatorMu.RUnlock()

	now := time.Now()

	for deviceID, agg := range c.deviceAggregators {
		if now.Sub(agg.lastPushTS) >= agg.pushInterval {
			c.publishAggregatedDevice(deviceID, agg)
		}
	}
}

// publishAggregatedDevice publishes all aggregated points for a device
func (c *Client) publishAggregatedDevice(deviceID string, agg *deviceAggregator) {
	agg.mu.RLock()
	defer agg.mu.RUnlock()

	if len(agg.points) == 0 {
		return
	}

	// Build points map and find latest timestamp
	points := make(map[string]any)
	var latestTS time.Time
	var quality string

	for _, v := range agg.points {
		points[v.PointID] = v.Value
		if v.TS.After(latestTS) {
			latestTS = v.TS
		}
		if v.Quality != "" {
			quality = v.Quality
		}
	}

	if quality == "" {
		quality = "good"
	}

	// Publish device-level data
	c.publishDeviceData(deviceID, points, quality, latestTS)

	// Update last push time
	agg.lastPushTS = time.Now()
}

// publishDeviceData publishes device-level data with all points
func (c *Client) publishDeviceData(deviceID string, points map[string]any, quality string, ts time.Time) {
	topic := fmt.Sprintf("edgex/data/%s/%s", c.nodeID, deviceID)
	dataMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      c.nodeID,
			MessageType: "data",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":   c.nodeID,
			"device_id": deviceID,
			"timestamp": ts.UnixMilli(),
			"points":    points,
			"quality":   quality,
		},
	}

	if err := c.publishMessage(topic, dataMessage, 0); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device data to edgeOS(MQTT)",
			zap.Error(err),
			zap.String("device", deviceID),
			zap.Int("points_count", len(points)),
		)
	} else {
		atomic.AddInt64(&c.successCount, 1)
		atomic.AddInt64(&c.publishCount, 1)
	}
}

// PublishDeviceStatus publishes device status changes
func (c *Client) PublishDeviceStatus(deviceID string, status int) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	_, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	if !ok {
		return
	}

	topic := fmt.Sprintf("edgex/devices/%s/%s/status", nodeID, deviceID)
	statusStr := "online"
	if status != 0 {
		statusStr = "offline"
	}

	statusMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_status",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":   nodeID,
			"device_id": deviceID,
			"status":    statusStr,
			"timestamp": time.Now().UnixMilli(),
		},
	}

	if err := c.publishMessage(topic, statusMessage, 1); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device status",
			zap.Error(err),
			zap.String("device", deviceID),
			zap.String("topic", topic),
		)
	} else {
		atomic.AddInt64(&c.successCount, 1)
		zap.L().Info("Published device status",
			zap.String("node_id", nodeID),
			zap.String("device_id", deviceID),
			zap.String("status", statusStr),
			zap.String("topic", topic),
		)
	}
}

// PublishHeartbeat publishes enriched node heartbeat
func (c *Client) PublishHeartbeat(metrics map[string]any) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	nodeID := c.nodeID
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-02/03): V1 命令面开关——false 时不再发布 V1 心跳（EAN Heartbeat 已覆盖）。
	// | V1 command plane switch: when false, skip V1 heartbeat publish (EAN heartbeat covers it).
	if !v1CmdEnabled {
		return
	}

	// Get current stats
	stats := c.GetStats()

	// Build enriched heartbeat message
	heartbeatBody := HeartbeatMessage{
		NodeID:         nodeID,
		Status:         c.getStatusString(),
		Timestamp:      time.Now().UnixMilli(),
		Sequence:       atomic.AddInt64(&c.publishCount, 1),
		UptimeSeconds:  c.calculateUptime(),
		Version:        "1.0.0",
		SystemMetrics:  c.collectSystemMetrics(),
		DeviceSummary:  c.collectDeviceSummary(),
		ChannelSummary: c.collectChannelSummary(),
		TaskSummary: TaskSummary{
			TotalCount:   0,
			RunningCount: 0,
			PausedCount:  0,
			ErrorCount:   0,
		},
		ConnectionStats: ConnectionStats{
			ReconnectCount:  stats.ReconnectCount,
			LastOnlineTime:  stats.LastOnlineTime,
			LastOfflineTime: stats.LastOfflineTime,
			ConnectedSince:  stats.LastOnlineTime,
			PublishCount:    stats.PublishCount,
			ProtocolVersion: "MQTTv3.1.1",
		},
		CustomMetrics: metrics,
	}

	heartbeatMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "heartbeat",
			Version:     "1.0",
		},
		Body: heartbeatBody,
	}

	topic := fmt.Sprintf("edgex/heartbeat/%s", nodeID)
	if err := c.publishMessage(topic, heartbeatMessage, 0); err != nil {
		zap.L().Error("Failed to publish heartbeat",
			zap.Error(err),
		)
	}
}

// getStatusString returns the current status as a string
func (c *Client) getStatusString() string {
	switch c.GetStatus() {
	case StatusConnected:
		return "active"
	case StatusDisconnected:
		return "offline"
	case StatusReconnecting:
		return "reconnecting"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// calculateUptime calculates the node uptime in seconds
func (c *Client) calculateUptime() int64 {
	lastOnline := atomic.LoadInt64(&c.lastOnlineTime)
	if lastOnline == 0 {
		return 0
	}
	return (time.Now().UnixMilli() - lastOnline) / 1000
}

// collectSystemMetrics collects system metrics using gopsutil
func (c *Client) collectSystemMetrics() SystemMetrics {
	// Get CPU usage
	cpuUsage := 0.0
	if cpuInfos, err := cpu.Percent(0, false); err == nil && len(cpuInfos) > 0 {
		cpuUsage = cpuInfos[0]
	}

	// Get memory stats
	memoryUsage := 0.0
	memoryTotal := int64(0)
	memoryUsed := int64(0)
	if memStats, err := mem.VirtualMemory(); err == nil {
		memoryUsage = memStats.UsedPercent
		memoryTotal = int64(memStats.Total)
		memoryUsed = int64(memStats.Used)
	}

	// Get disk stats
	diskUsage := 0.0
	diskTotal := int64(0)
	diskUsed := int64(0)
	if diskStats, err := disk.Usage("/"); err == nil {
		diskUsage = diskStats.UsedPercent
		diskTotal = int64(diskStats.Total)
		diskUsed = int64(diskStats.Used)
	}

	return SystemMetrics{
		CPUUsage:       cpuUsage,
		MemoryUsage:    memoryUsage,
		MemoryTotal:    memoryTotal,
		MemoryUsed:     memoryUsed,
		DiskUsage:      diskUsage,
		DiskTotal:      diskTotal,
		DiskUsed:       diskUsed,
		LoadAverage:    0,
		NetworkRXBytes: 0,
		NetworkTXBytes: 0,
		ProcessCount:   runtime.NumGoroutine(),
		ThreadCount:    0,
	}
}

// collectDeviceSummary collects device statistics summary from southbound manager
func (c *Client) collectDeviceSummary() DeviceSummary {
	if c.sb == nil {
		return DeviceSummary{}
	}

	summary := DeviceSummary{}
	channels := c.sb.GetChannels()

	for _, ch := range channels {
		summary.TotalCount += len(ch.Devices)
		devices := c.sb.GetChannelDevices(ch.ID)
		for _, dev := range devices {
			switch dev.State {
			case 0:
				summary.OnlineCount++
			case 1:
				summary.DegradedCount++
			case 2:
				summary.OfflineCount++
			case 3:
				summary.ErrorCount++
			default:
				summary.OfflineCount++
			}
		}
	}

	return summary
}

// collectChannelSummary collects channel statistics summary from southbound manager
func (c *Client) collectChannelSummary() ChannelSummary {
	if c.sb == nil {
		return ChannelSummary{}
	}

	summary := ChannelSummary{}
	channels := c.sb.GetChannels()
	summary.TotalCount = len(channels)

	for _, ch := range channels {
		if ch.Enable {
			summary.ConnectedCount++
		}
	}

	return summary
}

// publishMessage publishes a message with proper error handling
func (c *Client) publishMessage(topic string, msg Message, qos byte) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	token := c.client.Publish(topic, qos, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	zap.L().Debug("Published to edgeOS(MQTT)",
		zap.String("topic", topic),
		zap.String("message_type", msg.Header.MessageType),
		zap.Int("bytes", len(data)),
	)

	return nil
}

// scheduleReconnect starts a single reconnect loop if one is not already running.
func (c *Client) scheduleReconnect() {
	if !c.reconnectSched.TryStart() {
		return
	}
	go c.reconnectLogic()
}

// reconnectLogic handles reconnection attempts
func (c *Client) reconnectLogic() {
	defer c.reconnectSched.Done()

	var logThrottle reconnect.LogThrottle
	retryCount := 0

	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		if c.client != nil && c.client.IsConnected() {
			return
		}

		c.setStatus(StatusReconnecting)
		attempt := retryCount + 1
		if logThrottle.ShouldLog(attempt, 30*time.Second, 10) {
			zap.L().Info("edgeOS(MQTT) reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("node_id", c.nodeID),
			)
		} else {
			zap.L().Debug("edgeOS(MQTT) reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("node_id", c.nodeID),
			)
		}

		token := c.client.Connect()
		if token.Wait() && token.Error() == nil {
			atomic.AddInt64(&c.reconnectCount, 1)
			zap.L().Info("edgeOS(MQTT) reconnected",
				zap.String("node_id", c.nodeID),
			)
			return
		}

		retryCount++
		delay := reconnect.Backoff(retryCount)

		if retryCount <= 10 {
			if logThrottle.ShouldLog(retryCount, 30*time.Second, 10) {
				zap.L().Warn("edgeOS(MQTT) reconnect failed, retrying",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
				)
			} else {
				zap.L().Debug("edgeOS(MQTT) reconnect failed, retrying",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
				)
			}
		} else {
			c.setStatus(StatusError)
			if logThrottle.ShouldLog(retryCount, 60*time.Second, 10) {
				zap.L().Error("edgeOS(MQTT) reconnect failed repeatedly, backing off",
					zap.Int("attempts", retryCount),
					zap.Duration("backoff", delay),
				)
			}
			retryCount = 0
		}

		select {
		case <-c.stopChan:
			return
		case <-time.After(delay):
		}
	}
}

// retryLoop handles periodic retries
func (c *Client) retryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if c.GetStatus() == StatusDisconnected || c.GetStatus() == StatusError {
				c.scheduleReconnect()
			}
		}
	}
}

// setStatus sets the connection status
func (c *Client) setStatus(s int) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status = s
}

// Stop stops the client
func (c *Client) Stop() {
	// 使用 StopCapabilityRuntime 而非 stopEAN：确保 eanRuntime 置 nil，
	// 避免重连后 OnConnect 误判 Runtime 已存在而跳过创建。
	// | Use StopCapabilityRuntime (not stopEAN) to nil out eanRuntime,
	// | preventing OnConnect from skipping Runtime creation after reconnect.
	c.StopCapabilityRuntime()

	close(c.stopChan)

	if c.client != nil && c.client.IsConnected() {
		c.configMu.RLock()
		nodeID := c.config.NodeID
		c.configMu.RUnlock()

		// Publish offline status (V1.0 compat)
		topic := fmt.Sprintf("edgex/nodes/%s/offline", nodeID)
		payload := `{"status":"offline","timestamp":` + fmt.Sprintf("%d", time.Now().UnixMilli()) + `}`
		token := c.client.Publish(topic, 1, true, payload)
		token.WaitTimeout(2 * time.Second)

		c.client.Disconnect(250)
	}

	c.setStatus(StatusDisconnected)
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "msg-" + hex.EncodeToString(b)
}

// PublishRaw publishes raw data to a specific topic
func (c *Client) PublishRaw(topic string, payload []byte) error {
	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	token := c.client.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		atomic.AddInt64(&c.failCount, 1)
		return token.Error()
	}

	atomic.AddInt64(&c.successCount, 1)
	atomic.AddInt64(&c.publishCount, 1)
	return nil
}

// PublishPointsMetadata publishes point definitions metadata to EdgeOS for each device separately
func (c *Client) PublishPointsMetadata() error {
	zap.L().Info("PublishPointsMetadata called")

	if c.client == nil || !c.client.IsConnected() {
		zap.L().Error("Client not connected, cannot publish points metadata")
		return fmt.Errorf("client not connected")
	}

	zap.L().Info("Client is connected, proceeding with points metadata")

	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	if c.sb == nil {
		return fmt.Errorf("southbound manager not initialized")
	}

	// Get all channels and devices
	channels := c.sb.GetChannels()
	totalDevices := 0
	totalPoints := 0

	for _, channel := range channels {
		for _, device := range channel.Devices {
			// Check if device is enabled in config
			c.configMu.RLock()
			devices := c.config.Devices
			c.configMu.RUnlock()

			if len(devices) > 0 {
				if deviceConfig, ok := devices[device.ID]; !ok || !deviceConfig.Enable {
					continue
				}
			}

			// Get device points
			points, err := c.sb.GetDevicePoints(channel.ID, device.ID)
			if err != nil {
				zap.L().Warn("Failed to get device points for metadata",
					zap.String("device", device.ID),
					zap.Error(err),
				)
				continue
			}

			if len(points) == 0 {
				continue
			}

			// Build point list for this device
			var devicePoints []map[string]any
			for _, point := range points {
				pointInfo := map[string]any{
					"address":    point.Address,
					"data_type":  point.DataType,
					"point_id":   point.ID,
					"point_name": point.Name,
					"rw":         point.ReadWrite,
					"unit":       point.Unit,
				}
				devicePoints = append(devicePoints, pointInfo)
			}

			// Send point report for this device separately
			metadataMessage := Message{
				Header: MessageHeader{
					MessageID:   generateMessageID(),
					Timestamp:   time.Now().UnixMilli(),
					Source:      nodeID,
					MessageType: "point_report",
					Version:     "1.0",
				},
				Body: map[string]any{
					"channel_id":  channel.ID,
					"device_id":   device.ID,
					"node_id":     nodeID,
					"device_name": device.Name,
					"points":      devicePoints,
				},
			}

			if err := c.publishMessage("edgex/points/report", metadataMessage, 1); err != nil {
				zap.L().Error("Failed to publish points metadata for device",
					zap.Error(err),
					zap.String("node_id", nodeID),
					zap.String("device_id", device.ID),
				)
				continue
			}

			totalDevices++
			totalPoints += len(devicePoints)
		}
	}

	zap.L().Info("Points metadata published successfully",
		zap.String("node_id", nodeID),
		zap.Int("device_count", totalDevices),
		zap.Int("total_point_count", totalPoints),
	)
	return nil
}

// PublishPointsSync publishes all point current values for a specific device to EdgeOS
func (c *Client) PublishPointsSync(channelID, deviceID string) error {
	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	if c.sb == nil {
		return fmt.Errorf("southbound manager not initialized")
	}

	// Get device points
	points, err := c.sb.GetDevicePoints(channelID, deviceID)
	if err != nil {
		zap.L().Warn("Failed to get device points for sync",
			zap.String("device", deviceID),
			zap.Error(err),
		)
		return err
	}

	// Build point values map
	pointValues := make(map[string]any)
	var latestTS time.Time
	var quality string

	for _, point := range points {
		pointValues[point.ID] = point.Value
		if point.Quality != "" {
			quality = point.Quality
		}
		if !point.Timestamp.IsZero() && point.Timestamp.After(latestTS) {
			latestTS = point.Timestamp
		}
	}

	if quality == "" {
		quality = "good"
	}
	if latestTS.IsZero() {
		latestTS = time.Now()
	}

	syncMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "points_sync",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":   nodeID,
			"device_id": deviceID,
			"timestamp": latestTS.UnixMilli(),
			"points":    pointValues,
			"quality":   quality,
		},
	}

	topic := fmt.Sprintf("edgex/points/%s/%s", nodeID, deviceID)
	if err := c.publishMessage(topic, syncMessage, 1); err != nil {
		zap.L().Error("Failed to publish points sync",
			zap.Error(err),
			zap.String("device", deviceID),
		)
		return err
	}

	zap.L().Info("Points sync published successfully",
		zap.String("node_id", nodeID),
		zap.String("device_id", deviceID),
		zap.Int("point_count", len(pointValues)),
	)
	return nil
}

// PublishDeviceOnline publishes sub-device online notification to EdgeOS
func (c *Client) PublishDeviceOnline(deviceID, deviceName string, details map[string]any) error {
	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	onlineMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_online",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":     nodeID,
			"device_id":   deviceID,
			"device_name": deviceName,
			"online_time": time.Now().UnixMilli(),
			"status":      "online",
			"details":     details,
		},
	}

	topic := fmt.Sprintf("edgex/devices/%s/%s/online", nodeID, deviceID)
	if err := c.publishMessage(topic, onlineMessage, 2); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device online notification",
			zap.Error(err),
			zap.String("device_id", deviceID),
			zap.String("topic", topic),
		)
		return err
	}

	atomic.AddInt64(&c.successCount, 1)
	atomic.AddInt64(&c.publishCount, 1)
	zap.L().Info("Published device online notification",
		zap.String("node_id", nodeID),
		zap.String("device_id", deviceID),
		zap.String("device_name", deviceName),
		zap.String("topic", topic),
	)
	return nil
}

// PublishDeviceOffline publishes sub-device offline notification to EdgeOS
func (c *Client) PublishDeviceOffline(deviceID, deviceName, reason string, details map[string]any) error {
	if c.client == nil || !c.client.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	c.configMu.RLock()
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	offlineMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_offline",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":      nodeID,
			"device_id":    deviceID,
			"device_name":  deviceName,
			"offline_time": time.Now().UnixMilli(),
			"status":       "offline",
			"reason":       reason,
			"details":      details,
		},
	}

	topic := fmt.Sprintf("edgex/devices/%s/%s/offline", nodeID, deviceID)
	if err := c.publishMessage(topic, offlineMessage, 2); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device offline notification",
			zap.Error(err),
			zap.String("device_id", deviceID),
			zap.String("topic", topic),
		)
		return err
	}

	atomic.AddInt64(&c.successCount, 1)
	atomic.AddInt64(&c.publishCount, 1)
	zap.L().Info("Published device offline notification",
		zap.String("node_id", nodeID),
		zap.String("device_id", deviceID),
		zap.String("device_name", deviceName),
		zap.String("reason", reason),
		zap.String("topic", topic),
	)
	return nil
}
