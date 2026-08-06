package validate

import (
	"testing"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
	"github.com/stretchr/testify/assert"
)

func TestValidateDeliverables_Pass(t *testing.T) {
	v := New()
	report := v.ValidateDeliverables(&aitypes.Deliverables{
		ProtocolModel: &aitypes.ProtocolModel{ProtocolID: "modbus-tcp", Confidence: 0.95},
		PointDefinition: &aitypes.PointDefinition{
			ProtocolID: "modbus-tcp",
			Points: []aitypes.PointCandidate{
				{ID: "uab", Address: "40001", Datatype: "float32", Confidence: 0.87},
			},
		},
		DriverParameter: &aitypes.DriverParameter{
			ProtocolID: "modbus-tcp", Name: "ch1",
			Connection: map[string]any{"ip": "192.168.1.1"},
		},
		ValidationCase: &aitypes.ValidationCase{
			Cases: []aitypes.ValidationCaseEntry{
				{PointID: "uab", ExpectedValue: 220.5, FrameEvidence: aitypes.FrameEvidence{RawHex: "43DC6666"}},
			},
		},
	})
	assert.NotNil(t, report)
	assert.Greater(t, report.PassRate, 80.0)
}

func TestValidateDeliverables_Empty(t *testing.T) {
	v := New()
	report := v.ValidateDeliverables(nil)
	assert.False(t, report.Passed)
}
