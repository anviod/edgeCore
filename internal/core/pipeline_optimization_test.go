package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestPipeline_Optimization(t *testing.T) {
	dp := NewDataPipeline(10)
	// Don't call Start() yet, so we can simulate "consumer slow/stopped"

	// Simulate data for Point 1
	// Key construction: ChannelID + "/" + DeviceID + "/" + PointID
	// Here empty ChannelID and DeviceID means key is "//p1"
	val1 := model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p1", Value: 1, TS: time.Now()}
	val2 := model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p1", Value: 2, TS: time.Now()}
	val3 := model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p1", Value: 3, TS: time.Now()}

	dp.Push(val1)
	dp.Push(val2)
	dp.Push(val3)

	// Check buffer content manually (since we are in 'core' package, we can access private fields if in same package)
	// However, this test file is in 'core' package.

	key := "c1/d1/p1"
	dp.mu.Lock()
	buf := dp.pointBuf[key]
	dp.mu.Unlock()

	if len(buf) != 2 {
		t.Errorf("Expected buffer size 2, got %d", len(buf))
	}
	if buf[0].Value != 2 || buf[1].Value != 3 {
		t.Errorf("Expected values [2, 3], got [%v, %v]", buf[0].Value, buf[1].Value)
	}

	// Test another point
	val4 := model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p2", Value: 10, TS: time.Now()}
	dp.Push(val4)

	dp.mu.Lock()
	buf2 := dp.pointBuf["c1/d1/p2"]
	dp.mu.Unlock()

	if len(buf2) != 1 {
		t.Errorf("Expected buffer size 1 for p2, got %d", len(buf2))
	}
}

func TestPipeline_StartHandlersAndBatch(t *testing.T) {
	dp := NewDataPipeline(10)
	dp.Start()

	var singleCount, batchCount int
	dp.AddHandler(func(v model.Value) {
		singleCount++
	})
	dp.AddBatchHandler(func(batch []model.Value) {
		batchCount++
		if len(batch) == 0 {
			t.Error("batch handler received empty batch")
		}
	})

	val := model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p1", Value: 42}
	dp.Push(val)
	dp.PushBatch([]model.Value{
		{ChannelID: "c1", DeviceID: "d1", PointID: "p2", Value: 1},
		{ChannelID: "c1", DeviceID: "d1", PointID: "p3", Value: 2},
	})

	deadline := time.After(2 * time.Second)
	for singleCount == 0 || batchCount == 0 {
		select {
		case <-deadline:
			t.Fatalf("handlers not invoked: single=%d batch=%d", singleCount, batchCount)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	sc := NewShadowCore()
	ingress := NewShadowIngress(sc, 64, 10*time.Millisecond)
	dp.SetShadowIngress(ingress)
	dp.process(model.Value{ChannelID: "c1", DeviceID: "d1", PointID: "p4", Value: 99})
	dp.PushBatch(nil)
}
