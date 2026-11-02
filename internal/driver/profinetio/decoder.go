package profinetio

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// ProfinetDecoder encodes/decodes IO data by datatype and endianness.
type ProfinetDecoder struct{}

func NewProfinetDecoder() *ProfinetDecoder {
	return &ProfinetDecoder{}
}

func (d *ProfinetDecoder) DecodeValue(data []byte, dataType string, addr *ParsedAddress) (any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty profinet-io payload")
	}

	dt := strings.ToLower(strings.TrimSpace(dataType))
	var endian binary.ByteOrder = binary.BigEndian
	if addr != nil && addr.Endian == EndianLittle {
		endian = binary.LittleEndian
	}

	switch dt {
	case "bit", "bool":
		bit := 0
		if addr != nil && addr.IsBit {
			bit = addr.Bit
		}
		return (data[0]>>uint(bit))&0x01 != 0, nil
	case "int8":
		// #nosec G115 -- 单字节按补码位模式重解释为有符号数
		return int8(data[0]), nil
	case "uint8":
		return data[0], nil
	case "int16":
		if len(data) < 2 {
			return nil, fmt.Errorf("need 2 bytes for int16")
		}
		var buf [2]byte
		copy(buf[:], data[:2])
		if addr != nil && addr.Endian == EndianLittle {
			// #nosec G115 -- 16 位按补码位模式重解释为有符号数
			return int16(binary.LittleEndian.Uint16(buf[:])), nil
		}
		// #nosec G115 -- 16 位按补码位模式重解释为有符号数
		return int16(endian.Uint16(buf[:])), nil
	case "uint16":
		if len(data) < 2 {
			return nil, fmt.Errorf("need 2 bytes for uint16")
		}
		var buf [2]byte
		copy(buf[:], data[:2])
		if addr != nil && addr.Endian == EndianLittle {
			return binary.LittleEndian.Uint16(buf[:]), nil
		}
		return endian.Uint16(buf[:]), nil
	case "int32":
		if len(data) < 4 {
			return nil, fmt.Errorf("need 4 bytes for int32")
		}
		var buf [4]byte
		copy(buf[:], data[:4])
		if addr != nil && addr.Endian == EndianLittle {
			// #nosec G115 -- 32 位按补码位模式重解释为有符号数
			return int32(binary.LittleEndian.Uint32(buf[:])), nil
		}
		// #nosec G115 -- 32 位按补码位模式重解释为有符号数
		return int32(endian.Uint32(buf[:])), nil
	case "uint32":
		if len(data) < 4 {
			return nil, fmt.Errorf("need 4 bytes for uint32")
		}
		var buf [4]byte
		copy(buf[:], data[:4])
		if addr != nil && addr.Endian == EndianLittle {
			return binary.LittleEndian.Uint32(buf[:]), nil
		}
		return endian.Uint32(buf[:]), nil
	case "int64":
		if len(data) < 8 {
			return nil, fmt.Errorf("need 8 bytes for int64")
		}
		var buf [8]byte
		copy(buf[:], data[:8])
		if addr != nil && addr.Endian == EndianLittle {
			// #nosec G115 -- 64 位按补码位模式重解释为有符号数
			return int64(binary.LittleEndian.Uint64(buf[:])), nil
		}
		// #nosec G115 -- 64 位按补码位模式重解释为有符号数
		return int64(endian.Uint64(buf[:])), nil
	case "uint64":
		if len(data) < 8 {
			return nil, fmt.Errorf("need 8 bytes for uint64")
		}
		var buf [8]byte
		copy(buf[:], data[:8])
		if addr != nil && addr.Endian == EndianLittle {
			return binary.LittleEndian.Uint64(buf[:]), nil
		}
		return endian.Uint64(buf[:]), nil
	case "float", "float32":
		if len(data) < 4 {
			return nil, fmt.Errorf("need 4 bytes for float")
		}
		var bits uint32
		if addr != nil && addr.Endian == EndianLittle {
			bits = binary.LittleEndian.Uint32(data[:4])
		} else {
			bits = endian.Uint32(data[:4])
		}
		return math.Float32frombits(bits), nil
	case "double", "float64":
		if len(data) < 8 {
			return nil, fmt.Errorf("need 8 bytes for double")
		}
		var bits uint64
		if addr != nil && addr.Endian == EndianLittle {
			bits = binary.LittleEndian.Uint64(data[:8])
		} else {
			bits = endian.Uint64(data[:8])
		}
		return math.Float64frombits(bits), nil
	default:
		return nil, fmt.Errorf("unsupported profinet-io datatype: %s", dataType)
	}
}

func (d *ProfinetDecoder) EncodeValue(value any, dataType string, addr *ParsedAddress) ([]byte, error) {
	dt := strings.ToLower(strings.TrimSpace(dataType))
	size := ByteSize(dt)
	buf := make([]byte, size)
	var endian binary.ByteOrder = binary.BigEndian
	if addr != nil && addr.Endian == EndianLittle {
		endian = binary.LittleEndian
	}

	switch dt {
	case "bit", "bool":
		b, err := toBool(value)
		if err != nil {
			return nil, err
		}
		bit := 0
		if addr != nil && addr.IsBit {
			bit = addr.Bit
		}
		if b {
			buf[0] = 1 << uint(bit)
		}
	case "int8", "uint8":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		buf[0] = byte(n & 0xFF)
	case "int16", "uint16":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		if addr != nil && addr.Endian == EndianLittle {
			binary.LittleEndian.PutUint16(buf, uint16(n&0xFFFF))
		} else {
			endian.PutUint16(buf, uint16(n&0xFFFF))
		}
	case "int32", "uint32":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		if addr != nil && addr.Endian == EndianLittle {
			binary.LittleEndian.PutUint32(buf, uint32(n&0xFFFFFFFF))
		} else {
			endian.PutUint32(buf, uint32(n&0xFFFFFFFF))
		}
	case "int64", "uint64":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		if addr != nil && addr.Endian == EndianLittle {
			// #nosec G115 -- 64 位按补码位模式编码为无符号数
			binary.LittleEndian.PutUint64(buf, uint64(n))
		} else {
			// #nosec G115 -- 64 位按补码位模式编码为无符号数
			endian.PutUint64(buf, uint64(n))
		}
	case "float", "float32":
		f, err := toFloat64(value)
		if err != nil {
			return nil, err
		}
		bits := math.Float32bits(float32(f))
		if addr != nil && addr.Endian == EndianLittle {
			binary.LittleEndian.PutUint32(buf, bits)
		} else {
			endian.PutUint32(buf, bits)
		}
	case "double", "float64":
		f, err := toFloat64(value)
		if err != nil {
			return nil, err
		}
		bits := math.Float64bits(f)
		if addr != nil && addr.Endian == EndianLittle {
			binary.LittleEndian.PutUint64(buf, bits)
		} else {
			endian.PutUint64(buf, bits)
		}
	default:
		return nil, fmt.Errorf("unsupported profinet-io datatype for write: %s", dataType)
	}
	return buf, nil
}

func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return val == "true" || val == "1", nil
	case float64:
		return val != 0, nil
	case int:
		return val != 0, nil
	default:
		return fmt.Sprintf("%v", v) == "true", nil
	}
}

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
	case float64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	case bool:
		if val {
			return 1, nil
		}
		return 0, nil
	case string:
		if val == "" {
			return 0, nil
		}
		var n int64
		_, err := fmt.Sscan(val, &n)
		return n, err
	default:
		return 0, fmt.Errorf("cannot convert %T to integer", v)
	}
}

func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		var f float64
		_, err := fmt.Sscan(val, &f)
		return f, err
	default:
		return 0, fmt.Errorf("cannot convert %T to float", v)
	}
}
