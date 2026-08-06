package mitsubishi

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

type MCScheduler struct {
	transport *MCTransport
	decoder   *MCDecoder
	batchMax  int

	totalRequests int64
	successCount  int64
	failureCount  int64
	mu            sync.Mutex
}

func NewMCScheduler(transport *MCTransport, decoder *MCDecoder, batchMax int) *MCScheduler {
	if batchMax < 1 {
		batchMax = 1
	}
	return &MCScheduler{
		transport: transport,
		decoder:   decoder,
		batchMax:  batchMax,
	}
}

type parsedPoint struct {
	Point model.Point
	Addr  *MCAddress
}

func (s *MCScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value)
	var parsed []parsedPoint

	for _, p := range points {
		addr, err := ParseAddress(p.Address)
		if err != nil {
			zap.L().Warn("[Mitsubishi] invalid address",
				zap.String("point", p.Name),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			continue
		}
		parsed = append(parsed, parsedPoint{Point: p, Addr: addr})
	}

	groups := s.groupPoints(parsed)
	for _, group := range groups {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		s.readGroup(group, results)
	}

	return results, nil
}

func (s *MCScheduler) groupPoints(points []parsedPoint) [][]parsedPoint {
	groupMap := make(map[string][]parsedPoint)
	for _, p := range points {
		key := p.Addr.groupKey()
		groupMap[key] = append(groupMap[key], p)
	}

	var groups [][]parsedPoint
	for _, pts := range groupMap {
		sort.Slice(pts, func(i, j int) bool {
			return pts[i].Addr.readOffset() < pts[j].Addr.readOffset()
		})
		for i := 0; i < len(pts); i += s.batchMax {
			end := i + s.batchMax
			if end > len(pts) {
				end = len(pts)
			}
			groups = append(groups, pts[i:end])
		}
	}
	return groups
}

func (s *MCScheduler) readGroup(group []parsedPoint, results map[string]model.Value) {
	for _, pp := range group {
		s.readSingle(pp, results)
	}
}

func (s *MCScheduler) readSingle(pp parsedPoint, results map[string]model.Value) {
	s.incTotal()
	byteLen, isBit := s.decoder.ReadSize(pp.Point.DataType, pp.Addr)

	data, err := s.transport.ReadRaw(pp.Addr, byteLen, isBit)
	if err != nil {
		zap.L().Debug("[Mitsubishi] read failed",
			zap.String("point", pp.Point.Name),
			zap.String("address", pp.Point.Address),
			zap.Error(err),
		)
		results[pp.Point.ID] = model.Value{PointID: pp.Point.ID, Quality: "Bad", TS: time.Now()}
		s.incFailure()
		return
	}

	val, err := s.decoder.DecodeValue(data, pp.Addr, pp.Point.DataType)
	if err != nil {
		results[pp.Point.ID] = model.Value{PointID: pp.Point.ID, Quality: "Bad", TS: time.Now()}
		s.incFailure()
		return
	}

	results[pp.Point.ID] = model.Value{
		PointID: pp.Point.ID,
		Value:   val,
		Quality: "Good",
		TS:      time.Now(),
	}
	s.incSuccess()
}

func (s *MCScheduler) WritePoint(ctx context.Context, p model.Point, value interface{}) error {
	addr, err := ParseAddress(p.Address)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", p.Address, err)
	}

	data, isBit, err := s.decoder.EncodeValue(addr, p.DataType, value)
	if err != nil {
		return err
	}

	s.incTotal()
	if err := s.transport.WriteRaw(addr, data, isBit); err != nil {
		s.incFailure()
		return err
	}
	s.incSuccess()
	return nil
}

func (s *MCScheduler) GetStats() (total, success, failure int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRequests, s.successCount, s.failureCount
}

func (s *MCScheduler) incTotal() {
	s.mu.Lock()
	s.totalRequests++
	s.mu.Unlock()
}

func (s *MCScheduler) incSuccess() {
	s.mu.Lock()
	s.successCount++
	s.mu.Unlock()
}

func (s *MCScheduler) incFailure() {
	s.mu.Lock()
	s.failureCount++
	s.mu.Unlock()
}
