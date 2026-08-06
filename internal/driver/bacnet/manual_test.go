//go:build manual

package bacnet

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// TestBACnet_ManualWrite tests writing to a real BACnet device.
// Run with: go test -tags=manual -v -run TestBACnet_ManualWrite ./internal/driver/bacnet
func TestBACnet_ManualWrite(t *testing.T) {
	// Configuration from user's environment
	channelConfig := model.DriverConfig{
		Config: map[string]any{
			"ip":   "192.168.3.112",
			"port": 47809, // Use a different port to avoid conflict if gateway is running
		},
	}
	deviceConfig := map[string]any{
		"device_id": 2228317,
		"ip":        "192.168.3.112",
		"port":      47808,
	}

	// Initialize Driver
	d := NewBACnetDriver().(*BACnetDriver)

	err := d.Init(channelConfig)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Connect
	fmt.Println("Connecting...")
	err = d.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer d.Disconnect()

	// Set Device Config to trigger discovery
	fmt.Println("Setting Device Config & Discovering...")
	err = d.SetDeviceConfig(deviceConfig)
	if err != nil {
		t.Fatalf("SetDeviceConfig failed: %v", err)
	}

	// Give it a moment to discover
	time.Sleep(2 * time.Second)

	// Verify scheduler
	if d.scheduler == nil {
		t.Fatalf("Scheduler not initialized. Device discovery might have failed.")
	}
	fmt.Printf("Device discovered. Target Device ID: %d\n", d.targetDevice.DeviceID)

	// Define Point to Write
	// AnalogValue:2
	point := model.Point{
		ID:        "AnalogValue_2",
		Name:      "Setpoint.2",
		Address:   "AnalogValue:2",
		DataType:  "float32",
		ReadWrite: "RW",
	}

	// Value to Write (String format as per user report)
	writeVal := "2175"
	expectedVal := float32(2175.0)

	fmt.Printf("Writing %s to %s...\n", writeVal, point.Address)
	err = d.WritePoint(context.Background(), point, writeVal)
	if err != nil {
		t.Errorf("WritePoint failed: %v", err)
	} else {
		fmt.Println("WritePoint successful!")
	}

	// Read back to verify
	time.Sleep(1 * time.Second)
	fmt.Println("Reading back...")
	results, err := d.ReadPoints(context.Background(), []model.Point{point})
	if err != nil {
		t.Errorf("ReadPoints failed: %v", err)
	} else {
		if val, ok := results[point.ID]; ok {
			fmt.Printf("Read value: %v\n", val.Value)
			// Check with tolerance for float
			vFloat, ok := val.Value.(float32)
			if !ok {
				// It might come back as float64 depending on implementation, but let's see
				if vF64, ok2 := val.Value.(float64); ok2 {
					vFloat = float32(vF64)
				}
			}

			diff := vFloat - expectedVal
			if diff < 0 {
				diff = -diff
			}
			if diff < 0.1 {
				fmt.Println("Readback matches written value!")
			} else {
				fmt.Printf("Readback value %v (Expected %v)\n", val.Value, expectedVal)
			}
		} else {
			t.Errorf("Point not found in read results")
		}
	}
}
