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

// IsolationMockClient simulates devices with configurable latency/errors
type IsolationMockClient struct {
	SmartMockClient
	Delays      map[int]time.Duration
	Errors      map[int]error
	CallCounter map[int]int
	mu          sync.Mutex
}

func (m *IsolationMockClient) ReadMultiProperty(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error) {
	m.mu.Lock()
	m.CallCounter[dev.DeviceID]++
	delay := m.Delays[dev.DeviceID]
	err := m.Errors[dev.DeviceID]
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	if err != nil {
		return btypes.MultiplePropertyData{}, err
	}

	return m.SmartMockClient.ReadMultiProperty(dev, rp)
}

func (m *IsolationMockClient) ReadProperty(dest btypes.Device, rp btypes.PropertyData) (btypes.PropertyData, error) {
	return m.ReadPropertyWithTimeout(dest, rp, 10*time.Second)
}

func (m *IsolationMockClient) ReadPropertyWithTimeout(dest btypes.Device, rp btypes.PropertyData, timeout time.Duration) (btypes.PropertyData, error) {
	m.mu.Lock()
	delay := m.Delays[dest.DeviceID]
	err := m.Errors[dest.DeviceID]
	m.mu.Unlock()

	if delay > 0 {
		if delay > timeout {
			time.Sleep(timeout)
			return rp, fmt.Errorf("receive timed out")
		}
		time.Sleep(delay)
	}
	if err != nil {
		return rp, err
	}
	return m.SmartMockClient.ReadProperty(dest, rp)
}

func (m *IsolationMockClient) ReadMultiPropertyWithTimeout(dev btypes.Device, rp btypes.MultiplePropertyData, timeout time.Duration) (btypes.MultiplePropertyData, error) {
	m.mu.Lock()
	m.CallCounter[dev.DeviceID]++
	delay := m.Delays[dev.DeviceID]
	err := m.Errors[dev.DeviceID]
	m.mu.Unlock()

	if delay > 0 {
		if delay > timeout {
			time.Sleep(timeout)
			return btypes.MultiplePropertyData{}, fmt.Errorf("receive timed out")
		}
		time.Sleep(delay)
	}

	if err != nil {
		return btypes.MultiplePropertyData{}, err
	}

	return m.SmartMockClient.ReadMultiProperty(dev, rp)
}

func (m *IsolationMockClient) WhoIs(wh *WhoIsOpts) ([]btypes.Device, error) {
	return m.SmartMockClient.WhoIs(wh)
}

func (m *IsolationMockClient) SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error {
	return nil
}
func (m *IsolationMockClient) CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error {
	return nil
}

func (m *IsolationMockClient) WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error) {
	return btypes.COVNotification{}, fmt.Errorf("not implemented")
}

func TestDeviceIsolation(t *testing.T) {
	mock := &IsolationMockClient{
		SmartMockClient: SmartMockClient{
			Devices: map[int]btypes.Device{
				1001:    {DeviceID: 1001, Ip: "192.168.1.10", Port: 47808, Addr: btypes.Address{Mac: []byte{192, 168, 1, 10, 0xBA, 0xC0}}},
				2228319: {DeviceID: 2228319, Ip: "192.168.3.112", Port: 57611, Addr: btypes.Address{Mac: []byte{192, 168, 3, 112, 0xE1, 0x0B}}},
			},
			Values: map[string]interface{}{
				"1001:0:1":    float32(100.0),
				"2228319:0:1": float32(319.0),
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

	d.SetDeviceConfig(map[string]any{"instance_id": 1001, "ip": "192.168.1.10", "_internal_device_id": "dev-1001"})
	d.SetDeviceConfig(map[string]any{"instance_id": 2228319, "ip": "192.168.3.112", "_internal_device_id": "dev-bad"})
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	pointsGood := []model.Point{{ID: "P1", DeviceID: "dev-1001", Address: "0:1", DataType: "float32"}}
	pointsBad := []model.Point{{ID: "P2", DeviceID: "dev-bad", Address: "0:1", DataType: "float32"}}

	res, err := d.ReadPoints(ctx, pointsGood)
	if err != nil {
		t.Fatalf("Initial read for good device failed: %v", err)
	}
	if v, ok := res["P1"]; !ok || v.Value != float32(100.0) {
		t.Fatalf("Unexpected good device value: %+v", res)
	}

	mock.mu.Lock()
	mock.Errors[2228319] = fmt.Errorf("timeout")
	mock.Delays[2228319] = 100 * time.Millisecond
	mock.mu.Unlock()

	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()
		start := time.Now()
		_, err := d.ReadPoints(ctx, pointsBad)
		if err == nil {
			errors <- fmt.Errorf("expected error for failing device")
		}
		if time.Since(start) > 3*time.Second {
			errors <- fmt.Errorf("failing device read took too long: %v", time.Since(start))
		}
	}()

	go func() {
		defer wg.Done()
		res, err := d.ReadPoints(ctx, pointsGood)
		if err != nil {
			errors <- fmt.Errorf("good device failed while bad device errored: %v", err)
			return
		}
		if v, ok := res["P1"]; !ok || v.Value != float32(100.0) {
			errors <- fmt.Errorf("good device wrong value: %+v", res)
		}
	}()

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
