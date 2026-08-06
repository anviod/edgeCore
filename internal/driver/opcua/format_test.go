package opcua

import (
	"encoding/binary"
	"testing"

	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

func TestOpcua_FormatPoint_UnsignedInt(t *testing.T) {
	raw := []byte{0x00, 0x64}
	got, err := dataformat.FormatPoint(raw, binary.BigEndian, dataformat.FormatUnsignedInt, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "100" {
		t.Fatalf("expect 100, got %s", got)
	}
}
