package execution_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/capability"
	"github.com/anviod/edgeCore/internal/execution"
	"github.com/anviod/edgeCore/internal/model"
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
		ID:       "modbus_tcp.read_point",
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
		ID:       "modbus_tcp.write_point",
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
			"device_id":     "dev1",
			"address":       "p1",
			"prefer_shadow": true,
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
		Capability: "modbus_tcp.write_point",
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
			"value":     7,
		},
	})
	require.Equal(t, capability.InvokeCompleted, resp.Status)
	require.True(t, resp.Result.Success)

	fail := disp.Dispatch(context.Background(), capability.InvokeRequest{
		Capability: "modbus_tcp.write_point",
		Arguments: map[string]any{
			"device_id": "missing",
			"address":   "p1",
			"value":     1,
		},
	})
	require.Equal(t, capability.InvokeFailed, fail.Status)
	require.Equal(t, "E001", fail.Result.ErrorCode)
}

func TestUnifiedCapabilityAutoProtocolRouting(t *testing.T) {
	sb := newMockSB()
	exec := execution.NewDriverExecutor(sb)
	mapper := execution.NewCapabilityMapper(exec)

	// Unified read_points: no protocol in Metadata, no protocol in args.
	// DriverExecutor should auto-resolve protocol from channel.
	unifiedReadCap := capability.Capability{
		ID:       "read_points",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"driver_command": "ReadPoints", "unified": true},
	}
	out, err := mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: unifiedReadCap.ID,
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
		},
	}, unifiedReadCap)
	require.NoError(t, err)
	m := out.(map[string]any)
	require.Equal(t, "dev1", m["device_id"])
	require.Equal(t, "modbus-tcp", m["protocol"], "protocol should be auto-resolved from channel")
	require.Len(t, sb.reads, 1)

	// Unified write_points: protocol auto-resolved
	unifiedWriteCap := capability.Capability{
		ID:       "write_points",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"driver_command": "WritePoint", "unified": true},
	}
	out, err = mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: unifiedWriteCap.ID,
		Arguments: map[string]any{
			"device_id": "dev1",
			"address":   "p1",
			"value":     99,
		},
	}, unifiedWriteCap)
	require.NoError(t, err)
	wm := out.(map[string]any)
	require.Equal(t, true, wm["success"])
	require.Equal(t, "modbus-tcp", wm["protocol"], "protocol should be auto-resolved from channel")

	// Unified get_diagnostics
	unifiedDiagCap := capability.Capability{
		ID:       "get_diagnostics",
		Category: capability.CategorySystem,
		Metadata: map[string]any{"driver_command": "Diagnostics", "unified": true},
	}
	out, err = mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: unifiedDiagCap.ID,
		Arguments:  map[string]any{"device_id": "dev1"},
	}, unifiedDiagCap)
	require.NoError(t, err)
	require.NotNil(t, out)

	// Unified list_points: protocol auto-resolved
	unifiedListCap := capability.Capability{
		ID:       "list_points",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"driver_command": "GetDevicePoints", "unified": true},
	}
	out, err = mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: unifiedListCap.ID,
		Arguments:  map[string]any{"device_id": "dev1"},
	}, unifiedListCap)
	require.NoError(t, err)
	lm := out.(map[string]any)
	require.Equal(t, "modbus-tcp", lm["protocol"], "protocol should be auto-resolved from channel")
}

// TestUnifiedBatchWrite verifies the write_points batch mode (writes[] array).
func TestUnifiedBatchWrite(t *testing.T) {
	sb := newMockSB()
	exec := execution.NewDriverExecutor(sb)
	mapper := execution.NewCapabilityMapper(exec)

	unifiedWriteCap := capability.Capability{
		ID:       "write_points",
		Category: capability.CategoryDevice,
		Metadata: map[string]any{"driver_command": "WritePoint", "unified": true},
	}
	out, err := mapper.MapAndExecute(context.Background(), capability.InvokeRequest{
		Capability: unifiedWriteCap.ID,
		Arguments: map[string]any{
			"device_id": "dev1",
			"writes": []any{
				map[string]any{"address": "p1", "value": 10},
				map[string]any{"address": "p2", "value": 20},
				map[string]any{"address": "p3", "value": 30},
			},
		},
	}, unifiedWriteCap)
	require.NoError(t, err)
	wm := out.(map[string]any)
	require.Equal(t, true, wm["success"])
	require.Equal(t, "modbus-tcp", wm["protocol"], "protocol should be auto-resolved from channel")
	require.Equal(t, 3, wm["count"])
	require.Equal(t, 3, wm["success_count"])
	results := wm["results"].([]map[string]any)
	require.Len(t, results, 3)
	require.Equal(t, "p1", results[0]["address"])
	require.Equal(t, 10, results[0]["value"])
	// Verify all 3 writes hit the southbound manager
	require.Len(t, sb.writes, 3)
}

// TestAddressResolutionAllForms verifies that read_points and write_points
// accept three forms of point identification: point_id, address, and name.
// This bridges the semantic gap between list_points (returns PDU offsets as
// "address") and read/write (which need internal point_id).
//
// 验证 read_points/write_points 接受三种点位标识形式：point_id、address、name。
// 解决 list_points 返回 PDU 偏移而 read/write 需要 point_id 的语义不一致问题。
func TestAddressResolutionAllForms(t *testing.T) {
	// Enhanced mock with multiple points where ID, Address, and Name are all distinct.
	// 使用 ID、Address、Name 各不相同的多个点位构建增强 mock。
	sb := &mockSB{
		channels: map[string]model.Channel{
			"ch_test": {
				ID:       "ch_test",
				Name:     "TestChannel",
				Protocol: "modbus-tcp",
				Enable:   true,
				Devices: []model.Device{{
					ID:     "dev_test",
					Name:   "TestDevice",
					Enable: true,
					Points: []model.Point{
						{ID: "pt_001", Name: "temperature", Address: "0", DataType: "int16", ReadWrite: "R"},
						{ID: "pt_002", Name: "humidity", Address: "2", DataType: "int16", ReadWrite: "R"},
						{ID: "pt_003", Name: "pressure", Address: "40001", DataType: "float32", ReadWrite: "RW"},
					},
				}},
			},
		},
		shadow: map[string]model.ShadowPoint{
			"ch_test/dev_test/pt_001": {Value: 25, Quality: "Good", Timestamp: time.Now()},
			"ch_test/dev_test/pt_002": {Value: 60, Quality: "Good", Timestamp: time.Now()},
			"ch_test/dev_test/pt_003": {Value: 1.013, Quality: "Good", Timestamp: time.Now()},
		},
	}

	exec := execution.NewDriverExecutor(sb)

	// ── Read by point_id (preferred form) ──
	out, err := exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":     "dev_test",
			"point_id":      "pt_001",
			"prefer_shadow": true,
		},
	})
	require.NoError(t, err)
	m := out.(map[string]any)
	vals := m["values"].([]map[string]any)
	require.Len(t, vals, 1)
	require.Equal(t, "pt_001", vals[0]["point_id"], "point_id should be resolved internal ID")
	require.Equal(t, "pt_001", vals[0]["address"], "address field should preserve original input")
	require.Equal(t, 25, vals[0]["value"])

	// ── Read by address (PDU offset "0" → pt_001) ──
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":     "dev_test",
			"address":       "0", // PDU offset from list_points
			"prefer_shadow": true,
		},
	})
	require.NoError(t, err)
	m = out.(map[string]any)
	vals = m["values"].([]map[string]any)
	require.Len(t, vals, 1)
	require.Equal(t, "pt_001", vals[0]["point_id"], "address '0' should resolve to pt_001")
	require.Equal(t, "0", vals[0]["address"], "address field should preserve original input '0'")

	// ── Read by name (case-insensitive "temperature" → pt_001) ──
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":     "dev_test",
			"address":       "TEMPERATURE", // case-insensitive name
			"prefer_shadow": true,
		},
	})
	require.NoError(t, err)
	m = out.(map[string]any)
	vals = m["values"].([]map[string]any)
	require.Len(t, vals, 1)
	require.Equal(t, "pt_001", vals[0]["point_id"], "name 'TEMPERATURE' should resolve to pt_001")

	// ── Read by PLC address ("40001" → pt_003) ──
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":     "dev_test",
			"address":       "40001",
			"prefer_shadow": true,
		},
	})
	require.NoError(t, err)
	m = out.(map[string]any)
	vals = m["values"].([]map[string]any)
	require.Len(t, vals, 1)
	require.Equal(t, "pt_003", vals[0]["point_id"], "PLC address '40001' should resolve to pt_003")

	// ── Batch read with mixed forms (point_id + address + name) ──
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "ReadPoints",
		Args: map[string]any{
			"device_id":     "dev_test",
			"addresses":     []string{"pt_001", "2", "pressure"},
			"prefer_shadow": true,
		},
	})
	require.NoError(t, err)
	m = out.(map[string]any)
	vals = m["values"].([]map[string]any)
	require.Len(t, vals, 3, "should resolve 3 points from mixed forms")
	// Verify each resolved to the correct internal point_id
	resolvedIDs := map[string]bool{}
	for _, v := range vals {
		resolvedIDs[v["point_id"].(string)] = true
	}
	require.True(t, resolvedIDs["pt_001"], "pt_001 should be resolved from 'pt_001'")
	require.True(t, resolvedIDs["pt_002"], "pt_002 should be resolved from address '2'")
	require.True(t, resolvedIDs["pt_003"], "pt_003 should be resolved from name 'pressure'")

	// ── Write by point_id ──
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "WritePoint",
		Args: map[string]any{
			"device_id": "dev_test",
			"point_id":  "pt_003",
			"value":     1.5,
		},
	})
	require.NoError(t, err)
	wm := out.(map[string]any)
	require.Equal(t, true, wm["success"])
	require.Equal(t, "pt_003", wm["point_id"])
	require.Len(t, sb.writes, 1)
	require.Equal(t, "pt_003", sb.writes[0].PointID)

	// ── Write by address (PLC address "40001" → pt_003) ──
	sb.writes = nil
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "WritePoint",
		Args: map[string]any{
			"device_id": "dev_test",
			"address":   "40001",
			"value":     2.0,
		},
	})
	require.NoError(t, err)
	wm = out.(map[string]any)
	require.Equal(t, true, wm["success"])
	require.Equal(t, "pt_003", wm["point_id"], "address '40001' should resolve to pt_003")
	require.Equal(t, "40001", wm["address"], "response should preserve original input")
	require.Len(t, sb.writes, 1)
	require.Equal(t, "pt_003", sb.writes[0].PointID, "write should use resolved point_id")

	// ── Batch write with point_id and address mixed ──
	sb.writes = nil
	out, err = exec.Execute(context.Background(), execution.DriverCommand{
		Command: "WritePoint",
		Args: map[string]any{
			"device_id": "dev_test",
			"writes": []any{
				map[string]any{"point_id": "pt_003", "value": 3.0},
				map[string]any{"address": "40001", "value": 4.0},
			},
		},
	})
	require.NoError(t, err)
	wm = out.(map[string]any)
	require.Equal(t, true, wm["success"])
	require.Equal(t, 2, wm["success_count"])
	require.Len(t, sb.writes, 2)
	// Both writes should target the same point (pt_003)
	for _, w := range sb.writes {
		require.Equal(t, "pt_003", w.PointID, "all batch writes should resolve to pt_003")
	}
}
