#!/usr/bin/env bash
# px-install.sh — установка ParanoidX на чистую машину под Ubuntu/Debian.
#
# Что ставит:
#   1. Системные пакеты (Go, Docker, xray, вспомогательное)
#   2. Собирает ноду из исходников → ~/bin/ParanoidX
#   3. Поднимает Docker-стек (tor, coturn, smp-server, xftp-server)
#   4. Ставит polkit-правило и wrapper для управления демоном без пароля
#   5. Ставит user-юниты: node-monitor, px-xray-refresh (12ч)
#   6. Собирает Flutter-приложения ( isle_app, royal_app) — если есть SDK
#   7. Создаёт конфиг из шаблона, БЕЗ секретов
#
# Идемпотентен: можно запускать повторно для обновления.
# Секреты НЕ хранятся в репозитории — см. px-config.template.json.
set -euo pipefail

REPO="${REPO:-$HOME/simplex-node}"
BIN="$HOME/bin"
DATA="$HOME/.local/share/simplex-node"
XDIR="$BIN/v2ray"
LOGDIR="$HOME/.local/share/ParanoidX.logs"
FLUTTER_DIR="${FLUTTER_DIR:-/mnt/data/Export}"
DRY_RUN="${DRY_RUN:-0}"

say()  { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m[warn] %s\033[0m\n' "$*"; }
die()  { printf '\033[1;31m[fail] %s\033[0m\n' "$*" >&2; exit 1; }
run()  { if [ "$DRY_RUN" = 1 ]; then echo "  (dry-run) $*"; else "$@"; fi; }

[ "$(id -u)" -eq 0 ] && die "Запускай от обычного пользователя. sudo нужно только внутри скрипта (есть)."
[ -d "$REPO" ] || die "Репозиторий не найден: $REPO (задай REPO=...)"

# ── 1. Системные пакеты ───────────────────────────────────────────────────
say "1/7 Системные пакеты"
PKGS=(build-essential git curl unzip ca-certificates
      golang-go docker.io docker-compose-v2 python3 python3-venv
      jq socat tor openssl ca-certificates)
MISSING=()
for p in "${PKGS[@]}"; do
  case "$p" in
    golang-go)      command -v go       >/dev/null || MISSING+=("$p");;
    docker.io)      command -v docker   >/dev/null || MISSING+=("$p");;
    docker-compose-v2) command -v "docker compose" >/dev/null || MISSING+=("$p");;
    *)              dpkg -s "$p" >/dev/null 2>&1 || MISSING+=("$p");;
  esac
done
if [ ${#MISSING[@]} -gt 0 ]; then
  echo "  ставлю: ${MISSING[*]}"
  run sudo apt-get update -qq
  run sudo apt-get install -y -qq "${MISSING[@]}"
else
  echo "  все пакеты на месте"
fi
if ! command -v go >/dev/null; then die "Go не установился"; fi
echo "  go:       $(go version | awk '{print $3}')"
echo "  docker:   $(docker --version 2>/dev/null | awk '{print $3}' | tr -d ,)"
echo "  python:   $(python3 --version | awk '{print $2}')"

# ── 2. Сборка ноды ───────────────────────────────────────────────────────
say "2/7 Сборка ParanoidX из исходников"
mkdir -p "$BIN"
cd "$REPO"
echo "  go build ./cmd/ParanoidX/"
run go build -o "$BIN/ParanoidX" ./cmd/ParanoidX/
[ -x "$BIN/ParanoidX" ] || die "сборка не дала бинарь"
echo "  → $BIN/ParanoidX ($(stat -c%s "$BIN/ParanoidX") байт)"

# ── 3. Xray ──────────────────────────────────────────────────────────────
say "3/7 Xray"
mkdir -p "$XDIR" "$LOGDIR"
if [ ! -x "$XDIR/xray" ]; then
  warn "бинарь xray не найден — ставлю официальный"
  run sudo apt-get install -y -qq xray
  if [ -x /usr/bin/xray ]; then
    run cp /usr/bin/xray "$XDIR/xray"
  fi
fi
if [ -x "$XDIR/xray" ]; then
  echo "  xray: $(  "$XDIR/xray" version 2>/dev/null | head -1)"
else
  warn "xray отсутствует — подписка и прокси не заработают"
fi

# ── 4. Docker-стек ───────────────────────────────────────────────────────
say "4/7 Docker-стек (tor, coturn, smp, xftp)"
if command -v docker >/dev/null; then
  run sudo systemctl enable --now docker
  cd "$REPO/docker"
  if [ -f docker-compose.yml ]; then
    run docker compose up -d
    sleep 3
    echo "  контейнеры:"
    docker compose ps --format '    {{.Name}}  {{.State}}' 2>/dev/null || true
  else
    warn "нет docker/docker-compose.yml"
  fi
else
  warn "docker не установлен — стек SimpleX не поднимется"
fi

# ── 5. Управление демоном без пароля (polkit) ───────────────────────────
say "5/7 Polkit-правило для управления демоном"
# Тонкое правило: node-monitor может только start/stop/restart
# ParanoidX-dashboard.service. Никакого общего sudo.
if [ -f "$REPO/deploy/10-px-node-control.rules" ]; then
  # НЕ трогаем права каталога: /etc/polkit-1/rules.d у Ubuntu идёт 0750
  # root:polkitd, и это штатная защита — ослаблять её до 0755 не нужно.
  # `install -d -m` перезаписал бы её.
  if [ "$DRY_RUN" = 1 ]; then
    echo "  (dry-run) подставил бы пользователя: $(id -un)"
  else
    sudo install -d -m 0750 -o root -g polkitd /etc/polkit-1/rules.d 2>/dev/null \
      || sudo install -d -m 0755 /etc/polkit-1/rules.d
    sed "s/PX_USER_PLACEHOLDER/$(id -un)/" \
        "$REPO/deploy/10-px-node-control.rules" \
      | sudo tee /etc/polkit-1/rules.d/10-px-node-control.rules >/dev/null
    sudo chmod 0644 /etc/polkit-1/rules.d/10-px-node-control.rules
    echo "  правило установлено для пользователя $(id -un)"
  fi
else
  warn "нет deploy/10-px-node-control.rules — кнопки OFF/ON демона не заработают"
fi
if [ -f "$REPO/deploy/px-node-control" ]; then
  run sudo install -m 0755 "$REPO/deploy/px-node-control" /usr/local/bin/px-node-control
  echo "  wrapper установлен"
fi

# ── 6. User-юниты ───────────────────────────────────────────────────────
say "6/7 systemd user-юниты (монитор + 12-часовой таймер)"
mkdir -p "$HOME/.config/systemd/user"
# Юниты в deploy/ переносимые: пути записаны как PX_REPO/PX_HOME.
# Подставляем реальные значения, иначе на другой машине systemd
# не найдёт ни node-monitor.py, ни xray-sub.py.
for u in node-monitor.service px-xray-refresh.service px-xray-refresh.timer; do
  if [ -f "$REPO/deploy/$u" ]; then
    if [ "$DRY_RUN" = 1 ]; then
      echo "  (dry-run) подставил бы пути в $u"
    else
      sed -e "s|PX_REPO|$REPO|g" -e "s|PX_HOME|$HOME|g" \
          "$REPO/deploy/$u" > "$HOME/.config/systemd/user/$u"
      echo "  установлен: $u"
    fi
  else
    warn "нет deploy/$u"
  fi
done
run systemctl --user daemon-reload
# Монитор требует GUI (GTK-трей). На headless-машине не запускаем.
if [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
  run systemctl --user enable --now node-monitor.service
  echo "  node-monitor: $(systemctl --user is-active node-monitor.service)"
else
  warn "нет DISPLAY — монитор с треем не запущен (это нормально для headless)"
fi
run systemctl --user enable --now px-xray-refresh.timer
echo "  таймер подписки:"
systemctl --user list-timers px-xray-refresh.timer --no-pager 2>/dev/null | head -3 || true

# ── 7. Flutter-приложения ───────────────────────────────────────────────
say "7/7 Flutter-приложения (isle_app, royal_app)"
if command -v flutter >/dev/null; then
  echo "  flutter: $(flutter --version 2>/dev/null | head -1)"
  # Тулчейн для `flutter build linux`. Без него Flutter выдаёт
  # "CMake was unable to find a build program corresponding to Ninja"
  # и молча не собирает приложение — а скрипт решил бы, что сборка
  # прошла, если бы проверял только наличие каталога bundle.
  NEED_TOOLS=(clang cmake ninja pkg-config libgtk-3-dev)
  MISSING_TOOLS=()
  for t in "${NEED_TOOLS[@]}"; do
    case "$t" in
      libgtk-3-dev) dpkg -s libgtk-3-dev >/dev/null 2>&1 || MISSING_TOOLS+=("$t");;
      *)            command -v "$t" >/dev/null || MISSING_TOOLS+=("$t");;
    esac
  done
  if [ ${#MISSING_TOOLS[@]} -gt 0 ]; then
    echo "  ставлю тулчейн: ${MISSING_TOOLS[*]}"
    run sudo apt-get install -y -qq "${MISSING_TOOLS[@]}"
  else
    echo "  тулчейн на месте (clang, cmake, ninja, libgtk-3-dev)"
  fi
  for app in isle_app royal_app; do
    SRC=""
    # $REPO/apps/$app ПЕРВЫМ: исходники в репозитории — единственный
    # источник, который есть на свежом клоне. $FLUTTER_DIR (/mnt/data/Export)
    # — локальный архив этой машины; проверять его первым нельзя, иначе
    # установщик соберёт приложение из старой копии вместо той, что в git,
    # и расхождение всплывёт позже. Он остаётся как fallback.
    for cand in "$REPO/apps/$app" "$FLUTTER_DIR/The-Isle/$app" "$FLUTTER_DIR/Royal-Isle/$app"; do
      [ -f "$cand/pubspec.yaml" ] && { SRC="$cand"; break; }
    done
    if [ -z "$SRC" ]; then
      warn "$app: исходники не найдены (искал в $REPO/apps и $FLUTTER_DIR)"
      continue
    fi
    echo "  $app ← $SRC"
    # pub get --offline, если lock-файл есть и кэш на месте: на машине без
    # сети (а Tor не пускает pub.dev) обычный pub get падает.
    ( cd "$SRC" && if [ -f pubspec.lock ]; then
        run flutter pub get --offline >/dev/null 2>&1 \
          || run flutter pub get >/dev/null
      else
        run flutter pub get >/dev/null
      fi
      run flutter build linux --release >/dev/null )
    BUILT="$SRC/build/linux/x64/release/bundle"
    # Проверяем КОМПЛЕКТНОСТЬ, а не наличие каталога. Launcher Flutter —
    # это ~25 КБ ELF, а весь код живёт в lib/libapp.so (~8 МБ) и
    # lib/libflutter_linux_gtk.so (~17 МБ). Каталог bundle может
    # существовать, но быть пустым (так и у isle_app на этой машине) —
    # тогда «сборка успешна» была бы ложью.
    if [ -x "$BUILT/$app" ] && [ -f "$BUILT/lib/libapp.so" ] \
       && [ -f "$BUILT/lib/libflutter_linux_gtk.so" ]; then
      DEST="$BIN/the-isle"
      [ "$app" = royal_app ] && DEST="$BIN/the-royal"
      mkdir -p "$DEST"
      if [ "$DRY_RUN" = 1 ]; then
        echo "    (dry-run) скопировал бы bundle → $DEST/$app"
      else
        rm -rf "$DEST/$app"
        cp -r "$BUILT" "$DEST/$app"
        echo "    → $DEST/$app ($(du -sh "$DEST/$app" | cut -f1))"
      fi
    else
      warn "    bundle неполный: нет $app и/или lib/libapp.so, lib/libflutter_linux_gtk.so"
      warn "    приложение НЕ установлено — сначала разберись со сборкой"
    fi
  done
else
  warn "Flutter SDK не найден — приложения isle/royal нужно собрать вручную"
fi

# ── Конфиг ───────────────────────────────────────────────────────────────
say "Конфигурация"
mkdir -p "$DATA"
if [ ! -f "$DATA/simplex-node.json" ]; then
  if [ -f "$REPO/deploy/simplex-node.json.template" ]; then
    cp "$REPO/deploy/simplex-node.json.template" "$DATA/simplex-node.json"
    echo "  создан шаблон: $DATA/simplex-node.json"
  fi
  warn "ЗАПОЛНИ СЕКРЕТЫ вручную: $DATA/simplex-node.json"
  warn "  (ask_steward_token, dark_pushkin_token, torquemada_token, inquisitor_token)"
else
  echo "  конфиг уже есть: $DATA/simplex-node.json"
fi

# ── Итог ─────────────────────────────────────────────────────────────────
say "Готово. Проверка:"
printf '  %-34s %s\n' "бинарь ноды"    "$([ -x "$BIN/ParanoidX" ] && echo ОК || echo НЕТ)"
printf '  %-34s %s\n' "xray"           "$([ -x "$XDIR/xray" ] && echo ОК || echo НЕТ)"
printf '  %-34s %s\n' "конфиг"         "$([ -f "$DATA/simplex-node.json" ] && echo ОК || echo 'НЕТ — заполни')"
printf '  %-34s %s\n' "монитор"        "$(systemctl --user is-active node-monitor.service 2>/dev/null || echo 'не запущен')"
printf '  %-34s %s\n' "таймер подписки" "$(systemctl --user is-active px-xray-refresh.timer 2>/dev/null || echo 'нет')"
printf '  %-34s %s\n' "контейнеры"     "$(docker ps -q 2>/dev/null | wc -l) шт"
echo
echo "  Запуск ноды вручную (systemd-юнита может не быть):"
echo "    cd $REPO && ./scripts/launch-node.sh"
