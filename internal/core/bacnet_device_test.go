package core

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBACnet_AddDeviceFromScanPayload(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	raw := `[{
		"id": "bacnet-2228316",
		"name": "RoomController.Simulator",
		"interval": "10s",
		"enable": true,
		"config": {
			"bacnet_device_id": 2228316,
			"ip": "192.168.3.106",
			"port": 54103,
			"vendor_name": "Test Vendor",
			"model_name": "Room_FC_2014"
		},
		"points": []
	}]`
	var devices []model.Device
	require.NoError(t, json.Unmarshal([]byte(raw), &devices))
	for i := range devices {
		require.NoError(t, model.EnsureDeviceID(&devices[i]))
		err := cm.AddDevice(channelID, &devices[i])
		require.NoError(t, err, "AddDevice failed")
	}

	got := cm.GetChannelDevices(channelID)
	require.Len(t, got, 1)
	require.Equal(t, "bacnet-2228316", got[0].ID)
	require.Equal(t, 2228316, got[0].Config["bacnet_device_id"])
}

func TestBACnet_AddDeviceFromScanPayload_DuplicateInstance(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	existing := &model.Device{
		ID:       "manual-device-name",
		Name:     "Existing Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	require.NoError(t, cm.AddDevice(channelID, existing))

	scanDev := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Scanned Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
			"ip":               "192.168.3.106",
			"port":             54103,
		},
	}
	err := cm.AddDevice(channelID, scanDev)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Instance ID 2228316 already exists")
}

func TestBACnet_AddDeviceFromScanPayload_DuplicateInstance_BacnetDeviceID(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	existing := &model.Device{
		ID:       "manual-device",
		Name:     "Manual Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	require.NoError(t, cm.AddDevice(channelID, existing))

	scanDev := &model.Device{
		ID:       "bacnet-2228316",
		Name:     "Scanned Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"bacnet_device_id": 2228316,
		},
	}
	err := cm.AddDevice(channelID, scanDev)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Instance ID 2228316 already exists")
}

func TestBACnet_BatchAddDevicesFromScanPayload(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	channelID := "bacnet-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "BACnet",
		Protocol: "bacnet-ip",
		Enable:   false,
		Config:   map[string]any{},
	}
	require.NoError(t, cm.AddChannel(ch))

	for _, id := range []int{2228316, 2228317, 2228318} {
		dev := &model.Device{
			ID:       fmt.Sprintf("bacnet-%d", id),
			Name:     fmt.Sprintf("Device %d", id),
			Interval: model.Duration(10 * time.Second),
			Enable:   true,
			Config: map[string]any{
				"bacnet_device_id": id,
				"ip":               "192.168.3.106",
				"port":             47808,
			},
		}
		require.NoError(t, cm.AddDevice(channelID, dev))
	}

	got := cm.GetChannelDevices(channelID)
	require.Len(t, got, 3)
}
