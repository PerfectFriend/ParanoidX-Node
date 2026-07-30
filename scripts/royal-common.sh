#!/bin/bash
# Centralized paths for royal bot/scripts (sourced by listener, launch-*, test, send, signal, version-checkpoint).
# Reduces 40+ hardcodes, prevents drift across A* copies, eases rename/port.
# Usage: source "$(dirname "$0")/royal-common.sh"  or full path.

# Base (override via env if testing in other home)
: "${ROYAL_BASE:=$HOME}"
: "${${PARANOIDX_SRC:-$HOME/ParanoidX}:=${ROYAL_BASE}/simplex-node}"
: "${DATA_DIR:=${ROYAL_BASE}/.local/share/simplex-node}"

# Config
TOKEN_FILE="${HOME}/.config/royal-bot.token"
CHAT_FILE="${HOME}/.config/royal-bot.chat"
OFFSET_FILE="${DATA_DIR}/royal_last_update.txt"

# Scripts dir (this file's dir)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$SCRIPT_DIR"

# Key files / bins
BIN="${ROYAL_BASE}/bin/ParanoidX"
NODE_SRC_DIR="$SIMPLEX_SR./cmd/ParanoidX"
LAUNCH_NODE="$SCRIPTS_DIR/launch-node.sh"
LAUNCH_BOT="$SCRIPTS_DIR/launch-bot-listener.sh"
LISTENER="$SCRIPTS_DIR/royal-telegram-command-listener.sh"
TEST_ROYAL="$SCRIPTS_DIR/test-royal.sh"
SEND_TO="$SCRIPTS_DIR/send-to-torquemada.sh"
SIGNAL_DONE="$SCRIPTS_DIR/signal_step_done.sh"
VERSION_CHECK="$SCRIPTS_DIR/version-checkpoint.sh"

# Plan snapshot (dynamic: prefer current session, fallback to known)
find_plan_snapshot() {
  local p
  p=$(find "${HOME}/.grok/sessions" -path '*%2Fhome%2Ftomas*' -name plan.md 2>/dev/null | head -1)
  if [ -z "$p" ]; then
    p="${HOME}/.grok/sessions/%2Fhome%2Ftomas/019e79fa-9fa5-7c51-a6d2-52d0f52edddb/plan.md"
  fi
  echo "$p"
}
PLAN_SNAPSHOT="$(find_plan_snapshot)"

# Common helpers (used by listener etc)
load_chat_id() {
  if [ -n "${2:-}" ]; then echo "$2"; return; fi
  if [ -f "$CHAT_FILE" ]; then cat "$CHAT_FILE"; return; fi
  if [ -n "${OWNER_CHAT_ID:-}" ]; then echo "$OWNER_CHAT_ID"; return; fi
  echo ""
}

load_token() {
  if [ ! -f "$TOKEN_FILE" ]; then
    echo "ERROR: no token $TOKEN_FILE" >&2
    return 1
  fi
  cat "$TOKEN_FILE"
}

# For python -c that need paths: export them before python
export_royal_env() {
  export ROYAL_BASE ${PARANOIDX_SRC:-$HOME/ParanoidX} DATA_DIR TOKEN_FILE CHAT_FILE OFFSET_FILE SCRIPT_DIR SCRIPTS_DIR BIN PLAN_SNAPSHOT
}

# Note: after source, scripts can use $DATA_DIR etc instead of hard /home/tomas/...
# Listener/update_plan/version-checkpoint now use $PLAN_SNAPSHOT (dynamic).
