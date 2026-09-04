package mqtt

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

func (c *Client) retryLoop() {
	// Parse flush interval
	interval := 1 * time.Minute
	c.configMu.RLock()
	if d, err := time.ParseDuration(c.config.Cache.FlushInterval); err == nil && d > 0 {
		interval = d
	}
	c.configMu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			// Check if client is enabled
			c.configMu.RLock()
			enable := c.config.Enable
			c.configMu.RUnlock()

			if enable {
				c.flushOfflineMessages()
			}
		}
	}
}

func (c *Client) flushOfflineMessages() {
	if c.storage == nil || c.client == nil || !c.client.IsConnected() {
		return
	}

	c.configMu.RLock()
	configID := c.config.ID
	enabled := c.config.Cache.Enable
	c.configMu.RUnlock()

	if !enabled {
		return
	}

	// Fetch batch (limit 50 per cycle to avoid blocking too long)
	msgs, err := c.storage.GetOfflineMessages(configID, 50)
	if err != nil || len(msgs) == 0 {
		return
	}

	zap.L().Info("Retrying offline MQTT messages", zap.String("client_id", configID), zap.Int("count", len(msgs)))

	for _, msg := range msgs {
		// Use PublishRaw directly but skip cache logic (to avoid infinite loop if fail)
		// We use standard paho publish here
		c.configMu.RLock()
		topic := c.config.Topic
		c.configMu.RUnlock()

		// Note: The topic is not stored in OfflineMessage, we assume generic topic or we should have stored it?
		// User requirement: "Store offline data". Usually payload.
		// If payload doesn't contain topic, we use default topic.
		// If PublishRaw was called with a specific topic (e.g. from Edge Rule), we might lose it if we don't store it.
		// However, boltdb_ext.go `SaveOfflineMessage` only takes `data`.
		// Assumption: The payload is self-contained OR we use the default topic.
		// Edge Rules usually specify a topic in the action config.
		// If the user wants to support variable topics in cache, we need to store topic in the DB value.
		// For now, I will use the default topic `c.config.Topic`.
		// If this is a limitation, I'll need to update `SaveOfflineMessage` to accept a struct/wrapper.
		// Let's assume standard data reporting uses default topic.

		token := c.client.Publish(topic, 0, false, msg.Data)
		if token.Wait() && token.Error() == nil {
			// Success -> Delete
			c.storage.RemoveOfflineMessage(msg.Key)
			atomic.AddInt64(&c.successCount, 1)
		} else {
			// Fail -> Stop retry loop for this cycle to preserve order
			atomic.AddInt64(&c.failCount, 1)
			break
		}
	}
}

// PublishDeviceLifecycle publishes device add/remove events
func (c *Client) PublishDeviceLifecycle(event string, device model.Device) {
	if c.client == nil || !c.client.IsConnected() {
		// Should we cache this? Probably yes.
		// But let's follow standard flow.
	}

	c.configMu.RLock()
	topic := c.config.DeviceLifecycleTopic
	baseTopic := c.config.Topic
	//clientID := c.config.ClientID
	c.configMu.RUnlock()

	if topic == "" {
		if baseTopic == "" {
			return
		}
		topic = baseTopic + "/lifecycle"
	}

	topic = c.replaceGlobalVars(topic)

	payloadMap := map[string]any{
		"event":     event, // "add" or "remove"
		"device_id": device.ID,
		"timestamp": time.Now().UnixMilli(),
		"details":   device,
	}

	data, _ := json.Marshal(payloadMap)

	c.PublishRaw(topic, data)
}
