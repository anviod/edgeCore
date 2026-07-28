package capability

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// NewMessageID returns a unique EAN message id.
func NewMessageID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "msg-" + hex.EncodeToString(b)
}

// NewInvokeID returns a unique invoke id.
func NewInvokeID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "invoke-" + hex.EncodeToString(b)
}

// NewEventID returns a unique event id.
func NewEventID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "evt-" + hex.EncodeToString(b)
}

// NowMilli returns current unix epoch milliseconds.
func NowMilli() int64 {
	return time.Now().UnixMilli()
}

// NewEnvelope builds a standard EAN 2.0 message.
func NewEnvelope(source, messageType string, body any) Message {
	return Message{
		Header: MessageHeader{
			MessageID:   NewMessageID(),
			Timestamp:   NowMilli(),
			Source:      source,
			MessageType: messageType,
			Version:     ProtocolVersion,
		},
		Body: body,
	}
}

// NewEnvelopeTo builds a directed EAN 2.0 message.
func NewEnvelopeTo(source, destination, messageType, correlationID string, body any) Message {
	msg := NewEnvelope(source, messageType, body)
	msg.Header.Destination = destination
	msg.Header.CorrelationID = correlationID
	return msg
}
