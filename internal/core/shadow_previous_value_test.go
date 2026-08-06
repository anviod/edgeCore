package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestShadowCore_NotifyIncludesPreviousValue(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	var mu sync.Mutex
	var got []map[string]model.ShadowPoint
	sc.Subscribe(func(_ string, points map[string]model.ShadowPoint) {
		cp := make(map[string]model.ShadowPoint, len(points))
		for k, v := range points {
			cp[k] = v
		}
		mu.Lock()
		got = append(got, cp)
		mu.Unlock()
	})

	write := func(v float64) {
		_, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
			MessageID: "m",
			DeviceID:  "dev-pv",
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "temp", Value: v, Quality: "good"},
			},
		})
		if err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	write(10.0)
	write(20.5)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("expected 2 notifies, got %d", len(got))
	}
	first := got[0]["temp"]
	if first.PreviousValue != nil {
		t.Fatalf("first write should have nil previous_value, got %#v", first.PreviousValue)
	}
	second := got[1]["temp"]
	if second.Value != 20.5 {
		t.Fatalf("second value want 20.5 got %#v", second.Value)
	}
	prev, ok := second.PreviousValue.(float64)
	if !ok || prev != 10.0 {
		t.Fatalf("second previous_value want 10.0 got %#v", second.PreviousValue)
	}
	// Stored snapshot must not retain notify-only PreviousValue.
	dev, err := sc.GetShadowDevice("shadow-dev-pv")
	if err != nil {
		t.Fatalf("get shadow: %v", err)
	}
	if pt := dev.Points["temp"]; pt.PreviousValue != nil {
		t.Fatalf("stored shadow must not keep previous_value, got %#v", pt.PreviousValue)
	}
}
