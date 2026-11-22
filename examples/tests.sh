#!/bin/bash
# Smoke tests for zoom-notifier v2
# Requires: server running on localhost:8888
# Usage: ADMIN_KEY=devkey ZOOM_SECRET=<zoom.webhook_secret> ./tests.sh

BASE_URL="${BASE_URL:-http://localhost:8888}"
ADMIN_KEY="${ADMIN_KEY:-devkey}"
ZOOM_SECRET="${ZOOM_SECRET:?set ZOOM_SECRET to the server zoom.webhook_secret}"

set -e

# post_zoom_webhook FILE — POST a webhook payload signed like Zoom does
# (x-zm-signature = v0=HMAC-SHA256(secret, "v0:{ts}:{body}")).
post_zoom_webhook() {
  local body ts sig
  body=$(cat "$1")
  ts=$(date +%s)
  sig="v0=$(printf 'v0:%s:%s' "$ts" "$body" | openssl dgst -sha256 -hmac "$ZOOM_SECRET" -r | cut -d' ' -f1)"
  curl -s -X POST "$BASE_URL/webhook/zoom" \
    -H "Content-Type: application/json" \
    -H "x-zm-request-timestamp: $ts" \
    -H "x-zm-signature: $sig" \
    --data-binary "$body"
}

echo "=== Health Check ==="
curl -s "$BASE_URL/healthz" | python3 -m json.tool
echo

echo "=== Create Tenant ==="
TENANT_RESP=$(curl -s -X POST "$BASE_URL/api/v1/tenants" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -d @./api/create_tenant_request.json)
echo "$TENANT_RESP" | python3 -m json.tool
TENANT_KEY=$(echo "$TENANT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])")
echo "Tenant API Key: $TENANT_KEY"
echo

echo "=== List Tenants ==="
curl -s "$BASE_URL/api/v1/tenants" \
  -H "Authorization: Bearer $ADMIN_KEY" | python3 -m json.tool
echo

echo "=== Get Tenant ==="
curl -s "$BASE_URL/api/v1/tenants/T-EXAMPLE" \
  -H "Authorization: Bearer $TENANT_KEY" | python3 -m json.tool
echo

echo "=== Create Subscription ==="
curl -s -X POST "$BASE_URL/api/v1/tenants/T-EXAMPLE/subscriptions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TENANT_KEY" \
  -d @./api/create_subscription_request.json | python3 -m json.tool
echo

echo "=== List Subscriptions ==="
curl -s "$BASE_URL/api/v1/tenants/T-EXAMPLE/subscriptions" \
  -H "Authorization: Bearer $TENANT_KEY" | python3 -m json.tool
echo

echo "=== Put IRC Config ==="
curl -s -X PUT "$BASE_URL/api/v1/tenants/T-EXAMPLE/irc" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TENANT_KEY" \
  -d @./api/irc_config_request.json | python3 -m json.tool
echo

echo "=== Zoom Webhook: Participant Joined ==="
post_zoom_webhook ./zoom/participant_joined.json
echo

echo "=== Zoom Webhook: Participant Left ==="
post_zoom_webhook ./zoom/participant_left.json
echo

echo
echo "=== All smoke tests passed ==="
