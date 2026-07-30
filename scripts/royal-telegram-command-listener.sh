#!/bin/bash
# Automatic Telegram command listener for royal node control.
# Run via launch-bot-listener.sh (while once + sleep 2).
# After each owner cmd: signal_step_done (exact "шаг (...) завершен..." template).
# Supports plan/build, market, edit (shlex), checkpoint, gobuild, test, etc.

set -euo pipefail

# Centralized paths (reduces hardcodes, portable)
source "$(dirname "$0")/royal-common.sh" 2>/dev/null || true

# Fallbacks if common not present (for safety during transition)
: "${TOKEN_FILE:=${HOME}/.config/royal-bot.token}"
: "${CHAT_FILE:=${HOME}/.config/royal-bot.chat}"
: "${OFFSET_FILE:=${DATA_DIR}/royal_last_update.txt}"
: "${DATA_DIR:=${HOME}/.local/share/simplex-node}"
: "${${PARANOIDX_SRC:-$HOME/ParanoidX}:=${HOME}/simplex-node}"
: "${SCRIPTS_DIR:=$(dirname "$0")}"
: "${PLAN_SNAPSHOT:=$(find "${HOME}/.grok/sessions" -path '*%2Fhome%2Ftomas*' -name plan.md 2>/dev/null | head -1 || echo "${HOME}/.grok/sessions/%2Fhome%2Ftomas/019e79fa-9fa5-7c51-a6d2-52d0f52edddb/plan.md")}"

if [ ! -f "$TOKEN_FILE" ] || [ ! -f "$CHAT_FILE" ]; then
  echo "Config missing"
  exit 1
fi

TOKEN=$(cat "$TOKEN_FILE")
CHAT_ID=$(cat "$CHAT_FILE")

MODE="${1:-once}"

get_updates() {
  OFFSET=""
  if [ -f "$OFFSET_FILE" ]; then
    LAST=$(cat "$OFFSET_FILE")
    OFFSET="&offset=$((LAST + 1))"
  fi
  curl -s "https://api.telegram.org/bot${TOKEN}/getUpdates?limit=5${OFFSET}" 2>/dev/null || echo '{}'
}

dispatch() {
  local cmd="$1"
  local lower=$(echo "$cmd" | tr '[:upper:]' '[:lower:]')
  local result="Unknown command. Use 'help' for list."

  # Always log EVERY owner message to full context/prompt log, so AI can read full conversation from bot
  echo "$(date '+%Y-%m-%d %H:%M:%S') USER: $cmd" >> "$DATA_DIR/bot_full_prompt.log" 2>/dev/null || true

  case "$lower" in
    help|list_commands)
      result="Commands: help, status, silver, royal, test, launch, kill, backup, plan, build, todo, todo_done <id>, market_list, market_sell <id> <price>, market_buy <id>, edit <file> <old> <new>, gobuild|compile|build_bin, sync_a1, research <q>, spawn_tester, update_plan <text>, send <msg>, poll, set_king_chat <id>, king <text>, set_treasurer_chat <id>, treasurer <text>, set_inquisitor_chat <rid> <id>, send_role <role> <text>, show_context|read_prompt|context. After any: ✅ DONE signaled. Use king/treasurer/send_role to write via the Island services SimpleX bot to other contacts (King etc add the services contact). All owner messages are logged to bot_full_prompt.log as full prompt/context."
      ;;
    status)
      local reserve=$(cat "$DATA_DIR/silver_reserve_ng.txt" 2>/dev/null || echo 0)
      local is_royal=$( [ -f "$DATA_DIR/royal.enabled" ] && echo true || echo false )
      local plan_mode=$( [ -f /tmp/royal_plan_mode ] && echo "PLAN" || echo "BUILD" )
      result="Reserve: $(echo "scale=3; $reserve / 1000000000" | bc)g | Royal: $is_royal | Mode: $plan_mode | Node: $(pgrep -x ParanoidX >/dev/null && echo up || echo down)"
      ;;
    silver|reserve)
      local ng=$(cat "$DATA_DIR/silver_reserve_ng.txt" 2>/dev/null || echo 0)
      result="Silver: ${ng} ng = $(echo "scale=3; $ng / 1e9" | bc) g = $(echo "scale=4; $ng / 31103480000" | bc) oz"
      ;;
    royal)
      result="Royal mode: $( [ -f "$DATA_DIR/royal.enabled" ] && echo active || echo inactive )"
      ;;
    test)
      result=$("$TEST_ROYAL" 2>&1 | tail -15 | tr '\n' ' ')
      ;;
    launch)
      "$LAUNCH_NODE" >/dev/null 2>&1 &
      result="Node launch initiated via launch script."
      ;;
    kill|stop)
      pkill -x ParanoidX 2>/dev/null || true
      result="Node stopped."
      ;;
    backup)
      DATE=$(date +%Y%m%d-%H%M%S)
      cp -r "$${PARANOIDX_SRC:-$HOME/ParanoidX}" "$${PARANOIDX_SRC:-$HOME/ParanoidX}-A1-$DATE" 2>/dev/null || true
      result="A1 backup triggered (see plan for full)."
      ;;
    plan)
      touch /tmp/royal_plan_mode
      result="Entered PLAN mode. Use 'build' to exit."
      ;;
    build)
      rm -f /tmp/royal_plan_mode
      result="Exited to BUILD mode."
      ;;
    todo)
      # simple, from previous todo
      result="See todo list in context. Use todo_done <id> e.g. a1-verify"
      ;;
    todo_done*)
      id=$(echo "$cmd" | cut -d' ' -f2)
      # mark in todo, but since todo is in memory, note it
      echo "$id completed" >> /tmp/royal_todo_done.log
      result="Marked $id as done. (Update todo list in plan if needed)"
      ;;
    market_list)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      if ! curl -s --max-time 2 "$API/api/status" | grep -q '"status":"running"'; then result="Node not up (use launch first)"; else result=$(curl -s "$API/api/market/list" | python3 -c 'import sys,json; d=json.load(sys.stdin); print("Listings:", len(d.get("listings",[])))' 2>/dev/null || echo "err"); fi
      ;;
    market_sell*)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      args=$(echo "$cmd" | cut -d' ' -f2-); id=$(echo "$args" | cut -d' ' -f1); price=$(echo "$args" | cut -d' ' -f2)
      if ! curl -s --max-time 2 "$API/api/status" | grep -q '"status":"running"'; then result="Node not up"; else result=$(curl -s -X POST "$API/api/market/sell" -H 'Content-Type: application/json' -d "{\"id\":\"$id\",\"price_ng\":$price}" 2>/dev/null | cat || echo "err (node?)"); fi
      ;;
    market_buy*)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      id=$(echo "$cmd" | cut -d' ' -f2)
      if ! curl -s --max-time 2 "$API/api/status" | grep -q '"status":"running"'; then result="Node not up"; else result=$(curl -s -X POST "$API/api/market/buy" -H 'Content-Type: application/json' -d "{\"id\":\"$id\"}" 2>/dev/null | cat || echo "err"); fi
      ;;
    edit*)
      # edit <file> <old> <new> - robust with shlex (handles spaces in strings if quoted)
      python3 -c '
import sys, shlex
try:
  parts = shlex.split(sys.argv[1])
  if len(parts) >= 4:
    file = parts[1]
    old = " ".join(parts[2:-1])
    new = parts[-1]
    if open(file).read().find(old) != -1:
      content = open(file).read().replace(old, new, 1)
      open(file, "w").write(content)
      print("Replaced in", file)
    else:
      print("old string not found exactly")
  else:
    print("usage: edit <file> <old> <new> (quote strings with spaces)")
except Exception as e: print("edit err:", e)
' "$cmd"
      result="Edit processed (see output)"
      ;;
    gobuild|compile|build_bin)
      (cd "$${PARANOIDX_SRC:-$HOME/ParanoidX}" && go build -o "$BIN" ./cmd/ParanoidX) 2>&1 | tail -3
      result="Build done."
      ;;
    sync_a1)
      rsync -a --delete "$${PARANOIDX_SRC:-$HOME/ParanoidX}/" "$${PARANOIDX_SRC:-$HOME/ParanoidX}-A1-current/" 2>/dev/null || cp -r "$${PARANOIDX_SRC:-$HOME/ParanoidX}" "$${PARANOIDX_SRC:-$HOME/ParanoidX}-A1-$(date +%s)"
      result="Synced to A1 backup."
      ;;
    research*)
      q=$(echo "$cmd" | cut -d' ' -f2-)
      # simulate with curl or note (full web_search via my tool when I see)
      result="Research '$q' - use web_search tool in my context or run curl. (I will handle advanced)"
      ;;
    spawn_tester)
      # note for me to spawn
      result="Spawn tester requested. (I will use spawn_subagent tool)"
      ;;
    update_plan*)
      text=$(echo "$cmd" | cut -d' ' -f2-)
      echo "$text" >> "$PLAN_SNAPSHOT"
      result="Appended to plan."
      ;;
    send*)
      msg=$(echo "$cmd" | cut -d' ' -f2-)
      "$SEND_TO" "$msg" "$CHAT_ID"
      result="Sent."
      ;;
    poll)
      result="Manual poll done."
      ;;
    checkpoint*)
      desc=$(echo "$cmd" | cut -d' ' -f2-)
      "$VERSION_CHECK" "${desc:-via bot command}" minor
      result="Checkpoint created (see bot report)."
      ;;
    versions)
      result=$(cat "$DATA_DIR/versions.log" 2>/dev/null | tail -5 || echo "no log")
      ;;
    tron_deposits)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      if ! curl -s --max-time 2 "$API/api/status" 2>/dev/null | grep -q '"status":"running"'; then result="Node not up (launch first)"; else result=$(curl -s "$API/api/treasury/usdt-deposits" 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print("deposits:", len(d.get("recent_deposits",[])), "treasury:", d.get("treasury"))' || echo "err"); fi
      ;;
    disk)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      result=$(curl -s --max-time 4 "$API/api/status" 2>/dev/null | python3 -c '
import sys,json
d=json.load(sys.stdin)
disk = d.get("disk",{})
print("root:", disk.get("root_used_pct","?"), "% used,", disk.get("root_avail_mb",0), "MB avail")
print("data_dir:", disk.get("data_dir_mb",0), "MB | backups:", disk.get("backups_total_mb",0), "MB (", disk.get("backups_count",0), "dirs)")
vuser = disk.get("vault_user_mb", 0)
vres = disk.get("vault_reservation_mb", 2048)
print("vault: user", round(vuser,1), "MB + reservation", vres, "MB container (.reserved) — total du", disk.get("vault_total_du_mb",0), "MB")
print("user % of vault quota:", round(disk.get("vault_user_pct_of_quota",0), 2), "%")
' || echo "err (node up?)")
      ;;
    disk_check)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      res=$(curl -s -X POST --max-time 6 "$API/api/disk-check" 2>/dev/null)
      result=$(echo "$res" | python3 -c '
import sys,json
d=json.load(sys.stdin)
print("disk check:", "ALERTS:" if d.get("alerts") else "ok")
for a in (d.get("alerts") or []): print(" -", a)
disk = d.get("disk",{})
print("root avail:", disk.get("root_avail_mb","?"), "MB")
print("vault user:", round(disk.get("vault_user_mb",0),1), "MB / reservation", disk.get("vault_reservation_mb",2048), "MB")
' || echo "check failed")
      ;;
    set_threshold*)
      t=$(echo "$cmd" | cut -d' ' -f2); echo "$t" > /tmp/tron_threshold.txt; result="Threshold set to $t USDT (listener can use for auto)."
      ;;
    auto_round)
      # stub: if threshold met in deposits, trigger (demo)
      result="Auto round stub (enhance with real sum check + init-round call)."
      ;;
    royal_control*)
      # stub call to /royal/control (node must be up)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      if ! curl -s --max-time 2 "$API/api/status" 2>/dev/null | grep -q '"status":"running"'; then result="Node not up (launch first)"; else result=$(curl -s -X POST "$API/royal/control" -H 'Content-Type: application/json' -d '{"cmd":"test","data":{}}' 2>/dev/null | cat || echo "err"); fi
      ;;
    royal_sync)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      if ! curl -s --max-time 2 "$API/api/status" 2>/dev/null | grep -q '"status":"running"'; then result="Node not up (launch first)"; else result=$(curl -s -X POST "$API/royal/sync" -H 'Content-Type: application/json' -d '{}' 2>/dev/null | cat || echo "err"); fi
      ;;
    set_king_chat*)
      id=$(echo "$cmd" | cut -d' ' -f2)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s "$API/api/set_role_chat?role=king&chat=$id" | cat
      result="King chat registered as $id (now can send_to_role king ... via services bot)"
      ;;
    king*)
      text=$(echo "$cmd" | cut -d' ' -f2-)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s --get --data-urlencode "role=king" --data-urlencode "text=$text" "$API/api/send_to_role" | cat
      result="Sent to king (via Island services SimpleX contact): $text"
      ;;
    set_treasurer_chat*)
      id=$(echo "$cmd" | cut -d' ' -f2)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s "$API/api/set_role_chat?role=treasurer&chat=$id" | cat
      result="Treasurer chat set"
      ;;
    treasurer*)
      text=$(echo "$cmd" | cut -d' ' -f2-)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s --get --data-urlencode "role=treasurer" --data-urlencode "text=$text" "$API/api/send_to_role" | cat
      result="Sent to treasurer via services bot"
      ;;
    set_inquisitor_chat*)
      rid=$(echo "$cmd" | cut -d' ' -f2)
      id=$(echo "$cmd" | cut -d' ' -f3)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s "$API/api/set_role_chat?role=inquisitor_$rid&chat=$id" | cat
      result="Inquisitor $rid chat set (for top auditors etc)"
      ;;
    send_role*)
      role=$(echo "$cmd" | cut -d' ' -f2)
      text=$(echo "$cmd" | cut -d' ' -f3-)
      PORT=${PORT:-8080}; API="http://127.0.0.1:$PORT"
      curl -s --get --data-urlencode "role=$role" --data-urlencode "text=$text" "$API/api/send_to_role" | cat
      result="Sent to role $role via Island services bot (SimpleX E2EE)"
      ;;
    show_context|read_prompt|context|prompt)
      if [ -f "$DATA_DIR/bot_full_prompt.log" ]; then
        result=$(tail -30 "$DATA_DIR/bot_full_prompt.log" 2>/dev/null || echo "no log")
      else
        result="No bot_full_prompt.log yet. All owner messages are now logged for full prompt/context."
      fi
      ;;
    *)
      result="Message logged to full prompt/context (bot_full_prompt.log). Use 'show_context' to read recent. 'help' for list."
      ;;
  esac
  echo "$result"
}

process_updates() {
  local updates=$(get_updates)
  export LISTENER="${LISTENER:-}" SIGNAL_DONE="${SIGNAL_DONE:-}" SCRIPTS_DIR="${SCRIPTS_DIR:-}" DATA_DIR="${DATA_DIR:-}" PLAN_SNAPSHOT="${PLAN_SNAPSHOT:-}" OFFSET_FILE="${OFFSET_FILE:-/tmp/royal_last_update.txt}" CHAT_ID="${CHAT_ID:-}"
  echo "$updates" | python3 -c '
import sys, json, subprocess, os
d = json.load(sys.stdin)
chat = os.environ.get("CHAT_ID", "")
offset_file = os.environ.get("OFFSET_FILE", "/tmp/royal_last_update.txt")
max_id = None
for u in d.get("result", []):
  uid = u.get("update_id")
  if uid is not None:
    if max_id is None or uid > max_id:
      max_id = uid
  m = u.get("message") or u.get("edited_message") or {}
  if str(m.get("chat",{}).get("id")) != chat: continue
  text = (m.get("text") or m.get("caption") or "").strip()
  cb = u.get("callback_query") or {}
  if not text and cb.get("data"):
    text = "cb:" + (cb.get("data") or "")
  if not text:
    # log non-text from owner so it still enters full prompt/context (photo/voice/sticker etc)
    cap = m.get("caption") or ""
    typ = "photo" if m.get("photo") else ("voice" if m.get("voice") else ("document" if m.get("document") else "non-text"))
    text = f"[non-text:{typ}] {cap}".strip()
    if not text: text = "[non-text update from owner]"
    # still log below via dispatch, but mark it
  update_id = uid
  print(f"New cmd from bot: {text}")
  # execute dispatch in bash (dispatch always does USER: log for full prompt, and --signal triggers the exact step done signal)
  try:
    listener_path = os.environ.get("LISTENER") or os.environ.get("SCRIPTS_DIR","/home/tomas/ParanoidX/scripts") + "/royal-telegram-command-listener.sh"
    out = subprocess.check_output([listener_path, "dispatch", "--signal", text], stderr=subprocess.STDOUT, timeout=120).decode()
    print("Executed:", out[:200])
  except Exception as e:
    out = str(e)
  # Note: no duplicate signal here - the --signal in dispatch already calls signal_step_done exactly once per owner msg to avoid spam.
# advance offset past ALL seen updates (even non-owner) to prevent replay storms
if max_id is not None:
  with open(offset_file, "w") as f: f.write(str(max_id))
'
}

if [ "$MODE" = "dispatch" ]; then
  # internal for the python above (and manual with --signal for testing)
  shift
  sig=0
  # support "dispatch --signal cmd..." or "dispatch cmd --signal" or "dispatch cmd"
  if [ "${1:-}" = "--signal" ]; then sig=1; shift; fi
  cmdargs="$*"
  if [ "${!#}" = "--signal" ]; then sig=1; cmdargs="${*%% --signal}"; fi
  dispatch "$cmdargs"
  if [ $sig -eq 1 ]; then
    "$SIGNAL_DONE" "dispatch $cmdargs" "manual signal" 2>/dev/null || /home/tomas/ParanoidX/scripts/signal_step_done.sh "dispatch $cmdargs" "manual signal"
  fi
  exit 0
fi

if [ "$MODE" = "loop" ]; then
  echo "Loop mode (use launch-bot-listener.sh for proper)"
  while true; do
    process_updates
    sleep 2
  done
else
  process_updates
fi
