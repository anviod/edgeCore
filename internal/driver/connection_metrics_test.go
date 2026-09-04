package driver_test

import (
	"testing"

	_ "github.com/anviod/bacnet" // 导入BACnet驱动
	"github.com/anviod/edgeCore/internal/driver"
	_ "github.com/anviod/edgeCore/internal/driver/dlt645"     // 导入DLT645驱动
	_ "github.com/anviod/edgeCore/internal/driver/ethernetip" // 导入EtherNet/IP驱动
	_ "github.com/anviod/edgeCore/internal/driver/ice104"     // 导入ICE104驱动
	_ "github.com/anviod/edgeCore/internal/driver/mitsubishi" // 导入Mitsubishi驱动
	_ "github.com/anviod/edgeCore/internal/driver/modbus"     // 导入Modbus驱动
	_ "github.com/anviod/edgeCore/internal/driver/omron"      // 导入Omron驱动
	_ "github.com/anviod/edgeCore/internal/driver/opcua"      // 导入OPC UA驱动
	_ "github.com/anviod/edgeCore/internal/driver/s7"         // 导入S7驱动
	_ "github.com/anviod/edgeCore/internal/driver/snmp"       // 导入SNMP驱动
	"github.com/anviod/edgeCore/internal/model"
)

// testGetMetrics 辅助函数，测试GetMetrics方法
func testGetMetrics(t *testing.T, driver driver.Driver, expectedProtocol string) {
	if metricsProvider, ok := driver.(interface{ GetMetrics() model.ChannelMetrics }); ok {
		metrics := metricsProvider.GetMetrics()
		if metrics.Protocol != expectedProtocol {
			t.Errorf("Expected protocol '%s', got '%s'", expectedProtocol, metrics.Protocol)
		}
		if metrics.QualityScore < 0 || metrics.QualityScore > 100 {
			t.Errorf("Expected quality score between 0-100, got %d", metrics.QualityScore)
		}
		if metrics.TotalRequests < 0 {
			t.Errorf("Expected non-negative total requests, got %d", metrics.TotalRequests)
		}
		if metrics.SuccessCount < 0 || metrics.FailureCount < 0 {
			t.Errorf("Expected non-negative success/failure counts")
		}
		if metrics.SuccessRate < 0 || metrics.SuccessRate > 1 {
			t.Errorf("Expected success rate between 0-1, got %f", metrics.SuccessRate)
		}
	}
}

// TestBACnetConnectionMetrics 测试BACnet驱动的连接指标收集
func TestBACnetConnectionMetrics(t *testing.T) {
	// 创建BACnet驱动实例
	d, ok := driver.GetDriver("bacnet-ip")
	if !ok {
		t.Logf("BACnet driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("BACnet driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"interface_ip":   "127.0.0.1",
			"interface_port": 47808,
			"subnet_cidr":    24,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init BACnet driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "127.0.0.1:47808" {
		t.Errorf("Expected local addr '127.0.0.1:47808', got '%s'", localAddr)
	}
	if remoteAddr != "广播" {
		t.Errorf("Expected remote addr '广播', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "BACnet")
}

// TestModbusConnectionMetrics 测试Modbus驱动的连接指标收集
func TestModbusConnectionMetrics(t *testing.T) {
	// 创建Modbus驱动实例
	d, ok := driver.GetDriver("modbus-tcp")
	if !ok {
		t.Logf("Modbus driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("Modbus driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":   "127.0.0.1",
			"port": 502,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init Modbus driver: %v", err)
	}

	// 测试GetConnectionMetrics方法存在
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	// Modbus驱动的GetConnectionMetrics委托给transport，所以初始值可能为0
	t.Logf("Initial metrics: connSec=%d, reconCount=%d, localAddr='%s', remoteAddr='%s', lastDisc=%v",
		connSec, reconCount, localAddr, remoteAddr, lastDisc)

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "Modbus")
}

// TestOpcUaConnectionMetrics 测试OPC UA驱动的连接指标收集
func TestOpcUaConnectionMetrics(t *testing.T) {
	// 创建OPC UA驱动实例
	d, ok := driver.GetDriver("opc-ua")
	if !ok {
		t.Logf("OPC UA driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("OPC UA driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"endpoint": "opc.tcp://127.0.0.1:4840",
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init OPC UA driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "127.0.0.1:0" {
		t.Errorf("Expected local addr '127.0.0.1:0', got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:4840" {
		t.Errorf("Expected remote addr '127.0.0.1:4840', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "OPC-UA")
}

// TestS7ConnectionMetrics 测试S7驱动的连接指标收集
func TestS7ConnectionMetrics(t *testing.T) {
	// 创建S7驱动实例
	d, ok := driver.GetDriver("s7")
	if !ok {
		t.Logf("S7 driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("S7 driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":   "127.0.0.1",
			"port": 102,
			"rack": 0,
			"slot": 1,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init S7 driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:102" {
		t.Errorf("Expected remote addr '127.0.0.1:102', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "S7")
}

// TestDlt645ConnectionMetrics 测试DLT645驱动的连接指标收集
func TestDlt645ConnectionMetrics(t *testing.T) {
	// 创建DLT645驱动实例
	d, ok := driver.GetDriver("dlt645")
	if !ok {
		t.Logf("DLT645 driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("DLT645 driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"connectionType": "tcp",
			"ip":             "127.0.0.1",
			"port":           10000,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init DLT645 driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:10000" {
		t.Errorf("Expected remote addr '127.0.0.1:10000', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "DLT645")
}

// TestEtherNetIPConnectionMetrics 测试EtherNet/IP驱动的连接指标收集
func TestEtherNetIPConnectionMetrics(t *testing.T) {
	// 创建EtherNet/IP驱动实例
	d, ok := driver.GetDriver("ethernet-ip")
	if !ok {
		t.Logf("EtherNet/IP driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("EtherNet/IP driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":   "127.0.0.1",
			"port": 44818,
			"slot": 0,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init EtherNet/IP driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:44818" {
		t.Errorf("Expected remote addr '127.0.0.1:44818', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "EtherNet/IP")
}

// TestOmronConnectionMetrics 测试Omron驱动的连接指标收集
func TestOmronConnectionMetrics(t *testing.T) {
	// 创建Omron驱动实例
	d, ok := driver.GetDriver("omron-fins")
	if !ok {
		t.Logf("Omron driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("Omron driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":   "127.0.0.1",
			"port": 9600,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init Omron driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:9600" {
		t.Errorf("Expected remote addr '127.0.0.1:9600', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "Omron FINS")
}

// TestICE104ConnectionMetrics 测试 ICE104 驱动的连接指标收集
func TestICE104ConnectionMetrics(t *testing.T) {
	d, ok := driver.GetDriver("iec60870-5-104")
	if !ok || d == nil {
		t.Fatalf("iec60870-5-104 driver not available")
	}

	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":            "127.0.0.1",
			"port":          2404,
			"commonAddress": 1,
		},
	}
	if err := d.Init(config); err != nil {
		t.Fatalf("Failed to init ICE104 driver: %v", err)
	}

	_, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:2404" {
		t.Errorf("Expected remote addr '127.0.0.1:2404', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}
}

// TestSNMPConnectionMetrics 测试 SNMP 驱动的连接指标收集
func TestSNMPConnectionMetrics(t *testing.T) {
	d, ok := driver.GetDriver("snmp")
	if !ok || d == nil {
		t.Fatalf("snmp driver not available")
	}

	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":          "127.0.0.1",
			"port":        161,
			"snmpVersion": "v2c",
			"community":   "public",
		},
	}
	if err := d.Init(config); err != nil {
		t.Fatalf("Failed to init SNMP driver: %v", err)
	}

	_, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:161" {
		t.Errorf("Expected remote addr '127.0.0.1:161', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}
}

// TestMitsubishiConnectionMetrics 测试Mitsubishi驱动的连接指标收集
func TestMitsubishiConnectionMetrics(t *testing.T) {
	// 创建Mitsubishi驱动实例
	d, ok := driver.GetDriver("mitsubishi-slmp")
	if !ok {
		t.Logf("Mitsubishi driver not available, skipping test")
		return
	}
	if d == nil {
		t.Logf("Mitsubishi driver is nil, skipping test")
		return
	}

	// 初始化配置
	config := model.DriverConfig{
		ChannelID: "test-channel",
		Config: map[string]any{
			"ip":   "127.0.0.1",
			"port": 2000,
		},
	}

	err := d.Init(config)
	if err != nil {
		t.Fatalf("Failed to init Mitsubishi driver: %v", err)
	}

	// 测试初始状态
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()
	if connSec != 0 {
		t.Errorf("Expected connection seconds 0, got %d", connSec)
	}
	if reconCount != 0 {
		t.Errorf("Expected reconnect count 0, got %d", reconCount)
	}
	if localAddr != "" {
		t.Errorf("Expected empty local addr, got '%s'", localAddr)
	}
	if remoteAddr != "127.0.0.1:2000" {
		t.Errorf("Expected remote addr '127.0.0.1:2000', got '%s'", remoteAddr)
	}
	if !lastDisc.IsZero() {
		t.Errorf("Expected zero last disconnect time, got %v", lastDisc)
	}

	// 测试GetMetrics方法（如果支持）
	testGetMetrics(t, d, "Mitsubishi MC")
}
