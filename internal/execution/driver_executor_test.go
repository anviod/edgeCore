package execution_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"
	"github.com/anviod/edgex/internal/model"
	"github.com/stretchr/testify/require"
)

type mockSB struct {
	channels map[string]model.Channel
	shadow   map[string]model.ShadowPoint
	writes   []writeCall
	reads    []readCall
}

type writeCall struct {
	ChannelID, DeviceID, PointID string
	Value                        any
}

type readCall struct {
	ChannelID, DeviceID, PointID string
}

func newMockSB() *mockSB {
	return &mockSB{
		channels: map[string]model.Channel{
			"ch1": {
				ID:       "ch1",
				Name:     "Modbus",
				Protocol: "modbus-tcp",
				Enable:   true,
				Devices: []model.Device{{
					ID:     "dev1",
					Name:   "Slave1",
					Enable: true,
					Points: []model.Point{{ID: "p1", Name: "temp", Address: "40001", ReadWrite: "rw"}},
				}},
			},
		},
		shadow: map[string]model.ShadowPoint{
			"ch1/dev1/p1": {Value: 42.5, Quality: "Good", Timestamp: time.Now()},
		},
	}
}

func (m *mockSB) GetChannels() []model.Channel {
	out := make([]model.Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		out = append(out, ch)
	}
	return out
}

func (m *mockSB) GetChannelDevices(channelID string) []model.Device {
	ch, ok := m.channels[channelID]
	if !ok {
		return nil
	}
	return ch.Devices
}

func (m *mockSB) GetDevice(channelID, deviceID string) *model.Device {
	ch, ok := m.channels[channelID]
	if !ok {
		return nil
	}
	for i := range ch.Devices {
		if ch.Devices[i].ID == deviceID {
			return &ch.Devices[i]
		}
	}
	return nil
}

func (m *mockSB) WritePoint(channelID, deviceID, pointID string, value any) error {
	m.writes = append(m.writes, writeCall{channelID, deviceID, pointID, value})
	return nil
}

func (m *mockSB) GetDevicePoints(channelID, deviceID string) ([]model.PointData, error) {
	dev := m.GetDevice(channelID, deviceID)
	if dev == nil {
		return nil, fmt.Errorf("device not found")
	}
	out := make([]model.PointData, 0, len(dev.Points))
	for _, p := range dev.Points {
		out = append(out, model.PointData{ID: p.ID, Name: p.Name, Address: p.Address, Value: 1})
	}
	return out, nil
}

func (m *mockSB) GetShadowPoint(channelID, deviceID, pointID string) (*model.ShadowPoint, error) {
	key := channelID + "/" + deviceID + "/" + pointID
	pt, ok := m.shadow[key]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return &pt, nil
}

func (m *mockSB) ReadPoint(channelID, deviceID, pointID string) (model.Value, error) {
	m.reads = append(m.reads, readCall{channelID, deviceID, pointID})
	return model.Value{Value: 99, Quality: "Good", TS: time.Now()}, nil
}

func (m *mockSB) GetDeviceDiagnostics(deviceID string) map[string]any {
	return map[string]any{"device_id": deviceID, "online": true}
}

func TestDriverExecutorReadWriteViaMapper(t *testing.T) {
	sb := newMockSB()
	exec := execution.NewDriverExecutor(sb)
	mapper := execution.NewCapabilityMapper(exec)

	cap := capability.Capability{
		ID:       "modbus_tcp.read_holding_register",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"protocol": "modbus-tcp", "driver_command": "ReadPoints"},
	}
	out, err := mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: cap.ID,
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
		},
	}, cap)
	require.NoError(t, err)
	m := out.(map[string]any)
	require.Equal(t, "dev1", m["device_id"])
	require.Len(t, sb.reads, 1)

	writeCap := capability.Capability{
		ID:       "modbus_tcp.write_register",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"protocol": "modbus-tcp", "driver_command": "WritePoint"},
	}
	out, err = mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: writeCap.ID,
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
			"value":     25.5,
		},
	}, writeCap)
	require.NoError(t, err)
	require.Len(t, sb.writes, 1)
	require.Equal(t, 25.5, sb.writes[0].Value)
	wm := out.(map[string]any)
	require.Equal(t, true, wm["success"])
}

func TestDriverExecutorPreferShadow(t *testing.T) {
	sb := newMockSB()
	exec := execution.NewDriverExecutor(sb)
	out, err := exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":      "dev1",
			"address":        "p1",
			"prefer_shadow":  true,
		},
	})
	require.NoError(t, err)
	require.Empty(t, sb.reads)
	m := out.(map[string]any)
	vals := m["values"].([]map[string]any)
	require.Equal(t, "shadow", vals[0]["source"])
	require.Equal(t, 42.5, vals[0]["value"])
}

func TestDriverExecutorDiagnosticsAndReject(t *testing.T) {
	sb := newMockSB()
	exec := execution.NewDriverExecutor(sb)
	out, err := exec.Execute(context.Background(), execution.DriverCommand{
		Command: "Diagnostics",
		Args:    map[string]any{"device_id": "dev1"},
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	_, err = exec.Execute(context.Background(), execution.DriverCommand{Command: "AI.doc_parse"})
	require.Error(t, err)

	_, err = execution.NewDriverExecutor(nil).Execute(context.Background(), execution.DriverCommand{Command: "ReadPoints"})
	require.Error(t, err)
}

func TestDispatcherInvokeStateMachineWithExecutor(t *testing.T) {
	sb := newMockSB()
	agent := capability.Agent{ID: "node-1", Kind: capability.AgentKindDevice, Version: "2.0.0"}
	reg := capability.NewRegistry(agent)
	require.NoError(t, reg.RegisterAll(capability.GenerateProtocolCapabilities("node-1", "modbus-tcp")))

	disp := capability.NewDispatcher(reg)
	disp.SetMapper(execution.NewCapabilityMapper(execution.NewDriverExecutor(sb)))

	resp := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "modbus_tcp.write_register",
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
			"value":     7,
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)

	fail := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "modbus_tcp.write_register",
		Arguments: map[string]any{
			"device_id": "missing",
			"address":   "p1",
			"value":     1,
		},
	})
	require.Equal(t, capability.InvokeFailed, fail.Status)
	require.Equal(t, "E001", fail.Result.ErrorCode)
}
