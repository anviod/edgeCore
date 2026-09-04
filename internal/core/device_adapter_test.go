package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestDeviceAdapterManager(t *testing.T) {
	manager := NewDeviceAdapterManager()
	deviceID := "test-device"

	// 测试获取设备适配器
	adapter := manager.GetAdapter(deviceID)
	if adapter == nil {
		t.Error("Expected adapter to be non-nil")
	}

	// 测试获取设备画像
	profile, err := adapter.GetDeviceProfile(deviceID)
	if err != nil {
		t.Errorf("Expected no error when getting device profile, got %v", err)
	}
	if profile == nil {
		t.Error("Expected device profile to be non-nil")
	}
	if profile.DeviceID != deviceID {
		t.Errorf("Expected device ID to be %s, got %s", deviceID, profile.DeviceID)
	}

	// 测试优化采集参数
	params, err := adapter.OptimizeCollectionParams(deviceID)
	if err != nil {
		t.Errorf("Expected no error when optimizing collection parameters, got %v", err)
	}
	if params == nil {
		t.Error("Expected params to be non-nil")
	}

	// 测试更新RTT
	err = adapter.UpdateRTT(deviceID, 100000) // 100ms
	if err != nil {
		t.Errorf("Expected no error when updating RTT, got %v", err)
	}

	// 测试更新MTU
	err = adapter.UpdateMTU(deviceID, 1024)
	if err != nil {
		t.Errorf("Expected no error when updating MTU, got %v", err)
	}

	// 测试更新Gap
	err = adapter.UpdateGap(deviceID, 64)
	if err != nil {
		t.Errorf("Expected no error when updating Gap, got %v", err)
	}

	// 测试计算稳定性评分
	score, err := adapter.CalculateStabilityScore(deviceID)
	if err != nil {
		t.Errorf("Expected no error when calculating stability score, got %v", err)
	}
	if score < 0 || score > 100 {
		t.Errorf("Expected stability score to be between 0 and 100, got %f", score)
	}
}

func TestDeviceAdapterManager_RegisterAdapter(t *testing.T) {
	manager := NewDeviceAdapterManager()
	custom := manager.GetAdapter("custom-dev")
	manager.RegisterAdapter("custom-dev", custom)

	if manager.GetAdapter("custom-dev") == nil {
		t.Fatal("registered adapter should be retrievable")
	}

	profile := &model.DeviceCommunicationProfile{DeviceID: "custom-dev", StabilityScore: 88}
	if err := manager.UpdateDeviceProfile(profile); err != nil {
		t.Fatalf("UpdateDeviceProfile: %v", err)
	}
	got, err := manager.GetDeviceProfile("custom-dev")
	if err != nil || got.StabilityScore != 88 {
		t.Fatalf("GetDeviceProfile = %+v, err=%v", got, err)
	}
}
