package knxnetip

import (
	"context"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

// KNXScheduler coordinates point reads and writes.
type KNXScheduler struct {
	transport *KNXTransport
	decoder   *KNXDecoder

	totalRequests int64
	successCount  int64
	failureCount  int64
	mu            sync.Mutex
}

func NewKNXScheduler(transport *KNXTransport, decoder *KNXDecoder) *KNXScheduler {
	return &KNXScheduler{
		transport: transport,
		decoder:   decoder,
	}
}

func (s *KNXScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value, len(points))

	for _, p := range points {
		parsed, err := s.decoder.ParseAddress(p.Address)
		if err != nil {
			zap.L().Warn("[KNXnet/IP] failed to parse address",
				zap.String("point", p.Name),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			continue
		}

		val, err := s.readPoint(ctx, p, parsed)
		quality := "Good"
		if err != nil {
			quality = "Bad"
			s.incFailure()
			zap.L().Warn("[KNXnet/IP] read failed",
				zap.String("point", p.Name),
				zap.Error(err),
			)
		} else {
			s.incSuccess()
		}

		results[p.ID] = model.Value{
			PointID: p.ID,
			Value:   val,
			Quality: quality,
			TS:      time.Now(),
		}
	}

	return results, nil
}

func (s *KNXScheduler) readPoint(ctx context.Context, p model.Point, addr *ParsedAddress) (any, error) {
	s.incRequest()

	raw, err := s.transport.ReadGroup(ctx, addr.GroupAddr)
	if err != nil {
		return nil, err
	}

	return DecodeValue(raw, p.DataType, addr, p.Scale, p.Offset)
}

func (s *KNXScheduler) WritePoint(ctx context.Context, p model.Point, value any) error {
	parsed, err := s.decoder.ParseAddress(p.Address)
	if err != nil {
		return err
	}

	payload, err := EncodeValue(value, p.DataType, parsed)
	if err != nil {
		return err
	}

	s.incRequest()
	if err := s.transport.WriteGroup(ctx, parsed.GroupAddr, payload); err != nil {
		s.incFailure()
		return err
	}
	s.incSuccess()
	return nil
}

func (s *KNXScheduler) GetStats() (totalRequests, successCount, failureCount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRequests, s.successCount, s.failureCount
}

func (s *KNXScheduler) incRequest() {
	s.mu.Lock()
	s.totalRequests++
	s.mu.Unlock()
}

func (s *KNXScheduler) incSuccess() {
	s.mu.Lock()
	s.successCount++
	s.mu.Unlock()
}

func (s *KNXScheduler) incFailure() {
	s.mu.Lock()
	s.failureCount++
	s.mu.Unlock()
}
