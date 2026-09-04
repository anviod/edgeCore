package core

import (
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// ShadowWriteRingBuffer is a fixed-capacity queue for shadow ingress messages.
// Producers push message pointers (zero-copy payload); Flush drains and returns
// the batch for batch apply to ShadowCore.
type ShadowWriteRingBuffer struct {
	mu       sync.Mutex
	slots    []*model.ShadowIngressMessage
	head     int
	count    int
	capacity int
}

func NewShadowWriteRingBuffer(capacity int) *ShadowWriteRingBuffer {
	if capacity <= 0 {
		capacity = 4096
	}
	return &ShadowWriteRingBuffer{
		slots:    make([]*model.ShadowIngressMessage, capacity),
		capacity: capacity,
	}
}

func (rb *ShadowWriteRingBuffer) Push(msg *model.ShadowIngressMessage) bool {
	if msg == nil {
		return true
	}
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count >= rb.capacity {
		return false
	}
	idx := (rb.head + rb.count) % rb.capacity
	rb.slots[idx] = msg
	rb.count++
	return true
}

func (rb *ShadowWriteRingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *ShadowWriteRingBuffer) Flush() []*model.ShadowIngressMessage {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		return nil
	}
	out := make([]*model.ShadowIngressMessage, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		out = append(out, rb.slots[idx])
		rb.slots[idx] = nil
	}
	rb.head = 0
	rb.count = 0
	return out
}
