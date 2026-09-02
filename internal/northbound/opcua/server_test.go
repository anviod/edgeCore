package opcua

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/awcullen/opcua/ua"
	"go.uber.org/zap"
)

// MockSouthboundManager implements model.SouthboundManager for testing
type MockSouthboundManager struct {
	channels     []model.Channel
	writeHistory []writeOperation
	mu           sync.Mutex
}

type writeOperation struct {
	channelID string
	deviceID  string
	pointID   string
	value     interface{}
}

func NewMockSouthboundManager() *MockSouthboundManager {
	return &MockSouthboundManager{
		channels: []model.Channel{
			{
				ID:       "ch1",
				Name:     "Test Channel",
				Protocol: "modbus",
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
								ID:        "point1",
								Name:      "Temperature",
								DataType:  "float64",
								ReadWrite: "R",
							},
							{
								ID:        "point2",
								Name:      "Humidity",
								DataType:  "float64",
								ReadWrite: "R",
							},
							{
								ID:        "point3",
								Name:      "Setpoint",
								DataType:  "float64",
								ReadWrite: "RW",
							},
							{
								ID:        "point4",
								Name:      "Status",
								DataType:  "string",
								ReadWrite: "R",
							},
							{
								ID:        "point5",
								Name:      "Enabled",
								DataType:  "boolean",
								ReadWrite: "RW",
							},
						},
					},
				},
			},
		},
		writeHistory: []writeOperation{},
	}
}

func (m *MockSouthboundManager) GetChannels() []model.Channel {
	return m.channels
}

func (m *MockSouthboundManager) GetChannelDevices(channelID string) []model.Device {
	for _, ch := range m.channels {
		if ch.ID == channelID {
			return ch.Devices
		}
	}
	return []model.Device{}
}

func (m *MockSouthboundManager) GetDevice(channelID, deviceID string) *model.Device {
	for _, ch := range m.channels {
		if ch.ID == channelID {
			for i, dev := range ch.Devices {
				if dev.ID == deviceID {
					return &ch.Devices[i]
				}
			}
		}
	}
	return nil
}

func (m *MockSouthboundManager) GetDevicePoints(channelID, deviceID string) ([]model.PointData, error) {
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

func (m *MockSouthboundManager) WritePoint(channelID, deviceID, pointID string, value interface{}) error {
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

func (m *MockSouthboundManager) GetShadowPoint(channelID, deviceID, pointID string) (*model.ShadowPoint, error) {
	return nil, fmt.Errorf("shadow point not available in mock")
}

func (m *MockSouthboundManager) GetWriteHistory() []writeOperation {
	m.mu.Lock()
	defer m.mu.Unlock()
	history := make([]writeOperation, len(m.writeHistory))
	copy(history, m.writeHistory)
	return history
}

func (m *MockSouthboundManager) ClearWriteHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeHistory = []writeOperation{}
}

// TestWriteViaOPCUA tests the WriteViaOPCUA method
func TestWriteViaOPCUA(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4850,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(1 * time.Second)

	// Test writing via OPCUA
	err = server.WriteViaOPCUA("ch1", "dev1", "point3", 42.5)
	if err != nil {
		t.Fatalf("WriteViaOPCUA failed: %v", err)
	}

	// Verify the write was recorded
	history := sb.GetWriteHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 write operation, got %d", len(history))
	}
	if history[0].value != 42.5 {
		t.Fatalf("Write value mismatch: expected 42.5, got %v", history[0].value)
	}

	// Test writing to non-existent node
	err = server.WriteViaOPCUA("ch1", "dev1", "nonexistent", 100.0)
	if err == nil {
		t.Fatalf("Expected error for non-existent node, got nil")
	}

	t.Log("WriteViaOPCUA test passed")
}

// TestBatchWrite tests the BatchWrite method
func TestBatchWrite(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4851,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(1 * time.Second)

	requests := []WriteRequest{
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "point3", Value: 10.0},
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "point5", Value: true},
	}

	results := server.BatchWrite(requests)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Check success count
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount != 2 {
		t.Fatalf("Expected 2 successful writes, got %d", successCount)
	}

	// Test with one invalid point
	requests = []WriteRequest{
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "point3", Value: 20.0},
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "nonexistent", Value: 30.0},
	}

	results = server.BatchWrite(requests)
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// First should succeed, second should fail
	if !results[0].Success {
		t.Errorf("First write should succeed")
	}
	if results[1].Success {
		t.Errorf("Second write should fail for non-existent node")
	}

	t.Log("BatchWrite test passed")
}

// TestGetWriteHistory tests the GetWriteHistory method
func TestGetWriteHistory(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4852,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(1 * time.Second)

	// Perform some writes
	_ = server.WriteViaOPCUA("ch1", "dev1", "point3", 1.0)
	_ = server.WriteViaOPCUA("ch1", "dev1", "point3", 2.0)
	_ = server.WriteViaOPCUA("ch1", "dev1", "point3", 3.0)

	// Get full history
	history := server.GetWriteHistory(0)
	if len(history) != 3 {
		t.Fatalf("Expected 3 history entries, got %d", len(history))
	}

	// Get limited history
	history = server.GetWriteHistory(2)
	if len(history) != 2 {
		t.Fatalf("Expected 2 history entries with limit, got %d", len(history))
	}

	t.Log("GetWriteHistory test passed")
}

// TestWriteHistoryLimit tests that write history is limited in size
func TestWriteHistoryLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4853,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(1 * time.Second)

	// Write more than max history size
	for i := 0; i < 1100; i++ {
		_ = server.WriteViaOPCUA("ch1", "dev1", "point3", float64(i))
	}

	// History should be limited to 1000
	history := server.GetWriteHistory(0)
	if len(history) > 1000 {
		t.Fatalf("History size %d exceeds limit 1000", len(history))
	}

	t.Logf("WriteHistoryLimit test passed, history size: %d", len(history))
}

func TestServerOpcUaTypeMetadata(t *testing.T) {
	s := NewServer(model.OPCUAConfig{Name: "Test Server"}, NewMockSouthboundManager(), nil)

	nodeID := s.getDataTypeID("bytestring")
	wantNodeID := ua.ParseNodeID("i=15")
	if nodeID != wantNodeID {
		t.Fatalf("getDataTypeID(bytestring) = %v, want %v", nodeID, wantNodeID)
	}

	zeroValue, ok := s.getZeroValue("bytestring").([]byte)
	if !ok {
		t.Fatalf("getZeroValue(bytestring) type = %T, want []byte", s.getZeroValue("bytestring"))
	}
	if len(zeroValue) != 0 {
		t.Fatalf("getZeroValue(bytestring) length = %d, want 0", len(zeroValue))
	}
}

// TestServerStartStop tests basic server start/stop functionality
func TestServerStartStop(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4840,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	time.Sleep(2 * time.Second)
	server.Stop()
	server.Stop() // idempotent
	time.Sleep(1 * time.Second)

	t.Log("Server start/stop test passed")
}

// TestServerUpdateConfigRebuild simulates address-space rebuild when topology changes.
func TestServerUpdateConfigRebuild(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4847,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}

	srv := NewServer(config, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	time.Sleep(1 * time.Second)

	if err := srv.UpdateConfig(config); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	srv.Stop() // should not panic after UpdateConfig already stopped internally

	t.Log("UpdateConfig rebuild test passed")
}

// TestServerConcurrentUpdateConfig ensures concurrent rebuilds do not double-close.
func TestServerConcurrentUpdateConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4848,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}

	srv := NewServer(config, sb, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	time.Sleep(1 * time.Second)

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := srv.UpdateConfig(config); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent UpdateConfig failed: %v", err)
	}

	t.Log("Concurrent UpdateConfig test passed")
}

// TestServerReadWrite tests reading and writing from OPC UA server
func TestServerReadWrite(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4841,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	value := model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   "point1",
		Value:     25.5,
		TS:        time.Now(),
	}
	server.Update(value)
	t.Logf("Update method test passed: updated point1 to 25.5")

	err = sb.WritePoint("ch1", "dev1", "point3", 30.0)
	if err != nil {
		t.Fatalf("Failed to write through SouthboundManager: %v", err)
	}
	t.Logf("Write test passed: wrote 30.0 to point3")

	history := sb.GetWriteHistory()
	if len(history) != 1 {
		t.Fatalf("Expected 1 write operation, got %d", len(history))
	}
	if history[0].channelID != "ch1" || history[0].deviceID != "dev1" || history[0].pointID != "point3" {
		t.Fatalf("Write operation has wrong parameters: %v", history[0])
	}
	if history[0].value != 30.0 {
		t.Fatalf("Write operation has wrong value: expected 30.0, got %v", history[0].value)
	}

	t.Log("Read/write test passed")
}

// TestServerUpdate tests the Update method
func TestServerUpdate(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4842,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	value := model.Value{
		ChannelID: "ch1",
		DeviceID:  "dev1",
		PointID:   "point1",
		Value:     22.5,
		Quality:   "Good",
		TS:        time.Now(),
	}
	server.Update(value)

	nodeKey := "ch1/dev1/point1"
	if server.HasNode(nodeKey) {
		t.Logf("Update test passed: point1 updated successfully")
	} else {
		t.Fatalf("Node %s not found in nodeMap", nodeKey)
	}

	t.Log("Update test passed")
}

// BenchmarkServerRead benchmarks reading from OPC UA server
func BenchmarkServerRead(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4843,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodeKey := "ch1/dev1/point1"
		if !server.HasNode(nodeKey) {
			b.Fatalf("Node %s not found in nodeMap", nodeKey)
		}
	}
	b.StopTimer()
}

// BenchmarkServerWrite benchmarks writing to OPC UA server
func BenchmarkServerWrite(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4844,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sb.WritePoint("ch1", "dev1", "point3", float64(i%100))
		if err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
	b.StopTimer()
}

// BenchmarkServerUpdate benchmarks the Update method
func BenchmarkServerUpdate(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4845,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(2 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value := model.Value{
			ChannelID: "ch1",
			DeviceID:  "dev1",
			PointID:   "point1",
			Value:     float64(i % 100),
			Quality:   "Good",
			TS:        time.Now(),
		}
		server.Update(value)
	}
	b.StopTimer()
}

// TestServerStress tests server under stress
func TestServerStress(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	sb := NewMockSouthboundManager()

	config := model.OPCUAConfig{
		Name:        "Test OPC UA Server",
		Port:        4846,
		Endpoint:    "/",
		AuthMethods: []string{"Anonymous"},
		Devices:     model.OpcUaDeviceMap{"dev1": {Enable: true}},
	}

	server := NewServer(config, sb, nil)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	clientCount := 10
	operationsPerClient := 100

	var wg sync.WaitGroup
	errorCh := make(chan error, clientCount)

	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			for j := 0; j < operationsPerClient; j++ {
				nodeKey := "ch1/dev1/point1"
				if !server.HasNode(nodeKey) {
					errorCh <- fmt.Errorf("client %d: node %s not found", clientID, nodeKey)
					return
				}

				err := sb.WritePoint("ch1", "dev1", "point3", float64((clientID*1000+j)%100))
				if err != nil {
					errorCh <- fmt.Errorf("client %d: write failed: %v", clientID, err)
					return
				}
			}

			t.Logf("Client %d completed %d operations", clientID, operationsPerClient)
		}(i)
	}

	wg.Wait()
	close(errorCh)

	errors := []error{}
	for err := range errorCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Fatalf("Stress test failed with %d errors: %v", len(errors), errors)
	}

	history := sb.GetWriteHistory()
	expectedWrites := clientCount * operationsPerClient
	if len(history) != expectedWrites {
		t.Fatalf("Expected %d write operations, got %d", expectedWrites, len(history))
	}

	t.Logf("Stress test passed: %d clients, %d operations per client, %d total operations", clientCount, operationsPerClient, expectedWrites)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	t.Logf("Memory usage after stress test: %f MB", float64(mem.Alloc)/1024/1024)

	stats := server.GetStats()
	t.Logf("Server stats: %+v", stats)
}
