#!/bin/bash
ISLE_HOME="${ISLE_HOME:-$HOME}"  # переносимый дом проекта: переопредели env-ом, дефолт $HOME
# Integration test suite for simplex-node
# Usage: bash scripts/test-integration.sh [--host http://127.0.0.1:8080]

HOST="${1:-http://127.0.0.1:8080}"
PASS=0
FAIL=0
FAILED_TESTS=""

test_endpoint() {
  local name="$1" method="$2" url="$3" expect="$4" extra="$5"
  local resp
  resp=$(curl -s --max-time 5 -X "$method" "$HOST$url" 2>/dev/null)
  if echo "$resp" | grep -q "$expect"; then
    echo "  ✓ $name"
    PASS=$((PASS+1))
  else
    echo "  ✗ $name (expected '$expect', got: ${resp:0:100})"
    FAIL=$((FAIL+1))
    FAILED_TESTS="$FAILED_TESTS  $name\n"
  fi
}

echo "═══════════════════════════════════════"
echo "  simplex-node Integration Tests"
echo "  Host: $HOST"
echo "═══════════════════════════════════════"

echo ""
echo "--- Core API ---"
test_endpoint "version" GET "/api/version" "simplex-node-"
test_endpoint "health" GET "/api/health" '"healthy":true'
test_endpoint "status" GET "/api/status" '"status":"running"'

echo ""
echo "--- Docker ---"
test_endpoint "docker status" GET "/api/admin/docker" '"healthy":true'

echo ""
echo "--- System Metrics ---"
test_endpoint "system metrics" GET "/api/admin/metrics/system" '"cpus"'
test_endpoint "data dir size" GET "/api/admin/metrics/system" '"data_dir"'

echo ""
echo "--- Radio ---"
test_endpoint "stations" GET "/api/radio?action=stations" '"count"'
test_endpoint "formula" GET "/api/radio?action=formula" '"playlist"'
test_endpoint "liberty-voice-en" GET "/api/radio?action=playlist&station=liberty-voice-en" '"count":2'
# Track serve tested separately (binary content)
TRACK_RESP=$(curl -s --max-time 5 -o /dev/null -w "%{http_code}" "$HOST/api/radio/track?path=$ISLE_HOME/.local/share/simplex-node/radio/%D0%9A%D0%BE%D0%BB%D1%91%D1%81%D0%B0%20%D0%B2%D1%80%D0%B0%D0%B7%D0%BD%D0%BE%D1%81%20(1).mp3" 2>/dev/null)
if [ "$TRACK_RESP" = "200" ]; then echo "  ✓ track serve (HTTP 200)"; PASS=$((PASS+1)); else echo "  ✗ track serve (HTTP $TRACK_RESP)"; FAIL=$((FAIL+1)); FAILED_TESTS="$FAILED_TESTS  track serve\n"; fi

echo ""
echo "--- Chat ---"
test_endpoint "chat status" GET "/api/chat/status" '"bridge"'

echo ""
echo "--- Economy ---"
test_endpoint "silver oracle" GET "/api/economy/oracle" '"current_price"'

echo ""
echo "--- Security Headers ---"
HEADERS=$(curl -sI --max-time 5 "$HOST/api/version" 2>/dev/null)
if echo "$HEADERS" | grep -q "Content-Security-Policy"; then echo "  ✓ CSP header"; PASS=$((PASS+1)); else echo "  ✗ CSP header missing"; FAIL=$((FAIL+1)); FAILED_TESTS="$FAILED_TESTS  CSP header\n"; fi
if echo "$HEADERS" | grep -q "Strict-Transport-Security"; then echo "  ✓ HSTS header"; PASS=$((PASS+1)); else echo "  ✗ HSTS header missing"; FAIL=$((FAIL+1)); FAILED_TESTS="$FAILED_TESTS  HSTS header\n"; fi
if echo "$HEADERS" | grep -q "X-Request-Id"; then echo "  ✓ X-Request-ID"; PASS=$((PASS+1)); else echo "  ✗ X-Request-ID missing"; FAIL=$((FAIL+1)); FAILED_TESTS="$FAILED_TESTS  X-Request-ID\n"; fi

echo ""
echo "═══════════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed"
if [ $FAIL -gt 0 ]; then
  echo ""
  echo "  Failed:"
  echo -e "$FAILED_TESTS"
fi
echo "═══════════════════════════════════════"
exit $FAIL
