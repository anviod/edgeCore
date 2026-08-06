package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestChannelManager_AddDeviceAndPoint_PersistViaSaveFunc(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{})
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	channelID := "ch-persist"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "Persist Channel",
		Protocol: "modbus-tcp",
		Enable:   false,
		Config:   map[string]any{"url": "tcp://127.0.0.1:502"},
		Devices:  []model.Device{},
	}
	cm.driverMus[channelID] = &sync.Mutex{}

	dev := &model.Device{
		ID:       "dev-persist",
		Name:     "Persist Device",
		Enable:   true,
		Interval: model.Duration(1000 * time.Millisecond),
		Config:   map[string]any{"slave_id": 1},
		Points:   []model.Point{},
	}
	if err := cm.AddDevice("ch-persist", dev); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}

	point := &model.Point{
		ID:       "pt-persist",
		Name:     "Temperature",
		Address:  "10",
		DataType: "int16",
	}
	if err := cm.AddPoint("ch-persist", "dev-persist", point); err != nil {
		t.Fatalf("AddPoint: %v", err)
	}

	<-saveCh // wait for async save
	if len(saved) == 0 {
		t.Fatal("saveFunc was never called")
	}
	last := saved
	if len(last) != 1 {
		t.Fatalf("expected 1 channel in save, got %d", len(last))
	}
	if len(last[0].Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(last[0].Devices))
	}
	if len(last[0].Devices[0].Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(last[0].Devices[0].Points))
	}
	if last[0].Devices[0].Points[0].ID != "pt-persist" {
		t.Errorf("point id: %s", last[0].Devices[0].Points[0].ID)
	}
}

func TestPointUpdateRequiresDeviceRestart(t *testing.T) {
	base := model.Point{
		ID: "hr_0", Address: "0", DataType: "int16", ReadWrite: "R",
	}
	rwOnly := base
	rwOnly.ReadWrite = "RW"
	if pointUpdateRequiresDeviceRestart(base, rwOnly) {
		t.Fatal("readwrite-only change should not require device restart")
	}
	addrChanged := base
	addrChanged.Address = "1"
	if !pointUpdateRequiresDeviceRestart(base, addrChanged) {
		t.Fatal("address change should require device restart")
	}
}

func TestChannelManager_UpdatePoint_NotifiesTopology(t *testing.T) {
	var notified int
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	defer cm.cancel()
	cm.SetTopologyChangeHandler(func() {
		notified++
	})

	channelID := "ch-topology"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "Topology Channel",
		Protocol: "modbus-tcp",
		Enable:   false,
		Devices: []model.Device{{
			ID:     "dev-1",
			Name:   "Device 1",
			Enable: true,
			Points: []model.Point{{
				ID:        "hr_0",
				Name:      "HR 0",
				Address:   "0",
				DataType:  "int16",
				ReadWrite: "R",
			}},
		}},
	}
	cm.driverMus[channelID] = &sync.Mutex{}

	updated := &model.Point{
		ID:        "hr_0",
		Name:      "HR 0",
		Address:   "0",
		DataType:  "int16",
		ReadWrite: "RW",
	}
	restarted, err := cm.UpdatePoint(channelID, "dev-1", updated)
	if err != nil {
		t.Fatalf("UpdatePoint: %v", err)
	}
	if restarted {
		t.Fatal("expected no southbound device restart for readwrite-only update")
	}
	time.Sleep(700 * time.Millisecond)
	if notified != 1 {
		t.Fatalf("expected topology notification once, got %d", notified)
	}
	if cm.channels[channelID].Devices[0].Points[0].ReadWrite != "RW" {
		t.Fatalf("point readwrite not updated: %s", cm.channels[channelID].Devices[0].Points[0].ReadWrite)
	}
}

func TestChannelManager_AddDevice_RejectsEmptyID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-empty-id"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "Channel",
		Protocol: "modbus-tcp",
		Devices:  []model.Device{},
	}
	cm.driverMus[channelID] = &sync.Mutex{}

	err := cm.AddDevice(channelID, &model.Device{ID: "", Name: ""})
	if err == nil {
		t.Fatal("expected error for empty device ID")
	}
}

func TestChannelManager_AddPoint_PersistsWhenIntervalInvalid(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{})
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	channelID := "ch-interval"
	cm.channels[channelID] = &model.Channel{
		ID:       channelID,
		Name:     "Channel",
		Protocol: "modbus-tcp",
		Enable:   true,
		Devices: []model.Device{
			{
				ID:       "dev-1",
				Name:     "Device",
				Enable:   true,
				Interval: 0,
				Points:   []model.Point{},
			},
		},
	}
	cm.driverMus[channelID] = &sync.Mutex{}

	point := &model.Point{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"}
	if err := cm.AddPoint(channelID, "dev-1", point); err != nil {
		t.Fatalf("AddPoint: %v", err)
	}

	<-saveCh // wait for async save
	if len(saved) == 0 || len(saved[0].Devices[0].Points) != 1 {
		t.Fatalf("expected point to be saved despite invalid interval, saved=%v", saved)
	}
}
