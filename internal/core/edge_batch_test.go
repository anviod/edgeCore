package core

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

type MockBatchWriter struct {
	Writes map[string]any // key: deviceID, value: value written
}

func (m *MockBatchWriter) WritePoint(channelID, deviceID, pointID string, value any) error {
	if m.Writes == nil {
		m.Writes = make(map[string]any)
	}
	m.Writes[deviceID] = value
	return nil
}

func (m *MockBatchWriter) ReadPoint(channelID, deviceID, pointID string) (model.Value, error) {
	return model.Value{Value: 0}, nil
}

func TestEdgeAction_BatchBitExtraction(t *testing.T) {
	em := NewEdgeComputeManager(nil, nil, nil)
	mockWriter := &MockBatchWriter{}
	em.SetDeviceWriter(mockWriter)

	// Trigger value: 1 (Binary 0001)
	// Bit 1 (v.1) should be 1
	// Bit 4 (v.4) should be 0
	val := model.Value{Value: 1}
	env := map[string]any{"v": 1}

	action := model.RuleAction{
		Type: "device_control",
		Config: map[string]any{
			"targets": []interface{}{
				map[string]interface{}{
					"channel_id": "ch1",
					"device_id":  "DeviceA",
					"point_id":   "p1",
					"expression": "v.1", // Should be bit 0 -> 1
				},
				map[string]interface{}{
					"channel_id": "ch1",
					"device_id":  "DeviceB",
					"point_id":   "p1",
					"expression": "v.4", // Should be bit 3 -> 0
				},
			},
		},
	}

	err := em.executeSingleAction(context.Background(), "rule1", action, val, env)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	// Verify DeviceA
	if v, ok := mockWriter.Writes["DeviceA"]; !ok {
		t.Error("DeviceA was not written to")
	} else {
		val, err := toInt64(v)
		if err != nil {
			t.Errorf("DeviceA value error: %v", err)
		} else if val != 1 {
			t.Errorf("DeviceA: expected 1, got %v", val)
		}
	}

	// Verify DeviceB
	if v, ok := mockWriter.Writes["DeviceB"]; !ok {
		t.Error("DeviceB was not written to")
	} else {
		val, err := toInt64(v)
		if err != nil {
			t.Errorf("DeviceB value error: %v", err)
		} else if val != 0 {
			t.Errorf("DeviceB: expected 0, got %v", val)
		}
	}
}
