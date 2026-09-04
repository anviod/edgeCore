package ethernetip

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestWritePointDataTypeConversion(t *testing.T) {
	testCases := []struct {
		name      string
		dataType  string
		input     string
		expectErr bool
	}{
		// BOOL type tests
		{"BOOL true", "BOOL", "true", false},
		{"BOOL false", "BOOL", "false", false},
		{"BOOL 1", "BOOL", "1", false},
		{"BOOL 0", "BOOL", "0", false},
		{"BOOL invalid", "BOOL", "invalid", true},

		// INT type tests
		{"INT positive", "INT", "123", false},
		{"INT negative", "INT", "-456", false},
		{"INT zero", "INT", "0", false},
		{"INT max int32", "INT", "2147483647", false},
		{"INT min int32", "INT", "-2147483648", false},
		{"INT invalid", "INT", "abc", true},

		// SINT type tests
		{"SINT positive", "SINT", "127", false},
		{"SINT negative", "SINT", "-128", false},
		{"SINT zero", "SINT", "0", false},
		{"SINT invalid", "SINT", "xyz", true},

		// UINT type tests
		{"UINT positive", "UINT", "65535", false},
		{"UINT zero", "UINT", "0", false},
		{"UINT invalid negative", "UINT", "-1", true},

		// USINT type tests
		{"USINT positive", "USINT", "255", false},
		{"USINT zero", "USINT", "0", false},
		{"USINT invalid", "USINT", "-5", true},

		// DINT type tests
		{"DINT positive", "DINT", "2147483647", false},
		{"DINT negative", "DINT", "-2147483648", false},
		{"DINT zero", "DINT", "0", false},
		{"DINT invalid", "DINT", "not_a_number", true},

		// UDINT type tests
		{"UDINT positive", "UDINT", "4294967295", false},
		{"UDINT zero", "UDINT", "0", false},
		{"UDINT invalid negative", "UDINT", "-100", true},

		// LINT type tests
		{"LINT positive", "LINT", "9223372036854775807", false},
		{"LINT negative", "LINT", "-9223372036854775808", false},
		{"LINT zero", "LINT", "0", false},

		// ULINT type tests
		{"ULINT positive", "ULINT", "18446744073709551615", false},
		{"ULINT zero", "ULINT", "0", false},

		// REAL type tests
		{"REAL positive", "REAL", "3.14", false},
		{"REAL negative", "REAL", "-2.718", false},
		{"REAL zero", "REAL", "0.0", false},
		{"REAL scientific", "REAL", "1.5e10", false},
		{"REAL invalid", "REAL", "not_a_float", true},

		// LREAL type tests
		{"LREAL positive", "LREAL", "3.141592653589793", false},
		{"LREAL negative", "LREAL", "-2.718281828459045", false},
		{"LREAL zero", "LREAL", "0", false},

		// STRING type tests
		{"STRING normal", "STRING", "Hello World", false},
		{"STRING empty", "STRING", "", false},
		{"STRING special chars", "STRING", "Test@#$%", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			point := model.Point{
				ID:       "TestPoint",
				DataType: tc.dataType,
				Address:  "Program:MainProgram.TestTag",
			}

			err := testWritePointConversion(point, tc.input)

			if tc.expectErr && err == nil {
				t.Errorf("Expected error for %s with input %q, got nil", tc.dataType, tc.input)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Unexpected error for %s with input %q: %v", tc.dataType, tc.input, err)
			}
		})
	}
}

func testWritePointConversion(point model.Point, value interface{}) error {
	switch v := value.(type) {
	case bool:
		// bool type is always valid
	case int:
		// int type is always valid
	case int32:
		// int32 type is always valid
	case int64:
		// int64 type is always valid
	case float32:
		// float32 type is always valid
	case float64:
		// float64 type is always valid
	case string:
		switch strings.ToUpper(point.DataType) {
		case "BOOL":
			_, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("invalid bool value: %w", err)
			}
		case "INT", "SINT":
			_, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid int value: %w", err)
			}
		case "UINT", "USINT":
			_, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid uint value: %w", err)
			}
		case "DINT":
			_, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid dint value: %w", err)
			}
		case "UDINT":
			_, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid udint value: %w", err)
			}
		case "LINT":
			_, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid lint value: %w", err)
			}
		case "ULINT":
			_, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ulint value: %w", err)
			}
		case "REAL", "LREAL":
			_, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("invalid float value: %w", err)
			}
		case "STRING":
			// String type doesn't need conversion
		default:
			return fmt.Errorf("unsupported data type: %s", point.DataType)
		}
	default:
		return fmt.Errorf("unsupported value type: %T", value)
	}
	return nil
}

func TestWritePointWithNumericTypes(t *testing.T) {
	testCases := []struct {
		name     string
		dataType string
		value    interface{}
	}{
		{"int value", "INT", 42},
		{"int32 value", "INT", int32(100)},
		{"int64 value", "INT", int64(200)},
		{"bool true", "BOOL", true},
		{"bool false", "BOOL", false},
		{"float32 value", "REAL", float32(3.14)},
		{"float64 value", "REAL", float64(2.718)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			point := model.Point{
				ID:       "TestPoint",
				DataType: tc.dataType,
				Address:  "Program:MainProgram.TestTag",
			}

			err := testWritePointConversion(point, tc.value)
			if err != nil {
				t.Errorf("Unexpected error for %s with value %v: %v", tc.dataType, tc.value, err)
			}
		})
	}
}
