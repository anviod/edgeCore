package mitsubishi

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type MCDecoder struct{}

func NewMCDecoder() *MCDecoder {
	return &MCDecoder{}
}

func (d *MCDecoder) ReadSize(dataType string, addr *MCAddress) (byteLen int, isBit bool) {
	dt := strings.ToUpper(strings.TrimSpace(dataType))
	if addr.StringLen > 0 {
		return addr.StringLen, false
	}
	if addr.BitOffset >= 0 && !addr.IsBit {
		return 2, false
	}
	if addr.IsBit || dt == "BIT" || dt == "BOOL" {
		return 1, true
	}
	switch dt {
	case "INT8", "UINT8":
		return 1, false
	case "INT16", "UINT16":
		return 2, false
	case "INT32", "UINT32", "FLOAT", "FLOAT32":
		return 4, false
	case "INT64", "UINT64", "DOUBLE", "FLOAT64":
		return 8, false
	default:
		return 2, false
	}
}

func (d *MCDecoder) DecodeValue(data []byte, addr *MCAddress, dataType string) (interface{}, error) {
	dt := strings.ToUpper(strings.TrimSpace(dataType))

	if addr.StringLen > 0 {
		return decodeString(data, addr.StringLen, addr.StringHigh), nil
	}

	if addr.BitOffset >= 0 && !addr.IsBit {
		if len(data) < 2 {
			return nil, fmt.Errorf("insufficient data for word bit read")
		}
		word := binary.LittleEndian.Uint16(data[:2])
		bit := (word >> addr.BitOffset) & 1
		return bit == 1, nil
	}

	if addr.IsBit || dt == "BIT" || dt == "BOOL" {
		if len(data) < 1 {
			return nil, fmt.Errorf("insufficient data for bit read")
		}
		return (data[0] & 0x10) != 0, nil
	}

	switch dt {
	case "INT8":
		if len(data) < 1 {
			return nil, fmt.Errorf("insufficient data")
		}
		// #nosec G115 -- 单字节按补码位模式重解释为有符号数
		return int8(data[0]), nil
	case "UINT8":
		if len(data) < 1 {
			return nil, fmt.Errorf("insufficient data")
		}
		return data[0], nil
	case "INT16":
		if len(data) < 2 {
			return nil, fmt.Errorf("insufficient data")
		}
		// #nosec G115 -- 16 位按补码位模式重解释为有符号数
		return int16(binary.LittleEndian.Uint16(data[:2])), nil
	case "UINT16":
		if len(data) < 2 {
			return nil, fmt.Errorf("insufficient data")
		}
		return binary.LittleEndian.Uint16(data[:2]), nil
	case "INT32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data")
		}
		// #nosec G115 -- 32 位按补码位模式重解释为有符号数
		return int32(binary.LittleEndian.Uint32(data[:4])), nil
	case "UINT32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data")
		}
		return binary.LittleEndian.Uint32(data[:4]), nil
	case "INT64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data")
		}
		// #nosec G115 -- 64 位按补码位模式重解释为有符号数
		return int64(binary.LittleEndian.Uint64(data[:8])), nil
	case "UINT64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data")
		}
		return binary.LittleEndian.Uint64(data[:8]), nil
	case "FLOAT", "FLOAT32":
		if len(data) < 4 {
			return nil, fmt.Errorf("insufficient data")
		}
		bits := binary.LittleEndian.Uint32(data[:4])
		return math.Float32frombits(bits), nil
	case "DOUBLE", "FLOAT64":
		if len(data) < 8 {
			return nil, fmt.Errorf("insufficient data")
		}
		bits := binary.LittleEndian.Uint64(data[:8])
		return math.Float64frombits(bits), nil
	default:
		if len(data) < 2 {
			return nil, fmt.Errorf("insufficient data")
		}
		// #nosec G115 -- 16 位按补码位模式重解释为有符号数
		return int16(binary.LittleEndian.Uint16(data[:2])), nil
	}
}

func (d *MCDecoder) EncodeValue(addr *MCAddress, dataType string, value interface{}) ([]byte, bool, error) {
	dt := strings.ToUpper(strings.TrimSpace(dataType))

	if addr.BitOffset >= 0 && !addr.IsBit {
		return nil, false, fmt.Errorf("bit-within-word write not supported for %s", addr.DeviceName)
	}

	if addr.IsBit || dt == "BIT" || dt == "BOOL" {
		b, err := toBool(value)
		if err != nil {
			return nil, true, err
		}
		if b {
			return []byte{0x10}, true, nil
		}
		return []byte{0x00}, true, nil
	}

	switch dt {
	case "INT8":
		v, err := toInt64(value)
		if err != nil {
			return nil, false, err
		}
		return []byte{byte(v & 0xFF)}, false, nil
	case "UINT8":
		v, err := toUint64(value)
		if err != nil {
			return nil, false, err
		}
		return []byte{byte(v & 0xFF)}, false, nil
	case "INT16":
		v, err := toInt64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(v&0xFFFF))
		return buf, false, nil
	case "UINT16":
		v, err := toUint64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(v&0xFFFF))
		return buf, false, nil
	case "INT32":
		v, err := toInt64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(v&0xFFFFFFFF))
		return buf, false, nil
	case "UINT32":
		v, err := toUint64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(v&0xFFFFFFFF))
		return buf, false, nil
	case "FLOAT", "FLOAT32":
		v, err := toFloat64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(v)))
		return buf, false, nil
	case "DOUBLE", "FLOAT64":
		v, err := toFloat64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, math.Float64bits(v))
		return buf, false, nil
	default:
		v, err := toInt64(value)
		if err != nil {
			return nil, false, err
		}
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(v&0xFFFF))
		return buf, false, nil
	}
}

func decodeString(data []byte, length int, highFirst bool) string {
	if length <= 0 || len(data) == 0 {
		return ""
	}
	n := length
	if n > len(data) {
		n = len(data)
	}
	runes := make([]rune, 0, n/2+1)
	if highFirst {
		for i := 0; i+1 < n; i += 2 {
			runes = append(runes, rune(data[i])<<8|rune(data[i+1]))
		}
	} else {
		for i := 0; i+1 < n; i += 2 {
			runes = append(runes, rune(data[i+1])<<8|rune(data[i]))
		}
	}
	return strings.TrimRight(string(runes), "\x00")
}

func toBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case float64:
		return val != 0, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "on":
			return true, nil
		case "0", "false", "off":
			return false, nil
		default:
			return false, fmt.Errorf("invalid bool value: %s", val)
		}
	default:
		return false, fmt.Errorf("unsupported bool type %T", v)
	}
}

func toInt64(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int:
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
	default:
		return 0, fmt.Errorf("unsupported int type %T", v)
	}
}

func toUint64(v interface{}) (uint64, error) {
	switch val := v.(type) {
	case uint:
		return uint64(val), nil
	case uint16:
		return uint64(val), nil
	case uint32:
		return uint64(val), nil
	case uint64:
		return val, nil
	case int:
		if val < 0 {
			return 0, fmt.Errorf("negative uint value")
		}
		return uint64(val), nil
	case float64:
		if val < 0 {
			return 0, fmt.Errorf("negative uint value")
		}
		return uint64(val), nil
	default:
		return 0, fmt.Errorf("unsupported uint type %T", v)
	}
}

func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported float type %T", v)
	}
}
