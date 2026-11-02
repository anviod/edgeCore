package omron

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	finslib "github.com/anviod/fins"
)

type udpScheduler struct {
	client  udpClient
	decoder *finslib.Decoder

	maxFrameLength int
	minInterval    time.Duration

	totalRequests atomic.Int64
	successCount  atomic.Int64
	failureCount  atomic.Int64

	mu              sync.Mutex
	lastRequestTime time.Time
}

type groupedPoint struct {
	point  finslib.Point
	parsed *finslib.ParsedAddress
	offset int
}

type pointGroup struct {
	areaCode  uint8
	startAddr uint16
	count     uint16
	points    []groupedPoint
	isBit     bool
}

func newUDPScheduler(backend *udpBackend, decoder *finslib.Decoder, cfg map[string]interface{}) *udpScheduler {
	s := &udpScheduler{
		decoder:        decoder,
		maxFrameLength: 64,
	}
	if v, ok := cfg["maxFrameLength"].(float64); ok && int(v) > 0 {
		s.maxFrameLength = int(v)
	} else if v, ok := cfg["maxFrameLength"].(int); ok && v > 0 {
		s.maxFrameLength = v
	}
	if v, ok := cfg["minInterval"].(float64); ok {
		s.minInterval = time.Duration(v) * time.Millisecond
	} else if v, ok := cfg["minInterval"].(int); ok {
		s.minInterval = time.Duration(v) * time.Millisecond
	}
	return s
}

func (s *udpScheduler) setClient(client udpClient) {
	s.client = client
}

func (s *udpScheduler) ReadPoints(ctx context.Context, points []finslib.Point) (map[string]finslib.Value, error) {
	if len(points) == 0 {
		return map[string]finslib.Value{}, nil
	}
	if s.client == nil {
		return nil, finslib.ErrNotConnected
	}

	results := make(map[string]finslib.Value)
	parsedPoints := make([]struct {
		point  finslib.Point
		parsed *finslib.ParsedAddress
		err    error
	}, len(points))

	for i, p := range points {
		parsed, err := s.decoder.ParseAddress(p.Address)
		parsedPoints[i] = struct {
			point  finslib.Point
			parsed *finslib.ParsedAddress
			err    error
		}{point: p, parsed: parsed, err: err}
		if err != nil {
			results[p.ID] = finslib.Value{Quality: finslib.QualityBad, TS: time.Now()}
		}
	}

	for _, group := range s.groupPoints(parsedPoints) {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		s.waitMinInterval()
		s.totalRequests.Add(1)

		var data []byte
		var err error
		if group.isBit {
			data, err = s.readBitGroup(group)
		} else {
			data, err = s.readWordGroup(group)
		}

		if err != nil {
			s.failureCount.Add(1)
			now := time.Now()
			for _, gp := range group.points {
				results[gp.point.ID] = finslib.Value{Quality: finslib.QualityBad, TS: now}
			}
			continue
		}

		s.successCount.Add(1)
		now := time.Now()
		for _, gp := range group.points {
			val, decErr := s.decodePointValue(data, gp, group)
			if decErr != nil {
				results[gp.point.ID] = finslib.Value{Quality: finslib.QualityBad, TS: now}
			} else {
				results[gp.point.ID] = finslib.Value{Value: val, Quality: finslib.QualityGood, TS: now}
			}
		}
	}

	return results, nil
}

func (s *udpScheduler) WritePoint(ctx context.Context, point finslib.Point, value interface{}) error {
	if s.client == nil {
		return finslib.ErrNotConnected
	}

	s.totalRequests.Add(1)
	parsed, err := s.decoder.ParseAddress(point.Address)
	if err != nil {
		s.failureCount.Add(1)
		return err
	}

	isBit := parsed.IsBit || point.DataType == finslib.DataTypeBIT
	encoded, err := s.decoder.EncodeValue(value, point.DataType, parsed)
	if err != nil {
		s.failureCount.Add(1)
		return err
	}

	s.waitMinInterval()

	if isBit {
		bitArea, err := finslib.GetBitAreaCode(parsed.AreaCode)
		if err != nil {
			bitArea = parsed.AreaCode
		}
		boolVal := len(encoded) > 0 && encoded[0] != 0
		if err := s.client.WriteBits(bitArea, uint16(parsed.Address&0xFFFF), uint8(parsed.Bit&0xFF), []bool{boolVal}); err != nil {
			s.failureCount.Add(1)
			return err
		}
	} else {
		wordCount := uint16(((len(encoded) + 1) / 2) & 0xFFFF)
		if point.DataType == finslib.DataTypeSTRING && parsed.StringLen > 0 {
			wordCount = uint16(((parsed.StringLen + 1) / 2) & 0xFFFF)
			if len(encoded) < int(wordCount)*2 {
				padded := make([]byte, int(wordCount)*2)
				copy(padded, encoded)
				encoded = padded
			}
		}
		if err := s.client.WriteBytes(parsed.AreaCode, uint16(parsed.Address&0xFFFF), encoded); err != nil {
			s.failureCount.Add(1)
			return err
		}
	}

	s.successCount.Add(1)
	return nil
}

func (s *udpScheduler) GetStats() finslib.SchedulerStats {
	return finslib.SchedulerStats{
		TotalRequests: s.totalRequests.Load(),
		SuccessCount:  s.successCount.Load(),
		FailureCount:  s.failureCount.Load(),
	}
}

func (s *udpScheduler) groupPoints(parsedPoints []struct {
	point  finslib.Point
	parsed *finslib.ParsedAddress
	err    error
}) []pointGroup {
	type groupKey struct {
		areaCode uint8
		isBit    bool
	}

	groupsMap := make(map[groupKey][]groupedPoint)
	for _, pp := range parsedPoints {
		if pp.err != nil {
			continue
		}
		isBit := pp.parsed.IsBit || pp.point.DataType == finslib.DataTypeBIT
		key := groupKey{areaCode: pp.parsed.AreaCode, isBit: isBit}
		if isBit {
			if bitCode, err := finslib.GetBitAreaCode(pp.parsed.AreaCode); err == nil {
				key.areaCode = bitCode
			}
		}
		groupsMap[key] = append(groupsMap[key], groupedPoint{point: pp.point, parsed: pp.parsed})
	}

	var result []pointGroup
	for key, points := range groupsMap {
		sort.Slice(points, func(i, j int) bool {
			if points[i].parsed.Address != points[j].parsed.Address {
				return points[i].parsed.Address < points[j].parsed.Address
			}
			return points[i].parsed.Bit < points[j].parsed.Bit
		})

		if key.isBit {
			for _, p := range points {
				result = append(result, pointGroup{
					areaCode:  key.areaCode,
					startAddr: uint16(p.parsed.Address & 0xFFFF),
					count:     1,
					points:    []groupedPoint{p},
					isBit:     true,
				})
			}
			continue
		}

		current := pointGroup{areaCode: key.areaCode}
		maxBytes := s.maxFrameLength * 2
		for _, p := range points {
			typeSize, _ := s.decoder.DataTypeSize(p.point.DataType)
			if typeSize == 0 {
				if p.parsed.IsString {
					typeSize = p.parsed.StringLen
				} else {
					typeSize = 2
				}
			}
			wordOffset := p.parsed.Address
			wordCount := (typeSize + 1) / 2

			if len(current.points) == 0 {
				current.startAddr = uint16(wordOffset & 0xFFFF)
				current.count = uint16(wordCount & 0xFFFF)
				p.offset = 0
				current.points = append(current.points, p)
				continue
			}

			endAddr := int(current.startAddr) + int(current.count)
			newEnd := wordOffset + wordCount
			totalBytes := (newEnd - int(current.startAddr)) * 2
			if wordOffset <= endAddr+10 && totalBytes <= maxBytes {
				if wordOffset+wordCount > int(current.startAddr)+int(current.count) {
					current.count = uint16((wordOffset + wordCount - int(current.startAddr)) & 0xFFFF)
				}
				p.offset = (wordOffset - int(current.startAddr)) * 2
				current.points = append(current.points, p)
			} else {
				result = append(result, current)
				current = pointGroup{
					areaCode:  key.areaCode,
					startAddr: uint16(wordOffset & 0xFFFF),
					count:     uint16(wordCount & 0xFFFF),
					points:    []groupedPoint{p},
				}
				p.offset = 0
			}
		}
		if len(current.points) > 0 {
			result = append(result, current)
		}
	}
	return result
}

func (s *udpScheduler) readWordGroup(group pointGroup) ([]byte, error) {
	return s.client.ReadBytes(group.areaCode, group.startAddr, group.count)
}

func (s *udpScheduler) readBitGroup(group pointGroup) ([]byte, error) {
	if len(group.points) == 0 {
		return nil, nil
	}
	p := group.points[0]
	bits, err := s.client.ReadBits(group.areaCode, uint16(p.parsed.Address&0xFFFF), uint8(p.parsed.Bit&0xFF), 1)
	if err != nil {
		return nil, err
	}
	data := make([]byte, len(bits))
	for i, b := range bits {
		if b {
			data[i] = 0x01
		}
	}
	return data, nil
}

func (s *udpScheduler) decodePointValue(data []byte, gp groupedPoint, group pointGroup) (interface{}, error) {
	if group.isBit {
		if len(data) < 1 {
			return nil, finslib.ErrDataTooShort
		}
		return data[0]&0x01 != 0, nil
	}

	offset := gp.offset
	typeSize, _ := s.decoder.DataTypeSize(gp.point.DataType)
	if gp.parsed.IsString {
		end := offset + gp.parsed.StringLen
		if end > len(data) {
			end = len(data)
		}
		return s.decoder.DecodeValue(data[offset:end], finslib.DataTypeSTRING, gp.parsed)
	}
	if typeSize == 0 {
		typeSize = 2
	}
	if offset+typeSize > len(data) {
		return nil, finslib.ErrDataTooShort
	}
	return s.decoder.DecodeValue(data[offset:offset+typeSize], gp.point.DataType, gp.parsed)
}

func (s *udpScheduler) waitMinInterval() {
	if s.minInterval <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.lastRequestTime)
	if elapsed < s.minInterval {
		time.Sleep(s.minInterval - elapsed)
	}
	s.lastRequestTime = time.Now()
}
