package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/reconnect"
	"github.com/anviod/edgeCore/internal/storage"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

const (
	StatusDisconnected = 0
	StatusConnected    = 1
	StatusReconnecting = 2
	StatusError        = 3
)

type MQTTStats struct {
	SuccessCount    int64 `json:"success_count"`
	FailCount       int64 `json:"fail_count"`
	ReconnectCount  int64 `json:"reconnect_count"`
	LastOfflineTime int64 `json:"last_offline_time"`
	LastOnlineTime  int64 `json:"last_online_time"`
}

type Client struct {
	config     model.MQTTConfig
	configMu   sync.RWMutex
	client     mqtt.Client
	lastValues sync.Map

	status   int
	statusMu sync.RWMutex
	stopChan chan struct{}

	bufferMu sync.Mutex
	buffers  map[string]*bufferItem

	periodicMu sync.Mutex
	periodic   map[string]*periodicItem

	// Southbound Manager for writing
	sb model.SouthboundManager

	// Storage for offline caching
	storage *storage.Storage

	// Stats counters (using atomic int64)
	successCount    int64
	failCount       int64
	reconnectCount  int64
	lastOfflineTime int64
	lastOnlineTime  int64

	reconnectSched reconnect.Scheduler
}

type AggregatedPayload struct {
	Timestamp int64          `json:"timestamp"`
	Node      string         `json:"node"`
	Group     string         `json:"group"`
	Values    map[string]any `json:"values"`
	Errors    map[string]any `json:"errors"`
	Metas     map[string]any `json:"metas"`
}

type bufferItem struct {
	payload *AggregatedPayload
	timer   *time.Timer
}

type periodicItem struct {
	channelID string
	values    map[string]model.Value
	ticker    *time.Ticker
	stop      chan struct{}
}

func NewClient(cfg model.MQTTConfig, sb model.SouthboundManager, s *storage.Storage) *Client {
	c := &Client{
		config:   cfg,
		sb:       sb,
		storage:  s,
		stopChan: make(chan struct{}),
		buffers:  make(map[string]*bufferItem),
		periodic: make(map[string]*periodicItem),
	}
	return c
}

func (c *Client) GetStatus() int {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

func (c *Client) GetStats() MQTTStats {
	return MQTTStats{
		SuccessCount:    atomic.LoadInt64(&c.successCount),
		FailCount:       atomic.LoadInt64(&c.failCount),
		ReconnectCount:  atomic.LoadInt64(&c.reconnectCount),
		LastOfflineTime: atomic.LoadInt64(&c.lastOfflineTime),
		LastOnlineTime:  atomic.LoadInt64(&c.lastOnlineTime),
	}
}

func (c *Client) setStatus(s int) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.status = s
}

func (c *Client) UpdateConfig(cfg model.MQTTConfig) error {
	c.configMu.RLock()
	needRestart := c.config.Broker != cfg.Broker ||
		c.config.ClientID != cfg.ClientID ||
		c.config.Username != cfg.Username ||
		c.config.Password != cfg.Password
	c.configMu.RUnlock()

	c.configMu.Lock()
	c.config = cfg
	c.configMu.Unlock()

	if needRestart {
		c.Stop()
		// Re-init stop chan
		c.stopChan = make(chan struct{})
		// Only start if enabled
		c.configMu.RLock()
		enable := c.config.Enable
		c.configMu.RUnlock()
		if enable {
			return c.Start()
		}
	}

	// Update periodic tasks if devices config changed (but no full restart)
	c.updatePeriodicTasks()

	return nil
}

func (c *Client) Start() error {
	// Check if client is enabled
	c.configMu.RLock()
	enable := c.config.Enable
	c.configMu.RUnlock()

	if !enable {
		return nil
	}

	// Start connection in background to support custom retry logic
	go c.connectLoop()
	go c.retryLoop()
	c.updatePeriodicTasks()
	return nil
}

func (c *Client) updatePeriodicTasks() {
	c.periodicMu.Lock()
	defer c.periodicMu.Unlock()

	c.configMu.RLock()
	devices := model.OpcUaDeviceMap(c.config.Devices)
	virtualDevices := c.config.VirtualDevices
	c.configMu.RUnlock()

	isPeriodicEnabled := func(devID string) (model.DevicePublishConfig, bool) {
		cfg, ok := model.LookupNorthboundPublishConfig(devID, devices, virtualDevices)
		if !ok || cfg.Strategy != "periodic" || time.Duration(cfg.Interval) <= 0 {
			return cfg, false
		}
		return cfg, true
	}

	// Stop removed or changed tasks
	for devID, item := range c.periodic {
		if _, ok := isPeriodicEnabled(devID); !ok {
			close(item.stop)
			item.ticker.Stop()
			delete(c.periodic, devID)
		}
	}

	// Start new tasks from real and virtual device maps
	startPeriodic := func(devID string, devCfg model.DevicePublishConfig) {
		if devCfg.Strategy != "periodic" || time.Duration(devCfg.Interval) <= 0 || !devCfg.Enable {
			return
		}
		if _, exists := c.periodic[devID]; exists {
			return
		}
		item := &periodicItem{
			values: make(map[string]model.Value),
			ticker: time.NewTicker(time.Duration(devCfg.Interval)),
			stop:   make(chan struct{}),
		}
		c.periodic[devID] = item
		go c.runPeriodicTask(devID, item)
	}

	for devID, devCfg := range devices {
		startPeriodic(devID, devCfg)
	}
	for devID, devCfg := range virtualDevices {
		startPeriodic(devID, devCfg)
	}
}

func (c *Client) runPeriodicTask(deviceID string, item *periodicItem) {
	for {
		select {
		case <-item.stop:
			return
		case <-c.stopChan:
			return
		case <-item.ticker.C:
			c.flushPeriodic(deviceID, item)
		}
	}
}

func (c *Client) flushPeriodic(deviceID string, item *periodicItem) {
	c.periodicMu.Lock()
	if len(item.values) == 0 {
		c.periodicMu.Unlock()
		return
	}

	// Construct payload
	payload := &AggregatedPayload{
		Timestamp: time.Now().UnixMilli(),
		Node:      deviceID,
		Group:     item.channelID,
		Values:    make(map[string]any),
		Errors:    make(map[string]any),
		Metas:     make(map[string]any),
	}

	for _, v := range item.values {
		payload.Values[v.PointID] = v.Value
		if v.Quality != "Good" {
			payload.Errors[v.PointID] = v.Quality
		}
	}
	c.periodicMu.Unlock()

	c.configMu.RLock()
	ignoreOffline := c.config.IgnoreOfflineData
	c.configMu.RUnlock()

	if ignoreOffline {
		if len(payload.Values) > 0 && len(payload.Errors) == len(payload.Values) {
			return
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		zap.L().Error("Failed to marshal periodic value for MQTT",
			zap.Error(err),
			zap.String("component", "mqtt-client"),
		)
		return
	}

	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	topic := c.config.Topic
	c.configMu.RUnlock()

	topic = c.replaceGlobalVars(topic)

	token := c.client.Publish(topic, 0, false, data)
	go func() {
		if token.Wait() && token.Error() != nil {
			atomic.AddInt64(&c.failCount, 1)
			zap.L().Error("Failed to publish to MQTT",
				zap.Error(token.Error()),
				zap.String("component", "mqtt-client"),
			)
		} else {
			atomic.AddInt64(&c.successCount, 1)
			zap.L().Debug("Published to MQTT",
				zap.String("topic", topic),
				zap.Int("bytes", len(data)),
				zap.String("payload", string(data)),
				zap.String("component", "mqtt-client"),
			)
		}
	}()
}

// PublishRaw publishes raw data to a specific topic
func (c *Client) PublishRaw(topic string, payload []byte) error {
	// Check if client is enabled
	c.configMu.RLock()
	enable := c.config.Enable
	c.configMu.RUnlock()

	if !enable {
		return mqtt.ErrNotConnected
	}

	connected := c.client != nil && c.client.IsConnected()

	if !connected {
		// Cache logic
		c.configMu.RLock()
		cacheCfg := c.config.Cache
		configID := c.config.ID
		c.configMu.RUnlock()

		if cacheCfg.Enable && c.storage != nil {
			if err := c.storage.SaveOfflineMessage(configID, payload, cacheCfg.MaxCount); err != nil {
				return err
			}
			return nil // Queued successfully
		}
		return mqtt.ErrNotConnected
	}

	token := c.client.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		// Cache logic on failure
		c.configMu.RLock()
		cacheCfg := c.config.Cache
		configID := c.config.ID
		c.configMu.RUnlock()

		if cacheCfg.Enable && c.storage != nil {
			c.storage.SaveOfflineMessage(configID, payload, cacheCfg.MaxCount)
			return nil // Queued
		}
		return token.Error()
	}
	return nil
}

func (c *Client) connectLoop() {
	// Check if client is enabled
	c.configMu.RLock()
	enable := c.config.Enable
	c.configMu.RUnlock()

	if !enable {
		c.setStatus(StatusDisconnected)
		return
	}

	c.setStatus(StatusReconnecting)

	c.configMu.RLock()
	broker := c.config.Broker
	clientID := c.config.ClientID
	username := c.config.Username
	password := c.config.Password
	statusTopic := c.config.StatusTopic
	lwtTopic := c.config.LwtTopic
	topic := c.config.Topic
	offlinePayload := c.config.OfflinePayload
	lwtPayload := c.config.LwtPayload
	c.configMu.RUnlock()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	// Disable auto reconnect to control it manually
	opts.SetAutoReconnect(false)

	// Determine LWT Topic
	finalLwtTopic := lwtTopic
	if finalLwtTopic == "" {
		finalLwtTopic = statusTopic
	}
	if finalLwtTopic == "" && topic != "" {
		finalLwtTopic = topic + "/status"
	}
	// Replace vars in LWT Topic
	finalLwtTopic = c.replaceGlobalVars(finalLwtTopic)

	if finalLwtTopic != "" {
		// Determine LWT Payload
		finalLwtPayload := lwtPayload
		if finalLwtPayload == "" {
			finalLwtPayload = offlinePayload // Fallback to OfflinePayload if LwtPayload is not set
		}
		if finalLwtPayload == "" {
			b, _ := json.Marshal(map[string]any{
				"status":    "offline",
				"timestamp": time.Now().UnixMilli(),
			})
			finalLwtPayload = string(b)
		}

		// Handle variables in Payload for LWT
		// Use replaceDeviceVars with clientID as deviceID for gateway-level LWT
		payload := c.replaceDeviceVars(finalLwtPayload, clientID)
		payload = strings.ReplaceAll(payload, "{status}", "lwt")

		opts.SetWill(finalLwtTopic, payload, 1, true)
	}

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		zap.L().Info("Connected to MQTT Broker",
			zap.String("broker", broker),
			zap.String("component", "mqtt-client"),
		)
		c.setStatus(StatusConnected)
		atomic.StoreInt64(&c.lastOnlineTime, time.Now().UnixMilli())

		// Publish Online Status
		// Re-evaluate statusTopic here in case config changed (though config update restarts client)
		c.configMu.RLock()
		statusTopic := c.config.StatusTopic
		topic := c.config.Topic
		onlinePayload := c.config.OnlinePayload
		subscribeTopic := c.config.SubscribeTopic
		c.configMu.RUnlock()

		if statusTopic == "" && topic != "" {
			statusTopic = topic + "/status"
		}

		// Replace vars in Status Topic
		statusTopic = c.replaceGlobalVars(statusTopic)

		if statusTopic != "" {
			if onlinePayload == "" {
				b, _ := json.Marshal(map[string]any{
					"status":    "online",
					"timestamp": time.Now().UnixMilli(),
				})
				onlinePayload = string(b)
			}
			// Handle variables in Payload for Online Status
			payload := c.replaceDeviceVars(onlinePayload, clientID)
			payload = strings.ReplaceAll(payload, "{status}", "online")

			client.Publish(statusTopic, 1, true, payload)
		}

		// Subscribe to write requests if configured
		if subscribeTopic != "" {
			subscribeTopic = c.replaceGlobalVars(subscribeTopic)
			token := client.Subscribe(subscribeTopic, 0, c.handleWriteRequest)
			if token.Wait() && token.Error() != nil {
				zap.L().Error("Failed to subscribe to write topic",
					zap.String("topic", subscribeTopic),
					zap.Error(token.Error()),
					zap.String("component", "mqtt-client"),
				)
			} else {
				zap.L().Info("Subscribed to write topic",
					zap.String("topic", subscribeTopic),
					zap.String("component", "mqtt-client"),
				)
			}
		}
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		zap.L().Warn("MQTT Connection Lost",
			zap.Error(err),
			zap.String("component", "mqtt-client"),
		)
		c.setStatus(StatusDisconnected)
		atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
		c.scheduleReconnect()
	})

	c.client = mqtt.NewClient(opts)

	// Initial connection attempt
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		zap.L().Error("Initial MQTT connection failed",
			zap.Error(token.Error()),
			zap.String("component", "mqtt-client"),
		)
		c.setStatus(StatusDisconnected)
		atomic.StoreInt64(&c.lastOfflineTime, time.Now().UnixMilli())
		c.scheduleReconnect()
	} else {
		atomic.StoreInt64(&c.lastOnlineTime, time.Now().UnixMilli())
	}
}

// WriteRequest represents the payload for writing points via MQTT
type WriteRequest struct {
	UUID   string         `json:"uuid"`
	Group  string         `json:"group"` // Channel ID
	Node   string         `json:"node"`  // Device ID
	Values map[string]any `json:"values"`
}

// WriteResponse represents the response payload
type WriteResponse struct {
	UUID    string `json:"uuid"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func (c *Client) handleWriteRequest(client mqtt.Client, msg mqtt.Message) {
	zap.L().Info("Received write request",
		zap.String("topic", msg.Topic()),
		zap.String("component", "mqtt-client"),
	)

	var req WriteRequest
	if err := json.Unmarshal(msg.Payload(), &req); err != nil {
		zap.L().Error("Failed to unmarshal write request",
			zap.Error(err),
			zap.String("component", "mqtt-client"),
		)
		return
	}

	if req.Group == "" || req.Node == "" || len(req.Values) == 0 {
		zap.L().Warn("Invalid write request: missing group, node or values",
			zap.String("component", "mqtt-client"),
		)
		return
	}

	if c.sb == nil {
		zap.L().Error("Southbound manager not initialized",
			zap.String("component", "mqtt-client"),
		)
		return
	}

	var errs []string
	success := true

	for pointID, val := range req.Values {
		if err := c.sb.WritePoint(req.Group, req.Node, pointID, val); err != nil {
			zap.L().Error("Failed to write point",
				zap.String("device", req.Node),
				zap.String("point", pointID),
				zap.Error(err),
				zap.String("component", "mqtt-client"),
			)
			errs = append(errs, pointID+": "+err.Error())
			success = false
		} else {
			zap.L().Info("Write point success",
				zap.String("device", req.Node),
				zap.String("point", pointID),
				zap.Any("value", val),
				zap.String("component", "mqtt-client"),
			)
		}
	}

	// Send response if UUID is present
	if req.UUID != "" {
		c.configMu.RLock()
		respTopic := c.config.WriteResponseTopic
		c.configMu.RUnlock()
		if respTopic == "" {
			// Derive response topic from request topic: replace trailing "req" with "resp" when present
			t := msg.Topic()
			parts := strings.Split(t, "/")
			if len(parts) > 0 && parts[len(parts)-1] == "req" {
				parts[len(parts)-1] = "resp"
				respTopic = strings.Join(parts, "/")
			} else {
				respTopic = t + "/resp"
			}
		} else {
			respTopic = c.replaceGlobalVars(respTopic)
		}
		resp := WriteResponse{
			UUID:    req.UUID,
			Success: success,
		}
		if len(errs) > 0 {
			// Join errors
			msg := ""
			for i, e := range errs {
				if i > 0 {
					msg += "; "
				}
				msg += e
			}
			resp.Message = msg
		}

		data, _ := json.Marshal(resp)
		if err := c.PublishRaw(respTopic, data); err != nil {
			zap.L().Error("Failed to publish write response",
				zap.String("topic", respTopic),
				zap.Error(err),
				zap.String("component", "mqtt-client"),
			)
		} else {
			zap.L().Debug("Published write response",
				zap.String("topic", respTopic),
				zap.String("payload", string(data)),
				zap.String("component", "mqtt-client"),
			)
		}
	}
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

		if c.client != nil && c.client.IsConnected() {
			return
		}

		c.setStatus(StatusReconnecting)
		attempt := retryCount + 1
		broker := func() string {
			c.configMu.RLock()
			defer c.configMu.RUnlock()
			return c.config.Broker
		}()
		if logThrottle.ShouldLog(attempt, 30*time.Second, 10) {
			zap.L().Info("MQTT reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("broker", broker),
				zap.String("component", "mqtt-client"),
			)
		} else {
			zap.L().Debug("MQTT reconnect attempt",
				zap.Int("attempt", attempt),
				zap.String("broker", broker),
				zap.String("component", "mqtt-client"),
			)
		}

		token := c.client.Connect()
		if token.Wait() && token.Error() == nil {
			atomic.AddInt64(&c.reconnectCount, 1)
			zap.L().Info("MQTT reconnected",
				zap.String("broker", broker),
				zap.String("component", "mqtt-client"),
			)
			return
		}

		retryCount++
		delay := reconnect.Backoff(retryCount)

		if retryCount <= 10 {
			if logThrottle.ShouldLog(retryCount, 30*time.Second, 10) {
				zap.L().Warn("MQTT reconnect failed, retrying shortly",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
					zap.String("component", "mqtt-client"),
				)
			} else {
				zap.L().Debug("MQTT reconnect failed, retrying",
					zap.Int("attempt", retryCount),
					zap.Duration("next_retry_in", delay),
					zap.String("component", "mqtt-client"),
				)
			}
		} else {
			c.setStatus(StatusError)
			if logThrottle.ShouldLog(retryCount, 60*time.Second, 10) {
				zap.L().Error("MQTT reconnect failed repeatedly, backing off",
					zap.Int("attempts", retryCount),
					zap.Duration("backoff", delay),
					zap.String("component", "mqtt-client"),
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

func (c *Client) Publish(v model.Value) {
	// Check if client is enabled
	c.configMu.RLock()
	enable := c.config.Enable
	c.configMu.RUnlock()

	if !enable || c.client == nil || !c.client.IsConnected() {
		return
	}

	// Filter based on device config if configured
	c.configMu.RLock()
	devCfg, ok := model.LookupNorthboundPublishConfig(v.DeviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	c.configMu.RUnlock()

	if !ok {
		return
	}

	// Strategy: COV (Change of Value)
	if devCfg.Strategy == "cov" || devCfg.Strategy == "change" {
		key := v.DeviceID + ":" + v.PointID
		lastVal, loaded := c.lastValues.Load(key)
		if loaded && lastVal == v.Value {
			return
		}
		c.lastValues.Store(key, v.Value)
	} else if devCfg.Strategy == "periodic" && time.Duration(devCfg.Interval) > 0 {
		// Periodic with Interval: Cache value only
		c.periodicMu.Lock()
		if item, ok := c.periodic[v.DeviceID]; ok {
			item.channelID = v.ChannelID
			item.values[v.PointID] = v
		}
		c.periodicMu.Unlock()
		return // Don't publish immediately
	}

	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	item, ok := c.buffers[v.DeviceID]
	if !ok {
		payload := &AggregatedPayload{
			Timestamp: v.TS.UnixMilli(),
			Node:      v.DeviceID,  // Node maps to DeviceID
			Group:     v.ChannelID, // Group maps to ChannelID
			Values:    make(map[string]any),
			Errors:    make(map[string]any),
			Metas:     make(map[string]any),
		}
		item = &bufferItem{
			payload: payload,
		}
		// Start timer to flush (100ms delay to aggregate points)
		item.timer = time.AfterFunc(100*time.Millisecond, func() {
			c.flushDevice(v.DeviceID)
		})
		c.buffers[v.DeviceID] = item
	}

	// Add value to buffer
	item.payload.Values[v.PointID] = v.Value
	if v.Quality != "Good" {
		item.payload.Errors[v.PointID] = v.Quality
	}
}

func (c *Client) flushDevice(deviceID string) {
	c.bufferMu.Lock()
	item, ok := c.buffers[deviceID]
	if !ok {
		c.bufferMu.Unlock()
		return
	}
	delete(c.buffers, deviceID)
	c.bufferMu.Unlock()

	c.configMu.RLock()
	ignoreOffline := c.config.IgnoreOfflineData
	c.configMu.RUnlock()

	if ignoreOffline {
		if len(item.payload.Values) > 0 && len(item.payload.Errors) == len(item.payload.Values) {
			return
		}
	}

	data, err := json.Marshal(item.payload)
	if err != nil {
		zap.L().Error("Failed to marshal value for MQTT",
			zap.Error(err),
			zap.String("component", "mqtt-client"),
		)
		return
	}

	if c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	topic := c.config.Topic
	c.configMu.RUnlock()

	topic = c.replaceGlobalVars(topic)

	token := c.client.Publish(topic, 0, false, data)
	go func() {
		if token.Wait() && token.Error() != nil {
			atomic.AddInt64(&c.failCount, 1)
			zap.L().Error("Failed to publish to MQTT",
				zap.Error(token.Error()),
				zap.String("component", "mqtt-client"),
			)
		} else {
			atomic.AddInt64(&c.successCount, 1)
			zap.L().Debug("Published to MQTT",
				zap.String("topic", topic),
				zap.Int("bytes", len(data)),
				zap.String("payload", string(data)),
				zap.String("component", "mqtt-client"),
			)
		}
	}()
}

// PublishDeviceStatus publishes device online/offline status
func (c *Client) PublishDeviceStatus(deviceID string, status int) {
	// Check if client is enabled
	c.configMu.RLock()
	enable := c.config.Enable
	c.configMu.RUnlock()

	if !enable || c.client == nil || !c.client.IsConnected() {
		return
	}

	// Check if device is enabled in this channel
	c.configMu.RLock()
	_, ok := model.LookupNorthboundPublishConfig(deviceID, model.OpcUaDeviceMap(c.config.Devices), c.config.VirtualDevices)
	statusTopic := c.config.StatusTopic
	topic := c.config.Topic
	onlinePayload := c.config.OnlinePayload
	offlinePayload := c.config.OfflinePayload
	c.configMu.RUnlock()

	if !ok {
		return
	}

	// Determine status string
	statusStr := "offline"
	if status == 0 { // NodeStateOnline
		statusStr = "online"
	}

	// Determine Topic
	if statusTopic == "" {
		if topic == "" {
			return
		}
		statusTopic = topic + "/status"
	}

	// Determine Payload Template
	var payloadTmpl string
	if statusStr == "online" {
		payloadTmpl = onlinePayload
	} else {
		payloadTmpl = offlinePayload
	}

	if payloadTmpl == "" {
		// Default
		b, _ := json.Marshal(map[string]any{
			"status":    statusStr,
			"timestamp": time.Now().UnixMilli(),
			"device_id": deviceID,
		})
		payloadTmpl = string(b)
	}

	// Handle variables in Topic
	topic = c.replaceDeviceVars(statusTopic, deviceID)

	// Handle variables in Payload
	payload := c.replaceDeviceVars(payloadTmpl, deviceID)
	payload = strings.ReplaceAll(payload, "{status}", statusStr)

	token := c.client.Publish(topic, 0, false, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			atomic.AddInt64(&c.failCount, 1)
			zap.L().Error("Failed to publish device status",
				zap.String("device", deviceID),
				zap.String("status", statusStr),
				zap.Error(token.Error()),
				zap.String("component", "mqtt-client"),
			)
		} else {
			atomic.AddInt64(&c.successCount, 1)
			zap.L().Debug("Published device status",
				zap.String("device", deviceID),
				zap.String("status", statusStr),
				zap.String("topic", topic),
				zap.String("payload", payload),
				zap.String("component", "mqtt-client"),
			)
		}
	}()
}

func (c *Client) Stop() {
	close(c.stopChan)
	if c.client != nil && c.client.IsConnected() {
		// Publish Offline Status (Graceful)
		c.configMu.RLock()
		statusTopic := c.config.StatusTopic
		topic := c.config.Topic
		offlinePayload := c.config.OfflinePayload
		clientID := c.config.ClientID
		c.configMu.RUnlock()

		if statusTopic == "" && topic != "" {
			statusTopic = topic + "/status"
		}

		if statusTopic != "" {
			if offlinePayload == "" {
				b, _ := json.Marshal(map[string]any{
					"status":    "offline",
					"timestamp": time.Now().UnixMilli(),
				})
				offlinePayload = string(b)
			}
			// Handle variables in Payload
			payload := c.replaceDeviceVars(offlinePayload, clientID)
			payload = strings.ReplaceAll(payload, "{status}", "offline")

			token := c.client.Publish(statusTopic, 1, true, payload)
			token.WaitTimeout(2 * time.Second)
		}

		c.client.Disconnect(250)
	}
	c.setStatus(StatusDisconnected)
}

func (c *Client) replaceGlobalVars(text string) string {
	c.configMu.RLock()
	clientID := c.config.ClientID
	c.configMu.RUnlock()

	text = strings.ReplaceAll(text, "{client_id}", clientID)
	// Add other global variables if needed
	return text
}

func (c *Client) replaceDeviceVars(text, deviceID string) string {
	// First replace global vars
	text = c.replaceGlobalVars(text)

	// Replace device specific vars
	text = strings.ReplaceAll(text, "{device_id}", deviceID)
	text = strings.ReplaceAll(text, "{device_name}", deviceID) // Fallback
	text = strings.ReplaceAll(text, "%device_id%", deviceID)

	// Timestamp
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	text = strings.ReplaceAll(text, "{timestamp}", ts)
	text = strings.ReplaceAll(text, "%timestamp%", ts)

	return text
}
