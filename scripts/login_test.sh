#!/bin/bash
NONCE=$(curl -s http://127.0.0.1:8080/api/auth/nonce | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['nonce'])")
echo "Nonce: $NONCE"

HASHES=(
  "a9f69237484b481739bfa7f1c4fc6ac61a16aedd8d641e9988c38ee23dc2b128"
  "96fcac5d110b91137d0faadcb4035c07fcc34a4c19f0fe3ec5553bbeb5577322"
  "4773ffde95b94a50812eacd6918269f856fca1b80bdc55ed9fbabfae30357c15"
  "a50ec2bd1bdb9a421de2919473c557a21e1a12c3ed1866a408555d121e6ed692"
  "9da6f88c3aafd6bf2613dd13a7516878daf718c08e9b703c1ea552e2cf1b1409"
  "a14092455098e5fa07900845da607f9b851dd3380b39223d37b1843831e7a3b4"
)

NAMES=("admin" "admin123" "12345678" "edgeCore" "edgeCore123" "password")

for i in "${!HASHES[@]}"; do
  h="${HASHES[$i]}"
  name="${NAMES[$i]}"
  result=$(curl -s -X POST http://127.0.0.1:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d "{\"loginFlag\":true,\"loginType\":\"local\",\"data\":{\"username\":\"admin\",\"password\":\"$h\",\"nonce\":\"$NONCE\"}}")
  echo "$name -> $result"
done