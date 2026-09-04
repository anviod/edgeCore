package execution_test

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/execution"
	"github.com/stretchr/testify/require"
)

func TestCapabilityMapperMapAndExecuteWithoutExecutor(t *testing.T) {
	mapper := execution.NewCapabilityMapper(nil)
	cap := capability.Capability{
		ID:       "s7.read_point",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"protocol": "s7"},
	}
	out, err := mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: cap.ID,
		Arguments:  map[string]any{"device_id": "plc1"},
	}, cap)
	require.NoError(t, err)
	m, ok := out.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, m["mapped"])
	require.Equal(t, "ReadPoints", m["command"])
}
