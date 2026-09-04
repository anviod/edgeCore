package bacnet

import (
	"context"
	"fmt"
	"testing"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/edgeCore/internal/model"
)

// TestCompliance_BACnet_Isolation implements the test plan from "BACnet 多设备隔离采集测试方案.md"
func TestCompliance_BACnet_Isolation(t *testing.T) {
	mock := &RealWorldMockClient{
		SmartMockClient: SmartMockClient{
			Devices: map[int]btypes.Device{
				2228316: {DeviceID: 2228316, Ip: "192.168.3.110", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 110, 0xBA, 0xC0}}},
				2228317: {DeviceID: 2228317, Ip: "192.168.3.111", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 111, 0xBA, 0xC0}}},
				2228318: {DeviceID: 2228318, Ip: "192.168.3.112", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 112, 0xBA, 0xC0}}},
				2228319: {DeviceID: 2228319, Ip: "192.168.3.113", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 3, 113, 0xBA, 0xC0}}},
			},
			Values: map[string]interface{}{
				"2228316:2:1": float32(316.00),
				"2228317:2:1": float32(317.00),
				"2228318:2:1": float32(318.00),
				"2228319:2:1": float32(319.00),
			},
		},
		Delays: map[int]time.Duration{
			2228319: 2 * time.Second,
		},
		Errors: map[int]error{
			2228319: context.DeadlineExceeded,
		},
		CallCounter: make(map[int]int),
	}

	d := NewBACnetDriver().(*BACnetDriver)
	d.clientFactory = func(cb *bacnetlib.ClientBuilder) (Client, error) {
		return mock, nil
	}
	d.Init(model.DriverConfig{Config: map[string]any{"ip": "0.0.0.0"}})
	d.Connect(context.Background())
	defer d.Disconnect()

	devices := []struct {
		ID   int
		Name string
	}{
		{2228316, "bacnet-16"},
		{2228317, "bacnet-17"},
		{2228318, "bacnet-18"},
		{2228319, "Room_FC_2014_19"},
	}

	for _, dev := range devices {
		d.SetDeviceConfig(map[string]any{
			"instance_id":         dev.ID,
			"ip":                  fmt.Sprintf("192.168.3.%d", dev.ID%100),
			"_internal_device_id": dev.Name,
		})
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()

	p16 := []model.Point{{ID: "P16", DeviceID: "bacnet-16", Address: "AnalogValue:1", DataType: "float32"}}
	p17 := []model.Point{{ID: "P17", DeviceID: "bacnet-17", Address: "AnalogValue:1", DataType: "float32"}}
	p18 := []model.Point{{ID: "P18", DeviceID: "bacnet-18", Address: "AnalogValue:1", DataType: "float32"}}
	p19 := []model.Point{{ID: "P19", DeviceID: "Room_FC_2014_19", Address: "AnalogValue:1", DataType: "float32"}}

	t.Log("=== Use Case 1: Normal Read Test ===")

	verifyPoint := func(p []model.Point, expected float32) {
		start := time.Now()
		res, err := d.ReadPoints(ctx, p)
		dur := time.Since(start)

		if err != nil {
			t.Errorf("Device %s read failed: %v", p[0].DeviceID, err)
			return
		}

		val, ok := res[p[0].ID]
		if !ok {
			t.Errorf("Device %s missing point in result", p[0].DeviceID)
			return
		}

		if val.Quality != "Good" {
			t.Errorf("Device %s Quality should be Good, got %s", p[0].DeviceID, val.Quality)
		}
		if val.Value != expected {
			t.Errorf("Device %s Value mismatch: got %v, want %v", p[0].DeviceID, val.Value, expected)
		}
		if dur > 3*time.Second {
			t.Errorf("Device %s read too slow: %v", p[0].DeviceID, dur)
		}
		t.Logf("✅ Device %s Normal Read OK (Time: %v, Quality: %s)", p[0].DeviceID, dur, val.Quality)
	}

	verifyPoint(p16, 316.0)
	verifyPoint(p17, 317.0)
	verifyPoint(p18, 318.0)

	t.Log("=== Use Case 2: Offline Device Error Propagation ===")

	start := time.Now()
	_, err := d.ReadPoints(ctx, p19)
	dur := time.Since(start)
	if err == nil {
		t.Errorf("Expected error for offline device 19")
	} else {
		t.Logf("✅ Device 19 returned error as expected: %v (Time: %v)", err, dur)
	}
	if dur > 8*time.Second {
		t.Errorf("Offline device read exceeded 8s budget: %v", dur)
	}

	verifyPoint(p16, 316.0)
	verifyPoint(p17, 317.0)
	verifyPoint(p18, 318.0)

	t.Log("=== Use Case 3: Healthy Devices Unaffected ===")
	verifyPoint(p16, 316.0)

	t.Log("=== Compliance Test Summary ===")
	t.Log("1. All online devices (16,17,18) returned Good quality.")
	t.Log("2. Offline device (19) returned an error to ScanEngine.")
	t.Log("3. Offline device (19) did not impact (16,17,18).")
	t.Log("4. Reads stayed within the 3s budget.")
}
