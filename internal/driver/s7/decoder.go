package s7

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/anviod/gos7"
)

// S7Area S7地址区域信息
type S7Area struct {
	Area     int  // S7AreaDB, S7AreaMK, etc.
	DBNumber int  // 数据块号（仅DB区域有效）
	ByteOff  int  // 字节偏移
	BitOff   int  // 位偏移（仅位操作有效）
	WordLen  int  // 字长：S7WLBit, S7WLByte, S7WLWord, S7WLDWord, S7WLReal
	IsBit    bool // 是否为位操作
}

// S7Decoder S7地址解码器
type S7Decoder struct {
	helper gos7.Helper
}

// NewS7Decoder 创建S7解码器
func NewS7Decoder() *S7Decoder {
	return &S7Decoder{}
}

// 地址正则表达式
var (
	// DB地址: DB1.DBD0, DB1.DBW2, DB1.DBX0.1, DB1.DBB4
	reDB = regexp.MustCompile(`^DB(\d+)\.DB([DWBX])(\d+)(?:\.(\d+))?$`)

	// M区地址: M0.0, MD0, MW0, MB0
	reM = regexp.MustCompile(`^M([DWB]?)(\d+)(?:\.(\d+))?$`)

	// I区地址: I0.0, ID0, IW0, IB0
	reI = regexp.MustCompile(`^I([DWB]?)(\d+)(?:\.(\d+))?$`)

	// Q区地址: Q0.0, QD0, QW0, QB0
	reQ = regexp.MustCompile(`^Q([DWB]?)(\d+)(?:\.(\d+))?$`)

	// T区地址: T0
	reT = regexp.MustCompile(`^T(\d+)$`)

	// C区地址: C0
	reC = regexp.MustCompile(`^C(\d+)$`)
)

// ParseAddress 解析S7地址字符串
// 支持格式：
//
//	DB1.DBD0    -> DB双字 (float32/int32/uint32)
//	DB1.DBW2    -> DB字 (int16/uint16)
//	DB1.DBX0.1  -> DB位 (bool)
//	DB1.DBB4    -> DB字节 (int8/uint8)
//	M0.0        -> M区位 (bool)
//	MD0         -> M区双字
//	MW0         -> M区字
//	MB0         -> M区字节
//	I0.0        -> 输入位 (bool)
//	ID0         -> 输入双字
//	Q0.0        -> 输出位 (bool)
//	QD0         -> 输出双字
//	T0          -> 定时器
//	C0          -> 计数器
func (d *S7Decoder) ParseAddress(addr string) (*S7Area, error) {
	addr = strings.TrimSpace(strings.ToUpper(addr))

	// DB地址
	if m := reDB.FindStringSubmatch(addr); m != nil {
		dbNum, _ := strconv.Atoi(m[1])
		typeCode := m[2]
		offset, _ := strconv.Atoi(m[3])
		bitOff := 0
		if m[4] != "" {
			bitOff, _ = strconv.Atoi(m[4])
		}

		area := &S7Area{
			Area:     S7AreaDB,
			DBNumber: dbNum,
			ByteOff:  offset,
		}

		switch typeCode {
		case "D":
			area.WordLen = S7WLDWord
		case "W":
			area.WordLen = S7WLWord
		case "B":
			area.WordLen = S7WLByte
		case "X":
			area.WordLen = S7WLBit
			area.IsBit = true
			area.BitOff = bitOff
		}

		return area, nil
	}

	// M区地址
	if m := reM.FindStringSubmatch(addr); m != nil {
		return d.parseMerkerArea(m[1], m[2], m[3], S7AreaMK)
	}

	// I区地址
	if m := reI.FindStringSubmatch(addr); m != nil {
		return d.parseMerkerArea(m[1], m[2], m[3], S7AreaPE)
	}

	// Q区地址
	if m := reQ.FindStringSubmatch(addr); m != nil {
		return d.parseMerkerArea(m[1], m[2], m[3], S7AreaPA)
	}

	// T区地址
	if m := reT.FindStringSubmatch(addr); m != nil {
		offset, _ := strconv.Atoi(m[1])
		return &S7Area{
			Area:    S7AreaTM,
			ByteOff: offset,
			WordLen: S7WLTimer,
		}, nil
	}

	// C区地址
	if m := reC.FindStringSubmatch(addr); m != nil {
		offset, _ := strconv.Atoi(m[1])
		return &S7Area{
			Area:    S7AreaCT,
			ByteOff: offset,
			WordLen: S7WLCounter,
		}, nil
	}

	return nil, fmt.Errorf("unsupported S7 address format: %s", addr)
}

// parseMerkerArea 解析M/I/Q区地址
func (d *S7Decoder) parseMerkerArea(typeCode, offsetStr, bitStr string, areaCode int) (*S7Area, error) {
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return nil, fmt.Errorf("invalid offset: %s", offsetStr)
	}

	area := &S7Area{
		Area:    areaCode,
		ByteOff: offset,
	}

	switch typeCode {
	case "D":
		area.WordLen = S7WLDWord
	case "W":
		area.WordLen = S7WLWord
	case "B":
		area.WordLen = S7WLByte
	case "":
		// 无前缀，默认为位地址 (如 M0.0)
		bitOff := 0
		if bitStr != "" {
			bitOff, err = strconv.Atoi(bitStr)
			if err != nil {
				return nil, fmt.Errorf("invalid bit offset: %s", bitStr)
			}
		}
		area.WordLen = S7WLBit
		area.IsBit = true
		area.BitOff = bitOff
	default:
		return nil, fmt.Errorf("unsupported type prefix: %s", typeCode)
	}

	return area, nil
}

// ReadSizeForArea 根据区域信息确定需要读取的字节数
func (d *S7Decoder) ReadSizeForArea(area *S7Area) int {
	switch area.WordLen {
	case S7WLBit:
		return 1 // 位操作需要读取至少1字节
	case S7WLByte:
		return 1
	case S7WLWord:
		return 2
	case S7WLDWord, S7WLReal:
		return 4
	case S7WLCounter, S7WLTimer:
		return 2
	default:
		return 1
	}
}

// DecodeValue 从字节缓冲区解码值
func (d *S7Decoder) DecodeValue(buffer []byte, area *S7Area, dataType string) (interface{}, error) {
	if len(buffer) == 0 {
		return nil, fmt.Errorf("empty buffer")
	}

	switch dataType {
	case "bool":
		if area.IsBit {
			return d.helper.GetBoolAt(buffer[0], uint(area.BitOff)), nil
		}
		return buffer[0] != 0, nil

	case "uint8":
		return buffer[0], nil

	case "int8":
		return int8(buffer[0]), nil

	case "uint16":
		if len(buffer) < 2 {
			return nil, fmt.Errorf("buffer too short for uint16: need 2, got %d", len(buffer))
		}
		var val uint16
		d.helper.GetValueAt(buffer, 0, &val)
		return val, nil

	case "int16":
		if len(buffer) < 2 {
			return nil, fmt.Errorf("buffer too short for int16: need 2, got %d", len(buffer))
		}
		var val int16
		d.helper.GetValueAt(buffer, 0, &val)
		return val, nil

	case "uint32":
		if len(buffer) < 4 {
			return nil, fmt.Errorf("buffer too short for uint32: need 4, got %d", len(buffer))
		}
		var val uint32
		d.helper.GetValueAt(buffer, 0, &val)
		return val, nil

	case "int32":
		if len(buffer) < 4 {
			return nil, fmt.Errorf("buffer too short for int32: need 4, got %d", len(buffer))
		}
		var val int32
		d.helper.GetValueAt(buffer, 0, &val)
		return val, nil

	case "float32", "float":
		if len(buffer) < 4 {
			return nil, fmt.Errorf("buffer too short for float32: need 4, got %d", len(buffer))
		}
		return d.helper.GetRealAt(buffer, 0), nil

	case "float64", "double":
		if len(buffer) < 8 {
			return nil, fmt.Errorf("buffer too short for float64: need 8, got %d", len(buffer))
		}
		return d.helper.GetLRealAt(buffer, 0), nil

	case "string":
		// S7字符串格式：第一字节=最大长度，第二字节=实际长度，后续为内容
		if len(buffer) < 2 {
			return string(buffer), nil
		}
		actualLen := int(buffer[1])
		if actualLen+2 > len(buffer) {
			actualLen = len(buffer) - 2
		}
		return string(buffer[2 : 2+actualLen]), nil

	default:
		return nil, fmt.Errorf("unsupported data type: %s", dataType)
	}
}

// EncodeValue 将值编码到字节缓冲区
func (d *S7Decoder) EncodeValue(buffer []byte, area *S7Area, dataType string, value interface{}) error {
	switch dataType {
	case "bool":
		var b bool
		switch v := value.(type) {
		case bool:
			b = v
		case string:
			b = v == "true" || v == "1"
		default:
			b = fmt.Sprintf("%v", value) == "true"
		}
		if area.IsBit {
			// 位操作：先读取原始字节，修改指定位
			buffer[0] = d.helper.SetBoolAt(buffer[0], uint(area.BitOff), b)
		} else {
			if b {
				buffer[0] = 1
			} else {
				buffer[0] = 0
			}
		}

	case "uint8":
		switch v := value.(type) {
		case uint8:
			buffer[0] = v
		default:
			buffer[0] = toUint8(value)
		}

	case "int8":
		switch v := value.(type) {
		case int8:
			buffer[0] = byte(v)
		default:
			buffer[0] = byte(toInt8(value))
		}

	case "uint16":
		var val uint16
		switch v := value.(type) {
		case uint16:
			val = v
		default:
			val = toUint16(value)
		}
		d.helper.SetValueAt(buffer, 0, val)

	case "int16":
		var val int16
		switch v := value.(type) {
		case int16:
			val = v
		default:
			val = toInt16(value)
		}
		d.helper.SetValueAt(buffer, 0, val)

	case "uint32":
		var val uint32
		switch v := value.(type) {
		case uint32:
			val = v
		default:
			val = toUint32(value)
		}
		d.helper.SetValueAt(buffer, 0, val)

	case "int32":
		var val int32
		switch v := value.(type) {
		case int32:
			val = v
		default:
			val = toInt32(value)
		}
		d.helper.SetValueAt(buffer, 0, val)

	case "float32", "float":
		var val float32
		switch v := value.(type) {
		case float32:
			val = v
		default:
			val = toFloat32(value)
		}
		d.helper.SetRealAt(buffer, 0, val)

	case "float64", "double":
		var val float64
		switch v := value.(type) {
		case float64:
			val = v
		default:
			val = toFloat64(value)
		}
		d.helper.SetLRealAt(buffer, 0, val)

	default:
		return fmt.Errorf("unsupported data type for write: %s", dataType)
	}

	return nil
}

// DataTypeToWordLen 将数据类型字符串映射到S7 WordLen
func DataTypeToWordLen(dataType string) int {
	switch strings.ToLower(dataType) {
	case "bool":
		return S7WLBit
	case "uint8", "int8", "byte":
		return S7WLByte
	case "uint16", "int16", "word":
		return S7WLWord
	case "uint32", "int32", "dword":
		return S7WLDWord
	case "float32", "float", "real":
		return S7WLReal
	case "float64", "double", "lreal":
		return S7WLDWord // 读取4字节，由Helper转为float64
	default:
		return S7WLByte
	}
}

// 类型转换辅助函数
func toUint8(v interface{}) uint8 {
	switch val := v.(type) {
	case float64:
		return uint8(val)
	case int:
		return uint8(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 8)
		return uint8(n)
	default:
		return 0
	}
}

func toInt8(v interface{}) int8 {
	switch val := v.(type) {
	case float64:
		return int8(val)
	case int:
		return int8(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 8)
		return int8(n)
	default:
		return 0
	}
}

func toUint16(v interface{}) uint16 {
	switch val := v.(type) {
	case float64:
		return uint16(val)
	case int:
		return uint16(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 16)
		return uint16(n)
	default:
		return 0
	}
}

// clampInt16 将数值饱和钳制到 int16 可表示范围（防御性收窄，gosec G115）。
func clampInt16(v float64) int16 {
	if v != v || v < math.MinInt16 {
		return math.MinInt16
	}
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(v)
}

// clampInt32 将数值饱和钳制到 int32 可表示范围。
func clampInt32(v float64) int32 {
	if v != v || v < math.MinInt32 {
		return math.MinInt32
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

// clampUint32 将数值饱和钳制到 uint32 可表示范围。
func clampUint32(v float64) uint32 {
	if v != v || v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}

func toInt16(v interface{}) int16 {
	switch val := v.(type) {
	case float64:
		return clampInt16(val)
	case int:
		if val < math.MinInt16 {
			return math.MinInt16
		}
		if val > math.MaxInt16 {
			return math.MaxInt16
		}
		return int16(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 16)
		return int16(n)
	default:
		return 0
	}
}

func toUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case float64:
		return clampUint32(val)
	case int:
		if val < 0 {
			return 0
		}
		if val > math.MaxUint32 {
			return math.MaxUint32
		}
		return uint32(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 32)
		return uint32(n)
	default:
		return 0
	}
}

func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case float64:
		return clampInt32(val)
	case int:
		if val < math.MinInt32 {
			return math.MinInt32
		}
		if val > math.MaxInt32 {
			return math.MaxInt32
		}
		return int32(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 32)
		return int32(n)
	default:
		return 0
	}
}

func toFloat32(v interface{}) float32 {
	switch val := v.(type) {
	case float64:
		return float32(val)
	case int:
		return float32(val)
	case string:
		n, _ := strconv.ParseFloat(val, 32)
		return float32(n)
	default:
		return 0
	}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		n, _ := strconv.ParseFloat(val, 64)
		return n
	default:
		return 0
	}
}
