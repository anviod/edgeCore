package core

import (
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func setupMockChannel(t *testing.T, enable bool) (*ChannelManager, string) {
	t.Helper()
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-crud"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "CRUD Channel",
		Protocol: addChannelMockProtocol,
		Enable:   enable,
		Config:   map[string]any{},
		Devices: []model.Device{{
			ID:       "dev-1",
			Name:     "Device 1",
			Enable:   true,
			Interval: model.Duration(time.Second),
			Config:   map[string]any{"slave_id": 1},
			Points: []model.Point{
				{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"},
				{ID: "pt-2", Name: "Point 2", Address: "1", DataType: "int16"},
			},
		}},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	return cm, channelID
}

func TestChannelManager_UpdateAndRemoveChannel(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)

	updated := &model.Channel{
		ID:       channelID,
		Name:     "Updated Channel",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{"timeout": 5},
		Devices: []model.Device{{
			ID:       "dev-1",
			Name:     "Device 1",
			Enable:   true,
			Interval: model.Duration(time.Second),
			Points:   []model.Point{{ID: "pt-1", Name: "Point 1", Address: "0", DataType: "int16"}},
		}},
	}
	if err := cm.UpdateChannel(updated); err != nil {
		t.Fatalf("UpdateChannel: %v", err)
	}
	ch := cm.GetChannel(channelID)
	if ch == nil || ch.Name != "Updated Channel" {
		t.Fatalf("GetChannel after update = %+v", ch)
	}

	if err := cm.RemoveChannel(channelID); err != nil {
		t.Fatalf("RemoveChannel: %v", err)
	}
	if err := cm.RemoveChannel(channelID); err == nil {
		t.Fatal("expected error removing missing channel")
	}
}

func TestChannelManager_StartStopChannel(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	if err := cm.StartChannel(channelID); err != nil {
		t.Fatalf("StartChannel disabled: %v", err)
	}
	if err := cm.StopChannel(channelID); err != nil {
		t.Fatalf("StopChannel: %v", err)
	}
	if err := cm.StartChannel("missing"); err == nil {
		t.Fatal("expected error starting missing channel")
	}

	cm2, channelID2 := setupMockChannel(t, true)
	if err := cm2.StartChannel(channelID2); err != nil {
		t.Fatalf("StartChannel enabled: %v", err)
	}
	if err := cm2.StopChannel(channelID2); err != nil {
		t.Fatalf("StopChannel enabled: %v", err)
	}
}

func TestChannelManager_GetDriverAndAllPoints(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)

	if cm.GetDriver(channelID) == nil {
		t.Fatal("GetDriver should return mock driver")
	}
	if cm.GetDriver("missing") != nil {
		t.Fatal("missing channel driver should be nil")
	}

	points := cm.GetAllPoints()
	if len(points) != 2 {
		t.Fatalf("GetAllPoints = %d, want 2", len(points))
	}
}

func TestChannelManager_GetDevicePoints_FromShadow(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	sc := NewShadowCore()
	cm.SetShadowCore(sc)

	_, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		ChannelID: channelID,
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "pt-1", Value: 42, Quality: "Good"},
			{PointID: "pt-2", Value: 7, Quality: "Good"},
		},
	})
	if err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	points, err := cm.GetDevicePoints(channelID, "dev-1")
	if err != nil {
		t.Fatalf("GetDevicePoints: %v", err)
	}
	if len(points) != 2 || points[0].Value != 42 {
		t.Fatalf("unexpected points: %+v", points)
	}
}

func TestChannelManager_GetShadowPoint(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	sc := NewShadowCore()
	cm.SetShadowCore(sc)

	_, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		ChannelID: channelID,
		Timestamp: time.Now(),
		Points:    []model.ShadowIngressPoint{{PointID: "pt-1", Value: 99, Quality: "Good"}},
	})
	if err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	pt, err := cm.GetShadowPoint(channelID, "dev-1", "pt-1")
	if err != nil || pt.Value != 99 {
		t.Fatalf("GetShadowPoint = %+v, err=%v", pt, err)
	}
	if _, err := cm.GetShadowPoint(channelID, "dev-1", "missing"); err == nil {
		t.Fatal("expected error for missing point")
	}
}

func TestChannelManager_SetShadowIngress(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	sc := NewShadowCore()
	si := NewShadowIngress(sc, 10, time.Millisecond)
	cm.SetShadowIngress(si)
	if cm.shadowCore != sc {
		t.Fatal("SetShadowIngress should wire shadow core")
	}
	cm.SetShadowIngress(nil)
}

func TestChannelManager_RemovePointAndPoints(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)

	if err := cm.RemovePoint(channelID, "dev-1", "pt-2"); err != nil {
		t.Fatalf("RemovePoint: %v", err)
	}
	dev := cm.GetDevice(channelID, "dev-1")
	if dev == nil || len(dev.Points) != 1 {
		t.Fatalf("after RemovePoint points = %d", len(dev.Points))
	}

	if err := cm.RemovePoints(channelID, "dev-1", []string{"pt-1"}); err != nil {
		t.Fatalf("RemovePoints: %v", err)
	}
	dev = cm.GetDevice(channelID, "dev-1")
	if dev == nil || len(dev.Points) != 0 {
		t.Fatalf("after RemovePoints points = %d", len(dev.Points))
	}
	if err := cm.RemovePoints(channelID, "dev-1", []string{"pt-1"}); err == nil {
		t.Fatal("expected error removing already removed points")
	}
}

func TestChannelManager_RemoveDevices(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)

	if err := cm.AddDevice(channelID, &model.Device{
		ID: "dev-2", Name: "Device 2", Enable: true, Interval: model.Duration(time.Second),
	}); err != nil {
		t.Fatalf("AddDevice: %v", err)
	}
	if err := cm.RemoveDevices(channelID, []string{"dev-1", "dev-2"}); err != nil {
		t.Fatalf("RemoveDevices: %v", err)
	}
	devices := cm.GetChannelDevices(channelID)
	if len(devices) != 0 {
		t.Fatalf("devices after remove = %d", len(devices))
	}
}

func TestChannelManager_AddPoints(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)

	err := cm.AddPoints(channelID, "dev-1", []model.Point{
		{ID: "pt-new", Name: "New", Address: "2", DataType: "int16"},
	})
	if err != nil {
		t.Fatalf("AddPoints: %v", err)
	}
	dev := cm.GetDevice(channelID, "dev-1")
	if dev == nil || len(dev.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(dev.Points))
	}
}

func TestChannelManager_GenerateDeviceRegisterPoints(t *testing.T) {
	cm := NewChannelManager(nil, nil)
	t.Cleanup(func() { cm.cancel() })

	channelID := "ch-modbus-gen"
	cm.channels[channelID] = &model.Channel{
		ID: channelID, Name: "Modbus", Protocol: "modbus-tcp",
		Devices: []model.Device{{ID: "dev-1", Name: "Dev", Points: []model.Point{}}},
	}
	cm.drivers[channelID] = &stubChannelDriver{}
	cm.driverMus[channelID] = &sync.Mutex{}

	dev, err := cm.GenerateDeviceRegisterPoints(channelID, "dev-1", ModbusRegisterGenOptions{
		Start: 0, End: 2, RegisterType: model.RegHolding, FunctionCode: 3, DataType: "int16",
	}, "replace")
	if err != nil {
		t.Fatalf("GenerateDeviceRegisterPoints: %v", err)
	}
	if len(dev.Points) != 3 {
		t.Fatalf("expected 3 generated points, got %d", len(dev.Points))
	}
}

func TestChannelManager_GetScanEngineMetricsAndDiagnostics(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	snap := cm.GetScanEngineMetricsSnapshot()
	if snap == nil {
		t.Fatal("metrics snapshot should not be nil")
	}
	diag := cm.GetDeviceDiagnostics("dev-1")
	if diag["device_id"] != "dev-1" {
		t.Fatalf("diagnostics = %+v", diag)
	}
	_ = channelID
}

func TestChannelManager_SetStatusHandler(t *testing.T) {
	cm := newTestChannelManager()
	called := false
	cm.SetStatusHandler(func(_ string, _ int) { called = true })
	cm.statusHandler("dev-1", 0)
	if !called {
		t.Fatal("status handler not invoked")
	}
}

func TestChannelManager_TryConnectChannel(t *testing.T) {
	cm, channelID := setupMockChannel(t, false)
	cm.tryConnectChannel(channelID)
	cm.tryConnectChannel("missing")
}

func TestNormalizeModbusChannelConfig(t *testing.T) {
	cfg := map[string]any{"url": "127.0.0.1:502"}
	normalizeModbusChannelConfig(cfg)
	if cfg["url"] != "tcp://127.0.0.1:502" {
		t.Fatalf("url = %v", cfg["url"])
	}

	existing := map[string]any{"url": "tcp://host:502"}
	normalizeModbusChannelConfig(existing)
	if existing["url"] != "tcp://host:502" {
		t.Fatalf("existing scheme url = %v", existing["url"])
	}
	normalizeModbusChannelConfig(nil)
}

func TestChannelManager_AutoGenerateModbusPointsFromConfig(t *testing.T) {
	cm := newTestChannelManager()
	dev := &model.Device{
		ID: "dev-auto",
		Config: map[string]any{
			"auto_points_range": "0-2",
		},
	}
	cm.autoGenerateModbusPointsFromConfig(dev)
	if len(dev.Points) != 3 {
		t.Fatalf("auto generated points = %d, want 3", len(dev.Points))
	}
}

func TestChannelManager_AddDevices_SinglePersist(t *testing.T) {
	cm := newTestChannelManager()
	channelID := "ch-batch-dev"
	if err := cm.AddChannel(&model.Channel{
		ID:       channelID,
		Name:     "Batch Devices",
		Protocol: addChannelMockProtocol,
		Enable:   false,
		Config:   map[string]any{},
	}); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	saves := 0
	saveDone := make(chan struct{})
	cm.saveFunc = func(channels []model.Channel) error {
		saves++
		saveDone <- struct{}{}
		return nil
	}

	devices := []model.Device{
		{ID: "dev-a", Name: "A", Enable: true, Interval: model.Duration(time.Second), Config: map[string]any{"slave_id": 1}},
		{ID: "dev-b", Name: "B", Enable: true, Interval: model.Duration(time.Second), Config: map[string]any{"slave_id": 2}},
	}

	created, err := cm.AddDevices(channelID, devices)
	if err != nil {
		t.Fatalf("AddDevices: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %d, want 2", len(created))
	}
	// Wait for async save to complete
	<-saveDone
	if saves != 1 {
		t.Fatalf("saveChannels calls = %d, want 1", saves)
	}
	stored := cm.GetChannelDevices(channelID)
	if len(stored) != 2 {
		t.Fatalf("stored devices = %d, want 2", len(stored))
	}
}
