#!/bin/bash
# backup.sh — Full system backup to USB (tar-based for FAT32 compatibility)
# Usage: bash backup.sh [/path/to/backup/dir]
# Default: /run/media/tomas/UBUNTU-SERV/simplex-node-backup/

set -euo pipefail

BACKUP_BASE="${1:-/run/media/tomas/UBUNTU-SERV/simplex-node-backup}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="$BACKUP_BASE/$TIMESTAMP"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ME="tomas"
LOG="/tmp/simplex-backup-${TIMESTAMP}.log"

mkdir -p "$BACKUP_DIR"
exec > >(tee "$LOG") 2>&1

echo "=== simplex-node backup to $BACKUP_DIR ==="
echo "Timestamp: $TIMESTAMP"

# 1. Project source (tar - skips binary, docker state, node_modules)
echo "--- Backing up project source ---"
tar cf "$BACKUP_DIR/simplex-node-source.tar" \
  --exclude='simplex-node' \
  --exclude='simplex-node.bak' \
  --exclude='docker/smp_state' \
  --exclude='docker/xftp_state' \
  --exclude='docker/tor/hidden_services' \
  --exclude='__pycache__' \
  --exclude='*.pyc' \
  --exclude='.git' \
  --exclude='opencode-config-backup/node_modules' \
  --exclude='opencode-config-backup/.npm' \
  -C "$PROJECT_DIR" .
echo "  $(du -h "$BACKUP_DIR/simplex-node-source.tar" | cut -f1)"

# 2. Runtime data (state JSON files, no vault)
echo "--- Backing up runtime data (state) ---"
DATA_DIR="/home/$ME/.local/share/simplex-node"
if [ -d "$DATA_DIR" ]; then
  tar cf "$BACKUP_DIR/simplex-node-data.tar" \
    --exclude='vault' \
    -C "$DATA_DIR" .
  echo "  $(du -h "$BACKUP_DIR/simplex-node-data.tar" | cut -f1)"
fi

# 2b. Vault files (2GB, large file storage - optional restore)
echo "--- Backing up vault ---"
if [ -d "$DATA_DIR/vault" ]; then
  tar cf "$BACKUP_DIR/simplex-node-vault.tar" \
    -C "$DATA_DIR" vault/
  echo "  $(du -h "$BACKUP_DIR/simplex-node-vault.tar" | cut -f1)"
fi

# 3. Config and tokens
echo "--- Backing up config and tokens ---"
mkdir -p "$BACKUP_DIR/config"
for f in /home/$ME/.config/admin-bot.token /home/$ME/.config/royal-bot.token /home/$ME/.config/poet-bot.token /home/$ME/.config/simplex-node.env; do
  if [ -f "$f" ]; then
    cp --preserve=timestamps "$f" "$BACKUP_DIR/config/"
    echo "  saved $(basename $f)"
  fi
done

# 4. Systemd service files
echo "--- Backing up systemd services ---"
mkdir -p "$BACKUP_DIR/systemd"
for f in /etc/systemd/system/simplex-node.target /etc/systemd/system/simplex-node-*.service; do
  [ -f "$f" ] && cp --preserve=timestamps "$f" "$BACKUP_DIR/systemd/"
done

# 5. Docker compose and configs
echo "--- Backing up docker configs ---"
tar cf "$BACKUP_DIR/simplex-node-docker.tar" \
  --exclude='smp_state' \
  --exclude='xftp_state' \
  --exclude='tor/hidden_services' \
  -C "$PROJECT_DIR/docker" .
echo "  $(du -h "$BACKUP_DIR/simplex-node-docker.tar" | cut -f1)"

# 6. Kilo AI (if exists)
echo "--- Backing up Kilo AI ---"
if [ -d "/home/$ME/.kilo" ]; then
  tar cf "$BACKUP_DIR/simplex-node-kilo.tar" -C "/home/$ME/.kilo" .
  echo "  $(du -h "$BACKUP_DIR/simplex-node-kilo.tar" | cut -f1)"
fi

# 7. OpenCode config
echo "--- Backing up OpenCode config ---"
if [ -d "/home/$ME/.opencode" ]; then
  tar cf "$BACKUP_DIR/simplex-node-opencode.tar" \
    --exclude='storage' \
    -C "/home/$ME/.opencode" .
  echo "  $(du -h "$BACKUP_DIR/simplex-node-opencode.tar" | cut -f1)"
fi

# 8. Package manifests
echo "--- Saving package manifests ---"
dpkg -l > "$BACKUP_DIR/dpkg-packages.txt" 2>/dev/null || true
which nix 2>/dev/null && nix-env -q > "$BACKUP_DIR/nix-packages.txt" 2>/dev/null || true

# 9. Manifest
cat > "$BACKUP_DIR/MANIFEST.txt" << EOF
Backup: simplex-node $TIMESTAMP
User: $ME
Host: $(hostname)

Archives:
  simplex-node-source.tar      — project source (no binary, docker volumes, node_modules)
  simplex-node-data.tar        — runtime state (ledger, banknotes, lock, addresses, json config)
  simplex-node-vault.tar       — vault file storage (2GB, optional)
  simplex-node-docker.tar      — docker compose + configs (no state/volumes)
  simplex-node-kilo.tar        — Kilo AI binary and models
  simplex-node-opencode.tar    — OpenCode configuration

Files:
  config/                      — bot tokens, env file
  systemd/                     — systemd service files
  dpkg-packages.txt            — installed packages
  nix-packages.txt             — Nix packages
EOF

# 10. Summary
echo ""
echo "=== Backup complete ==="
echo "Location: $BACKUP_DIR"
echo "Size: $(du -sh "$BACKUP_DIR" | cut -f1)"
echo "Log: $LOG"
echo ""
echo "To restore:"
echo "  bash systemd/backup-restore.sh $BACKUP_DIR"
