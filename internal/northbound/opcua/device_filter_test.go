package opcua

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestServer_DeviceFiltering(t *testing.T) {
	// Setup Mock Data
	mockSB := &MockSouthboundManager{
		channels: []model.Channel{
			{
				ID:       "ch1",
				Name:     "Test Channel",
				Protocol: "modbus",
				Devices: []model.Device{
					{
						ID:   "dev1",
						Name: "Device 1",
						Points: []model.Point{
							{ID: "p1", Name: "Point 1", DataType: "int16", ReadWrite: "R"},
						},
					},
					{
						ID:   "dev2",
						Name: "Device 2",
						Points: []model.Point{
							{ID: "p1", Name: "Point 1", DataType: "int16", ReadWrite: "R"},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		configDevices model.OpcUaDeviceMap
		expectDev1    bool
		expectDev2    bool
	}{
		{
			name:          "Empty Config (Allow All)",
			configDevices: nil,
			expectDev1:    true,
			expectDev2:    true,
		},
		{
			name:          "Empty Map (Allow All)",
			configDevices: model.OpcUaDeviceMap{},
			expectDev1:    true,
			expectDev2:    true,
		},
		{
			name:          "Dev1 Enabled Only",
			configDevices: model.OpcUaDeviceMap{"dev1": {Enable: true}},
			expectDev1:    true,
			expectDev2:    true,
		},
		{
			name:          "Dev2 Enabled Only",
			configDevices: model.OpcUaDeviceMap{"dev2": {Enable: true}},
			expectDev1:    true,
			expectDev2:    true,
		},
		{
			name:          "Dev1 Disabled explicitly",
			configDevices: model.OpcUaDeviceMap{"dev1": {Enable: false}, "dev2": {Enable: true}},
			expectDev1:    false,
			expectDev2:    true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := model.OPCUAConfig{
				Enable:   true,
				ID:       "test-server",
				Name:     "Test Server",
				Port:     55000 + i, // Use different ports
				Endpoint: "/test",
				Devices:  tt.configDevices,
			}

			srv := NewServer(cfg, mockSB, nil)
			if err := srv.Start(); err != nil {
				t.Fatalf("Failed to start server: %v", err)
			}
			defer srv.Stop()

			// Allow some time for async operations if any (though Start calls buildAddressSpace synchronously)
			// But check keys immediately

			dev1Key := "ch1/dev1/p1"
			dev2Key := "ch1/dev2/p1"

			// Check Dev1
			_, hasDev1 := srv.nodeMap[dev1Key]
			if hasDev1 != tt.expectDev1 {
				t.Errorf("Expect Dev1: %v, got: %v", tt.expectDev1, hasDev1)
			}

			// Check Dev2
			_, hasDev2 := srv.nodeMap[dev2Key]
			if hasDev2 != tt.expectDev2 {
				t.Errorf("Expect Dev2: %v, got: %v", tt.expectDev2, hasDev2)
			}
		})
	}
}
