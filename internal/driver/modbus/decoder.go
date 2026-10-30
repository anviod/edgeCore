package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/pkg/dataformat"
)

// Decoder 接口定义
type Decoder interface {
	Decode(point model.Point, raw []byte) (any, string, error)
	Encode(point model.Point, value any) ([]uint16, error)
	ParseAddress(addr string) (model.RegisterType, uint16, error)
	GetRegisterCount(dataType string) uint16
	GetPointRegisterCount(point model.Point) uint16
}

// PointDecoder 实现 Decoder 接口
type PointDecoder struct {
	byteOrder4    string
	startAddress  int
	useDataformat bool
	addressBase   uint16
}

func NewPointDecoder(byteOrder4 string, startAddress int, addressBase uint16) *PointDecoder {
	if byteOrder4 == "" {
		byteOrder4 = "ABCD"
	}
	return &PointDecoder{
		byteOrder4:   byteOrder4,
		startAddress: startAddress,
		addressBase:  addressBase,
	}
}

func (d *PointDecoder) EnableDataformatDecoder(enable bool) {
	d.useDataformat = enable
}

// ParseAddress 解析Modbus点位地址
// 支持两种格式：
// 1. 标准格式：40001-50000(Holding), 30001-40000(Input), 10001-20000(Discrete), 1-10000(Coil)
// 2. 裸地址格式：直接使用偏移量，需配合register_type字段
// 3. 范围格式：如 "0-1" 表示从地址0开始的连续寄存器
func (d *PointDecoder) ParseAddress(addr string) (model.RegisterType, uint16, error) {
	addr = strings.TrimSpace(addr)

	// 处理范围地址格式，如 "0-1" 或 "1-2"
	if strings.Contains(addr, "-") {
		parts := strings.Split(addr, "-")
		if len(parts) > 0 {
			addr = strings.TrimSpace(parts[0])
		}
	}

	addrInt, err := strconv.Atoi(addr)
	if err != nil {
		return model.RegHolding, 0, fmt.Errorf("invalid address format: %s", addr)
	}

	var regType model.RegisterType
	var offset uint16

	// 严格按照Modbus标准地址范围解析
	// Device address ranges:
	// 1-10000: Coil (outputs) - Function 01 R/W, offset = address - 1
	// 10001-20000: Discrete Inputs - Function 02 Read, offset = address - 10001
	// 30001-40000: Input Registers - Function 04 Read, offset = address - 30001
	// 40001-50000: Holding Registers - Function 03 R/W, offset = address - 40001
	switch {
	case addrInt >= 40001 && addrInt <= 50000:
		// Holding Register (4xxxx) - Function 03
		regType = model.RegHolding
		offset = uint16(addrInt - 40001)
	case addrInt >= 30001 && addrInt <= 40000:
		// Input Register (3xxxx) - Function 04
		regType = model.RegInput
		offset = uint16(addrInt - 30001)
	case addrInt >= 10001 && addrInt <= 20000:
		// Discrete Input (1xxxx) - Function 02
		regType = model.RegDiscreteInput
		offset = uint16(addrInt - 10001)
	case addrInt >= 1001 && addrInt <= 10000:
		// Coil (0xxxx) - Function 01 (only for addresses 1001-10000)
		regType = model.RegCoil
		offset = uint16(addrInt - 1)
	default:
		// 非标地址：默认作为Holding Register处理
		// 用户可以通过register_type字段指定其他类型
		regType = model.RegHolding
		// 使用有符号运算避免 uint16 下溢（当 addrInt < addressBase 时）
		rawOffset := addrInt - int(d.addressBase)
		if rawOffset < 0 {
			rawOffset = 0
		}
		if rawOffset > 0xFFFF {
			return model.RegHolding, 0, fmt.Errorf("address %d out of range (base=%d)", addrInt, d.addressBase)
		}
		offset = uint16(rawOffset)
	}

	return regType, offset, nil
}

// GetRegisterCount 根据数据类型获取占用的寄存器数
func (d *PointDecoder) GetRegisterCount(dataType string) uint16 {
	switch dataType {
	case "float32", "int32", "uint32":
		return 2
	case "int64", "uint64", "float64":
		return 4
	default:
		return 1
	}
}

// GetPointRegisterCount returns the register width of the raw value.
// ParseType takes precedence so newly saved points cannot read a different width
// from the type shown in the point configuration.
func (d *PointDecoder) GetPointRegisterCount(point model.Point) uint16 {
	if point.ParseType != "" {
		switch strings.ToUpper(point.ParseType) {
		case "FLOAT32", "INT32", "UINT32", "BCD32":
			return 2
		case "FLOAT64", "INT64", "UINT64":
			return 4
		case "INT16", "UINT16", "BCD16", "FLOAT16":
			return 1
		}
	}
	return d.GetRegisterCount(point.DataType)
}

// Decode 解码原始字节数据
func (d *PointDecoder) Decode(point model.Point, raw []byte) (any, string, error) {
	if d.useDataformat {
		return d.decodeWithDataformat(point, raw)
	}
	val, err := d.decodeRaw(point, raw)
	if err != nil {
		return nil, "Bad", err
	}

	if point.ReadFormula != "" {
		val, err = d.applyFormula(point.ReadFormula, val)
		if err != nil {
			return nil, "Bad", err
		}
	} else {
		val = d.applyScaleOffset(point, val)
	}

	// TODO: 可以添加范围检查以确定 Quality
	return val, "Good", nil
}

func (d *PointDecoder) decodeRaw(point model.Point, b []byte) (any, error) {
	if len(b) < 2 {
		return nil, fmt.Errorf("not enough bytes")
	}

	dataType := point.DataType
	if point.ParseType != "" {
		switch strings.ToUpper(point.ParseType) {
		case "INT16":
			dataType = "int16"
		case "UINT16":
			dataType = "uint16"
		case "INT32":
			dataType = "int32"
		case "UINT32":
			dataType = "uint32"
		case "FLOAT32":
			dataType = "float32"
		case "FLOAT64":
			dataType = "float64"
		}
	}

	switch dataType {
	case "int16":
		// 16 位寄存器值按补码位模式重解释为 int16，是 Modbus int16 点型的定义行为
		//（如 0xFFFF 读作 -1），非溢出。
		if swapsBytes16(point) {
			// #nosec G115 -- 位模式补码重解释
			return int16(binary.LittleEndian.Uint16(b)), nil
		}
		// #nosec G115 -- 位模式补码重解释
		return int16(binary.BigEndian.Uint16(b)), nil
	case "uint16":
		if swapsBytes16(point) {
			return binary.LittleEndian.Uint16(b), nil
		}
		return binary.BigEndian.Uint16(b), nil
	case "float32":
		if len(b) < 4 {
			return nil, fmt.Errorf("not enough bytes for float32")
		}
		orderedBytes := d.applyByteOrder(b)
		bits := binary.BigEndian.Uint32(orderedBytes)
		return math.Float32frombits(bits), nil
	case "int32":
		if len(b) < 4 {
			return nil, fmt.Errorf("not enough bytes for int32")
		}
		orderedBytes := d.applyByteOrder(b)
		// #nosec G115 -- 32 位寄存器值按补码位模式重解释为 int32，同 int16 点型定义行为
		return int32(binary.BigEndian.Uint32(orderedBytes)), nil
	case "uint32":
		if len(b) < 4 {
			return nil, fmt.Errorf("not enough bytes for uint32")
		}
		orderedBytes := d.applyByteOrder(b)
		return binary.BigEndian.Uint32(orderedBytes), nil
	default:
		return binary.BigEndian.Uint16(b), nil
	}
}

func (d *PointDecoder) decodeWithDataformat(point model.Point, b []byte) (any, string, error) {
	cfg := dataformat.ResolvePointFormat(point, d.byteOrder4)
	order := cfg.Order

	// 16 位点位若自身配置了会交换字节的 word order（如 UI 的 BA → DCBA），
	// dataformat 包的 Uint16 恒按大端解析，这里在解析前先行换序
	if cfg.Swap16 && len(b) == 2 {
		b = []byte{b[1], b[0]}
	}

	s, err := dataformat.FormatPoint(b, order, cfg.Type, cfg.ReadExpr)
	if err != nil {
		return nil, "Bad", err
	}

	val, convErr := d.convertFormattedValue(point, cfg.Type, s, len(b))
	if convErr != nil {
		return nil, "Bad", convErr
	}

	if point.ReadFormula == "" && (point.Scale != 0 || point.Offset != 0) {
		val = d.applyScaleOffset(point, val)
	}

	return val, "Good", nil
}

func (d *PointDecoder) convertFormattedValue(point model.Point, fmtType dataformat.FormatType, s string, byteLen int) (any, error) {
	dt := strings.ToLower(point.DataType)
	if point.ParseType != "" {
		switch strings.ToUpper(point.ParseType) {
		case "INT16":
			dt = "int16"
		case "UINT16":
			dt = "uint16"
		case "INT32":
			dt = "int32"
		case "UINT32":
			dt = "uint32"
		case "FLOAT32":
			dt = "float32"
		case "FLOAT64":
			dt = "float64"
		}
	}

	switch fmtType {
	case dataformat.FormatHexString, dataformat.FormatBinaryString:
		return s, nil
	case dataformat.FormatFloat32, dataformat.FormatFloat64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}
		if dt == "float32" || dt == "float" {
			return float32(f), nil
		}
		return f, nil
	case dataformat.FormatSignedInt, dataformat.FormatUnsignedInt, dataformat.FormatExpr16, dataformat.FormatExpr32:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}

		if dt == "bool" || dt == "boolean" || strings.HasPrefix(dt, "bit") {
			return i != 0, nil
		}

		if dt == "int16" {
			if i < math.MinInt16 || i > math.MaxInt16 {
				return nil, fmt.Errorf("value %d out of range for int16", i)
			}
			return int16(i), nil
		}
		if dt == "uint16" {
			if i < 0 || i > math.MaxUint16 {
				return nil, fmt.Errorf("value %d out of range for uint16", i)
			}
			return uint16(i), nil
		}
		if dt == "int32" {
			if i < math.MinInt32 || i > math.MaxInt32 {
				return nil, fmt.Errorf("value %d out of range for int32", i)
			}
			return int32(i), nil
		}
		if dt == "uint32" {
			if i < 0 || i > math.MaxUint32 {
				return nil, fmt.Errorf("value %d out of range for uint32", i)
			}
			return uint32(i), nil
		}

		if byteLen == 2 {
			if i < math.MinInt16 || i > math.MaxInt16 {
				return nil, fmt.Errorf("value %d out of range for 16-bit register", i)
			}
			return int16(i), nil
		}
		if byteLen == 4 {
			if i < math.MinInt32 || i > math.MaxInt32 {
				return nil, fmt.Errorf("value %d out of range for 32-bit register", i)
			}
			return int32(i), nil
		}

		return i, nil
	default:
		return s, nil
	}
}

func (d *PointDecoder) applyScaleOffset(point model.Point, val any) any {
	if point.Scale == 0 && point.Offset == 0 {
		return val
	}

	scale := point.Scale
	if scale == 0 {
		scale = 1.0
	}

	var fVal float64
	switch v := val.(type) {
	case float64:
		fVal = v
	case float32:
		fVal = float64(v)
	case int16:
		fVal = float64(v)
	case uint16:
		fVal = float64(v)
	case int32:
		fVal = float64(v)
	case uint32:
		fVal = float64(v)
	default:
		return val
	}

	return fVal*scale + point.Offset
}

// applyByteOrder applies the configured 4-byte byte order
func (d *PointDecoder) applyByteOrder(b []byte) []byte {
	if len(b) != 4 {
		return b
	}
	newB := make([]byte, 4)
	switch d.byteOrder4 {
	case "ABCD":
		copy(newB, b)
	case "CDAB":
		newB[0], newB[1], newB[2], newB[3] = b[2], b[3], b[0], b[1]
	case "BADC":
		newB[0], newB[1], newB[2], newB[3] = b[1], b[0], b[3], b[2]
	case "DCBA":
		newB[0], newB[1], newB[2], newB[3] = b[3], b[2], b[1], b[0]
	default:
		copy(newB, b)
	}
	return newB
}

// swapsBytes16 判断点位自身 word order 是否意味着 16 位寄存器内字节交换
// （UI 的 2 字节 BA 选项落盘为 DCBA；与 dataformat.ResolvePointFormat 的 Swap16 判定一致）。
func swapsBytes16(point model.Point) bool {
	switch strings.ToUpper(strings.TrimSpace(point.WordOrder)) {
	case "BADC", "DCBA", "BA":
		return true
	}
	return false
}

// Encode 将值编码为寄存器数组（用于写入）
func (d *PointDecoder) Encode(point model.Point, value any) ([]uint16, error) {
	rawValue := value
	if point.WriteFormula != "" {
		var err error
		rawValue, err = d.applyFormula(point.WriteFormula, value)
		if err != nil {
			return nil, err
		}
	} else {
		rawValue = d.reverseScaleOffset(point, value)
	}

	return d.encodeRaw(point, rawValue)
}

func (d *PointDecoder) applyFormula(expr string, value any) (any, error) {
	var number int64
	switch v := value.(type) {
	case int:
		number = int64(v)
	case uint:
		if uint64(v) > math.MaxInt64 {
			return nil, fmt.Errorf("value %d overflows int64", v)
		}
		number = int64(v)
	case float64:
		number = int64(v)
	case float32:
		number = int64(v)
	case int16:
		number = int64(v)
	case uint16:
		number = int64(v)
	case int32:
		number = int64(v)
	case uint32:
		number = int64(v)
	case int64:
		number = v
	default:
		return nil, fmt.Errorf("formula requires numeric value, got %T", value)
	}
	result, err := dataformat.EvalExpression(expr, number)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *PointDecoder) reverseScaleOffset(point model.Point, value any) any {
	if point.Scale == 0 && point.Offset == 0 {
		return value
	}

	// value - Offset / Scale
	var fVal float64
	switch v := value.(type) {
	case float64:
		fVal = v
	case float32:
		fVal = float64(v)
	case int:
		fVal = float64(v)
	case int64:
		fVal = float64(v)
	case int32:
		fVal = float64(v)
	case uint32:
		fVal = float64(v)
	case int16:
		fVal = float64(v)
	case uint16:
		fVal = float64(v)
	case string:
		fVal, _ = strconv.ParseFloat(v, 64)
	default:
		// 简单处理，如果类型不对可能后续会报错
		return value
	}

	if point.Scale != 0 {
		return (fVal - point.Offset) / point.Scale
	}
	return fVal - point.Offset
}

func rawDataType(point model.Point) string {
	if point.ParseType == "" {
		return strings.ToLower(point.DataType)
	}

	switch strings.ToUpper(point.ParseType) {
	case "INT16", "INT16_SWAP":
		return "int16"
	case "UINT16", "UINT16_SWAP", "BCD16":
		return "uint16"
	case "INT32", "INT32_SWAP":
		return "int32"
	case "UINT32", "UINT32_SWAP", "BCD32":
		return "uint32"
	case "FLOAT32", "FLOAT32_SWAP":
		return "float32"
	}

	return strings.ToLower(point.DataType)
}

func (d *PointDecoder) encodeRaw(point model.Point, value any) ([]uint16, error) {
	rawType := rawDataType(point)
	switch rawType {
	case "int16", "uint16":
		// 统一归一到 int64 后按点型域校验：int16 允许负数（补码位模式写入寄存器），
		// uint16 仅接受非负；超范围拒绝写入，避免整数收窄静默回绕把错误值写进寄存器。
		var n int64
		switch v := value.(type) {
		case float64:
			if v != v || v < math.MinInt16 || v > math.MaxUint16 {
				return nil, fmt.Errorf("value %v out of range for 16-bit register", v)
			}
			n = int64(v)
		case int:
			n = int64(v)
		case int64:
			n = v
		case int32:
			n = int64(v)
		case uint32:
			n = int64(v)
		case int16:
			n = int64(v)
		case uint16:
			n = int64(v)
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid integer value %q: %w", v, err)
			}
			n = i
		default:
			return nil, fmt.Errorf("unsupported value type: %T", value)
		}
		if rawType == "int16" {
			if n < math.MinInt16 || n > math.MaxInt16 {
				return nil, fmt.Errorf("value %d out of range for int16", n)
			}
		} else {
			if n < 0 || n > math.MaxUint16 {
				return nil, fmt.Errorf("value %d out of range for uint16", n)
			}
		}
		// #nosec G115 -- 已按点型域校验；int16 负数为补码位模式重解释（Modbus 寄存器定义行为）
		intVal := uint16(n)
		// 与读取侧一致：点位自身 word order 要求 16 位字节交换时，写寄存器值先行换序
		if swapsBytes16(point) {
			intVal = intVal<<8 | intVal>>8
		}
		return []uint16{intVal}, nil

	case "float32":
		var fVal float32
		switch v := value.(type) {
		case float64:
			fVal = float32(v)
		case float32:
			fVal = v
		case int:
			fVal = float32(v)
		case int32:
			fVal = float32(v)
		case uint32:
			fVal = float32(v)
		case int64:
			fVal = float32(v)
		case string:
			f, _ := strconv.ParseFloat(v, 32)
			fVal = float32(f)
		default:
			return nil, fmt.Errorf("unsupported value type for float32: %T", value)
		}

		bits := math.Float32bits(fVal)
		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, bits)
		orderedBytes := d.applyByteOrder(bytes)

		reg1 := binary.BigEndian.Uint16(orderedBytes[0:2])
		reg2 := binary.BigEndian.Uint16(orderedBytes[2:4])
		return []uint16{reg1, reg2}, nil

	case "int32", "uint32":
		// 同 16 位分支：归一到 int64 后按点型域校验，int32 允许负数（补码位模式），uint32 仅非负。
		var n int64
		switch v := value.(type) {
		case float64:
			if v != v || v < math.MinInt32 || v > math.MaxUint32 {
				return nil, fmt.Errorf("value %v out of range for 32-bit register", v)
			}
			n = int64(v)
		case int:
			n = int64(v)
		case int64:
			n = v
		case int32:
			n = int64(v)
		case uint32:
			n = int64(v)
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid integer value %q: %w", v, err)
			}
			n = i
		default:
			return nil, fmt.Errorf("unsupported value type for int32/uint32: %T", value)
		}
		if rawType == "int32" {
			if n < math.MinInt32 || n > math.MaxInt32 {
				return nil, fmt.Errorf("value %d out of range for int32", n)
			}
		} else {
			if n < 0 || n > math.MaxUint32 {
				return nil, fmt.Errorf("value %d out of range for uint32", n)
			}
		}
		// #nosec G115 -- 已按点型域校验；int32 负数为补码位模式重解释（Modbus 寄存器定义行为）
		uVal := uint32(n)

		bytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bytes, uVal)
		orderedBytes := d.applyByteOrder(bytes)

		reg1 := binary.BigEndian.Uint16(orderedBytes[0:2])
		reg2 := binary.BigEndian.Uint16(orderedBytes[2:4])
		return []uint16{reg1, reg2}, nil
	}

	return nil, fmt.Errorf("encode not supported for type: %s", rawType)
}
