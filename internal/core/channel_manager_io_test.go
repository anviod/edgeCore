package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

type rwStubDriver struct {
	stubChannelDriver
	writeVal any
	readVal  model.Value
	writeErr error
	readErr  error
}

func (r *rwStubDriver) WritePoint(_ context.Context, pt model.Point, value any) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.writeVal = value
	return nil
}

func (r *rwStubDriver) ReadPoints(_ context.Context, pts []model.Point) (map[string]model.Value, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	out := make(map[string]model.Value, len(pts))
	for _, p := range pts {
		v := r.readVal
		v.PointID = p.ID
		out[p.ID] = v
	}
	return out, nil
}

func channelWithPoint(cm *ChannelManager, rw string) {
	cm.channels["ch-rw"] = &model.Channel{
		ID:       "ch-rw",
		Protocol: "modbus-tcp",
		Enable:   true,
		Devices: []model.Device{
			{
				ID:     "dev-rw",
				Enable: true,
				Config: map[string]any{"slave_id": 1},
				Points: []model.Point{
					{ID: "p-ro", ReadWrite: "R", Address: "40001", DataType: "int16"},
					{ID: "p-rw", ReadWrite: "RW", Address: "40002", DataType: "int16"},
				},
			},
		},
	}
	cm.drivers["ch-rw"] = &rwStubDriver{}
	cm.driverMus["ch-rw"] = &sync.Mutex{}
	cm.stateManager.RegisterNode("dev-rw", "dev-rw")
}

func TestChannelManager_WritePoint(t *testing.T) {
	cm := newTestChannelManager()
	channelWithPoint(cm, "RW")

	if err := cm.WritePoint("ch-rw", "dev-rw", "p-ro", 1); err == nil {
		t.Fatal("read-only point should fail")
	}
	if err := cm.WritePoint("ch-rw", "missing", "p-rw", 1); err == nil {
		t.Fatal("missing device should fail")
	}
	if err := cm.WritePoint("ch-rw", "dev-rw", "missing", 1); err == nil {
		t.Fatal("missing point should fail")
	}

	drv := cm.drivers["ch-rw"].(*rwStubDriver)
	if err := cm.WritePoint("ch-rw", "dev-rw", "p-rw", int16(42)); err != nil {
		t.Fatalf("WritePoint: %v", err)
	}
	if drv.writeVal != int16(42) {
		t.Fatalf("written value = %v", drv.writeVal)
	}
}

func TestChannelManager_WritePoint_ShadowSync(t *testing.T) {
	cm := newTestChannelManager()
	channelWithPoint(cm, "RW")
	sc := NewShadowCore()
	cm.SetShadowCore(sc)

	if err := cm.WritePoint("ch-rw", "dev-rw", "p-rw", 7); err != nil {
		t.Fatalf("WritePoint: %v", err)
	}
	shadow, err := sc.GetShadowDevice("shadow-dev-rw")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	if shadow.Points["p-rw"].Value != 7 {
		t.Fatalf("shadow value = %v", shadow.Points["p-rw"].Value)
	}
}

func TestChannelManager_ReadPoint(t *testing.T) {
	cm := newTestChannelManager()
	channelWithPoint(cm, "RW")
	drv := cm.drivers["ch-rw"].(*rwStubDriver)
	drv.readVal = model.Value{Value: int16(99), Quality: "Good"}

	val, err := cm.ReadPoint("ch-rw", "dev-rw", "p-rw")
	if err != nil {
		t.Fatalf("ReadPoint: %v", err)
	}
	if val.Value != int16(99) {
		t.Fatalf("value = %v", val.Value)
	}

	drv.readErr = errors.New("read failed")
	if _, err := cm.ReadPoint("ch-rw", "dev-rw", "p-rw"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestChannelManager_PublishWrittenValue_Pipeline(t *testing.T) {
	pipeline := NewDataPipeline(8)
	pipeline.Start()
	var got model.Value
	pipeline.AddHandler(func(v model.Value) { got = v })

	cm := NewChannelManager(pipeline, nil)
	cm.publishWrittenValue("ch1", "dev1", "p1", 3.14)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got.PointID == "p1" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got.Value != 3.14 {
		t.Fatalf("pipeline value = %v", got.Value)
	}
}

func TestConnectorStartWarning(t *testing.T) {
	if msg := connectorStartWarning("MQTT", "broker", nil); msg != "" {
		t.Fatalf("nil err message = %q", msg)
	}
	msg := connectorStartWarning("MQTT", "broker", errors.New("connection refused"))
	if msg == "" {
		t.Fatal("expected warning message")
	}
}
