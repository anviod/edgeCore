package capability

// Bus is the minimal publish/subscribe transport used by Discovery / Event /
// Invoke publishers. MQTT and NATS northbound clients adapt to this interface.
type Bus interface {
	Publish(topic string, payload []byte, qos byte) error
	Subscribe(topic string, qos byte, handler func(topic string, payload []byte)) error
	IsConnected() bool
}

// NoopBus discards publishes and ignores subscriptions (useful for unit tests / offline).
type NoopBus struct{}

func (NoopBus) Publish(topic string, payload []byte, qos byte) error { return nil }
func (NoopBus) Subscribe(topic string, qos byte, handler func(topic string, payload []byte)) error {
	return nil
}
func (NoopBus) IsConnected() bool { return false }
