package edgos_nats

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/execution"

	nats "github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// MqttTopicToNatsSubject converts an MQTT-style slash-separated topic to a
// NATS dot-separated subject, translating MQTT wildcards in the process:
//
//	"/" → "."   (token separator)
//	"+" → "*"   (single-level wildcard)
//	"#" → ">"   (multi-level wildcard)
//
// EAN 2.0 topic constants use the MQTT slash form so a single set of templates
// works for both transports; the NATS bus adapter applies this conversion before
// every Publish / Subscribe call, restoring NATS-native dot subjects.
//
// | 将 MQTT 风格的斜杠 Topic 转换为 NATS 点分隔 Subject。
// | 遵循 NATS 消息队列约定：/ → .  + → *  # → >
func MqttTopicToNatsSubject(topic string) string {
	s := strings.ReplaceAll(topic, "/", ".")
	s = strings.ReplaceAll(s, "+", "*")
	s = strings.ReplaceAll(s, "#", ">")
	return s
}

// natsBus adapts the edgeOS NATS client to capability.Bus for EAN 2.0 subjects.
// V1.0 edgeCore.* subjects remain handled by existing Client methods unchanged.
// EAN topic constants use MQTT slash form; natsBus converts them to NATS dot
// subjects via MqttTopicToNatsSubject before every Publish / Subscribe call,
// complying with NATS subject conventions (dot-separated hierarchical tokens).
// | NATS 适配器：将 MQTT 斜杠 Topic 转换为 NATS 点分隔 Subject 后再发布/订阅。
type natsBus struct {
	client *Client
}

func (b natsBus) IsConnected() bool {
	return b.client != nil && b.client.nc != nil && b.client.nc.IsConnected()
}

func (b natsBus) Publish(topic string, payload []byte, _ byte) error {
	if !b.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	subject := MqttTopicToNatsSubject(topic)
	if err := b.client.nc.Publish(subject, payload); err != nil {
		atomic.AddInt64(&b.client.failCount, 1)
		return err
	}
	atomic.AddInt64(&b.client.successCount, 1)
	atomic.AddInt64(&b.client.publishCount, 1)
	return nil
}

func (b natsBus) Subscribe(topic string, _ byte, handler func(topic string, payload []byte)) error {
	if !b.IsConnected() {
		return nil
	}
	subject := MqttTopicToNatsSubject(topic)
	sub, err := b.client.nc.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	if err != nil {
		return err
	}
	b.client.subMu.Lock()
	b.client.subscriptions[topic] = sub
	b.client.subMu.Unlock()
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

// AttachCapabilityRuntime binds an EAN 2.0 Capability Runtime to this NATS client.
func (c *Client) AttachCapabilityRuntime(rt *capability.Runtime) {
	c.eanMu.Lock()
	defer c.eanMu.Unlock()
	c.eanRuntime = rt
	if rt != nil {
		rt.SetBus(natsBus{client: c})
	}
}

// EnsureCapabilityRuntime creates a default edgeCore Capability Runtime if none is attached.
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
		Transport:            capability.TransportNATS,
		HeartbeatIntervalSec: heartbeatSec,
		Metadata: map[string]any{
			"northbound":   "edgeos_nats",
			"subject_form": "nats_dot", // MQTT slash topics converted to NATS dot subjects
		},
	}, natsBus{client: c})
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

// EANEventAutoPublishEnabled reports whether the EAN capability layer should
// auto-publish point-change events over $edgeos.event.* (NATS dot subjects).
// Used by the Shadow→Event bridge to gate EAN event publishing independent of the runtime.
func (c *Client) EANEventAutoPublishEnabled() bool {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return c.config.EANEnabled && c.config.EANEventAutoPublish
}

func (c *Client) startEANLocked(ctx context.Context) {
	c.eanMu.RLock()
	rt := c.eanRuntime
	c.eanMu.RUnlock()
	if rt == nil {
		return
	}
	rt.SetBus(natsBus{client: c})
	if err := rt.Start(ctx); err != nil {
		zap.L().Warn("EAN Capability Runtime start failed (NATS)", zap.Error(err))
	}
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

// StopCapabilityRuntime stops and clears the EAN Runtime, cleaning up $edgeos.invoke.* subscriptions.
// Called when EANEnabled transitions from true→false or channel is disabled.
func (c *Client) StopCapabilityRuntime() {
	c.eanMu.Lock()
	rt := c.eanRuntime
	c.eanRuntime = nil
	c.eanMu.Unlock()
	if rt != nil {
		rt.Stop()
		zap.L().Info("EAN Capability Runtime stopped (NATS) (EANEnabled=false or channel disabled)")
		c.notifyEANRuntimeChanged()
	}
}

// StartEAN starts the EAN Runtime if one exists. Called after EnsureCapabilityRuntime succeeds.
func (c *Client) StartEAN(ctx context.Context) {
	c.startEANLocked(ctx)
}
