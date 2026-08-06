package core

import (
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// DataPipeline handles the flow of collected data
type DataPipeline struct {
	mu            sync.Mutex
	pointBuf      map[string][]model.Value
	signalChan    chan struct{}
	handlers      []func(model.Value)
	batchHandlers []func([]model.Value)
	shadowIngress *ShadowIngress
}

func NewDataPipeline(bufferSize int) *DataPipeline {
	return &DataPipeline{
		pointBuf:   make(map[string][]model.Value),
		signalChan: make(chan struct{}, 1), // Non-blocking signal with size 1
		handlers:   make([]func(model.Value), 0),
	}
}

func (dp *DataPipeline) AddHandler(h func(model.Value)) {
	dp.handlers = append(dp.handlers, h)
}

// AddBatchHandler 注册批量处理器，一次 drain 调用一次，减少 handler 开销。
func (dp *DataPipeline) AddBatchHandler(h func([]model.Value)) {
	dp.batchHandlers = append(dp.batchHandlers, h)
}

func (dp *DataPipeline) SetShadowIngress(si *ShadowIngress) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.shadowIngress = si
}

func (dp *DataPipeline) Start() {
	go func() {
		for range dp.signalChan {
			dp.drainAndProcess()
		}
	}()
}

func (dp *DataPipeline) Push(val model.Value) {
	dp.PushBatch([]model.Value{val})
}

func (dp *DataPipeline) PushBatch(vals []model.Value) {
	if len(vals) == 0 {
		return
	}

	dp.mu.Lock()
	for _, val := range vals {
		key := val.ChannelID + "/" + val.DeviceID + "/" + val.PointID
		buf := dp.pointBuf[key]
		if len(buf) >= 2 {
			buf = buf[1:]
		}
		buf = append(buf, val)
		dp.pointBuf[key] = buf
	}
	dp.mu.Unlock()

	select {
	case dp.signalChan <- struct{}{}:
	default:
	}
}

func (dp *DataPipeline) drainAndProcess() {
	dp.mu.Lock()
	if len(dp.pointBuf) == 0 {
		dp.mu.Unlock()
		return
	}

	var batch []model.Value
	for k, buf := range dp.pointBuf {
		batch = append(batch, buf...)
		delete(dp.pointBuf, k)
	}
	dp.mu.Unlock()

	dp.processBatch(batch)
}

func (dp *DataPipeline) processBatch(batch []model.Value) {
	dp.mu.Lock()
	handlers := make([]func(model.Value), len(dp.handlers))
	copy(handlers, dp.handlers)
	batchHandlers := make([]func([]model.Value), len(dp.batchHandlers))
	copy(batchHandlers, dp.batchHandlers)
	shadowIngress := dp.shadowIngress
	dp.mu.Unlock()

	if shadowIngress != nil && len(batch) > 0 {
		if err := shadowIngress.IngestBatch(batch); err != nil {
			// Shadow device is an enhancement, not critical path
		}
	}

	for _, h := range batchHandlers {
		h(batch)
	}

	for _, val := range batch {
		dp.processValue(val, handlers)
	}
}

func (dp *DataPipeline) process(val model.Value) {
	dp.mu.Lock()
	handlers := make([]func(model.Value), len(dp.handlers))
	copy(handlers, dp.handlers)
	shadowIngress := dp.shadowIngress
	dp.mu.Unlock()

	if shadowIngress != nil {
		if err := shadowIngress.Ingest(val); err != nil {
			// Log error but continue processing
		}
	}

	dp.processValue(val, handlers)
}

func (dp *DataPipeline) processValue(val model.Value, handlers []func(model.Value)) {
	for _, h := range handlers {
		h(val)
	}
}
