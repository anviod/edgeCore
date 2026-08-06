package pipeline

import (
	"testing"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockRunner_GenerateDeliverables(t *testing.T) {
	t.Parallel()
	r := NewMockRunner("local")
	d := r.GenerateDeliverables(aitypes.SkillProtocolReverse, "modbus-tcp", "test.pcap", nil)

	require.NotNil(t, d.ProtocolModel)
	assert.Equal(t, "modbus-tcp", d.ProtocolModel.ProtocolID)
	require.NotNil(t, d.PointDefinition)
	assert.NotEmpty(t, d.PointDefinition.Points)
	require.NotNil(t, d.DriverParameter)
	require.NotNil(t, d.ValidationCase)
}

func TestMockRunner_GenerateEdgeRuleDraft(t *testing.T) {
	t.Parallel()
	r := NewMockRunner("local")
	draft := r.GenerateEdgeRuleDraft("冷机出水温度超过12度持续30秒触发MQTT报警")

	assert.Equal(t, "threshold", draft["trigger"].(map[string]any)["type"])
	actions, ok := draft["actions"].([]map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, actions)
}
