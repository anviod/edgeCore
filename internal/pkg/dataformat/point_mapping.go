package dataformat

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

type PointFormatConfig struct {
	Type     FormatType
	Order    binary.ByteOrder
	ReadExpr string
}

func ResolvePointFormat(p model.Point, defaultWordOrder string) PointFormatConfig {
	dt := strings.ToLower(p.DataType)
	if p.ParseType != "" {
		switch strings.ToUpper(p.ParseType) {
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
	format := strings.ToLower(p.Format)
	wordOrder := p.WordOrder
	if wordOrder == "" {
		wordOrder = defaultWordOrder
	}
	wordOrder = strings.ToUpper(wordOrder)

	order := resolveWordOrder(wordOrder)
	ft := resolveFormatType(dt, format, p.ReadFormula)
	readExpr := p.ReadFormula

	return PointFormatConfig{
		Type:     ft,
		Order:    order,
		ReadExpr: readExpr,
	}
}

func resolveWordOrder(name string) binary.ByteOrder {
	switch name {
	case "CDAB":
		return OrderCDAB
	case "BADC":
		return OrderBADC
	case "DCBA":
		return OrderDCBA
	default:
		return OrderABCD
	}
}

func resolveFormatType(dt string, format string, readExpr string) FormatType {
	if format == "hex" {
		return FormatHexString
	}
	if format == "binary" {
		return FormatBinaryString
	}

	switch dt {
	case "float32", "float":
		return FormatFloat32
	case "float64", "double":
		return FormatFloat64
	case "bool", "boolean", "bit":
		return FormatUnsignedInt
	}

	if strings.HasPrefix(dt, "uint") || dt == "word" || dt == "dword" || dt == "lword" {
		if readExpr != "" {
			if dt == "uint32" || dt == "dword" || dt == "lword" {
				return FormatExpr32
			}
			return FormatExpr16
		}
		return FormatUnsignedInt
	}

	if strings.HasPrefix(dt, "int") {
		if readExpr != "" {
			if dt == "int32" {
				return FormatExpr32
			}
			return FormatExpr16
		}
		return FormatSignedInt
	}

	if readExpr != "" {
		return FormatExpr32
	}

	return FormatSignedInt
}

func EvalExpression(exprStr string, v int64) (int64, error) {
	return evalExpression(exprStr, v)
}

func FormatScalar(p model.Point, defaultWordOrder string, raw any) (any, error) {
	cfg := ResolvePointFormat(p, defaultWordOrder)

	if cfg.Type != FormatHexString && cfg.Type != FormatBinaryString && cfg.Type != FormatExpr16 && cfg.Type != FormatExpr32 {
		return raw, nil
	}

	value, ok := toInt64(raw)
	if !ok {
		return raw, nil
	}

	var buf []byte

	switch cfg.Type {
	case FormatExpr16, FormatHexString, FormatBinaryString:
		buf = make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(value))
	default:
		buf = make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(value))
	}

	s, err := FormatPoint(buf, cfg.Order, cfg.Type, cfg.ReadExpr)
	if err != nil {
		return nil, err
	}

	if cfg.Type == FormatHexString || cfg.Type == FormatBinaryString {
		return s, nil
	}

	intVal, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}

	dt := strings.ToLower(p.DataType)

	switch dt {
	case "int16":
		return int16(intVal), nil
	case "uint16":
		return uint16(intVal), nil
	case "int32":
		return int32(intVal), nil
	case "uint32":
		return uint32(intVal), nil
	case "bool", "boolean", "bit":
		return intVal != 0, nil
	default:
		return intVal, nil
	}
}

// ApplyFormula evaluates a point formula against a numeric value. Formula
// evaluation is intentionally separate from scale/offset so callers can make
// the formula-over-scale precedence explicit.
func ApplyFormula(expr string, value any) (any, error) {
	valueInt, ok := toInt64(value)
	if !ok {
		return nil, fmt.Errorf("formula requires numeric value, got %T", value)
	}
	return EvalExpression(expr, valueInt)
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int:
		return int64(t), true
	case int8:
		return int64(t), true
	case int16:
		return int64(t), true
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case uint:
		return int64(t), true
	case uint8:
		return int64(t), true
	case uint16:
		return int64(t), true
	case uint32:
		return int64(t), true
	case uint64:
		return int64(t), true
	case float32:
		return int64(t), true
	case float64:
		return int64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}
