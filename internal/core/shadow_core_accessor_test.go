package core

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestShadowCore_GetAllShadowDevicesAndPoint(t *testing.T) {
	sc := NewShadowCore()

	_, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID: "dev-a", ChannelID: "ch1", Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{{PointID: "p1", Value: 1, Quality: "good"}},
	})
	if err != nil {
		t.Fatalf("WriteShadowDevice dev-a: %v", err)
	}
	_, err = sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID: "dev-b", ChannelID: "ch1", Timestamp: time.Now(),
		Points: []model.ShadowIngressPoint{{PointID: "p2", Value: 2, Quality: "good"}},
	})
	if err != nil {
		t.Fatalf("WriteShadowDevice dev-b: %v", err)
	}

	all := sc.GetAllShadowDevices()
	if len(all) != 2 {
		t.Fatalf("GetAllShadowDevices = %d, want 2", len(all))
	}

	pt, err := sc.GetShadowPoint("shadow-dev-a", "p1")
	if err != nil || pt.Value != 1 {
		t.Fatalf("GetShadowPoint = %+v, err=%v", pt, err)
	}
	if _, err := sc.GetShadowPoint("shadow-dev-a", "missing"); err == nil {
		t.Fatal("expected error for missing point")
	}
	if _, err := sc.GetShadowPoint("shadow-missing", "p1"); err == nil {
		t.Fatal("expected error for missing device")
	}
}

func TestShadowCore_GetDeviceOptimization(t *testing.T) {
	sc := NewShadowCore()
	opt := sc.GetDeviceOptimization("dev-1")
	if opt == nil {
		t.Fatal("optimizer should return default params map")
	}
	if opt["gap"] == nil {
		t.Fatalf("expected default gap in optimization map: %+v", opt)
	}

	sc.UpdateDeviceRTT("dev-1", 5000)
	opt = sc.GetDeviceOptimization("dev-1")
	if opt == nil {
		t.Fatal("expected optimization after RTT update")
	}
}

func TestChannelManager_DeviceIOProfile_WithOptimizer(t *testing.T) {
	cm := newTestChannelManager()
	sc := NewShadowCore()
	cm.SetShadowCore(sc)
	sc.UpdateDeviceRTT("dev-1", 8000)

	profile := cm.deviceIOProfile("dev-1")
	if profile.Gap <= 0 || profile.BatchSize <= 0 {
		t.Fatalf("profile with optimizer = %+v", profile)
	}
}

func TestShadowCore_CloneHelpers(t *testing.T) {
	profile := &model.DeviceCommunicationProfile{
		DeviceID: "dev-1", BatchSize: 32,
		ProtocolParams: map[string]interface{}{"gap": 32, "mtu": 64},
	}
	src := &model.ShadowDevice{
		ShadowDeviceID:       "shadow-dev-1",
		PhysicalDeviceID:     "dev-1",
		ChannelID:            "ch1",
		Points:               map[string]model.ShadowPoint{"p1": {Value: 10, Quality: "good"}},
		CommunicationProfile: profile,
	}
	cloned := cloneShadowDevice(src)
	if cloned == nil || cloned.Points["p1"].Value != 10 {
		t.Fatalf("cloneShadowDevice = %+v", cloned)
	}
	cloned.Points["p1"] = model.ShadowPoint{Value: 99}
	if src.Points["p1"].Value != 10 {
		t.Fatal("cloneShadowDevice should deep-copy points")
	}

	clonedProfile := cloneCommunicationProfile(src.CommunicationProfile)
	if clonedProfile == nil || clonedProfile.BatchSize != 32 {
		t.Fatalf("cloneCommunicationProfile = %+v", clonedProfile)
	}
	if cloneShadowDevice(nil) != nil {
		t.Fatal("nil clone should be nil")
	}
	if cloneCommunicationProfile(nil) != nil {
		t.Fatal("nil profile clone should be nil")
	}
}
