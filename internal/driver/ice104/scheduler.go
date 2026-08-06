package ice104

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

type ICE104Scheduler struct {
	transport *ICE104Transport
	decoder   *ICE104Decoder
	cfg       deviceConfig
}

func NewICE104Scheduler(transport *ICE104Transport, decoder *ICE104Decoder, cfg map[string]any) *ICE104Scheduler {
	return &ICE104Scheduler{
		transport: transport,
		decoder:   decoder,
		cfg:       parseDeviceConfig(cfg),
	}
}

func (s *ICE104Scheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if s.transport == nil || !s.transport.IsConnected() {
		return nil, fmt.Errorf("ice104 not connected")
	}
	if len(points) == 0 {
		return map[string]model.Value{}, nil
	}

	needsGeneralCall := false
	for _, p := range points {
		if p.ReportMode == "event" || p.ReadWrite == "Subscribe" {
			continue
		}
		needsGeneralCall = true
		break
	}

	if needsGeneralCall {
		callCtx, cancel := context.WithTimeout(ctx, s.cfg.T1)
		err := s.transport.SendGeneralCall(callCtx)
		cancel()
		if err != nil {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
	}

	out := make(map[string]model.Value, len(points))
	deadline := time.Now().Add(s.cfg.T1)

	for _, p := range points {
		typeID, ioa, err := s.decoder.PointMeta(p)
		if err != nil {
			out[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			continue
		}
		key := s.decoder.PointKey(typeID, ioa)

		var cp cachedPoint
		var ok bool
		for time.Now().Before(deadline) {
			cp, ok = s.transport.GetCached(key)
			if ok {
				break
			}
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
		if !ok {
			out[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
			continue
		}
		out[p.ID] = s.decoder.ToModelValue(p, cp)
	}
	return out, nil
}

func (s *ICE104Scheduler) WritePoint(ctx context.Context, point model.Point, value any) error {
	if s.transport == nil || !s.transport.IsConnected() {
		return fmt.Errorf("ice104 not connected")
	}
	_, ioa, err := s.decoder.PointMeta(point)
	if err != nil {
		return err
	}

	execute := false
	switch v := value.(type) {
	case bool:
		execute = v
	case float64:
		execute = v != 0
	case int:
		execute = v != 0
	case int64:
		execute = v != 0
	case string:
		execute = v == "1" || v == "true" || v == "on"
	default:
		execute = true
	}

	selectCtx, cancel := context.WithTimeout(ctx, s.cfg.T1)
	defer cancel()
	if err := s.transport.SendSingleCommand(selectCtx, ioa, false); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return s.transport.SendSingleCommand(selectCtx, ioa, execute)
}
