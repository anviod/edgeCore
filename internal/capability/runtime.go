package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RuntimeConfig configures the EdgeX Capability Runtime.
type RuntimeConfig struct {
	AgentID              string
	AgentVersion         string
	Transport            TransportType
	HeartbeatIntervalSec int
	Protocols            []string // driver protocols to auto-register; empty = KnownDriverProtocols
	Metadata             map[string]any
	Endpoint             *AgentEndpoint
	// Unified generates 9 consolidated capabilities instead of 63 protocol-specific ones.
	// Used by MCP Runtime to reduce LLM tool count. Northbound EAN Runtime uses false (default).
	Unified bool
}

// Runtime is the EdgeX Capability Runtime entrypoint (EAN 2.0).
// EdgeOS acts as Coordination Platform; EdgeX exposes Capabilities via this runtime.
type Runtime struct {
	cfg        RuntimeConfig
	registry   *Registry
	dispatcher *Dispatcher
	discovery  *DiscoveryPublisher
	events     *EventPublisher
	bus        Bus
	logger     *zap.Logger

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Invoke metrics collector (EX-P3-07)
	metrics *invokeMetricsCollector
}

// NewRuntime builds a Capability Runtime with auto-generated Capabilities.
func NewRuntime(cfg RuntimeConfig, bus Bus) (*Runtime, error) {
	if cfg.AgentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if cfg.AgentVersion == "" {
		cfg.AgentVersion = RuntimeVersion
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportMQTT
	}
	if cfg.HeartbeatIntervalSec <= 0 {
		cfg.HeartbeatIntervalSec = 30
	}
	if bus == nil {
		bus = NoopBus{}
	}

	agent := Agent{
		ID:                   cfg.AgentID,
		Kind:                 AgentKindDevice,
		Version:              cfg.AgentVersion,
		Status:               AgentStatusOnline,
		Transport:            cfg.Transport,
		HeartbeatIntervalSec: cfg.HeartbeatIntervalSec,
		Endpoint:             cfg.Endpoint,
		Metadata:             cfg.Metadata,
	}

	registry := NewRegistry(agent)
	var caps []Capability
	if cfg.Unified {
		caps = GenerateUnifiedCapabilities(cfg.AgentID)
	} else {
		caps = GenerateDefaultCapabilities(cfg.AgentID, cfg.Protocols)
	}
	if err := registry.RegisterAll(caps); err != nil {
		return nil, err
	}

	dispatcher := NewDispatcher(registry)
	discovery := NewDiscoveryPublisher(registry, bus)
	events := NewEventPublisher(cfg.AgentID, bus)

	return &Runtime{
		cfg:        cfg,
		registry:   registry,
		dispatcher: dispatcher,
		discovery:  discovery,
		events:     events,
		bus:        bus,
		logger:     zap.L().With(zap.String("component", "ean-capability-runtime")),
		stopCh:     make(chan struct{}),
		metrics:    newInvokeMetricsCollector(),
	}, nil
}

// Registry returns the local Capability Registry.
func (r *Runtime) Registry() *Registry { return r.registry }

// Dispatcher returns the Invoke Dispatcher.
func (r *Runtime) Dispatcher() *Dispatcher { return r.dispatcher }

// Discovery returns the Discovery Publisher.
func (r *Runtime) Discovery() *DiscoveryPublisher { return r.discovery }

// Events returns the Event Publisher.
func (r *Runtime) Events() *EventPublisher { return r.events }

// SetBus swaps the transport bus (e.g. when MQTT connects) and refreshes publishers.
func (r *Runtime) SetBus(bus Bus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if bus == nil {
		bus = NoopBus{}
	}
	r.bus = bus
	r.discovery = NewDiscoveryPublisher(r.registry, bus)
	r.events = NewEventPublisher(r.cfg.AgentID, bus)
}

// SetMapper installs Capability → Driver execution mapping.
func (r *Runtime) SetMapper(mapper Mapper) {
	r.dispatcher.SetMapper(mapper)
}

// RegisterHandler registers a category handler on the dispatcher.
func (r *Runtime) RegisterHandler(category CapabilityCategory, handler Handler) {
	r.dispatcher.RegisterHandler(category, handler)
}

// Start publishes discovery descriptors, subscribes invoke/discovery topics,
// and starts the heartbeat loop. Safe to call when bus is offline (soft-fail).
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	r.started = true
	r.stopCh = make(chan struct{})
	r.mu.Unlock()

	if err := r.subscribeTopics(); err != nil {
		r.logger.Warn("EAN subscribe failed (will retry on reconnect)", zap.Error(err))
	}
	if err := r.discovery.PublishStartup(); err != nil {
		r.logger.Warn("EAN discovery startup publish failed", zap.Error(err))
	} else {
		r.logger.Info("EAN discovery published",
			zap.String("agent_id", r.cfg.AgentID),
			zap.Int("capabilities", r.registry.Count()),
		)
	}

	r.wg.Add(1)
	go r.heartbeatLoop(ctx)
	return nil
}

// OnConnected should be called when the northbound bus becomes connected.
func (r *Runtime) OnConnected(ctx context.Context) {
	if err := r.subscribeTopics(); err != nil {
		r.logger.Warn("EAN subscribe on connect failed", zap.Error(err))
	}
	if err := r.discovery.PublishStartup(); err != nil {
		r.logger.Warn("EAN rediscovery on connect failed", zap.Error(err))
	}
}

// Stop publishes offline and stops background loops.
func (r *Runtime) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	r.started = false
	close(r.stopCh)
	r.mu.Unlock()

	_ = r.discovery.PublishOffline("graceful_shutdown")
	r.wg.Wait()
}

// Invoke is a local (in-process) Capability Invoke entry — used by HTTP/SDK/MCP adapters.
func (r *Runtime) Invoke(ctx context.Context, req InvokeRequest) InvokeResponse {
	resp := r.dispatcher.Dispatch(ctx, req)
	_ = r.events.PublishCapabilityInvoked(req.Capability, resp.Status, map[string]any{
		"invoke_id":  resp.InvokeID,
		"latency_ms": resp.LatencyMs,
	})
	// 采集 Invoke metrics (EX-P3-07) | Collect Invoke metrics
	if r.metrics != nil {
		r.metrics.Record(resp)
	}
	return resp
}

// Metrics returns a point-in-time snapshot of Invoke metrics for API exposure.
func (r *Runtime) Metrics() InvokeMetricsSnapshot {
	if r.metrics == nil {
		return InvokeMetricsSnapshot{}
	}
	return r.metrics.Snapshot()
}

func (r *Runtime) subscribeTopics() error {
	if r.bus == nil || !r.bus.IsConnected() {
		return nil
	}
	agentID := r.cfg.AgentID
	if err := r.bus.Subscribe(TopicInvoke(agentID), QoSInvoke, r.handleInvokeMessage); err != nil {
		return fmt.Errorf("subscribe invoke: %w", err)
	}
	if err := r.bus.Subscribe(TopicInvokeStatus(agentID), QoSQuery, r.handleInvokeStatusMessage); err != nil {
		return fmt.Errorf("subscribe invoke status: %w", err)
	}
	if err := r.bus.Subscribe(TopicDiscoveryQuery, QoSQuery, func(_ string, payload []byte) {
		if err := r.discovery.HandleDiscoveryQuery(payload); err != nil {
			r.logger.Warn("discovery query failed", zap.Error(err))
		}
	}); err != nil {
		return fmt.Errorf("subscribe discovery query: %w", err)
	}
	return nil
}

func (r *Runtime) handleInvokeMessage(_ string, payload []byte) {
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		r.logger.Warn("invalid invoke message", zap.Error(err))
		return
	}
	req, err := DecodeBody[InvokeRequest](msg.Body)
	if err != nil {
		r.logger.Warn("invalid invoke body", zap.Error(err))
		return
	}
	resp := r.Invoke(context.Background(), req)
	reply := NewEnvelopeTo(r.cfg.AgentID, msg.Header.Source, MessageTypeInvokeResponse, msg.Header.CorrelationID, resp)
	data, err := json.Marshal(reply)
	if err != nil {
		return
	}
	replyTo := TopicReply(msg.Header.Source)
	if msg.Header.Source == "" {
		replyTo = TopicReply(r.cfg.AgentID)
	}
	if r.bus != nil && r.bus.IsConnected() {
		_ = r.bus.Publish(replyTo, data, QoSInvoke)
	}
}

func (r *Runtime) handleInvokeStatusMessage(_ string, payload []byte) {
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	query, err := DecodeBody[InvokeStatusQuery](msg.Body)
	if err != nil {
		return
	}
	status, ok := r.dispatcher.GetStatus(query.InvokeID)
	if !ok {
		status = InvokeResponse{
			InvokeID: query.InvokeID,
			Status:   InvokeRejected,
			Result: InvokeResult{
				Success:   false,
				Error:     "invoke not found",
				ErrorCode: "E009",
				Timestamp: NowMilli(),
			},
		}
	}
	reply := NewEnvelopeTo(r.cfg.AgentID, msg.Header.Source, MessageTypeInvokeResponse, msg.Header.CorrelationID, status)
	data, err := json.Marshal(reply)
	if err != nil {
		return
	}
	replyTo := TopicReply(msg.Header.Source)
	if msg.Header.Source == "" {
		replyTo = TopicReply(r.cfg.AgentID)
	}
	if r.bus != nil && r.bus.IsConnected() {
		_ = r.bus.Publish(replyTo, data, QoSInvoke)
	}
}

func (r *Runtime) heartbeatLoop(ctx context.Context) {
	defer r.wg.Done()
	interval := time.Duration(r.cfg.HeartbeatIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 每 60s 周期性重发 Capability Descriptor，解决启动时序竞态
	capPublishInterval := int64(60 / r.cfg.HeartbeatIntervalSec)
	if capPublishInterval < 1 {
		capPublishInterval = 1
	}
	var tickCount int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			if err := r.discovery.PublishHeartbeat(); err != nil {
				r.logger.Debug("heartbeat publish failed", zap.Error(err))
			}
			tickCount++
			if tickCount%capPublishInterval == 0 {
				if err := r.discovery.PublishCapabilityDescriptor(); err != nil {
					r.logger.Debug("capability descriptor publish failed", zap.Error(err))
				} else {
					r.logger.Info("EAN capability descriptor republished",
						zap.String("agent_id", r.cfg.AgentID),
						zap.Int("capabilities", r.registry.Count()),
					)
				}
			}
		}
	}
}
