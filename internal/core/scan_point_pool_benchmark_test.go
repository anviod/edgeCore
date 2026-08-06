package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func BenchmarkExecutionLayer_LoadPoints_Pooled(b *testing.B) {
	el := NewExecutionLayer()
	points := make([]model.Point, 100)
	for i := range points {
		points[i] = model.Point{ID: "p", Address: "0", DataType: "int16"}
	}
	task := &ScanTask{
		DeviceKey: "dev-1",
		Points:    points,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = el.loadPoints(task)
	}
}

func BenchmarkScanEngine_ApplyCollectToShadow_Pooled(b *testing.B) {
	sc := NewShadowCore()
	se := NewScanEngine(ScanEngineConfig{})
	se.SetShadowCore(sc)

	pointIDs := make([]string, 100)
	for i := range pointIDs {
		pointIDs[i] = "p"
	}
	task := &ScanTask{
		DeviceKey: "dev-1",
		Interval:  time.Second,
		PointIDs:  pointIDs,
	}
	result := &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p": {PointID: "p", Value: 1.0, Quality: "Good"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		se.applyCollectToShadow(task, result)
	}
}
