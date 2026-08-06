package dlt645

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

// DLT645Scheduler coordinates batch reads grouped by meter address.
type DLT645Scheduler struct {
	transport *DLT645Transport
	decoder   *DLT645Decoder

	totalRequests int64
	successCount  int64
	failureCount  int64
	mu            sync.Mutex
}

func NewDLT645Scheduler(transport *DLT645Transport, decoder *DLT645Decoder) *DLT645Scheduler {
	return &DLT645Scheduler{
		transport: transport,
		decoder:   decoder,
	}
}

type pointWithAddr struct {
	Point model.Point
	Addr  *ParsedAddress
}

// ReadPoints reads all points, grouping by meter address when possible.
func (s *DLT645Scheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value, len(points))
	hasError := false

	for _, p := range points {
		parsed, err := s.decoder.ParseAddress(p.Address)
		if err != nil {
			zap.L().Warn("[DLT645] Failed to parse address",
				zap.String("point", p.Name),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			s.incFailure()
			hasError = true
			continue
		}

		val, err := s.readPoint(ctx, p, parsed)
		quality := "Good"
		if err != nil {
			quality = "Bad"
			hasError = true
			s.incFailure()
			zap.L().Warn("[DLT645] Read failed",
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

	if hasError {
		if s.transport != nil {
			s.transport.RecordFailure(fmt.Errorf("partial read failure"))
		}
	} else if s.transport != nil {
		s.transport.RecordSuccess()
	}

	return results, nil
}

func (s *DLT645Scheduler) readPoint(ctx context.Context, p model.Point, addr *ParsedAddress) (any, error) {
	s.incRequest()

	raw, err := s.transport.ReadData(ctx, addr.MeterAddr, addr.DataID)
	if err != nil {
		return nil, err
	}

	return DecodeValue(raw, p.DataType, p.Scale, p.Offset)
}

// WritePoint writes a single point when the meter supports it.
func (s *DLT645Scheduler) WritePoint(ctx context.Context, p model.Point, value any) error {
	parsed, err := s.decoder.ParseAddress(p.Address)
	if err != nil {
		return err
	}

	payload, err := encodeWritePayload(value, p.DataType)
	if err != nil {
		return err
	}

	s.incRequest()
	if err := s.transport.WriteData(ctx, parsed.MeterAddr, parsed.DataID, payload); err != nil {
		s.incFailure()
		return err
	}
	s.incSuccess()
	return nil
}

func encodeWritePayload(value any, dataType string) ([]byte, error) {
	_ = dataType
	switch v := value.(type) {
	case float64:
		return encodeBCDFromInt(int64(v * 100)), nil
	case float32:
		return encodeBCDFromInt(int64(v * 100)), nil
	case int:
		return encodeBCDFromInt(int64(v)), nil
	case int64:
		return encodeBCDFromInt(v), nil
	case int32:
		return encodeBCDFromInt(int64(v)), nil
	default:
		return nil, fmt.Errorf("unsupported write value type: %T", value)
	}
}

func encodeBCDFromInt(val int64) []byte {
	if val < 0 {
		val = -val
	}
	digits := fmt.Sprintf("%012d", val)
	out := make([]byte, 6)
	for i := 0; i < 6; i++ {
		hi := digits[11-i*2] - '0'
		lo := digits[11-i*2-1] - '0'
		out[i] = byte(hi<<4 | lo)
	}
	return out
}

func (s *DLT645Scheduler) GetStats() (totalRequests, successCount, failureCount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRequests, s.successCount, s.failureCount
}

func (s *DLT645Scheduler) incRequest() {
	s.mu.Lock()
	s.totalRequests++
	s.mu.Unlock()
}

func (s *DLT645Scheduler) incSuccess() {
	s.mu.Lock()
	s.successCount++
	s.mu.Unlock()
}

func (s *DLT645Scheduler) incFailure() {
	s.mu.Lock()
	s.failureCount++
	s.mu.Unlock()
}
