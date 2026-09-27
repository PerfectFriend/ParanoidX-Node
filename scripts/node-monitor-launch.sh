#!/usr/bin/env bash
# Запускает node-monitor.py, дождавшись рабочей графической сессии.
#
# Зачем: раньше node-monitor.service стартовал по WantedBy=default.target.
# Вместе с Linger=yes это означало запуск ДО графического входа — X ещё не
# поднят, Xauthority отсутствует, и GTK падал с
#   "Can't create a GtkStyleContext without a display connection"
#   Main process exited, code=dumped, status=6/ABRT
# systemd делал 5 попыток и сдавался: после ребута монитора не было.
#
# Теперь юнит привязан к graphical-session.target, но номер DISPLAY всё
# равно нельзя зашивать: он меняется между сессиями. Этот скрипт находит
# живой X-сервер сам.
set -u

MAX_WAIT="${PX_MONITOR_DISPLAY_WAIT:-60}"
log() { printf '%s node-monitor-launch: %s\n' "$(date -Is)" "$*" >&2; }

# Сессия может быть Wayland (GNOME — обычно так) или X11.
# На Wayland Xauthority нет в принципе: там свой протокол, и Xwayland
# существует лишь для совместимости. Поэтому сначала пробуем Wayland —
# тогда DISPLAY/XAUTHORITY вообще не нужны. X — запасной путь.
runtime="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

find_wayland() {
  local sock
  for sock in "$runtime"/wayland-*; do
    case "$(basename "$sock")" in
      wayland-*.lock) continue;;
    esac
    [ -S "$sock" ] && { printf '%s' "$(basename "$sock")"; return 0; }
  done
  return 1
}

find_x11() {
  local d xauth
  xauth="${XAUTHORITY:-$HOME/.Xauthority}"
  for d in "${DISPLAY:-}" :0 :1 :2; do
    [ -n "$d" ] || continue
    # timeout ОБЯЗАТЕЛЕН: xdpyinfo на несуществующем display (например
    # :1, когда X ещё не поднят) не отказывает сразу — он висит, пока
    # клиент сам не сдастся. Без timeout launcher простаивал бы по 20 с
    # на каждой попытке и не успевал дождаться графической сессии.
    if XAUTHORITY="$xauth" DISPLAY="$d" timeout 2 xdpyinfo >/dev/null 2>&1; then
      printf '%s' "$d"; return 0
    fi
  done
  return 1
}

# 1. Ждём появления графической сессии: при старте после ребута
#    (Linger=yes) её ещё может не быть.
deadline=$(( SECONDS + MAX_WAIT ))
BACKEND=""; WL=""; DISP=""
while [ "$SECONDS" -lt "$deadline" ]; do
  if WL="$(find_wayland)"; then BACKEND=wayland; break; fi
  if DISP="$(find_x11)"; then    BACKEND=x11;    break; fi
  sleep 2
done

if [ -z "$BACKEND" ]; then
  log "графическая сессия не появилась за ${MAX_WAIT}s — выхожу без аварии"
  log "монитор GUI требует активной сессии; нода, таймеры и xray работают без него"
  exit 0
fi

# 2. Готовим окружение для GTK.
export GDK_BACKEND="$BACKEND"
if [ "$BACKEND" = wayland ]; then
  export WAYLAND_DISPLAY="$WL"
  # Xwayland нужен GTK-приложениям, но его сокет живёт не в XDG_RUNTIME_DIR,
  # поэтому DISPLAY здесь не указываем — GDK_BACKEND решает.
  log "сессия Wayland ($WL) — запускаю монитор"
else
  export DISPLAY="$DISP"
  export XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"
  export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}"
  log "сессия X11 (DISPLAY=$DISP XAUTHORITY=$XAUTHORITY) — запускаю монитор"
fi

export GI_TYPELIB_PATH="${GI_TYPELIB_PATH:-$HOME/.local/share/girepository-1.0}"
export XDG_RUNTIME_DIR="$runtime"
export PYTHONUNBUFFERED=1
# urllib не умеет socks5 (в окружении прописан Tor-прокси) — локальный
# API надо опрашивать напрямую.
export NO_PROXY="localhost,127.0.0.1,::1"
export no_proxy="$NO_PROXY"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

# КРИТИЧНО: убрать PYTHONPATH. В окружении Hermes он указывает на
# venv python3.11, и монитор импортировал PIL ОТТУДА:
#   ImportError: cannot import name '_imaging' from 'PIL'
#   (/home/tomas/.hermes/hermes-agent/venv/lib/python3.11/site-packages/PIL)
# Собранный под 3.11 `_imaging` не грузится в системном 3.14, и падал
# весь процесс с status=1. В мониторе нужны только gi и PIL из
# системных пакетов (python3-gi, python3-pil), которые в venv отсутствуют.
unset PYTHONPATH

exec /usr/bin/python3 /home/tomas/simplex-node/node-monitor.py
