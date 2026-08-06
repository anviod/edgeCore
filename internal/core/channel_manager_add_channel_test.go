package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

const addChannelMockProtocol = "add-channel-mock"

type addChannelMockDriver struct {
	initErr error
}

func (m *addChannelMockDriver) Init(_ model.DriverConfig) error { return m.initErr }
func (m *addChannelMockDriver) Connect(_ context.Context) error { return nil }
func (m *addChannelMockDriver) Disconnect() error               { return nil }
func (m *addChannelMockDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, nil
}
func (m *addChannelMockDriver) WritePoint(_ context.Context, _ model.Point, _ any) error { return nil }
func (m *addChannelMockDriver) Health() driver.HealthStatus                              { return driver.HealthStatusGood }
func (m *addChannelMockDriver) SetSlaveID(_ uint8) error                                 { return nil }
func (m *addChannelMockDriver) SetDeviceConfig(_ map[string]any) error                   { return nil }
func (m *addChannelMockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func init() {
	driver.RegisterDriver(addChannelMockProtocol, func() driver.Driver {
		return &addChannelMockDriver{}
	})
}

func TestChannelManager_AddChannel_RejectsDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-dup",
		Name:     "Duplicate",
		Protocol: addChannelMockProtocol,
		Config:   map[string]any{},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("first AddChannel: %v", err)
	}
	err := cm.AddChannel(ch)
	if err == nil {
		t.Fatal("expected duplicate channel error")
	}
}

func TestChannelManager_AddChannel_RejectsUnknownProtocol(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "ch-unknown",
		Name:     "Unknown",
		Protocol: "not-a-real-protocol",
	})
	if err == nil {
		t.Fatal("expected unknown protocol error")
	}
}

func TestChannelManager_AddChannel_RejectsEmptyID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "",
		Name:     "",
		Protocol: addChannelMockProtocol,
	})
	if err == nil {
		t.Fatal("expected empty ID error")
	}
}

func TestChannelManager_AddChannel_RejectsDriverInitFailure(t *testing.T) {
	const failProtocol = "add-channel-init-fail"
	driver.RegisterDriver(failProtocol, func() driver.Driver {
		return &addChannelMockDriver{initErr: fmt.Errorf("invalid config")}
	})

	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddChannel(&model.Channel{
		ID:       "ch-init-fail",
		Name:     "Init Fail",
		Protocol: failProtocol,
		Config:   map[string]any{},
	})
	if err == nil {
		t.Fatal("expected init failure error")
	}
}

func TestChannelManager_AddChannel_NilConfig(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-nil-config",
		Name:     "Nil Config",
		Protocol: addChannelMockProtocol,
		Config:   nil,
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel with nil config: %v", err)
	}
}

func TestChannelManager_AddChannel_WithDevices(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{}, 1)
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-with-devices",
		Name:     "With Devices",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:     "dev-1",
				Name:   "Device 1",
				Enable: true,
				Points: []model.Point{
					{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"},
				},
			},
		},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	<-saveCh // wait for async save
	if len(saved) != 1 || len(saved[0].Devices) != 1 || len(saved[0].Devices[0].Points) != 1 {
		t.Fatalf("unexpected saved state: %+v", saved)
	}
	if _, ok := cm.drivers["ch-with-devices"]; !ok {
		t.Fatal("driver not registered")
	}
	if cm.driverMus["ch-with-devices"] == nil {
		t.Fatal("driver mutex not registered")
	}
}

func TestChannelManager_AddChannel_UsesNameAsID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		Name:     "named-channel",
		Protocol: addChannelMockProtocol,
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if ch.ID != "named-channel" {
		t.Fatalf("expected ID from name, got %q", ch.ID)
	}
}

func TestChannelManager_AddChannel_ConcurrentDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	ch := &model.Channel{
		ID:       "ch-race",
		Name:     "Race",
		Protocol: addChannelMockProtocol,
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- cm.AddChannel(ch)
		}()
	}
	wg.Wait()
	close(errCh)

	var okCount, failCount int
	for err := range errCh {
		if err == nil {
			okCount++
		} else {
			failCount++
		}
	}
	if okCount != 1 || failCount != 1 {
		t.Fatalf("expected exactly one success and one failure, got ok=%d fail=%d", okCount, failCount)
	}
}
