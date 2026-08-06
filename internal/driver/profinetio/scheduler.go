package profinetio

import (
	"context"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

// ProfinetScheduler reads/writes points via transport and decoder.
type ProfinetScheduler struct {
	transport *ProfinetTransport
	decoder   *ProfinetDecoder

	totalRequests int64
	successCount  int64
	failureCount  int64
	mu            sync.Mutex
}

func NewProfinetScheduler(transport *ProfinetTransport, decoder *ProfinetDecoder) *ProfinetScheduler {
	return &ProfinetScheduler{
		transport: transport,
		decoder:   decoder,
	}
}

type pointWithAddr struct {
	Point model.Point
	Addr  *ParsedAddress
}

func (s *ProfinetScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value)
	hasError := false

	for _, p := range points {
		addr, err := ParseAddress(p.Address)
		if err != nil {
			zap.L().Warn("[Profinet IO] invalid address",
				zap.String("point", p.Name),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			hasError = true
			continue
		}

		size := ByteSize(p.DataType)
		data, err := s.transport.ReadIO(ctx, addr.Slot, addr.SubSlot, addr.Index, size)
		if err != nil {
			zap.L().Warn("[Profinet IO] read failed",
				zap.String("point", p.Name),
				zap.Error(err),
			)
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			hasError = true
			continue
		}

		val, err := s.decoder.DecodeValue(data, p.DataType, addr)
		if err != nil {
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			hasError = true
			continue
		}

		if p.Scale != 0 || p.Offset != 0 {
			if f, err := toFloat64(val); err == nil {
				val = f*p.Scale + p.Offset
			}
		}

		results[p.ID] = model.Value{
			PointID: p.ID,
			Value:   val,
			Quality: "Good",
			TS:      time.Now(),
		}
		s.incSuccess()
	}

	if !hasError {
		s.transport.RecordSuccess()
	}
	return results, nil
}

func (s *ProfinetScheduler) WritePoint(ctx context.Context, p model.Point, value any) error {
	addr, err := ParseAddress(p.Address)
	if err != nil {
		return err
	}
	data, err := s.decoder.EncodeValue(value, p.DataType, addr)
	if err != nil {
		return err
	}
	if err := s.transport.WriteIO(ctx, addr.Slot, addr.SubSlot, addr.Index, data); err != nil {
		s.incFailure()
		return err
	}
	s.incSuccess()
	s.transport.RecordSuccess()
	return nil
}

func (s *ProfinetScheduler) GetStats() (total, success, failure int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRequests, s.successCount, s.failureCount
}

func (s *ProfinetScheduler) incSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.successCount++
}

func (s *ProfinetScheduler) incFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.failureCount++
}
