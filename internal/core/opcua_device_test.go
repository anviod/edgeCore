package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOpcUaDriver implements driver.Driver, Scanner, and ObjectScanner for testing
type mockOpcUaDriver struct {
	mu             sync.Mutex
	connected      bool
	config         map[string]any
	scanResults    []map[string]any
	scanObjectsErr error
	readPointsFn   func(ctx context.Context, points []model.Point) (map[string]model.Value, error)
}

func newMockOpcUaDriver() *mockOpcUaDriver {
	return &mockOpcUaDriver{
		scanResults: []map[string]any{
			{
				"device_id":   "opcua-default",
				"endpoint":    "opc.tcp://localhost:4840",
				"name":        "Local OPC UA Server",
				"description": "Default OPC UA Server on localhost",
				"vendor_name": "TestVendor",
				"model_name":  "TestModel",
				"version":     "1.0.0",
			},
			{
				"device_id":   "opcua-simulation",
				"endpoint":    "opc.tcp://localhost:5050/test",
				"name":        "Simulation OPC UA Server",
				"description": "Simulation OPC UA Server",
				"vendor_name": "SimVendor",
				"model_name":  "SimModel",
				"version":     "2.0.0",
			},
		},
	}
}

func (m *mockOpcUaDriver) Init(cfg model.DriverConfig) error { return nil }
func (m *mockOpcUaDriver) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}
func (m *mockOpcUaDriver) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}
func (m *mockOpcUaDriver) Health() drv.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connected {
		return drv.HealthStatusGood
	}
	return drv.HealthStatusUnknown
}
func (m *mockOpcUaDriver) SetSlaveID(slaveID uint8) error { return nil }
func (m *mockOpcUaDriver) SetDeviceConfig(config map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}
func (m *mockOpcUaDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (m *mockOpcUaDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readPointsFn != nil {
		return m.readPointsFn(ctx, points)
	}
	result := make(map[string]model.Value)
	for _, p := range points {
		result[p.ID] = model.Value{
			ChannelID: "test-ch",
			DeviceID:  p.DeviceID,
			PointID:   p.ID,
			Value:     42.0,
			Quality:   "Good",
			TS:        time.Now(),
		}
	}
	return result, nil
}
func (m *mockOpcUaDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	return nil
}
func (m *mockOpcUaDriver) Scan(ctx context.Context, params map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if endpoint, ok := params["endpoint"].(string); ok && endpoint != "" {
		return []map[string]any{
			{
				"device_id":   endpoint,
				"endpoint":    endpoint,
				"name":        "OPC UA Server at " + endpoint,
				"description": "Connected server",
				"vendor_name": "ConnectedVendor",
				"model_name":  "ConnectedModel",
				"version":     "3.0.0",
			},
		}, nil
	}
	return m.scanResults, nil
}
func (m *mockOpcUaDriver) ScanObjects(ctx context.Context, config map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scanObjectsErr != nil {
		return nil, m.scanObjectsErr
	}
	_, _ = config["endpoint"].(string)
	return []map[string]any{
		{
			"node_id":  "ns=2;s=Temperature",
			"name":     "Temperature",
			"class":    "Variable",
			"type":     "Variable",
			"address":  "ns=2;s=Temperature",
			"datatype": "Double",
		},
		{
			"node_id":  "ns=2;s=Pressure",
			"name":     "Pressure",
			"class":    "Variable",
			"type":     "Variable",
			"address":  "ns=2;s=Pressure",
			"datatype": "Float",
		},
		{
			"node_id":  "ns=2;s=Status",
			"name":     "Status",
			"class":    "Variable",
			"type":     "Variable",
			"address":  "ns=2;s=Status",
			"datatype": "Boolean",
		},
	}, nil
}

// createTestChannelManager creates a ChannelManager with in-memory save capture.
func createTestChannelManager(t *testing.T) (*ChannelManager, string, func()) {
	t.Helper()
	tempDir := testOutputDir(t)
	cm := NewChannelManager(nil, func(channels []model.Channel) error { return nil })
	return cm, tempDir, func() {
		cm.cancel()
	}
}

// ============================================================
// Test 1: Add OPC-UA devices and verify ID/Name preservation
// ============================================================
func TestOpcUa_AddDevice_IDNamePreserved(t *testing.T) {
	cm, tempDir, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = newMockOpcUaDriver()
	cm.driverMus[channelID] = &sync.Mutex{}

	tests := []struct {
		name     string
		device   model.Device
		wantID   string
		wantName string
	}{
		{
			name: "Device with explicit ID and Name",
			device: model.Device{
				ID:       "opcua-device-001",
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
			},
			wantID:   "opcua-device-001",
			wantName: "Temperature Sensor",
		},
		{
			name: "Device with ID only, Name fallback to ID",
			device: model.Device{
				ID:       "opcua-device-002",
				Name:     "",
				Interval: model.Duration(5 * time.Second),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
			},
			wantID:   "opcua-device-002",
			wantName: "",
		},
		{
			name: "Device with endpoint-based config",
			device: model.Device{
				ID:       "opcua-simulation",
				Name:     "Simulation Server",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config: map[string]any{
					"endpoint":        "opc.tcp://localhost:5050/test",
					"security_policy": "None",
					"security_mode":   "None",
					"auth_method":     "Anonymous",
				},
			},
			wantID:   "opcua-simulation",
			wantName: "Simulation Server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.AddDevice(channelID, &tt.device)
			require.NoError(t, err)

			devices := cm.GetChannelDevices(channelID)
			var found *model.Device
			for i := range devices {
				if devices[i].ID == tt.wantID {
					found = &devices[i]
					break
				}
			}
			require.NotNil(t, found, "Device %s should exist in channel", tt.wantID)
			assert.Equal(t, tt.wantID, found.ID, "Device ID should be preserved")
			assert.Equal(t, tt.wantName, found.Name, "Device Name should be preserved")
		})
	}

	_ = tempDir
}

// ============================================================
// Test 2: Add device and verify save callback receives full config
// ============================================================
func TestOpcUa_AddDevice_PersistedViaSaveFunc(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{})
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	channelID := "test-opcua-ch"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = newMockOpcUaDriver()
	cm.driverMus[channelID] = &sync.Mutex{}

	deviceID := "temp-sensor-01"
	dev := &model.Device{
		ID:       deviceID,
		Name:     "Temperature Sensor 01",
		Interval: model.Duration(5 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"endpoint":        "opc.tcp://192.168.1.100:4840",
			"security_policy": "None",
			"security_mode":   "None",
			"auth_method":     "Anonymous",
		},
		Points: []model.Point{
			{
				ID:       "temp",
				Name:     "Temperature",
				Address:  "ns=2;s=Temperature",
				DataType: "float64",
				Unit:     "°C",
			},
		},
		Storage: model.DeviceStorage{
			Enable:     true,
			Strategy:   "interval",
			Interval:   5,
			MaxRecords: 1000,
		},
	}

	err := cm.AddDevice(channelID, dev)
	require.NoError(t, err)

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 1)
	assert.Equal(t, deviceID, devices[0].ID)
	assert.Equal(t, "Temperature Sensor 01", devices[0].Name)
	assert.Equal(t, "opc.tcp://192.168.1.100:4840", devices[0].Config["endpoint"])
	require.Len(t, devices[0].Points, 1)
	assert.Equal(t, "temp", devices[0].Points[0].ID)
	assert.True(t, devices[0].Storage.Enable)
	assert.Equal(t, "interval", devices[0].Storage.Strategy)

	<-saveCh // wait for async save
	require.Len(t, saved, 1)
	require.Len(t, saved[0].Devices, 1)
	assert.Equal(t, deviceID, saved[0].Devices[0].ID)
	require.Len(t, saved[0].Devices[0].Points, 1)
}

// ============================================================
// Test 3: Scan channel and verify scan results
// ============================================================
func TestOpcUa_ScanChannel_DefaultEndpoints(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	results, err := cm.ScanChannel(context.Background(), channelID, nil)
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok, "Scan results should be a list")
	require.Len(t, resultList, 2, "Should return 2 default endpoints")

	assert.Equal(t, "opcua-default", resultList[0]["device_id"])
	assert.Equal(t, "opc.tcp://localhost:4840", resultList[0]["endpoint"])
	assert.Equal(t, "Local OPC UA Server", resultList[0]["name"])

	assert.Equal(t, "opcua-simulation", resultList[1]["device_id"])
	assert.Equal(t, "opc.tcp://localhost:5050/test", resultList[1]["endpoint"])
	assert.Equal(t, "Simulation OPC UA Server", resultList[1]["name"])
}

func TestOpcUa_ScanChannel_WithEndpoint(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	endpoint := "opc.tcp://192.168.1.200:4840"
	results, err := cm.ScanChannel(context.Background(), channelID, map[string]any{"endpoint": endpoint})
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok)
	require.Len(t, resultList, 1)
	assert.Equal(t, endpoint, resultList[0]["device_id"])
	assert.Equal(t, endpoint, resultList[0]["endpoint"])
	assert.Contains(t, resultList[0]["name"], endpoint)
}

func TestOpcUa_ScanChannel_InheritsChannelURL(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	channelURL := "opc.tcp://192.168.1.100:4840"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{"url": channelURL},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	results, err := cm.ScanChannel(context.Background(), channelID, nil)
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok)
	require.Len(t, resultList, 1)
	assert.Equal(t, channelURL, resultList[0]["endpoint"])
}

func TestOpcUa_AddDevice_InheritsChannelEndpoint(t *testing.T) {
	var saved []model.Channel
	saveCh := make(chan struct{})
	cm := NewChannelManager(nil, func(channels []model.Channel) error {
		saved = channels
		saveCh <- struct{}{}
		return nil
	})
	defer cm.cancel()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	channelURL := "opc.tcp://channel-server:4840"
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{"url": channelURL},
		Devices:  []model.Device{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	dev := &model.Device{
		ID:       "opcua-device-1",
		Name:     "OPC UA Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config:   map[string]any{},
	}
	err := cm.AddDevice(channelID, dev)
	require.NoError(t, err)
	<-saveCh // wait for async save
	require.Len(t, saved, 1)
	require.Len(t, saved[0].Devices, 1)
	assert.Equal(t, channelURL, saved[0].Devices[0].Config["endpoint"])
}

func TestOpcUa_ScanDevice_InheritsChannelEndpoint(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()
	channelURL := "opc.tcp://localhost:4840"

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{"url": channelURL},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config:   map[string]any{},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	results, err := cm.ScanDevice(context.Background(), channelID, deviceID, nil)
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok)
	require.Len(t, resultList, 3)
}

func TestOpcUa_ScanChannel_NoDriver(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	_, err := cm.ScanChannel(context.Background(), "nonexistent-channel", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel driver not found")
}

// ============================================================
// Test 4: Scan device objects (points)
// ============================================================
func TestOpcUa_ScanDevice_Points(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config: map[string]any{
					"endpoint": "opc.tcp://localhost:4840",
				},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	results, err := cm.ScanDevice(context.Background(), channelID, deviceID, nil)
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok)
	require.Len(t, resultList, 3)

	assert.Equal(t, "Temperature", resultList[0]["name"])
	assert.Equal(t, "Variable", resultList[0]["type"])
	assert.Equal(t, "ns=2;s=Temperature", resultList[0]["address"])

	assert.Equal(t, "Pressure", resultList[1]["name"])
	assert.Equal(t, "Float", resultList[1]["datatype"])

	assert.Equal(t, "Status", resultList[2]["name"])
	assert.Equal(t, "Boolean", resultList[2]["datatype"])
}

func TestOpcUa_ScanDevice_DeviceNotFound(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	_, err := cm.ScanDevice(context.Background(), channelID, "nonexistent-device", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device not found")
}

func TestOpcUa_ScanDevice_ConfigMerged(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   true,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config: map[string]any{
					"endpoint":        "opc.tcp://192.168.1.100:4840",
					"security_policy": "Basic256Sha256",
					"security_mode":   "SignAndEncrypt",
					"auth_method":     "UserName",
					"username":        "admin",
					"password":        "secret",
				},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	results, err := cm.ScanDevice(context.Background(), channelID, deviceID, map[string]any{"root_node_id": "ns=2;s=CustomRoot"})
	require.NoError(t, err)

	resultList, ok := results.([]map[string]any)
	require.True(t, ok)
	require.Len(t, resultList, 3)
}

// ============================================================
// Test 5: Add device from scan results (simulating frontend flow)
// ============================================================
func TestOpcUa_AddDeviceFromScanResults(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	scanResults := []map[string]any{
		{
			"device_id":   "opcua-default",
			"endpoint":    "opc.tcp://localhost:4840",
			"name":        "Local OPC UA Server",
			"vendor_name": "TestVendor",
			"model_name":  "TestModel",
			"version":     "1.0.0",
		},
		{
			"device_id":   "opcua-simulation",
			"endpoint":    "opc.tcp://localhost:5050/test",
			"name":        "Simulation OPC UA Server",
			"vendor_name": "SimVendor",
			"model_name":  "SimModel",
			"version":     "2.0.0",
		},
	}

	for _, scanItem := range scanResults {
		endpoint, _ := scanItem["endpoint"].(string)
		deviceName, _ := scanItem["name"].(string)
		if deviceName == "" {
			deviceName = "OPC UA Server"
		}

		dev := &model.Device{
			ID:       endpoint, // simulate legacy frontend bug: endpoint used as ID
			Name:     deviceName,
			Interval: model.Duration(10 * time.Second),
			Enable:   true,
			Config: map[string]any{
				"endpoint":    endpoint,
				"name":        scanItem["name"],
				"vendor_name": scanItem["vendor_name"],
				"model_name":  scanItem["model_name"],
				"version":     scanItem["version"],
			},
			Points: []model.Point{},
		}

		err := cm.AddDevice(channelID, dev)
		require.NoError(t, err, "Failed to add device for endpoint %s", endpoint)
	}

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 2)

	for _, dev := range devices {
		assert.False(t, model.IsEndpointLikeDeviceID(dev.ID), "device ID must not be endpoint URL")
		assert.NotEmpty(t, dev.Config["endpoint"])
	}

	assert.Equal(t, "opc.tcp://localhost:4840", devices[0].Config["endpoint"])
	assert.Equal(t, "Local OPC UA Server", devices[0].Name)
	assert.Equal(t, "TestVendor", devices[0].Config["vendor_name"])

	assert.Equal(t, "opc.tcp://localhost:5050/test", devices[1].Config["endpoint"])
	assert.Equal(t, "Simulation OPC UA Server", devices[1].Name)
	assert.Equal(t, "SimVendor", devices[1].Config["vendor_name"])
}

// ============================================================
// Test 6: Add points to OPC-UA device
// ============================================================
func TestOpcUa_AddPointsToDevice(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config: map[string]any{
					"endpoint": "opc.tcp://localhost:4840",
				},
				Points: []model.Point{},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	newPoints := []model.Point{
		{
			ID:        "temperature",
			Name:      "Temperature",
			Address:   "ns=2;s=Temperature",
			DataType:  "float64",
			Unit:      "°C",
			ReadWrite: "R",
		},
		{
			ID:        "pressure",
			Name:      "Pressure",
			Address:   "ns=2;s=Pressure",
			DataType:  "float32",
			Unit:      "Pa",
			ReadWrite: "R",
		},
		{
			ID:        "status",
			Name:      "Status",
			Address:   "ns=2;s=Status",
			DataType:  "bool",
			ReadWrite: "R",
		},
	}

	for _, p := range newPoints {
		pCopy := p
		pCopy.DeviceID = deviceID
		err := cm.AddPoint(channelID, deviceID, &pCopy)
		require.NoError(t, err, "Failed to add point %s", p.ID)
	}

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 1)
	require.Len(t, devices[0].Points, 3)

	assert.Equal(t, "temperature", devices[0].Points[0].ID)
	assert.Equal(t, "Temperature", devices[0].Points[0].Name)
	assert.Equal(t, "ns=2;s=Temperature", devices[0].Points[0].Address)
	assert.Equal(t, "float64", devices[0].Points[0].DataType)

	assert.Equal(t, "pressure", devices[0].Points[1].ID)
	assert.Equal(t, "ns=2;s=Pressure", devices[0].Points[1].Address)

	assert.Equal(t, "status", devices[0].Points[2].ID)
	assert.Equal(t, "bool", devices[0].Points[2].DataType)
}

// ============================================================
// Test 7: Add device with empty ID (OPC UA auto-generates short ID)
// ============================================================
func TestOpcUa_AddDevice_EmptyID(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	dev := &model.Device{
		ID:       "",
		Name:     "fallback-name-device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
	}

	err := cm.AddDevice(channelID, dev)
	require.NoError(t, err)

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 1)
	assert.False(t, model.IsEndpointLikeDeviceID(devices[0].ID), "OPC UA empty ID should become a short generated ID")
	assert.Len(t, devices[0].ID, 16)
	assert.Equal(t, "fallback-name-device", devices[0].Name, "Name should be preserved")
}

// ============================================================
// Test 8: Add duplicate device (should fail)
// ============================================================
func TestOpcUa_AddDevice_Duplicate(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	dev1 := &model.Device{
		ID:       "dup-device",
		Name:     "First Device",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
	}
	err := cm.AddDevice(channelID, dev1)
	require.NoError(t, err)

	dev2 := &model.Device{
		ID:       "dup-device",
		Name:     "Second Device",
		Interval: model.Duration(5 * time.Second),
		Enable:   true,
		Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
	}
	err = cm.AddDevice(channelID, dev2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// ============================================================
// Test 9: Update device and verify persistence
// ============================================================
func TestOpcUa_UpdateDevice(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config: map[string]any{
					"endpoint": "opc.tcp://localhost:4840",
				},
				Points: []model.Point{},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	updatedDev := &model.Device{
		ID:       deviceID,
		Name:     "Temperature Sensor (Updated)",
		Interval: model.Duration(5 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"endpoint":        "opc.tcp://192.168.1.200:4840",
			"security_policy": "Basic256",
		},
		Points: []model.Point{
			{
				ID:       "temp",
				Name:     "Temperature",
				Address:  "ns=2;s=Temperature",
				DataType: "float64",
			},
		},
	}

	err := cm.UpdateDevice(channelID, updatedDev)
	require.NoError(t, err)

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 1)
	assert.Equal(t, deviceID, devices[0].ID)
	assert.Equal(t, "Temperature Sensor (Updated)", devices[0].Name)
	assert.Equal(t, "opc.tcp://192.168.1.200:4840", devices[0].Config["endpoint"])
	assert.Equal(t, "Basic256", devices[0].Config["security_policy"])
	// UpdateDevice preserves existing points (points are managed via separate API)
	require.Len(t, devices[0].Points, 0)
}

// ============================================================
// Test 10: Remove device
// ============================================================
func TestOpcUa_RemoveDevice(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       "device-1",
				Name:     "Device 1",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
			},
			{
				ID:       "device-2",
				Name:     "Device 2",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:5050"},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	err := cm.RemoveDevice(channelID, "device-1")
	require.NoError(t, err)

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 1)
	assert.Equal(t, "device-2", devices[0].ID)
}

// ============================================================
// Test 11: Device interval validation (non-positive interval)
// ============================================================
func TestOpcUa_DeviceIntervalValidation(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"Zero interval", 0},
		{"Negative interval", -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &model.Device{
				ID:       fmt.Sprintf("dev-%s", strings.ReplaceAll(tt.name, " ", "-")),
				Name:     tt.name,
				Interval: model.Duration(tt.interval),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
			}

			err := cm.AddDevice(channelID, dev)
			require.NoError(t, err, "AddDevice should not fail even with invalid interval")

			devices := cm.GetChannelDevices(channelID)
			require.NotEmpty(t, devices)
		})
	}
}

// ============================================================
// Test 13: JSON serialization round-trip (simulating API flow)
// ============================================================
func TestOpcUa_DeviceJSONRoundTrip(t *testing.T) {
	original := model.Device{
		ID:       "json-test-device",
		Name:     "JSON Round Trip Test",
		Interval: model.Duration(10 * time.Second),
		Enable:   true,
		Config: map[string]any{
			"endpoint":        "opc.tcp://192.168.1.100:4840",
			"security_policy": "None",
			"security_mode":   "None",
			"auth_method":     "Anonymous",
		},
		Points: []model.Point{
			{
				ID:        "temp",
				Name:      "Temperature",
				Address:   "ns=2;s=Temperature",
				DataType:  "float64",
				Unit:      "°C",
				ReadWrite: "R",
			},
		},
	}

	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	var loaded model.Device
	err = json.Unmarshal(jsonBytes, &loaded)
	require.NoError(t, err)

	assert.Equal(t, original.ID, loaded.ID, "ID should survive JSON round-trip")
	assert.Equal(t, original.Name, loaded.Name, "Name should survive JSON round-trip")
	assert.Equal(t, original.Interval, loaded.Interval)
	assert.Equal(t, original.Config["endpoint"], loaded.Config["endpoint"])
	require.Len(t, loaded.Points, 1)
	assert.Equal(t, "temp", loaded.Points[0].ID)
	assert.Equal(t, "ns=2;s=Temperature", loaded.Points[0].Address)
}

// ============================================================
// Test 14: Batch add devices
// ============================================================
func TestOpcUa_BatchAddDevices(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	mockDriver := newMockOpcUaDriver()
	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	devicesToAdd := []model.Device{
		{
			ID:       "batch-device-1",
			Name:     "Batch Device 1",
			Interval: model.Duration(10 * time.Second),
			Enable:   true,
			Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
		},
		{
			ID:       "batch-device-2",
			Name:     "Batch Device 2",
			Interval: model.Duration(5 * time.Second),
			Enable:   true,
			Config:   map[string]any{"endpoint": "opc.tcp://localhost:5050"},
		},
		{
			ID:       "batch-device-3",
			Name:     "Batch Device 3",
			Interval: model.Duration(15 * time.Second),
			Enable:   false,
			Config:   map[string]any{"endpoint": "opc.tcp://192.168.1.100:4840"},
		},
	}

	for i := range devicesToAdd {
		err := cm.AddDevice(channelID, &devicesToAdd[i])
		require.NoError(t, err, "Failed to add batch device %d", i)
	}

	devices := cm.GetChannelDevices(channelID)
	require.Len(t, devices, 3)

	assert.Equal(t, "batch-device-1", devices[0].ID)
	assert.Equal(t, "Batch Device 1", devices[0].Name)

	assert.Equal(t, "batch-device-2", devices[1].ID)
	assert.Equal(t, "Batch Device 2", devices[1].Name)

	assert.Equal(t, "batch-device-3", devices[2].ID)
	assert.Equal(t, "Batch Device 3", devices[2].Name)
	assert.False(t, devices[2].Enable)
}

// ============================================================
// Test 15: AddPoint validation
// ============================================================
func TestOpcUa_AddPoint_DuplicatePointID(t *testing.T) {
	cm, _, cleanup := createTestChannelManager(t)
	defer cleanup()

	channelID := "test-opcua-ch"
	deviceID := "temp-sensor-01"
	mockDriver := newMockOpcUaDriver()

	ch := &model.Channel{
		ID:       channelID,
		Name:     "OPC UA Test Channel",
		Protocol: "opc-ua",
		Enable:   false,
		Config:   map[string]any{},
		Devices: []model.Device{
			{
				ID:       deviceID,
				Name:     "Temperature Sensor",
				Interval: model.Duration(10 * time.Second),
				Enable:   true,
				Config:   map[string]any{"endpoint": "opc.tcp://localhost:4840"},
				Points: []model.Point{
					{ID: "temp", Name: "Temperature", Address: "ns=2;s=Temperature", DataType: "float64"},
				},
			},
		},
	}
	cm.channels[channelID] = ch
	cm.drivers[channelID] = mockDriver
	cm.driverMus[channelID] = &sync.Mutex{}

	duplicatePoint := &model.Point{
		ID:       "temp",
		Name:     "Temperature Duplicate",
		Address:  "ns=2;s=Temperature2",
		DataType: "float64",
	}

	err := cm.AddPoint(channelID, deviceID, duplicatePoint)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}
