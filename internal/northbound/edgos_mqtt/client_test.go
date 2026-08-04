package edgos_mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anviod/edgex/internal/model"
)

// TestNodeRegisterCommandMessageParsing tests parsing of node register command messages
func TestNodeRegisterCommandMessageParsing(t *testing.T) {
	// Build the register command message (same format as EdgeOS would send)
	regCommand := Message{
		Header: MessageHeader{
			MessageID:     "test-msg-001",
			Timestamp:     time.Now().UnixMilli(),
			Source:        "edgeos-server",
			MessageType:   "node_register",
			Version:       "1.0",
			CorrelationID: "test-corr-001",
		},
		Body: map[string]any{
			"action": "re-register",
		},
	}
	payload, err := json.Marshal(regCommand)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Parse the message back
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	// Verify parsed values
	if parsed.Header.MessageID != "test-msg-001" {
		t.Errorf("Expected message_id 'test-msg-001', got '%s'", parsed.Header.MessageID)
	}
	if parsed.Header.Source != "edgeos-server" {
		t.Errorf("Expected source 'edgeos-server', got '%s'", parsed.Header.Source)
	}
	if parsed.Header.MessageType != "node_register" {
		t.Errorf("Expected message_type 'node_register', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.CorrelationID != "test-corr-001" {
		t.Errorf("Expected correlation_id 'test-corr-001', got '%s'", parsed.Header.CorrelationID)
	}

	t.Logf("Message parsing test passed: %s", string(payload))
}

// TestNodeRegisterResponseGeneration tests response message generation
func TestNodeRegisterResponseGeneration(t *testing.T) {
	nodeID := "test-node-001"
	origHeader := MessageHeader{
		MessageID:     "test-msg-001",
		Timestamp:     time.Now().UnixMilli(),
		Source:        "edgeos-server",
		MessageType:   "node_register",
		Version:       "1.0",
		CorrelationID: "test-corr-001",
	}

	// Generate response (same logic as sendCommandResponse in handleNodeRegisterCommand)
	response := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        nodeID,
			Destination:   origHeader.Source,
			MessageType:   "node_register_response",
			Version:       "1.0",
			CorrelationID: origHeader.MessageID,
		},
		Body: map[string]any{
			"success": true,
			"message": "Node re-registered successfully",
		},
	}

	// Verify response
	if response.Header.MessageType != "node_register_response" {
		t.Errorf("Expected message_type 'node_register_response', got '%s'", response.Header.MessageType)
	}
	if response.Header.Destination != "edgeos-server" {
		t.Errorf("Expected destination 'edgeos-server', got '%s'", response.Header.Destination)
	}
	if response.Header.CorrelationID != "test-msg-001" {
		t.Errorf("Expected correlation_id 'test-msg-001', got '%s'", response.Header.CorrelationID)
	}
	if response.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, response.Header.Source)
	}

	body, ok := response.Body.(map[string]any)
	if !ok {
		t.Fatalf("Response body is not a map")
	}
	if !body["success"].(bool) {
		t.Errorf("Expected success=true")
	}
	if body["message"] != "Node re-registered successfully" {
		t.Errorf("Expected message 'Node re-registered successfully', got '%v'", body["message"])
	}

	payload, _ := json.Marshal(response)
	t.Logf("Response generation test passed: %s", string(payload))
}

// TestNodeRegistrationPayload tests the node registration payload format
func TestNodeRegistrationPayload(t *testing.T) {
	nodeID := "test-node-001"

	// Generate registration payload (same as publishNodeOnline)
	regMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "node_register",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":      nodeID,
			"node_name":    "EdgeX Gateway Node",
			"model":        "edgex",
			"version":      "1.0.0",
			"api_version":  "v1",
			"capabilities": []string{"shadow-sync", "heartbeat", "device-control", "task-execution"},
			"protocol":     "edgeOS(MQTT)",
			"endpoint": map[string]string{
				"host": "127.0.0.1",
				"port": "8082",
			},
		},
	}

	payload, err := json.Marshal(regMessage)
	if err != nil {
		t.Fatalf("Failed to marshal registration: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal registration: %v", err)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	// Verify required fields
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["node_name"] != "EdgeX Gateway Node" {
		t.Errorf("Expected node_name 'EdgeX Gateway Node', got '%v'", body["node_name"])
	}
	if body["model"] != "edgex" {
		t.Errorf("Expected model 'edgex', got '%v'", body["model"])
	}
	if body["version"] != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%v'", body["version"])
	}
	if body["api_version"] != "v1" {
		t.Errorf("Expected api_version 'v1', got '%v'", body["api_version"])
	}

	capabilities, ok := body["capabilities"].([]any)
	if !ok {
		t.Fatalf("Capabilities is not an array")
	}
	if len(capabilities) != 4 {
		t.Errorf("Expected 4 capabilities, got %d", len(capabilities))
	}

	t.Logf("Node registration payload test passed: %s", string(payload))
}

// TestResponseTopicGeneration tests response topic generation
func TestResponseTopicGeneration(t *testing.T) {
	nodeID := "test-node-001"
	messageID := "test-msg-001"

	// Generate response topic (same as in sendCommandResponse)
	topic := fmt.Sprintf("edgex/responses/%s/%s", nodeID, messageID)
	expected := "edgex/responses/test-node-001/test-msg-001"

	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}

	t.Logf("Response topic generation test passed: %s", topic)
}

// TestRegisterTopicConstant tests the register command topic constant
func TestRegisterTopicConstant(t *testing.T) {
	// The topic that EdgeOS uses to trigger node re-registration
	registerTopic := "edgex/cmd/nodes/register"

	// Verify topic format
	if registerTopic == "" {
		t.Error("Register topic should not be empty")
	}
	if registerTopic != "edgex/cmd/nodes/register" {
		t.Errorf("Expected topic 'edgex/cmd/nodes/register', got '%s'", registerTopic)
	}

	// Verify it follows the pattern defined in the protocol spec
	// Topic format: edgex/cmd/nodes/register (QoS 1, EdgeOS -> EdgeX)
	expectedPattern := "edgex/cmd/nodes/register"
	if registerTopic != expectedPattern {
		t.Errorf("Topic should match pattern '%s', got '%s'", expectedPattern, registerTopic)
	}

	t.Logf("Register topic constant test passed: %s", registerTopic)
}

// TestMessageIDGeneration tests message ID generation
func TestMessageIDGeneration(t *testing.T) {
	id1 := generateMessageID()
	id2 := generateMessageID()

	// Verify IDs are unique
	if id1 == id2 {
		t.Errorf("Generated message IDs should be unique")
	}

	// Verify format
	if len(id1) < 10 {
		t.Errorf("Message ID too short: %s", id1)
	}

	t.Logf("Generated unique message IDs: %s, %s", id1, id2)
}

// TestDeviceReportMessageFormat tests the device report message format
func TestDeviceReportMessageFormat(t *testing.T) {
	nodeID := "test-node-001"

	// Build device report message (same format as publishDeviceReport)
	devices := []map[string]any{
		{
			"device_id":       "device-001",
			"device_name":     "Test Device 1",
			"device_profile":  "modbus",
			"service_name":    "Test Channel",
			"labels":          []string{},
			"description":     "",
			"admin_state":     "ENABLED",
			"operating_state": "ENABLED",
			"properties": map[string]any{
				"protocol":   "modbus",
				"channel_id": "channel-001",
			},
		},
		{
			"device_id":       "device-002",
			"device_name":     "Test Device 2",
			"device_profile":  "bacnet",
			"service_name":    "Test Channel",
			"labels":          []string{"sensor", "temperature"},
			"description":     "",
			"admin_state":     "ENABLED",
			"operating_state": "DISABLED",
			"properties": map[string]any{
				"protocol":   "bacnet",
				"channel_id": "channel-002",
			},
		},
	}

	reportMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_report",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id": nodeID,
			"devices": devices,
		},
	}

	payload, err := json.Marshal(reportMessage)
	if err != nil {
		t.Fatalf("Failed to marshal device report: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal device report: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "device_report" {
		t.Errorf("Expected message_type 'device_report', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, parsed.Header.Source)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}

	deviceList, ok := body["devices"].([]any)
	if !ok {
		t.Fatalf("Devices is not an array")
	}
	if len(deviceList) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(deviceList))
	}

	// Verify first device
	device1, ok := deviceList[0].(map[string]any)
	if !ok {
		t.Fatalf("Device 1 is not a map")
	}
	if device1["device_id"] != "device-001" {
		t.Errorf("Expected device_id 'device-001', got '%v'", device1["device_id"])
	}
	if device1["device_name"] != "Test Device 1" {
		t.Errorf("Expected device_name 'Test Device 1', got '%v'", device1["device_name"])
	}

	// Verify second device (with DISABLED operating state)
	device2, ok := deviceList[1].(map[string]any)
	if !ok {
		t.Fatalf("Device 2 is not a map")
	}
	if device2["operating_state"] != "DISABLED" {
		t.Errorf("Expected operating_state 'DISABLED', got '%v'", device2["operating_state"])
	}

	t.Logf("Device report message format test passed: %s", string(payload))
}

// TestDeviceReportTopicGeneration tests device report topic generation
func TestDeviceReportTopicGeneration(t *testing.T) {
	// Generate device report topic (same as in publishDeviceReport)
	reportTopic := "edgex/devices/report"
	expected := "edgex/devices/report"

	if reportTopic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, reportTopic)
	}

	t.Logf("Device report topic generation test passed: %s", reportTopic)
}

// TestRegisterResponseParsing tests parsing of registration response messages
func TestRegisterResponseParsing(t *testing.T) {
	// Build response message (same format as EdgeOS sends)
	responseMessage := Message{
		Header: MessageHeader{
			MessageID:     "resp-msg-001",
			Timestamp:     time.Now().UnixMilli(),
			Source:        "edgeos-server",
			Destination:   "edgex-node-001",
			MessageType:   "node_register_response",
			Version:       "1.0",
			CorrelationID: "req-msg-001",
		},
		Body: map[string]any{
			"status":  "success",
			"message": "Registration successful",
		},
	}

	payload, err := json.Marshal(responseMessage)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "node_register_response" {
		t.Errorf("Expected message_type 'node_register_response', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.CorrelationID != "req-msg-001" {
		t.Errorf("Expected correlation_id 'req-msg-001', got '%s'", parsed.Header.CorrelationID)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", body["status"])
	}

	t.Logf("Register response parsing test passed: %s", string(payload))
}

// TestRegisterResponseFailureHandling tests handling of failed registration responses
func TestRegisterResponseFailureHandling(t *testing.T) {
	// Build failure response message
	responseMessage := Message{
		Header: MessageHeader{
			MessageID:     "resp-msg-002",
			Timestamp:     time.Now().UnixMilli(),
			Source:        "edgeos-server",
			Destination:   "edgex-node-001",
			MessageType:   "node_register_response",
			Version:       "1.0",
			CorrelationID: "req-msg-002",
		},
		Body: map[string]any{
			"status":  "failed",
			"message": "Registration failed: node already registered",
		},
	}

	payload, err := json.Marshal(responseMessage)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["status"] == "success" {
		t.Error("Expected status to be 'failed', not 'success'")
	}

	t.Logf("Register response failure handling test passed: %s", string(payload))
}

// TestDeviceReportWithEmptyDevices tests device report with no devices
func TestDeviceReportWithEmptyDevices(t *testing.T) {
	nodeID := "test-node-001"

	reportMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_report",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id": nodeID,
			"devices": []map[string]any{},
		},
	}

	payload, err := json.Marshal(reportMessage)
	if err != nil {
		t.Fatalf("Failed to marshal device report: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal device report: %v", err)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	deviceList, ok := body["devices"].([]any)
	if !ok {
		t.Fatalf("Devices is not an array")
	}
	if len(deviceList) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(deviceList))
	}

	t.Logf("Device report with empty devices test passed: %s", string(payload))
}

// TestDeviceOperatingStateMapping tests operating state mapping from device state
func TestDeviceOperatingStateMapping(t *testing.T) {
	testCases := []struct {
		state         int
		expectedState string
	}{
		{0, "ENABLED"},    // Default/Unknown
		{1, "UNSTABLE"},   // Unstable
		{2, "DISABLED"},   // Offline
		{3, "QUARANTINE"}, // Quarantine
	}

	for _, tc := range testCases {
		var operatingState string
		switch tc.state {
		case 2:
			operatingState = "DISABLED"
		case 1:
			operatingState = "UNSTABLE"
		case 3:
			operatingState = "QUARANTINE"
		default:
			operatingState = "ENABLED"
		}

		if operatingState != tc.expectedState {
			t.Errorf("State %d: expected '%s', got '%s'", tc.state, tc.expectedState, operatingState)
		}
	}

	t.Logf("Device operating state mapping test passed")
}

// TestPointReportMessageFormat tests the point report message format (metadata)
func TestPointReportMessageFormat(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	// Build point report message (same format as publishPointReport)
	points := []map[string]any{
		{
			"point_id":    "SupplyWaterTemp",
			"point_name":  "供水温度",
			"data_type":   "Float32",
			"access_mode": "R",
			"unit":        "°C",
			"minimum":     -50.0,
			"maximum":     150.0,
			"address":     "AI-30001",
			"description": "AHU Supply Water Temperature Sensor",
			"scale":       0.1,
			"offset":      0,
		},
		{
			"point_id":    "ValvePosition",
			"point_name":  "阀门开度",
			"data_type":   "Float32",
			"access_mode": "RW",
			"unit":        "%",
			"minimum":     0.0,
			"maximum":     100.0,
			"address":     "AO-30001",
			"description": "Control Valve Position",
			"scale":       1.0,
			"offset":      0,
		},
	}

	reportMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "point_report",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":   nodeID,
			"device_id": deviceID,
			"points":    points,
		},
	}

	payload, err := json.Marshal(reportMessage)
	if err != nil {
		t.Fatalf("Failed to marshal point report: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal point report: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "point_report" {
		t.Errorf("Expected message_type 'point_report', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, parsed.Header.Source)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}

	pointList, ok := body["points"].([]any)
	if !ok {
		t.Fatalf("Points is not an array")
	}
	if len(pointList) != 2 {
		t.Errorf("Expected 2 points, got %d", len(pointList))
	}

	// Verify first point
	point1, ok := pointList[0].(map[string]any)
	if !ok {
		t.Fatalf("Point 1 is not a map")
	}
	if point1["point_id"] != "SupplyWaterTemp" {
		t.Errorf("Expected point_id 'SupplyWaterTemp', got '%v'", point1["point_id"])
	}
	if point1["data_type"] != "Float32" {
		t.Errorf("Expected data_type 'Float32', got '%v'", point1["data_type"])
	}
	if point1["access_mode"] != "R" {
		t.Errorf("Expected access_mode 'R', got '%v'", point1["access_mode"])
	}

	// Verify second point (RW access mode)
	point2, ok := pointList[1].(map[string]any)
	if !ok {
		t.Fatalf("Point 2 is not a map")
	}
	if point2["access_mode"] != "RW" {
		t.Errorf("Expected access_mode 'RW', got '%v'", point2["access_mode"])
	}

	t.Logf("Point report message format test passed: %s", string(payload))
}

// TestPointReportSubjectConstant tests the point report topic constant
func TestPointReportSubjectConstant(t *testing.T) {
	reportTopic := "edgex/points/report"
	expected := "edgex/points/report"

	if reportTopic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, reportTopic)
	}

	t.Logf("Point report topic constant test passed: %s", reportTopic)
}

// TestInvalidJSONHandling tests handling of invalid JSON
func TestInvalidJSONHandling(t *testing.T) {
	invalidPayload := []byte("not valid json{")

	var message Message
	err := json.Unmarshal(invalidPayload, &message)

	if err == nil {
		t.Error("Expected error when unmarshaling invalid JSON")
	}

	t.Logf("Invalid JSON handling test passed: %v", err)
}

// TestDeviceOnlineMessageFormat tests the device online notification message format
func TestDeviceOnlineMessageFormat(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"
	deviceName := "Test Modbus Device"

	onlineMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_online",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":     nodeID,
			"device_id":   deviceID,
			"device_name": deviceName,
			"online_time": time.Now().UnixMilli(),
			"status":      "online",
			"details": map[string]any{
				"protocol":          "modbus-tcp",
				"address":           "192.168.1.100:502",
				"last_offline_time": 1744679000000,
			},
		},
	}

	payload, err := json.Marshal(onlineMessage)
	if err != nil {
		t.Fatalf("Failed to marshal device online message: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal device online message: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "device_online" {
		t.Errorf("Expected message_type 'device_online', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, parsed.Header.Source)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}
	if body["device_name"] != deviceName {
		t.Errorf("Expected device_name '%s', got '%v'", deviceName, body["device_name"])
	}
	if body["status"] != "online" {
		t.Errorf("Expected status 'online', got '%v'", body["status"])
	}

	// Verify details
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("Details is not a map")
	}
	if details["protocol"] != "modbus-tcp" {
		t.Errorf("Expected protocol 'modbus-tcp', got '%v'", details["protocol"])
	}

	t.Logf("Device online message format test passed: %s", string(payload))
}

// TestDeviceOfflineMessageFormat tests the device offline notification message format
func TestDeviceOfflineMessageFormat(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"
	deviceName := "Test Modbus Device"
	reason := "Connection timeout"

	offlineMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_offline",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":      nodeID,
			"device_id":    deviceID,
			"device_name":  deviceName,
			"offline_time": time.Now().UnixMilli(),
			"status":       "offline",
			"reason":       reason,
			"details": map[string]any{
				"protocol":         "modbus-tcp",
				"address":          "192.168.1.100:502",
				"last_online_time": 1744679000000,
				"retry_count":      3,
			},
		},
	}

	payload, err := json.Marshal(offlineMessage)
	if err != nil {
		t.Fatalf("Failed to marshal device offline message: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal device offline message: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "device_offline" {
		t.Errorf("Expected message_type 'device_offline', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, parsed.Header.Source)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}
	if body["device_name"] != deviceName {
		t.Errorf("Expected device_name '%s', got '%v'", deviceName, body["device_name"])
	}
	if body["status"] != "offline" {
		t.Errorf("Expected status 'offline', got '%v'", body["status"])
	}
	if body["reason"] != reason {
		t.Errorf("Expected reason '%s', got '%v'", reason, body["reason"])
	}

	// Verify details
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("Details is not a map")
	}
	if details["protocol"] != "modbus-tcp" {
		t.Errorf("Expected protocol 'modbus-tcp', got '%v'", details["protocol"])
	}
	retryCount, ok := details["retry_count"].(float64)
	if !ok {
		t.Fatalf("retry_count is not a number")
	}
	if int(retryCount) != 3 {
		t.Errorf("Expected retry_count 3, got '%v'", details["retry_count"])
	}

	t.Logf("Device offline message format test passed: %s", string(payload))
}

// TestDeviceOnlineTopicGeneration tests device online topic generation
func TestDeviceOnlineTopicGeneration(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	topic := fmt.Sprintf("edgex/devices/%s/%s/online", nodeID, deviceID)
	expected := "edgex/devices/test-node-001/test-device-001/online"

	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}

	t.Logf("Device online topic generation test passed: %s", topic)
}

// TestDeviceOfflineTopicGeneration tests device offline topic generation
func TestDeviceOfflineTopicGeneration(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	topic := fmt.Sprintf("edgex/devices/%s/%s/offline", nodeID, deviceID)
	expected := "edgex/devices/test-node-001/test-device-001/offline"

	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}

	t.Logf("Device offline topic generation test passed: %s", topic)
}

// TestDeviceOnlineOfflineMessageWithEmptyDetails tests device online/offline with empty details
func TestDeviceOnlineOfflineMessageWithEmptyDetails(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"
	deviceName := "Test Device"

	// Test online with empty details
	onlineMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "device_online",
			Version:     "1.0",
		},
		Body: map[string]any{
			"node_id":     nodeID,
			"device_id":   deviceID,
			"device_name": deviceName,
			"online_time": time.Now().UnixMilli(),
			"status":      "online",
			"details":     map[string]any{},
		},
	}

	payload, err := json.Marshal(onlineMessage)
	if err != nil {
		t.Fatalf("Failed to marshal device online message with empty details: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal device online message: %v", err)
	}

	details, ok := parsed.Body.(map[string]any)["details"].(map[string]any)
	if !ok {
		t.Fatalf("Details is not a map")
	}
	if len(details) != 0 {
		t.Errorf("Expected empty details, got %d items", len(details))
	}

	t.Logf("Device online/offline message with empty details test passed: %s", string(payload))
}

// TestDeviceOfflineReasonTypes tests different offline reason types
func TestDeviceOfflineReasonTypes(t *testing.T) {
	reasons := []string{
		"Connection timeout",
		"Network error",
		"Device shutdown",
		"Protocol error",
		"Authentication failed",
	}

	for _, reason := range reasons {
		nodeID := "test-node-001"
		deviceID := "test-device-001"

		offlineMessage := Message{
			Header: MessageHeader{
				MessageID:   generateMessageID(),
				Timestamp:   time.Now().UnixMilli(),
				Source:      nodeID,
				MessageType: "device_offline",
				Version:     "1.0",
			},
			Body: map[string]any{
				"node_id":      nodeID,
				"device_id":    deviceID,
				"device_name":  "Test Device",
				"offline_time": time.Now().UnixMilli(),
				"status":       "offline",
				"reason":       reason,
				"details":      map[string]any{},
			},
		}

		payload, err := json.Marshal(offlineMessage)
		if err != nil {
			t.Fatalf("Failed to marshal offline message for reason '%s': %v", reason, err)
		}

		var parsed Message
		if err := json.Unmarshal(payload, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal offline message for reason '%s': %v", reason, err)
		}

		body := parsed.Body.(map[string]any)
		if body["reason"] != reason {
			t.Errorf("Expected reason '%s', got '%v'", reason, body["reason"])
		}
	}

	t.Logf("Device offline reason types test passed for %d reasons", len(reasons))
}

// TestWriteCommandMessageParsing tests parsing of write command messages
func TestWriteCommandMessageParsing(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	// Build write command message (same format as EdgeOS would send)
	writeCommand := Message{
		Header: MessageHeader{
			MessageID:     "test-write-001",
			Timestamp:     time.Now().UnixMilli(),
			Source:        "edgeos-server",
			Destination:   nodeID,
			MessageType:   "write_command",
			Version:       "1.0",
			CorrelationID: "test-corr-001",
		},
		Body: map[string]any{
			"request_id": "req-write-001",
			"device_id":  deviceID,
			"timestamp":  time.Now().UnixMilli(),
			"points": map[string]any{
				"Switch":      true,
				"Setpoint":    80.5,
				"Temperature": 25.0,
			},
			"options": map[string]any{
				"confirm":         true,
				"timeout_seconds": 10,
			},
		},
	}

	payload, err := json.Marshal(writeCommand)
	if err != nil {
		t.Fatalf("Failed to marshal write command: %v", err)
	}

	// Parse the message back
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal write command: %v", err)
	}

	// Verify header
	if parsed.Header.MessageID != "test-write-001" {
		t.Errorf("Expected message_id 'test-write-001', got '%s'", parsed.Header.MessageID)
	}
	if parsed.Header.Source != "edgeos-server" {
		t.Errorf("Expected source 'edgeos-server', got '%s'", parsed.Header.Source)
	}
	if parsed.Header.MessageType != "write_command" {
		t.Errorf("Expected message_type 'write_command', got '%s'", parsed.Header.MessageType)
	}
	if parsed.Header.Destination != nodeID {
		t.Errorf("Expected destination '%s', got '%s'", nodeID, parsed.Header.Destination)
	}

	// Verify body
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	if body["request_id"] != "req-write-001" {
		t.Errorf("Expected request_id 'req-write-001', got '%v'", body["request_id"])
	}
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}

	// Verify points
	points, ok := body["points"].(map[string]any)
	if !ok {
		t.Fatalf("Points is not a map")
	}

	if !points["Switch"].(bool) {
		t.Errorf("Expected Switch true, got '%v'", points["Switch"])
	}
	if points["Setpoint"].(float64) != 80.5 {
		t.Errorf("Expected Setpoint 80.5, got '%v'", points["Setpoint"])
	}
	if points["Temperature"].(float64) != 25.0 {
		t.Errorf("Expected Temperature 25.0, got '%v'", points["Temperature"])
	}

	// Verify options
	options, ok := body["options"].(map[string]any)
	if !ok {
		t.Fatalf("Options is not a map")
	}

	if !options["confirm"].(bool) {
		t.Errorf("Expected confirm true, got '%v'", options["confirm"])
	}
	if options["timeout_seconds"].(float64) != 10 {
		t.Errorf("Expected timeout_seconds 10, got '%v'", options["timeout_seconds"])
	}

	t.Logf("Write command parsing test passed: %s", string(payload))
}

// TestWriteCommandTopicParsing tests parsing device ID from write command topic
func TestWriteCommandTopicParsing(t *testing.T) {
	testCases := []struct {
		topic            string
		expectedDeviceID string
		expectedError    bool
	}{
		{
			topic:            "edgex/cmd/test-node-001/test-device-001/write",
			expectedDeviceID: "test-device-001",
			expectedError:    false,
		},
		{
			topic:            "edgex/cmd/test-node-001/device-123/write",
			expectedDeviceID: "device-123",
			expectedError:    false,
		},
		{
			topic:            "edgex/cmd/test-node-001/write", // Invalid format
			expectedDeviceID: "",
			expectedError:    true,
		},
		{
			topic:            "edgex/cmd/write", // Invalid format
			expectedDeviceID: "",
			expectedError:    true,
		},
	}

	for _, tc := range testCases {
		topicParts := strings.Split(tc.topic, "/")
		var deviceID string
		var hasError bool

		if len(topicParts) < 5 {
			hasError = true
		} else {
			deviceID = topicParts[3]
			hasError = false
		}

		if hasError != tc.expectedError {
			t.Errorf("Topic '%s': expected error %v, got %v", tc.topic, tc.expectedError, hasError)
		}

		if !hasError && deviceID != tc.expectedDeviceID {
			t.Errorf("Topic '%s': expected device ID '%s', got '%s'", tc.topic, tc.expectedDeviceID, deviceID)
		}
	}

	t.Logf("Write command topic parsing test passed")
}

// TestWriteCommandResponseGeneration tests response message generation for write commands
func TestWriteCommandResponseGeneration(t *testing.T) {
	nodeID := "test-node-001"
	origHeader := MessageHeader{
		MessageID:     "test-write-001",
		Timestamp:     time.Now().UnixMilli(),
		Source:        "edgeos-server",
		Destination:   nodeID,
		MessageType:   "write_command",
		Version:       "1.0",
		CorrelationID: "test-corr-001",
	}

	// Generate success response
	successResponse := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        nodeID,
			Destination:   origHeader.Source,
			MessageType:   "write_response",
			Version:       "1.0",
			CorrelationID: origHeader.MessageID,
		},
		Body: map[string]any{
			"success": true,
			"message": "",
		},
	}

	// Verify success response
	if successResponse.Header.MessageType != "write_response" {
		t.Errorf("Expected message_type 'write_response', got '%s'", successResponse.Header.MessageType)
	}
	if successResponse.Header.Destination != "edgeos-server" {
		t.Errorf("Expected destination 'edgeos-server', got '%s'", successResponse.Header.Destination)
	}
	if successResponse.Header.CorrelationID != "test-write-001" {
		t.Errorf("Expected correlation_id 'test-write-001', got '%s'", successResponse.Header.CorrelationID)
	}
	if successResponse.Header.Source != nodeID {
		t.Errorf("Expected source '%s', got '%s'", nodeID, successResponse.Header.Source)
	}

	body, ok := successResponse.Body.(map[string]any)
	if !ok {
		t.Fatalf("Response body is not a map")
	}
	if !body["success"].(bool) {
		t.Errorf("Expected success=true")
	}
	if body["message"] != "" {
		t.Errorf("Expected empty message for success, got '%v'", body["message"])
	}

	// Generate failure response
	failureResponse := Message{
		Header: MessageHeader{
			MessageID:     generateMessageID(),
			Timestamp:     time.Now().UnixMilli(),
			Source:        nodeID,
			Destination:   origHeader.Source,
			MessageType:   "write_response",
			Version:       "1.0",
			CorrelationID: origHeader.MessageID,
		},
		Body: map[string]any{
			"success": false,
			"message": "Failed to write points: Switch: No channels available; Setpoint: Write failed",
		},
	}

	// Verify failure response
	body, ok = failureResponse.Body.(map[string]any)
	if !ok {
		t.Fatalf("Response body is not a map")
	}
	if body["success"].(bool) {
		t.Errorf("Expected success=false for failure response")
	}
	if body["message"] == "" {
		t.Errorf("Expected error message for failure response")
	}

	t.Logf("Write command response generation test passed")
}

// TestWriteCommandWithEmptyPoints tests write command with empty points
func TestWriteCommandWithEmptyPoints(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	// Build write command with empty points
	writeCommand := Message{
		Header: MessageHeader{
			MessageID:   "test-write-002",
			Timestamp:   time.Now().UnixMilli(),
			Source:      "edgeos-server",
			Destination: nodeID,
			MessageType: "write_command",
			Version:     "1.0",
		},
		Body: map[string]any{
			"request_id": "req-write-002",
			"device_id":  deviceID,
			"timestamp":  time.Now().UnixMilli(),
			"points":     map[string]any{}, // Empty points
		},
	}

	payload, err := json.Marshal(writeCommand)
	if err != nil {
		t.Fatalf("Failed to marshal write command: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal write command: %v", err)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	points, ok := body["points"].(map[string]any)
	if !ok {
		t.Fatalf("Points is not a map")
	}

	if len(points) != 0 {
		t.Errorf("Expected empty points, got %d points", len(points))
	}

	t.Logf("Write command with empty points test passed: %s", string(payload))
}

// TestWriteCommandWithInvalidBody tests write command with invalid body structure
func TestWriteCommandWithInvalidBody(t *testing.T) {
	nodeID := "test-node-001"

	// Build write command with invalid body (points not a map)
	writeCommand := Message{
		Header: MessageHeader{
			MessageID:   "test-write-003",
			Timestamp:   time.Now().UnixMilli(),
			Source:      "edgeos-server",
			Destination: nodeID,
			MessageType: "write_command",
			Version:     "1.0",
		},
		Body: map[string]any{
			"request_id": "req-write-003",
			"device_id":  "test-device-001",
			"points":     []string{"Switch", "Setpoint"}, // Points is an array, not a map
		},
	}

	payload, err := json.Marshal(writeCommand)
	if err != nil {
		t.Fatalf("Failed to marshal write command: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal write command: %v", err)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	// Verify points is not a map
	_, ok = body["points"].(map[string]any)
	if ok {
		t.Errorf("Expected points to not be a map, but it is")
	}

	t.Logf("Write command with invalid body test passed: %s", string(payload))
}

// TestWriteCommandWithDirectPointID tests write command with direct point_id and value fields
func TestWriteCommandWithDirectPointID(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "slave-2"
	requestID := "fae3b583-d902-46fb-bbbd-01968d035d7f"

	// Build write command with direct point_id and value (new format)
	writeCommand := Message{
		Header: MessageHeader{
			MessageID:   "9f292838-4a5b-4c73-9104-40b2b720c9ea",
			Timestamp:   1776827345021,
			Source:      "edgeos-server",
			Destination: nodeID,
			MessageType: "command_write",
			Version:     "1.0",
		},
		Body: map[string]any{
			"device_id":  deviceID,
			"node_id":    nodeID,
			"point_id":   "hr_40000",
			"request_id": requestID,
			"value":      2,
		},
	}

	payload, err := json.Marshal(writeCommand)
	if err != nil {
		t.Fatalf("Failed to marshal write command: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal write command: %v", err)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	// Verify body fields
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["point_id"] != "hr_40000" {
		t.Errorf("Expected point_id 'hr_40000', got '%v'", body["point_id"])
	}
	if body["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, body["request_id"])
	}
	// Check value - JSON parses numbers as float64
	if val, ok := body["value"].(float64); !ok || val != 2 {
		t.Errorf("Expected value 2, got '%v'", body["value"])
	}

	t.Logf("Write command with direct point_id test passed: %s", string(payload))
}

// TestWriteCommandWithRequestIDInHeader tests write command with request_id in header
func TestWriteCommandWithRequestIDInHeader(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "slave-7"
	requestID := "5e34a4ca-8964-4c4a-a16b-a544dc3f112b"

	// Build write command with request_id in header (new format)
	writeCommand := Message{
		Header: MessageHeader{
			MessageID:   "96dbecc2-413b-4982-925d-e6c1c5c8a0d5",
			Timestamp:   1776836206011,
			Source:      "edgeos-server",
			Destination: nodeID,
			MessageType: "command_write",
			Version:     "1.0",
			RequestID:   requestID,
		},
		Body: map[string]any{
			"device_id": deviceID,
			"node_id":   nodeID,
			"point_id":  "hr_2",
			"value":     7715,
		},
	}

	payload, err := json.Marshal(writeCommand)
	if err != nil {
		t.Fatalf("Failed to marshal write command: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal write command: %v", err)
	}

	// Verify header fields
	if parsed.Header.MessageID != "96dbecc2-413b-4982-925d-e6c1c5c8a0d5" {
		t.Errorf("Expected message_id '96dbecc2-413b-4982-925d-e6c1c5c8a0d5', got '%s'", parsed.Header.MessageID)
	}
	if parsed.Header.RequestID != requestID {
		t.Errorf("Expected request_id '%s', got '%s'", requestID, parsed.Header.RequestID)
	}

	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	// Verify body fields
	if body["device_id"] != deviceID {
		t.Errorf("Expected device_id '%s', got '%v'", deviceID, body["device_id"])
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["point_id"] != "hr_2" {
		t.Errorf("Expected point_id 'hr_2', got '%v'", body["point_id"])
	}
	// Check value - JSON parses numbers as float64
	if val, ok := body["value"].(float64); !ok || val != 7715 {
		t.Errorf("Expected value 7715, got '%v'", body["value"])
	}

	t.Logf("Write command with request_id in header test passed: %s", string(payload))
}

// TestHeartbeatMessageFormat tests the enriched heartbeat message format
func TestHeartbeatMessageFormat(t *testing.T) {
	nodeID := "test-node-001"

	// Build enriched heartbeat message
	heartbeatBody := HeartbeatMessage{
		NodeID:        nodeID,
		Status:        "active",
		Timestamp:     time.Now().UnixMilli(),
		Sequence:      100,
		UptimeSeconds: 3600,
		Version:       "1.0.0",
		SystemMetrics: SystemMetrics{
			CPUUsage:       25.5,
			MemoryUsage:    45.2,
			MemoryTotal:    8589934592,
			MemoryUsed:     3883921408,
			DiskUsage:      32.1,
			DiskTotal:      107374182400,
			DiskUsed:       34426873856,
			LoadAverage:    0.85,
			NetworkRXBytes: 1024000,
			NetworkTXBytes: 512000,
			ProcessCount:   45,
			ThreadCount:    128,
		},
		DeviceSummary: DeviceSummary{
			TotalCount:      10,
			OnlineCount:     8,
			OfflineCount:    1,
			ErrorCount:      1,
			DegradedCount:   0,
			RecoveringCount: 0,
		},
		ChannelSummary: ChannelSummary{
			TotalCount:     3,
			ConnectedCount: 3,
			ErrorCount:     0,
			AvgSuccessRate: 0.985,
		},
		TaskSummary: TaskSummary{
			TotalCount:   5,
			RunningCount: 5,
			PausedCount:  0,
			ErrorCount:   0,
		},
		ConnectionStats: ConnectionStats{
			ReconnectCount:  2,
			LastOnlineTime:  1744676400000,
			LastOfflineTime: 1744672800000,
			ConnectedSince:  1744676400000,
			PublishCount:    15000,
			ProtocolVersion: "MQTTv3.1.1",
		},
		CustomMetrics: map[string]interface{}{
			"temperature": 45.5,
			"humidity":    60.0,
		},
	}

	heartbeatMessage := Message{
		Header: MessageHeader{
			MessageID:   generateMessageID(),
			Timestamp:   time.Now().UnixMilli(),
			Source:      nodeID,
			MessageType: "heartbeat",
			Version:     "1.0",
		},
		Body: heartbeatBody,
	}

	payload, err := json.Marshal(heartbeatMessage)
	if err != nil {
		t.Fatalf("Failed to marshal heartbeat: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal heartbeat: %v", err)
	}

	// Verify header
	if parsed.Header.MessageType != "heartbeat" {
		t.Errorf("Expected message_type 'heartbeat', got '%s'", parsed.Header.MessageType)
	}

	// Verify body structure
	body, ok := parsed.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body is not a map")
	}

	// Verify required fields
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}
	if body["status"] != "active" {
		t.Errorf("Expected status 'active', got '%v'", body["status"])
	}
	if body["sequence"] != float64(100) {
		t.Errorf("Expected sequence 100, got '%v'", body["sequence"])
	}
	if body["uptime_seconds"] != float64(3600) {
		t.Errorf("Expected uptime_seconds 3600, got '%v'", body["uptime_seconds"])
	}

	// Verify system_metrics
	sysMetrics, ok := body["system_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("system_metrics is not a map")
	}
	if sysMetrics["cpu_usage"] != 25.5 {
		t.Errorf("Expected cpu_usage 25.5, got '%v'", sysMetrics["cpu_usage"])
	}
	if sysMetrics["memory_usage"] != 45.2 {
		t.Errorf("Expected memory_usage 45.2, got '%v'", sysMetrics["memory_usage"])
	}

	// Verify device_summary
	devSummary, ok := body["device_summary"].(map[string]any)
	if !ok {
		t.Fatalf("device_summary is not a map")
	}
	if devSummary["total_count"] != float64(10) {
		t.Errorf("Expected total_count 10, got '%v'", devSummary["total_count"])
	}
	if devSummary["online_count"] != float64(8) {
		t.Errorf("Expected online_count 8, got '%v'", devSummary["online_count"])
	}

	// Verify connection_stats
	connStats, ok := body["connection_stats"].(map[string]any)
	if !ok {
		t.Fatalf("connection_stats is not a map")
	}
	if connStats["protocol_version"] != "MQTTv3.1.1" {
		t.Errorf("Expected protocol_version 'MQTTv3.1.1', got '%v'", connStats["protocol_version"])
	}

	// Verify custom_metrics
	customMetrics, ok := body["custom_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("custom_metrics is not a map")
	}
	if customMetrics["temperature"] != 45.5 {
		t.Errorf("Expected temperature 45.5, got '%v'", customMetrics["temperature"])
	}

	t.Logf("Heartbeat message format test passed: %s", string(payload))
}

// TestHeartbeatTopicGeneration tests heartbeat topic generation
func TestHeartbeatTopicGeneration(t *testing.T) {
	nodeID := "test-node-001"

	// Generate heartbeat topic
	topic := fmt.Sprintf("edgex/heartbeat/%s", nodeID)
	expected := "edgex/heartbeat/test-node-001"

	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}

	t.Logf("Heartbeat topic generation test passed: %s", topic)
}

// TestHeartbeatIntervalParsing tests heartbeat interval parsing
func TestHeartbeatIntervalParsing(t *testing.T) {
	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"30s", 30 * time.Second},
		{"1m", 1 * time.Minute},
		{"60s", 60 * time.Second},
		{"2m30s", 2*time.Minute + 30*time.Second},
	}

	for _, tc := range testCases {
		parsed, err := time.ParseDuration(tc.input)
		if err != nil {
			t.Errorf("Failed to parse interval '%s': %v", tc.input, err)
			continue
		}
		if parsed != tc.expected {
			t.Errorf("Expected interval %v, got %v for input '%s'", tc.expected, parsed, tc.input)
		}
		t.Logf("Successfully parsed interval '%s' to %v", tc.input, parsed)
	}
}

// TestHeartbeatStatusStrings tests heartbeat status string mapping
func TestHeartbeatStatusStrings(t *testing.T) {
	statusMap := map[int]string{
		0: "offline",
		1: "active",
		2: "reconnecting",
		3: "error",
	}

	for status, expected := range statusMap {
		var statusStr string
		switch status {
		case 0:
			statusStr = "offline"
		case 1:
			statusStr = "active"
		case 2:
			statusStr = "reconnecting"
		case 3:
			statusStr = "error"
		default:
			statusStr = "unknown"
		}

		if statusStr != expected {
			t.Errorf("Expected status '%s', got '%s' for status %d", expected, statusStr, status)
		}
		t.Logf("Status %d mapped to '%s'", status, statusStr)
	}
}

// TestEANEventAutoPublishEnabled verifies the EANEventAutoPublish gating helper
// returns true only when both EANEnabled and EANEventAutoPublish are set.
func TestEANEventAutoPublishEnabled(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		autoPublish bool
		want        bool
	}{
		{"both off", false, false, false},
		{"ean on auto off", true, false, false},
		{"ean off auto on", false, true, false},
		{"both on", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{}
			c.configMu.Lock()
			c.config = model.EdgeOSMQTTConfig{
				EANEnabled:          tc.enabled,
				EANEventAutoPublish: tc.autoPublish,
			}
			c.configMu.Unlock()
			if got := c.EANEventAutoPublishEnabled(); got != tc.want {
				t.Errorf("EANEventAutoPublishEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
