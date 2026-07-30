#!/bin/bash

# ===== simplex-node USB Backup Script =====
# Streams backup directly to USB (no /tmp space needed).
# Usage: bash scripts/backup-to-usb.sh [--path /mnt/usb]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
HOSTNAME="$(hostname)"
DATE="$(date +%Y%m%d-%H%M%S)"
BACKUP_NAME="simplex-node-backup-${HOSTNAME}-${DATE}"
CUSTOM_PATH=""
RC=0

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

echo -e "${CYAN}══════════════════════════════════════════════${NC}"
echo -e "${CYAN}  simplex-node USB Backup — ${BACKUP_NAME}${NC}"
echo -e "${CYAN}══════════════════════════════════════════════${NC}"

for arg in "$@"; do
  case "$arg" in --path=*) CUSTOM_PATH="${arg#*=}" ;; esac
done

DATA_DIR="$HOME/.local/share/simplex-node"
GO_BIN="$HOME/bin/simplex-node"
FLUTTER_DIR="$HOME/.local/bin/the-isle"

# === Determine destination ===
if [ -n "$CUSTOM_PATH" ]; then
  BACKUP_DIR="$CUSTOM_PATH"
  mkdir -p "$BACKUP_DIR"
elif [ -d /mnt/simplex-backup ]; then
  BACKUP_DIR="/mnt/simplex-backup"
else
  echo -e "${YELLOW}--path not specified, checking for mounted SIMPLEX-BACKUP USB...${NC}"
  MOUNT=$(lsblk -o MOUNTPOINT,LABEL 2>/dev/null | awk '/SIMPLEX-BACKUP/ {print $1}')
  if [ -n "$MOUNT" ]; then
    BACKUP_DIR="$MOUNT"
  else
    echo -e "${RED}USB not found. Mount it or use --path=/mnt/usb${NC}"
    exit 1
  fi
fi

BACKUP_FILE="${BACKUP_DIR}/${BACKUP_NAME}.tar"
CHECKSUM_FILE="${BACKUP_DIR}/${BACKUP_NAME}.sha256"

echo -e "${GREEN}Backup target: $BACKUP_FILE${NC}"
echo -e "${YELLOW}Checking free space...${NC}"
AVAIL=$(df -BM "$BACKUP_DIR" | awk 'NR==2 {print $4}' | tr -d 'M')
echo -e "${GREEN}Available: ${AVAIL}M${NC}"

# === Build tar directly on USB (append mode) ===
echo -e "${YELLOW}Building backup archive (streaming to USB)...${NC}"

# Step 1: metadata
(
  echo "Hostname: $HOSTNAME"
  echo "Date: $(date)"
  echo "User: $(whoami)"
  echo "Git: $(cd "$PROJECT_DIR" && git rev-parse HEAD 2>/dev/null || echo '?')"
  echo "Version: $(curl -s http://localhost:8080/api/version 2>/dev/null | grep -oP '"build":"[^"]*"' | tr -d '"' || echo 'not running')"
) | tar -cf "$BACKUP_FILE" --transform="s|.*|simplex-node-backup/backup-info.txt|" -T - 2>/dev/null

# Step 2: git bundle (portable git history)
GIT_BUNDLE_FILE="$BACKUP_DIR/${BACKUP_NAME}.gitbundle"
echo -e "  Creating git bundle..."
(cd "$PROJECT_DIR" && git bundle create "$GIT_BUNDLE_FILE" --all 2>/dev/null) || RC=1
echo -e "  Git bundle: $(ls -lh "$GIT_BUNDLE_FILE" | awk '{print $5}')"

# Step 3: codebase (exclude heavy build artifacts, include git index/staging)
echo -e "  Adding codebase..."
(cd "$PROJECT_DIR" && tar -rf "$BACKUP_FILE" \
  --exclude='.git' --exclude='apps/isle_app/build' \
  --exclude='apps/isle_app/.dart_tool' --exclude='backups' \
  --exclude='opencode-config-backup/node_modules' \
  --exclude='.kilo/node_modules' \
  --exclude='node_modules' \
  --transform="s|^|simplex-node-backup/codebase/|" . 2>/dev/null) || RC=1

# Step 3b: session context backup
echo -e "  Adding session context..."
for src in "$PROJECT_DIR/AGENTS.md" "$HOME/.opencode/sessions"*; do
  [ -e "$src" ] || continue
  (cd "$(dirname "$src")" && tar -rf "$BACKUP_FILE" \
    --transform="s|^$(basename "$src")$|simplex-node-backup/session/$(basename "$src")|" \
    "$(basename "$src")" 2>/dev/null) || true
done
# Also copy the latest session files from .opencode if exists
if [ -d "$PROJECT_DIR/.opencode" ]; then
  (cd "$PROJECT_DIR" && tar -rf "$BACKUP_FILE" \
    --exclude='.opencode/node_modules' \
    --transform="s|^\.opencode|simplex-node-backup/session/opencode|" \
    ".opencode" 2>/dev/null) || true
fi

# Step 4: data dir (exclude vault sparse reservation files)
echo -e "  Adding data dir..."
(cd "$HOME" && tar -rf "$BACKUP_FILE" \
  --exclude='.local/share/simplex-node/vault/.vault_reserve' \
  --exclude='.local/share/simplex-node/vault/.reserved' \
  --transform="s|^|simplex-node-backup/data/|" \
  ".local/share/simplex-node" 2>/dev/null) || RC=1

# Step 5: Flutter bundle
echo -e "  Adding Flutter bundle..."
(cd "$HOME" && tar -rf "$BACKUP_FILE" \
  --transform="s|^|simplex-node-backup/flutter/|" \
  ".local/bin/the-isle" 2>/dev/null) || RC=1

# Step 6: Go binary
echo -e "  Adding Go binary..."
(cd "$HOME" && tar -rf "$BACKUP_FILE" \
  --transform="s|^|simplex-node-backup/bin/|" \
  "bin/simplex-node" 2>/dev/null) || RC=1

# Step 7: opencode config + context
echo -e "  Adding context files..."
for src in "$HOME/.config/opencode" "$PROJECT_DIR/.opencode" "$PROJECT_DIR/AGENTS.md"; do
  [ -e "$src" ] || continue
  (cd "$(dirname "$src")" && tar -rf "$BACKUP_FILE" \
    --transform="s|^$(basename "$src")$|simplex-node-backup/config/$(basename "$src")|" \
    "$(basename "$src")" 2>/dev/null) || true
done

echo -e "${GREEN}Tar created: $(ls -lh "$BACKUP_FILE" | awk '{print $5}')${NC}"

# Step 8: compress (still on USB, no /tmp)
echo -e "${YELLOW}Compressing (gzip)...${NC}"
gzip -f "$BACKUP_FILE"
BACKUP_FILE="${BACKUP_FILE}.gz"

# Checksum
sha256sum "$BACKUP_FILE" | tee "$CHECKSUM_FILE"

# Copy restore script alongside
cp "$SCRIPT_DIR/restore-from-usb.sh" "${BACKUP_DIR}/restore-from-usb.sh" 2>/dev/null || true

# === Rotation: keep latest 5 backups ===
echo -e "\n${YELLOW}Rotating old backups (keeping 5 latest)...${NC}"
# Keep only the 5 newest .tar.gz files, remove older ones
ls -t "${BACKUP_DIR}/"simplex-node-backup-*.tar.gz 2>/dev/null | tail -n +6 | xargs -r -I{} sh -c 'rm -f "$1" "${1%.gz}.sha256" "${1%.tar.gz}.gitbundle" 2>/dev/null; echo "  Removed: $(basename "$1")"' -- {}

# === Verify ===
echo -e "\n${YELLOW}Verifying...${NC}"
gzip -t "$BACKUP_FILE" 2>/dev/null && echo -e "${GREEN}✓ Archive valid${NC}" || echo -e "${RED}✗ Archive corrupt${NC}"
sha256sum -c "$CHECKSUM_FILE" --quiet 2>/dev/null && echo -e "${GREEN}✓ Checksum OK${NC}" || echo -e "${RED}✗ Checksum mismatch${NC}"

echo -e "\n${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Backup complete!${NC}"
echo -e "${GREEN}  File: $BACKUP_FILE ($(ls -lh "$BACKUP_FILE" | awk '{print $5}'))${NC}"
echo -e "${GREEN}  SHA256: $(awk '{print $1}' "$CHECKSUM_FILE")${NC}"
echo -e "${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${CYAN}Restore: bash scripts/restore-from-usb.sh --from=$BACKUP_FILE${NC}"
exit $RC
