package core

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestShadowNotifyPool_BoundedWorkers(t *testing.T) {
	sc := NewShadowCoreWithNotifyWorkers(4)
	sc.Start()
	defer sc.Stop()

	var notifyCount atomic.Int64
	sc.Subscribe(func(_ string, _ map[string]model.ShadowPoint) {
		notifyCount.Add(1)
	})

	const writes = 100
	for i := 0; i < writes; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("m-%d", i),
			DeviceID:  fmt.Sprintf("dev-%d", i%20),
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "p1", Value: float64(i), Quality: "good"},
			},
		}
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	if got := notifyCount.Load(); got != int64(writes) {
		t.Fatalf("expected %d notifies, got %d", writes, got)
	}
	if got := sc.NotifyWorkerCount(); got != 4 {
		t.Fatalf("expected 4 notify workers, got %d", got)
	}
}

func TestShadowNotifyPool_NoUnboundedGoroutines(t *testing.T) {
	sc := NewShadowCoreWithNotifyWorkers(6)
	sc.Start()
	defer sc.Stop()

	sc.Subscribe(func(_ string, _ map[string]model.ShadowPoint) {})

	before := runtime.NumGoroutine()
	for i := 0; i < 500; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("m-%d", i),
			DeviceID:  "dev1",
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "p1", Value: float64(i), Quality: "good"},
			},
		}
		_, _ = sc.WriteShadowDevice(msg)
	}
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Allow slack for test runtime; should not grow by hundreds.
	if after > before+20 {
		t.Fatalf("goroutine growth too high: before=%d after=%d", before, after)
	}
}

func TestShadowCOW_ConcurrentReadWriteConsistent(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	msg := model.ShadowIngressMessage{
		MessageID: "init",
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 1.0, Quality: "good"},
			{PointID: "p2", Value: 2.0, Quality: "good"},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("init write: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					dev, err := sc.GetShadowDevice("shadow-dev1")
					if err != nil {
						t.Errorf("read: %v", err)
						return
					}
					if len(dev.Points) != 2 {
						t.Errorf("expected 2 points, got %d", len(dev.Points))
						return
					}
					_ = dev.Points["p1"].Value
				}
			}
		}(i)
	}

	for i := 0; i < 100; i++ {
		msg.Points = []model.ShadowIngressPoint{
			{PointID: "p1", Value: float64(i), Quality: "good"},
		}
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestShadowWriteRingBuffer_BatchFlush(t *testing.T) {
	rb := NewShadowWriteRingBuffer(8)
	msgs := make([]*model.ShadowIngressMessage, 5)
	for i := range msgs {
		msgs[i] = &model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("m-%d", i),
			DeviceID:  "dev1",
		}
		if !rb.Push(msgs[i]) {
			t.Fatalf("push %d failed", i)
		}
	}
	if rb.Len() != 5 {
		t.Fatalf("expected len 5, got %d", rb.Len())
	}
	out := rb.Flush()
	if len(out) != 5 {
		t.Fatalf("expected flush 5, got %d", len(out))
	}
	if rb.Len() != 0 {
		t.Fatalf("expected empty after flush")
	}
}

func TestShadowCore_ApplyShadowWrites_Batch(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	var notifyCount atomic.Int64
	var lastDelta atomic.Int64
	sc.Subscribe(func(_ string, points map[string]model.ShadowPoint) {
		notifyCount.Add(1)
		lastDelta.Store(int64(len(points)))
	})

	msgs := make([]model.ShadowIngressMessage, 3)
	for i := range msgs {
		msgs[i] = model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("b-%d", i),
			DeviceID:  "dev1",
			ChannelID: "ch1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: fmt.Sprintf("p%d", i), Value: float64(i), Quality: "good"},
			},
		}
	}
	if err := sc.ApplyShadowWrites(msgs); err != nil {
		t.Fatalf("batch apply: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	dev, err := sc.GetShadowDevice("shadow-dev1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(dev.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(dev.Points))
	}
	if notifyCount.Load() != 1 {
		t.Fatalf("expected 1 notify for same device batch, got %d", notifyCount.Load())
	}
	if lastDelta.Load() != 3 {
		t.Fatalf("expected merged delta of 3 points, got %d", lastDelta.Load())
	}
}

func TestShadowIngress_ScanEnginePath(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	si := NewShadowIngress(sc, 64, 5*time.Millisecond)
	si.Start()
	defer si.Stop()

	var received atomic.Int64
	sc.Subscribe(func(_ string, _ map[string]model.ShadowPoint) {
		received.Add(1)
	})

	se := NewScanEngine(ScanEngineConfig{WorkerCount: 1})
	se.SetShadowIngress(si)

	msg := model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 25.0, Quality: "good"},
		},
		Meta: model.ShadowIngressMeta{Source: "scan_engine"},
	}
	if err := si.IngestDirect(msg); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	if received.Load() == 0 {
		t.Fatal("expected pipeline notify after ingress flush")
	}
	_ = se
}

func TestShadowDeviceOptimizer_LazyProfileUpdate(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()
	deviceID := "lazy-dev"

	sdo.UpdateDeviceRTT(deviceID, 100000) // 100ms

	profile := (*model.DeviceCommunicationProfile)(nil)
	if !sdo.UpdateShadowDeviceProfileIfNeeded(deviceID, "ch1", &profile) {
		t.Fatal("expected first profile creation")
	}
	firstRTT := profile.EWMARTT
	firstUpdated := profile.LastUpdated

	time.Sleep(2 * time.Millisecond)
	profile2 := profile
	sdo.UpdateDeviceRTT(deviceID, 102000) // +2ms < 10ms threshold
	if sdo.UpdateShadowDeviceProfileIfNeeded(deviceID, "ch1", &profile2) {
		t.Fatal("expected no profile update for small RTT change")
	}
	if profile2.LastUpdated != firstUpdated {
		t.Fatal("profile LastUpdated should be unchanged")
	}

	sdo.UpdateDeviceRTT(deviceID, 200000) // large jump triggers threshold
	profile3 := profile2
	if !sdo.UpdateShadowDeviceProfileIfNeeded(deviceID, "ch1", &profile3) {
		t.Fatal("expected profile update for large RTT change")
	}
	if profile3.EWMARTT <= firstRTT {
		t.Fatalf("expected EWMARTT to increase from %d, got %d", firstRTT, profile3.EWMARTT)
	}
}

func BenchmarkGetShadowDevice_COW(b *testing.B) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	points := make([]model.ShadowIngressPoint, 100)
	for i := range points {
		points[i] = model.ShadowIngressPoint{
			PointID: fmt.Sprintf("p%d", i),
			Value:   float64(i),
			Quality: "good",
		}
	}
	msg := model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    points,
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sc.GetShadowDevice("shadow-dev1"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkApplyShadowWrites_10kTags(b *testing.B) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	const tagCount = 10000
	points := make([]model.ShadowIngressPoint, tagCount)
	for i := range points {
		points[i] = model.ShadowIngressPoint{
			PointID: fmt.Sprintf("tag-%d", i),
			Value:   float64(i),
			Quality: "good",
		}
	}
	msg := model.ShadowIngressMessage{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    points,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sc.ApplyShadowWrites([]model.ShadowIngressMessage{msg}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStress_ShadowRingBuffer_10kThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10k ring buffer stress in short mode")
	}

	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	const tagCount = 10000
	points := make([]model.ShadowIngressPoint, tagCount)
	for i := range points {
		points[i] = model.ShadowIngressPoint{
			PointID: fmt.Sprintf("tag-%d", i),
			Value:   float64(i),
			Quality: "good",
		}
	}

	start := time.Now()
	if err := sc.ApplyShadowWrites([]model.ShadowIngressMessage{{
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points:    points,
	}}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	elapsed := time.Since(start)

	dev, err := sc.GetShadowDevice("shadow-dev1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(dev.Points) != tagCount {
		t.Fatalf("expected %d points, got %d", tagCount, len(dev.Points))
	}

	t.Logf("10k tag batch apply: %v (%.0f tags/sec)", elapsed, float64(tagCount)/elapsed.Seconds())
}
