#!/bin/bash

# ===== ParanoidX USB Restore Script =====
# Restores codebase + data + binaries from USB backup archive.
# Usage: bash scripts/restore-from-usb.sh [--from /path/to/backup.tar.gz]

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RC=0

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

BACKUP_FILE=""

for arg in "$@"; do
  case "$arg" in --from=*) BACKUP_FILE="${arg#*=}" ;; esac
done

# Find backup if not specified
if [ -z "$BACKUP_FILE" ]; then
  echo -e "${YELLOW}Scanning for backups...${NC}"
  CANDIDATES=()
  for dir in /mnt/simplex-backup /media/*; do
    [ -d "$dir" ] || continue
    while IFS= read -r -d '' f; do CANDIDATES+=("$f"); done \
      < <(find "$dir" -name "ParanoidX-backup-*.tar.gz" -print0 2>/dev/null)
  done
  # Also check local
  while IFS= read -r -d '' f; do CANDIDATES+=("$f"); done \
    < <(find "$PROJECT_DIR/backups" -name "ParanoidX-backup-*.tar.gz" -print0 2>/dev/null)

  if [ ${#CANDIDATES[@]} -eq 0 ]; then
    echo -e "${RED}No backups found. Use --from=/path/to/backup.tar.gz${NC}"
    exit 1
  fi
  echo -e "${GREEN}Available backups:${NC}"
  for i in "${!CANDIDATES[@]}"; do
    echo "  [$((i+1))] $(ls -lh "${CANDIDATES[$i]}" | awk '{print $5}') ${CANDIDATES[$i]}"
  done
  [ ${#CANDIDATES[@]} -eq 1 ] && IDX=0 || read -p "Select [1-${#CANDIDATES[@]}]: " IDX && IDX=$((IDX-1))
  BACKUP_FILE="${CANDIDATES[$IDX]}"
fi

if [ ! -f "$BACKUP_FILE" ]; then
  echo -e "${RED}File not found: $BACKUP_FILE${NC}"
  exit 1
fi

echo -e "${GREEN}Selected: $BACKUP_FILE${NC}"

# Verify
echo -e "${YELLOW}Verifying archive...${NC}"
gzip -t "$BACKUP_FILE" 2>/dev/null && echo -e "${GREEN}✓ Archive valid${NC}" || { echo -e "${RED}✗ Corrupt${NC}"; exit 1; }
CHECKSUM="${BACKUP_FILE%.gz}.sha256"
[ -f "$CHECKSUM" ] && sha256sum -c "$CHECKSUM" --quiet 2>/dev/null && echo -e "${GREEN}✓ Checksum OK${NC}" || echo -e "${YELLOW}⚠ No checksum${NC}"

# Preview
echo -e "\n${YELLOW}Archive contents:${NC}"
tar -tzf "$BACKUP_FILE" | head -20
echo "  ..."

# Confirm
echo -e "\n${RED}⚠ This will STOP the server and OVERWRITE files!${NC}"
read -p "Restore? [y/N]: " CONFIRM
[ "$CONFIRM" != "y" ] && { echo -e "${YELLOW}Cancelled${NC}"; exit 0; }

# Stop server
echo -e "${YELLOW}Stopping server...${NC}"
fuser -k "$HOME/bin/ParanoidX" 2>/dev/null || true
sleep 2

# Extract
echo -e "${YELLOW}Extracting to $HOME...${NC}"
tar -xzf "$BACKUP_FILE" -C / 2>/dev/null || { echo -e "${RED}Extract failed${NC}"; exit 1; }

# The backup is structured as:
# ParanoidX-backup/codebase/...
# ParanoidX-backup/data/...
# ParanoidX-backup/bin/...
# ParanoidX-backup/flutter/...
# ParanoidX-backup/config/...

SRC="/ParanoidX-backup"

# Restore codebase
if [ -d "${SRC}/codebase" ]; then
  echo -e "${GREEN}Restoring codebase...${NC}"
  rsync -a --delete "${SRC}/codebase/" "$PROJECT_DIR/" 2>/dev/null || cp -af "${SRC}/codebase/" "$PROJECT_DIR/"
fi

# Restore data dir
if [ -d "${SRC}/data" ]; then
  echo -e "${GREEN}Restoring data...${NC}"
  mkdir -p "$HOME/.local/share"
  rsync -a --delete "${SRC}/data/" "$HOME/.local/share/simplex-node/" 2>/dev/null || cp -af "${SRC}/data/" "$HOME/.local/share/simplex-node/"
fi

# Restore binary
if [ -f "${SRC}/bin/ParanoidX" ]; then
  echo -e "${GREEN}Restoring binary...${NC}"
  mkdir -p "$HOME/bin"
  cp -f "${SRC}/bin/ParanoidX" "$HOME/bin/ParanoidX" && chmod +x "$HOME/bin/ParanoidX"
fi

# Restore Flutter
if [ -d "${SRC}/flutter" ]; then
  echo -e "${GREEN}Restoring Flutter...${NC}"
  mkdir -p "$HOME/.local/bin"
  rsync -a --delete "${SRC}/flutter/" "$HOME/.local/bin/the-isle/" 2>/dev/null || cp -af "${SRC}/flutter/" "$HOME/.local/bin/the-isle/"
fi

# Restore config
if [ -d "${SRC}/config" ]; then
  echo -e "${GREEN}Restoring config...${NC}"
  cp -af "${SRC}/config/" "$HOME/.config/opencode/" 2>/dev/null || true
  [ -f "${SRC}/config/AGENTS.md" ] && cp -f "${SRC}/config/AGENTS.md" "$PROJECT_DIR/AGENTS.md"
fi

# Cleanup extracted root
rm -rf "/ParanoidX-backup" 2>/dev/null || true

echo -e "\n${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Restore complete! Starting server...${NC}"
echo -e "${GREEN}══════════════════════════════════════════════${NC}"

# Start server
"$HOME/bin/ParanoidX" &>/tmp/ParanoidX.log &
sleep 5
VERSION=$(curl -s --max-time 3 http://localhost:8080/api/version 2>/dev/null | grep -oP '"build":"[^"]*"' || echo "check manually")
echo -e "${CYAN}Server: $VERSION${NC}"
echo -e "${GREEN}Done.${NC}"
