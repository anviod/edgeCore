package sparkplugb

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestPublishRespectsPeriodicStrategy(t *testing.T) {
	c := NewClient(model.SparkplugBConfig{
		Enable:  true,
		GroupID: "g1",
		NodeID:  "n1",
		Devices: model.OpcUaDeviceMap{
			"dev-1": {Enable: true, Strategy: "periodic", Interval: model.Duration(time.Second)},
		},
	})

	c.periodicMu.Lock()
	c.periodic["dev-1"] = &periodicItem{
		values: make(map[string]model.Value),
		ticker: time.NewTicker(time.Second),
		stop:   make(chan struct{}),
	}
	c.periodicMu.Unlock()

	v := model.Value{DeviceID: "dev-1", PointID: "p1", Value: 42}
	c.Publish(v)

	c.periodicMu.Lock()
	defer c.periodicMu.Unlock()
	if len(c.periodic["dev-1"].values) != 1 {
		t.Fatalf("expected periodic cache to hold value, got %d", len(c.periodic["dev-1"].values))
	}
}

func TestPublishRespectsChangeStrategy(t *testing.T) {
	c := NewClient(model.SparkplugBConfig{
		Enable:  true,
		GroupID: "g1",
		NodeID:  "n1",
		Devices: model.OpcUaDeviceMap{
			"dev-1": {Enable: true, Strategy: "change"},
		},
	})

	v := model.Value{DeviceID: "dev-1", PointID: "p1", Value: 42}
	c.lastValues.Store("dev-1:p1", 42)

	// No MQTT client connected; Publish returns early after change detection would skip duplicate.
	c.Publish(v)
}

func TestScheduleReconnectSingleFlight(t *testing.T) {
	c := NewClient(model.SparkplugBConfig{Enable: true, Broker: "127.0.0.1", Port: 1})

	var started int32
	for i := 0; i < 10; i++ {
		if c.reconnectSched.TryStart() {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("expected exactly one reconnect loop to start, got %d", started)
	}
	c.reconnectSched.Done()
	if !c.reconnectSched.TryStart() {
		t.Fatal("expected TryStart to succeed after Done")
	}
	c.reconnectSched.Done()
}
