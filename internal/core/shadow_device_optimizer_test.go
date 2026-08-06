package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestShadowDeviceOptimizer_UpdateDeviceRTT(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()
	deviceID := "test-device-1"

	// 测试更新RTT
	sdo.UpdateDeviceRTT(deviceID, 50000) // 50ms

	// 验证RTT更新
	rtt := sdo.GetRTTManager().GetEWMARTT(deviceID)
	if rtt == 0 {
		t.Errorf("Expected RTT to be updated, got 0")
	}

	// 验证MTU更新
	mtu := sdo.GetMTUManager().GetCurrentMTU(deviceID)
	if mtu == 0 {
		t.Errorf("Expected MTU to be updated, got 0")
	}

	// 验证Gap更新
	gap := sdo.GetGapOptimizer().GetCurrentGap(deviceID)
	if gap == 0 {
		t.Errorf("Expected Gap to be updated, got 0")
	}

	// 测试获取优化参数
	opts := sdo.GetDeviceOptimization(deviceID)
	if opts["rtt"] == nil {
		t.Errorf("Expected RTT in optimization result")
	}
	if opts["mtu"] == nil {
		t.Errorf("Expected MTU in optimization result")
	}
	if opts["gap"] == nil {
		t.Errorf("Expected Gap in optimization result")
	}
}

func TestShadowDeviceOptimizer_UpdateShadowDeviceProfile(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()
	deviceID := "test-device-2"

	// 先更新RTT
	sdo.UpdateDeviceRTT(deviceID, 100000) // 100ms

	// 创建影子设备
	shadowDevice := &model.ShadowDevice{
		ShadowDeviceID:   "shadow-test-device-2",
		PhysicalDeviceID: deviceID,
		ChannelID:        "channel-1",
		Version:          1,
		UpdatedAt:        time.Now(),
		Points:           make(map[string]model.ShadowPoint),
	}

	// 更新通信画像
	sdo.UpdateShadowDeviceProfile(shadowDevice)

	// 验证通信画像被创建
	if shadowDevice.CommunicationProfile == nil {
		t.Fatalf("Expected CommunicationProfile to be created")
	}

	// 验证通信画像字段
	if shadowDevice.CommunicationProfile.DeviceID != deviceID {
		t.Errorf("Expected DeviceID to be %s, got %s", deviceID, shadowDevice.CommunicationProfile.DeviceID)
	}

	if shadowDevice.CommunicationProfile.ChannelID != "channel-1" {
		t.Errorf("Expected ChannelID to be channel-1, got %s", shadowDevice.CommunicationProfile.ChannelID)
	}

	if shadowDevice.CommunicationProfile.EWMARTT == 0 {
		t.Errorf("Expected EWMARTT to be updated, got 0")
	}

	if shadowDevice.CommunicationProfile.CurrentMTU == 0 {
		t.Errorf("Expected CurrentMTU to be updated, got 0")
	}

	if shadowDevice.CommunicationProfile.CurrentGap == 0 {
		t.Errorf("Expected CurrentGap to be updated, got 0")
	}
}

func TestShadowDeviceOptimizer_ClearDeviceData(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()
	deviceID := "test-device-3"

	// 先更新RTT
	sdo.UpdateDeviceRTT(deviceID, 50000) // 50ms

	// 验证数据存在
	rtt := sdo.GetRTTManager().GetEWMARTT(deviceID)
	if rtt == 0 {
		t.Errorf("Expected RTT to be updated, got 0")
	}

	// 清除设备数据
	sdo.ClearDeviceData(deviceID)

	// 验证数据被清除
	rtt = sdo.GetRTTManager().GetEWMARTT(deviceID)
	if rtt != 0 {
		t.Errorf("Expected RTT to be 0 after clear, got %d", rtt)
	}

	mtu := sdo.GetMTUManager().GetCurrentMTU(deviceID)
	if mtu != 512 { // 默认值
		t.Errorf("Expected MTU to be 512 after clear, got %d", mtu)
	}

	gap := sdo.GetGapOptimizer().GetCurrentGap(deviceID)
	if gap != 64 { // 默认值
		t.Errorf("Expected Gap to be 64 after clear, got %d", gap)
	}
}

func TestShadowCore_UpdateDeviceRTT(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	deviceID := "test-device-4"

	// 先创建影子设备
	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-1",
		DeviceID:  deviceID,
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.5, Quality: "good"},
		},
	}

	_, err = sc.WriteShadowDevice(msg)
	if err != nil {
		t.Fatalf("WriteShadowDevice failed: %v", err)
	}

	// 更新RTT
	sc.UpdateDeviceRTT(deviceID, 75000) // 75ms

	// 获取影子设备
	shadowDeviceID := "shadow-" + deviceID
	device, err := sc.GetShadowDevice(shadowDeviceID)
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}

	// 验证通信画像被更新
	if device.CommunicationProfile == nil {
		t.Fatalf("Expected CommunicationProfile to be created")
	}

	if device.CommunicationProfile.EWMARTT == 0 {
		t.Errorf("Expected EWMARTT to be updated, got 0")
	}

	if device.CommunicationProfile.CurrentMTU == 0 {
		t.Errorf("Expected CurrentMTU to be updated, got 0")
	}

	if device.CommunicationProfile.CurrentGap == 0 {
		t.Errorf("Expected CurrentGap to be updated, got 0")
	}
}

func TestShadowCore_WriteShadowDevice_WithOptimization(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sc := NewShadowCore()
	deviceID := "test-device-5"

	// 先更新RTT
	sc.UpdateDeviceRTT(deviceID, 30000) // 30ms

	// 写入影子设备
	msg := model.ShadowIngressMessage{
		MessageID: "test-msg-2",
		DeviceID:  deviceID,
		ChannelID: "channel-1",
		Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{
			{PointID: "point-1", Value: 42.5, Quality: "good"},
			{PointID: "point-2", Value: 100.0, Quality: "good"},
		},
	}

	resp, err := sc.WriteShadowDevice(msg)
	if err != nil {
		t.Fatalf("WriteShadowDevice failed: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success, got failure")
	}

	// 获取影子设备
	shadowDeviceID := "shadow-" + deviceID
	device, err := sc.GetShadowDevice(shadowDeviceID)
	if err != nil {
		t.Fatalf("GetShadowDevice failed: %v", err)
	}

	// 验证通信画像被创建和更新
	if device.CommunicationProfile == nil {
		t.Fatalf("Expected CommunicationProfile to be created")
	}

	if device.CommunicationProfile.EWMARTT == 0 {
		t.Errorf("Expected EWMARTT to be updated, got 0")
	}

	if device.CommunicationProfile.CurrentMTU == 0 {
		t.Errorf("Expected CurrentMTU to be updated, got 0")
	}

	if device.CommunicationProfile.CurrentGap == 0 {
		t.Errorf("Expected CurrentGap to be updated, got 0")
	}

	// 验证点位数据
	if len(device.Points) != 2 {
		t.Errorf("Expected 2 points, got %d", len(device.Points))
	}

	if _, exists := device.Points["point-1"]; !exists {
		t.Errorf("Expected point-1 to exist")
	}

	if _, exists := device.Points["point-2"]; !exists {
		t.Errorf("Expected point-2 to exist")
	}
}

func TestShadowDeviceOptimizer_LogDeviceOptimization(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()
	deviceID := "test-device-6"

	// 先更新RTT
	sdo.UpdateDeviceRTT(deviceID, 50000) // 50ms

	// 测试日志功能（不验证输出，只确保不崩溃）
	sdo.LogDeviceOptimization(deviceID)
}

func TestShadowDeviceOptimizer_GetAllDevices(t *testing.T) {
	sdo := NewShadowDeviceOptimizer()

	// 测试获取所有设备（暂时返回空列表）
	devices := sdo.GetAllDevices()
	if devices == nil {
		t.Errorf("Expected non-nil devices list")
	}
}
