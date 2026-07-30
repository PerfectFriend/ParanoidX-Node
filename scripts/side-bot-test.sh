#!/bin/bash
# side-bot-test.sh — Live node simplex bot activity test.
# Runs alongside the running node (no port conflict, no isolation).
# Tests all bot-accessible features through the live API.
# Usage: ./scripts/side-bot-test.sh [api-base-url]
#   default: http://127.0.0.1:8080
set -euo pipefail

API="${1:-http://127.0.0.1:8080}"
CURL="curl -s --max-time 8"
PASS=0; FAIL=0

pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
fail() { FAIL=$((FAIL+1)); echo "  ❌ $1: $2"; }
check_field() {
  local label="$1" json="$2" field="$3" expected="$4"
  val=$(echo "$json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$field','__MISSING__'))" 2>/dev/null || echo "__PARSE_ERR__")
  if [ "$val" = "$expected" ]; then
    pass "$label ($field=$val)"
  else
    fail "$label" "expected $expected, got $val"
  fi
}

echo "═══ SIDE BOT TEST — live node at $API ═══"
echo ""

# ── 1. Bridge status ──
echo "─── 1. BRIDGE / BOT CONNECTION ───"
STATUS=$($CURL "$API/api/status" 2>/dev/null || echo '{}')
check_field "status" "$STATUS" "status" "running"
LINK_LEN=$(echo "$STATUS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('island_services_contact','')))" 2>/dev/null || echo 0)
UPTIME=$(echo "$STATUS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('uptime_seconds',0))" 2>/dev/null || echo 0)
echo "  contact link: len=$LINK_LEN $( [ "$LINK_LEN" -gt 50 ] && echo '✅' || echo '❌' )"
echo "  bridge uptime: ${UPTIME}s"
[ "$LINK_LEN" -gt 50 ] && pass "contact link present" || fail "contact link" "too short"

# ── 2. Treasury State (banknote visibility) ──
echo ""
echo "─── 2. TREASURY (wallet response simulation) ───"
STATE=$($CURL "$API/api/treasury/state" 2>/dev/null || echo '{}')
BN_COUNT=$(echo "$STATE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('banknotes',[])))" 2>/dev/null || echo "0")
RWA_COUNT=$(echo "$STATE" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('rwa',[])))" 2>/dev/null || echo "0")
RESERVE=$(echo "$STATE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('current_reserve_ng',0))" 2>/dev/null || echo "0")
echo "  banknotes: $BN_COUNT (expected ≥3)   rwa: $RWA_COUNT   reserve: $RESERVE ng"
if [ "$BN_COUNT" -ge 3 ]; then pass "banknotes visible in treasury"; else fail "banknotes" "got $BN_COUNT, expected ≥3"; fi
if [ "$RWA_COUNT" -ge 1 ]; then pass "RWA visible in treasury"; else fail "RWA" "got $RWA_COUNT, expected ≥1"; fi

# ── 3. Pre-mint (pool available for pack purchases) ──
echo ""
echo "─── 3. PRE-MINT (pack pool) ───"
PREM=$($CURL "$API/api/economy/pre-mint" 2>/dev/null || echo '{}')
PREM_AVAIL=$(echo "$PREM" | python3 -c "import sys,json; print(json.load(sys.stdin).get('available',0))" 2>/dev/null || echo "0")
PREM_TOTAL=$(echo "$PREM" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total',0))" 2>/dev/null || echo "0")
echo "  available: $PREM_AVAIL / $PREM_TOTAL total"
if [ "$PREM_AVAIL" -ge 10 ]; then pass "pre-mint available ≥10"; else fail "pre-mint" "got $PREM_AVAIL available"; fi

# ── 4. Genesis info ──
echo ""
echo "─── 4. GENESIS INFO ───"
GEN=$($CURL "$API/api/genesis/info" 2>/dev/null || echo '{}')
GEN_COUNT=$(echo "$GEN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('count',0))" 2>/dev/null || echo "0")
GEN_HAS=$(echo "$GEN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('has_genesis',False))" 2>/dev/null || echo "false")
echo "  genesis cards: $GEN_COUNT   has_genesis: $GEN_HAS"
if [ "$GEN_COUNT" -ge 1 ]; then pass "genesis card exists"; else fail "genesis" "got $GEN_COUNT cards"; fi

# ── 5. Wallet system (keys + balance) ──
echo ""
echo "─── 5. WALLET (ID / tokenize foundation) ───"
WALLET=$($CURL -X POST "$API/api/wallet/create" 2>/dev/null || echo '{}')
echo "$WALLET" | python3 -c '
import sys,json; d=json.load(sys.stdin)
pk = d.get("pubkey","")
print(f"  created pubkey: {pk[:40]}... ({len(pk)} chars)" if pk else "  ❌ no pubkey")
' 2>/dev/null || fail "wallet create" "python error"

# Clean up test wallet wallet by checking balance
BAL=$(echo "$WALLET" | python3 -c "
import sys,json; d=json.load(sys.stdin)
pk = d.get('pubkey','')
import urllib.request
import urllib.parse
url = '$API/api/wallet/balance?' + urllib.parse.urlencode({'pubkey': pk})
try:
    r = urllib.request.urlopen(url, timeout=5)
    b = json.loads(r.read())
    print(b.get('balance_ng',0))
except:
    print('0')
" 2>/dev/null || echo "0")
echo "  test wallet balance: $BAL ng"

# ── 6. Addresses (all 5 onion services) ──
echo ""
echo "─── 6. ADDRESSES (5 .onion services) ───"
ADDR=$($CURL "$API/api/addresses" 2>/dev/null || echo '{}')
echo "$ADDR" | python3 -c "
import sys,json; d=json.load(sys.stdin)
for k in ['smp','xftp','contact','ice','auditor']:
    v = d.get(k,'')
    ok = '.onion' in v
    print(f'  {k}: {\"✅\" if ok else \"❌\"} ({v[:25]}...)')
" 2>/dev/null || fail "addresses" "python error"

# ── 7. Pack shop availability ──
echo ""
echo "─── 7. PACK SHOP (starter pack mechanic) ───"
PACK_LIST=$($CURL "$API/api/pack/list?pubkey=test-side-bot" 2>/dev/null || echo '{}')
HAS_PACKS=$(echo "$PACK_LIST" | python3 -c "import sys,json; d=json.load(sys.stdin); print('packs' in d)" 2>/dev/null || echo "False")
echo "  pack/list accessible: $( [ "$HAS_PACKS" = "True" ] && echo '✅' || echo '❌' )"
[ "$HAS_PACKS" = "True" ] && pass "pack/list endpoint" || fail "pack/list" "unavailable"

# ── 8. Golden Wheel ──
echo ""
echo "─── 8. GOLDEN WHEEL (daily spin) ───"
if curl -s --max-time 8 "http://127.0.0.1:8080/api/golden-wheel/state?pubkey=side-test-w" | python3 -c "import sys,json; assert json.load(sys.stdin).get('can_spin')==True" 2>/dev/null; then
  pass "golden wheel endpoint"
  echo "  wheel: ✅"
else
  fail "golden wheel" "endpoint or python check failed"
  echo "  wheel: ❌"
fi

echo "─── 9. KNOWN ROLES (king auto-assign) ───"
if [ -f ~/.local/share/simplex-node/known_roles.json ]; then
  ROLES=$(cat ~/.local/share/simplex-node/known_roles.json)
  echo "$ROLES" | python3 -c '
import sys,json; d=json.load(sys.stdin)
king=d.get("king","(none)")
print(f"  king contact: {king}" if king != "(none)" else "  king: not yet assigned (no contact connected)")
' 2>/dev/null
  pass "known_roles file exists"
else
  fail "known_roles" "file not found"
fi

# ── 10. Dashboard log — bridge messages ──
echo ""
echo "─── 10. BRIDGE LOG (recent island events) ───"
LOG=~/.local/share/ParanoidX.logs/dashboard.log
if [ -f "$LOG" ]; then
  BRIDGE_MSGS=$(grep -c "\[island\]" "$LOG" 2>/dev/null || echo 0)
  LAST_CONNECT=$(grep "connected to Island Services" "$LOG" | tail -1 2>/dev/null || echo "(never)")
  echo "  island log entries: $BRIDGE_MSGS"
  echo "  last connect: $LAST_CONNECT"
  if [ "$BRIDGE_MSGS" -ge 3 ]; then pass "bridge activity logged"; else fail "bridge log" "only $BRIDGE_MSGS entries"; fi
else
  fail "dashboard.log" "not found"
fi

# ── 10. Contact file integrity ──
echo ""
echo "─── 11. CONTACT FILE (citizen entry point) ───"
CONTACT=~/.local/share/simplex-node/island_services_contact.txt
if [ -f "$CONTACT" ]; then
  LINK=$(grep "simplex:" "$CONTACT" | head -1 2>/dev/null || echo "")
  if echo "$LINK" | grep -q "simplex:/contact"; then
    pass "valid contact link in file"
  else
    fail "contact link" "no simplex:/contact found"
  fi
  if grep -q "Королевские\|ISLAND ROYAL" "$CONTACT" 2>/dev/null; then
    pass "welcome text in contact file"
  else
    fail "welcome text" "missing"
  fi
else
  fail "contact file" "not found"
fi

echo ""
echo "═══ RESULTS: $PASS passed, $FAIL failed ═══"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
