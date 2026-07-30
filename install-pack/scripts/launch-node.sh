#!/bin/bash
# =============================================================================
# КАНОНИЧЕСКИЙ ЛАУНЧЕР simplex-node
# ВСЕГДА запускай и перезапускай ноду ТОЛЬКО этой командой:
#   /home/tomas/simplex-node/scripts/launch-node.sh   (or via bot "launch")
#
# ПРЯМОЙ ЗАПУСК бинаря ( ./bin/simplex-node ... или nohup без этого скрипта )
# создаёт фоновые задачи, которые система отслеживает и шлёт уведомления
# "Background task ... completed" в этот чат. Это раздражает и засоряет контекст.
# Используй ТОЛЬКО этот скрипт — он делает pkill, чистый старт, свежий дашборд.
# =============================================================================

# Signal node-monitor that restart is intentional (touch-file with 30min expiry)
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
mkdir -p "$DATA_DIR"
touch -t "$(date -d '+30 minutes' '+%Y%m%d%H%M.%S')" "$DATA_DIR/.maintenance" 2>/dev/null || touch "$DATA_DIR/.maintenance"

# Очищаем прокси для прямых вызовов API Telegram и других сервисов
export NO_PROXY="localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.local,.onion,api.telegram.org"
export no_proxy="$NO_PROXY"

# Радио на USB — создаём symlink если его нет
USB_RADIO="/run/media/tomas/SIMPLEX-USB/radio"
LOCAL_RADIO="/home/tomas/.local/share/simplex-node/radio"
if [ -d "$USB_RADIO" ] && [ ! -L "$LOCAL_RADIO" ]; then
  [ -d "$LOCAL_RADIO" ] && mv "$LOCAL_RADIO" "${LOCAL_RADIO}.bak" 2>/dev/null
  ln -sf "$USB_RADIO" "$LOCAL_RADIO"
  echo "Radio symlinked to USB: $USB_RADIO"
fi

# Нативный xray вместо Docker V2Ray
XRAY_BIN="/home/tomas/bin/v2ray/xray"
XRAY_CONFIG="/home/tomas/bin/v2ray/config.json"
if [ -x "$XRAY_BIN" ] && [ -f "$XRAY_CONFIG" ]; then
  if ! pgrep -x xray >/dev/null 2>&1; then
    nohup "$XRAY_BIN" run -c "$XRAY_CONFIG" > /home/tomas/.local/share/simplex-node/logs/xray.log 2>&1 &
    echo "xray started (native, PID $!)"
  else
    echo "xray already running (native)"
  fi
  # Verify xray is serving SOCKS5
  sleep 1
  if ss -tlnp 2>/dev/null | grep -q :10810; then
    echo "xray SOCKS5 verified on :10810 ✓"
  else
    echo "WARNING: xray binary running but not listening on :10810"
  fi
fi

source "$(dirname "$0")/royal-common.sh" 2>/dev/null || true
: "${DATA:=$DATA_DIR}"
: "${BIN:=$BIN}"
: "${SRC_DASH:=$SIMPLEX_SRC/docker/dashboard.html}"
LOG=$DATA/logs/dashboard.log

# Island Royal Services setup (background — non-blocking)
if [ -x "$SCRIPTS_DIR/island-bot-setup.sh" ]; then
  echo "Preparing Island Services bot (background)..."
  nohup "$SCRIPTS_DIR/island-bot-setup.sh" >> $DATA/logs/island-bot-setup.log 2>&1 &
  disown
else
  echo "NOTE: island-bot-setup.sh missing"
fi

# Royal TG admin bot listener (background — non-blocking)
if [ -x "$SCRIPTS_DIR/launch-bot-listener.sh" ]; then
  echo "Starting royal Telegram listener (background)..."
  nohup "$SCRIPTS_DIR/launch-bot-listener.sh" >> $DATA/logs/bot-listener.log 2>&1 &
  disown
else
  echo "NOTE: launch-bot-listener.sh missing"
fi

mkdir -p $DATA/logs

# Disk preflight (from the 100% full incident). Warn loudly if root or key dirs are low.
# This runs before any cp / launch. Complements the Go background checker + /api/disk-check + TG alerts.
ROOT_AVAIL=$(df -m / 2>/dev/null | awk 'NR==2 {print $4}' || echo 0)
DATA_MB=$(du -sm $DATA 2>/dev/null | awk '{print $1}' || echo 0)
VAULT_USER_MB=$(du -sm --exclude='.*' $DATA/vault 2>/dev/null | awk '{print $1}' || echo 0)
BACKUPS_MB=$(du -sm /home/tomas/A1-backups 2>/dev/null | awk '{print $1}' || echo 0)
if [ "$ROOT_AVAIL" -lt 5000 ] || [ "$VAULT_USER_MB" -gt 1950 ] || [ "$DATA_MB" -gt 10000 ] || [ "$BACKUPS_MB" -gt 10000 ]; then
  echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
  echo ">>> ВНИМАНИЕ: МАЛО МЕСТА НА ДИСКЕ (или vault/data/backups раздулись) <<<"
  echo "    root avail: ${ROOT_AVAIL}MB | data: ${DATA_MB}MB | vault_user: ${VAULT_USER_MB}MB | backups: ${BACKUPS_MB}MB"
  echo "    Рекомендация: после запуска используй 'bot disk' / 'disk_check' или dashboard 'Проверить диск + алерт'."
  echo "    .reserved (2GB) — это намеренный контейнер-резервация под квоту Vault (по дизайну). В будущем Vault + .reserved переедут на отдельный большой RAID-массив (десятки ТБ с защитой). Здесь считаем только реальные user-данные (без .reserved). Бэкапы версий — частая причина давления на диск."
  echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
fi

# Aggressive clean: kill by name + any process bound to 8080 (prevents orphans)
pkill -x simplex-node 2>/dev/null || true
sleep 0.3
if command -v fuser >/dev/null 2>&1; then
  fuser -k 8080/tcp 2>/dev/null || true
fi
pkill -f 'simplex-node -listen' 2>/dev/null || true
sleep 0.5

# Best-effort refresh of the served dashboard.html so the owner always gets the rich Treasury UI
if [ -r "$SRC_DASH" ]; then
  if cp -f "$SRC_DASH" "$DATA/dashboard.html" 2>/dev/null; then
    chown tomas:tomas "$DATA/dashboard.html" 2>/dev/null || true
    echo "dashboard.html refreshed from source (rich Island/royal/black-hole UI active)"
  else
    echo "NOTE: could not overwrite $DATA/dashboard.html (probably root owned)."
    echo "      Run this once to get the full new UI with Register forms etc:"
    echo "      sudo cp $SRC_DASH $DATA/dashboard.html && sudo chown tomas:tomas $DATA/dashboard.html"
  fi
fi

# Launch node binary — exec replaces shell, systemd tracks the real PID
echo "Launching simplex-node (data=$DATA)..."
echo "Dashboard will be at http://127.0.0.1:8080"
echo "================================================================"
exec $BIN -config $DATA/simplex-node.json
