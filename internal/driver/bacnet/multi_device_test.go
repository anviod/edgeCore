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

// MultiDeviceMockClient implements Client interface for testing
type MultiDeviceMockClient struct {
	devices map[int]btypes.Device
	values  map[int]map[string]any // DeviceID -> ObjectKey -> Value
}

func (m *MultiDeviceMockClient) Close() error    { return nil }
func (m *MultiDeviceMockClient) IsRunning() bool { return true }
func (m *MultiDeviceMockClient) ClientRun()      {}

func (m *MultiDeviceMockClient) WhoIs(wh *WhoIsOpts) ([]btypes.Device, error) {
	var found []btypes.Device
	// wh.Low/High might be 0 if not set, or specific IDs
	low := wh.Low
	high := wh.High

	// If searching for specific device (Low==High)
	for id, dev := range m.devices {
		if low != 0 && high != 0 {
			if id >= low && id <= high {
				found = append(found, dev)
			}
		} else {
			// Broadcast or wild
			found = append(found, dev)
		}
	}
	return found, nil
}

// SubscribeCOV registers for COV notifications (stub for external lib compatibility)
func (m *MultiDeviceMockClient) SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error {
	return nil
}

// CancelSubscribeCOV cancels a COV subscription (stub for external lib compatibility)
func (m *MultiDeviceMockClient) CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error {
	return nil
}

func (m *MultiDeviceMockClient) ReadProperty(dest btypes.Device, rp btypes.PropertyData) (btypes.PropertyData, error) {
	// Simple mock for ReadProperty
	return rp, nil
}

// We need to implement ReadMultiProperty because PointScheduler uses it likely
func (m *MultiDeviceMockClient) ReadMultiProperty(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error) {
	// Mock response
	// rp.Objects contains the list of objects to read
	// We need to fill rp.Objects[i].Properties[j].Data

	devID := int(dev.DeviceID)

	for i, obj := range rp.Objects {
		for j := range obj.Properties {
			// Construct key: Type:Instance
			key := fmt.Sprintf("%d:%d", obj.ID.Type, obj.ID.Instance)
			if devVals, ok := m.values[devID]; ok {
				if val, ok := devVals[key]; ok {
					rp.Objects[i].Properties[j].Data = val
				} else {
					rp.Objects[i].Properties[j].Data = float32(0.0) // Default
				}
			}
		}
	}
	return rp, nil
}

// Stubs for other interface methods
func (m *MultiDeviceMockClient) WhatIsNetworkNumber() []*btypes.Address           { return nil }
func (m *MultiDeviceMockClient) IAm(dest btypes.Address, iam btypes.IAm) error    { return nil }
func (m *MultiDeviceMockClient) WhoIsRouterToNetwork() (resp *[]btypes.Address)   { return nil }
func (m *MultiDeviceMockClient) Objects(dev btypes.Device) (btypes.Device, error) { return dev, nil }
func (m *MultiDeviceMockClient) WriteProperty(dest btypes.Device, wp btypes.PropertyData) error {
	return nil
}
func (m *MultiDeviceMockClient) WriteMultiProperty(dev btypes.Device, wp btypes.MultiplePropertyData) error {
	return nil
}

func (m *MultiDeviceMockClient) ReadPropertyWithTimeout(dest btypes.Device, rp btypes.PropertyData, timeout time.Duration) (btypes.PropertyData, error) {
	return m.ReadProperty(dest, rp)
}

func (m *MultiDeviceMockClient) ReadMultiPropertyWithTimeout(dev btypes.Device, rp btypes.MultiplePropertyData, timeout time.Duration) (btypes.MultiplePropertyData, error) {
	return m.ReadMultiProperty(dev, rp)
}

func TestBACnet_MultiDevice_Scheduling(t *testing.T) {
	// 1. Setup Mock Client
	mockClient := &MultiDeviceMockClient{
		devices: make(map[int]btypes.Device),
		values:  make(map[int]map[string]any),
	}

	// Device A: 2228316
	mockClient.devices[2228316] = btypes.Device{
		DeviceID: 2228316,
		Addr:     btypes.Address{Mac: []byte{192, 168, 3, 106, 0xBA, 0xC0}, MacLen: 6}, // 192.168.3.112:47808
	}
	mockClient.values[2228316] = map[string]any{
		"0:0": float32(16.5), // AnalogInput:0
	}

	// Device B: 2228317
	mockClient.devices[2228317] = btypes.Device{
		DeviceID: 2228317,
		Addr:     btypes.Address{Mac: []byte{192, 168, 3, 106, 0xBA, 0xC0}, MacLen: 6}, // Same IP/Port
	}
	mockClient.values[2228317] = map[string]any{
		"0:0": float32(17.5), // AnalogInput:0
	}

	// 2. Setup Driver
	d := NewBACnetDriver().(*BACnetDriver)
	// Inject Mock Factory
	d.clientFactory = func(cb *bacnetlib.ClientBuilder) (Client, error) {
		return mockClient, nil
	}

	// Init with generic config
	err := d.Init(model.DriverConfig{
		Config: map[string]any{
			"interface_ip": "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Connect (starts client)
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer d.Disconnect()

	// 3. Test Multi-Device Read

	// Configure Device A
	fmt.Println("Configuring Device 2228316...")
	d.SetDeviceConfig(map[string]any{"instance_id": 2228316, "_internal_device_id": "2228316"})
	time.Sleep(50 * time.Millisecond) // Wait for discovery

	pointsA := []model.Point{{ID: "p1", DeviceID: "2228316", Address: "AnalogInput:0", DataType: "float32"}}

	// Wait for cache population
	var resA map[string]model.Value
	for i := 0; i < 5; i++ {
		resA, err = d.ReadPoints(context.Background(), pointsA)
		if err == nil && len(resA) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err != nil {
		t.Fatalf("ReadPoints A failed: %v", err)
	}
	if val, ok := resA["p1"]; !ok || val.Value != float32(16.5) {
		t.Errorf("Device A Read Mismatch: got %v, want 16.5", val)
	} else {
		fmt.Printf("Device A Read Success: %v\n", val.Value)
	}

	// Configure Device B
	fmt.Println("Configuring Device 2228317...")
	d.SetDeviceConfig(map[string]any{"instance_id": 2228317, "_internal_device_id": "2228317"})
	time.Sleep(50 * time.Millisecond)

	pointsB := []model.Point{{ID: "p1", DeviceID: "2228317", Address: "AnalogInput:0", DataType: "float32"}}

	// Wait for cache population
	var resB map[string]model.Value
	for i := 0; i < 5; i++ {
		resB, err = d.ReadPoints(context.Background(), pointsB)
		if err == nil && len(resB) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err != nil {
		t.Fatalf("ReadPoints B failed: %v", err)
	}
	if val, ok := resB["p1"]; !ok || val.Value != float32(17.5) {
		t.Errorf("Device B Read Mismatch: got %v, want 17.5", val)
	} else {
		fmt.Printf("Device B Read Success: %v\n", val.Value)
	}

	// 4. Switch back to A without re-discovery (should be fast/cached)
	// Note: SetDeviceConfig will check if context exists.
	fmt.Println("Switching back to Device 2228316...")

	// No need to call SetDeviceConfig again if using unique internal IDs, but test logic implies "switching focus"
	// Actually, ReadPoints uses DeviceID in point.

	resA2, err := d.ReadPoints(context.Background(), pointsA)
	if err != nil {
		t.Fatalf("ReadPoints A (2nd time) failed: %v", err)
	}
	if val, ok := resA2["p1"]; !ok || val.Value != float32(16.5) {
		t.Errorf("Device A (2nd time) Read Mismatch: got %v, want 16.5", val)
	} else {
		fmt.Printf("Device A (2nd time) Read Success: %v\n", val.Value)
	}

	// Verify that we have 2 contexts
	d.mu.Lock()
	ctxCount := len(d.deviceContexts)
	d.mu.Unlock()
	if ctxCount != 2 {
		t.Errorf("Expected 2 device contexts, got %d", ctxCount)
	}
}

func (m *MultiDeviceMockClient) WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error) {
	return btypes.COVNotification{}, fmt.Errorf("not implemented")
}
