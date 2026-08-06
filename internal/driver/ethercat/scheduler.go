package ethercat

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

// EtherCATScheduler orchestrates ReadPoints and WritePoint operations.
// It delegates PDO memory reads to the transport layer's snapshot and
// SDO reads to the transport's mailbox channel.
//
// Design: ReadPoints is zero-wait for PDO — it reads from atomic snapshots
// maintained by the PDO cycle thread. SDO reads are synchronous with
// independent timeout. WritePoint writes to RxPDO buffer for next-cycle
// delivery by the cycle thread.

type EtherCATScheduler struct {
	transport *EtherCATTransport
	decoder   *EtherCATDecoder
}

// NewEtherCATScheduler creates a new scheduler instance.
func NewEtherCATScheduler(transport *EtherCATTransport, decoder *EtherCATDecoder) *EtherCATScheduler {
	return &EtherCATScheduler{
		transport: transport,
		decoder:   decoder,
	}
}

// ReadPoints reads values for the given points.
// PDO points are read from the transport's atomic snapshot (zero-wait).
// SDO points are read via CoE mailbox with independent timeout.
// Each point is processed independently — a failure on one point does not
// abort the entire batch; failed points are returned with Quality="Bad".
func (s *EtherCATScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value, len(points))

	for _, p := range points {
		addr, err := ParseAddress(p.Address)
		if err != nil {
			results[p.ID] = model.Value{
				PointID: p.ID,
				Value:   nil,
				Quality: "Bad",
				TS:      time.Now(),
				Meta:    map[string]any{"error": err.Error()},
			}
			s.transport.incFailure()
			zap.L().Warn("ethercat: address parse failed",
				zap.String("point_id", p.ID),
				zap.String("address", p.Address),
				zap.Error(err),
			)
			continue
		}

		if addr.IsSDO {
			// SDO path: synchronous CoE mailbox read with independent timeout
			val, meta := s.readSDOValue(ctx, p, addr)
			if meta != nil {
				val.Meta = meta
			}
			results[p.ID] = val
			if val.Quality == "Good" {
				s.transport.incSuccess()
			} else {
				s.transport.incFailure()
			}
		} else {
			// PDO path: zero-wait snapshot read
			val, meta := s.readPDOValue(p, addr)
			if meta != nil {
				val.Meta = meta
			}
			results[p.ID] = val
			if val.Quality == "Good" {
				s.transport.incSuccess()
			} else {
				s.transport.incFailure()
			}
		}
	}

	return results, nil
}

// readPDOValue reads a single PDO point from the transport's atomic snapshot.
func (s *EtherCATScheduler) readPDOValue(p model.Point, addr *ParsedAddress) (model.Value, map[string]any) {
	byteSize := s.decoder.ByteSize(p.DataType)
	if byteSize == 0 {
		byteSize = 1
	}

	data := s.transport.getTxPDOSnapshot(addr.Position, addr.Offset, byteSize)
	if data == nil {
		return model.Value{
			PointID: p.ID,
			Value:   nil,
			Quality: "Bad",
			TS:      time.Now(),
		}, map[string]any{"error": "PDO snapshot data unavailable"}
	}

	val, err := s.decoder.DecodeValue(data, p.DataType, addr)
	if err != nil {
		return model.Value{
			PointID: p.ID,
			Value:   nil,
			Quality: "Bad",
			TS:      time.Now(),
		}, map[string]any{"error": err.Error()}
	}

	// Apply scale/offset if configured
	if p.Scale != 0 || p.Offset != 0 {
		if fv, ok := toFloat64(val); ok == nil {
			val = fv*p.Scale + p.Offset
		}
	}

	return model.Value{
		PointID: p.ID,
		Value:   val,
		Quality: "Good",
		TS:      time.Now(),
	}, nil
}

// readSDOValue reads a single SDO point via CoE mailbox.
func (s *EtherCATScheduler) readSDOValue(ctx context.Context, p model.Point, addr *ParsedAddress) (model.Value, map[string]any) {
	// Create a context with timeout for the SDO operation
	sdoCtx, cancel := context.WithTimeout(ctx, s.transport.channelCfg.timeout)
	defer cancel()

	// Use a channel to implement timeout
	type sdoResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan sdoResult, 1)

	go func() {
		data, err := s.transport.readSDO(sdoCtx, addr.Position, addr.Index, addr.SubIndex)
		resultCh <- sdoResult{data: data, err: err}
	}()

	select {
	case <-sdoCtx.Done():
		return model.Value{
			PointID: p.ID,
			Value:   nil,
			Quality: "Bad",
			TS:      time.Now(),
		}, map[string]any{"error": "SDO read timeout"}
	case res := <-resultCh:
		if res.err != nil {
			return model.Value{
				PointID: p.ID,
				Value:   nil,
				Quality: "Bad",
				TS:      time.Now(),
			}, map[string]any{"error": res.err.Error()}
		}

		val, err := s.decoder.DecodeValue(res.data, p.DataType, addr)
		if err != nil {
			return model.Value{
				PointID: p.ID,
				Value:   nil,
				Quality: "Bad",
				TS:      time.Now(),
			}, map[string]any{"error": err.Error()}
		}

		return model.Value{
			PointID: p.ID,
			Value:   val,
			Quality: "Good",
			TS:      time.Now(),
		}, nil
	}
}

// WritePoint writes a value to a single point.
// PDO points are encoded and written to the RxPDO buffer for next-cycle delivery.
// SDO points are written synchronously via CoE mailbox.
func (s *EtherCATScheduler) WritePoint(ctx context.Context, p model.Point, value any) error {
	addr, err := ParseAddress(p.Address)
	if err != nil {
		return fmt.Errorf("ethercat WritePoint: address parse: %w", err)
	}

	if addr.IsSDO {
		// SDO write: synchronous CoE mailbox
		data, err := s.decoder.EncodeValue(value, p.DataType, addr)
		if err != nil {
			return fmt.Errorf("ethercat WritePoint: encode: %w", err)
		}
		return s.transport.writeSDO(ctx, addr.Position, addr.Index, addr.SubIndex, data)
	}

	// PDO write: encode and write to RxPDO buffer
	data, err := s.decoder.EncodeValue(value, p.DataType, addr)
	if err != nil {
		return fmt.Errorf("ethercat WritePoint: encode: %w", err)
	}
	return s.transport.setRxPDOBuffer(addr.Position, addr.Offset, data)
}

// GetTransport returns the underlying transport for metrics access.
func (s *EtherCATScheduler) GetTransport() *EtherCATTransport {
	return s.transport
}
