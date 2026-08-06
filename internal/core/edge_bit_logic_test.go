package core

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestEdgeAction_BitValueLogic(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &MockBatchWriter{}
	em.SetDeviceWriter(mockWriter)

	// Input value: 9 (Binary 1001)
	// v.1 (Bit 0) is 1
	// v.4 (Bit 3) is 1
	val := model.Value{Value: 9}
	env := map[string]any{"v": 9}

	// Action 1: Standard v.4 (Expect 1)
	action1 := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"targets": []interface{}{
				map[string]interface{}{
					"channel_id": "ch1",
					"device_id":  "DeviceStandard",
					"point_id":   "p1",
					"expression": "v.4",
				},
			},
		},
	}

	// Action 2: Shifted v.4 << 3 (Expect 8)
	action2 := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"targets": []interface{}{
				map[string]interface{}{
					"channel_id": "ch1",
					"device_id":  "DeviceShifted",
					"point_id":   "p1",
					"expression": "v.4 * 8",
				},
			},
		},
	}

	// Execute Action 1
	if err := em.executeSingleAction(context.Background(), "rule_bit_std", action1, val, env); err != nil {
		t.Fatalf("Action 1 failed: %v", err)
	}

	// Execute Action 2
	if err := em.executeSingleAction(context.Background(), "rule_bit_shift", action2, val, env); err != nil {
		t.Fatalf("Action 2 failed: %v", err)
	}

	// Verify DeviceStandard received 8 (because v.4 implies writing to Bit 3)
	if v, ok := mockWriter.Writes["DeviceStandard"]; !ok {
		t.Error("DeviceStandard not written")
	} else {
		val, _ := toInt64(v)
		if val != 8 {
			t.Errorf("DeviceStandard: expected 8 (Bit 3 set), got %v", val)
		}
	}

	// Verify DeviceShifted received 8
	if v, ok := mockWriter.Writes["DeviceShifted"]; !ok {
		t.Error("DeviceShifted not written")
	} else {
		val, _ := toInt64(v)
		if val != 8 {
			t.Errorf("DeviceShifted: expected 8, got %v", val)
		}
	}
}
