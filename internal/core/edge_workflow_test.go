package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// MockDeviceIO for testing ReadPoint/WritePoint
type MockDeviceIO struct {
	ReadFunc  func(cid, did, pid string) (model.Value, error)
	WriteFunc func(cid, did, pid string, val any) error
}

func (m *MockDeviceIO) ReadPoint(cid, did, pid string) (model.Value, error) {
	if m.ReadFunc != nil {
		return m.ReadFunc(cid, did, pid)
	}
	return model.Value{}, fmt.Errorf("read not implemented")
}

func (m *MockDeviceIO) WritePoint(cid, did, pid string, val any) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(cid, did, pid, val)
	}
	return nil
}

func TestWorkflow_Sequence_Success(t *testing.T) {
	em := &EdgeComputeManager{}

	// Track executed actions
	var executedActions []string
	em.actionHook = func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		if action.Type == "log" {
			msg, _ := action.Config["message"].(string)
			executedActions = append(executedActions, msg)
		}
	}

	// Define Sequence Action
	action := model.RuleAction{
		Type: "sequence",
		Config: map[string]any{
			"steps": []interface{}{
				map[string]interface{}{
					"type":   "log",
					"config": map[string]interface{}{"message": "Step 1"},
				},
				map[string]interface{}{
					"type":   "log",
					"config": map[string]interface{}{"message": "Step 2"},
				},
			},
		},
	}

	err := em.executeSingleAction(context.Background(), "test-rule", action, model.Value{}, nil)
	if err != nil {
		t.Fatalf("Sequence failed: %v", err)
	}

	if len(executedActions) != 2 {
		t.Fatalf("Expected 2 steps, got %d", len(executedActions))
	}
	if executedActions[0] != "Step 1" || executedActions[1] != "Step 2" {
		t.Errorf("Unexpected execution order: %v", executedActions)
	}
}

func TestWorkflow_Sequence_SafetyTermination(t *testing.T) {
	// Setup Mock Device that returns value 5 (condition > 10 will fail)
	mockIO := &MockDeviceIO{
		ReadFunc: func(cid, did, pid string) (model.Value, error) {
			return model.Value{Value: 5.0}, nil
		},
	}
	em := &EdgeComputeManager{
		writer: mockIO,
	}

	var executedActions []string
	em.actionHook = func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		if action.Type == "log" {
			msg, _ := action.Config["message"].(string)
			executedActions = append(executedActions, msg)
		}
	}

	// Sequence: [Check (Fail), Log (Should NOT run)]
	action := model.RuleAction{
		Type: "sequence",
		Config: map[string]any{
			"steps": []interface{}{
				map[string]interface{}{
					"type": "check",
					"config": map[string]interface{}{
						"expression": "v > 10",
						"retry":      1,
					},
				},
				map[string]interface{}{
					"type":   "log",
					"config": map[string]interface{}{"message": "Step 2"},
				},
			},
		},
	}

	err := em.executeSingleAction(context.Background(), "test-rule", action, model.Value{}, nil)

	// Verify error occurred (Check failed)
	if err == nil {
		t.Fatal("Expected error from failed Check, got nil")
	}

	// Verify Step 2 was NOT executed
	if len(executedActions) > 0 {
		t.Errorf("Sequence continued after failure! Executed: %v", executedActions)
	}
}

func TestWorkflow_Delay(t *testing.T) {
	em := &EdgeComputeManager{}

	action := model.RuleAction{
		Type: "delay",
		Config: map[string]any{
			"duration": "100ms",
		},
	}

	start := time.Now()
	err := em.executeSingleAction(context.Background(), "test-rule", action, model.Value{}, nil)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Delay failed: %v", err)
	}

	if duration < 100*time.Millisecond {
		t.Errorf("Delay too short: %v", duration)
	}
}

func TestWorkflow_Check_Success(t *testing.T) {
	mockIO := &MockDeviceIO{
		ReadFunc: func(cid, did, pid string) (model.Value, error) {
			return model.Value{Value: 20.0}, nil
		},
	}
	em := &EdgeComputeManager{writer: mockIO}

	action := model.RuleAction{
		Type: "check",
		Config: map[string]any{
			"expression": "v > 10",
			"retry":      1,
		},
	}

	err := em.executeSingleAction(context.Background(), "test-rule", action, model.Value{}, nil)
	if err != nil {
		t.Fatalf("Check should pass: %v", err)
	}
}

func TestWorkflow_Check_Fail_WithRollback(t *testing.T) {
	mockIO := &MockDeviceIO{
		ReadFunc: func(cid, did, pid string) (model.Value, error) {
			return model.Value{Value: 5.0}, nil
		},
	}
	em := &EdgeComputeManager{writer: mockIO}

	var executedActions []string
	em.actionHook = func(ruleID string, action model.RuleAction, val model.Value, env map[string]any, err error) {
		if action.Type == "log" {
			msg, _ := action.Config["message"].(string)
			executedActions = append(executedActions, msg)
		}
	}

	action := model.RuleAction{
		Type: "check",
		Config: map[string]any{
			"expression": "v > 10",
			"retry":      1,
			"on_fail": []interface{}{
				map[string]interface{}{
					"type":   "log",
					"config": map[string]interface{}{"message": "Rollback Executed"},
				},
			},
		},
	}

	err := em.executeSingleAction(context.Background(), "test-rule", action, model.Value{}, nil)

	// Should return error because Check failed
	if err == nil {
		t.Fatal("Check should fail")
	}

	// But Rollback actions should have been executed
	if len(executedActions) != 1 || executedActions[0] != "Rollback Executed" {
		t.Errorf("Rollback action not executed correctly: %v", executedActions)
	}
}
