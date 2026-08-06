package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/anviod/edgeCore/internal/model"
)

func TestShadowCore_ARMv7_VersionCounterAlignment(t *testing.T) {
	sc := NewShadowCore()
	offset := unsafe.Offsetof(sc.versionCounter)
	if offset%8 != 0 {
		t.Fatalf("versionCounter offset %d is not 8-byte aligned (ARMv7 atomic.Uint64 requires alignment)", offset)
	}

	// 并发 Add/Load 不应 panic（32-bit ARM 上未对齐 uint64 会 fault）。
	const goroutines = 16
	const perG = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				sc.versionCounter.Add(1)
				_ = sc.versionCounter.Load()
			}
		}()
	}
	wg.Wait()

	if got := sc.versionCounter.Load(); got != uint64(goroutines*perG) {
		t.Fatalf("expected version counter %d, got %d", goroutines*perG, got)
	}
}

func TestShadowCore_ARMv7_ShadowPointVersionField(t *testing.T) {
	// ShadowPoint.Version 在 struct 内由编译器对齐；验证无 panic 的赋值路径。
	pt := model.ShadowPoint{Version: 1}
	pt.Version = 42
	if pt.Version != 42 {
		t.Fatalf("unexpected version %d", pt.Version)
	}
}

func TestShadowCore_NotifyDeltaOnly(t *testing.T) {
	sc := NewShadowCore()

	var mu sync.Mutex
	var lastLen int
	sc.Subscribe(func(_ string, points map[string]model.ShadowPoint) {
		mu.Lock()
		lastLen = len(points)
		mu.Unlock()
	})

	msg := model.ShadowIngressMessage{
		MessageID: "m1",
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 1.0, Quality: "good"},
			{PointID: "p2", Value: 2.0, Quality: "good"},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if lastLen != 2 {
		t.Fatalf("first notify expected 2 delta points, got %d", lastLen)
	}
	mu.Unlock()

	msg.Points = []model.ShadowIngressPoint{
		{PointID: "p1", Value: 3.0, Quality: "good"},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("write2: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if lastLen != 1 {
		t.Fatalf("second notify expected 1 delta point, got %d", lastLen)
	}
	mu.Unlock()
}

func TestShadowCore_ResolvePublishTarget_NoClone(t *testing.T) {
	sc := NewShadowCore()
	msg := model.ShadowIngressMessage{
		MessageID: "m1",
		DeviceID:  "dev1",
		ChannelID: "ch1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "p1", Value: 1.0, Quality: "good"},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	ch, dev, err := sc.ResolvePublishTarget("shadow-dev1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ch != "ch1" || dev != "dev1" {
		t.Fatalf("got channel=%q device=%q", ch, dev)
	}
}

func BenchmarkWriteShadowDevice(b *testing.B) {
	sc := NewShadowCore()
	msg := model.ShadowIngressMessage{
		MessageID: "bench",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.5, Quality: "good"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MessageID = fmt.Sprintf("bench-%d", i)
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteShadowDevice_MultiPoint(b *testing.B) {
	sc := NewShadowCore()
	points := make([]model.ShadowIngressPoint, 10)
	for i := range points {
		points[i] = model.ShadowIngressPoint{
			PointID: fmt.Sprintf("point-%d", i),
			Value:   float64(i),
			Quality: "good",
		}
	}
	msg := model.ShadowIngressMessage{
		MessageID: "bench",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points:    points,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MessageID = fmt.Sprintf("bench-%d", i)
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetShadowDevice(b *testing.B) {
	sc := NewShadowCore()
	for i := 0; i < 100; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deviceID := fmt.Sprintf("shadow-device-%d", i%100)
		if _, err := sc.GetShadowDevice(deviceID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNotifySubscribers(b *testing.B) {
	sc := NewShadowCore()
	const subs = 10
	for i := 0; i < subs; i++ {
		sc.Subscribe(func(_ string, _ map[string]model.ShadowPoint) {})
	}

	msg := model.ShadowIngressMessage{
		MessageID: "bench",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.5, Quality: "good"},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		b.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.MessageID = fmt.Sprintf("bench-%d", i)
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStress_ShadowCoreConcurrentReadWriteMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress metrics in short mode")
	}

	sc := NewShadowCore()
	for i := 0; i < 50; i++ {
		msg := model.ShadowIngressMessage{
			MessageID: fmt.Sprintf("init-%d", i),
			DeviceID:  fmt.Sprintf("device-%d", i),
			ChannelID: "channel-1",
			Timestamp: time.Now(),
			Points: []model.ShadowIngressPoint{
				{PointID: "point-1", Value: float64(i), Quality: "good"},
			},
		}
		if _, err := sc.WriteShadowDevice(msg); err != nil {
			t.Fatalf("init write: %v", err)
		}
	}

	var writeCount atomic.Int64
	var readCount atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if id%2 == 0 {
						msg := model.ShadowIngressMessage{
							MessageID: fmt.Sprintf("rw-%d", writeCount.Load()),
							DeviceID:  fmt.Sprintf("device-%d", id%50),
							ChannelID: "channel-1",
							Timestamp: time.Now(),
							Points: []model.ShadowIngressPoint{
								{PointID: "point-1", Value: float64(id), Quality: "good"},
							},
						}
						if _, err := sc.WriteShadowDevice(msg); err == nil {
							writeCount.Add(1)
						}
					} else {
						deviceID := fmt.Sprintf("shadow-device-%d", id%50)
						if _, err := sc.GetShadowDevice(deviceID); err == nil {
							readCount.Add(1)
						}
					}
				}
			}
		}(i)
	}

	duration := 3 * time.Second
	time.Sleep(duration)
	close(stop)
	wg.Wait()

	writes := writeCount.Load()
	reads := readCount.Load()
	total := writes + reads

	t.Logf("Shadow ARMv7 stress metrics:")
	t.Logf("  duration:           %v", duration)
	t.Logf("  write ops:          %d (%.0f ops/sec)", writes, float64(writes)/duration.Seconds())
	t.Logf("  read ops:           %d (%.0f ops/sec)", reads, float64(reads)/duration.Seconds())
	t.Logf("  total throughput:   %.0f ops/sec", float64(total)/duration.Seconds())
	t.Logf("  shadow devices:     %d", sc.GetMetrics()["real_shadow_count"])
}
