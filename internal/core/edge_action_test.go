package core

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// Reusing MockDeviceWriter logic for this test file
type TestDeviceWriter struct {
	WriteCount int
	LastValue  any
}

func (m *TestDeviceWriter) WritePoint(channelID, deviceID, pointID string, value any) error {
	m.WriteCount++
	m.LastValue = value
	return nil
}

func (m *TestDeviceWriter) ReadPoint(channelID, deviceID, pointID string) (model.Value, error) {
	return model.Value{Value: 0}, nil
}

func TestEdgeAction_DirectWrite(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &TestDeviceWriter{}
	em.SetDeviceWriter(mockWriter)

	// Action without expression, only value config
	action := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"channel_id": "ch1",
			"device_id":  "dev1",
			"point_id":   "p1",
			"value":      "123", // Static value
		},
	}

	val := model.Value{Value: 10}
	env := map[string]any{"v": 10}

	err := em.executeSingleAction(context.Background(), "rule1", action, val, env)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if mockWriter.WriteCount != 1 {
		t.Errorf("Expected 1 write, got %d", mockWriter.WriteCount)
	}
	if mockWriter.LastValue != "123" {
		t.Errorf("Expected value '123', got %v", mockWriter.LastValue)
	}
}

func TestEdgeAction_DirectWrite_Fallback(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &TestDeviceWriter{}
	em.SetDeviceWriter(mockWriter)

	// Action without expression and without value config -> use triggering value
	action := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"channel_id": "ch1",
			"device_id":  "dev1",
			"point_id":   "p1",
		},
	}

	val := model.Value{Value: 999}
	env := map[string]any{"v": 999}

	err := em.executeSingleAction(context.Background(), "rule1", action, val, env)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if mockWriter.WriteCount != 1 {
		t.Errorf("Expected 1 write, got %d", mockWriter.WriteCount)
	}
	if mockWriter.LastValue != 999 {
		t.Errorf("Expected value 999, got %v", mockWriter.LastValue)
	}
}

func TestEdgeAction_FrequencyLimit(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &TestDeviceWriter{}
	em.SetDeviceWriter(mockWriter)

	ruleID := "rule_freq"
	// Initialize state
	em.ruleStates[ruleID] = &model.RuleRuntimeState{
		RuleID: ruleID,
	}

	action := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"channel_id": "ch1",
			"device_id":  "dev1",
			"point_id":   "p1",
			"value":      "test",
			"interval":   "100ms", // Limit
		},
	}
	actions := []model.RuleAction{action}

	val := model.Value{Value: 1}
	env := map[string]any{}

	// 1st Execution: Should pass
	em.executeActions(model.EdgeRule{ID: ruleID}, actions, val, env, nil)
	time.Sleep(10 * time.Millisecond) // Wait for goroutine

	if mockWriter.WriteCount != 1 {
		t.Errorf("First execution failed. Writes: %d", mockWriter.WriteCount)
	}

	// 2nd Execution (Immediate): Should be skipped
	em.executeActions(model.EdgeRule{ID: ruleID}, actions, val, env, nil)
	time.Sleep(10 * time.Millisecond)

	if mockWriter.WriteCount != 1 {
		t.Errorf("Second execution should be skipped. Writes: %d", mockWriter.WriteCount)
	}

	// Wait for interval
	time.Sleep(100 * time.Millisecond)

	// 3rd Execution: Should pass
	em.executeActions(model.EdgeRule{ID: ruleID}, actions, val, env, nil)
	time.Sleep(10 * time.Millisecond)

	if mockWriter.WriteCount != 2 {
		t.Errorf("Third execution failed. Writes: %d", mockWriter.WriteCount)
	}
}
