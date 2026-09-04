package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type execStubDriver struct {
	readErr error
	values  map[string]model.Value
}

func (s *execStubDriver) Init(_ model.DriverConfig) error { return nil }
func (s *execStubDriver) Connect(_ context.Context) error { return nil }
func (s *execStubDriver) Disconnect() error               { return nil }
func (s *execStubDriver) ReadPoints(_ context.Context, pts []model.Point) (map[string]model.Value, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	if s.values != nil {
		return s.values, nil
	}
	out := make(map[string]model.Value, len(pts))
	for _, p := range pts {
		out[p.ID] = model.Value{PointID: p.ID, Quality: "Good", Value: 1}
	}
	return out, nil
}
func (s *execStubDriver) WritePoint(_ context.Context, _ model.Point, _ any) error { return nil }
func (s *execStubDriver) Health() driver.HealthStatus                              { return driver.HealthStatusGood }
func (s *execStubDriver) SetSlaveID(_ uint8) error                                 { return nil }
func (s *execStubDriver) SetDeviceConfig(_ map[string]any) error                   { return nil }
func (s *execStubDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func TestExecutionLayer_SerialExecute(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	drv := &execStubDriver{}
	el.RegisterDriver("dev-serial", drv)

	task := &ScanTask{
		DeviceKey: "dev-serial",
		Protocol:  "modbus-tcp",
		Points:    []model.Point{{ID: "p1"}},
	}
	result := el.Execute(task)
	if result == nil || !result.Success {
		t.Fatalf("serial execute = %+v", result)
	}
	if len(result.Values) != 1 {
		t.Fatalf("values = %d, want 1", len(result.Values))
	}
}

func TestExecutionLayer_ParallelExecute(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("opc-ua", ProtocolTypeParallel)
	el.RegisterDriver("dev-par", &execStubDriver{})

	task := &ScanTask{
		DeviceKey: "dev-par",
		Protocol:  "opc-ua",
		Points:    []model.Point{{ID: "p1"}, {ID: "p2"}},
	}
	result := el.Execute(task)
	if result == nil || !result.Success {
		t.Fatalf("parallel execute = %+v", result)
	}
}

func TestExecutionLayer_DriverNotFound(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)

	task := &ScanTask{
		DeviceKey: "missing",
		Protocol:  "modbus-tcp",
		Points:    []model.Point{{ID: "p1"}},
	}
	result := el.Execute(task)
	if result == nil || result.Success {
		t.Fatal("expected failure for missing driver")
	}
	if !errors.Is(result.Error, ErrDriverNotFound) {
		t.Fatalf("error = %v, want ErrDriverNotFound", result.Error)
	}
}

func TestExecutionLayer_ReadError(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	el.RegisterDriver("dev-err", &execStubDriver{readErr: errors.New("read failed")})

	task := &ScanTask{
		DeviceKey: "dev-err",
		Protocol:  "modbus-tcp",
		Points:    []model.Point{{ID: "p1"}},
	}
	result := el.Execute(task)
	if result == nil || result.Success {
		t.Fatal("expected read failure")
	}
}

func TestExecutionLayer_IOProfileProvider(t *testing.T) {
	el := NewExecutionLayer()
	el.ioProfileProvider = func(deviceID string) DeviceIOProfile {
		return DeviceIOProfile{Gap: 3, BatchSize: 20}
	}
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	el.RegisterDriver("dev-io", &execStubDriver{})

	task := &ScanTask{
		DeviceKey: "dev-io",
		Protocol:  "modbus-tcp",
		Points:    []model.Point{{ID: "p1"}},
	}
	result := el.Execute(task)
	if result == nil || !result.Success {
		t.Fatalf("execute with io profile = %+v", result)
	}
}

func TestExecutionLayer_SetPointDegradation(t *testing.T) {
	el := NewExecutionLayer()
	pd := NewPointDegradationManager()
	el.SetPointDegradation(pd)
	if el.pointDegradation != pd {
		t.Fatal("point degradation manager not set")
	}
}

func TestExecutionLayer_Stop(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("modbus-tcp", ProtocolTypeSerial)
	el.RegisterDriver("dev-stop", &execStubDriver{})
	el.Stop()
}

func TestExecutionLayer_LimitedProtocol(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterProtocol("s7", ProtocolTypeLimited)
	el.RegisterDriver("dev-s7", &execStubDriver{})

	task := &ScanTask{
		DeviceKey: "dev-s7",
		Protocol:  "s7",
		Points:    []model.Point{{ID: "p1"}},
	}
	result := el.Execute(task)
	if result == nil || !result.Success {
		t.Fatalf("limited execute = %+v", result)
	}
}

func TestExecutionLayer_UnregisterDriver(t *testing.T) {
	el := NewExecutionLayer()
	el.RegisterDriver("dev-rm", &execStubDriver{})
	el.GetCircuitBreaker().Record("dev-rm", false, true)
	el.UnregisterDriver("dev-rm")
	if el.GetDriver("dev-rm") != nil {
		t.Fatal("driver should be removed")
	}
	snap := el.GetCircuitBreaker().Snapshot()
	devices, _ := snap["devices"].(map[string]any)
	if _, ok := devices["dev-rm"]; ok {
		t.Fatal("circuit breaker entry should be removed")
	}
}

func TestExecutionLayer_CircuitBreakerKey(t *testing.T) {
	el := NewExecutionLayer()
	task := &ScanTask{DeviceKey: "dev-cb", Params: map[string]any{"channelID": "ch1"}}
	key := el.circuitBreakerKey(task)
	if key == "" {
		t.Fatal("circuit breaker key should not be empty")
	}
}
