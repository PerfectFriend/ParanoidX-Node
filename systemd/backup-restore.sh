#!/bin/bash
# backup-restore.sh — Restore simplex-node from USB backup
# Usage: sudo bash backup-restore.sh /path/to/backup/TIMESTAMP/

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: sudo bash backup-restore.sh /path/to/backup/TIMESTAMP/"
  exit 1
fi

BACKUP_DIR="$1"
ME="tomas"
PROJECT_DIR="/home/$ME/simplex-node"
LOG="/tmp/simplex-restore-$(date +%Y%m%d-%H%M%S).log"

exec > >(tee "$LOG") 2>&1

echo "=== Restoring from $BACKUP_DIR ==="

# Verify backup
if [ ! -f "$BACKUP_DIR/MANIFEST.txt" ]; then
  echo "ERROR: $BACKUP_DIR/MANIFEST.txt not found. Not a valid backup."
  exit 1
fi

# 1. Restore project source
echo "--- Restoring project source ---"
if [ -d "$BACKUP_DIR/simplex-node" ]; then
  rsync -a --delete \
    --exclude='docker/smp_state/' \
    --exclude='docker/xftp_state/' \
    "$BACKUP_DIR/simplex-node/" "$PROJECT_DIR/"
  echo "Project source restored"
fi

# 2. Restore runtime data
echo "--- Restoring runtime data ---"
DATA_DIR="/home/$ME/.local/share/simplex-node"
if [ -d "$BACKUP_DIR/data" ]; then
  mkdir -p "$DATA_DIR"
  rsync -a --delete "$BACKUP_DIR/data/" "$DATA_DIR/"
  chown -R "$ME:$ME" "$DATA_DIR"
  echo "Runtime data restored"
fi

# 3. Restore config and tokens
echo "--- Restoring config and tokens ---"
if [ -d "$BACKUP_DIR/config" ]; then
  for f in "$BACKUP_DIR/config"/*.token "$BACKUP_DIR/config"/simplex-node.env; do
    if [ -f "$f" ]; then
      cp -a "$f" "/home/$ME/.config/"
      chown "$ME:$ME" "/home/$ME/.config/$(basename "$f")"
      chmod 600 "/home/$ME/.config/$(basename "$f")"
      echo "Restored $(basename "$f")"
    fi
  done
fi

# 4. Restore Kilo AI
echo "--- Restoring Kilo AI ---"
if [ -d "$BACKUP_DIR/kilo" ]; then
  rsync -a --delete "$BACKUP_DIR/kilo/" "/home/$ME/.kilo/"
  chown -R "$ME:$ME" "/home/$ME/.kilo/"
  echo "Kilo AI restored"
fi

# 5. Restore OpenCode config
echo "--- Restoring OpenCode config ---"
if [ -d "$BACKUP_DIR/opencode" ]; then
  rsync -a --delete "$BACKUP_DIR/opencode/" "/home/$ME/.opencode/"
  chown -R "$ME:$ME" "/home/$ME/.opencode/"
  echo "OpenCode config restored"
fi

# 6. Restore systemd services
echo "--- Restoring systemd services ---"
if [ -d "$BACKUP_DIR/systemd" ]; then
  cp "$BACKUP_DIR/systemd/"*.service /etc/systemd/system/ 2>/dev/null || true
  cp "$BACKUP_DIR/systemd/"*.target /etc/systemd/system/ 2>/dev/null || true
  systemctl daemon-reload
  echo "Systemd services restored"
fi

# 7. Restore docker configs (but not volumes)
echo "--- Restoring docker configs ---"
if [ -f "$BACKUP_DIR/docker-compose.yaml" ]; then
  cp "$BACKUP_DIR/docker-compose.yaml" "$PROJECT_DIR/docker/compose.yaml"
fi
if [ -d "$BACKUP_DIR/smp_configs" ]; then
  cp -a "$BACKUP_DIR/smp_configs/" "$PROJECT_DIR/docker/"
fi
if [ -d "$BACKUP_DIR/xftp_configs" ]; then
  cp -a "$BACKUP_DIR/xftp_configs/" "$PROJECT_DIR/docker/"
fi
if [ -d "$BACKUP_DIR/tor" ]; then
  cp -a "$BACKUP_DIR/tor/" "$PROJECT_DIR/docker/"
fi
chown -R "$ME:$ME" "$PROJECT_DIR/docker/"
echo "Docker configs restored"

# 8. Restart services
echo "--- Restarting services ---"
systemctl restart simplex-node.target 2>/dev/null || true
echo "Services restarted (check with systemctl status simplex-node.target)"

echo ""
echo "=== Restore complete ==="
echo "Log: $LOG"
echo ""
echo "Post-restore checks:"
echo "  systemctl status simplex-node.target"
echo "  curl -s http://127.0.0.1:8080/api/status | python3 -m json.tool"
echo "  docker ps"
