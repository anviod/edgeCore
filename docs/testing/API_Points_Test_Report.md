---
layout: default
title: API Points Response Optimization Test Report
description: edgeCore API Points Test Report
---

# API Points Response Optimization Test Report

## 1. Test Objective
Verify that the API endpoint `/api/channels/:channelId/devices/:deviceId/points` dynamically filters response fields based on the channel protocol to prevent field pollution (e.g., Modbus fields appearing in BACnet responses).

## 2. Test Environment
- **Component**: `internal/server`
- **Test File**: `internal/server/points_api_test.go`
- **Mocking**: `MockDriver` for data reading, `ChannelManager` for device configuration.
- **Authentication**: Valid JWT token generated for testing.

## 3. Test Scenarios

### Scenario A: Modbus TCP Device
- **Input**: GET request for a Modbus device.
- **Expected Output**:
  - `protocol`: "modbus-tcp"
  - `slave_id`: Present (e.g., 1)
  - `register_type`: Present (e.g., "Holding Registers")
  - `function_code`: Present (e.g., 3)

### Scenario B: BACnet IP Device
- **Input**: GET request for a BACnet device.
- **Expected Output**:
  - `protocol`: "bacnet-ip"
  - `slave_id`: **Absent**
  - `register_type`: **Absent**
  - `function_code`: **Absent**

## 4. Test Execution
Command: `go test -v ./internal/server`

### Output Log
```
=== RUN   TestGetDevicePoints_ProtocolFields
--- PASS: TestGetDevicePoints_ProtocolFields (0.01s)
PASS
ok      github.com/anviod/edgeCore/internal/server    1.236s
```

## 5. Conclusion
The API correctly identifies the channel protocol and filters the response fields accordingly.
- **Modbus** responses contain necessary Modbus-specific fields.
- **BACnet** responses are clean and do not contain irrelevant Modbus fields.
- The `protocol` field is correctly added to all responses.

## 6. Code Reference
- Test Implementation: [points_api_test.go](https://github.com/anviod/edgeCore/blob/dev/internal/server/points_api_test.go)
- Server Logic: [server.go](https://github.com/anviod/edgeCore/blob/dev/internal/server/server.go)
