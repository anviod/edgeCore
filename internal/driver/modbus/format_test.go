package modbus

import (
	"encoding/binary"
	"testing"

	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

func TestModbus_FormatPoint_Int16(t *testing.T) {
	raw := []byte{0x01, 0x00}
	got, err := dataformat.FormatPoint(raw, binary.BigEndian, dataformat.FormatSignedInt, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "256" {
		t.Fatalf("expect 256, got %s", got)
	}
}

func TestModbus_FormatPoint_Float32_OrderABCD(t *testing.T) {
	raw := []byte{0x42, 0xf6, 0xe9, 0x79}
	got, err := dataformat.FormatPoint(raw, dataformat.OrderABCD, dataformat.FormatFloat32, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("expect non-empty formatted value")
	}
}
