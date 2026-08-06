package opcua_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/driver/opcua"
	"github.com/anviod/edgeCore/internal/model"
	nbopcua "github.com/anviod/edgeCore/internal/northbound/opcua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSouthboundManager for testing
type MockSB struct {
	Channels []model.Channel
}

func (m *MockSB) GetChannels() []model.Channel {
	return m.Channels
}

func (m *MockSB) GetChannelDevices(channelID string) []model.Device {
	for _, c := range m.Channels {
		if c.ID == channelID {
			return c.Devices
		}
	}
	return nil
}

func (m *MockSB) GetDevice(channelID, deviceID string) *model.Device {
	for _, c := range m.Channels {
		if c.ID == channelID {
			for _, d := range c.Devices {
				if d.ID == deviceID {
					return &d
				}
			}
		}
	}
	return nil
}

func (m *MockSB) GetDevicePoints(channelID, deviceID string) ([]model.PointData, error) {
	dev := m.GetDevice(channelID, deviceID)
	if dev == nil {
		return []model.PointData{}, nil
	}

	points := make([]model.PointData, 0, len(dev.Points))
	for _, p := range dev.Points {
		points = append(points, model.PointData{
			ID:        p.ID,
			Name:      p.Name,
			DataType:  p.DataType,
			ReadWrite: p.ReadWrite,
			Address:   p.Address,
		})
	}
	return points, nil
}

func (m *MockSB) WritePoint(channelID, deviceID, pointID string, value any) error {
	return nil
}

func (m *MockSB) GetShadowPoint(channelID, deviceID, pointID string) (*model.ShadowPoint, error) {
	return nil, fmt.Errorf("shadow point not available in mock")
}

func TestScanLargeNumberOfPoints(t *testing.T) {
	// 1. Setup Mock Data with > 100 points to trigger potential pagination or limits
	pointCount := 200
	points := make([]model.Point, pointCount)
	for i := 0; i < pointCount; i++ {
		points[i] = model.Point{
			ID:        fmt.Sprintf("p%d", i),
			Name:      fmt.Sprintf("Point %03d", i),
			DataType:  "float64",
			ReadWrite: "R",
		}
	}

	mockSB := &MockSB{
		Channels: []model.Channel{
			{
				ID:       "ch1",
				Name:     "Test Channel",
				Protocol: "modbus",
				Devices: []model.Device{
					{
						ID:     "dev1",
						Name:   "LargeDevice",
						Points: points,
					},
				},
			},
		},
	}

	// 2. Start Internal OPC UA Server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	tmpDir := testOutputDir(t)
	cfg := model.OPCUAConfig{
		Enable:   true,
		Name:     "LargeScanTestServer",
		Port:     port,
		Endpoint: "/test",
		CertFile: filepath.Join(tmpDir, "server.crt"),
		KeyFile:  filepath.Join(tmpDir, "server.key"),
	}

	srv := nbopcua.NewServer(cfg, mockSB, nil)
	err = srv.Start()
	require.NoError(t, err, "Failed to start OPC UA server")
	defer srv.Stop()

	// Give server some time to start
	time.Sleep(1 * time.Second)

	// 3. Configure Driver to scan the server
	endpoint := fmt.Sprintf("opc.tcp://127.0.0.1:%d/test", port)
	d := opcua.NewOpcUaDriver()

	scanner, ok := d.(driver.ObjectScanner)
	require.True(t, ok, "Driver should implement ObjectScanner")

	scanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 4. Scan
	// We scan from Root.
	// The structure should be: Objects -> Gateway -> Channels -> ch1 -> Devices -> dev1 -> [Points...]
	// We can try to scan from dev1 if we knew the NodeID, but let's scan from Objects and traverse.
	// Or we can just scan everything.

	// Scan config
	scanConfig := map[string]any{
		"endpoint": endpoint,
		// "root_node_id": "ns=0;i=85", // Default is Objects
	}

	resultsRaw, err := scanner.ScanObjects(scanCtx, scanConfig)
	require.NoError(t, err, "ScanObjects failed")

	results := resultsRaw.([]map[string]any)

	// 5. Verify results
	// We need to find "LargeDevice" folder and count its children.
	// Helper to find node by name
	var findNode func(list []map[string]any, name string) map[string]any
	findNode = func(list []map[string]any, name string) map[string]any {
		for _, node := range list {
			if node["name"] == name {
				return node
			}
		}
		return nil
	}

	// Navigate: Objects (implicit root) -> Gateway -> Channels -> ch1 -> Devices -> dev1
	// Wait, internal server structure might differ.
	// server.go: Objects -> Gateway -> Channels -> ...

	gateway := findNode(results, "Gateway")
	require.NotNil(t, gateway, "Gateway node not found")

	gatewayChildren := gateway["children"].([]map[string]any)
	channels := findNode(gatewayChildren, "Channels")
	require.NotNil(t, channels, "Channels node not found")

	channelsChildren := channels["children"].([]map[string]any)
	ch1 := findNode(channelsChildren, "ch1") // ID is used as BrowseName? or Name? server.go uses Name or ID.
	// Let's assume ID or Name. Mock uses "ch1".
	if ch1 == nil {
		ch1 = findNode(channelsChildren, "Test Channel")
	}
	require.NotNil(t, ch1, "Channel ch1 not found")

	ch1Children := ch1["children"].([]map[string]any)
	devices := findNode(ch1Children, "Devices")
	require.NotNil(t, devices, "Devices node not found")

	devicesChildren := devices["children"].([]map[string]any)
	dev1 := findNode(devicesChildren, "dev1") // ID is used as Name
	require.NotNil(t, dev1, "Device dev1 not found")

	// 6. Check Point Count
	dev1Children := dev1["children"].([]map[string]any)
	pointsFolder := findNode(dev1Children, "Points")
	require.NotNil(t, pointsFolder, "Points folder not found")

	pointsChildren := pointsFolder["children"].([]map[string]any)

	// Filter for variables (Points)
	var pointCountFound int
	for _, child := range pointsChildren {
		if child["type"] == "Variable" {
			pointCountFound++
		}
	}

	fmt.Printf("Found %d points in LargeDevice\n", pointCountFound)
	assert.Equal(t, pointCount, pointCountFound, "Should find exactly 200 points")
}
