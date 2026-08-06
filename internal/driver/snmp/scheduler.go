package snmp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
)

type SNMPScheduler struct {
	transport *SNMPTransport
	decoder   *SNMPDecoder
	cfg       deviceConfig

	totalRequests atomic.Int64
	successCount  atomic.Int64
	failureCount  atomic.Int64
	mu            sync.Mutex
}

func NewSNMPScheduler(transport *SNMPTransport, decoder *SNMPDecoder, cfg map[string]any) *SNMPScheduler {
	return &SNMPScheduler{
		transport: transport,
		decoder:   decoder,
		cfg:       parseDeviceConfig(cfg),
	}
}

func (s *SNMPScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if s.transport == nil || !s.transport.IsConnected() {
		return nil, fmt.Errorf("snmp not connected")
	}
	if len(points) == 0 {
		return map[string]model.Value{}, nil
	}

	groups := s.groupPoints(points)
	out := make(map[string]model.Value, len(points))

	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return out, err
		}

		if len(group.points) == 1 {
			s.readSingle(ctx, group, out)
		} else {
			s.readBatch(ctx, group, out)
		}

		if s.cfg.SendInterval > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(s.cfg.SendInterval):
			}
		}
	}
	return out, nil
}

func (s *SNMPScheduler) WritePoint(ctx context.Context, point model.Point, value any) error {
	if s.transport == nil || !s.transport.IsConnected() {
		return fmt.Errorf("snmp not connected")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	addr, err := s.decoder.ParseAddress(point.Address, s.cfg)
	if err != nil {
		return err
	}
	community := s.resolveCommunity(addr)

	encoded, asnType, err := s.decoder.EncodeValue(value, point.DataType)
	if err != nil {
		return err
	}

	s.totalRequests.Add(1)
	if err := s.transport.Set(addr.OID, encoded, asnType, community); err != nil {
		s.failureCount.Add(1)
		return err
	}
	s.successCount.Add(1)
	return nil
}

func (s *SNMPScheduler) GetStats() (totalRequests, successCount, failureCount int64) {
	return s.totalRequests.Load(), s.successCount.Load(), s.failureCount.Load()
}

type pointGroup struct {
	key       string
	community string
	points    []model.Point
	oids      []string
}

func (s *SNMPScheduler) groupPoints(points []model.Point) []pointGroup {
	groupMap := make(map[string]*pointGroup)
	order := make([]string, 0)

	for _, p := range points {
		addr, err := s.decoder.ParseAddress(p.Address, s.cfg)
		if err != nil {
			continue
		}
		key := s.groupKey(addr)
		g, ok := groupMap[key]
		if !ok {
			g = &pointGroup{
				key:       key,
				community: s.resolveCommunity(addr),
			}
			groupMap[key] = g
			order = append(order, key)
		}
		g.points = append(g.points, p)
		g.oids = append(g.oids, addr.OID)
	}

	out := make([]pointGroup, 0, len(order))
	for _, key := range order {
		out = append(out, *groupMap[key])
	}
	return out
}

func (s *SNMPScheduler) groupKey(addr *Address) string {
	if s.cfg.isV3() {
		return "v3:" + addr.SecurityName
	}
	return "v2c:" + addr.Community
}

func (s *SNMPScheduler) resolveCommunity(addr *Address) string {
	if s.cfg.isV3() {
		return ""
	}
	if addr.Community != "" {
		return addr.Community
	}
	return s.cfg.Community
}

func (s *SNMPScheduler) readSingle(ctx context.Context, group pointGroup, out map[string]model.Value) {
	point := group.points[0]
	s.totalRequests.Add(1)

	pdus, err := s.transport.Get([]string{group.oids[0]}, group.community)
	if err != nil || len(pdus) == 0 {
		s.failureCount.Add(1)
		out[point.ID] = model.Value{PointID: point.ID, Quality: "Bad", TS: time.Now()}
		return
	}
	s.successCount.Add(1)
	out[point.ID] = s.decoder.ToModelValue(point, pdus[0])
	_ = ctx
}

func (s *SNMPScheduler) readBatch(ctx context.Context, group pointGroup, out map[string]model.Value) {
	s.totalRequests.Add(1)
	pdus, err := s.transport.Get(group.oids, group.community)
	if err != nil {
		s.failureCount.Add(1)
		for _, p := range group.points {
			out[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
		}
		return
	}
	s.successCount.Add(1)

	pduByOID := make(map[string]gosnmp.SnmpPDU, len(pdus))
	for _, pdu := range pdus {
		pduByOID[pdu.Name] = pdu
	}

	for i, point := range group.points {
		oid := group.oids[i]
		pdu, ok := pduByOID[oid]
		if !ok {
			out[point.ID] = model.Value{PointID: point.ID, Quality: "Bad", TS: time.Now()}
			continue
		}
		out[point.ID] = s.decoder.ToModelValue(point, pdu)
	}
	_ = ctx
}
