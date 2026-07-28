package capability_test

import (
	"testing"

	"github.com/anviod/edgex/internal/capability"
	"github.com/stretchr/testify/require"
)

func TestShadowEventBridgePublishesPointChanged(t *testing.T) {
	bus := capability.NewMemoryBus()
	pub := capability.NewEventPublisher("edgex-node-001", bus)
	bridge := capability.NewShadowEventBridge()
	bridge.AddPublisher(pub)

	bridge.HandleDelta("slave-1", "ch-1", map[string]capability.ShadowPointView{
		"temperature": {
			Value:         45.2,
			PreviousValue: 42.1,
			Quality:       "Good",
			TimestampMs:   123,
		},
	})

	msgs := bus.Published()
	require.NotEmpty(t, msgs)
	require.Equal(t, capability.TopicEvent("edgex-node-001"), msgs[0].Topic)
}
