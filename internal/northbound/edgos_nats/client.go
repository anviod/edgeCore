package edgos_nats

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/reconnect"
	"github.com/anviod/edgeCore/internal/storage"

	nats "github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	StatusDisconnected = 0
	StatusConnected    = 1
	StatusReconnecting = 2
	StatusError        = 3
)

// MessageHeader represents standard edgeOS message header
type MessageHeader struct {
	MessageID     string `json:"message_id"`
	Timestamp     int64  `json:"timestamp"`
	Source        string `json:"source"`
	Destination   string `json:"destination,omitempty"`
	MessageType   string `json:"message_type"`
	Version       string `json:"version"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Message represents a standard edgeOS message
type Message struct {
	Header MessageHeader `json:"header"`
	Body   interface{}   `json:"body"`
}

// Client implements edgeOS(NATS) northbound channel
type Client struct {
	config   model.EdgeOSNATSConfig
	configMu sync.RWMutex
	nc       *nats.Conn
	js       nats.JetStreamContext
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

	// Subscriptions
	subscriptions map[string]*nats.Subscription
	subMu         sync.Mutex

	// Device aggregation for periodic push
	deviceAggregators map[string]*deviceAggregator
	aggregatorMu      sync.RWMutex

	reconnectSched reconnect.Scheduler

	// EAN 2.0 Capability Runtime (optional; V1.0 edgeCore.* subjects remain unchanged)
	eanMu               sync.RWMutex
	eanRuntime          *capability.Runtime
	onEANRuntimeChanged func()

	// deviceReportGen bumps on each connect; fallback timer ignores stale gens.
	// deviceReportOK is set after a successful publishDeviceReport in this generation.
	deviceReportGen atomic.Uint64
	deviceReportOK  atomic.Bool
}

// NewClient creates a new edgeOS(NATS) client
func NewClient(cfg model.EdgeOSNATSConfig, sb model.SouthboundManager, s *storage.Storage) *Client {
	logger := zap.L().With(
		zap.String("component", "edgos-nats-client"),
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
		subscriptions:     make(map[string]*nats.Subscription),
		deviceAggregators: make(map[string]*deviceAggregator),
	}
}

// GetStatus returns current connection status
func (c *Client) GetStatus() int {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

// GetStats returns client statistics (including EAN Runtime metrics if enabled)
func (c *Client) GetStats() EdgeOSNATSStats {
	stats := EdgeOSNATSStats{
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

// EdgeOSNATSStats represents client statistics
type EdgeOSNATSStats struct {
	SuccessCount    int64                             `json:"success_count"`
	FailCount       int64                             `json:"fail_count"`
	ReconnectCount  int64                             `json:"reconnect_count"`
	PublishCount    int64                             `json:"publish_count"`
	LastOfflineTime int64                             `json:"last_offline_time"`
	LastOnlineTime  int64                             `json:"last_online_time"`
	EANMetrics      *capability.InvokeMetricsSnapshot `json:"ean_metrics,omitempty"`
}

// deviceAggregator aggregates points for periodic device-level push
type deviceAggregator struct {
	points       map[string]model.Value // pointID -> Value
	lastPushTS   time.Time
	pushInterval time.Duration
	mu           sync.RWMutex
}

// UpdateConfig updates client configuration.
// 连接级字段（URL/ClientID/Username/Password/NodeID）变化触发全量重连；
// EAN 字段（EANEnabled/EANHeartbeatSec）变化走热更新路径，不中断北向连接。
// | Connection-level fields trigger full restart;
// | EAN fields are hot-applied without dropping the northbound connection.
func (c *Client) UpdateConfig(cfg model.EdgeOSNATSConfig) error {
	// 在锁内捕获旧 EAN 状态和重连决策，锁外执行重连/热更新以避免死锁
	// | Capture old EAN state and restart decision under lock;
	// | execute restart/hot-update outside lock to avoid deadlock.
	c.configMu.Lock()
	oldEANEnabled := c.config.EANEnabled
	oldHeartbeat := c.config.EANHeartbeatSec
	oldAutoPublish := c.config.EANEventAutoPublish
	oldV1Cmd := c.config.V1CommandEnabled
	oldDevices := c.config.Devices

	needRestart := c.config.URL != cfg.URL ||
		c.config.ClientID != cfg.ClientID ||
		c.config.Username != cfg.Username ||
		c.config.Password != cfg.Password ||
		c.config.NodeID != cfg.NodeID ||
		// Phase 4 (EX-P4): V1 命令面开关变化 → 全量重连以重新订阅/下线 V1 命令 Subject
		// | V1 command plane switch change → full reconnect to (re)subscribe V1 command subjects
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

	// 设备映射变化：重新上报设备清单（edgeCore.devices.report），确保 EdgeOS 设备列表同步。
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
				zap.L().Warn("EAN hot-start failed (NATS)", zap.Error(err))
			} else if rt != nil {
				c.StartEAN(context.Background())
				zap.L().Info("EAN Runtime hot-started (NATS) (EANEnabled false→true)")
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
				zap.L().Warn("EAN hot-restart failed (NATS) (heartbeat change)", zap.Error(err))
			} else if rt != nil {
				c.StartEAN(context.Background())
				zap.L().Info("EAN Runtime hot-restarted (NATS) (heartbeat changed)",
					zap.Int("old_sec", oldHeartbeat),
					zap.Int("new_sec", newHeartbeat))
			}
		}
	}

	// EANEventAutoPublish 变化：刷新 Shadow→Event publisher（EAN 仍启用时）
	// | EANEventAutoPublish changed while EAN enabled: refresh Shadow→Event publishers
	if newEnabled && oldAutoPublish != newAutoPublish {
		c.notifyEANRuntimeChanged()
		zap.L().Info("EAN event auto-publish toggled (NATS); Shadow→Event publishers refreshed",
			zap.Bool("old", oldAutoPublish),
			zap.Bool("new", newAutoPublish))
	}
}

// Start starts the edgeOS(NATS) client
func (c *Client) Start() error {
	go c.connectLoop()
	go c.retryLoop()
	go c.periodicPushLoop()
	return nil
}

// connectLoop performs the initial NATS connection attempt.
func (c *Client) connectLoop() {
	c.setStatus(StatusReconnecting)

	if err := c.doConnect(); err != nil {
		c.configMu.RLock()
		url := c.config.URL
		nodeID := c.config.NodeID
		c.configMu.RUnlock()

		zap.L().Error("Initial edgeOS(NATS) connection failed",
			zap.Error(err),
			zap.String("url", url),
			zap.String("node_id", nodeID),
			zap.String("component", "edgos-nats-client"),
		)
		c.setStatus(StatusDisconnected)
		atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
		c.scheduleReconnect()
	}
}

func (c *Client) buildNatsOptions(clientID, nodeID, username, password string) []nats.Option {
	opts := []nats.Option{
		nats.Name(clientID),
		nats.MaxReconnects(0), // disable built-in reconnect; we control it manually
		nats.PingInterval(20 * time.Second),
		nats.MaxPingsOutstanding(5),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			select {
			case <-c.stopChan:
				return
			default:
			}
			zap.L().Warn("edgeOS(NATS) Disconnected",
				zap.Error(err),
				zap.String("node_id", nodeID),
				zap.String("component", "edgos-nats-client"),
			)
			c.setStatus(StatusDisconnected)
			atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
			c.scheduleReconnect()
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			select {
			case <-c.stopChan:
				return
			default:
			}
			zap.L().Info("edgeOS(NATS) Connection Closed",
				zap.String("node_id", nodeID),
				zap.String("component", "edgos-nats-client"),
			)
			c.setStatus(StatusDisconnected)
			c.scheduleReconnect()
		}),
	}
	if username != "" && password != "" {
		opts = append(opts, nats.UserInfo(username, password))
	}
	return opts
}

func (c *Client) doConnect() error {
	c.configMu.RLock()
	url := c.config.URL
	clientID := c.config.ClientID
	username := c.config.Username
	password := c.config.Password
	nodeID := c.config.NodeID
	jetStreamEnabled := c.config.JetStreamEnabled
	c.configMu.RUnlock()

	if c.nc != nil {
		c.subMu.Lock()
		for subject, sub := range c.subscriptions {
			if err := sub.Unsubscribe(); err != nil {
				zap.L().Error("Failed to unsubscribe during reconnect",
					zap.Error(err),
					zap.String("subject", subject),
				)
			}
			delete(c.subscriptions, subject)
		}
		c.subMu.Unlock()
		c.nc.Close()
		c.nc = nil
		c.js = nil
	}

	opts := c.buildNatsOptions(clientID, nodeID, username, password)
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return err
	}
	c.nc = nc

	if jetStreamEnabled {
		c.js, err = c.nc.JetStream()
		if err != nil {
			zap.L().Error("Failed to enable JetStream",
				zap.Error(err),
				zap.String("node_id", nodeID),
			)
		} else {
			zap.L().Info("edgeOS(NATS) JetStream enabled",
				zap.String("node_id", nodeID),
			)
		}
	}

	c.setStatus(StatusConnected)
	atomic.StoreInt64(&c.lastOnlineTime, time.Now().UnixMilli())

	zap.L().Info("edgeOS(NATS) Connected",
		zap.String("url", url),
		zap.String("node_id", nodeID),
		zap.String("component", "edgos-nats-client"),
	)

	// Subscribe before publishing registration so we do not miss
	// register_response (which also triggers publishDeviceReport).
	c.subscribeToCommands()
	c.publishNodeOnline()

	// Inventory on connect (same as re-register): EdgeOS may skip
	// register_response when the node is already known.
	c.deviceReportOK.Store(false)
	c.publishDeviceReport()
	c.scheduleDeviceReportFallback()

	// EAN 2.0 Capability Runtime: auto-ensure + publish $edgeos/* descriptors.
	// V1.0 edgeCore.* compatibility paths above are unchanged.
	// EnsureCapabilityRuntime returns (nil, nil) when EANEnabled=false — skip start in that case.
	rt, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion)
	if err != nil {
		zap.L().Warn("Failed to ensure EAN Capability Runtime (NATS)",
			zap.Error(err),
			zap.String("node_id", nodeID),
		)
	} else if rt != nil {
		c.startEANLocked(context.Background())
	} else {
		zap.L().Info("EAN capability layer disabled, skipping Runtime start (NATS)",
			zap.String("node_id", nodeID))
	}
	return nil
}

func (c *Client) scheduleReconnect() {
	if !c.reconnectSched.TryStart() {
		return
	}
	go c.reconnectLogic()
}

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

		if c.nc != nil && c.nc.IsConnected() {
			return
		}

		c.setStatus(StatusReconnecting)
		attempt := retryCount + 1

		c.configMu.RLock()
		url := c.config.URL
		nodeID := c.config.NodeID
		c.configMu.RUnlock()

		if logThrottle.ShouldLog(attempt, 30*time.Second, 10) {
			zap.L().Info("edgeOS(NATS) reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("url", url),
				zap.String("node_id", nodeID),
				zap.String("component", "edgos-nats-client"),
			)
		} else {
			zap.L().Debug("edgeOS(NATS) reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("url", url),
				zap.String("node_id", nodeID),
				zap.String("component", "edgos-nats-client"),
			)
		}

		if err := c.doConnect(); err == nil {
			atomic.AddInt64(&c.reconnectCount, 1)
			zap.L().Info("edgeOS(NATS) reconnected",
				zap.String("url", url),
				zap.String("node_id", nodeID),
				zap.String("component", "edgos-nats-client"),
			)
			return
		}

		retryCount++
		delay := reconnect.Backoff(retryCount)

		if retryCount <= 10 {
			if logThrottle.ShouldLog(retryCount, 30*time.Second, 10) {
				zap.L().Warn("edgeOS(NATS) reconnect failed, retrying",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
					zap.String("node_id", nodeID),
					zap.String("component", "edgos-nats-client"),
				)
			} else {
				zap.L().Debug("edgeOS(NATS) reconnect failed, retrying",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
					zap.String("node_id", nodeID),
					zap.String("component", "edgos-nats-client"),
				)
			}
		} else {
			c.setStatus(StatusError)
			if logThrottle.ShouldLog(retryCount, 60*time.Second, 10) {
				zap.L().Error("edgeOS(NATS) reconnect failed repeatedly, backing off",
					zap.Int("attempts", retryCount),
					zap.Duration("backoff", delay),
					zap.String("node_id", nodeID),
					zap.String("component", "edgos-nats-client"),
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
	zap.L().Warn("DEPRECATED: publishing V1 node registration (edgeCore.nodes.*); use EAN $edgeos/discovery/agent",
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
		Body: map[string]interface{}{
			"node_id":      nodeID,
			"node_name":    "edgeCore Gateway Node",
			"model":        "edgeCore",
			"version":      "1.0.0",
			"api_version":  "v1",
			"capabilities": []string{"shadow-sync", "heartbeat", "device-control", "task-execution"},
			"protocol":     "edgeOS(NATS)",
			"endpoint": map[string]string{
				"host": "127.0.0.1",
				"port": "8082",
			},
		},
	}

	if err := c.publishMessage("edgeCore.nodes.register", regMessage); err != nil {
		zap.L().Error("Failed to publish node registration",
			zap.Error(err),
			zap.String("node_id", nodeID),
		)
	}

	// Publish online status
	topic := fmt.Sprintf("edgeCore.nodes.%s.status", nodeID)
	payload := map[string]interface{}{
		"status":    "online",
		"timestamp": time.Now().UnixMilli(),
	}
	data, _ := json.Marshal(payload)
	if err := c.nc.Publish(topic, data); err != nil {
		zap.L().Error("Failed to publish online status",
			zap.Error(err),
		)
	}
}

// subscribeToCommands subscribes to edgeOS command subjects
// Phase 4 (EX-P4-01/03): V1 command plane is DEPRECATED — subscriptions retained only
// for backward compatibility; all command traffic must migrate to EAN Capability Invoke.
// After the transition window passes, these subscriptions are removed (V1 command plane off).
func (c *Client) subscribeToCommands() {
	c.configMu.RLock()
	nodeID := c.config.NodeID
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-03): V1 命令面开关——false 时全面下线 V1 命令 Subject 订阅。
	// | V1 command plane switch: when false, decommission V1 command subject subscriptions.
	if !v1CmdEnabled {
		zap.L().Info("V1 command plane disabled (V1CommandEnabled=false); skipping V1 command subject subscriptions",
			zap.String("node_id", nodeID))
		return
	}

	c.subMu.Lock()
	defer c.subMu.Unlock()

	// V1 命令面（edgeCore.cmd.*）Phase 4 标记 deprecated，仅保留兼容订阅，不再维护。
	// | V1 command plane (edgeCore.cmd.*) marked DEPRECATED in Phase 4; retained only for compat.
	zap.L().Warn("DEPRECATED: subscribing V1 command subjects (edgeCore.cmd.*); migrate to EAN Capability Invoke",
		zap.String("node_id", nodeID))

	// Subscribe to device discovery command
	discoverSubject := fmt.Sprintf("edgeCore.cmd.%s.discover", nodeID)
	if sub, err := c.nc.Subscribe(discoverSubject, c.handleDiscoverCommand); err != nil {
		zap.L().Error("Failed to subscribe to discover subject",
			zap.Error(err),
			zap.String("subject", discoverSubject),
		)
	} else {
		c.subscriptions[discoverSubject] = sub
		zap.L().Warn("DEPRECATED: subscribed to V1 discover subject (retained for compat)",
			zap.String("subject", discoverSubject))
	}

	// Subscribe to write commands for all devices
	writeSubject := fmt.Sprintf("edgeCore.cmd.%s.*.write", nodeID)
	if sub, err := c.nc.Subscribe(writeSubject, c.handleWriteCommand); err != nil {
		zap.L().Error("Failed to subscribe to write subject",
			zap.Error(err),
			zap.String("subject", writeSubject),
		)
	} else {
		c.subscriptions[writeSubject] = sub
		zap.L().Warn("DEPRECATED: subscribed to V1 write subject (retained for compat)",
			zap.String("subject", writeSubject))
	}

	// Subscribe to task control commands
	taskSubject := fmt.Sprintf("edgeCore.cmd.%s.task.*.*", nodeID)
	if sub, err := c.nc.Subscribe(taskSubject, c.handleTaskCommand); err != nil {
		zap.L().Error("Failed to subscribe to task subject",
			zap.Error(err),
			zap.String("subject", taskSubject),
		)
	} else {
		c.subscriptions[taskSubject] = sub
		zap.L().Warn("DEPRECATED: subscribed to V1 task subject (retained for compat)",
			zap.String("subject", taskSubject))
	}

	// Subscribe to global node register command (triggered by EdgeOS for proactive re-registration)
	registerSubject := "edgeCore.cmd.nodes.register"
	if sub, err := c.nc.Subscribe(registerSubject, c.handleNodeRegisterCommand); err != nil {
		zap.L().Error("Failed to subscribe to register subject",
			zap.Error(err),
			zap.String("subject", registerSubject),
		)
	} else {
		c.subscriptions[registerSubject] = sub
		zap.L().Warn("DEPRECATED: subscribed to V1 node register subject (retained for compat)",
			zap.String("subject", registerSubject))
	}

	// Subscribe to node registration response (EdgeOS responds to our registration request)
	responseSubject := fmt.Sprintf("edgeCore.nodes.%s.response", nodeID)
	if sub, err := c.nc.Subscribe(responseSubject, c.handleRegisterResponseCommand); err != nil {
		zap.L().Error("Failed to subscribe to response subject",
			zap.Error(err),
			zap.String("subject", responseSubject),
		)
	} else {
		c.subscriptions[responseSubject] = sub
		zap.L().Warn("DEPRECATED: subscribed to V1 node register-response subject (retained for compat)",
			zap.String("subject", responseSubject))
	}
}

// handleDiscoverCommand handles device discovery commands
func (c *Client) handleDiscoverCommand(msg *nats.Msg) {
	zap.L().Warn("DEPRECATED: V1 discover command received (NATS), migrate to EAN *.scan_devices Capability Invoke",
		zap.String("subject", msg.Subject))
	var message Message
	if err := json.Unmarshal(msg.Data, &message); err != nil {
		zap.L().Error("Failed to unmarshal discover command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received discover command",
		zap.String("message_id", message.Header.MessageID),
	)

	// Send response
	response := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        c.nodeID,
			Destination:   message.Header.Source,
			MessageType:   "discover_response",
			Version:       "1.0",
			CorrelationID: message.Header.MessageID,
		},
		Body: map[string]interface{}{
			"success": true,
			"message": "Discovery triggered",
		},
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// handleWriteCommand handles write commands for devices
func (c *Client) handleWriteCommand(msg *nats.Msg) {
	zap.L().Warn("DEPRECATED: V1 write command received (NATS), migrate to EAN *.write_point Capability Invoke",
		zap.String("subject", msg.Subject))
	var message Message
	if err := json.Unmarshal(msg.Data, &message); err != nil {
		zap.L().Error("Failed to unmarshal write command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received write command",
		zap.String("message_id", message.Header.MessageID),
		zap.String("subject", msg.Subject),
	)

	// Extract device ID from subject
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 4 {
		zap.L().Error("Invalid write subject", zap.String("subject", msg.Subject))
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Invalid subject",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}
	deviceID := subjectParts[3]

	c.configMu.RLock()
	virtualDevices := c.config.VirtualDevices
	c.configMu.RUnlock()
	if model.IsNorthboundVirtualDevice(deviceID, virtualDevices) {
		zap.L().Warn("Write rejected for virtual device", zap.String("device", deviceID))
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Virtual device is read-only",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}

	// Parse write command body
	body, ok := message.Body.(map[string]interface{})
	if !ok {
		zap.L().Error("Invalid write command body")
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Invalid body",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}

	points, ok := body["points"].(map[string]interface{})
	if !ok {
		zap.L().Error("Invalid points in write command")
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Invalid points",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}

	// Execute writes through southbound manager
	if c.sb == nil {
		zap.L().Error("Southbound manager not initialized")
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Southbound not available",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}

	c.configMu.RLock()
	deviceConfig, deviceExists := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	c.configMu.RUnlock()
	if !deviceExists {
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Device not found in configuration",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}
	if !deviceConfig.Enable {
		response := Message{
			Header: MessageHeader{
				MessageID:     generateMessageID(),
				Timestamp:     time.Now().UnixMilli(),
				Source:        c.nodeID,
				Destination:   message.Header.Source,
				MessageType:   "write_response",
				Version:       "1.0",
				CorrelationID: message.Header.MessageID,
			},
			Body: map[string]interface{}{
				"success": false,
				"message": "Device is disabled",
			},
		}
		data, _ := json.Marshal(response)
		msg.Respond(data)
		return
	}

	var errors []string
	for pointID, value := range points {
		// Try to find channel ID - use first available channel for now
		channels := c.sb.GetChannels()
		if len(channels) == 0 {
			errors = append(errors, pointID+": No channels available")
			continue
		}
		channelID := channels[0].ID

		if err := c.sb.WritePoint(channelID, deviceID, pointID, value); err != nil {
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

	response := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        c.nodeID,
			Destination:   message.Header.Source,
			MessageType:   "write_response",
			Version:       "1.0",
			CorrelationID: message.Header.MessageID,
		},
		Body: map[string]interface{}{
			"success": success,
			"message": errorMsg,
		},
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// handleTaskCommand handles task control commands
func (c *Client) handleTaskCommand(msg *nats.Msg) {
	zap.L().Warn("DEPRECATED: V1 task command received (NATS), migrate to EAN Capability Invoke",
		zap.String("subject", msg.Subject))
	var message Message
	if err := json.Unmarshal(msg.Data, &message); err != nil {
		zap.L().Error("Failed to unmarshal task command",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received task command",
		zap.String("message_id", message.Header.MessageID),
		zap.String("type", message.Header.MessageType),
	)

	// For now, just acknowledge command
	response := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        c.nodeID,
			Destination:   message.Header.Source,
			MessageType:   "task_response",
			Version:       "1.0",
			CorrelationID: message.Header.MessageID,
		},
		Body: map[string]interface{}{
			"success": true,
			"message": "Task command received",
		},
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// handleNodeRegisterCommand handles proactive node re-registration command from EdgeOS
func (c *Client) handleNodeRegisterCommand(msg *nats.Msg) {
	var message Message
	if err := json.Unmarshal(msg.Data, &message); err != nil {
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
	response := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        c.nodeID,
			Destination:   message.Header.Source,
			MessageType:   "node_register_response",
			Version:       "1.0",
			CorrelationID: message.Header.MessageID,
		},
		Body: map[string]interface{}{
			"success": true,
			"message": "Node re-registered successfully",
		},
	}

	data, _ := json.Marshal(response)
	msg.Respond(data)
}

// handleRegisterResponseCommand handles registration response from EdgeOS
func (c *Client) handleRegisterResponseCommand(msg *nats.Msg) {
	zap.L().Info("NATS handleRegisterResponseCommand called",
		zap.String("subject", msg.Subject),
	)

	var message Message
	if err := json.Unmarshal(msg.Data, &message); err != nil {
		zap.L().Error("Failed to unmarshal register response",
			zap.Error(err),
		)
		return
	}

	zap.L().Info("Received register response",
		zap.String("subject", msg.Subject),
		zap.String("message_id", message.Header.MessageID),
		zap.String("message_type", message.Header.MessageType),
	)

	// Check if registration was successful
	body, ok := message.Body.(map[string]interface{})
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
		zap.L().Info("NATS device_report fallback: republishing inventory after connect",
			zap.String("node_id", c.nodeID),
		)
		c.publishDeviceReport()
	}()
}

// publishDeviceReport publishes all device information to EdgeOS
func (c *Client) publishDeviceReport() {
	zap.L().Info("NATS publishDeviceReport called")

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
	var devices []map[string]interface{}

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

			deviceInfo := map[string]interface{}{
				"device_id":       device.ID,
				"device_name":     device.Name,
				"device_profile":  channel.Protocol, // Use protocol as profile
				"service_name":    channel.Name,     // Use channel name as service
				"labels":          []string{},
				"description":     "",
				"admin_state":     "ENABLED",
				"operating_state": operatingState,
				"properties": map[string]interface{}{
					"protocol":   channel.Protocol,
					"channel_id": channel.ID,
				},
			}

			// Add config properties
			if device.Config != nil {
				for k, v := range device.Config {
					deviceInfo["properties"].(map[string]interface{})[k] = v
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
		Body: map[string]interface{}{
			"node_id": nodeID,
			"devices": devices,
		},
	}

	if err := c.publishMessage("edgeCore.devices.report", reportMessage); err != nil {
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

// PublishDeviceReport republishes the device inventory (edgeCore.devices.report) to EdgeOS.
// Used when channels/devices are added/updated or the northbound Devices mapping changes,
// so EdgeOS stays in sync without requiring a re-connect.
func (c *Client) PublishDeviceReport() {
	c.publishDeviceReport()
}

// Publish publishes a value to edgeOS
func (c *Client) Publish(v model.Value) {
	if c.nc == nil || !c.nc.IsConnected() {
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
		c.publishDeviceData(v.DeviceID, map[string]interface{}{v.PointID: v.Value}, v.Quality, v.TS)
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

// checkAndPushPeriodicDevices checks which devices need periodic push
func (c *Client) checkAndPushPeriodicDevices() {
	if c.nc == nil || !c.nc.IsConnected() {
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
	points := make(map[string]interface{})
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
func (c *Client) publishDeviceData(deviceID string, points map[string]interface{}, quality string, ts time.Time) {
	subject := fmt.Sprintf("edgeCore.data.%s.%s", c.nodeID, deviceID)
	dataMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      c.nodeID,
			MessageType: "data",
			Version:     "1.0",
		},
		Body: map[string]interface{}{
			"node_id":   c.nodeID,
			"device_id": deviceID,
			"timestamp": ts.UnixMilli(),
			"points":    points,
			"quality":   quality,
		},
	}

	if err := c.publishMessage(subject, dataMessage); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device data to edgeOS(NATS)",
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
	if c.nc == nil || !c.nc.IsConnected() {
		return
	}

	c.configMu.RLock()
	_, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	nodeID := c.config.NodeID
	c.configMu.RUnlock()

	if !ok {
		return
	}

	subject := fmt.Sprintf("edgeCore.devices.%s.%s.status", nodeID, deviceID)
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
		Body: map[string]interface{}{
			"node_id":   nodeID,
			"device_id": deviceID,
			"status":    statusStr,
			"timestamp": time.Now().UnixMilli(),
		},
	}

	if err := c.publishMessage(subject, statusMessage); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device status",
			zap.Error(err),
			zap.String("device", deviceID),
			zap.String("subject", subject),
		)
	} else {
		atomic.AddInt64(&c.successCount, 1)
		zap.L().Info("Published device status",
			zap.String("node_id", nodeID),
			zap.String("device_id", deviceID),
			zap.String("status", statusStr),
			zap.String("subject", subject),
		)
	}
}

// PublishHeartbeat publishes node heartbeat
func (c *Client) PublishHeartbeat(metrics map[string]interface{}) {
	if c.nc == nil || !c.nc.IsConnected() {
		return
	}

	c.configMu.RLock()
	nodeID := c.config.NodeID
	v1CmdEnabled := c.config.V1CommandEnabled
	c.configMu.RUnlock()

	// Phase 4 (EX-P4-02/03): V1 命令面开关——false 时不再发布 V1 心跳（EAN Heartbeat 已覆盖）。
	// | V1 command plane switch: when false, skip V1 heartbeat publish (EAN heartbeat covers it).
	if !v1CmdEnabled {
		return
	}

	// Phase 4 (EX-P4-02): V1 心跳上报标记 deprecated——EAN $edgeos/heartbeat/{agent} 已替代。
	// | V1 heartbeat publish marked DEPRECATED; superseded by EAN heartbeat.
	zap.L().Warn("DEPRECATED: V1 heartbeat publish (edgeCore.heartbeat.{node}) retained for compat; use EAN $edgeos/heartbeat/{agent}",
		zap.String("node_id", nodeID))

	heartbeatMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "heartbeat",
			Version:     "1.0",
		},
		Body: map[string]interface{}{
			"node_id":   nodeID,
			"status":    "active",
			"timestamp": time.Now().UnixMilli(),
			"metrics":   metrics,
		},
	}

	subject := fmt.Sprintf("edgeCore.heartbeat.%s", nodeID)
	if err := c.publishMessage(subject, heartbeatMessage); err != nil {
		zap.L().Error("Failed to publish heartbeat",
			zap.Error(err),
		)
	}
}

// publishMessage publishes a message with proper error handling
func (c *Client) publishMessage(subject string, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := c.nc.Publish(subject, data); err != nil {
		return err
	}

	zap.L().Debug("Published to edgeOS(NATS)",
		zap.String("subject", subject),
		zap.String("message_type", msg.Header.MessageType),
		zap.Int("bytes", len(data)),
	)

	return nil
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

// setStatus sets connection status
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

	c.subMu.Lock()
	for subject, sub := range c.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			zap.L().Error("Failed to unsubscribe",
				zap.Error(err),
				zap.String("subject", subject),
			)
		}
		delete(c.subscriptions, subject)
	}
	c.subMu.Unlock()

	if c.nc != nil && c.nc.IsConnected() {
		c.configMu.RLock()
		nodeID := c.config.NodeID
		c.configMu.RUnlock()

		// Publish offline status (V1.0 compat)
		subject := fmt.Sprintf("edgeCore.nodes.%s.status", nodeID)
		payload := map[string]interface{}{
			"status":    "offline",
			"timestamp": time.Now().UnixMilli(),
		}
		data, _ := json.Marshal(payload)
		c.nc.Publish(subject, data)

		c.nc.Close()
	}

	c.setStatus(StatusDisconnected)
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "msg-" + hex.EncodeToString(b)
}

// PublishRaw publishes raw data to a specific subject
func (c *Client) PublishRaw(subject string, payload []byte) error {
	if c.nc == nil || !c.nc.IsConnected() {
		return fmt.Errorf("client not connected")
	}

	if err := c.nc.Publish(subject, payload); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		return err
	}

	atomic.AddInt64(&c.successCount, 1)
	atomic.AddInt64(&c.publishCount, 1)
	return nil
}

// PublishPointsMetadata publishes point definitions metadata to EdgeOS for each device separately
func (c *Client) PublishPointsMetadata() error {
	zap.L().Info("NATS PublishPointsMetadata called")

	if c.nc == nil || !c.nc.IsConnected() {
		zap.L().Error("NATS client not connected, cannot publish points metadata")
		return fmt.Errorf("client not connected")
	}

	zap.L().Info("NATS client is connected, proceeding with points metadata")

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
			var devicePoints []map[string]interface{}
			for _, point := range points {
				pointInfo := map[string]interface{}{
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
				Body: map[string]interface{}{
					"channel_id":  channel.ID,
					"device_id":   device.ID,
					"node_id":     nodeID,
					"device_name": device.Name,
					"points":      devicePoints,
				},
			}

			if err := c.publishMessage("edgeCore.points.report", metadataMessage); err != nil {
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
	if c.nc == nil || !c.nc.IsConnected() {
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
	pointValues := make(map[string]interface{})
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
		Body: map[string]interface{}{
			"node_id":   nodeID,
			"device_id": deviceID,
			"timestamp": latestTS.UnixMilli(),
			"points":    pointValues,
			"quality":   quality,
		},
	}

	subject := fmt.Sprintf("edgeCore.points.%s.%s", nodeID, deviceID)
	if err := c.publishMessage(subject, syncMessage); err != nil {
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
func (c *Client) PublishDeviceOnline(deviceID, deviceName string, details map[string]interface{}) error {
	if c.nc == nil || !c.nc.IsConnected() {
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
		Body: map[string]interface{}{
			"node_id":     nodeID,
			"device_id":   deviceID,
			"device_name": deviceName,
			"online_time": time.Now().UnixMilli(),
			"status":      "online",
			"details":     details,
		},
	}

	subject := fmt.Sprintf("edgeCore.devices.%s.%s.online", nodeID, deviceID)
	if err := c.publishMessage(subject, onlineMessage); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device online notification",
			zap.Error(err),
			zap.String("device_id", deviceID),
			zap.String("subject", subject),
		)
		return err
	}

	atomic.AddInt64(&c.successCount, 1)
	atomic.AddInt64(&c.publishCount, 1)
	zap.L().Info("Published device online notification",
		zap.String("node_id", nodeID),
		zap.String("device_id", deviceID),
		zap.String("device_name", deviceName),
		zap.String("subject", subject),
	)
	return nil
}

// PublishDeviceOffline publishes sub-device offline notification to EdgeOS
func (c *Client) PublishDeviceOffline(deviceID, deviceName, reason string, details map[string]interface{}) error {
	if c.nc == nil || !c.nc.IsConnected() {
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
		Body: map[string]interface{}{
			"node_id":      nodeID,
			"device_id":    deviceID,
			"device_name":  deviceName,
			"offline_time": time.Now().UnixMilli(),
			"status":       "offline",
			"reason":       reason,
			"details":      details,
		},
	}

	subject := fmt.Sprintf("edgeCore.devices.%s.%s.offline", nodeID, deviceID)
	if err := c.publishMessage(subject, offlineMessage); err != nil {
		atomic.AddInt64(&c.failCount, 1)
		zap.L().Error("Failed to publish device offline notification",
			zap.Error(err),
			zap.String("device_id", deviceID),
			zap.String("subject", subject),
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
		zap.String("subject", subject),
	)
	return nil
}
