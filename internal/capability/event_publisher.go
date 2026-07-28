package capability

import (
	"encoding/json"
)

// EventPublisher publishes EAN 2.0 events to EdgeOS.
type EventPublisher struct {
	agentID string
	bus     Bus
}

// NewEventPublisher creates an EventPublisher for the given agent.
func NewEventPublisher(agentID string, bus Bus) *EventPublisher {
	if bus == nil {
		bus = NoopBus{}
	}
	return &EventPublisher{agentID: agentID, bus: bus}
}

// Publish sends an Event on $edgeos/event/{agent_id} (and device topic when set).
func (p *EventPublisher) Publish(evt Event) error {
	if evt.EventID == "" {
		evt.EventID = NewEventID()
	}
	if evt.AgentID == "" {
		evt.AgentID = p.agentID
	}
	if evt.Timestamp == 0 {
		evt.Timestamp = NowMilli()
	}
	if evt.Severity == "" {
		evt.Severity = SeverityInfo
	}

	msg := NewEnvelope(p.agentID, MessageTypeEvent, evt)
	if err := p.publish(TopicEvent(p.agentID), msg, QoSEvent); err != nil {
		return err
	}
	if evt.DeviceID != "" {
		return p.publish(TopicEventDevice(p.agentID, evt.DeviceID), msg, QoSEvent)
	}
	return nil
}

// PublishBroadcast publishes to $edgeos/event/broadcast.
func (p *EventPublisher) PublishBroadcast(evt Event) error {
	if evt.EventID == "" {
		evt.EventID = NewEventID()
	}
	if evt.AgentID == "" {
		evt.AgentID = p.agentID
	}
	if evt.Timestamp == 0 {
		evt.Timestamp = NowMilli()
	}
	msg := NewEnvelope(p.agentID, MessageTypeEvent, evt)
	return p.publish(TopicEventBroadcast, msg, QoSEvent)
}

// PublishPointChanged emits "{point_id}.changed".
func (p *EventPublisher) PublishPointChanged(deviceID, pointID string, value, previous any, metadata map[string]any) error {
	return p.Publish(Event{
		EventType:     pointID + ".changed",
		DeviceID:      deviceID,
		PointID:       pointID,
		Value:         value,
		PreviousValue: previous,
		Severity:      SeverityInfo,
		Metadata:      metadata,
	})
}

// PublishDeviceOnline emits device.online.
func (p *EventPublisher) PublishDeviceOnline(deviceID string) error {
	return p.Publish(Event{
		EventType: "device.online",
		DeviceID:  deviceID,
		Severity:  SeverityInfo,
	})
}

// PublishDeviceOffline emits device.offline.
func (p *EventPublisher) PublishDeviceOffline(deviceID string) error {
	return p.Publish(Event{
		EventType: "device.offline",
		DeviceID:  deviceID,
		Severity:  SeverityWarning,
	})
}

// PublishCapabilityInvoked emits capability.invoked after a successful/failed invoke.
func (p *EventPublisher) PublishCapabilityInvoked(capabilityID string, status InvokeStatus, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["capability"] = capabilityID
	metadata["status"] = string(status)
	return p.Publish(Event{
		EventType: "capability.invoked",
		Severity:  SeverityInfo,
		Metadata:  metadata,
	})
}

func (p *EventPublisher) publish(topic string, msg Message, qos byte) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if p.bus == nil || !p.bus.IsConnected() {
		return nil
	}
	return p.bus.Publish(topic, data, qos)
}
