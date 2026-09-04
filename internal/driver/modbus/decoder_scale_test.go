package modbus

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestReverseScaleOffset_Int32(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)

	// Case 1: Point with Scale, input int32
	// Expectation: int32 should be converted to float, scaled, then encoded.
	// If Scale=0.1, Offset=0. Input 6 (int32).
	// Calculation: (6 - 0) / 0.1 = 60.
	// Result should be 60.

	point := model.Point{
		ID:       "test-point",
		DataType: "uint16",
		Address:  "0",
		Scale:    0.1,
		Offset:   0,
	}

	valInt32 := int32(6)

	regs, err := d.Encode(point, valInt32)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(regs) != 1 {
		t.Fatalf("Expected 1 register, got %d", len(regs))
	}

	if regs[0] != 60 {
		t.Errorf("Expected 60, got %d. (If 6, then reverseScaleOffset failed to convert type)", regs[0])
	}
}

func TestReverseScaleOffset_UInt32(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)

	point := model.Point{
		ID:       "test-point",
		DataType: "uint16",
		Address:  "0",
		Scale:    0.1,
		Offset:   0,
	}

	valUint32 := uint32(6)

	regs, err := d.Encode(point, valUint32)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if regs[0] != 60 {
		t.Errorf("Expected 60, got %d", regs[0])
	}
}

func TestReverseScaleOffset_NoScale_Int32(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)

	point := model.Point{
		ID:       "test-point",
		DataType: "uint16",
		Address:  "0",
		Scale:    0,
		Offset:   0,
	}

	valInt32 := int32(6)

	regs, err := d.Encode(point, valInt32)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if regs[0] != 6 {
		t.Errorf("Expected 6, got %d", regs[0])
	}
}
