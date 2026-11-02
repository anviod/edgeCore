package knxnetip

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// KNXDecoder handles group address value encoding/decoding.
type KNXDecoder struct{}

func NewKNXDecoder() *KNXDecoder {
	return &KNXDecoder{}
}

func (d *KNXDecoder) ParseAddress(addr string) (*ParsedAddress, error) {
	return ParseAddress(addr)
}

func DecodeValue(data []byte, dataType string, addr *ParsedAddress, scale, offset float64) (any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty KNX payload")
	}

	dt := strings.ToUpper(strings.TrimSpace(dataType))
	switch dt {
	case "BIT", "BOOL":
		if addr != nil && addr.BitWidth > 0 && addr.BitWidth < 8 {
			mask := byte((1 << addr.BitWidth) - 1)
			shift := 8 - addr.BitWidth
			val := (data[0] >> shift) & mask
			return val != 0, nil
		}
		return (data[0] & 0x01) != 0, nil
	case "INT8":
		// #nosec G115 -- 单字节按补码位模式重解释为有符号数
		v := int8(data[0])
		return applyScale(float64(v), scale, offset), nil
	case "UINT8":
		v := data[0]
		return applyScale(float64(v), scale, offset), nil
	case "INT16":
		if len(data) < 2 {
			return nil, fmt.Errorf("need 2 bytes for INT16")
		}
		// #nosec G115 -- 16 位按补码位模式重解释为有符号数
		v := int16(binary.BigEndian.Uint16(data[:2]))
		return applyScale(float64(v), scale, offset), nil
	case "UINT16":
		if len(data) < 2 {
			return nil, fmt.Errorf("need 2 bytes for UINT16")
		}
		v := binary.BigEndian.Uint16(data[:2])
		return applyScale(float64(v), scale, offset), nil
	case "INT32":
		if len(data) < 4 {
			return nil, fmt.Errorf("need 4 bytes for INT32")
		}
		// #nosec G115 -- 32 位按补码位模式重解释为有符号数
		v := int32(binary.BigEndian.Uint32(data[:4]))
		return applyScale(float64(v), scale, offset), nil
	case "UINT32":
		if len(data) < 4 {
			return nil, fmt.Errorf("need 4 bytes for UINT32")
		}
		v := binary.BigEndian.Uint32(data[:4])
		return applyScale(float64(v), scale, offset), nil
	case "FLOAT":
		if len(data) >= 4 {
			bits := binary.BigEndian.Uint32(data[:4])
			return applyScale(float64(math.Float32frombits(bits)), scale, offset), nil
		}
		if len(data) >= 2 {
			return applyScale(decodeDPT9(data[:2]), scale, offset), nil
		}
		return nil, fmt.Errorf("need at least 2 bytes for FLOAT")
	default:
		return nil, fmt.Errorf("unsupported datatype: %s", dataType)
	}
}

func EncodeValue(value any, dataType string, addr *ParsedAddress) ([]byte, error) {
	dt := strings.ToUpper(strings.TrimSpace(dataType))
	switch dt {
	case "BIT", "BOOL":
		b, err := toBool(value)
		if err != nil {
			return nil, err
		}
		if addr != nil && addr.BitWidth > 0 && addr.BitWidth < 8 {
			shift := 8 - addr.BitWidth
			if b {
				return []byte{byte(1 << shift)}, nil
			}
			return []byte{0}, nil
		}
		if b {
			return []byte{0x01}, nil
		}
		return []byte{0x00}, nil
	case "INT8", "UINT8":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		return []byte{byte(n & 0xFF)}, nil
	case "INT16", "UINT16":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(n&0xFFFF))
		return buf, nil
	case "INT32", "UINT32":
		n, err := toInt64(value)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(n&0xFFFFFFFF))
		return buf, nil
	case "FLOAT":
		f, err := toFloat64(value)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, math.Float32bits(float32(f)))
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported write datatype: %s", dataType)
	}
}

func applyScale(v, scale, offset float64) float64 {
	if scale == 0 {
		scale = 1
	}
	return v*scale + offset
}

func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case float64:
		return val != 0, nil
	case float32:
		return val != 0, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "on":
			return true, nil
		case "0", "false", "off":
			return false, nil
		}
	}
	return false, fmt.Errorf("unsupported bool value: %v", v)
}

func toInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int64:
		return val, nil
	case float64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	}
	return 0, fmt.Errorf("unsupported integer value: %v", v)
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
	}
	return 0, fmt.Errorf("unsupported float value: %v", v)
}

// decodeDPT9 decodes KNX 2-byte float (DPT 9.xxx).
func decodeDPT9(data []byte) float64 {
	if len(data) < 2 {
		return 0
	}
	raw := binary.BigEndian.Uint16(data)
	sign := (raw >> 15) & 0x01
	exp := (raw >> 11) & 0x0F
	mant := raw & 0x07FF
	if sign == 1 {
		mant = ^mant
	}
	value := float64(mant) * math.Pow(2, float64(exp))
	if sign == 1 {
		value = -value
	}
	return value
}
