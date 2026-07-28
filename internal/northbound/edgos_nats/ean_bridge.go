package edgos_nats

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"

	nats "github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// natsBus adapts the edgeOS NATS client to capability.Bus for EAN 2.0 subjects.
// V1.0 edgex.* subjects remain handled by existing Client methods unchanged.
// EAN subjects keep the protocol `$edgeos/...` form (slash tokens) per EAN 2.0 spec.
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
	if err := b.client.nc.Publish(topic, payload); err != nil {
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
	sub, err := b.client.nc.Subscribe(topic, func(msg *nats.Msg) {
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

// AttachCapabilityRuntime binds an EAN 2.0 Capability Runtime to this NATS client.
func (c *Client) AttachCapabilityRuntime(rt *capability.Runtime) {
	c.eanMu.Lock()
	defer c.eanMu.Unlock()
	c.eanRuntime = rt
	if rt != nil {
		rt.SetBus(natsBus{client: c})
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
		Transport:            capability.TransportNATS,
		HeartbeatIntervalSec: heartbeatSec,
		Metadata: map[string]any{
			"northbound": "edgeos_nats",
			"compat":     "v1_subjects_retained",
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
		zap.L().Info("EAN Capability Runtime stopped (NATS) (EANEnabled=false or channel disabled)")
	}
}

// StartEAN starts the EAN Runtime if one exists. Called after EnsureCapabilityRuntime succeeds.
func (c *Client) StartEAN(ctx context.Context) {
	c.startEANLocked(ctx)
}
