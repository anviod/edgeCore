package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anviod/edgeCore/internal/model"
)

// TestEncodeRaw_RangeGuard 锁定写入侧整型收窄修复（gosec G115）后的语义：
// int16/int32 点型允许负数（补码位模式写入寄存器，Modbus 定义行为），
// uint16/uint32 拒绝负数与超范围值，非法字符串报错（原实现静默写 0）。
func TestEncodeRaw_RangeGuard(t *testing.T) {
	dec := NewPointDecoder("ABCD", 0, 0)

	cases := []struct {
		name    string
		point   model.Point
		value   any
		want    []uint16
		wantErr bool
	}{
		{"int16 negative keeps two's complement", model.Point{DataType: "int16"}, int64(-5), []uint16{0xFFFB}, false},
		{"int16 min", model.Point{DataType: "int16"}, -32768, []uint16{0x8000}, false},
		{"int16 max", model.Point{DataType: "int16"}, 32767, []uint16{0x7FFF}, false},
		{"int16 overflow rejected", model.Point{DataType: "int16"}, 32768, nil, true},
		{"uint16 negative rejected", model.Point{DataType: "uint16"}, -1, nil, true},
		{"uint16 max", model.Point{DataType: "uint16"}, 65535, []uint16{0xFFFF}, false},
		{"uint16 overflow rejected", model.Point{DataType: "uint16"}, 65536, nil, true},
		{"int32 negative keeps two's complement", model.Point{DataType: "int32"}, int64(-1), []uint16{0xFFFF, 0xFFFF}, false},
		{"uint32 negative rejected", model.Point{DataType: "uint32"}, -1, nil, true},
		{"uint32 max", model.Point{DataType: "uint32"}, int64(4294967295), []uint16{0xFFFF, 0xFFFF}, false},
		{"uint32 overflow rejected", model.Point{DataType: "uint32"}, int64(4294967296), nil, true},
		{"invalid string rejected", model.Point{DataType: "uint16"}, "abc", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			regs, err := dec.Encode(tc.point, tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, regs)
		})
	}
}

// TestDecodeInt16BitPattern 锁定读取侧位模式补码重解释语义（0xFFFF 读 int16 点 = -1）。
func TestDecodeInt16BitPattern(t *testing.T) {
	dec := NewPointDecoder("ABCD", 0, 0)
	val, quality, err := dec.Decode(model.Point{DataType: "int16"}, []byte{0xFF, 0xFF})
	require.NoError(t, err)
	assert.Equal(t, "Good", quality)
	assert.Equal(t, int16(-1), val)
}
