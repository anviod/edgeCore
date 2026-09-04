//go:build integration

package bacnet

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/edgeCore/internal/model"
)

type portTrackingMockClient struct {
	SmartMockClient
	mu          sync.Mutex
	whoIsPorts  []int
	whoIsDestIP string
}

func (m *portTrackingMockClient) WhoIs(wh *WhoIsOpts) ([]btypes.Device, error) {
	m.mu.Lock()
	if wh.Destination != nil && len(wh.Destination.Mac) >= 6 {
		m.whoIsDestIP = net.IP([]byte{wh.Destination.Mac[0], wh.Destination.Mac[1], wh.Destination.Mac[2], wh.Destination.Mac[3]}).String()
		m.whoIsPorts = append(m.whoIsPorts, int(wh.Destination.Mac[4])<<8|int(wh.Destination.Mac[5]))
	} else {
		m.whoIsPorts = append(m.whoIsPorts, 0)
	}
	m.mu.Unlock()

	if wh.Destination != nil && len(wh.Destination.Mac) >= 6 {
		port := int(wh.Destination.Mac[4])<<8 | int(wh.Destination.Mac[5])
		if port == 49152 {
			return nil, nil
		}
	}

	return m.SmartMockClient.WhoIs(wh)
}

func (m *portTrackingMockClient) SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error {
	return nil
}
func (m *portTrackingMockClient) CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error {
	return nil
}

func TestWhoIsDiscoverDevice_BroadcastPreservesPort(t *testing.T) {
	newPort := 54321
	addr := datalink.IPPortToAddress(net.ParseIP("192.168.3.119"), newPort)
	mock := &portTrackingMockClient{
		SmartMockClient: SmartMockClient{
			Devices: map[int]btypes.Device{
				2228319: {DeviceID: 2228319, Ip: "192.168.3.119", Port: newPort, Addr: *addr},
			},
		},
	}

	d := NewBACnetDriver().(*BACnetDriver)
	found, ok := d.whoIsDiscoverDevice(mock, 2228319, "192.168.3.119", 49152)
	if !ok {
		t.Fatal("expected discovery via broadcast WhoIs")
	}
	// The ephemeral port must be preserved from the I-Am response,
	// not overwritten by a unicast Destination address.
	if devicePortFromAddr(found) != newPort {
		t.Fatalf("expected discovered port %d, got %d", newPort, devicePortFromAddr(found))
	}

	// Verify broadcast WhoIs was used (Destination == nil, port tracked as 0)
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.whoIsPorts) == 0 || mock.whoIsPorts[0] != 0 {
		t.Fatalf("expected broadcast WhoIs (port 0), got %v", mock.whoIsPorts)
	}
}

func TestProbeDevice_UpdatesStalePort(t *testing.T) {
	newPort := 54321
	addr := datalink.IPPortToAddress(net.ParseIP("192.168.3.119"), newPort)
	mock := &CoverageMockClient{
		WhoIsFunc: func(wh *WhoIsOpts) ([]btypes.Device, error) {
			return []btypes.Device{{
				DeviceID: 2228319,
				Ip:       "192.168.3.119",
				Port:     newPort,
				Addr:     *addr,
			}}, nil
		},
	}

	d := NewBACnetDriver().(*BACnetDriver)
	d.clientFactory = func(cb *bacnetlib.ClientBuilder) (Client, error) { return mock, nil }
	d.Init(model.DriverConfig{Config: map[string]any{"ip": "0.0.0.0"}})
	d.Connect(context.Background())
	defer d.Disconnect()

	d.mu.Lock()
	d.deviceContexts[2228319] = &DeviceContext{
		State: DeviceStateOnline,
		Config: DeviceConfig{
			DeviceID: 2228319,
			IP:       "192.168.3.119",
			Port:     49152,
		},
		DeviceKey:     "bacnet-2228319",
		LastDiscovery: time.Now().Add(-2 * time.Minute),
		Scheduler:     NewPointScheduler(mock, btypes.Device{}, 20, 10*time.Millisecond, 10*time.Second, false),
	}
	d.mu.Unlock()

	if !d.probeDevice(mock, 2228319, "192.168.3.119", 49152) {
		t.Fatal("probeDevice should succeed after port change")
	}

	d.mu.Lock()
	ctx := d.deviceContexts[2228319]
	d.mu.Unlock()

	if ctx.Config.Port != newPort {
		t.Fatalf("expected config port %d, got %d", newPort, ctx.Config.Port)
	}
	if ctx.Scheduler == nil {
		t.Fatal("scheduler should be rebuilt after probe")
	}
}

func TestReadPoints_TriggersRecoveryAfterFailures(t *testing.T) {
	newPort := 54321
	addr := datalink.IPPortToAddress(net.ParseIP("192.168.3.119"), newPort)
	probeCh := make(chan struct{}, 1)

	mock := &CoverageMockClient{}
	mock.WhoIsFunc = func(wh *WhoIsOpts) ([]btypes.Device, error) {
		select {
		case probeCh <- struct{}{}:
		default:
		}
		return []btypes.Device{{
			DeviceID: 2228319,
			Ip:       "192.168.3.119",
			Port:     newPort,
			Addr:     *addr,
		}}, nil
	}
	mock.ReadMultiPropertyFunc = func(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error) {
		return btypes.MultiplePropertyData{}, fmt.Errorf("receive timed out")
	}
	mock.SmartMockClient.Devices = map[int]btypes.Device{
		2228319: {DeviceID: 2228319, Ip: "192.168.3.119", Port: 49152, Addr: *datalink.IPPortToAddress(net.ParseIP("192.168.3.119"), 49152)},
	}
	mock.SmartMockClient.Values = map[string]interface{}{"2228319:2:1": float32(319.0)}

	d := NewBACnetDriver().(*BACnetDriver)
	d.clientFactory = func(cb *bacnetlib.ClientBuilder) (Client, error) { return mock, nil }
	d.Init(model.DriverConfig{Config: map[string]any{"ip": "0.0.0.0"}})
	d.Connect(context.Background())
	defer d.Disconnect()

	staleAddr := datalink.IPPortToAddress(net.ParseIP("192.168.3.119"), 49152)
	d.mu.Lock()
	d.deviceContexts[2228319] = &DeviceContext{
		State:         DeviceStateOnline,
		DeviceKey:     "bacnet-2228319",
		LastDiscovery: time.Now().Add(-2 * time.Minute),
		Config:        DeviceConfig{DeviceID: 2228319, IP: "192.168.3.119", Port: 49152},
		Device:        btypes.Device{DeviceID: 2228319, Addr: *staleAddr},
		Scheduler:     NewPointScheduler(mock, btypes.Device{DeviceID: 2228319, Addr: *staleAddr}, 20, 10*time.Millisecond, 10*time.Second, false),
	}
	d.idMap["bacnet-2228319"] = 2228319
	d.mu.Unlock()

	points := []model.Point{{ID: "P1", DeviceID: "bacnet-2228319", Address: "AnalogValue:1", DataType: "float32"}}
	for i := 0; i < 3; i++ {
		_, _ = d.ReadPoints(context.Background(), points)
	}

	d.mu.Lock()
	failures := d.deviceContexts[2228319].ConsecutiveFailures
	d.mu.Unlock()
	if failures < 3 {
		t.Fatalf("expected at least 3 consecutive failures before recovery, got %d", failures)
	}

	select {
	case <-probeCh:
	case <-time.After(5 * time.Second):
		t.Fatal("expected recovery probe after repeated read failures")
	}
}

func (m *portTrackingMockClient) WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error) {
	return btypes.COVNotification{}, fmt.Errorf("not implemented")
}
