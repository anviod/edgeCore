package modbus

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

func TestResolvePointFormat_Int16_Default(t *testing.T) {
	p := model.Point{
		DataType: "int16",
	}
	cfg := dataformat.ResolvePointFormat(p, "ABCD")
	if cfg.Type != dataformat.FormatSignedInt {
		t.Fatalf("expect FormatSignedInt, got %d", cfg.Type)
	}
	if cfg.Order != dataformat.OrderABCD {
		t.Fatalf("expect OrderABCD")
	}
}

func TestResolvePointFormat_Expr32_WithWordOrder(t *testing.T) {
	p := model.Point{
		DataType:    "int32",
		ReadFormula: "v>>1",
		WordOrder:   "CDAB",
	}
	cfg := dataformat.ResolvePointFormat(p, "ABCD")
	if cfg.Type != dataformat.FormatExpr32 {
		t.Fatalf("expect FormatExpr32, got %d", cfg.Type)
	}
	if cfg.Order != dataformat.OrderCDAB {
		t.Fatalf("expect OrderCDAB")
	}
	if cfg.ReadExpr != "v>>1" {
		t.Fatalf("unexpected ReadExpr %s", cfg.ReadExpr)
	}
}

func TestPointDecoder_Decode_WithDataformat_Int16(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)
	d.EnableDataformatDecoder(true)

	point := model.Point{
		ID:       "p1",
		DataType: "int16",
		Scale:    0,
		Offset:   0,
	}
	raw := []byte{0x01, 0x00}

	val, quality, err := d.Decode(point, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quality != "Good" {
		t.Fatalf("expect Good, got %s", quality)
	}
	if v, ok := val.(int16); !ok || v != 256 {
		t.Fatalf("expect int16 256, got %#v", val)
	}
}

func TestPointDecoder_Decode_WithDataformat_Float32Expr(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)
	d.EnableDataformatDecoder(true)

	point := model.Point{
		ID:          "p2",
		DataType:    "int32",
		ReadFormula: "v>>2",
	}
	raw := []byte{0x00, 0x00, 0x01, 0x00}

	val, _, err := d.Decode(point, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := val.(int32); !ok || v != 64 {
		t.Fatalf("expect int32 64, got %#v", val)
	}
}

func TestPointDecoder_Decode_WithDataformat_Float32Order(t *testing.T) {
	d := NewPointDecoder("CDAB", 0, 0)
	d.EnableDataformatDecoder(true)

	point := model.Point{
		ID:       "p3",
		DataType: "float32",
	}

	f := float32(123.456)
	bits := math.Float32bits(f)
	var base [4]byte
	binary.BigEndian.PutUint32(base[:], bits)
	raw := []byte{base[2], base[3], base[0], base[1]}

	val, _, err := d.Decode(point, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := val.(float32)
	if !ok {
		t.Fatalf("expect float32, got %#v", val)
	}
	if diff := math.Abs(float64(f) - float64(v)); diff > 1e-3 {
		t.Fatalf("diff too large: %v", diff)
	}
}

func TestPointDecoder_Decode_WithDataformat_Int32_CDAB(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)
	d.EnableDataformatDecoder(true)

	point := model.Point{
		ID:        "p4",
		DataType:  "int32",
		WordOrder: "CDAB",
	}

	raw := []byte{0x33, 0x44, 0x11, 0x22}

	val, quality, err := d.Decode(point, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quality != "Good" {
		t.Fatalf("expect Good, got %s", quality)
	}
	v, ok := val.(int32)
	if !ok {
		t.Fatalf("expect int32, got %#v", val)
	}
	if v != int32(0x11223344) {
		t.Fatalf("expect 0x11223344, got %#x", uint32(v))
	}
}

func TestPointDecoder_Encode_WithWriteFormula(t *testing.T) {
	d := NewPointDecoder("ABCD", 0, 0)

	point := model.Point{
		ID:           "wf-point",
		DataType:     "uint16",
		Scale:        1,
		Offset:       0,
		WriteFormula: "v*10",
	}

	regs, err := d.Encode(point, 6.0)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expect 1 reg, got %d", len(regs))
	}
	if regs[0] != 60 {
		t.Fatalf("expect 60, got %d", regs[0])
	}
}
