#!/bin/bash
# Send reports to inquisitor bot (opencode-tg-bot) — supports text + document + photo.
# Every text report automatically includes uptime stats footer.
# Usage:
#   send-to-inquisitor.sh "text message"
#   send-to-inquisitor.sh -d /path/to/file.md "caption"
#   send-to-inquisitor.sh -p /path/to/photo.png "caption"
set -euo pipefail

# Collect uptime stats footer
append_uptime_footer() {
  local msg="$1"
  local uptime_line
  uptime_line=$(uptime -p 2>/dev/null || echo "uptime N/A")
  local load_line
  load_line=$(uptime | awk -F'load average:' '{print "load:" $2}' 2>/dev/null || echo "")
  local server_uptime
  server_uptime=$(curl -sf --max-time 2 http://127.0.0.1:8080/api/version 2>/dev/null | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('uptime_hours', 'N/A') + 'h')
except: print('N/A')
" 2>/dev/null || echo "N/A")
  local health
  health=$(curl -sf --max-time 2 http://127.0.0.1:8080/api/health 2>/dev/null | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    h = '✅' if d.get('healthy') else '❌'
    print(f'{h} bridge={d.get(\"bridge\",\"?\")} msgs={d.get(\"messages\",0)}')
except: print('N/A')
" 2>/dev/null || echo "N/A")
  printf '%s\n\n━━━ Uptime ━━━\nHost: %s | %s\nServer: %s | %s' \
    "$msg" "$uptime_line" "$load_line" "$server_uptime" "$health"
}

TOKEN_FILE="${HOME}/.config/opencode-tg-bot.token"
CHAT_FILE="${HOME}/.config/opencode-tg-bot.chat"

if [ ! -f "$TOKEN_FILE" ]; then
  echo "ERROR: no token at $TOKEN_FILE"
  exit 1
fi

TOKEN=$(tr -d '\n\r' < "$TOKEN_FILE")

# CHAT_ID: arg 2 (if -d/-p), or file, or arg 2 for text
if [ -f "$CHAT_FILE" ]; then
  CHAT_ID=$(tr -d '\n\r' < "$CHAT_FILE")
else
  echo "No chat_id file at $CHAT_FILE"
  exit 1
fi

case "${1:-}" in
  -d|--document)
    FILE="${2:?No file specified}"
    CAPTION="${3:-}"
    if [ ! -f "$FILE" ]; then
      echo "ERROR: file not found: $FILE"
      exit 1
    fi
    curl -s -X POST "https://api.telegram.org/bot${TOKEN}/sendDocument" \
      -F "chat_id=${CHAT_ID}" \
      -F "document=@${FILE}" \
      -F "caption=${CAPTION}" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if d.get("ok"):
    print("✅ Document sent, message_id:", d["result"]["message_id"])
else:
    print("❌ Error:", d.get("description", d))
'
    ;;

  -p|--photo)
    FILE="${2:?No file specified}"
    CAPTION="${3:-}"
    if [ ! -f "$FILE" ]; then
      echo "ERROR: file not found: $FILE"
      exit 1
    fi
    curl -s -X POST "https://api.telegram.org/bot${TOKEN}/sendPhoto" \
      -F "chat_id=${CHAT_ID}" \
      -F "photo=@${FILE}" \
      -F "caption=${CAPTION}" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if d.get("ok"):
    print("✅ Photo sent, message_id:", d["result"]["message_id"])
else:
    print("❌ Error:", d.get("description", d))
'
    ;;

  -*)
    echo "Usage: $0 [-d file caption | -p file caption | text]"
    exit 1
    ;;

  *)
    RAW_MSG="${1:-A1 update}"
    FULL_MSG=$(append_uptime_footer "$RAW_MSG")
    curl -s -X POST "https://api.telegram.org/bot${TOKEN}/sendMessage" \
      -H "Content-Type: application/json" \
      -d "$(python3 -c "
import json
msg = '''${FULL_MSG}'''
print(json.dumps({'chat_id': ${CHAT_ID}, 'text': msg[:4000]}))
")" | python3 -c '
import sys, json
d = json.load(sys.stdin)
if d.get("ok"):
    print("✅ Inquisitor msg sent, message_id:", d["result"]["message_id"])
else:
    print("❌ Error:", d.get("description", d))
'
    ;;
esac
