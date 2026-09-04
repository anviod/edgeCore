package modbus

import (
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
)

// TestCooling_ConnectionLevelFailureDoesNotStarve verifies that transport/link
// level failures (device offline / timeout) do NOT push points into SKIPPED.
// Once the link recovers, every point must be immediately readable — this is
// the anti-starvation guarantee (参考 Kepware: 设备级错误不做点位永久跳过).
func TestCooling_ConnectionLevelFailureDoesNotStarve(t *testing.T) {
	s := NewPointScheduler(nil, nil, 125, 50, 0)
	point := model.Point{ID: "p1", Address: "40001", RegisterType: model.RegHolding}
	s.pointStates["p1"] = &PointRuntime{Point: point, State: "OK"}

	// 10 次连接级失败：即使远超 3 次判定阈值，点位也必须始终停留 OK（无缝恢复）。
	for i := 0; i < 10; i++ {
		s.markPointFailed("p1", fmt.Errorf("modbus: not connected"), true)
		rt := s.pointStates["p1"]
		assert.Equal(t, "OK", rt.State, "iter %d: connection error must not cause SKIPPED", i)
		assert.Equal(t, 0, rt.FailCount, "iter %d: connection error must not accumulate FailCount", i)
	}

	for _, hint := range []string{
		"i/o timeout", "request timed out", "connection refused",
		"connection reset by peer", "network is unreachable", "broken pipe",
		"dial tcp 192.168.1.10:502: connect: connection refused",
	} {
		s.markPointFailed("p1", fmt.Errorf("%s", hint), true)
		rt := s.pointStates["p1"]
		assert.Equal(t, "OK", rt.State, "hint %q must keep point OK", hint)
	}
}

// TestCooling_ConnectionLevelErrorThawsSkippedPoint verifies the reconnect-side
// unfreeze: a point previously throttled (SKIPPED) is thawed back to OK once a
// connection-level error arrives, so post-recovery collection resumes immediately.
func TestCooling_ConnectionLevelErrorThawsSkippedPoint(t *testing.T) {
	s := NewPointScheduler(nil, nil, 125, 50, 0)
	point := model.Point{ID: "p2", Address: "1", RegisterType: model.RegCoil}
	s.pointStates["p2"] = &PointRuntime{Point: point, State: "SKIPPED", FailCount: 3,
		CooldownUntil: time.Now().Add(5 * time.Minute)}

	rt := s.pointStates["p2"]
	assert.Equal(t, "SKIPPED", rt.State, "precondition: point is throttled")

	s.markPointFailed("p2", fmt.Errorf("i/o timeout"), true)
	rt = s.pointStates["p2"]
	assert.Equal(t, "OK", rt.State, "connection error must thaw a SKIPPED point")
	assert.Equal(t, 0, rt.FailCount)
	assert.True(t, rt.CooldownUntil.IsZero(), "cooldown must be cleared after thaw")
}

// TestCooling_GroupLevelIllegalTextDoesNotPermanentSkip: the核心 anti-starvation
// guarantee. When a WHOLE register group fails (device offline / all points Bad),
// even if the propagated error text mentions an illegal address, no point may be
// marked as a permanent illegal address — device faults must not collapse into
// per-point permanent skip.
func TestCooling_GroupLevelIllegalTextDoesNotPermanentSkip(t *testing.T) {
	s := NewPointScheduler(nil, nil, 125, 50, 0)
	s.pointStates["g1"] = &PointRuntime{Point: model.Point{ID: "g1"}, State: "OK", FailCount: 9}

	// group 级批量失败：pointLevel=false → 不判永久非法
	s.markPointFailed("g1", fmt.Errorf("modbus exception 2: illegal data address"), false)
	rt := s.pointStates["g1"]
	assert.Equal(t, "SKIPPED", rt.State, "group-level failure may still enter the transient ladder")
	assert.NotEqual(t, permanentSkipUntil, rt.CooldownUntil,
		"group-level failure must NOT produce a permanent illegal-address skip")
	assert.True(t, rt.CooldownUntil.Before(time.Now().Add(10*time.Minute)),
		"group-level failure must stay on the short (5m) ladder, got %s",
		time.Until(rt.CooldownUntil).Round(time.Second))
}

// TestCooling_IsIllegalDataAddressRequiresExplicitAddressFault: only the protocol
// explicit "illegal data address" (exception 2) is a permanent illegal-address
// fault. "illegal data value" (exception 3), busy and unrelated text are NOT.
func TestCooling_IsIllegalDataAddressRequiresExplicitAddressFault(t *testing.T) {
	yes := []string{
		"modbus: exception '2' (illegal data address)",
		"modbus exception 2: illegal data address",
	}
	for _, msg := range yes {
		assert.True(t, isIllegalDataAddress(fmt.Errorf("%s", msg)), "%q must be illegal data address", msg)
	}
	no := []string{
		"modbus exception 3: illegal data value",
		"illegal data value",
		"gateway target device failed to respond",
		"i/o timeout",
	}
	for _, msg := range no {
		assert.False(t, isIllegalDataAddress(fmt.Errorf("%s", msg)), "%q must NOT be illegal data address", msg)
	}
}

// TestCooling_IllegalAddressPermanentAndManualResetByConfigChange: a point-level
// explicit illegal address becomes a permanent skip, and editing the point's
// address (人工重置) clears the skip without a restart.
func TestCooling_IllegalAddressPermanentAndManualResetByConfigChange(t *testing.T) {
	s := NewPointScheduler(nil, nil, 125, 50, 0)
	s.pointStates["p5"] = &PointRuntime{
		Point: model.Point{ID: "p5", Address: "40001", RegisterType: model.RegHolding}, State: "OK",
	}

	s.markPointFailed("p5", fmt.Errorf("modbus exception 2: illegal data address"), true)
	rt := s.pointStates["p5"]
	assert.Equal(t, "SKIPPED", rt.State, "point-level illegal address must be SKIPPED")
	assert.Equal(t, permanentSkipUntil, rt.CooldownUntil, "point-level illegal address must skip permanently")
	// 永久跳过不得被自动冷却到期恢复
	assert.False(t, time.Now().After(rt.CooldownUntil), "permanent skip must not elapse automatically")

	// 人工重置：修改地址后 prepareRuntimes 应自动解冻并重新参与采集
	edited := model.Point{ID: "p5", Address: "40002", RegisterType: model.RegHolding}
	active := s.prepareRuntimes([]model.Point{edited})
	rt = s.pointStates["p5"]
	assert.Equal(t, "OK", rt.State, "editing point config must clear permanent skip (manual reset)")
	assert.Equal(t, 0, rt.FailCount)
	assert.True(t, rt.CooldownUntil.IsZero())
	assert.Contains(t, active, edited, "reset point must be re-collected")
}

// TestCooling_PointLevelErrorStillDegrades verifies point-specific errors keep
// the existing SKIPPED ladder (60s / 5min) for non-illegal point faults.
func TestCooling_PointLevelErrorStillDegrades(t *testing.T) {
	s := NewPointScheduler(nil, nil, 125, 50, 0)
	s.pointStates["p4"] = &PointRuntime{Point: model.Point{ID: "p4"}, State: "OK"}
	for i := 0; i < 3; i++ {
		s.markPointFailed("p4", fmt.Errorf("read length mismatch"), true)
	}
	assert.Equal(t, "SKIPPED", s.pointStates["p4"].State, "3 point-level failures must enter SKIPPED")
	assert.NotEqual(t, permanentSkipUntil, s.pointStates["p4"].CooldownUntil,
		"generic point failures must transiently skip, never permanent")
}