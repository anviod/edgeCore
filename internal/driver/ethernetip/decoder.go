package ethernetip

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type ENIPTag struct {
	Name       string
	ArrayIndex int
	Path       []string
}

type ENIPDecoder struct {
}

func NewENIPDecoder() *ENIPDecoder {
	return &ENIPDecoder{}
}

var (
	reSimpleTag    = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)$`)
	reArrayTag     = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\[(\d+)\]$`)
	reFullArrayTag = regexp.MustCompile(`^(.+)\[(\d+)\]$`)
)

func (d *ENIPDecoder) ParseAddress(addr string) (*ENIPTag, error) {
	addr = strings.TrimSpace(addr)

	// 处理数组标签（如 MyArray[10]）
	if m := reArrayTag.FindStringSubmatch(addr); m != nil {
		tagName := m[1]
		index, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %s", m[2])
		}
		return &ENIPTag{
			Name:       tagName,
			ArrayIndex: index,
			Path:       []string{tagName},
		}, nil
	}

	// 处理简单标签（如 MyTag）
	if m := reSimpleTag.FindStringSubmatch(addr); m != nil {
		return &ENIPTag{
			Name:       m[1],
			ArrayIndex: -1,
			Path:       []string{m[1]},
		}, nil
	}

	// 处理包含冒号的地址（如 Program:Main.MyTag 或 Program:Main.MyArray[5]）
	if strings.Contains(addr, ":") {
		// 检查是否为数组形式
		if m := reFullArrayTag.FindStringSubmatch(addr); m != nil {
			// 包含数组索引的程序标签，数组索引包含在 Path 中，ArrayIndex 设为 0
			basePath := m[1]
			parts := strings.Split(basePath, ".")
			if len(parts) >= 2 {
				return &ENIPTag{
					Name:       parts[0],
					ArrayIndex: 0,
					Path:       []string{parts[0], parts[1] + "[" + m[2] + "]"},
				}, nil
			}
			return &ENIPTag{
				Name:       basePath,
				ArrayIndex: 0,
				Path:       []string{addr},
			}, nil
		}

		// 普通程序标签（如 Program:Main.MyTag）
		parts := strings.Split(addr, ".")
		if len(parts) >= 2 {
			return &ENIPTag{
				Name:       parts[0],
				ArrayIndex: -1,
				Path:       parts,
			}, nil
		}

		return &ENIPTag{
			Name:       addr,
			ArrayIndex: -1,
			Path:       []string{addr},
		}, nil
	}

	parts := strings.Split(addr, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid ENIP tag address: %s", addr)
	}

	tag := &ENIPTag{
		Name:       parts[0],
		ArrayIndex: -1,
		Path:       parts,
	}

	if len(parts) > 1 {
		tag.Name = parts[0]
	}

	return tag, nil
}

func (d *ENIPDecoder) EncodeValue(value interface{}, dataType string) ([]byte, error) {
	switch strings.ToUpper(dataType) {
	case "BOOL":
		switch v := value.(type) {
		case bool:
			if v {
				return []byte{1}, nil
			}
			return []byte{0}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "SINT", "INT8":
		switch v := value.(type) {
		case int8:
			return []byte{byte(v)}, nil
		case int:
			// CIP SINT 为 8 位有符号：域校验后负数按补码位模式编码
			if v < math.MinInt8 || v > math.MaxInt8 {
				return nil, fmt.Errorf("value %d out of range for SINT", v)
			}
			// #nosec G115 -- 已域校验，int8 负数为补码位模式重解释
			return []byte{uint8(int8(v))}, nil
		case float64:
			if v != v || v < math.MinInt8 || v > math.MaxInt8 {
				return nil, fmt.Errorf("value %v out of range for SINT", v)
			}
			// #nosec G115 -- 同上
			return []byte{uint8(int8(int(v)))}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "INT", "INT16":
		switch v := value.(type) {
		case int16:
			return []byte{byte(v & 0xFF), byte((v >> 8) & 0xFF)}, nil
		case int:
			return []byte{byte(v & 0xFF), byte((v >> 8) & 0xFF)}, nil
		case float64:
			return []byte{byte(int(v) & 0xFF), byte((int(v) >> 8) & 0xFF)}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "DINT", "INT32":
		switch v := value.(type) {
		case int32:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
			}, nil
		case int:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
			}, nil
		case float64:
			return []byte{
				byte(int(v) & 0xFF),
				byte((int(v) >> 8) & 0xFF),
				byte((int(v) >> 16) & 0xFF),
				byte((int(v) >> 24) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "LINT", "INT64":
		switch v := value.(type) {
		case int64:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
				byte((v >> 32) & 0xFF),
				byte((v >> 40) & 0xFF),
				byte((v >> 48) & 0xFF),
				byte((v >> 56) & 0xFF),
			}, nil
		case int:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
				byte((v >> 32) & 0xFF),
				byte((v >> 40) & 0xFF),
				byte((v >> 48) & 0xFF),
				byte((v >> 56) & 0xFF),
			}, nil
		case float64:
			return []byte{
				byte(int64(v) & 0xFF),
				byte((int64(v) >> 8) & 0xFF),
				byte((int64(v) >> 16) & 0xFF),
				byte((int64(v) >> 24) & 0xFF),
				byte((int64(v) >> 32) & 0xFF),
				byte((int64(v) >> 40) & 0xFF),
				byte((int64(v) >> 48) & 0xFF),
				byte((int64(v) >> 56) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "USINT", "UINT8":
		switch v := value.(type) {
		case uint8:
			return []byte{v}, nil
		case uint:
			if uint64(v) > math.MaxUint8 {
				return nil, fmt.Errorf("value %d out of range for USINT", v)
			}
			return []byte{uint8(v)}, nil
		case int:
			if v < 0 || v > math.MaxUint8 {
				return nil, fmt.Errorf("value %d out of range for USINT", v)
			}
			return []byte{uint8(v)}, nil
		case float64:
			if v != v || v < 0 || v > math.MaxUint8 {
				return nil, fmt.Errorf("value %v out of range for USINT", v)
			}
			return []byte{uint8(v)}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "UINT", "UINT16", "WORD":
		switch v := value.(type) {
		case uint16:
			return []byte{byte(v & 0xFF), byte((v >> 8) & 0xFF)}, nil
		case uint:
			return []byte{byte(v & 0xFF), byte((v >> 8) & 0xFF)}, nil
		case int:
			return []byte{byte(v & 0xFF), byte((v >> 8) & 0xFF)}, nil
		case float64:
			return []byte{byte(uint16(v) & 0xFF), byte((uint16(v) >> 8) & 0xFF)}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "UINT32", "UDINT", "DWORD":
		switch v := value.(type) {
		case uint32:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
			}, nil
		case uint:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
			}, nil
		case int:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
			}, nil
		case float64:
			return []byte{
				byte(uint32(v) & 0xFF),
				byte((uint32(v) >> 8) & 0xFF),
				byte((uint32(v) >> 16) & 0xFF),
				byte((uint32(v) >> 24) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "ULINT", "UINT64", "LWORD":
		switch v := value.(type) {
		case uint64:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
				byte((v >> 32) & 0xFF),
				byte((v >> 40) & 0xFF),
				byte((v >> 48) & 0xFF),
				byte((v >> 56) & 0xFF),
			}, nil
		case uint:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
				byte((v >> 32) & 0xFF),
				byte((v >> 40) & 0xFF),
				byte((v >> 48) & 0xFF),
				byte((v >> 56) & 0xFF),
			}, nil
		case int:
			return []byte{
				byte(v & 0xFF),
				byte((v >> 8) & 0xFF),
				byte((v >> 16) & 0xFF),
				byte((v >> 24) & 0xFF),
				byte((v >> 32) & 0xFF),
				byte((v >> 40) & 0xFF),
				byte((v >> 48) & 0xFF),
				byte((v >> 56) & 0xFF),
			}, nil
		case float64:
			return []byte{
				byte(uint64(v) & 0xFF),
				byte((uint64(v) >> 8) & 0xFF),
				byte((uint64(v) >> 16) & 0xFF),
				byte((uint64(v) >> 24) & 0xFF),
				byte((uint64(v) >> 32) & 0xFF),
				byte((uint64(v) >> 40) & 0xFF),
				byte((uint64(v) >> 48) & 0xFF),
				byte((uint64(v) >> 56) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "REAL", "FLOAT":
		switch v := value.(type) {
		case float32:
			bits := math.Float32bits(v)
			return []byte{
				byte(bits & 0xFF),
				byte((bits >> 8) & 0xFF),
				byte((bits >> 16) & 0xFF),
				byte((bits >> 24) & 0xFF),
			}, nil
		case float64:
			bits := math.Float32bits(float32(v))
			return []byte{
				byte(bits & 0xFF),
				byte((bits >> 8) & 0xFF),
				byte((bits >> 16) & 0xFF),
				byte((bits >> 24) & 0xFF),
			}, nil
		case string:
			f, err := strconv.ParseFloat(v, 32)
			if err != nil {
				return nil, fmt.Errorf("failed to parse string to float32: %w", err)
			}
			bits := math.Float32bits(float32(f))
			return []byte{
				byte(bits & 0xFF),
				byte((bits >> 8) & 0xFF),
				byte((bits >> 16) & 0xFF),
				byte((bits >> 24) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "LREAL", "DOUBLE":
		switch v := value.(type) {
		case float64:
			bits := math.Float64bits(v)
			return []byte{
				byte(bits & 0xFF),
				byte((bits >> 8) & 0xFF),
				byte((bits >> 16) & 0xFF),
				byte((bits >> 24) & 0xFF),
				byte((bits >> 32) & 0xFF),
				byte((bits >> 40) & 0xFF),
				byte((bits >> 48) & 0xFF),
				byte((bits >> 56) & 0xFF),
			}, nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse string to float64: %w", err)
			}
			bits := math.Float64bits(f)
			return []byte{
				byte(bits & 0xFF),
				byte((bits >> 8) & 0xFF),
				byte((bits >> 16) & 0xFF),
				byte((bits >> 24) & 0xFF),
				byte((bits >> 32) & 0xFF),
				byte((bits >> 40) & 0xFF),
				byte((bits >> 48) & 0xFF),
				byte((bits >> 56) & 0xFF),
			}, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	case "STRING":
		switch v := value.(type) {
		case string:
			// CIP STRING 格式：[length:2][max_capacity:2][data:n]
			// 总长度 = 4 + len(v)
			result := make([]byte, 4+len(v))
			// 前 2 字节：实际长度（小端）
			result[0] = byte(len(v) & 0xFF)
			result[1] = byte((len(v) >> 8) & 0xFF)
			// 接下来 2 字节：最大容量（小端，设为 255）
			result[2] = 0xFF
			result[3] = 0x00
			// 剩余字节：字符串数据
			copy(result[4:], []byte(v))
			return result, nil
		default:
			return nil, fmt.Errorf("unsupported data type for encoding: %T", value)
		}
	default:
		return nil, fmt.Errorf("unsupported data type: %s", dataType)
	}
}

func (d *ENIPDecoder) DecodeValue(data []byte, dataType string) (interface{}, error) {
	switch strings.ToUpper(dataType) {
	case "BOOL":
		if len(data) >= 1 {
			return data[0] != 0, nil
		}
		return false, nil
	case "SINT", "INT8":
		if len(data) >= 1 {
			return int8(data[0]), nil
		}
		return int8(0), nil
	case "INT", "INT16":
		if len(data) >= 2 {
			return int16(data[0]) | int16(data[1])<<8, nil
		}
		return int16(0), nil
	case "DINT", "INT32":
		if len(data) >= 4 {
			return int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16 | int32(data[3])<<24, nil
		}
		return int32(0), nil
	case "LINT", "INT64":
		if len(data) >= 8 {
			return int64(data[0]) | int64(data[1])<<8 | int64(data[2])<<16 | int64(data[3])<<24 |
				int64(data[4])<<32 | int64(data[5])<<40 | int64(data[6])<<48 | int64(data[7])<<56, nil
		}
		return int64(0), nil
	case "USINT", "UINT8":
		if len(data) >= 1 {
			return data[0], nil
		}
		return uint8(0), nil
	case "UINT", "UINT16", "WORD":
		if len(data) >= 2 {
			return uint16(data[0]) | uint16(data[1])<<8, nil
		}
		return uint16(0), nil
	case "UINT32", "UDINT", "DWORD":
		if len(data) >= 4 {
			return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24, nil
		}
		return uint32(0), nil
	case "ULINT", "UINT64", "LWORD":
		if len(data) >= 8 {
			return uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
				uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56, nil
		}
		return uint64(0), nil
	case "REAL", "FLOAT":
		if len(data) >= 4 {
			bits := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
			return math.Float32frombits(bits), nil
		}
		return float32(0), nil
	case "LREAL", "DOUBLE":
		if len(data) >= 8 {
			bits := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
				uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56
			return math.Float64frombits(bits), nil
		}
		return float64(0), nil
	case "STRING":
		return string(data), nil
	default:
		return nil, fmt.Errorf("unsupported data type: %s", dataType)
	}
}
