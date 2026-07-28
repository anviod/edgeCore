package edgos_nats_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/northbound/edgos_nats"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const natsURL = "nats://127.0.0.1:4222"

type stubSB struct{}

func (stubSB) GetChannels() []model.Channel                { return nil }
func (stubSB) GetChannelDevices(string) []model.Device     { return nil }
func (stubSB) GetDevice(string, string) *model.Device      { return nil }
func (stubSB) WritePoint(string, string, string, any) error { return fmt.Errorf("no device") }
func (stubSB) GetDevicePoints(string, string) ([]model.PointData, error) {
	return nil, fmt.Errorf("no device")
}
func (stubSB) GetShadowPoint(string, string, string) (*model.ShadowPoint, error) {
	return nil, fmt.Errorf("no shadow")
}

func TestEANIntegrationNATSDiscoveryInvokeEvent(t *testing.T) {
	nc, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		t.Skipf("NATS %s not reachable: %v", natsURL, err)
	}
	defer nc.Close()

	nodeID := fmt.Sprintf("ean-nats-it-%d", time.Now().UnixNano()%1_000_000)

	var (
		mu          sync.Mutex
		discoveries []string
		events      []capability.Event
		replies     []capability.InvokeResponse
	)

	subs := make([]*nats.Subscription, 0, 4)
	mustSub := func(subject string, handler func([]byte)) {
		sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
			handler(msg.Data)
		})
		require.NoError(t, err)
		subs = append(subs, sub)
	}
	defer func() {
		for _, s := range subs {
			_ = s.Unsubscribe()
		}
	}()

	mustSub(capability.TopicDiscoveryAgent, func(payload []byte) {
		mu.Lock()
		discoveries = append(discoveries, "agent:"+string(payload))
		mu.Unlock()
	})
	mustSub(capability.TopicDiscoveryCapability, func(payload []byte) {
		mu.Lock()
		discoveries = append(discoveries, "capability:"+string(payload))
		mu.Unlock()
	})
	mustSub(capability.TopicEvent(nodeID), func(payload []byte) {
		var msg capability.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			return
		}
		body, err := capability.DecodeBody[capability.Event](msg.Body)
		if err != nil {
			return
		}
		mu.Lock()
		events = append(events, body)
		mu.Unlock()
	})
	mustSub(capability.TopicReply("edgeos-planner"), func(payload []byte) {
		var msg capability.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			return
		}
		body, err := capability.DecodeBody[capability.InvokeResponse](msg.Body)
		if err != nil {
			return
		}
		mu.Lock()
		replies = append(replies, body)
		mu.Unlock()
	})
	require.NoError(t, nc.Flush())

	cfg := model.EdgeOSNATSConfig{
		ID:             "ean-nats-it-client",
		Name:           "EAN NATS Integration",
		Enable:         true,
		URL:            natsURL,
		NodeID:         nodeID,
		ConnectTimeout: 5,
		ReconnectWait:  1,
		MaxReconnects:  1,
	}
	client := edgos_nats.NewClient(cfg, stubSB{}, nil)
	require.NoError(t, client.Start())
	defer client.Stop()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if client.GetStatus() == edgos_nats.StatusConnected && client.CapabilityRuntime() != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, edgos_nats.StatusConnected, client.GetStatus(), "expected NATS connected")
	require.NotNil(t, client.CapabilityRuntime(), "expected EAN runtime")

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		hasAgent, hasCap := false, false
		for _, d := range discoveries {
			if len(d) > 6 && d[:6] == "agent:" {
				hasAgent = true
			}
			if len(d) > 11 && d[:11] == "capability:" {
				hasCap = true
			}
		}
		return hasAgent && hasCap
	}, 15*time.Second, 100*time.Millisecond, "expected discovery agent+capability on NATS")

	req := capability.NewEnvelope("edgeos-planner", capability.MessageTypeInvokeCapability, capability.InvokeRequest{
		InvokeID:   "invoke-nats-it-1",
		Target:     nodeID,
		Capability: "system.diagnostics",
		Arguments:  map[string]any{},
	})
	req.Header.CorrelationID = "corr-nats-it"
	payload, err := json.Marshal(req)
	require.NoError(t, err)
	require.NoError(t, nc.Publish(capability.TopicInvoke(nodeID), payload))
	require.NoError(t, nc.Flush())

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, r := range replies {
			if r.InvokeID == "invoke-nats-it-1" && r.Status == capability.InvokeCompleted {
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond, "expected invoke reply")

	rt := client.CapabilityRuntime()
	require.NoError(t, rt.Events().PublishPointChanged("dev-1", "temp", 36.6, 35.0, map[string]any{"quality": "Good"}))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, evt := range events {
			if evt.PointID == "temp" && evt.PreviousValue != nil {
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond, "expected EAN event with previous_value")

	mu.Lock()
	var matched capability.Event
	for _, evt := range events {
		if evt.PointID == "temp" {
			matched = evt
			break
		}
	}
	mu.Unlock()
	require.Equal(t, "temp.changed", matched.EventType)
	require.Equal(t, 36.6, matched.Value)
	require.Equal(t, 35.0, matched.PreviousValue)

	t.Logf("NATS EAN integration OK: discoveries=%d events=%d replies=%d node=%s",
		len(discoveries), len(events), len(replies), nodeID)
}
