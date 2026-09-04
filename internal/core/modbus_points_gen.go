package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// ModbusRegisterGenOptions 批量生成 Modbus 寄存器点位参数。
type ModbusRegisterGenOptions struct {
	Start        int
	End          int
	DataType     string
	ReadWrite    string
	RegisterType model.RegisterType
	FunctionCode byte
	DeviceID     string
}

// GenerateModbusRegisterPoints 按地址区间生成保持/输入等寄存器点位。
// mergeExisting 为 true 时保留同 ID 现有点位配置。
func GenerateModbusRegisterPoints(existing []model.Point, opts ModbusRegisterGenOptions, mergeExisting bool) []model.Point {
	start, end := opts.Start, opts.End
	if end < start {
		start, end = end, start
	}
	if opts.DataType == "" {
		opts.DataType = "int16"
	}
	if opts.ReadWrite == "" {
		opts.ReadWrite = "R"
	}
	if opts.RegisterType == model.RegHolding && opts.FunctionCode == 0 {
		opts.FunctionCode = 3
	}
	if opts.FunctionCode == 0 {
		opts.FunctionCode = opts.RegisterType.FunctionCode()
	}

	existingByID := make(map[string]model.Point)
	if mergeExisting {
		for _, p := range existing {
			existingByID[p.ID] = p
		}
	}

	prefix := registerPointPrefix(opts.RegisterType)
	points := make([]model.Point, 0, end-start+1)
	for addr := start; addr <= end; addr++ {
		pointID := fmt.Sprintf("%s_%d", prefix, addr)
		if mergeExisting {
			if ep, ok := existingByID[pointID]; ok {
				points = append(points, ep)
				continue
			}
		}
		points = append(points, model.Point{
			Name:         fmt.Sprintf("%s %d", strings.ToUpper(prefix), addr),
			ID:           pointID,
			DeviceID:     opts.DeviceID,
			Address:      strconv.Itoa(addr),
			DataType:     opts.DataType,
			ReadWrite:    opts.ReadWrite,
			Scale:        1,
			Offset:       0,
			RegisterType: opts.RegisterType,
			FunctionCode: opts.FunctionCode,
		})
	}
	return points
}

func registerPointPrefix(regType model.RegisterType) string {
	switch regType {
	case model.RegInput:
		return "ir"
	case model.RegCoil:
		return "coil"
	case model.RegDiscreteInput:
		return "di"
	default:
		return "hr"
	}
}

// ParseAutoPointsRange 解析 auto_points_range 配置（如 "0-199"）。
func ParseAutoPointsRange(rng string) (start, end int, ok bool) {
	parts := strings.Split(rng, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if end < start {
		start, end = end, start
	}
	return start, end, true
}

func modbusGenOptionsFromDevice(dev *model.Device) (ModbusRegisterGenOptions, bool) {
	if dev == nil || dev.Config == nil {
		return ModbusRegisterGenOptions{}, false
	}
	rng, ok := dev.Config["auto_points_range"]
	if !ok {
		return ModbusRegisterGenOptions{}, false
	}
	start, end, ok := ParseAutoPointsRange(fmt.Sprintf("%v", rng))
	if !ok {
		return ModbusRegisterGenOptions{}, false
	}
	opts := ModbusRegisterGenOptions{
		Start:        start,
		End:          end,
		DataType:     "int16",
		ReadWrite:    "R",
		RegisterType: model.RegHolding,
		FunctionCode: 3,
		DeviceID:     dev.ID,
	}
	if v, ok := dev.Config["auto_points_datatype"]; ok {
		opts.DataType = fmt.Sprintf("%v", v)
	}
	if v, ok := dev.Config["auto_points_readwrite"]; ok {
		opts.ReadWrite = fmt.Sprintf("%v", v)
	}
	if v, ok := dev.Config["auto_points_register_type"]; ok {
		opts.RegisterType = model.ParseRegisterType(fmt.Sprintf("%v", v))
	}
	if v, ok := dev.Config["auto_points_function_code"]; ok {
		switch n := v.(type) {
		case int:
			opts.FunctionCode = byte(n)
		case float64:
			opts.FunctionCode = byte(n)
		}
	}
	return opts, true
}
