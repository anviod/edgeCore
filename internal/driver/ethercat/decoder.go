package ethercat

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EtherCATDecoder handles type-aware encoding/decoding of PDO/SDO byte slices.
// Supports all standard industrial data types with configurable byte order.
type EtherCATDecoder struct{}

// NewEtherCATDecoder creates a new decoder instance.
func NewEtherCATDecoder() *EtherCATDecoder {
	return &EtherCATDecoder{}
}

// ByteSize returns the expected byte size for a given data type string.
// Returns 0 for unknown types.
func (d *EtherCATDecoder) ByteSize(dataType string) int {
	switch dataType {
	case "bit", "bool", "int8", "uint8":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "float", "float32":
		return 4
	case "int64", "uint64", "double", "float64":
		return 8
	default:
		return 0
	}
}

// DecodeValue decodes raw bytes into a typed Go value based on dataType and endianness.
// For bit addresses, the appropriate bit is extracted from the byte at offset.
func (d *EtherCATDecoder) DecodeValue(data []byte, dataType string, addr *ParsedAddress) (any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ethercat decode: empty data")
	}

	// bit extraction: reads the byte at addr.Offset, extracts the bit
	if addr.Bit >= 0 || dataType == "bit" || dataType == "bool" {
		if len(data) == 0 {
			return nil, fmt.Errorf("ethercat decode: empty data for bit extraction")
		}
		bitPos := addr.Bit
		if bitPos < 0 {
			bitPos = 0
		}
		if bitPos > 7 {
			bitPos = 7
		}
		bitVal := (data[0] >> bitPos) & 1
		if dataType == "bool" || dataType == "bit" {
			return bitVal == 1, nil
		}
		return int(bitVal), nil
	}

	// determine byte order
	var order binary.ByteOrder = binary.BigEndian
	if addr.Endian == "LE" {
		order = binary.LittleEndian
	}

	switch dataType {
	case "int8":
		// #nosec G115 -- 单字节按补码位模式重解释为有符号数
		return int8(data[0]), nil
	case "uint8":
		return data[0], nil
	case "int16":
		if len(data) < 2 {
			return nil, fmt.Errorf("ethercat decode: need 2 bytes for int16, got %d", len(data))
		}
		// #nosec G115 -- 16 位按补码位模式重解释为有符号数
		return int16(order.Uint16(data)), nil
	case "uint16":
		if len(data) < 2 {
			return nil, fmt.Errorf("ethercat decode: need 2 bytes for uint16, got %d", len(data))
		}
		return order.Uint16(data), nil
	case "int32":
		if len(data) < 4 {
			return nil, fmt.Errorf("ethercat decode: need 4 bytes for int32, got %d", len(data))
		}
		// #nosec G115 -- 32 位按补码位模式重解释为有符号数
		return int32(order.Uint32(data)), nil
	case "uint32":
		if len(data) < 4 {
			return nil, fmt.Errorf("ethercat decode: need 4 bytes for uint32, got %d", len(data))
		}
		return order.Uint32(data), nil
	case "int64":
		if len(data) < 8 {
			return nil, fmt.Errorf("ethercat decode: need 8 bytes for int64, got %d", len(data))
		}
		// #nosec G115 -- 64 位按补码位模式重解释为有符号数
		return int64(order.Uint64(data)), nil
	case "uint64":
		if len(data) < 8 {
			return nil, fmt.Errorf("ethercat decode: need 8 bytes for uint64, got %d", len(data))
		}
		return order.Uint64(data), nil
	case "float", "float32":
		if len(data) < 4 {
			return nil, fmt.Errorf("ethercat decode: need 4 bytes for float, got %d", len(data))
		}
		bits := order.Uint32(data)
		return math.Float32frombits(bits), nil
	case "double", "float64":
		if len(data) < 8 {
			return nil, fmt.Errorf("ethercat decode: need 8 bytes for double, got %d", len(data))
		}
		bits := order.Uint64(data)
		return math.Float64frombits(bits), nil
	default:
		// unknown type: return raw bytes as hex string
		return fmt.Sprintf("%x", data), nil
	}
}

// EncodeValue encodes a Go value into a byte slice based on dataType and endianness.
func (d *EtherCATDecoder) EncodeValue(value any, dataType string, addr *ParsedAddress) ([]byte, error) {
	size := d.ByteSize(dataType)
	if size == 0 {
		return nil, fmt.Errorf("ethercat encode: unknown data type %q", dataType)
	}

	buf := make([]byte, size)

	// determine byte order
	var order binary.ByteOrder = binary.BigEndian
	if addr.Endian == "LE" {
		order = binary.LittleEndian
	}

	switch dataType {
	case "int8":
		val, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		buf[0] = byte(val & 0xFF)
	case "uint8":
		val, err := toUint64(value)
		if err != nil {
			return nil, err
		}
		buf[0] = byte(val & 0xFF)
	case "int16":
		val, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint16(buf, uint16(val&0xFFFF))
	case "uint16":
		val, err := toUint64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint16(buf, uint16(val&0xFFFF))
	case "int32":
		val, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint32(buf, uint32(val&0xFFFFFFFF))
	case "uint32":
		val, err := toUint64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint32(buf, uint32(val&0xFFFFFFFF))
	case "int64":
		val, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		// #nosec G115 -- 64 位按补码位模式编码为无符号数
		order.PutUint64(buf, uint64(val))
	case "uint64":
		val, err := toUint64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint64(buf, val)
	case "float", "float32":
		val, err := toFloat64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint32(buf, math.Float32bits(float32(val)))
	case "double", "float64":
		val, err := toFloat64(value)
		if err != nil {
			return nil, err
		}
		order.PutUint64(buf, math.Float64bits(val))
	case "bit", "bool":
		val, err := toBool(value)
		if err != nil {
			return nil, err
		}
		if val {
			buf[0] = 1
		} else {
			buf[0] = 0
		}
	}

	return buf, nil
}

// --- type conversion helpers (aligned with profinetio/decoder.go) ---

// toBool converts various value types to bool.
// Handles JSON number → bool (0=false, non-zero=true) for compatibility.
func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case float64:
		return val != 0, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case string:
		switch val {
		case "true", "1", "True", "TRUE":
			return true, nil
		case "false", "0", "False", "FALSE":
			return false, nil
		}
	}
	return false, fmt.Errorf("ethercat: cannot convert %T to bool", v)
}

// toInt64 converts various value types to int64.
func toInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int:
		return int64(val), nil
	case int8:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int64:
		return val, nil
	case float64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	case uint:
		// #nosec G115 -- 调用方传入的索引/计数远小于 MaxInt64
		return int64(val), nil
	case uint8:
		return int64(val), nil
	case uint16:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint64:
		// #nosec G115 -- 调用方传入的索引/计数远小于 MaxInt64
		return int64(val), nil
	}
	return 0, fmt.Errorf("ethercat: cannot convert %T to int64", v)
}

// toUint64 converts various value types to uint64.
func toUint64(v any) (uint64, error) {
	switch val := v.(type) {
	case uint:
		return uint64(val), nil
	case uint8:
		return uint64(val), nil
	case uint16:
		return uint64(val), nil
	case uint32:
		return uint64(val), nil
	case uint64:
		return val, nil
	case int:
		// #nosec G115 -- 无符号编码按补码位模式解释，负数由调用方域校验
		return uint64(val), nil
	case int8:
		// #nosec G115 -- 同上
		return uint64(val), nil
	case int16:
		// #nosec G115 -- 同上
		return uint64(val), nil
	case int32:
		// #nosec G115 -- 同上
		return uint64(val), nil
	case int64:
		// #nosec G115 -- 同上
		return uint64(val), nil
	case float64:
		return uint64(val), nil
	case float32:
		return uint64(val), nil
	}
	return 0, fmt.Errorf("ethercat: cannot convert %T to uint64", v)
}

// toFloat64 converts various value types to float64.
func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	}
	return 0, fmt.Errorf("ethercat: cannot convert %T to float64", v)
}
