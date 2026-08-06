package core

import (
	"log"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/google/uuid"
)

const defaultIngressRingCapacity = 4096

type ShadowIngress struct {
	mu sync.RWMutex

	shadowCore *ShadowCore
	// writeShadow overrides shadowCore.WriteShadowDevice when set (tests only).
	writeShadow func(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error)

	ring            *ShadowWriteRingBuffer
	messageBuffer   []*model.ShadowIngressMessage
	pendingReliable []*model.ShadowIngressMessage
	bufferMu        sync.Mutex
	bufferSize      int
	flushInterval   time.Duration

	stopChan chan struct{}
	wg       sync.WaitGroup

	metrics ShadowIngressMetrics
}

type ShadowIngressMetrics struct {
	TotalMessages   uint64
	TotalPoints     uint64
	FailedMessages  uint64
	BatchFlushes    uint64
	LastProcessTime time.Time
}

func NewShadowIngress(sc *ShadowCore, bufferSize int, flushInterval time.Duration) *ShadowIngress {
	if bufferSize <= 0 {
		bufferSize = 256
	}
	if flushInterval <= 0 {
		flushInterval = 8 * time.Millisecond
	}
	si := &ShadowIngress{
		shadowCore:    sc,
		ring:          NewShadowWriteRingBuffer(defaultIngressRingCapacity),
		messageBuffer: make([]*model.ShadowIngressMessage, 0, bufferSize),
		bufferSize:    bufferSize,
		flushInterval: flushInterval,
		stopChan:      make(chan struct{}),
	}

	return si
}

func (si *ShadowIngress) writeShadowDevice(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
	if si.writeShadow != nil {
		return si.writeShadow(msg)
	}
	return si.shadowCore.WriteShadowDevice(msg)
}

func (si *ShadowIngress) Start() {
	si.replayReliable()
	si.wg.Add(1)
	go si.flushLoop()
	log.Println("[ShadowIngress] Started")
}

func (si *ShadowIngress) Stop() {
	close(si.stopChan)
	si.wg.Wait()
	si.flushBuffer()
	log.Println("[ShadowIngress] Stopped")
}

func (si *ShadowIngress) flushLoop() {
	defer si.wg.Done()

	ticker := time.NewTicker(si.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-si.stopChan:
			return
		case <-ticker.C:
			si.flushBuffer()
		}
	}
}

func (si *ShadowIngress) flushBuffer() {
	si.bufferMu.Lock()
	ringBatch := si.ring.Flush()
	sliceBatch := si.messageBuffer
	si.messageBuffer = make([]*model.ShadowIngressMessage, 0, si.bufferSize)
	si.bufferMu.Unlock()

	if len(ringBatch) == 0 && len(sliceBatch) == 0 {
		return
	}

	msgs := make([]model.ShadowIngressMessage, 0, len(ringBatch)+len(sliceBatch))
	for _, m := range ringBatch {
		if m != nil {
			msgs = append(msgs, *m)
		}
	}
	for _, m := range sliceBatch {
		if m != nil {
			msgs = append(msgs, *m)
		}
	}

	if err := si.shadowCore.ApplyShadowWrites(msgs); err != nil {
		log.Printf("[ShadowIngress] Failed batch write: %v", err)
		si.mu.Lock()
		si.metrics.FailedMessages += uint64(len(msgs))
		si.mu.Unlock()
		return
	}

	si.mu.Lock()
	si.metrics.BatchFlushes++
	si.mu.Unlock()
}

func (si *ShadowIngress) enqueue(msg *model.ShadowIngressMessage) {
	si.bufferMu.Lock()
	if !si.ring.Push(msg) {
		si.messageBuffer = append(si.messageBuffer, msg)
	}
	shouldFlush := si.ring.Len()+len(si.messageBuffer) >= si.bufferSize
	si.bufferMu.Unlock()

	if shouldFlush {
		go si.flushBuffer()
	}
}

func (si *ShadowIngress) Ingest(val model.Value) error {
	qos := 0
	if val.Meta != nil {
		if q, ok := val.Meta["qos"].(int); ok {
			qos = q
		}
	}
	msg := si.valueToMessage(val)
	msg.QoS = qos

	if qos >= 1 {
		if _, err := si.writeShadowDevice(*msg); err != nil {
			si.bufferReliable(msg)
			return err
		}
		si.mu.Lock()
		si.metrics.TotalMessages++
		si.metrics.TotalPoints++
		si.metrics.LastProcessTime = time.Now()
		si.mu.Unlock()
		return nil
	}

	si.enqueue(msg)

	si.mu.Lock()
	si.metrics.TotalMessages++
	si.metrics.TotalPoints++
	si.metrics.LastProcessTime = time.Now()
	si.mu.Unlock()

	return nil
}

func (si *ShadowIngress) bufferReliable(msg *model.ShadowIngressMessage) {
	si.bufferMu.Lock()
	si.pendingReliable = append(si.pendingReliable, msg)
	si.bufferMu.Unlock()
}

func (si *ShadowIngress) replayReliable() {
	si.bufferMu.Lock()
	pending := si.pendingReliable
	si.pendingReliable = nil
	si.bufferMu.Unlock()

	for _, msg := range pending {
		if _, err := si.writeShadowDevice(*msg); err != nil {
			si.bufferReliable(msg)
		}
	}
}

func (si *ShadowIngress) IngestBatch(values []model.Value) error {
	if len(values) == 0 {
		return nil
	}

	msg := si.valuesToMessage(values)
	si.enqueue(msg)

	si.mu.Lock()
	si.metrics.TotalMessages++
	si.metrics.TotalPoints += uint64(len(values))
	si.metrics.LastProcessTime = time.Now()
	si.mu.Unlock()

	return nil
}

func (si *ShadowIngress) valueToMessage(val model.Value) *model.ShadowIngressMessage {
	return &model.ShadowIngressMessage{
		MessageID: uuid.New().String(),
		QoS:       0,
		DeviceID:  val.DeviceID,
		ChannelID: val.ChannelID,
		Timestamp: val.TS,
		Points: []model.ShadowIngressPoint{
			{
				PointID:     val.PointID,
				Value:       val.Value,
				Quality:     val.Quality,
				CollectedAt: val.TS,
			},
		},
		Meta: model.ShadowIngressMeta{
			Source: "pipeline",
		},
	}
}

func (si *ShadowIngress) valuesToMessage(values []model.Value) *model.ShadowIngressMessage {
	if len(values) == 0 {
		return nil
	}

	points := make([]model.ShadowIngressPoint, 0, len(values))
	for _, val := range values {
		points = append(points, model.ShadowIngressPoint{
			PointID:     val.PointID,
			Value:       val.Value,
			Quality:     val.Quality,
			CollectedAt: val.TS,
		})
	}

	return &model.ShadowIngressMessage{
		MessageID: uuid.New().String(),
		QoS:       0,
		DeviceID:  values[0].DeviceID,
		ChannelID: values[0].ChannelID,
		Timestamp: time.Now(),
		Points:    points,
		Meta: model.ShadowIngressMeta{
			Source: "pipeline",
		},
	}
}

func (si *ShadowIngress) IngestDirect(msg model.ShadowIngressMessage) error {
	if msg.QoS >= 1 {
		if _, err := si.writeShadowDevice(msg); err != nil {
			copy := msg
			si.bufferReliable(&copy)
			return err
		}
		si.mu.Lock()
		si.metrics.TotalMessages++
		si.metrics.TotalPoints += uint64(len(msg.Points))
		si.metrics.LastProcessTime = time.Now()
		si.mu.Unlock()
		return nil
	}

	copy := msg
	si.enqueue(&copy)

	si.mu.Lock()
	si.metrics.TotalMessages++
	si.metrics.TotalPoints += uint64(len(msg.Points))
	si.metrics.LastProcessTime = time.Now()
	si.mu.Unlock()

	return nil
}

func (si *ShadowIngress) GetMetrics() ShadowIngressMetrics {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.metrics
}

func (si *ShadowIngress) GetBufferSize() int {
	si.bufferMu.Lock()
	defer si.bufferMu.Unlock()
	return si.ring.Len() + len(si.messageBuffer)
}
