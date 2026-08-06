package modbus

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestPointDecoder_Encode_Int64(t *testing.T) {
	decoder := NewPointDecoder("ABCD", 0, 0)

	tests := []struct {
		name      string
		point     model.Point
		value     any
		expectErr bool
	}{
		{
			name:      "int16 with int64",
			point:     model.Point{DataType: "int16"},
			value:     int64(123),
			expectErr: false,
		},
		{
			name:      "uint16 with int64",
			point:     model.Point{DataType: "uint16"},
			value:     int64(123),
			expectErr: false,
		},
		{
			name:      "int32 with int64",
			point:     model.Point{DataType: "int32"},
			value:     int64(123456),
			expectErr: false,
		},
		{
			name:      "uint32 with int64",
			point:     model.Point{DataType: "uint32"},
			value:     int64(123456),
			expectErr: false,
		},
		{
			name:      "float32 with int64",
			point:     model.Point{DataType: "float32"},
			value:     int64(123),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decoder.Encode(tt.point, tt.value)
			if (err != nil) != tt.expectErr {
				t.Errorf("Encode() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestPointDecoder_ReverseScaleOffset_Int64(t *testing.T) {
	decoder := NewPointDecoder("ABCD", 0, 0)

	// Test case where Scale/Offset are used with int64 input
	point := model.Point{
		DataType: "int16",
		Scale:    0.1,
		Offset:   0,
	}

	// Input 100 (int64) -> (100 - 0) / 0.1 = 1000
	val := int64(100)
	res := decoder.reverseScaleOffset(point, val)

	fRes, ok := res.(float64)
	if !ok {
		t.Errorf("Expected float64 result, got %T", res)
	}

	if fRes != 1000.0 {
		t.Errorf("Expected 1000.0, got %f", fRes)
	}
}
