#!/bin/bash
ISLE_HOME="${ISLE_HOME:-$HOME}"  # переносимый дом проекта: переопредели env-ом, дефолт $HOME
# auto-restart supervisor for isle_app (Flutter client)
set -e
APP="$ISLE_HOME/.local/bin/the-isle/isle_app"
LOG="/tmp/isle_app.log"

while true; do
    echo "[$(date)] starting isle_app..." >> "$LOG"
    "$APP" >> "$LOG" 2>&1
    EXIT_CODE=$?
    echo "[$(date)] isle_app exited ($EXIT_CODE), restarting in 2s..." >> "$LOG"
    sleep 2
done
