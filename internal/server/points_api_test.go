package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// GenerateTestToken creates a valid JWT token for testing
func GenerateTestToken() string {
	claims := CustomClaims{
		Name:  "TestUser",
		Email: "test@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			Issuer:    "test",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Key must match NewJWT() in auth.go
	signedToken, _ := token.SignedString([]byte("GATEWAY"))
	return signedToken
}

// MockDriver implements driver.Driver for testing
type MockDriver struct {
	ReadPointsFunc func(ctx context.Context, points []model.Point) (map[string]model.Value, error)
}

func (m *MockDriver) Init(cfg model.DriverConfig) error { return nil }
func (m *MockDriver) Connect(ctx context.Context) error { return nil }
func (m *MockDriver) Disconnect() error                 { return nil }
func (m *MockDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if m.ReadPointsFunc != nil {
		return m.ReadPointsFunc(ctx, points)
	}
	// Default behavior: return dummy values
	results := make(map[string]model.Value)
	for _, p := range points {
		results[p.ID] = model.Value{
			PointID: p.ID,
			Value:   123.45,
			Quality: "Good",
			TS:      time.Now(),
		}
	}
	return results, nil
}
func (m *MockDriver) WritePoint(ctx context.Context, point model.Point, value any) error { return nil }
func (m *MockDriver) Health() driver.HealthStatus                                        { return driver.HealthStatusGood }
func (m *MockDriver) SetSlaveID(slaveID uint8) error                                     { return nil }
func (m *MockDriver) SetDeviceConfig(config map[string]any) error                        { return nil }
func (m *MockDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}

func TestGetDevicePoints_ProtocolFields(t *testing.T) {
	// 1. Setup ChannelManager
	pipeline := core.NewDataPipeline(100)
	cm := core.NewChannelManager(pipeline, nil)

	// 2. Setup Modbus Channel & Device
	modbusCh := &model.Channel{
		ID:       "ch-modbus",
		Name:     "Modbus Channel",
		Protocol: "modbus-tcp",
		Enable:   true,
		Devices: []model.Device{
			{
				ID:     "dev-modbus",
				Name:   "Modbus Device",
				Enable: true,
				Config: map[string]any{
					"slave_id": 1,
				},
				Points: []model.Point{
					{
						ID:           "p1",
						Name:         "Modbus Point",
						RegisterType: model.RegHolding,
						FunctionCode: 3,
						Address:      "40001",
						DataType:     "uint16",
						// SlaveID:      1, // Removed as it's not in Point struct
					},
				},
			},
		},
	}

	// 3. Setup BACnet Channel & Device
	bacnetCh := &model.Channel{
		ID:       "ch-bacnet",
		Name:     "BACnet Channel",
		Protocol: "bacnet-ip",
		Enable:   true,
		Devices: []model.Device{
			{
				ID:     "dev-bacnet",
				Name:   "BACnet Device",
				Enable: true,
				Points: []model.Point{
					{
						ID:       "p2",
						Name:     "BACnet Point",
						Address:  "AnalogValue:1",
						DataType: "float32",
					},
				},
			},
		},
	}

	// 4. Register Drivers
	// We need to register mock driver so AddChannel can find it
	driver.RegisterDriver("modbus-tcp", func() driver.Driver {
		return &MockDriver{}
	})
	driver.RegisterDriver("bacnet-ip", func() driver.Driver {
		return &MockDriver{}
	})

	// 5. Add Channels
	// Add Modbus Channel
	// Note: AddChannel calls driver.Init. MockDriver.Init returns nil.
	err := cm.AddChannel(modbusCh)
	assert.NoError(t, err)

	// Add BACnet Channel
	err = cm.AddChannel(bacnetCh)
	assert.NoError(t, err)

	// 6. Setup Server
	// We need to pass nil for other dependencies as they are not used in getDevicePoints
	srv := NewServer(cm, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	// 7. Test Modbus Response
	token := GenerateTestToken()
	req := httptest.NewRequest("GET", "/api/channels/ch-modbus/devices/dev-modbus/points", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var modbusPoints []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&modbusPoints)
	assert.NoError(t, err)
	assert.NotEmpty(t, modbusPoints)

	mp := modbusPoints[0]
	assert.Equal(t, "p1", mp["id"])
	assert.Equal(t, "modbus-tcp", mp["protocol"])
	// Assert Modbus fields exist
	assert.Contains(t, mp, "slave_id")
	assert.Contains(t, mp, "register_type")
	assert.Contains(t, mp, "function_code")
	// Check values
	assert.Equal(t, float64(1), mp["slave_id"]) // JSON numbers are float64

	// 8. Test BACnet Response
	req = httptest.NewRequest("GET", "/api/channels/ch-bacnet/devices/dev-bacnet/points", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = srv.app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var bacnetPoints []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&bacnetPoints)
	assert.NoError(t, err)
	assert.NotEmpty(t, bacnetPoints)

	bp := bacnetPoints[0]
	assert.Equal(t, "p2", bp["id"])
	assert.Equal(t, "bacnet-ip", bp["protocol"])
	// Assert Modbus fields DO NOT exist
	assert.NotContains(t, bp, "slave_id")
	assert.NotContains(t, bp, "register_type")
	assert.NotContains(t, bp, "function_code")
}
