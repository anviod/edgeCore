package core

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// MockDeviceWriter 用于捕获 WritePoint 调用
type MockDeviceWriter struct {
	LastChannelID string
	LastDeviceID  string
	LastPointID   string
	LastValue     any
	WriteCount    int
	Err           error
}

func (m *MockDeviceWriter) WritePoint(channelID, deviceID, pointID string, value any) error {
	m.LastChannelID = channelID
	m.LastDeviceID = deviceID
	m.LastPointID = pointID
	m.LastValue = value
	m.WriteCount++
	return m.Err
}

func (m *MockDeviceWriter) ReadPoint(channelID, deviceID, pointID string) (model.Value, error) {
	return model.Value{Value: 0}, nil
}

func TestEdgeActionExpression(t *testing.T) {
	// 1. Setup
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &MockDeviceWriter{}
	em.SetDeviceWriter(mockWriter)

	// 2. Define Test Cases
	tests := []struct {
		Name          string
		Expression    string
		Value         string // Backup static value
		InputVal      any
		ExpectedVal   any
		ShouldUseExpr bool // Whether expression result is expected
	}{
		{
			Name:          "Basic Addition",
			Expression:    "v + 1",
			Value:         "999",
			InputVal:      10.0,
			ExpectedVal:   11.0,
			ShouldUseExpr: true,
		},
		{
			Name:          "Subtraction",
			Expression:    "v - 10",
			Value:         "999",
			InputVal:      20,
			ExpectedVal:   10.0,
			ShouldUseExpr: true,
		},
		{
			Name:          "Invalid Expression Fallback",
			Expression:    "v + invalid syntax",
			Value:         "FallbackValue",
			InputVal:      10,
			ExpectedVal:   "FallbackValue",
			ShouldUseExpr: false,
		},
		{
			Name:          "Empty Expression",
			Expression:    "",
			Value:         "StaticValue",
			InputVal:      10,
			ExpectedVal:   "StaticValue",
			ShouldUseExpr: false,
		},
		{
			Name:          "Float Calculation",
			Expression:    "v * 1.5",
			Value:         "0",
			InputVal:      10,
			ExpectedVal:   15.0,
			ShouldUseExpr: true,
		},
		{
			Name:          "Bitwise AND Function",
			Expression:    "bitand(v, 64)",
			Value:         "0",
			InputVal:      65.0,
			ExpectedVal:   64,
			ShouldUseExpr: true,
		},
		{
			Name:          "Bitwise OR Function",
			Expression:    "bitor(v, 1)",
			Value:         "0",
			InputVal:      64,
			ExpectedVal:   65,
			ShouldUseExpr: true,
		},
		{
			Name:          "Bitwise Set (bitset)",
			Expression:    "bitset(v, 4, 0)",
			Value:         "0",
			InputVal:      18, // 10010
			ExpectedVal:   2,  // 00010
			ShouldUseExpr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			// Reset Mock
			mockWriter.WriteCount = 0
			mockWriter.LastValue = nil

			// Construct Action
			config := map[string]any{
				"targets": []any{
					map[string]any{
						"channel_id": "ch1",
						"device_id":  "dev1",
						"point_id":   "p1",
						"value":      tc.Value,
						"expression": tc.Expression,
					},
				},
			}

			action := model.RuleAction{
				Type:   "device_control",
				Config: config,
			}

			// Construct Environment
			val := model.Value{
				Value: tc.InputVal,
				TS:    time.Now(),
			}
			env := map[string]any{
				"value": tc.InputVal,
			}

			// Execute Action directly
			err := em.executeSingleAction(context.Background(), "test_rule", action, val, env)

			// Verify Error
			if err != nil {
				t.Errorf("Action execution failed: %v", err)
			}

			// Verify Write
			if mockWriter.WriteCount != 1 {
				t.Errorf("Expected 1 write, got %d", mockWriter.WriteCount)
			}

			// Verify Value
			got := mockWriter.LastValue
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tc.ExpectedVal) {
				t.Errorf("Value mismatch. Got %v (%T), want %v (%T)", got, got, tc.ExpectedVal, tc.ExpectedVal)
			}
		})
	}
}

func TestEvaluateThreshold(t *testing.T) {
	tests := []struct {
		Name      string
		Condition string
		Env       map[string]any
		Want      bool
		WantErr   bool
	}{
		{
			Name:      "Simple True",
			Condition: "v > 10",
			Env:       map[string]any{"v": 11.0},
			Want:      true,
			WantErr:   false,
		},
		{
			Name:      "Simple False",
			Condition: "v > 10",
			Env:       map[string]any{"v": 9.0},
			Want:      false,
			WantErr:   false,
		},
		{
			Name:      "Multi Var Success",
			Condition: "t1 > 1 && t2 > 3",
			Env:       map[string]any{"t1": 2.0, "t2": 4.0},
			Want:      true,
			WantErr:   false,
		},
		{
			Name:      "Multi Var Missing t2 (nil) -> NaN",
			Condition: "t1 > 1 && t2 > 3",
			Env:       map[string]any{"t1": 2.0, "t2": math.NaN()},
			Want:      false,
			WantErr:   false,
		},
		{
			Name:      "Multi Var Missing t2 (NaN) < Check",
			Condition: "t2 < 5",
			Env:       map[string]any{"t2": math.NaN()},
			Want:      false,
			WantErr:   false,
		},
		{
			Name:      "Multi Var Missing t2 (absent)",
			Condition: "t1 > 1 && t2 > 3",
			Env:       map[string]any{"t1": 2.0},
			Want:      false,
			WantErr:   true, // Expr usually errors on missing vars
		},
		{
			Name:      "Bit Access Syntax (v.N)",
			Condition: "v.5 == 1",
			Env:       map[string]any{"v": 18},
			Want:      true,
			WantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := evaluateThreshold(tc.Condition, tc.Env)
			if tc.WantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else {
					t.Logf("Got expected error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if got != tc.Want {
					t.Errorf("Got %v, want %v", got, tc.Want)
				}
			}
		})
	}
}
