package capability

import (
	"sync"
)

// MemoryBus is an in-memory Bus for unit tests.
type MemoryBus struct {
	mu          sync.RWMutex
	connected   bool
	published   []PublishedMessage
	subscribers map[string][]func(topic string, payload []byte)
}

// PublishedMessage records a publish for assertions.
type PublishedMessage struct {
	Topic   string
	Payload []byte
	QoS     byte
}

// NewMemoryBus creates a connected in-memory bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		connected:   true,
		subscribers: make(map[string][]func(topic string, payload []byte)),
	}
}

func (b *MemoryBus) SetConnected(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = v
}

func (b *MemoryBus) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

func (b *MemoryBus) Publish(topic string, payload []byte, qos byte) error {
	b.mu.Lock()
	b.published = append(b.published, PublishedMessage{Topic: topic, Payload: payload, QoS: qos})
	handlers := append([]func(string, []byte){}, b.subscribers[topic]...)
	b.mu.Unlock()
	for _, h := range handlers {
		h(topic, payload)
	}
	return nil
}

func (b *MemoryBus) Subscribe(topic string, qos byte, handler func(topic string, payload []byte)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[topic] = append(b.subscribers[topic], handler)
	return nil
}

// Published returns a copy of published messages.
func (b *MemoryBus) Published() []PublishedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]PublishedMessage, len(b.published))
	copy(out, b.published)
	return out
}
