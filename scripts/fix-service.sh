#!/bin/bash
# fix-service.sh — universal fix dispatcher
# Usage: fix-service.sh <service> [--quiet]
# Services: node bridge docker:tor docker:smp docker:xftp docker:coturn docker:all
#           silver radio economy chat all
# Cycle:  CHECK → DIAGNOSE → FIX → RECHECK → ALERT
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="$HOME/.local/share/ParanoidX.logs/fix.log"
mkdir -p "$(dirname "$LOG")"
API="http://localhost:8080"
QUIET=false
[ "${2:-}" = "--quiet" ] && QUIET=true

log()  { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$LOG"; }
alert(){ local m="$1" u="${2:-normal}"; log "ALERT [$u]: $m"; notify-send --urgency="$u" "ParanoidX FIX" "$m" 2>/dev/null||true; }
q()    { "$@" >/dev/null 2>&1; }
api()  { curl -sf --max-time 5 "$API$1" 2>/dev/null || echo '{}'; }

check_endpoint() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$API$1" 2>/dev/null || echo "000")
  [ "$code" = "200" ]
}

check_docker_container() {
  local name="$1"
  local state
  state=$(docker inspect --format='{{.State.Status}}' "$name" 2>/dev/null || echo "missing")
  [ "$state" = "running" ]
}

notify_result() {
  local svc="$1" status="$2" detail="$3"
  if [ "$status" = "OK" ]; then
    $QUIET || alert "✓ $svc: $detail" "normal"
  else
    alert "✗ $svc: $detail — manual intervention needed" "critical"
  fi
}

do_node() {
  log "● Fix node: checking..."
  if check_endpoint "/api/health"; then
    log "  Node is alive"
    return 0
  fi
  log "  Node DOWN — restarting"
  alert "Node DOWN — restarting..." "critical"
  pkill -x ParanoidX 2>/dev/null || true
  sleep 1
  if [ -f "$HOME/bin/ParanoidX" ]; then
    nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
      >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
    disown
    sleep 3
  fi
  if check_endpoint "/api/health"; then
    notify_result "node" "OK" "restarted successfully"
    return 0
  else
    notify_result "node" "FAIL" "restart failed — check logs"
    return 1
  fi
}

do_bridge() {
  log "● Fix bridge: checking..."
  local data
  data=$(api "/api/chat/bridge-health")
  local connected
  connected=$(echo "$data" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('connected', d.get('bridge_connected', False)))" 2>/dev/null || echo "False")
  if [ "$connected" = "True" ]; then
    log "  Bridge connected"
    return 0
  fi
  log "  Bridge disconnected — restarting node (bridge embedded)"
  alert "Bridge DISCONNECTED — restarting node..." "critical"
  pkill -x ParanoidX 2>/dev/null || true
  sleep 2
  if [ -f "$HOME/bin/ParanoidX" ]; then
    nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
      >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
    disown
    sleep 5
  fi
  local data2
  data2=$(api "/api/chat/bridge-health")
  local connected2
  connected2=$(echo "$data2" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('connected', d.get('bridge_connected', False)))" 2>/dev/null || echo "False")
  if [ "$connected2" = "True" ]; then
    notify_result "bridge" "OK" "reconnected after node restart"
    return 0
  else
    notify_result "bridge" "FAIL" "could not reconnect — check bridge config"
    return 1
  fi
}

do_docker() {
  local container="$1"
  local full_name
  case "$container" in
    tor)    full_name="simplex-node-tor" ;;
    smp)    full_name="simplex-node-smp-server" ;;
    xftp)   full_name="simplex-node-xftp-server" ;;
    coturn) full_name="simplex-node-coturn" ;;
    all)    full_name="all" ;;
    *)      log "  Unknown container: $container"; return 1 ;;
  esac
  if [ "$full_name" = "all" ]; then
    local ok=true
    for c in tor smp xftp coturn; do
      do_docker "$c" || ok=false
    done
    $ok && notify_result "docker:all" "OK" "all containers healthy" \
          || notify_result "docker:all" "FAIL" "some containers still down"
    return 0
  fi
  log "  Checking $full_name..."
  if check_docker_container "$full_name"; then
    log "  $full_name running"
    return 0
  fi
  log "  $full_name DOWN — restarting"
  alert "$full_name DOWN — restarting..." "critical"
  local compose_dir="$HOME/ParanoidX/docker"
  if [ -f "$compose_dir/docker-compose.yml" ]; then
    (cd "$compose_dir" && docker-compose restart "$full_name" 2>/dev/null) || \
      (cd "$compose_dir" && docker-compose up -d "$full_name" 2>/dev/null) || true
    sleep 3
  fi
  if check_docker_container "$full_name"; then
    notify_result "docker:$container" "OK" "$full_name restarted"
    return 0
  else
    notify_result "docker:$container" "FAIL" "$full_name restart failed"
    return 1
  fi
}

do_silver() {
  log "● Fix silver oracle: checking..."
  local data
  data=$(api "/api/economy/oracle")
  local price
  price=$(echo "$data" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('price', d.get('xag', 0)))" 2>/dev/null || echo "0")
  if [ "$(echo "$price > 0" | bc 2>/dev/null || echo 0)" = "1" ]; then
    log "  Silver price: $price"
    return 0
  fi
  log "  Silver oracle silent — restarting node (oracle embedded)"
  alert "Silver oracle DOWN — restarting node..." "warning"
  pkill -x ParanoidX 2>/dev/null || true
  sleep 2
  if [ -f "$HOME/bin/ParanoidX" ]; then
    nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
      >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
    disown
    sleep 5
  fi
  local data2
  data2=$(api "/api/economy/oracle")
  local price2
  price2=$(echo "$data2" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('price', d.get('xag', 0)))" 2>/dev/null || echo "0")
  if [ "$(echo "$price2 > 0" | bc 2>/dev/null || echo 0)" = "1" ]; then
    notify_result "silver" "OK" "oracle restored after restart"
    return 0
  else
    notify_result "silver" "FAIL" "oracle still silent — check gold-api.com key"
    return 1
  fi
}

do_radio() {
  log "● Fix radio: checking..."
  if check_endpoint "/api/radio/ai-content"; then
    log "  Radio online"
    return 0
  fi
  log "  Radio offline — restarting node"
  alert "Radio DOWN — restarting node..." "warning"
  pkill -x ParanoidX 2>/dev/null || true
  sleep 2
  if [ -f "$HOME/bin/ParanoidX" ]; then
    nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
      >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
    disown
    sleep 5
  fi
  if check_endpoint "/api/radio/ai-content"; then
    notify_result "radio" "OK" "restored after restart"
    return 0
  else
    notify_result "radio" "FAIL" "still offline after restart"
    return 1
  fi
}

do_economy() {
  log "● Fix economy: checking..."
  local data
  data=$(api "/api/economy/oracle")
  local price
  price=$(echo "$data" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('price', d.get('xag', 0)))" 2>/dev/null || echo "0")
  if [ "$(echo "$price > 0" | bc 2>/dev/null || echo 0)" = "1" ]; then
    log "  Economy healthy (silver=$price)"
    # Also check treasury
    if check_endpoint "/api/treasury/state"; then
      return 0
    fi
    log "  Treasury endpoint not responding"
    alert "Treasury DOWN — restarting node..." "warning"
    pkill -x ParanoidX 2>/dev/null || true
    sleep 3
    if [ -f "$HOME/bin/ParanoidX" ]; then
      nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
        >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
      disown
      sleep 5
    fi
    if check_endpoint "/api/treasury/state"; then
      notify_result "economy" "OK" "treasury restored"
      return 0
    fi
    notify_result "economy" "FAIL" "treasury still down"
    return 1
  fi
  log "  Economy issue — silver oracle silent"
  notify_result "economy" "FAIL" "silver oracle down, economy affected"
  return 1
}

do_chat() {
  log "● Fix chat: checking..."
  if check_endpoint "/api/chat/status"; then
    local data
    data=$(api "/api/chat/status")
    local count
    count=$(echo "$data" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('message_count', d.get('msg_count', 0)))" 2>/dev/null || echo "0")
    log "  Chat active ($count messages)"
    return 0
  fi
  log "  Chat endpoint down — restarting node"
  alert "Chat DOWN — restarting node..." "critical"
  pkill -x ParanoidX 2>/dev/null || true
  sleep 3
  if [ -f "$HOME/bin/ParanoidX" ]; then
    nohup "$HOME/bin/ParanoidX" -config "$HOME/.local/share/simplex-node/simplex-node.json" \
      >> "$HOME/.local/share/ParanoidX.logs/dashboard.log" 2>&1 &
    disown
    sleep 5
  fi
  if check_endpoint "/api/chat/status"; then
    notify_result "chat" "OK" "restored after restart"
    return 0
  else
    notify_result "chat" "FAIL" "still down — check bridge + logs"
    return 1
  fi
}

do_all() {
  log "═══ FULL SYSTEM FIX ═══"
  local failed=0
  do_node        || ((failed++))
  do_bridge      || ((failed++))
  do_docker "all" || ((failed++))
  do_silver      || ((failed++))
  do_radio       || ((failed++))
  do_economy     || ((failed++))
  do_chat        || ((failed++))
  log "═══ Full fix complete: $failed services failed ═══"
  if [ "$failed" -eq 0 ]; then
    notify_result "system" "OK" "all services healthy"
  else
    notify_result "system" "FAIL" "$failed services still have issues"
  fi
  return $failed
}

# ── Dispatch ──
SERVICE="${1:-help}"
case "$SERVICE" in
  node)              do_node ;;
  bridge)            do_bridge ;;
  docker:tor)        do_docker "tor" ;;
  docker:smp)        do_docker "smp" ;;
  docker:xftp)       do_docker "xftp" ;;
  docker:coturn)     do_docker "coturn" ;;
  docker:all)        do_docker "all" ;;
  silver)            do_silver ;;
  radio)             do_radio ;;
  economy)           do_economy ;;
  chat)              do_chat ;;
  all)               do_all ;;
  help|--help|-h)
    echo "Usage: fix-service.sh <service> [--quiet]"
    echo "Services: node bridge docker:tor docker:smp docker:xftp docker:coturn docker:all"
    echo "          silver radio economy chat all"
    echo "Cycle: CHECK → DIAGNOSE → FIX → RECHECK → desktop notification"
    ;;
  *)
    log "ERROR: Unknown service '$SERVICE'"
    echo "Usage: fix-service.sh <service>"
    echo "Try: node bridge docker:tor silver radio economy chat all"
    exit 1
    ;;
esac
