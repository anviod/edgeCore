package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestVirtualShadowManager_SearchSourceDevices(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	cm.channels["ch1"] = &model.Channel{
		ID:   "ch1",
		Name: "Modbus主站",
		Devices: []model.Device{
			{ID: "slave-1", Name: "泵房从站", Points: []model.Point{{ID: "temp"}}},
			{ID: "slave-2", Name: "空调机组", Points: []model.Point{{ID: "hum"}}},
		},
	}
	mgr := NewVirtualShadowManager(nil, cm, nil, nil)

	results := mgr.SearchSourceDevices("泵房", "", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 device, got %d", len(results))
	}
	if results[0].DeviceID != "slave-1" {
		t.Fatalf("unexpected device: %s", results[0].DeviceID)
	}

	empty := mgr.SearchSourceDevices("", "", 10)
	if len(empty) != 0 {
		t.Fatalf("empty query without channel should return no results")
	}

	byChannel := mgr.SearchSourceDevices("", "ch1", 10)
	if len(byChannel) != 2 {
		t.Fatalf("channel-only list expected 2 devices, got %d", len(byChannel))
	}
}

func TestVirtualShadowManager_ListDevicePointSources(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	cm.channels["ch1"] = &model.Channel{
		ID:   "ch1",
		Name: "Ch1",
		Devices: []model.Device{
			{
				ID:   "dev1",
				Name: "Dev1",
				Points: []model.Point{
					{ID: "temp", Name: "温度"},
					{ID: "press", Name: "压力"},
				},
			},
		},
	}
	mgr := NewVirtualShadowManager(nil, cm, nil, nil)

	all, err := mgr.ListDevicePointSources("ch1", "dev1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 points, got %d", len(all))
	}

	filtered, err := mgr.ListDevicePointSources("ch1", "dev1", "温度")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].PointID != "temp" {
		t.Fatalf("unexpected filter result: %+v", filtered)
	}
}
