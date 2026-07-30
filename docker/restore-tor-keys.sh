#!/bin/bash
# restore-tor-keys.sh
# Защита от потери onion-адресов при перезапуске.
# Восстанавливает ключи скрытых сервисов Tor из бекапа,
# если bind mount директория оказалась пустой.
# Запускать ПЕРЕД docker compose up.
#
# Бекап: /home/tomas/.local/share/ParanoidX/tor-keys-backup/
# (вне git-репозитория, не удаляется при git clean)

set -e
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_BASE="/home/tomas/.local/share/ParanoidX/tor-keys-backup"
SERVICES=("smp" "xftp" "dashboard" "ice" "auditor")
RESTORED=0

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log()    { echo -e "[tor-keys $(date '+%H:%M:%S')] $*"; }
success(){ echo -e "${GREEN}[tor-keys] $*${NC}"; }
warn()   { echo -e "${YELLOW}[tor-keys] $*${NC}"; }
error()  { echo -e "${RED}[tor-keys] $*${NC}"; }

for svc in "${SERVICES[@]}"; do
    BIND_DIR="$SCRIPT_DIR/tor/hidden_services/$svc"
    KEY_FILE="$BIND_DIR/hs_ed25519_secret_key"
    BACKUP_DIR="$BACKUP_BASE/$svc"

    if [ -f "$KEY_FILE" ]; then
        log "$svc: ключ есть в bind mount ✓ ($(cat "$BIND_DIR/hostname" 2>/dev/null))"
        continue
    fi

    log "$svc: ключ отсутствует в bind mount!"

    if [ -f "$BACKUP_DIR/hs_ed25519_secret_key" ]; then
        log "$svc: восстанавливаю из бекапа..."
        mkdir -p "$BIND_DIR"
        cp "$BACKUP_DIR/hs_ed25519_secret_key" "$BIND_DIR/" || true
        cp "$BACKUP_DIR/hs_ed25519_public_key" "$BIND_DIR/" 2>/dev/null || true
        cp "$BACKUP_DIR/hostname" "$BIND_DIR/" 2>/dev/null || true
        chmod 700 "$BIND_DIR" 2>/dev/null || true
        chmod 600 "$BIND_DIR"/* 2>/dev/null || true
        restored_onion=$(cat "$BIND_DIR/hostname" 2>/dev/null)
        success "$svc: восстановлен адрес $restored_onion"
        RESTORED=$((RESTORED + 1))
    else
        warn "$svc: бекап не найден в $BACKUP_DIR/"
        warn "$svc: Tor сгенерирует новый адрес при старте"
    fi
done

if [ $RESTORED -gt 0 ]; then
    success "Восстановлено $RESTORED адресов из бекапа"
else
    log "Все адреса в порядке, восстановление не требуется"
fi
