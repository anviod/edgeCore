package capability

import (
	"sync"

	"go.uber.org/zap"
)

// ShadowPointView is a transport-neutral snapshot used by Shadow→Event bridging.
// Kept free of core/model imports so capability stays a leaf package.
type ShadowPointView struct {
	Value         any
	PreviousValue any
	Quality       string
	TimestampMs   int64
}

// ShadowEventBridge publishes EAN Events when Shadow point values change.
type ShadowEventBridge struct {
	mu     sync.RWMutex
	events []*EventPublisher
	logger *zap.Logger
}

// NewShadowEventBridge creates an empty bridge; attach publishers via AddPublisher.
func NewShadowEventBridge() *ShadowEventBridge {
	return &ShadowEventBridge{
		logger: zap.L().With(zap.String("component", "ean-shadow-event-bridge")),
	}
}

// AddPublisher registers an EventPublisher (e.g. from MQTT/NATS Capability Runtime).
func (b *ShadowEventBridge) AddPublisher(pub *EventPublisher) {
	if b == nil || pub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, pub)
}

// SetPublishers replaces the publisher list (used when runtimes reconnect).
func (b *ShadowEventBridge) SetPublishers(pubs []*EventPublisher) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append([]*EventPublisher{}, pubs...)
}

// HandleDelta is the hot-path callback for ShadowCore subscribers.
// deviceID should be the physical device id (not shadow- prefixed).
func (b *ShadowEventBridge) HandleDelta(deviceID, channelID string, points map[string]ShadowPointView) {
	if b == nil || len(points) == 0 {
		return
	}
	b.mu.RLock()
	pubs := b.events
	b.mu.RUnlock()
	if len(pubs) == 0 {
		return
	}
	for pointID, pt := range points {
		meta := map[string]any{}
		if channelID != "" {
			meta["channel_id"] = channelID
		}
		if pt.Quality != "" {
			meta["quality"] = pt.Quality
		}
		for _, pub := range pubs {
			if err := pub.PublishPointChanged(deviceID, pointID, pt.Value, pt.PreviousValue, meta); err != nil {
				b.logger.Debug("EAN shadow event publish failed",
					zap.String("device_id", deviceID),
					zap.String("point_id", pointID),
					zap.Error(err),
				)
			}
		}
	}
}
