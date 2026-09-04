package bacnet

import (
	"sync"
	"testing"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/edgeCore/internal/model"
)

// mockSouthboundManager 实现 model.SouthboundManager 用于测试
type mockSouthboundManager struct {
	channels     []model.Channel
	writeHistory []writeOperation
	mu           sync.Mutex
}

type writeOperation struct {
	channelID string
	deviceID  string
	pointID   string
	value     any
}

func newMockSouthboundManager() *mockSouthboundManager {
	return &mockSouthboundManager{
		channels: []model.Channel{
			{
				ID:       "ch1",
				Name:     "Test Channel",
				Protocol: "bacnet-ip",
				Enable:   true,
				Devices: []model.Device{
					{
						ID:     "dev1",
						Name:   "Test Device",
						Enable: true,
						Config: map[string]any{
							"vendor_name": "Test Vendor",
							"model_name":  "Test Model",
						},
						Points: []model.Point{
							{
								ID:        "temp_1",
								Name:      "Temperature",
								DataType:  "float32",
								ReadWrite: "R",
							},
							{
								ID:        "hum_1",
								Name:      "Humidity",
								DataType:  "float64",
								ReadWrite: "R",
							},
							{
								ID:        "setpoint_1",
								Name:      "Setpoint",
								DataType:  "float32",
								ReadWrite: "RW",
							},
							{
								ID:        "status_1",
								Name:      "Status",
								DataType:  "string",
								ReadWrite: "R",
							},
							{
								ID:        "enable_1",
								Name:      "Enabled",
								DataType:  "bool",
								ReadWrite: "RW",
							},
						},
					},
					{
						ID:     "dev2",
						Name:   "Second Device",
						Enable: true,
						Config: map[string]any{
							"vendor_name": "Test Vendor 2",
							"model_name":  "Test Model 2",
						},
						Points: []model.Point{
							{
								ID:        "press_1",
								Name:      "Pressure",
								DataType:  "float32",
								ReadWrite: "R",
							},
							{
								ID:        "valve_1",
								Name:      "Valve",
								DataType:  "bool",
								ReadWrite: "RW",
							},
						},
					},
				},
			},
		},
	}
}

func (m *mockSouthboundManager) GetChannels() []model.Channel {
	return m.channels
}

func (m *mockSouthboundManager) GetChannelDevices(channelID string) []model.Device {
	for _, ch := range m.channels {
		if ch.ID == channelID {
			return ch.Devices
		}
	}
	return nil
}

func (m *mockSouthboundManager) GetDevice(channelID, deviceID string) *model.Device {
	for _, ch := range m.channels {
		if ch.ID == channelID {
			for _, dev := range ch.Devices {
				if dev.ID == deviceID {
					return &dev
				}
			}
		}
	}
	return nil
}

func (m *mockSouthboundManager) WritePoint(channelID, deviceID, pointID string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeHistory = append(m.writeHistory, writeOperation{
		channelID: channelID,
		deviceID:  deviceID,
		pointID:   pointID,
		value:     value,
	})
	return nil
}

func (m *mockSouthboundManager) GetDevicePoints(channelID, deviceID string) ([]model.PointData, error) {
	for _, ch := range m.channels {
		if ch.ID == channelID {
			for _, dev := range ch.Devices {
				if dev.ID == deviceID {
					var points []model.PointData
					for _, pt := range dev.Points {
						points = append(points, model.PointData{
							ID:        pt.ID,
							Name:      pt.Name,
							DataType:  pt.DataType,
							ReadWrite: pt.ReadWrite,
							Value:     defaultValueForType(pt.DataType),
						})
					}
					return points, nil
				}
			}
		}
	}
	return nil, nil
}

func (m *mockSouthboundManager) GetShadowPoint(channelID, deviceID, pointID string) (*model.ShadowPoint, error) {
	return nil, nil
}

// =============================================================================
// 单元测试
// =============================================================================

// TestNewServer 测试 Server 创建
func TestNewServer(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:         "test-bacnet",
		Name:       "Test BACnet Server",
		Enable:     true,
		Port:       47808,
		DeviceID:   1000,
		DeviceName: "TestDevice",
		VendorID:   999,
	}
	sb := newMockSouthboundManager()
	srv := NewServer(cfg, sb)

	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.config.Name != "Test BACnet Server" {
		t.Errorf("expected name 'Test BACnet Server', got '%s'", srv.config.Name)
	}
	if len(srv.pointMap) != 0 {
		t.Errorf("expected empty pointMap, got %d entries", len(srv.pointMap))
	}
}

// TestInferBACnetObjectType 测试点位类型到 BACnet 对象类型的推断
func TestInferBACnetObjectType(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		rw       string
		expected btypes.ObjectType
	}{
		{"analog input read-only", "float32", "R", btypes.AnalogInput},
		{"analog value writable", "float32", "RW", btypes.AnalogValue},
		{"analog input float64", "float64", "R", btypes.AnalogInput},
		{"analog value float64", "float64", "RW", btypes.AnalogValue},
		{"binary input read-only", "bool", "R", btypes.BinaryInput},
		{"binary value writable", "bool", "RW", btypes.BinaryValue},
		{"boolean input", "boolean", "R", btypes.BinaryInput},
		{"multi-state input string", "string", "R", btypes.MultiStateInput},
		{"multi-state value string", "string", "RW", btypes.MultiStateValue},
		{"default analog input", "unknown", "R", btypes.AnalogInput},
		{"default analog value", "unknown", "RW", btypes.AnalogValue},
		{"int analog input", "int32", "R", btypes.AnalogInput},
		{"int analog value", "int32", "RW", btypes.AnalogValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := model.PointData{
				DataType:  tt.dataType,
				ReadWrite: tt.rw,
			}
			result := inferBACnetObjectType(pt)
			if result != tt.expected {
				t.Errorf("inferBACnetObjectType(%s, %s) = %v, want %v",
					tt.dataType, tt.rw, result, tt.expected)
			}
		})
	}
}

// TestPointKey 测试点位键生成
func TestPointKey(t *testing.T) {
	key := pointKey("ch1", "dev1", "temp_1")
	expected := "ch1/dev1/temp_1"
	if key != expected {
		t.Errorf("pointKey = %s, want %s", key, expected)
	}
}

// TestConvertToBACnetValue 测试值类型转换
func TestConvertToBACnetValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		objType  btypes.ObjectType
		expected any
	}{
		{"analog float64", float64(25.5), btypes.AnalogInput, float64(25.5)},
		{"analog float32", float32(25.5), btypes.AnalogValue, float64(25.5)},
		{"analog int", 42, btypes.AnalogInput, float64(42)},
		{"analog int32", int32(100), btypes.AnalogOutput, float64(100)},
		{"binary bool true", true, btypes.BinaryInput, true},
		{"binary bool false", false, btypes.BinaryValue, false},
		{"binary float64 zero", float64(0.0), btypes.BinaryInput, false},
		{"binary float64 non-zero", float64(1.0), btypes.BinaryValue, true},
		{"binary int zero", 0, btypes.BinaryInput, false},
		{"binary int non-zero", 1, btypes.BinaryValue, true},
		{"binary string true", "true", btypes.BinaryInput, true},
		{"binary string 1", "1", btypes.BinaryValue, true},
		{"binary string on", "on", btypes.BinaryInput, true},
		{"binary string false", "false", btypes.BinaryValue, false},
		{"multi-state uint32", uint32(3), btypes.MultiStateValue, uint32(3)},
		{"multi-state int", 5, btypes.MultiStateInput, uint32(5)},
		{"nil value", nil, btypes.AnalogInput, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToBACnetValue(tt.value, tt.objType)
			if result != tt.expected {
				t.Errorf("convertToBACnetValue(%v, %v) = %v, want %v",
					tt.value, tt.objType, result, tt.expected)
			}
		})
	}
}

// TestToFloat64 测试 float64 转换
func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected float64
	}{
		{"float64", float64(25.5), 25.5},
		{"float32", float32(25.5), 25.5},
		{"int", 42, 42.0},
		{"int32", int32(100), 100.0},
		{"int64", int64(200), 200.0},
		{"uint32", uint32(300), 300.0},
		{"bool true", true, 1.0},
		{"bool false", false, 0.0},
		{"string", "hello", 0.0},
		{"nil", nil, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64(tt.value)
			if result != tt.expected {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

// TestToBool 测试 bool 转换
func TestToBool(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"float64 zero", float64(0.0), false},
		{"float64 non-zero", float64(1.5), true},
		{"int zero", 0, false},
		{"int non-zero", 42, true},
		{"string true", "true", true},
		{"string TRUE", "TRUE", true},
		{"string 1", "1", true},
		{"string on", "on", true},
		{"string false", "false", false},
		{"string unknown", "unknown", false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toBool(tt.value)
			if result != tt.expected {
				t.Errorf("toBool(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

// TestToUint32 测试 uint32 转换
func TestToUint32(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected uint32
	}{
		{"uint32", uint32(42), 42},
		{"float64", float64(100.0), 100},
		{"int", 200, 200},
		{"int32", int32(300), 300},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"string", "hello", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toUint32(tt.value)
			if result != tt.expected {
				t.Errorf("toUint32(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

// TestRequiresServerRestart 测试重启判断
func TestRequiresServerRestart(t *testing.T) {
	base := model.BACnetServerConfig{
		ID:         "test",
		Name:       "Test",
		Port:       47808,
		DeviceID:   1000,
		VendorID:   999,
		MaxPDU:     1476,
		SubnetCIDR: 24,
	}

	tests := []struct {
		name     string
		modify   func(cfg *model.BACnetServerConfig)
		expected bool
	}{
		{"same config", func(cfg *model.BACnetServerConfig) {}, false},
		{"port changed", func(cfg *model.BACnetServerConfig) { cfg.Port = 47809 }, true},
		{"device ID changed", func(cfg *model.BACnetServerConfig) { cfg.DeviceID = 2000 }, true},
		{"max PDU changed", func(cfg *model.BACnetServerConfig) { cfg.MaxPDU = 2048 }, true},
		{"subnet CIDR changed", func(cfg *model.BACnetServerConfig) { cfg.SubnetCIDR = 16 }, true},
		{"IP changed", func(cfg *model.BACnetServerConfig) { cfg.IP = "192.168.1.1" }, true},
		{"interface changed", func(cfg *model.BACnetServerConfig) { cfg.Interface = "eth1" }, true},
		{"name changed", func(cfg *model.BACnetServerConfig) { cfg.Name = "New Name" }, false},
		{"vendor ID changed", func(cfg *model.BACnetServerConfig) { cfg.VendorID = 888 }, false},
		{"devices changed", func(cfg *model.BACnetServerConfig) {
			cfg.Devices = model.OpcUaDeviceMap{"dev1": {Enable: true}}
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modified := base
			tt.modify(&modified)
			result := requiresServerRestart(base, modified)
			if result != tt.expected {
				t.Errorf("requiresServerRestart() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGenerateDeviceID 测试设备 ID 生成
func TestGenerateDeviceID(t *testing.T) {
	cfg := model.BACnetServerConfig{
		Name: "TestServer",
	}
	srv := NewServer(cfg, nil)

	id1 := srv.generateDeviceID()
	id2 := srv.generateDeviceID()

	// 相同名称应生成相同 ID
	if id1 != id2 {
		t.Errorf("generateDeviceID() not deterministic: %d != %d", id1, id2)
	}

	// ID 应在有效范围内
	if id1 < 1000 || id1 > 4194303 {
		t.Errorf("generateDeviceID() = %d, out of range [1000, 4194303]", id1)
	}
}

// TestBuildDeviceConfig 测试 DeviceConfig 构建
func TestBuildDeviceConfig(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:         "test",
		Name:       "Test Server",
		Port:       47808,
		DeviceID:   1234,
		DeviceName: "MyDevice",
		VendorID:   999,
		Interface:  "eth0",
		IP:         "192.168.1.100",
		SubnetCIDR: 24,
		MaxPDU:     1476,
	}
	srv := NewServer(cfg, nil)
	devCfg := srv.buildDeviceConfig()

	if devCfg.Port != 47808 {
		t.Errorf("expected port 47808, got %d", devCfg.Port)
	}
	if devCfg.DeviceID != 1234 {
		t.Errorf("expected device ID 1234, got %d", devCfg.DeviceID)
	}
	if devCfg.DeviceName != "MyDevice" {
		t.Errorf("expected device name 'MyDevice', got '%s'", devCfg.DeviceName)
	}
}

// TestBuildDeviceConfigDefaults 测试默认值
func TestBuildDeviceConfigDefaults(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:   "test",
		Name: "Test Server",
	}
	srv := NewServer(cfg, nil)
	devCfg := srv.buildDeviceConfig()

	if devCfg.Port != 47808 {
		t.Errorf("expected default port 47808, got %d", devCfg.Port)
	}
	if devCfg.VendorID != 999 {
		t.Errorf("expected default vendor ID 999, got %d", devCfg.VendorID)
	}
	if devCfg.SubnetCIDR != 24 {
		t.Errorf("expected default subnet CIDR 24, got %d", devCfg.SubnetCIDR)
	}
	if devCfg.MaxPDU != 1476 {
		t.Errorf("expected default max PDU 1476, got %d", devCfg.MaxPDU)
	}
	if devCfg.DeviceName == "" {
		t.Error("device name should not be empty")
	}
}

// TestStats 测试统计信息
func TestStats(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:   "test",
		Name: "Test",
	}
	srv := NewServer(cfg, nil)

	stats := srv.GetStats()
	if stats.WriteCount != 0 {
		t.Errorf("expected write count 0, got %d", stats.WriteCount)
	}
	if stats.UpdateCount != 0 {
		t.Errorf("expected update count 0, got %d", stats.UpdateCount)
	}
}

// TestWriteHistory 测试写入历史
func TestWriteHistory(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:   "test",
		Name: "Test",
	}
	srv := NewServer(cfg, nil)

	// 初始为空
	history := srv.GetWriteHistory(10)
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d items", len(history))
	}

	// 记录写入历史
	srv.recordWriteHistory("ch1", "dev1", "point1", 25.5, true, "")
	srv.recordWriteHistory("ch1", "dev1", "point2", 30.0, false, "timeout")

	history = srv.GetWriteHistory(10)
	if len(history) != 2 {
		t.Errorf("expected 2 history items, got %d", len(history))
	}
	if history[0].PointID != "point1" {
		t.Errorf("expected first item point1, got %s", history[0].PointID)
	}
	if history[1].Success {
		t.Errorf("expected second item to have success=false")
	}
}

// TestWriteHistoryLimit 测试写入历史限制
func TestWriteHistoryLimit(t *testing.T) {
	cfg := model.BACnetServerConfig{
		ID:   "test",
		Name: "Test",
	}
	srv := NewServer(cfg, nil)

	// 记录超过 100 条
	for i := 0; i < 150; i++ {
		srv.recordWriteHistory("ch1", "dev1", "point", float64(i), true, "")
	}

	history := srv.GetWriteHistory(0)
	if len(history) > 100 {
		t.Errorf("history should be capped at 100, got %d", len(history))
	}
}

// TestDefaultValueForType 辅助函数测试
func defaultValueForType(dataType string) any {
	switch dataType {
	case "float32", "float64", "float":
		return float64(0.0)
	case "bool", "boolean":
		return false
	case "string":
		return ""
	default:
		return float64(0.0)
	}
}
