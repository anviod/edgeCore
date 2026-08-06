package edgos_mqtt_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/northbound/edgos_mqtt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

const mqttBroker = "tcp://127.0.0.1:18083"

type stubSB struct{}

func (stubSB) GetChannels() []model.Channel                 { return nil }
func (stubSB) GetChannelDevices(string) []model.Device      { return nil }
func (stubSB) GetDevice(string, string) *model.Device       { return nil }
func (stubSB) WritePoint(string, string, string, any) error { return fmt.Errorf("no device") }
func (stubSB) GetDevicePoints(string, string) ([]model.PointData, error) {
	return nil, fmt.Errorf("no device")
}
func (stubSB) GetShadowPoint(string, string, string) (*model.ShadowPoint, error) {
	return nil, fmt.Errorf("no shadow")
}

func TestEANIntegrationMQTTDiscoveryInvokeEvent(t *testing.T) {
	// Probe broker availability.
	probe := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(mqttBroker).SetClientID("ean-probe-" + fmt.Sprint(time.Now().UnixNano())).SetConnectTimeout(3 * time.Second))
	token := probe.Connect()
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Skipf("MQTT broker %s not reachable: %v", mqttBroker, token.Error())
	}
	probe.Disconnect(100)

	nodeID := fmt.Sprintf("ean-it-%d", time.Now().UnixNano()%1_000_000)
	observerID := "ean-observer-" + nodeID

	var (
		mu          sync.Mutex
		discoveries []string
		events      []string
		replies     []capability.InvokeResponse
	)

	obsOpts := mqtt.NewClientOptions()
	obsOpts.AddBroker(mqttBroker)
	obsOpts.SetClientID(observerID)
	obsOpts.SetConnectTimeout(5 * time.Second)
	observer := mqtt.NewClient(obsOpts)
	tok := observer.Connect()
	require.True(t, tok.WaitTimeout(8*time.Second))
	require.NoError(t, tok.Error())
	defer observer.Disconnect(250)

	sub := func(topic string, handler func([]byte)) {
		token := observer.Subscribe(topic, 1, func(_ mqtt.Client, msg mqtt.Message) {
			handler(msg.Payload())
		})
		require.True(t, token.WaitTimeout(5*time.Second))
		require.NoError(t, token.Error())
	}

	sub(capability.TopicDiscoveryAgent, func(payload []byte) {
		mu.Lock()
		discoveries = append(discoveries, "agent:"+string(payload))
		mu.Unlock()
	})
	sub(capability.TopicDiscoveryCapability, func(payload []byte) {
		mu.Lock()
		discoveries = append(discoveries, "capability:"+string(payload))
		mu.Unlock()
	})
	sub(capability.TopicEvent(nodeID), func(payload []byte) {
		mu.Lock()
		events = append(events, string(payload))
		mu.Unlock()
	})
	sub(capability.TopicReply("edgeos-planner"), func(payload []byte) {
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

	cfg := model.EdgeOSMQTTConfig{
		ID:             "ean-it-client",
		Name:           "EAN Integration",
		Enable:         true,
		Broker:         mqttBroker,
		ClientID:       "ean-edgeCore-" + nodeID,
		NodeID:         nodeID,
		CleanSession:   true,
		KeepAlive:      30,
		ConnectTimeout: 5,
		AutoReconnect:  false,
		EANEnabled:     true,
	}
	client := edgos_mqtt.NewClient(cfg, stubSB{}, nil)
	require.NoError(t, client.Start())
	defer client.Stop()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if client.GetStatus() == edgos_mqtt.StatusConnected && client.CapabilityRuntime() != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, edgos_mqtt.StatusConnected, client.GetStatus(), "expected MQTT connected")
	require.NotNil(t, client.CapabilityRuntime(), "expected EAN runtime")

	// Wait for Discovery publish
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
	}, 15*time.Second, 100*time.Millisecond, "expected discovery agent+capability on broker")

	// Invoke over bus
	req := capability.NewEnvelope("edgeos-planner", capability.MessageTypeInvokeCapability, capability.InvokeRequest{
		InvokeID:   "invoke-mqtt-it-1",
		Target:     nodeID,
		Capability: "system.diagnostics",
		Arguments:  map[string]any{},
	})
	req.Header.CorrelationID = "corr-mqtt-it"
	payload, err := json.Marshal(req)
	require.NoError(t, err)
	pubTok := observer.Publish(capability.TopicInvoke(nodeID), 1, false, payload)
	require.True(t, pubTok.WaitTimeout(5*time.Second))
	require.NoError(t, pubTok.Error())

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, r := range replies {
			if r.InvokeID == "invoke-mqtt-it-1" && r.Status == capability.InvokeCompleted {
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond, "expected invoke reply")

	// Event publish with previous_value
	rt := client.CapabilityRuntime()
	require.NoError(t, rt.Events().PublishPointChanged("dev-1", "temp", 36.6, 35.0, map[string]any{"quality": "Good"}))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) > 0
	}, 10*time.Second, 100*time.Millisecond, "expected EAN event on broker")

	mu.Lock()
	var evt capability.Event
	parsed := false
	for _, raw := range events {
		var msg capability.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		body, err := capability.DecodeBody[capability.Event](msg.Body)
		if err != nil {
			continue
		}
		if body.PointID == "temp" {
			evt = body
			parsed = true
			break
		}
	}
	mu.Unlock()
	require.True(t, parsed, "expected parseable temp.changed event")
	require.Equal(t, "temp.changed", evt.EventType)
	require.Equal(t, 36.6, evt.Value)
	require.Equal(t, 35.0, evt.PreviousValue)

	t.Logf("MQTT EAN integration OK: discoveries=%d events=%d replies=%d node=%s",
		len(discoveries), len(events), len(replies), nodeID)
}
