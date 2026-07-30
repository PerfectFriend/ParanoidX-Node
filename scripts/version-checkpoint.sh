#!/bin/bash
# Automatic version control checkpoint for the project.
# After significant changes (bot listener, marketplace, new commands, edits via bot, etc.):
# - Determines next version (A1.1, A1.2 for minor; A2 for major).
# - Creates dated backup simplex-node-VER-DATE with source, data, plan snapshot, VERSION-VER.
# - Appends to versions.log .
# - Sends report to bot via send-to-torquemada.sh .
# Usage: ./scripts/version-checkpoint.sh "description of change" [major]
# Example: ./scripts/version-checkpoint.sh "added automatic bot listener + version control + marketplace" minor

set -euo pipefail

source "$(dirname "$0")/royal-common.sh" 2>/dev/null || true

DESC="${1:-Significant update via bot commands}"
IS_MAJOR="${2:-minor}"

: "${BASE_DIR:=$HOME}"
: "${SRC:=$${PARANOIDX_SRC:-$HOME/ParanoidX}}"
: "${DATA:=$DATA_DIR}"
: "${PLAN_SNAPSHOT:=$PLAN_SNAPSHOT}"
: "${LOG:=$DATA/versions.log}"
: "${SEND_SCRIPT:=$SEND_TO}"

mkdir -p "$DATA"

# Determine current base version from main VERSION-A1 or A0
if [ -f "$SRC/VERSION-A1" ]; then
  CUR_VER=$(head -1 "$SRC/VERSION-A1" | awk '{print $1}')
elif [ -f "$SRC/VERSION-A0" ]; then
  CUR_VER=$(head -1 "$SRC/VERSION-A0" | awk '{print $1}')
else
  CUR_VER="A1"
fi

# Bump logic
if [ "$IS_MAJOR" = "major" ]; then
  # A1 -> A2
  BASE=$(echo "$CUR_VER" | sed 's/[0-9.]*$//')
  NUM=$(echo "$CUR_VER" | grep -o '[0-9]*' | head -1)
  NEXT_NUM=$(( ${NUM:-1} + 1 ))
  NEXT_VER="${BASE}${NEXT_NUM}"
else
  # A1 -> A1.1 , A1.1 -> A1.2
  if [[ "$CUR_VER" == *.* ]]; then
    BASE=$(echo "$CUR_VER" | cut -d. -f1)
    SUB=$(echo "$CUR_VER" | cut -d. -f2)
    NEXT_SUB=$(( ${SUB:-0} + 1 ))
    NEXT_VER="${BASE}.${NEXT_SUB}"
  else
    NEXT_VER="${CUR_VER}.1"
  fi
fi

DATE=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="$BASE_DIR/simplex-node-$NEXT_VER-$DATE"
BACKUP_DATA_DIR="$HOME/.local/share/simplex-node-$NEXT_VER-$DATE"

echo "Creating checkpoint $NEXT_VER-$DATE for: $DESC"

cp -r "$SRC" "$BACKUP_DIR"
cp -r "$DATA" "$BACKUP_DATA_DIR" 2>/dev/null || true
cp "$PLAN_SNAPSHOT" "$BACKUP_DIR/plan-snapshot.md" 2>/dev/null || true

echo "$NEXT_VER - $DESC (date $DATE). Includes: bot automatic listener (2s poll, passwordless, DONE signals), full commands (plan/build, edit, market_*, checkpoint, spawn_tester, research, todo_*, update_plan, etc.), marketplace hand-to-hand (list/sell/buy), automatic version control. Can rollback: cp -r $BACKUP_DIR $SRC . Branch for experiments." > "$BACKUP_DIR/VERSION-$NEXT_VER"

# log
echo "$NEXT_VER-$DATE: $DESC" >> "$LOG"

# report to bot
if [ -x "$SEND_SCRIPT" ]; then
  "$SEND_SCRIPT" "Новая контрольная точка версии $NEXT_VER-$DATE создана. Путь: $BACKUP_DIR . Откат: cp -r $BACKUP_DIR $SRC . В лог: $LOG . Описание: $DESC . Продолжаем план A1 (royal, testing, marketplace, bot control)."
fi

echo "Checkpoint $NEXT_VER-$DATE created successfully. Report sent to bot."
echo "To rollback: cp -r $BACKUP_DIR $SRC && cp -r $BACKUP_DATA_DIR $DATA"
