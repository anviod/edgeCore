//go:build integration

package bacnet

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/edgeCore/internal/model"
)

func TestBugVerification_Strict(t *testing.T) {
	mock := &RealWorldMockClient{
		SmartMockClient: SmartMockClient{
			Devices: map[int]btypes.Device{
				2228316: {DeviceID: 2228316, Ip: "192.168.3.116", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 116, 0xBA, 0xC0}}},
				2228317: {DeviceID: 2228317, Ip: "192.168.3.117", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 117, 0xBA, 0xC0}}},
				2228318: {DeviceID: 2228318, Ip: "192.168.3.118", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 118, 0xBA, 0xC0}}},
				2228319: {DeviceID: 2228319, Ip: "192.168.3.119", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 119, 0xBA, 0xC0}}},
			},
			Values: map[string]interface{}{
				"2228316:2:1": float32(316.00),
				"2228317:2:1": float32(317.00),
				"2228318:2:1": float32(318.00),
				"2228319:2:1": float32(319.00),
			},
		},
		Delays:      make(map[int]time.Duration),
		Errors:      make(map[int]error),
		CallCounter: make(map[int]int),
	}

	d := NewBACnetDriver().(*BACnetDriver)
	d.clientFactory = func(cb *bacnetlib.ClientBuilder) (Client, error) {
		return mock, nil
	}
	d.Init(model.DriverConfig{Config: map[string]any{"ip": "0.0.0.0"}})
	d.Connect(context.Background())
	defer d.Disconnect()

	devices := []int{2228316, 2228317, 2228318, 2228319}
	for _, id := range devices {
		d.SetDeviceConfig(map[string]any{
			"instance_id":         id,
			"ip":                  fmt.Sprintf("192.168.3.%d", id%1000),
			"_internal_device_id": fmt.Sprintf("bacnet-%d", (id%1000)-300),
		})
	}
	d.SetDeviceConfig(map[string]any{
		"instance_id":         2228319,
		"ip":                  "192.168.3.112",
		"_internal_device_id": "Room_FC_2014_19",
	})

	time.Sleep(200 * time.Millisecond)

	mock.mu.Lock()
	mock.Delays[2228319] = 5000 * time.Millisecond
	mock.mu.Unlock()

	p16 := []model.Point{{ID: "P16", DeviceID: "bacnet-16", Address: "AnalogValue:1", DataType: "float32"}}
	p17 := []model.Point{{ID: "P17", DeviceID: "bacnet-17", Address: "AnalogValue:1", DataType: "float32"}}
	p18 := []model.Point{{ID: "P18", DeviceID: "bacnet-18", Address: "AnalogValue:1", DataType: "float32"}}
	p19 := []model.Point{{ID: "P19", DeviceID: "Room_FC_2014_19", Address: "AnalogValue:1", DataType: "float32"}}

	ctx := context.Background()

	t.Log("--- Phase 1: Offline device returns error ---")
	start := time.Now()
	_, err := d.ReadPoints(ctx, p19)
	if err == nil {
		t.Fatalf("Expected error for offline device 19")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatalf("Offline device read exceeded 4s budget: %v", time.Since(start))
	}

	t.Log("--- Phase 2: Concurrent access ---")
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	wg.Add(4)

	go func() {
		defer wg.Done()
		start := time.Now()
		_, err := d.ReadPoints(ctx, p19)
		if err == nil {
			errors <- fmt.Errorf("device 19 should fail")
		}
		if time.Since(start) > 4*time.Second {
			errors <- fmt.Errorf("device 19 exceeded 4s budget: %v", time.Since(start))
		}
	}()

	checkHealthy := func(p []model.Point, expected float32) {
		defer wg.Done()
		res, err := d.ReadPoints(ctx, p)
		if err != nil {
			errors <- fmt.Errorf("device %s failed: %v", p[0].DeviceID, err)
			return
		}
		val, ok := res[p[0].ID]
		if !ok || val.Value != expected {
			errors <- fmt.Errorf("device %s wrong value: %+v", p[0].DeviceID, res)
		}
	}

	go checkHealthy(p16, 316.0)
	go checkHealthy(p17, 317.0)
	go checkHealthy(p18, 318.0)

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("Verification Failed: %v", err)
	}
}
