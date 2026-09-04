package core

import (
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

const scanPointSliceInitialCap = 16

var (
	pointSlicePool = sync.Pool{
		New: func() any {
			s := make([]model.Point, 0, scanPointSliceInitialCap)
			return &s
		},
	}
	shadowIngressPointSlicePool = sync.Pool{
		New: func() any {
			s := make([]model.ShadowIngressPoint, 0, scanPointSliceInitialCap)
			return &s
		},
	}
)

func borrowPointSlice(minCap int) *[]model.Point {
	raw := pointSlicePool.Get().(*[]model.Point)
	s := *raw
	if cap(s) < minCap {
		s = make([]model.Point, 0, minCap)
	} else {
		s = s[:0]
	}
	*raw = s
	return raw
}

func returnPointSlice(raw *[]model.Point) {
	if raw == nil {
		return
	}
	s := *raw
	if cap(s) > 512 {
		return
	}
	s = s[:0]
	*raw = s
	pointSlicePool.Put(raw)
}

func borrowShadowIngressPointSlice(minCap int) *[]model.ShadowIngressPoint {
	raw := shadowIngressPointSlicePool.Get().(*[]model.ShadowIngressPoint)
	s := *raw
	if cap(s) < minCap {
		s = make([]model.ShadowIngressPoint, 0, minCap)
	} else {
		s = s[:0]
	}
	*raw = s
	return raw
}

func returnShadowIngressPointSlice(raw *[]model.ShadowIngressPoint) {
	if raw == nil {
		return
	}
	s := *raw
	if cap(s) > 512 {
		return
	}
	s = s[:0]
	*raw = s
	shadowIngressPointSlicePool.Put(raw)
}
