package core

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

const integrationMockProtocol = "integration-mock"

type integrationMockDriver struct {
	mu            sync.Mutex
	writtenValues map[string]any
	readValues    map[string]any
}

func newIntegrationMockDriver() *integrationMockDriver {
	return &integrationMockDriver{
		writtenValues: make(map[string]any),
		readValues: map[string]any{
			"temp": 25.5,
			"sp":   20.0,
		},
	}
}

func (d *integrationMockDriver) Init(cfg model.DriverConfig) error           { return nil }
func (d *integrationMockDriver) Connect(ctx context.Context) error           { return nil }
func (d *integrationMockDriver) Disconnect() error                           { return nil }
func (d *integrationMockDriver) Health() driver.HealthStatus                 { return driver.HealthStatusGood }
func (d *integrationMockDriver) SetSlaveID(slaveID uint8) error              { return nil }
func (d *integrationMockDriver) SetDeviceConfig(config map[string]any) error { return nil }
func (d *integrationMockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func (d *integrationMockDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]model.Value, len(points))
	now := time.Now()
	for _, p := range points {
		val := d.readValues[p.ID]
		out[p.ID] = model.Value{PointID: p.ID, Value: val, Quality: "Good", TS: now}
	}
	return out, nil
}

func (d *integrationMockDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writtenValues[point.ID] = value
	d.readValues[point.ID] = value
	return nil
}

func (d *integrationMockDriver) lastWrite(pointID string) (any, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.writtenValues[pointID]
	return v, ok
}

func init() {
	driver.RegisterDriver(integrationMockProtocol, func() driver.Driver {
		return newIntegrationMockDriver()
	})
}

func setupIntegrationStack(t *testing.T) (*ChannelManager, *ShadowCore, *DataPipeline, *integrationMockDriver, func()) {
	t.Helper()

	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	sc := NewShadowCore()
	pipeline := NewDataPipeline(100)
	pipeline.Start()
	NewShadowBridge(pipeline).Attach(sc)

	cm := NewChannelManager(pipeline, nil)
	cm.SetShadowCore(sc)

	ch := &model.Channel{
		ID:       "ch-nb",
		Name:     "Integration Channel",
		Protocol: integrationMockProtocol,
		Enable:   true,
		Devices: []model.Device{
			{
				ID:     "dev-nb",
				Name:   "Integration Device",
				Enable: true,
				Points: []model.Point{
					{ID: "temp", Name: "Temperature", DataType: "float64", ReadWrite: "R"},
					{ID: "sp", Name: "Setpoint", DataType: "float64", ReadWrite: "RW"},
				},
			},
		},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	mockDrv := cm.drivers["ch-nb"].(*integrationMockDriver)

	cleanup := func() {
		sc.Stop()
		store.Close()
		os.RemoveAll(tmpDir)
	}
	return cm, sc, pipeline, mockDrv, cleanup
}

func waitForPipelineValues(t *testing.T, mu *sync.Mutex, received *[]model.Value, minCount int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*received)
		mu.Unlock()
		if n >= minCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	n := len(*received)
	mu.Unlock()
	t.Fatalf("pipeline received %d values, want >= %d within %s", n, minCount, timeout)
}

func TestSouthToNorth_CollectionPublishesToPipeline(t *testing.T) {
	_, sc, pipeline, _, cleanup := setupIntegrationStack(t)
	defer cleanup()

	var mu sync.Mutex
	var northboundReceived []model.Value
	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		northboundReceived = append(northboundReceived, v)
		mu.Unlock()
	})

	collectedAt := time.Now().UTC().Truncate(time.Millisecond)
	msg := model.ShadowIngressMessage{
		DeviceID:  "dev-nb",
		ChannelID: "ch-nb",
		Timestamp: collectedAt,
		Points: []model.ShadowIngressPoint{
			{PointID: "temp", Value: 25.5, Quality: "Good", CollectedAt: collectedAt},
			{PointID: "sp", Value: 20.0, Quality: "Good", CollectedAt: collectedAt},
		},
		Meta: model.ShadowIngressMeta{Source: "scan_engine"},
	}
	if _, err := sc.WriteShadowDevice(msg); err != nil {
		t.Fatalf("WriteShadowDevice: %v", err)
	}

	waitForPipelineValues(t, &mu, &northboundReceived, 2, 2*time.Second)

	mu.Lock()
	defer mu.Unlock()
	for _, v := range northboundReceived {
		if v.ChannelID != "ch-nb" || v.DeviceID != "dev-nb" {
			t.Errorf("unexpected routing: %+v", v)
		}
	}
}

func TestNorthToSouth_WritePointUpdatesDeviceAndShadow(t *testing.T) {
	cm, sc, pipeline, mockDrv, cleanup := setupIntegrationStack(t)
	defer cleanup()

	var mu sync.Mutex
	var pipelineReceived []model.Value
	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		pipelineReceived = append(pipelineReceived, v)
		mu.Unlock()
	})

	if err := cm.WritePoint("ch-nb", "dev-nb", "sp", 42.0); err != nil {
		t.Fatalf("WritePoint: %v", err)
	}

	val, ok := mockDrv.lastWrite("sp")
	if !ok {
		t.Fatal("driver did not receive write")
	}
	if val != 42.0 {
		t.Fatalf("driver write value = %v, want 42.0", val)
	}

	shadow, err := sc.GetShadowDevice("shadow-dev-nb")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	pt, ok := shadow.Points["sp"]
	if !ok {
		t.Fatal("shadow missing setpoint")
	}
	if pt.Value != 42.0 {
		t.Fatalf("shadow value = %v, want 42.0", pt.Value)
	}

	waitForPipelineValues(t, &mu, &pipelineReceived, 1, 2*time.Second)
	mu.Lock()
	last := pipelineReceived[len(pipelineReceived)-1]
	mu.Unlock()
	if last.PointID != "sp" || last.Value != 42.0 {
		t.Fatalf("pipeline last value = %+v, want sp=42.0", last)
	}
}

func TestNorthToSouth_WritePointRejectsReadOnly(t *testing.T) {
	cm, _, _, mockDrv, cleanup := setupIntegrationStack(t)
	defer cleanup()

	err := cm.WritePoint("ch-nb", "dev-nb", "temp", 99.0)
	if err == nil {
		t.Fatal("expected error writing read-only point")
	}
	if _, ok := mockDrv.lastWrite("temp"); ok {
		t.Fatal("driver should not receive write for read-only point")
	}
}

func TestBidirectionalCommunication_FullRoundTrip(t *testing.T) {
	cm, sc, pipeline, mockDrv, cleanup := setupIntegrationStack(t)
	defer cleanup()

	var mu sync.Mutex
	var northboundReceived []model.Value
	pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		northboundReceived = append(northboundReceived, v)
		mu.Unlock()
	})

	// 南向采集上报
	collectedAt := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := sc.WriteShadowDevice(model.ShadowIngressMessage{
		DeviceID:  "dev-nb",
		ChannelID: "ch-nb",
		Timestamp: collectedAt,
		Points: []model.ShadowIngressPoint{
			{PointID: "sp", Value: 20.0, Quality: "Good", CollectedAt: collectedAt},
		},
		Meta: model.ShadowIngressMeta{Source: "scan_engine"},
	}); err != nil {
		t.Fatalf("southbound ingest: %v", err)
	}
	waitForPipelineValues(t, &mu, &northboundReceived, 1, 2*time.Second)

	// 北向下发反控
	if err := cm.WritePoint("ch-nb", "dev-nb", "sp", 88.0); err != nil {
		t.Fatalf("northbound write: %v", err)
	}

	val, ok := mockDrv.lastWrite("sp")
	if !ok || val != 88.0 {
		t.Fatalf("device write = %v, want 88.0", val)
	}

	shadow, err := sc.GetShadowDevice("shadow-dev-nb")
	if err != nil {
		t.Fatalf("GetShadowDevice: %v", err)
	}
	if shadow.Points["sp"].Value != 88.0 {
		t.Fatalf("shadow after write = %v, want 88.0", shadow.Points["sp"].Value)
	}

	waitForPipelineValues(t, &mu, &northboundReceived, 2, 2*time.Second)
	mu.Lock()
	last := northboundReceived[len(northboundReceived)-1]
	mu.Unlock()
	if last.Value != 88.0 {
		t.Fatalf("northbound fan-out after write = %+v, want value 88.0", last)
	}
}

func TestNorthboundManager_ReceivesPipelineFromWrite(t *testing.T) {
	cm, sc, pipeline, _, cleanup := setupIntegrationStack(t)
	defer cleanup()

	nbm := NewNorthboundManager(model.NorthboundConfig{}, pipeline, cm, nil, nil)

	var mu sync.Mutex
	var mqttPublished []model.Value
	nbm.pipeline.AddHandler(func(v model.Value) {
		mu.Lock()
		mqttPublished = append(mqttPublished, v)
		mu.Unlock()
	})

	cm.SetShadowCore(sc)
	if err := cm.WritePoint("ch-nb", "dev-nb", "sp", 55.0); err != nil {
		t.Fatalf("WritePoint: %v", err)
	}

	waitForPipelineValues(t, &mu, &mqttPublished, 1, 2*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if mqttPublished[len(mqttPublished)-1].Value != 55.0 {
		t.Fatalf("northbound handler value = %v, want 55.0", mqttPublished[len(mqttPublished)-1].Value)
	}
}
