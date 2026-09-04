package capability

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
)

// DiscoveryPublisher publishes Agent / Capability descriptors and heartbeats.
type DiscoveryPublisher struct {
	registry *Registry
	bus      Bus
	seq      int64
}

// NewDiscoveryPublisher creates a DiscoveryPublisher.
func NewDiscoveryPublisher(registry *Registry, bus Bus) *DiscoveryPublisher {
	if bus == nil {
		bus = NoopBus{}
	}
	return &DiscoveryPublisher{registry: registry, bus: bus}
}

// PublishAgentDescriptor publishes the Agent Descriptor to EdgeOS.
func (p *DiscoveryPublisher) PublishAgentDescriptor() error {
	agent := p.registry.GetAgent()
	msg := NewEnvelope(agent.ID, MessageTypeAgentDescriptor, AgentDescriptorBody{Agent: agent})
	return p.publish(TopicDiscoveryAgent, msg, QoSDiscovery)
}

// PublishCapabilityDescriptor publishes all local Capabilities.
func (p *DiscoveryPublisher) PublishCapabilityDescriptor() error {
	agentID := p.registry.AgentID()
	msg := NewEnvelope(agentID, MessageTypeCapabilityDescriptor, CapabilityDescriptorBody{
		Capabilities: p.registry.List(),
	})
	return p.publish(TopicDiscoveryCapability, msg, QoSDiscovery)
}

// PublishOffline publishes graceful offline notification.
func (p *DiscoveryPublisher) PublishOffline(reason string) error {
	agentID := p.registry.AgentID()
	p.registry.SetStatus(AgentStatusOffline)
	msg := NewEnvelope(agentID, MessageTypeAgentOffline, AgentOfflineBody{
		AgentID: agentID,
		Reason:  reason,
	})
	return p.publish(TopicDiscoveryAgentOffline, msg, QoSDiscovery)
}

// PublishHeartbeat publishes one heartbeat tick.
func (p *DiscoveryPublisher) PublishHeartbeat() error {
	agent := p.registry.GetAgent()
	seq := atomic.AddInt64(&p.seq, 1)
	msg := NewEnvelope(agent.ID, MessageTypeHeartbeat, HeartbeatBody{
		AgentID:   agent.ID,
		Status:    agent.Status,
		Timestamp: NowMilli(),
		Sequence:  seq,
	})
	return p.publish(TopicHeartbeat(agent.ID), msg, QoSHeartbeat)
}

// PublishStartup performs the EAN startup discovery sequence.
func (p *DiscoveryPublisher) PublishStartup() error {
	if err := p.PublishAgentDescriptor(); err != nil {
		return fmt.Errorf("publish agent descriptor: %w", err)
	}
	if err := p.PublishCapabilityDescriptor(); err != nil {
		return fmt.Errorf("publish capability descriptor: %w", err)
	}
	if err := p.PublishHeartbeat(); err != nil {
		return fmt.Errorf("publish heartbeat: %w", err)
	}
	return nil
}

// HandleDiscoveryQuery responds to EdgeOS discovery queries.
func (p *DiscoveryPublisher) HandleDiscoveryQuery(raw []byte) error {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	query, err := DecodeBody[DiscoveryQuery](msg.Body)
	if err != nil {
		return err
	}

	agent := p.registry.GetAgent()
	body := DiscoveryResponseBody{}
	switch query.QueryType {
	case "agent":
		body.Agent = &agent
	case "capability":
		body.Capabilities = p.filterCapabilities(query)
	default:
		body.Agent = &agent
		body.Capabilities = p.filterCapabilities(query)
	}

	resp := NewEnvelopeTo(agent.ID, msg.Header.Source, MessageTypeDiscoveryResponse, msg.Header.CorrelationID, body)
	return p.publish(TopicDiscoveryResponse, resp, QoSDiscovery)
}

func (p *DiscoveryPublisher) filterCapabilities(query DiscoveryQuery) []Capability {
	caps := p.registry.List()
	if query.CapabilityID != "" {
		if cap, ok := p.registry.Get(query.CapabilityID); ok {
			return []Capability{cap}
		}
		return nil
	}
	if query.Category == "" && query.Keyword == "" {
		return caps
	}
	out := make([]Capability, 0)
	for _, cap := range caps {
		if query.Category != "" && string(cap.Category) != query.Category {
			continue
		}
		if query.Keyword != "" {
			if !strings.Contains(strings.ToLower(cap.ID), strings.ToLower(query.Keyword)) &&
				!strings.Contains(strings.ToLower(cap.Description), strings.ToLower(query.Keyword)) {
				continue
			}
		}
		out = append(out, cap)
	}
	return out
}

func (p *DiscoveryPublisher) publish(topic string, msg Message, qos byte) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// Soft-fail when offline: discovery must not block edgeCore industrial path.
	if p.bus == nil || !p.bus.IsConnected() {
		return nil
	}
	return p.bus.Publish(topic, data, qos)
}
