package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestGenerateModbusRegisterPoints(t *testing.T) {
	opts := ModbusRegisterGenOptions{
		Start:        0,
		End:          2,
		DataType:     "int16",
		ReadWrite:    "R",
		RegisterType: model.RegHolding,
		FunctionCode: 3,
		DeviceID:     "dev-1",
	}
	points := GenerateModbusRegisterPoints(nil, opts, false)
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}
	if points[0].ID != "hr_0" || points[0].FunctionCode != 3 {
		t.Fatalf("unexpected first point: %+v", points[0])
	}
}

func TestGenerateModbusRegisterPoints_MergeExisting(t *testing.T) {
	existing := []model.Point{
		{ID: "hr_1", Name: "Custom", Address: "1", DataType: "float32"},
	}
	opts := ModbusRegisterGenOptions{Start: 0, End: 1, RegisterType: model.RegHolding, FunctionCode: 3}
	points := GenerateModbusRegisterPoints(existing, opts, true)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[1].DataType != "float32" {
		t.Fatalf("expected merged point to keep datatype float32, got %s", points[1].DataType)
	}
}

func TestGenerateModbusRegisterPoints_InputRegister(t *testing.T) {
	opts := ModbusRegisterGenOptions{
		Start:        10,
		End:          10,
		DataType:     "int16",
		ReadWrite:    "R",
		RegisterType: model.RegInput,
		FunctionCode: 4,
		DeviceID:     "dev-1",
	}
	points := GenerateModbusRegisterPoints(nil, opts, false)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].ID != "ir_10" || points[0].FunctionCode != 4 {
		t.Fatalf("unexpected point: %+v", points[0])
	}
	if points[0].RegisterType != model.RegInput {
		t.Fatalf("expected input register type, got %v", points[0].RegisterType)
	}
}

func TestParseAutoPointsRange(t *testing.T) {
	start, end, ok := ParseAutoPointsRange("0-199")
	if !ok || start != 0 || end != 199 {
		t.Fatalf("parse failed: %d-%d ok=%v", start, end, ok)
	}
}
