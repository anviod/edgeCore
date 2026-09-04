#!/bin/bash
TOKEN=$(python3 /tmp/gen_token.py)

# First, check the current northbound config
echo "=== Current Northbound Config ==="
curl -s http://127.0.0.1:8080/api/northbound/config -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Configure BACnet Server
echo ""
echo "=== Configuring BACnet Server ==="
curl -s -X POST http://127.0.0.1:8080/api/northbound/bacnet_server \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "",
    "name": "BACnet Server",
    "enable": true,
    "mode": "slave",
    "device_name": "edgeCore-Gateway",
    "device_id": 1000,
    "vendor_id": 999,
    "ip": "0.0.0.0",
    "port": 47808,
    "subnet_cidr": 24,
    "max_pdu": 1476,
    "devices": {},
    "virtual_devices": {}
  }' | python3 -m json.tool

echo ""
echo "=== Verify Config ==="
curl -s http://127.0.0.1:8080/api/northbound/config -H "Authorization: Bearer $TOKEN" | python3 -m json.tool