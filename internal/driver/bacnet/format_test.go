package bacnet

import (
	"encoding/binary"
	"testing"

	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

func TestBacnet_FormatPoint_HexBinary(t *testing.T) {
	raw := []byte{0xde, 0xad}

	hexStr, err := dataformat.FormatPoint(raw, binary.BigEndian, dataformat.FormatHexString, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hexStr != "0xDEAD" {
		t.Fatalf("expect 0xDEAD, got %s", hexStr)
	}

	binStr, err := dataformat.FormatPoint(raw, binary.BigEndian, dataformat.FormatBinaryString, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if binStr == "" {
		t.Fatalf("expect non-empty binary string")
	}
}
