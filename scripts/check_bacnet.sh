#!/bin/bash
echo "=== BACnet Server Logs ==="
journalctl -u edgeCore --no-pager --since "8 hours ago" | grep -i "BACnet server"

echo ""
echo "=== Port listening ==="
ss -tuln | grep -E '47808|47809|47810'

echo ""
echo "=== BACnet Server config ==="
TOKEN=$(python3 /tmp/gen_token.py)
curl -s http://127.0.0.1:8080/api/northbound/config -H "Authorization: Bearer $TOKEN" | python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps(d.get('bacnet_server',[]), indent=2))"

echo ""
echo "=== Recent JSON BACnet logs ==="
journalctl -u edgeCore --no-pager --since "5 minutes ago" | grep '"caller":"server/' | tail -20