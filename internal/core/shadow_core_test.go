package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestShadowCore_WriteShadowDevice(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		QoS:       0,
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{
				PointID: "point-1",
				Value:   42.5,
				Unit:    "V",
				Quality: "good",
			},
		},
		Meta: model.ShadowIngressMeta{
			Source: "test",
		},
	}

	resp, err := sc.WriteShadowDevice(msg)
	if err != nil {
		t.Fatalf("WriteShadowDevice failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success, got failure")
	}

	if resp.Version == 0 {
		t.Errorf("Expected non-zero version")
	}

	device, err := sc.GetShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}

	if device.PhysicalDeviceID != "device-1" {
		t.Errorf("Expected device-1, got %s", device.PhysicalDeviceID)
	}

	if len(device.Points) != 1 {
		t.Errorf("Expected 1 point, got %d", len(device.Points))
	}

	point, exists := device.Points["point-1"]
	if !exists {
		t.Fatalf("Point point-1 not found")
	}

	if point.Value != 42.5 {
		t.Errorf("Expected 42.5, got %v", point.Value)
	}
}

func TestShadowCore_WriteShadowPoint(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 10.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	req := model.ShadowWriteRequest{
		ShadowDeviceID: "shadow-device-1",
		PointID:        "point-1",
		Value:          99.9,
		QoS:            0,
		Timestamp:      time.Now(),
	}

	resp, err := sc.WriteShadowPoint(req)
	if err != nil {
		t.Fatalf("WriteShadowPoint failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success, got failure")
	}

	device, _ := sc.GetShadowDevice("shadow-device-1")
	point := device.Points["point-1"]

	if point.Value != 99.9 {
		t.Errorf("Expected 99.9, got %v", point.Value)
	}
}

func TestShadowCore_CompareAndSwap(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 10.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	device, _ := sc.GetShadowDevice("shadow-device-1")
	expectedVersion := device.Version

	updates := map[string]any{
		"point-1": 20.0,
	}

	resp, err := sc.CompareAndSwap("shadow-device-1", expectedVersion, updates)
	if err != nil {
		t.Fatalf("CompareAndSwap failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success, got failure: %s", resp.Error)
	}

	device, _ = sc.GetShadowDevice("shadow-device-1")
	if device.Points["point-1"].Value != 20.0 {
		t.Errorf("Expected 20.0, got %v", device.Points["point-1"].Value)
	}

	resp, err = sc.CompareAndSwap("shadow-device-1", expectedVersion, updates)
	if err != nil {
		t.Fatalf("CompareAndSwap failed: %v", err)
	}

	if resp.Success {
		t.Errorf("Expected failure due to version mismatch, got success")
	}
}

func TestShadowCore_Subscribe(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	received := make(chan struct {
		deviceID string
		points   map[string]model.ShadowPoint
	}, 1)

	sc.Subscribe(func(deviceID string, points map[string]model.ShadowPoint) {
		received <- struct {
			deviceID string
			points   map[string]model.ShadowPoint
		}{deviceID, points}
	})

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	select {
	case r := <-received:
		if r.deviceID != "shadow-device-1" {
			t.Errorf("Expected shadow-device-1, got %s", r.deviceID)
		}
		if len(r.points) != 1 {
			t.Errorf("Expected 1 point, got %d", len(r.points))
		}
	case <-time.After(time.Second):
		t.Errorf("Timeout waiting for subscriber notification")
	}
}

func TestShadowCore_CheckConsistency(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
			{PointID: "point-2", Value: 10.0, Quality: "bad"},
		},
	}

	sc.WriteShadowDevice(msg)

	result, err := sc.CheckConsistency("shadow-device-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CheckConsistency failed: %v", err)
	}

	if result.Pass {
		t.Errorf("Expected consistency check to fail due to bad quality point")
	}

	if len(result.DiffPoints) == 0 {
		t.Errorf("Expected diff points to be reported")
	}
}

func TestShadowCore_MemoryOnly(t *testing.T) {
	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	sc2 := NewShadowCore()

	_, err := sc2.GetShadowDevice("shadow-device-1")
	if err == nil {
		t.Error("Expected empty shadow state after restart (memory-only)")
	}
}

func TestShadowCore_DeleteShadowDevice(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	err = sc.DeleteShadowDevice("shadow-device-1")
	if err != nil {
		t.Fatalf("DeleteShadowDevice failed: %v", err)
	}

	_, err = sc.GetShadowDevice("shadow-device-1")
	if err == nil {
		t.Errorf("Expected error after deletion, got nil")
	}
}

func TestShadowCore_ClearAllShadowDevices(t *testing.T) {
	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
		},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("WriteShadowDevice failed: %v", err)
	}
	sc.WriteVirtualShadowDevice("channel-1", "vdev-1", map[string]model.ShadowPoint{
		"vp1": {Value: 1.0, Quality: "good"},
	})

	sc.ClearAllShadowDevices()

	if _, err := sc.GetShadowDevice("shadow-device-1"); err == nil {
		t.Error("expected real shadow to be cleared")
	}
	if _, err := sc.GetVirtualShadowDevice("vdev-1"); err == nil {
		t.Error("expected virtual shadow to be cleared")
	}
	metrics := sc.GetMetrics()
	if metrics["real_shadow_count"].(int) != 0 || metrics["virtual_shadow_count"].(int) != 0 {
		t.Errorf("expected zero shadow counts, got %v", metrics)
	}
}

func TestShadowCore_GetMetrics(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()

	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  "device-1",
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.0, Quality: "good"},
		},
	}

	sc.WriteShadowDevice(msg)

	metrics := sc.GetMetrics()

	if metrics["real_shadow_count"].(int) != 1 {
		t.Errorf("Expected 1 real shadow, got %d", metrics["real_shadow_count"])
	}
}

func TestShadowCore_GetShadowDevice_ConcurrentReadWrite(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	const shadowID = "shadow-device-1"

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				msg := model.ShadowIngressMessage{
					MessageID: "msg",
					DeviceID:  "device-1",
					ChannelID: "channel-1",
					Timestamp: time.Now(),
					Points: []model.ShadowIngressPoint{
						{PointID: "point-1", Value: float64(i), Quality: "good"},
						{PointID: "point-2", Value: float64(i + 1), Quality: "good"},
					},
				}
				if _, err := sc.WriteShadowDevice(msg); err != nil {
					t.Errorf("WriteShadowDevice: %v", err)
					return
				}
				i++
			}
		}
	}()

	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					device, err := sc.GetShadowDevice(shadowID)
					if err != nil {
						continue
					}
					for pid, pt := range device.Points {
						_ = pid
						_ = pt.Value
					}
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
