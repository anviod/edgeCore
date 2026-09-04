package core

import (
	"sync"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestChannelManager_AddDevice_RejectsDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dup-dev"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "Dup Device Channel",
		Protocol: addChannelMockProtocol,
		Config:   map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	dev := &model.Device{ID: "dev-dup", Name: "Device"}
	if err := cm.AddDevice(channelID, dev); err != nil {
		t.Fatalf("first AddDevice: %v", err)
	}
	if err := cm.AddDevice(channelID, dev); err == nil {
		t.Fatal("expected duplicate device error")
	}
}

func TestChannelManager_AddDevice_RejectsMissingChannel(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	err := cm.AddDevice("missing-channel", &model.Device{ID: "dev-1", Name: "Device"})
	if err == nil {
		t.Fatal("expected channel not found error")
	}
}

func TestChannelManager_AddPoint_RejectsMissingDevice(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-no-dev"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "No Device Channel",
		Protocol: addChannelMockProtocol,
		Config:   map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	err := cm.AddPoint(channelID, "missing-device", &model.Point{
		ID: "pt-1", Name: "Point", Address: "0", DataType: "int16",
	})
	if err == nil {
		t.Fatal("expected device not found error")
	}
}

func TestChannelManager_AddPoint_RejectsDuplicate(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	defer cm.cancel()

	channelID := "ch-dup-pt"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "Dup Point Channel",
		Protocol: addChannelMockProtocol,
		Config:   map[string]any{},
		Devices: []model.Device{{
			ID:     "dev-1",
			Name:   "Device",
			Points: []model.Point{{ID: "pt-1", Name: "Existing", Address: "0", DataType: "int16"}},
		}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	err := cm.AddPoint(channelID, "dev-1", &model.Point{
		ID: "pt-1", Name: "Duplicate", Address: "1", DataType: "int16",
	})
	if err == nil {
		t.Fatal("expected duplicate point error")
	}
}

func TestChannelManager_AddPoint_ProtocolValidation(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		address  string
		dataType string
	}{
		{name: "modbus invalid address", protocol: "modbus-tcp", address: "not-a-number", dataType: "int16"},
		{name: "bacnet invalid address", protocol: "bacnet-ip", address: "bad-format", dataType: "float32"},
		{name: "omron invalid address", protocol: "omron-fins", address: "X100", dataType: "INT16"},
		{name: "knx invalid address", protocol: "knxnet-ip", address: "invalid", dataType: "int16"},
		{name: "mitsubishi invalid address", protocol: "mitsubishi-slmp", address: "123", dataType: "INT16"},
		{name: "dlt645 invalid address", protocol: "dlt645", address: "no-hash", dataType: "uint16"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewChannelManager(nil, nil)
			defer cm.cancel()

			channelID := "ch-" + tc.name
			cm.channels[channelID] = &model.Channel{
				ID:       channelID,
				Name:     tc.name,
				Protocol: tc.protocol,
				Devices: []model.Device{{
					ID:     "dev-1",
					Name:   "Device",
					Points: []model.Point{},
				}},
			}
			cm.driverMus[channelID] = new(sync.Mutex)

			err := cm.AddPoint(channelID, "dev-1", &model.Point{
				ID: tc.name, Name: tc.name, Address: tc.address, DataType: tc.dataType,
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
