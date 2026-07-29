package edgos_mqtt

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// mqttBus adapts the edgeOS MQTT client to capability.Bus for EAN 2.0 topics.
// V1.0 edgex/* topics remain handled by the existing Client methods unchanged.
type mqttBus struct {
	client *Client
}

func (b mqttBus) IsConnected() bool {
	return b.client != nil && b.client.client != nil && b.client.client.IsConnected()
}

func (b mqttBus) Publish(topic string, payload []byte, qos byte) error {
	if !b.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	// EAN discovery topics 使用 retained，确保后启动的订阅者能收到历史消息
	retained := strings.HasPrefix(topic, "$edgeos/discovery/")
	token := b.client.client.Publish(topic, qos, retained, payload)
	if token.Wait() && token.Error() != nil {
		atomic.AddInt64(&b.client.failCount, 1)
		return token.Error()
	}
	atomic.AddInt64(&b.client.successCount, 1)
	atomic.AddInt64(&b.client.publishCount, 1)
	return nil
}

func (b mqttBus) Subscribe(topic string, qos byte, handler func(topic string, payload []byte)) error {
	if !b.IsConnected() {
		return nil
	}
	token := b.client.client.Subscribe(topic, qos, func(_ mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}
	b.client.cmdMu.Lock()
	b.client.cmdSubscriptions[topic] = token
	b.client.cmdMu.Unlock()
	return nil
}

// SetOnEANRuntimeChanged registers a callback invoked when the EAN Capability
// Runtime is started or stopped (connect / disconnect / EAN enable toggles).
// Used by NorthboundManager to refresh shadow event publishers off the hot path.
func (c *Client) SetOnEANRuntimeChanged(fn func()) {
	c.eanMu.Lock()
	c.onEANRuntimeChanged = fn
	c.eanMu.Unlock()
}

func (c *Client) notifyEANRuntimeChanged() {
	c.eanMu.RLock()
	fn := c.onEANRuntimeChanged
	c.eanMu.RUnlock()
	if fn == nil {
		return
	}
	// Synchronous so Shadow→Event publishers update before the next delta.
	// refreshEANEventPublishers is deadlock-safe when the caller holds nm.mu
	// (TryRLock + deferred blocking refresh).
	fn()
}

// AttachCapabilityRuntime binds an EAN 2.0 Capability Runtime to this MQTT client.
// V1.0 edgex/* topics continue to work independently as the compatibility layer.
func (c *Client) AttachCapabilityRuntime(rt *capability.Runtime) {
	c.eanMu.Lock()
	defer c.eanMu.Unlock()
	c.eanRuntime = rt
	if rt != nil {
		rt.SetBus(mqttBus{client: c})
	}
}

// EnsureCapabilityRuntime creates a default EdgeX Capability Runtime if none is attached.
// Returns (nil, nil) when EANEnabled is false — caller must check before calling startEANLocked.
func (c *Client) EnsureCapabilityRuntime(agentVersion string) (*capability.Runtime, error) {
	c.eanMu.Lock()
	defer c.eanMu.Unlock()
	if c.eanRuntime != nil {
		return c.eanRuntime, nil
	}
	c.configMu.RLock()
	nodeID := c.config.NodeID
	eanEnabled := c.config.EANEnabled
	heartbeatSec := c.config.EANHeartbeatSec
	c.configMu.RUnlock()

	// EAN 能力层未启用，不创建 Runtime | EAN capability layer disabled
	if !eanEnabled {
		return nil, nil
	}
	if nodeID == "" {
		nodeID = c.nodeID
	}
	if heartbeatSec <= 0 {
		heartbeatSec = 60 // 默认 60s | default 60s
	}
	rt, err := capability.NewRuntime(capability.RuntimeConfig{
		AgentID:              nodeID,
		AgentVersion:         agentVersion,
		Transport:            capability.TransportMQTT,
		HeartbeatIntervalSec: heartbeatSec,
		Metadata: map[string]any{
			"northbound": "edgeos_mqtt",
			"compat":     "v1_topics_retained",
		},
	}, mqttBus{client: c})
	if err != nil {
		return nil, err
	}
	rt.SetMapper(execution.NewCapabilityMapper(execution.NewWiredExecutor(c.sb)))
	c.eanRuntime = rt
	return rt, nil
}

// CapabilityRuntime returns the attached runtime, if any.
func (c *Client) CapabilityRuntime() *capability.Runtime {
	c.eanMu.RLock()
	defer c.eanMu.RUnlock()
	return c.eanRuntime
}

func (c *Client) startEANLocked(ctx context.Context) {
	c.eanMu.RLock()
	rt := c.eanRuntime
	c.eanMu.RUnlock()
	if rt == nil {
		return
	}
	rt.SetBus(mqttBus{client: c})
	if err := rt.Start(ctx); err != nil {
		zap.L().Warn("EAN Capability Runtime start failed", zap.Error(err))
	}
	// Re-subscribe / re-publish on every MQTT connect (including reconnect).
	rt.OnConnected(ctx)
	c.notifyEANRuntimeChanged()
}

func (c *Client) stopEAN() {
	c.eanMu.Lock()
	rt := c.eanRuntime
	c.eanMu.Unlock()
	if rt != nil {
		rt.Stop()
	}
}

// StopCapabilityRuntime stops and clears the EAN Runtime, cleaning up $edgeos/invoke/* subscriptions.
// Called when EANEnabled transitions from true→false or channel is disabled.
func (c *Client) StopCapabilityRuntime() {
	c.eanMu.Lock()
	rt := c.eanRuntime
	c.eanRuntime = nil
	c.eanMu.Unlock()
	if rt != nil {
		rt.Stop()
		zap.L().Info("EAN Capability Runtime stopped (EANEnabled=false or channel disabled)")
		c.notifyEANRuntimeChanged()
	}
}

// StartEAN starts the EAN Runtime if one exists. Called after EnsureCapabilityRuntime succeeds.
func (c *Client) StartEAN(ctx context.Context) {
	c.startEANLocked(ctx)
}
