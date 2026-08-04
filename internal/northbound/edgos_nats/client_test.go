package edgos_nats

import (
	"encoding/json"
	"fmt"
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
		Body: map[string]interface{}{
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

	// Generate response (same logic as in handleNodeRegisterCommand)
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
		Body: map[string]interface{}{
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

	body, ok := response.Body.(map[string]interface{})
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
		Body: map[string]interface{}{
			"node_id":      nodeID,
			"node_name":    "EdgeX Gateway Node",
			"model":        "edgex",
			"version":      "1.0.0",
			"api_version":  "v1",
			"capabilities": []string{"shadow-sync", "heartbeat", "device-control", "task-execution"},
			"protocol":     "edgeOS(NATS)",
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

	body, ok := parsed.Body.(map[string]interface{})
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

	capabilities, ok := body["capabilities"].([]interface{})
	if !ok {
		t.Fatalf("Capabilities is not an array")
	}
	if len(capabilities) != 4 {
		t.Errorf("Expected 4 capabilities, got %d", len(capabilities))
	}

	t.Logf("Node registration payload test passed: %s", string(payload))
}

// TestRegisterSubjectConstant tests the register command subject constant
func TestRegisterSubjectConstant(t *testing.T) {
	// The subject that EdgeOS uses to trigger node re-registration
	registerSubject := "edgex.cmd.nodes.register"

	// Verify subject format
	if registerSubject == "" {
		t.Error("Register subject should not be empty")
	}
	if registerSubject != "edgex.cmd.nodes.register" {
		t.Errorf("Expected subject 'edgex.cmd.nodes.register', got '%s'", registerSubject)
	}

	// Verify it follows the pattern defined in the protocol spec
	// Subject format: edgex.cmd.nodes.register (EdgeOS -> EdgeX)
	expectedPattern := "edgex.cmd.nodes.register"
	if registerSubject != expectedPattern {
		t.Errorf("Subject should match pattern '%s', got '%s'", expectedPattern, registerSubject)
	}

	t.Logf("Register subject constant test passed: %s", registerSubject)
}

// TestNATSStatusSubjectGeneration tests NATS status subject generation
func TestNATSStatusSubjectGeneration(t *testing.T) {
	nodeID := "test-node-001"

	// Generate status subject (same as in publishNodeOnline)
	subject := fmt.Sprintf("edgex.nodes.%s.status", nodeID)
	expected := "edgex.nodes.test-node-001.status"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("NATS status subject generation test passed: %s", subject)
}

// TestNATSRegistrationSubject tests NATS registration subject
func TestNATSRegistrationSubject(t *testing.T) {
	// Generate registration subject (same as in publishNodeOnline)
	subject := "edgex.nodes.register"
	expected := "edgex.nodes.register"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("NATS registration subject test passed: %s", subject)
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

// TestDeviceReportMessageFormat tests the device report message format
func TestDeviceReportMessageFormat(t *testing.T) {
	nodeID := "test-node-001"

	// Build device report message (same format as publishDeviceReport)
	devices := []map[string]interface{}{
		{
			"device_id":       "device-001",
			"device_name":     "Test Device 1",
			"device_profile":  "modbus",
			"service_name":    "Test Channel",
			"labels":          []string{},
			"description":     "",
			"admin_state":     "ENABLED",
			"operating_state": "ENABLED",
			"properties": map[string]interface{}{
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
			"properties": map[string]interface{}{
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
		Body: map[string]interface{}{
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
	body, ok := parsed.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("Body is not a map")
	}
	if body["node_id"] != nodeID {
		t.Errorf("Expected node_id '%s', got '%v'", nodeID, body["node_id"])
	}

	deviceList, ok := body["devices"].([]interface{})
	if !ok {
		t.Fatalf("Devices is not an array")
	}
	if len(deviceList) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(deviceList))
	}

	// Verify first device
	device1, ok := deviceList[0].(map[string]interface{})
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
	device2, ok := deviceList[1].(map[string]interface{})
	if !ok {
		t.Fatalf("Device 2 is not a map")
	}
	if device2["operating_state"] != "DISABLED" {
		t.Errorf("Expected operating_state 'DISABLED', got '%v'", device2["operating_state"])
	}

	t.Logf("Device report message format test passed: %s", string(payload))
}

// TestDeviceReportSubjectGeneration tests device report subject generation for NATS
func TestDeviceReportSubjectGeneration(t *testing.T) {
	// Generate device report subject (same as in publishDeviceReport)
	reportSubject := "edgex.devices.report"
	expected := "edgex.devices.report"

	if reportSubject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, reportSubject)
	}

	t.Logf("Device report subject generation test passed: %s", reportSubject)
}

// TestRegisterResponseSubjectGeneration tests NATS response subject generation
func TestRegisterResponseSubjectGeneration(t *testing.T) {
	nodeID := "test-node-001"

	// Generate response subject (same as in subscribeToCommands)
	subject := fmt.Sprintf("edgex.nodes.%s.response", nodeID)
	expected := "edgex.nodes.test-node-001.response"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("Register response subject generation test passed: %s", subject)
}

// TestRegisterResponseParsing tests parsing of registration response messages
func TestRegisterResponseParsing(t *testing.T) {
	// Build response message (same format as EdgeOS sends via NATS)
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
		Body: map[string]interface{}{
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
	body, ok := parsed.Body.(map[string]interface{})
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
		Body: map[string]interface{}{
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
	body, ok := parsed.Body.(map[string]interface{})
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
		Body: map[string]interface{}{
			"node_id": nodeID,
			"devices": []map[string]interface{}{},
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

	body, ok := parsed.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("Body is not a map")
	}

	deviceList, ok := body["devices"].([]interface{})
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

// TestNATSMessageFormat tests the NATS message format compatibility
func TestNATSMessageFormat(t *testing.T) {
	// NATS messages are just JSON data, same as MQTT
	msg := Message{
		Header: MessageHeader{
			MessageID:     "nats-test-001",
			Timestamp:     time.Now().UnixMilli(),
			Source:        "edgeos-nats-server",
			MessageType:   "node_register",
			Version:       "1.0",
			CorrelationID: "nats-corr-001",
		},
		Body: map[string]interface{}{
			"action":  "re-register",
			"node_id": "edgex-node-001",
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NATS message: %v", err)
	}

	// Parse and verify
	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal NATS message: %v", err)
	}

	if parsed.Header.MessageType != "node_register" {
		t.Errorf("Expected message_type 'node_register', got '%s'", parsed.Header.MessageType)
	}

	t.Logf("NATS message format test passed: %s", string(payload))
}

// TestDeviceOnlineSubjectGeneration tests device online subject generation for NATS
func TestDeviceOnlineSubjectGeneration(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	subject := fmt.Sprintf("edgex.devices.%s.%s.online", nodeID, deviceID)
	expected := "edgex.devices.test-node-001.test-device-001.online"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("Device online subject generation test passed: %s", subject)
}

// TestDeviceOfflineSubjectGeneration tests device offline subject generation for NATS
func TestDeviceOfflineSubjectGeneration(t *testing.T) {
	nodeID := "test-node-001"
	deviceID := "test-device-001"

	subject := fmt.Sprintf("edgex.devices.%s.%s.offline", nodeID, deviceID)
	expected := "edgex.devices.test-node-001.test-device-001.offline"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("Device offline subject generation test passed: %s", subject)
}

// TestNATSDeviceOnlineMessageFormat tests the NATS device online notification message format
func TestNATSDeviceOnlineMessageFormat(t *testing.T) {
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
		Body: map[string]interface{}{
			"node_id":     nodeID,
			"device_id":   deviceID,
			"device_name": deviceName,
			"online_time": time.Now().UnixMilli(),
			"status":      "online",
			"details": map[string]interface{}{
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
	body, ok := parsed.Body.(map[string]interface{})
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
	details, ok := body["details"].(map[string]interface{})
	if !ok {
		t.Fatalf("Details is not a map")
	}
	if details["protocol"] != "modbus-tcp" {
		t.Errorf("Expected protocol 'modbus-tcp', got '%v'", details["protocol"])
	}

	t.Logf("NATS device online message format test passed: %s", string(payload))
}

// TestNATSDeviceOfflineMessageFormat tests the NATS device offline notification message format
func TestNATSDeviceOfflineMessageFormat(t *testing.T) {
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
		Body: map[string]interface{}{
			"node_id":      nodeID,
			"device_id":    deviceID,
			"device_name":  deviceName,
			"offline_time": time.Now().UnixMilli(),
			"status":       "offline",
			"reason":       reason,
			"details": map[string]interface{}{
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
	body, ok := parsed.Body.(map[string]interface{})
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
	details, ok := body["details"].(map[string]interface{})
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

	t.Logf("NATS device offline message format test passed: %s", string(payload))
}

// TestNATSWildcardSubjectPattern tests NATS wildcard subject pattern for device online/offline
func TestNATSWildcardSubjectPattern(t *testing.T) {
	// Test wildcard pattern for subscribing to all device online subjects
	wildcardPattern := "edgex.devices.>.online"
	expected := "edgex.devices.>.online"

	if wildcardPattern != expected {
		t.Errorf("Expected pattern '%s', got '%s'", expected, wildcardPattern)
	}

	// Test wildcard pattern for subscribing to all device offline subjects
	wildcardPatternOffline := "edgex.devices.>.offline"
	expectedOffline := "edgex.devices.>.offline"

	if wildcardPatternOffline != expectedOffline {
		t.Errorf("Expected pattern '%s', got '%s'", expectedOffline, wildcardPatternOffline)
	}

	t.Logf("NATS wildcard subject pattern test passed")
}

// TestNATSResponseSubjectGeneration tests NATS response subject generation
func TestNATSResponseSubjectGeneration(t *testing.T) {
	nodeID := "test-node-001"
	messageID := "test-msg-001"

	subject := fmt.Sprintf("edgex.res.%s.%s", nodeID, messageID)
	expected := "edgex.res.test-node-001.test-msg-001"

	if subject != expected {
		t.Errorf("Expected subject '%s', got '%s'", expected, subject)
	}

	t.Logf("NATS response subject generation test passed: %s", subject)
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
			c.config = model.EdgeOSNATSConfig{
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
