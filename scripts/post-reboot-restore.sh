#!/bin/bash
set -euo pipefail

# post-reboot-restore.sh
# Автоматический восстановление simplex-node после перезагрузки:
# 1) фикс прав
# 2) радио symlink на USB + xray native
# 3) запуск docker-стека
# 4) запуск дашборда
# 5) запуск телеграм-ботов
# 6) проверка Tor HS
# 7) быстрый тест
# 8) отчёт админу в Telegram

# Очищаем прокси для прямых вызовов API Telegram
export NO_PROXY="localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.local,.onion,api.telegram.org"
export no_proxy="$NO_PROXY"

# Радио на USB — symlink если USB подключён
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
    nohup "$XRAY_BIN" run -c "$XRAY_CONFIG" > /home/tomas/.local/share/ParanoidX.logs/xray.log 2>&1 &
    echo "xray started (native)"
  else
    echo "xray already running (native)"
  fi
fi

ADMIN_TOKEN=""
ADMIN_CHAT=""
if [ -f /home/tomas/.config/opencode-tg-bot.token ]; then
  ADMIN_TOKEN=$(cat /home/tomas/.config/opencode-tg-bot.token)
fi
if [ -f /home/tomas/.config/opencode-tg-bot.chat ]; then
  ADMIN_CHAT=$(cat /home/tomas/.config/opencode-tg-bot.chat)
fi

send_tg() {
  local text="$1"
  if [ -n "$ADMIN_TOKEN" ] && [ -n "$ADMIN_CHAT" ]; then
    curl -s -X POST "https://api.telegram.org/bot${ADMIN_TOKEN}/sendMessage" \
      -H "Content-Type: application/json" \
      -d "{\"chat_id\":${ADMIN_CHAT},\"text\":$(printf '%s' "$text" | python3 -c 'import sys,json;print(json.dumps(sys.stdin.read()))')}" >/dev/null 2>&1 || true
  fi
}

LOG="/home/tomas/.local/share/ParanoidX.logs/post-reboot-restore.log"
mkdir -p "$(dirname "$LOG")"
exec > >(tee -a "$LOG") 2>&1

echo "===== POST-REBOOT RESTORE START $(date) ====="
send_tg "🔄 Post-reboot restore started at $(date '+%H:%M:%S')"

# 1) Fix permissions
echo "[1/10] Fixing permissions..."
chown -R tomas:tomas /home/tomas/.local/share/simplex-node 2>/dev/null || true
chown -R tomas:tomas /home/tomas/ParanoidX 2>/dev/null || true
chmod +x /home/tomas/ParanoidX/scripts/*.sh /home/tomas/ParanoidX/scripts/*.py 2>/dev/null || true
chmod +x /home/tomas/bin/ParanoidX 2>/dev/null || true
echo "permissions fixed"
send_tg "✅ Permissions fixed"

# 2) Start docker stack (tor, smp, xftp, coturn — v2ray заменён на нативный xray)
echo "[2/10] Starting docker stack..."
cd /home/tomas/ParanoidX/docker
if command -v docker >/dev/null 2>&1; then
  if docker compose up -d --build; then
    echo "docker stack started (tor, smp, xftp, coturn)"
    send_tg "✅ Docker stack started (tor, smp, xftp, coturn)"
  else
    echo "docker stack FAILED"
    send_tg "❌ Docker stack failed to start"
    exit 1
  fi
else
  echo "docker not found, skip"
  send_tg "⚠️ Docker not found, skip"
fi

# 3) Wait for Tor
echo "[3/10] Waiting for Tor hidden services..."
sleep 5
if pgrep -x tor >/dev/null 2>&1; then
  echo "tor is running"
  send_tg "✅ Tor is running"
else
  echo "tor NOT running"
  send_tg "❌ Tor is NOT running"
fi

# 4) Start dashboard via launch-node.sh (canonical launcher)
echo "[4/10] Starting dashboard..."
if pgrep -x ParanoidX >/dev/null 2>&1; then
  echo "simplex-node already running (PID $(pgrep -x ParanoidX))"
  send_tg "✅ Dashboard already running"
else
  echo "starting simplex-node via launch-node.sh..."
  if [ -x /home/tomas/ParanoidX/scripts/launch-node.sh ]; then
    bash /home/tomas/ParanoidX/scripts/launch-node.sh
    sleep 4
    if pgrep -x ParanoidX >/dev/null 2>&1; then
      echo "dashboard UP (PID $(pgrep -x ParanoidX))"
      send_tg "✅ Dashboard started via launch-node.sh"
    else
      echo "dashboard DOWN"
      send_tg "❌ Dashboard is DOWN (check logs)"
    fi
  else
    echo "launch-node.sh missing, starting binary directly..."
    pkill -x ParanoidX 2>/dev/null || true
    sleep 1
    if [ -x /home/tomas/bin/ParanoidX ]; then
      nohup /home/tomas/bin/ParanoidX \
        >> /home/tomas/.local/share/ParanoidX.logs/dashboard.log 2>&1 &
      echo $! > /tmp/simplex-node.pid
      sleep 4
      if pgrep -x ParanoidX >/dev/null 2>&1; then
        echo "dashboard UP (manual)"
  send_tg "✅ Dashboard started on 0.0.0.0:8080"
        else
          echo "dashboard DOWN"
          send_tg "❌ Dashboard is DOWN (check /tmp/ParanoidX.log)"
        fi
      else
        echo "simplex-node binary missing"
        send_tg "❌ simplex-node binary missing"
      fi
    fi
  fi
fi

# 5) Start node-monitor (system tray — Wayland-indicator via AyatanaAppIndicator3)
echo "[5/10] Starting node-monitor..."
if pgrep -f "node-monitor.py" >/dev/null 2>&1; then
  MON_PID=$(pgrep -f "node-monitor.py" | head -1)
  echo "  node-monitor already running (PID $MON_PID)"
  send_tg "✅ node-monitor already running"
else
  if [ -f /home/tomas/ParanoidX/node-monitor.py ]; then
    GI_TYPELIB_PATH=/home/tomas/.local/share/girepository-1.0
    export GI_TYPELIB_PATH
    DISPLAY=:0 nohup /usr/bin/python3 /home/tomas/ParanoidX/node-monitor.py \
      > /home/tomas/.local/share/ParanoidX.logs/node-monitor.log 2>&1 &
    echo $! > /tmp/node-monitor.pid
    echo "  node-monitor started (PID $(cat /tmp/node-monitor.pid))"
    send_tg "✅ node-monitor started"
  else
    echo "  node-monitor.py not found, skip"
    send_tg "⚠️ node-monitor.py missing"
  fi
fi

# 6) Start Flutter client (supervised)
echo "[6/10] Starting Flutter client..."
FLUTTER_PID=$(pgrep -f 'isle_app$' 2>/dev/null || true)
if [ -n "$FLUTTER_PID" ]; then
  echo "  Flutter already running (PID $FLUTTER_PID)"
  send_tg "✅ Flutter client already running"
else
  if [ -f /home/tomas/ParanoidX/scripts/run-isle-app.sh ]; then
    DISPLAY=:0 nohup bash /home/tomas/ParanoidX/scripts/run-isle-app.sh \
      > /tmp/isle_app_supervisor.log 2>&1 &
    echo $! > /tmp/isle-app-supervisor.pid
    echo "  Flutter supervisor started (PID $(cat /tmp/isle-app-supervisor.pid))"
    send_tg "✅ Flutter client supervisor started"
  else
    echo "  run-isle-app.sh not found, skip"
    send_tg "⚠️ Flutter supervisor script missing"
  fi
fi

# 7) Health check
echo "[7/10] Health check..."
if pgrep -x ParanoidX >/dev/null 2>&1; then
  echo "  simplex-node running (PID $(pgrep -x ParanoidX))"
  HEALTH=$(curl -s --max-time 3 http://127.0.0.1:8080/api/health 2>/dev/null || echo '{"healthy":false}')
  HEALTHY=$(echo "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin).get('healthy',False))" 2>/dev/null)
  if [ "$HEALTHY" = "True" ]; then
    echo "  API health: OK"
    send_tg "✅ API health: OK"
  else
    echo "  API health: DEGRADED ($HEALTH)"
    send_tg "⚠️ API health: DEGRADED"
  fi
else
  echo "  simplex-node NOT running"
  send_tg "❌ simplex-node NOT running"
fi

# 8) Quick test
echo "[8/10] Running quick tests..."
if pgrep -x ParanoidX >/dev/null 2>&1; then
  if [ -x /home/tomas/ParanoidX/scripts/test-royal.sh ]; then
    TEST_OUT=$(/home/tomas/ParanoidX/scripts/test-royal.sh 2>&1 | tail -20 || true)
    echo "$TEST_OUT"
    if echo "$TEST_OUT" | grep -q "PASS"; then
      send_tg "✅ Quick test: PASS"
    else
      send_tg "⚠️ Quick test: see log"
    fi
  else
    echo "test-royal.sh missing"
    send_tg "⚠️ test-royal.sh missing"
  fi
else
  echo "skip tests — dashboard not running"
fi

# 9) Mount USB backup drive if available
echo "[9/10] Checking USB backup drive..."
if lsblk -o LABEL 2>/dev/null | grep -q SIMPLEX-BACKUP; then
  USB_DEV=$(lsblk -o NAME,LABEL 2>/dev/null | awk '/SIMPLEX-BACKUP/ {print $1}' | sed 's/^/\/dev\//')
  if [ -n "$USB_DEV" ] && ! mount | grep -q "$USB_DEV"; then
    mkdir -p /mnt/simplex-backup
    mount "$USB_DEV" /mnt/simplex-backup 2>/dev/null && echo "USB backup drive mounted at /mnt/simplex-backup" && send_tg "✅ USB backup drive mounted"
  else
    echo "USB backup drive already mounted or not found"
  fi
else
  echo "No SIMPLEX-BACKUP USB drive detected"
fi

# 10) Send comprehensive startup report to Inquisitor
echo "[10/10] Sending startup report to Inquisitor..."
STARTUP_REPORT_SCRIPT="/home/tomas/ParanoidX/scripts/startup-report.sh"
if [ -x "$STARTUP_REPORT_SCRIPT" ]; then
  bash "$STARTUP_REPORT_SCRIPT" 2>&1 || echo "startup report failed (non-fatal)"
else
  echo "  startup-report.sh not found, skip"
fi

echo "===== POST-REBOOT RESTORE COMPLETE $(date) ====="
send_tg "🏁 Post-reboot restore complete at $(date '+%H:%M:%S')"
exit 0
