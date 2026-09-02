package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type stubChannelDriver struct {
	health drv.HealthStatus
}

func (s *stubChannelDriver) Init(_ model.DriverConfig) error { return nil }
func (s *stubChannelDriver) Connect(_ context.Context) error { return nil }
func (s *stubChannelDriver) Disconnect() error               { return nil }
func (s *stubChannelDriver) ReadPoints(_ context.Context, _ []model.Point) (map[string]model.Value, error) {
	return nil, nil
}
func (s *stubChannelDriver) WritePoint(_ context.Context, _ model.Point, _ any) error { return nil }
func (s *stubChannelDriver) Health() drv.HealthStatus                                 { return s.health }
func (s *stubChannelDriver) SetSlaveID(_ uint8) error                                 { return nil }
func (s *stubChannelDriver) SetDeviceConfig(_ map[string]any) error                   { return nil }
func (s *stubChannelDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func newTestChannelManager() *ChannelManager {
	cm := NewChannelManager(NewDataPipeline(64), nil)
	// The helper mutates private maps to build fixtures. Stop the production
	// soak goroutine before those fixture writes so -race observes a happens-before edge.
	cm.soakMonitor.Stop()
	cm.channels["ch-1"] = &model.Channel{
		ID:       "ch-1",
		Name:     "modbus",
		Protocol: "modbus-tcp",
		Enable:   true,
		Devices: []model.Device{
			{ID: "dev-1", Name: "slave-1", Enable: true},
			{ID: "dev-2", Name: "slave-2", Enable: true},
		},
	}
	cm.drivers["ch-1"] = &stubChannelDriver{health: drv.HealthStatusGood}
	cm.driverMus["ch-1"] = &sync.Mutex{}
	cm.stateManager.RegisterNode("dev-1", "slave-1")
	cm.stateManager.RegisterNode("dev-2", "slave-2")
	return cm
}

func TestResolveEffectiveDeviceState_ChannelOffline(t *testing.T) {
	ch := &model.Channel{Enable: true}
	dev := &model.Device{Enable: true}
	driver := &stubChannelDriver{health: drv.HealthStatusBad}

	got := resolveEffectiveDeviceState(ch, driver, dev, int(NodeStateOnline))
	if got != int(NodeStateOffline) {
		t.Fatalf("expected offline when channel link is down, got %d", got)
	}
}

func TestResolveEffectiveDeviceState_ChannelOnline(t *testing.T) {
	ch := &model.Channel{Enable: true}
	dev := &model.Device{Enable: true}
	driver := &stubChannelDriver{health: drv.HealthStatusGood}

	got := resolveEffectiveDeviceState(ch, driver, dev, int(NodeStateUnstable))
	if got != int(NodeStateUnstable) {
		t.Fatalf("expected unstable when channel link is up, got %d", got)
	}
}

func TestResolveEffectiveDeviceState_UnknownLinkPreservesDeviceState(t *testing.T) {
	ch := &model.Channel{Enable: true}
	dev := &model.Device{Enable: true}
	driver := &stubChannelDriver{health: drv.HealthStatusUnknown}

	got := resolveEffectiveDeviceState(ch, driver, dev, int(NodeStateOnline))
	if got != int(NodeStateOnline) {
		t.Fatalf("expected online when link is unknown but not bad, got %d", got)
	}
}

func TestMarkChannelDevicesOffline(t *testing.T) {
	cm := newTestChannelManager()

	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateUnstable

	cm.markChannelDevicesOffline("ch-1")

	for _, id := range []string{"dev-1", "dev-2"} {
		node := cm.stateManager.GetNode(id)
		if node.Runtime.State != NodeStateOffline {
			t.Fatalf("device %s expected offline, got %d", id, node.Runtime.State)
		}
	}
}

func TestMarkChannelDevicesOffline_MarksShadowBad(t *testing.T) {
	sc := NewShadowCore()
	cm := newTestChannelManager()
	cm.SetShadowCore(sc)

	oldTime := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-1",
		ChannelID: "ch-1",
		Timestamp: oldTime,
		Points: []model.ShadowIngressPoint{
			{PointID: "hr_0", Value: 10.0, Quality: "Good", CollectedAt: oldTime},
		},
	}); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	cm.scanEngineAdapter.scanEngine.AddTask("dev-1", "modbus-tcp", 1*time.Second, 5, []string{"hr_0", "hr_1"}, map[string]any{"channelID": "ch-1"})

	cm.markChannelDevicesOffline("ch-1")

	shadow, err := sc.GetShadowDevice("shadow-dev-1")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	for _, id := range []string{"hr_0", "hr_1"} {
		pt, ok := shadow.Points[id]
		if !ok {
			t.Fatalf("missing point %s in shadow", id)
		}
		if pt.Quality != "Bad" {
			t.Fatalf("point %s quality = %q, want Bad", id, pt.Quality)
		}
	}
	if shadow.Points["hr_0"].Value != 10.0 {
		t.Fatalf("hr_0 value should be preserved, got %v", shadow.Points["hr_0"].Value)
	}
}

func TestFinalizeScanCollect_ChannelLinkErrorMarksAllOffline(t *testing.T) {
	cm := newTestChannelManager()
	cm.drivers["ch-1"] = &stubChannelDriver{health: drv.HealthStatusBad}

	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: false,
		Error:   errors.New("Modbus connection failed, entering coolDown: dial tcp 127.0.0.1:502: connect: connection refused"),
	})

	for _, id := range []string{"dev-1", "dev-2"} {
		node := cm.stateManager.GetNode(id)
		if node.Runtime.State != NodeStateOffline {
			t.Fatalf("device %s expected offline after channel link error, got %d", id, node.Runtime.State)
		}
	}
}

func TestFinalizeScanCollect_DeviceTimeoutDoesNotMarkAllOffline(t *testing.T) {
	cm := newTestChannelManager()

	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: false,
		Error:   errors.New("i/o timeout"),
	})

	if cm.stateManager.GetNode("dev-1").Runtime.State == NodeStateOnline {
		t.Fatal("dev-1 should not remain online after timeout failure")
	}
	if cm.stateManager.GetNode("dev-2").Runtime.State != NodeStateOnline {
		t.Fatalf("dev-2 should stay online, got %d", cm.stateManager.GetNode("dev-2").Runtime.State)
	}
}

func TestFinalizeScanCollect_BadPointQualityKeepsDeviceOnline(t *testing.T) {
	cm := newTestChannelManager()
	cm.scanEngineAdapter.scanEngine.AddTask("dev-1", "opc-ua", 1*time.Second, 5, []string{"p1", "p2", "p3"}, nil)

	node := cm.stateManager.GetNode("dev-1")
	node.Runtime.State = NodeStateUnstable
	node.Runtime.FailCount = 3

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {PointID: "p1", Quality: "Good", Value: 1.0},
			"p2": {PointID: "p2", Quality: "Bad", Value: 0.0},
			"p3": {PointID: "p3", Quality: "Bad", Value: 0.0},
		},
	})

	if node.Runtime.State != NodeStateOnline {
		t.Fatalf("expected online after transport success with partial Bad points, got %d", node.Runtime.State)
	}
	if node.Runtime.FailCount != 0 {
		t.Fatalf("expected fail count reset, got %d", node.Runtime.FailCount)
	}
}

func TestFinalizeScanCollect_AllBadPointsMarksOffline(t *testing.T) {
	cm := newTestChannelManager()
	cm.scanEngineAdapter.scanEngine.AddTask("dev-1", "modbus-tcp", 1*time.Second, 5, []string{"p1", "p2", "p3"}, nil)

	node := cm.stateManager.GetNode("dev-1")
	node.Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {PointID: "p1", Quality: "Bad", Value: nil},
			"p2": {PointID: "p2", Quality: "Bad", Value: nil},
			"p3": {PointID: "p3", Quality: "Bad", Value: nil},
		},
	})

	if node.Runtime.State != NodeStateOffline {
		t.Fatalf("expected offline when all points are Bad, got %d", node.Runtime.State)
	}
}

func TestCollectContextFromExecuteResult_CountsPointQuality(t *testing.T) {
	ctx := collectContextFromExecuteResult(&ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {Quality: "Bad"},
			"p2": {Quality: "Good"},
		},
	}, 2)
	if ctx.SuccessCmd != 1 || ctx.FailCmd != 1 {
		t.Fatalf("expected quality-based success=1 fail=1, got success=%d fail=%d", ctx.SuccessCmd, ctx.FailCmd)
	}

	allBad := collectContextFromExecuteResult(&ExecuteResult{
		Success: true,
		Values: map[string]model.Value{
			"p1": {Quality: "Bad"},
			"p2": {Quality: "Bad"},
		},
	}, 2)
	if allBad.SuccessCmd != 0 || allBad.FailCmd != 2 {
		t.Fatalf("expected all bad success=0 fail=2, got success=%d fail=%d", allBad.SuccessCmd, allBad.FailCmd)
	}
}

// TestCollectContextFromExecuteResult_AllSkippedIdleDoesNotOffline: when the link is
// healthy (nil error) but every point is in point-level cooldown, no I/O actually ran.
// Thread result must be treated as an idle success (device stays online), NOT as an
// unreachable failure — otherwise a cooled device is stuck offline until restart.
func TestCollectContextFromExecuteResult_AllSkippedIdleDoesNotOffline(t *testing.T) {
	idle := collectContextFromExecuteResult(&ExecuteResult{Success: true}, 3)
	if idle.SuccessCmd != 1 || idle.FailCmd != 0 {
		t.Fatalf("expected idle collect success=1 fail=0, got success=%d fail=%d",
			idle.SuccessCmd, idle.FailCmd)
	}
}

// TestFinalizeScanCollect_AllSkippedKeepsDeviceOnline verifies a device whose points
// are all cooled (empty idle result) recovers to Online from Offline without a restart.
func TestFinalizeScanCollect_AllSkippedKeepsDeviceOnline(t *testing.T) {
	cm := newTestChannelManager()
	cm.scanEngineAdapter.scanEngine.AddTask("dev-1", "modbus-tcp", 1*time.Second, 5, []string{"p1", "p2", "p3"}, nil)

	node := cm.stateManager.GetNode("dev-1")
	node.Runtime.State = NodeStateOffline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{Success: true})

	if node.Runtime.State == NodeStateOffline {
		t.Fatal("device whose points are all cooled (idle success) must not remain offline")
	}
}

func TestFinalizeScanCollect_LinkErrorWithLinkUpOnlyFailsDevice(t *testing.T) {
	cm := newTestChannelManager()

	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	cm.finalizeScanCollect("dev-1", &ExecuteResult{
		Success: false,
		Error:   ErrConnectionUnavailable,
	})

	if cm.stateManager.GetNode("dev-1").Runtime.State == NodeStateOnline {
		t.Fatal("dev-1 should not remain online after link error while link is up")
	}
	if cm.stateManager.GetNode("dev-2").Runtime.State != NodeStateOnline {
		t.Fatalf("dev-2 should stay online when channel link is still up, got %d", cm.stateManager.GetNode("dev-2").Runtime.State)
	}
}

func TestIsChannelLinkError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("i/o timeout"), false},
		{errors.New("read timeout on slave 3"), false},
		{errors.New("dial tcp 127.0.0.1:502: connect: connection refused"), true},
		{ErrConnectionUnavailable, true},
	}
	for _, tc := range cases {
		if got := isChannelLinkError(tc.err); got != tc.want {
			t.Fatalf("isChannelLinkError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestResolveDeviceQualityScore_FromStateWhenNoMetrics(t *testing.T) {
	cases := []struct {
		state int
		want  int
	}{
		{int(NodeStateOnline), 100},
		{int(NodeStateUnstable), 60},
		{int(NodeStateQuarantine), 20},
		{int(NodeStateOffline), 0},
	}
	for _, tc := range cases {
		dev := &model.Device{State: tc.state}
		got := resolveDeviceQualityScore(dev, &model.DeviceMetrics{})
		if got != tc.want {
			t.Fatalf("state %d: got quality %d, want %d", tc.state, got, tc.want)
		}
	}
}

func TestResolveDeviceQualityScore_PrefersCollectedMetrics(t *testing.T) {
	dev := &model.Device{State: int(NodeStateOnline)}
	metrics := &model.DeviceMetrics{
		LastCollectTime: time.Now(),
		HealthScore:     42,
	}
	if got := resolveDeviceQualityScore(dev, metrics); got != 42 {
		t.Fatalf("expected collected health score 42, got %d", got)
	}
}

func TestGetChannelDevices_QualityScoreFromOnlineState(t *testing.T) {
	cm := newTestChannelManager()
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateUnstable

	devices := cm.GetChannelDevices("ch-1")
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].QualityScore != 100 {
		t.Fatalf("online device quality score = %d, want 100", devices[0].QualityScore)
	}
	if devices[1].QualityScore != 60 {
		t.Fatalf("unstable device quality score = %d, want 60", devices[1].QualityScore)
	}
}

func TestGetChannelDevices_OverridesStateWhenChannelOffline(t *testing.T) {
	cm := newTestChannelManager()
	cm.drivers["ch-1"] = &stubChannelDriver{health: drv.HealthStatusBad}
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline

	devices := cm.GetChannelDevices("ch-1")
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].State != int(NodeStateOffline) {
		t.Fatalf("expected offline in API response, got %d", devices[0].State)
	}
}

func TestGetChannelStats_UnknownLinkDoesNotMarkChannelOffline(t *testing.T) {
	cm := newTestChannelManager()
	cm.drivers["ch-1"] = &stubChannelDriver{health: drv.HealthStatusUnknown}
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOnline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	stats := cm.GetChannelStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 channel stat, got %d", len(stats))
	}
	if stats[0].Status == "Offline" {
		t.Fatalf("channel should not be offline when link is unknown and devices are online, got %s", stats[0].Status)
	}
}

func TestGetChannelStats_SingleDeviceOfflineKeepsChannelOnline(t *testing.T) {
	cm := newTestChannelManager()
	cm.stateManager.GetNode("dev-1").Runtime.State = NodeStateOffline
	cm.stateManager.GetNode("dev-2").Runtime.State = NodeStateOnline

	stats := cm.GetChannelStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 channel stat, got %d", len(stats))
	}
	if stats[0].Status == "Offline" {
		t.Fatalf("single device offline must not mark channel offline, got %s", stats[0].Status)
	}
	if stats[0].OnlineCount != 1 || stats[0].OfflineCount != 1 {
		t.Fatalf("expected 1 online and 1 offline device, got online=%d offline=%d",
			stats[0].OnlineCount, stats[0].OfflineCount)
	}
}
