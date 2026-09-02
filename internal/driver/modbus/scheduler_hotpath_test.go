package modbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestPointScheduler_markPointFailed_IllegalAddress(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-1"] = &PointRuntime{State: "OK"}

	s.markPointFailed("pt-1", errors.New("modbus exception 2: illegal data address"), true)
	rt := s.pointStates["pt-1"]
	if rt.State != "SKIPPED" {
		t.Fatalf("state = %q, want SKIPPED", rt.State)
	}
	if rt.CooldownUntil.Before(time.Now().Add(23 * time.Hour)) {
		t.Fatal("expected long cooldown for illegal address")
	}
}

func TestPointScheduler_markPointFailed_RepeatedFailures(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-2"] = &PointRuntime{State: "OK", FailCount: 2}

	s.markPointFailed("pt-2", errors.New("read length mismatch"), true)
	rt := s.pointStates["pt-2"]
	if rt.FailCount != 3 {
		t.Fatalf("fail count = %d, want 3", rt.FailCount)
	}
	if rt.State != "SKIPPED" {
		t.Fatalf("state = %q, want SKIPPED after 3 point-level failures", rt.State)
	}
}

// TestPointScheduler_markPointFailed_TimeoutDoesNotCool asserts a link-level
// timeout is classified as a device fault, not a point fault: the point must not
// accumulate failure counters nor enter SKIPPED (anti-starvation on recovery).
func TestPointScheduler_markPointFailed_TimeoutDoesNotCool(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-3"] = &PointRuntime{State: "OK", FailCount: 2}

	s.markPointFailed("pt-3", errors.New("iot timeout"), true)
	rt := s.pointStates["pt-3"]
	if rt.FailCount != 0 {
		t.Fatalf("fail count = %d, want 0 (timeout must not degrade the point)", rt.FailCount)
	}
	if rt.State != "OK" {
		t.Fatalf("state = %q, want OK (point must stay readable for immediate recovery)", rt.State)
	}
}

func TestPointScheduler_readGroup_HoldingRegisters(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())
	tr.registers[0] = 0x1234
	tr.registers[1] = 0x5678

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	group := PointGroup{
		RegType:     model.RegHolding,
		StartOffset: 0,
		Count:       2,
		Points: []model.Point{
			{ID: "hr-0", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding},
			{ID: "hr-1", Address: "40002", DataType: "uint16", RegisterType: model.RegHolding},
		},
	}

	out, err := s.readGroup(context.Background(), group)
	if err != nil {
		t.Fatalf("readGroup: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("results = %+v", out)
	}
}

func TestPointScheduler_readGroup_Coil(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())
	tr.coil[5] = true

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	group := PointGroup{
		RegType:     model.RegCoil,
		StartOffset: 5,
		Count:       1,
		Points:      []model.Point{{ID: "coil-5", Address: "00006", DataType: "bool", RegisterType: model.RegCoil}},
	}

	out, err := s.readGroup(context.Background(), group)
	if err != nil {
		t.Fatalf("readGroup coil: %v", err)
	}
	if out["coil-5"] != true {
		t.Fatalf("coil value = %v", out["coil-5"])
	}
}

func TestPointScheduler_Read_HoldingBatch(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())
	tr.registers[0] = 10
	tr.registers[1] = 20

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.SetSlaveID(1)
	if s.GetSlaveID() != 1 {
		t.Fatalf("slave id = %d", s.GetSlaveID())
	}

	out, err := s.Read(context.Background(), []model.Point{
		{ID: "hr-0", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding},
		{ID: "hr-1", Address: "40002", DataType: "uint16", RegisterType: model.RegHolding},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if out["hr-0"].Quality != "Good" || out["hr-1"].Quality != "Good" {
		t.Fatalf("qualities: %+v", out)
	}
}

func TestPointScheduler_Read_SkipsCooldownPoints(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-skip"] = &PointRuntime{
		Point:         model.Point{ID: "pt-skip", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding},
		State:         "SKIPPED",
		CooldownUntil: time.Now().Add(time.Hour),
	}

	out, err := s.Read(context.Background(), []model.Point{
		{ID: "pt-skip", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty result for skipped points, got %+v", out)
	}
}

func TestPointScheduler_Write_HoldingRegister(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	pt := model.Point{ID: "hr-0", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding}
	if err := s.Write(context.Background(), pt, uint16(42)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if tr.registers[0] != 42 {
		t.Fatalf("register = %d, want 42", tr.registers[0])
	}
}

func TestPointScheduler_groupPoints_ContiguousHolding(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	groups, err := s.groupPoints([]model.Point{
		{ID: "a", Address: "40001", DataType: "uint16", RegisterType: model.RegHolding},
		{ID: "b", Address: "40002", DataType: "uint16", RegisterType: model.RegHolding},
		{ID: "c", Address: "40010", DataType: "uint16", RegisterType: model.RegHolding},
	})
	if err != nil {
		t.Fatalf("groupPoints: %v", err)
	}
	if len(groups) < 1 {
		t.Fatal("expected at least one group")
	}
}

func TestPointScheduler_markPointSuccess_andPacketConfig(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-ok"] = &PointRuntime{State: "SKIPPED", FailCount: 3}
	now := time.Now()
	s.markPointSuccess("pt-ok", now)
	rt := s.pointStates["pt-ok"]
	if rt.State != "OK" || rt.FailCount != 0 {
		t.Fatalf("runtime = %+v", rt)
	}

	s.SetMaxPacketSize(64)
	s.SetGroupThreshold(16)
	if got := s.getEffectiveMaxPacketSize(); got == 0 {
		t.Fatal("effective max packet size should be non-zero")
	}
	s.adaptBatchSize(true, 5*time.Millisecond)
	if s.GetDecoder() == nil {
		t.Fatal("decoder should be set")
	}
}

func TestPointScheduler_markPointFailed_TenFailures(t *testing.T) {
	s := NewPointScheduler(newMockModbusTransport(), NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	s.pointStates["pt-10"] = &PointRuntime{State: "OK", FailCount: 9}
	s.markPointFailed("pt-10", errors.New("busy"), false)
	rt := s.pointStates["pt-10"]
	if rt.State != "SKIPPED" {
		t.Fatalf("state = %q after 10 failures", rt.State)
	}
	if rt.CooldownUntil.Before(time.Now().Add(4 * time.Minute)) {
		t.Fatal("expected ~5m cooldown after 10 failures")
	}
}

func TestPointScheduler_readGroup_DiscreteInput(t *testing.T) {
	tr := newMockModbusTransport()
	_ = tr.Connect(context.Background())
	tr.coil[3] = true

	s := NewPointScheduler(tr, NewPointDecoder("ABCD", 0, 0), 256, 8, 0)
	out, err := s.readGroup(context.Background(), PointGroup{
		RegType:     model.RegDiscreteInput,
		StartOffset: 3,
		Count:       1,
		Points:      []model.Point{{ID: "di-3", Address: "10004", DataType: "bool", RegisterType: model.RegDiscreteInput}},
	})
	if err != nil {
		t.Fatalf("readGroup discrete: %v", err)
	}
	if out["di-3"] != true {
		t.Fatalf("discrete value = %v", out["di-3"])
	}
}
