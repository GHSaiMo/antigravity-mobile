#!/usr/bin/env bash
set -e

PORT=58912
AUTH_FILE=$(mktemp)
export AUTH_STORE_PATH="$AUTH_FILE"

# Start gateway in background
./bin/gateway -port "$PORT" > /tmp/gw_test.log 2>&1 &
GW_PID=$!

cleanup() {
  kill "$GW_PID" 2>/dev/null || true
  rm -f "$AUTH_FILE" /tmp/gw_test.log
}
trap cleanup EXIT

# Wait for gateway to start
sleep 1.5

echo "--- 1. Testing /gateway/status (whitelisted probe) ---"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/gateway/status")
echo "Status code: $STATUS (expected 200)"
[ "$STATUS" -eq 200 ]

echo "--- 2. Testing protected endpoint without token ---"
CODE_NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/gateway/projects")
echo "Status code without token: $CODE_NO_AUTH (expected 401)"
[ "$CODE_NO_AUTH" -eq 401 ]

echo "--- 3. Generating pairing session via Loopback API ---"
PAIR_SESSION=$(curl -s -X POST "http://127.0.0.1:$PORT/api/v1/auth/session")
echo "Pairing Session response: $PAIR_SESSION"
PAIR_CODE=$(echo "$PAIR_SESSION" | grep -o '"code":"[^"]*"' | cut -d'"' -f4)
echo "Extracted pairing code: $PAIR_CODE"
[ -n "$PAIR_CODE" ]

echo "--- 4. Pairing client with pairing code ---"
PAIR_RESP=$(curl -s -X POST "http://127.0.0.1:$PORT/api/v1/auth/pair" \
  -H "Content-Type: application/json" \
  -d "{\"pairing_code\":\"$PAIR_CODE\",\"device_name\":\"Test iPhone 16 Pro\",\"platform\":\"ios\"}")
echo "Pair response: $PAIR_RESP"
TOKEN=$(echo "$PAIR_RESP" | grep -o '"device_token":"[^"]*"' | cut -d'"' -f4)
DEV_ID=$(echo "$PAIR_RESP" | grep -o '"device_id":"[^"]*"' | cut -d'"' -f4)
echo "Issued Device Token: $TOKEN, Device ID: $DEV_ID"
[ -n "$TOKEN" ]
[ -n "$DEV_ID" ]

echo "--- 5. Anti-replay check: pairing again with same code ---"
REPLAY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:$PORT/api/v1/auth/pair" \
  -H "Content-Type: application/json" \
  -d "{\"pairing_code\":\"$PAIR_CODE\",\"device_name\":\"Test iPhone 16 Pro\",\"platform\":\"ios\"}")
echo "Replay pairing status: $REPLAY_STATUS (expected 401)"
[ "$REPLAY_STATUS" -eq 401 ]

echo "--- 6. Accessing protected endpoint with Bearer Token in Header ---"
AUTH_HEADER_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/gateway/projects" \
  -H "Authorization: Bearer $TOKEN")
echo "Protected endpoint with Header: $AUTH_HEADER_STATUS (expected 200)"
[ "$AUTH_HEADER_STATUS" -eq 200 ]

echo "--- 7. Accessing protected endpoint with Token in Query Param (WebSocket simulation) ---"
AUTH_QUERY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/gateway/projects?auth_token=$TOKEN")
echo "Protected endpoint with Query: $AUTH_QUERY_STATUS (expected 200)"
[ "$AUTH_QUERY_STATUS" -eq 200 ]

echo "--- 8. Listing paired devices ---"
DEVICES_LIST=$(curl -s "http://127.0.0.1:$PORT/api/v1/devices")
echo "Devices: $DEVICES_LIST"
echo "$DEVICES_LIST" | grep -q "$DEV_ID"

echo "--- 9. Revoking / Deleting device ---"
DEL_RESP=$(curl -s -X DELETE "http://127.0.0.1:$PORT/api/v1/devices/$DEV_ID")
echo "Delete response: $DEL_RESP"

echo "--- 10. Accessing protected endpoint after revocation ---"
REVOKED_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/gateway/projects" \
  -H "Authorization: Bearer $TOKEN")
echo "Protected endpoint after revocation: $REVOKED_STATUS (expected 401)"
[ "$REVOKED_STATUS" -eq 401 ]

echo "🎉 ALL INTEGRATION TESTS PASSED PERFECTLY!"
