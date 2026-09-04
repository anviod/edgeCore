package core

import (
	"errors"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestShadowIngress_Ingest(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 100*time.Millisecond)

	val := model.Value{
		ChannelID: "channel-1",
		DeviceID:  "device-1",
		PointID:   "point-1",
		Value:     42.5,
		Quality:   "good",
		TS:        time.Now(),
	}

	err = si.Ingest(val)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 1 {
		t.Errorf("Expected 1 message, got %d", metrics.TotalMessages)
	}

	if metrics.TotalPoints != 1 {
		t.Errorf("Expected 1 point, got %d", metrics.TotalPoints)
	}
}

func TestShadowIngress_IngestBatch(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 100*time.Millisecond)

	values := []model.Value{
		{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-1",
			Value:     10.0,
			Quality:   "good",
			TS:        time.Now(),
		},
		{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-2",
			Value:     20.0,
			Quality:   "good",
			TS:        time.Now(),
		},
	}

	err = si.IngestBatch(values)
	if err != nil {
		t.Fatalf("IngestBatch failed: %v", err)
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 1 {
		t.Errorf("Expected 1 message, got %d", metrics.TotalMessages)
	}

	if metrics.TotalPoints != 2 {
		t.Errorf("Expected 2 points, got %d", metrics.TotalPoints)
	}
}

func TestShadowIngress_AutoFlush(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 3, 1*time.Second)
	si.Start()
	defer si.Stop()

	for i := 0; i < 5; i++ {
		val := model.Value{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-1",
			Value:     float64(i),
			Quality:   "good",
			TS:        time.Now(),
		}
		si.Ingest(val)
	}

	time.Sleep(200 * time.Millisecond)

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}

	if len(device.Points) == 0 {
		t.Errorf("Expected points to be flushed to shadow device")
	}
}

func TestShadowIngress_DirectIngest(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 100*time.Millisecond)

	msg := model.ShadowIngressMessage{
		MessageID: "direct-msg-1",
		QoS:       1,
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 100.0, Quality: "good"},
			{PointID: "point-2", Value: 200.0, Quality: "good"},
		},
	}

	err = si.IngestDirect(msg)
	if err != nil {
		t.Fatalf("IngestDirect failed: %v", err)
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 1 {
		t.Errorf("Expected 1 message, got %d", metrics.TotalMessages)
	}

	if metrics.TotalPoints != 2 {
		t.Errorf("Expected 2 points, got %d", metrics.TotalPoints)
	}
}

func TestShadowIngress_GetBufferSize(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 100, 10*time.Second)

	for i := 0; i < 10; i++ {
		val := model.Value{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-1",
			Value:     float64(i),
			Quality:   "good",
			TS:        time.Now(),
		}
		si.Ingest(val)
	}

	bufferSize := si.GetBufferSize()
	if bufferSize != 10 {
		t.Errorf("Expected buffer size 10, got %d", bufferSize)
	}
}

func TestShadowIngress_StartStop(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 50*time.Millisecond)

	si.Start()

	for i := 0; i < 5; i++ {
		val := model.Value{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-1",
			Value:     float64(i),
			Quality:   "good",
			TS:        time.Now(),
		}
		si.Ingest(val)
	}

	time.Sleep(100 * time.Millisecond)

	si.Stop()

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}

	if len(device.Points) == 0 {
		t.Errorf("Expected points to be flushed after stop")
	}
}

func TestShadowIngress_Metrics(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 100*time.Millisecond)

	for i := 0; i < 100; i++ {
		val := model.Value{
			ChannelID: "channel-1",
			DeviceID:  "device-1",
			PointID:   "point-1",
			Value:     float64(i),
			Quality:   "good",
			TS:        time.Now(),
		}
		si.Ingest(val)
	}

	metrics := si.GetMetrics()

	if metrics.TotalMessages != 100 {
		t.Errorf("Expected 100 messages, got %d", metrics.TotalMessages)
	}

	if metrics.TotalPoints != 100 {
		t.Errorf("Expected 100 points, got %d", metrics.TotalPoints)
	}

	if metrics.LastProcessTime.IsZero() {
		t.Errorf("Expected non-zero last process time")
	}
}

func TestShadowIngress_IngestQoS1DirectWrite(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	si := NewShadowIngress(sc, 10, 100*time.Millisecond)

	val := model.Value{
		ChannelID: "channel-1",
		DeviceID:  "device-1",
		PointID:   "point-1",
		Value:     99.0,
		Quality:   "good",
		TS:        time.Now(),
		Meta:      map[string]any{"qos": 1},
	}

	if err := si.Ingest(val); err != nil {
		t.Fatalf("Ingest QoS1 failed: %v", err)
	}

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}
	if got := device.Points["point-1"].Value; got != 99.0 {
		t.Fatalf("expected point value 99.0, got %v", got)
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 1 || metrics.TotalPoints != 1 {
		t.Fatalf("metrics = %+v, want 1 message / 1 point", metrics)
	}
}

func TestShadowIngress_BufferReliable_IngestQoS1(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	si := NewShadowIngress(sc, 10, 100*time.Millisecond)
	si.writeShadow = func(_ model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
		return nil, errors.New("simulated write failure")
	}

	val := model.Value{
		ChannelID: "channel-1",
		DeviceID:  "device-1",
		PointID:   "point-1",
		Value:     42.0,
		Quality:   "good",
		TS:        time.Now(),
		Meta:      map[string]any{"qos": 1},
	}

	err := si.Ingest(val)
	if err == nil {
		t.Fatal("expected Ingest QoS1 to return write error")
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 0 || metrics.TotalPoints != 0 {
		t.Fatalf("metrics = %+v, want no successful writes", metrics)
	}

	si.writeShadow = func(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
		return sc.WriteShadowDevice(msg)
	}
	si.replayReliable()

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}
	if got := device.Points["point-1"].Value; got != 42.0 {
		t.Fatalf("expected replayed value 42.0, got %v", got)
	}
}

func TestShadowIngress_BufferReliable_IngestDirect(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	si := NewShadowIngress(sc, 10, 100*time.Millisecond)
	si.writeShadow = func(_ model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
		return nil, errors.New("simulated write failure")
	}

	msg := model.ShadowIngressMessage{
		MessageID: "reliable-msg-1",
		QoS:       1,
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 88.0, Quality: "good"},
		},
	}

	err := si.IngestDirect(msg)
	if err == nil {
		t.Fatal("expected IngestDirect QoS1 to return write error")
	}

	metrics := si.GetMetrics()
	if metrics.TotalMessages != 0 {
		t.Fatalf("metrics = %+v, want no successful writes", metrics)
	}

	si.writeShadow = nil
	si.replayReliable()

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}
	if got := device.Points["point-1"].Value; got != 88.0 {
		t.Fatalf("expected replayed value 88.0, got %v", got)
	}
}

func TestShadowIngress_ReplayReliable_OnStart(t *testing.T) {
	sc := NewShadowCore()
	sc.Start()
	defer sc.Stop()

	si := NewShadowIngress(sc, 10, 100*time.Millisecond)
	fail := true
	si.writeShadow = func(msg model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
		if fail {
			return nil, errors.New("simulated write failure")
		}
		return sc.WriteShadowDevice(msg)
	}

	msg := model.ShadowIngressMessage{
		MessageID: "start-replay-1",
		QoS:       1,
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 77.0, Quality: "good"},
		},
	}
	if err := si.IngestDirect(msg); err == nil {
		t.Fatal("expected write failure before Start")
	}

	fail = false
	si.Start()
	defer si.Stop()

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}
	if got := device.Points["point-1"].Value; got != 77.0 {
		t.Fatalf("expected replayed value 77.0 on Start, got %v", got)
	}
}

func TestShadowIngress_BufferReliable_ReplayStillFails(t *testing.T) {
	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, 100*time.Millisecond)
	si.writeShadow = func(_ model.ShadowIngressMessage) (*model.ShadowWriteResponse, error) {
		return nil, errors.New("simulated write failure")
	}

	msg := model.ShadowIngressMessage{
		MessageID: "still-fail-1",
		QoS:       1,
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 55.0, Quality: "good"},
		},
	}
	if err := si.IngestDirect(msg); err == nil {
		t.Fatal("expected write failure")
	}

	si.Start()
	si.Stop()

	if _, err := sc.GetShadowDevice("shadow-device-1"); err == nil {
		t.Fatal("expected no shadow device when replay keeps failing")
	}
}
