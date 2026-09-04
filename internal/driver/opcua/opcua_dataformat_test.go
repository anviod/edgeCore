package opcua

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

func TestFormatScalarForUint16ExprInOpcua(t *testing.T) {
	point := model.Point{
		ID:          "p1",
		DeviceID:    "dev1",
		Name:        "test",
		Address:     "ns=2;i=1",
		DataType:    "uint16",
		ReadFormula: "v*10",
	}

	raw := uint16(5)

	value, err := dataformat.FormatScalar(point, "ABCD", raw)
	if err != nil {
		t.Fatalf("FormatScalar returned error: %v", err)
	}

	got, ok := value.(uint16)
	if !ok {
		t.Fatalf("expected uint16 value, got %T", value)
	}

	if got != 50 {
		t.Fatalf("expected formatted value 50, got %d", got)
	}
}
