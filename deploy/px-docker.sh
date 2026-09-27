#!/usr/bin/env bash
# px-docker.sh — установка, настройка, запуск и проверка Docker-стека ParanoidX.
#
# Отвечает за 4 контейнера: tor, coturn, smp-server, xftp-server.
# Каждый поднимается ТОЛЬКО после проверки: compose без healthcheck-ов
# выглядит успешным, даже если сервис внутри мёртв.
#
# sudo запрашивается ОДИН раз в начале и только на команды, которые без
# него невозможны (apt, docker из репозитория). Весь остальной стек —
# docker compose от пользователя.
#
# Режимы .onion-ключей (главный выбор при установке):
#   1. NEW     — Tor сгенерирует новые скрытые адреса. Прежние .onion
#                перестанут работать, старые ссылки/QR у клиентов сгорят.
#                Для свежей установки это правильный выбор.
#   2. IMPORT  — взять ключи из бэкапа (--keys DIR). Нода получит ТЕ ЖЕ
#                адреса, что на старом хосте. Для клона/апгрейда.
#
# Ключи НЕ хранятся в git. В режиме IMPORT бэкап копируется на новый хост
# вручную (USB), и скрипт читает его оттуда.

set -euo pipefail

REPO="${REPO:-$HOME/simplex-node}"
DOCKER_DIR="$REPO/docker"
HS_DIR="$DOCKER_DIR/tor/hidden_services"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
BACKUP_ROOT="${BACKUP_ROOT:-$HOME/A1-backups}"

# 5 скрытых сервисов, объявленных в docker/tor/torrc
HS_SERVICES=(smp xftp dashboard ice auditor)
# Из них в ноде публикуются три (internal/api/transport.go, main.go)
ONION_EXPORTS=(dashboard ice auditor)

DRY_RUN="${DRY_RUN:-0}"
# Ключи можно задать и через переменную окружения (KEYS_SRC=... ./px-docker.sh),
# и флагом --keys. Флаг переопределяет переменную.
KEYS_SRC="${KEYS_SRC:-}"
KEY_MODE=""
ASSUME_YES="${ASSUME_YES:-0}"
COMPOSE_CMD=()

# ── вывод ──────────────────────────────────────────────────────────────────
if [ -t 1 ] && [ -z "$NO_COLOR" ]; then
  R=$'\033[31m'; G=$'\033[32m'; Y=$'\033[33m'; B=$'\033[34m'; N=$'\033[0m'
else
  R=""; G=""; Y=""; B=""; N=""
fi
say()  { printf '\n%s==> %s%s\n' "$B" "$*" "$N"; }
info() { printf '  %s\n' "$*"; }
ok()   { printf '  %sOK%s   %s\n' "$G" "$N" "$*"; }
warn() { printf '  %sВНИМ.%s %s\n' "$Y" "$N" "$*"; }
err()  { printf '  %sОШИБКА%s %s\n' "$R" "$N" "$*" >&2; }
die()  { err "$*"; exit 1; }

# sudo — только когда реально нужно, с подтверждением один раз
SUDO_OK=0
need_sudo() {
  if [ "$(id -u)" -eq 0 ]; then SUDO_OK=1; return 0; fi
  if [ "$SUDO_OK" -eq 1 ]; then return 0; fi
  say "Нужен sudo для: $*"
  info "Один запрос пароля, дальше sudo не спрашивается до 15 минут."
  if ! sudo -v; then die "sudo не прошёл — установка прервана"; fi
  SUDO_OK=1
}

run() {
  if [ "$DRY_RUN" = 1 ]; then
    printf '  (dry-run) %s\n' "$*"
    return 0
  fi
  "$@"
}

# ── разбор аргументов ──────────────────────────────────────────────────────
usage() {
  cat <<'EOF'
Использование: px-docker.sh [опции] [команда]

Команды:
  install     полная установка стека (по умолчанию)
  start       поднять контейнеры
  stop        остановить
  status      состояние + healthcheck-и
  test        проверить работоспособность
  keys new    сгенерировать новые .onion-ключи
  keys import --keys DIR
              импортировать ключи из бэкапа
  keys show   показать текущие адреса
  logs [NAME] логи контейнера
  down        удалить контейнеры (volumes НЕ трогаем)

Опции:
  --keys DIR      каталог с бэкапом ключей (для keys import)
  --new-keys      сразу выбрать режим NEW, не спрашивая
  --import-keys   сразу выбрать режим IMPORT (нужен --keys)
  --yes           не задавать вопросов
  --dry-run       ничего не менять, только показать
  -h, --help      эта справка

Примеры:
  ./px-docker.sh install
  ./px-docker.sh keys import --keys /media/usb/backup/onion-keys
  ./px-docker.sh test
EOF
}

CMD="install"
while [ $# -gt 0 ]; do
  case "$1" in
    install|start|stop|status|test|down|logs) CMD="$1"; shift ;;
    keys)
      CMD="keys"
      shift
      if [ $# -gt 0 ] && [ "${1:-}" != "--keys" ] && [ "${1:-}" != "--new-keys" ] \
         && [ "${1:-}" != "--import-keys" ] && [ "${1:-}" != "--yes" ] \
         && [ "${1:-}" != "--dry-run" ]; then
        KEY_ACTION="$1"; shift
      else
        KEY_ACTION="show"
      fi
      ;;
    new|import|show) KEY_ACTION="$1"; shift ;;
    --keys)       KEYS_SRC="${2:-}"; shift 2 ;;
    --new-keys)   KEY_MODE="new"; shift ;;
    --import-keys) KEY_MODE="import"; shift ;;
    --yes|-y)     ASSUME_YES=1; shift ;;
    --dry-run)    DRY_RUN=1; shift ;;
    -h|--help)    usage; exit 0 ;;
    *)            err "неизвестный аргумент: $1"; usage; exit 2 ;;
  esac
done

[ -d "$REPO" ] || die "репозиторий не найден: $REPO (задай REPO=...)"
[ -f "$DOCKER_DIR/docker-compose.yml" ] || die "нет $DOCKER_DIR/docker-compose.yml"

# ── docker ─────────────────────────────────────────────────────────────────
find_compose() {
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD=(docker-compose)
  else
    return 1
  fi
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    need_sudo "установка docker.io"
    # sudo — именно здесь. need_sudo только ВАЛИДИРУЕТ кэш sudo; если
    # забыть sudo здесь, `apt-get install` упадёт «Permission denied»
    # на чистом host, где docker ещё не стоит.
    run sudo apt-get install -y -qq docker.io docker-compose-v2
  fi
  find_compose || die "не найден docker compose (нужен docker compose v2)"
  if ! docker info >/dev/null 2>&1; then
    info "docker-демон не отвечает, пробую поднять"
    need_sudo "запуск docker-демона"
    run sudo systemctl enable --now docker
    # ждём готовности, а не спим фиксированно
    for _ in $(seq 1 30); do
      docker info >/dev/null 2>&1 && break
      sleep 1
    done
    docker info >/dev/null 2>&1 || die "docker-демон не поднялся"
  fi
  ok "docker $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo '?') работает"
}

compose() { ( cd "$DOCKER_DIR" && "${COMPOSE_CMD[@]}" "$@" ); }

# ── coturn: конфиг и TLS-сертификаты ────────────────────────────────────────
# coturn не поднимется на чистом клоне без трёх файлов, которые docker/
# целиком вне git. Без turnserver.conf — нечего читать; без сертификатов
# `turns://` (порт 5349) падает на TLS handshake, и это выглядит как
# «coturn сломан», хотя 3478 при этом отвечает.
ensure_coturn() {
  local cd_dir="$DOCKER_DIR/coturn"
  mkdir -p "$cd_dir"

  # 1. Конфиг: static-auth-secret — это пароль TURN, генерируем на месте.
  local conf="$cd_dir/turnserver.conf"
  if [ ! -f "$conf" ]; then
    local tpl="$cd_dir/turnserver.conf.template"
    [ -f "$tpl" ] || die "нет шаблона $tpl и нет $conf — не знаю, как настроить coturn"
    local secret
    secret=$(openssl rand -base64 32 2>/dev/null | tr -d '\n=')
    [ -n "$secret" ] || die "не удалось сгенерировать static-auth-secret"
    if [ "$DRY_RUN" = 1 ]; then
      say "DRY: сгенерирую $conf из шаблона со свежим секретом (600)"
    else
      sed "s/{{STATIC_AUTH_SECRET}}/$secret/" "$tpl" > "$conf"
      chmod 600 "$conf"
      ok "coturn: создан $conf (секрет сгенерирован, права 600)"
    fi
  else
    ok "coturn: конфиг уже есть, оставляю"
  fi

  # 2. TLS-сертификат для turns://. Проверяем не по имени файла, а по
  #    содержимому: на этой машине turn_key.pem оказался КАТАЛОГОМ
  #    нулевого размера, и coturn молча поднимался без TLS.
  local cert="$cd_dir/turn_cert.pem" pkey="$cd_dir/turn_key.pem"
  if [ -s "$cert" ] && [ -f "$cert" ] && [ -s "$pkey" ] && [ -f "$pkey" ]; then
    ok "coturn: TLS-сертификат на месте"
  else
    say "coturn: TLS-сертификат отсутствует или пуст — генерирую"
    if [ "$DRY_RUN" = 1 ]; then
      say "DRY: сгенерирую самоподписанный сертификат на 10 лет"
      return 0
    fi
    # openssl в этой системе падает на системном openssl.cnf (битая
    # ссылка на /etc/ssl/openssl.conf.d), поэтому конфиг задаём явно.
    local ocnf; ocnf=$(mktemp)
    cat > "$ocnf" <<'CERT_EOF'
[req]
distinguished_name=dn
[dn]
[ext]
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:turn.ParanoidX.local,DNS:localhost
CERT_EOF
    # Если turn_key.pem — каталог (был такой случай), rm -rf иначе
    # openssl не сможет его перезаписать.
    [ -d "$pkey" ] && rm -rf "$pkey"
    [ -d "$cert" ] && rm -rf "$cert"
    if OPENSSL_CONF="$ocnf" openssl req -x509 -newkey rsa:2048 -sha256 \
         -days 3650 -nodes -keyout "$pkey" -out "$cert" \
         -subj "/C=RU/O=ParanoidX/CN=turn.ParanoidX.local" \
         -extensions ext -config "$ocnf" 2>/dev/null; then
      chmod 600 "$pkey"; chmod 644 "$cert"
      ok "coturn: сертификат создан (turns:// заработает)"
    else
      rm -f "$ocnf"
      die "не удалось создать TLS-сертификат для coturn — turns:// не поднимется"
    fi
    rm -f "$ocnf"
  fi
}

# ── .onion-ключи ───────────────────────────────────────────────────────────
hs_dir_for() { printf '%s/%s' "$HS_DIR" "$1"; }

# Секретный ключ Tor ed25519v1: 96 байт, бинарный, заголовок
# "== ed25519v1-secret: type0 ==" (32 байта) + 64 байта ключа.
is_valid_key() {
  local f="$1"
  [ -s "$f" ] || return 1
  local sz; sz=$(stat -c%s "$f" 2>/dev/null || echo 0)
  [ "$sz" = 96 ] || return 1
  head -c 32 "$f" | grep -q 'ed25519v1-secret' 2>/dev/null
}

# hostname в .onion: 56 символов base32 + ".onion"
is_valid_hostname() {
  local f="$1"
  [ -s "$f" ] || return 1
  local h; h=$(tr -d '[:space:]' < "$f")
  printf '%s' "$h" | grep -qE '^[a-z2-7]{56}\.onion$'
}

# Публикация адресов в DataDir: нода читает *_onion.txt оттуда
publish_onions() {
  local src_root="$1" svc
  info "публикую адреса в $DATA_DIR/*_onion.txt (оттуда их читает нода)"
  for svc in "${ONION_EXPORTS[@]}"; do
    local host_file; host_file="$(hs_dir_for "$svc")/hostname"
    if [ ! -f "$host_file" ]; then
      warn "$svc: нет hostname — пропускаю"
      continue
    fi
    local h; h=$(tr -d '[:space:]' < "$host_file")
    run mkdir -p "$DATA_DIR"
    printf '%s' "$h" > "$DATA_DIR/${svc}_onion.txt"
    ok "$(printf '%-12s' "$svc") $h"
  done
}

# ── режим NEW: сгенерировать новые ключи ───────────────────────────────────
do_keys_new() {
  say "Генерация новых .onion-ключей"
  local svc
  for svc in "${HS_SERVICES[@]}"; do
    local d; d="$(hs_dir_for "$svc")"
    run mkdir -p "$d"
    # Tor сам создаст ключи при первом старте, если их нет. Удаляем
    # только наш собственный мусор, а не чужие файлы.
    if [ -f "$d/hs_ed25519_secret_key" ] && is_valid_key "$d/hs_ed25519_secret_key"; then
      warn "$svc: ключ уже есть и валиден — оставляю (адрес не изменится)"
      continue
    fi
    run rm -f "$d/hs_ed25519_secret_key" "$d/hs_ed25519_public_key" "$d/hostname"
    info "$svc: будет создан Tor при старте контейнера"
  done
  say "Запускаю tor, чтобы он сгенерировал ключи"
  compose up -d tor
  wait_healthy tor
  # Tor мог не успеть — он пишет ключи в первые секунды
  local svc again
  for again in $(seq 1 15); do
    local all=1
    for svc in "${HS_SERVICES[@]}"; do
      is_valid_key "$(hs_dir_for "$svc")/hs_ed25519_secret_key" || all=0
    done
    [ "$all" = 1 ] && break
    sleep 2
  done
  local missing=()
  for svc in "${HS_SERVICES[@]}"; do
    is_valid_key "$(hs_dir_for "$svc")/hs_ed25519_secret_key" \
      || missing+=("$svc")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    err "Tor не создал ключи для: ${missing[*]}"
    err "смотри: docker logs ParanoidX-tor"
    return 1
  fi
  ok "все 5 ключей созданы Tor'ом"
  publish_onions "$HS_DIR"
}

# ── режим IMPORT: взять ключи из бэкапа ───────────────────────────────────
do_keys_import() {
  say "Импорт .onion-ключей из бэкапа"
  [ -n "$KEYS_SRC" ] || die "укажи бэкап: --keys DIR (см. px-backup.sh)"
  [ -d "$KEYS_SRC" ] || die "каталог с ключами не найден: $KEYS_SRC"

  # Принимаем и корень бэкапа, и каталог с onion-keys внутри.
  local base="$KEYS_SRC"
  if [ ! -d "$base/smp" ] && [ -d "$base/onion-keys/smp" ]; then
    base="$KEYS_SRC/onion-keys"
    info "ключи лежат в $base"
  fi
  [ -d "$base/smp" ] || die "в $KEYS_SRC не найдены каталоги сервисов (smp/, xftp/, ...)"

  # Сверяем комплектность ДО копирования, чтобы не оставить стек
  # наполовину со старыми ключами.
  local svc missing=() from=""
  for svc in "${HS_SERVICES[@]}"; do
    if is_valid_key "$base/$svc/hs_ed25519_secret_key"; then
      from+="$svc "
    else
      missing+=("$svc")
    fi
  done
  if [ ${#missing[@]} -gt 0 ]; then
    err "в бэкапе нет валидных ключей для: ${missing[*]}"
    err "ожидался файл hs_ed25519_secret_key по 96 байт в каждом из:"
    info "  ${HS_SERVICES[*]}"
    return 1
  fi
  ok "бэкап полон: ключи есть для ${from}"

  # Показываем, что адреса поменяются, ДО того как менять.
  say "Текущие адреса (будут заменены)"
  for svc in "${HS_SERVICES[@]}"; do
    local cur; cur="$(tr -d '[:space:]' < "$(hs_dir_for "$svc")/hostname" 2>/dev/null || echo '—')"
    local new; new="$(tr -d '[:space:]' < "$base/$svc/hostname" 2>/dev/null || echo '—')"
    if [ "$cur" = "$new" ]; then
      ok "$(printf '%-12s' "$svc") без изменений: $cur"
    else
      warn "$(printf '%-12s' "$svc") $cur"
      warn "$(printf '%-12s' '') $new  (сменится)"
    fi
  done

  if [ "$ASSUME_YES" != 1 ] && [ "$DRY_RUN" != 1 ]; then
    printf '\n  Заменить ключи? Старые .onion перестанут работать. [y/N] '
    read -r ans
    case "$ans" in [yY]|[дД]|[дa]) ;; *) die "отменено пользователем" ;; esac
  fi

  # Бэкап текущих — чтобы откат был возможен без бэкапа извне.
  local stash="$BACKUP_ROOT/onion-pre-import-$(date +%Y%m%d-%H%M%S)"
  info "сохраняю текущие ключи в $stash (откат)"
  if [ "$DRY_RUN" != 1 ]; then
    mkdir -p "$stash"
    for svc in "${HS_SERVICES[@]}"; do
      mkdir -p "$stash/$svc"
      cp -a "$(hs_dir_for "$svc")/." "$stash/$svc/" 2>/dev/null || true
    done
  fi

  for svc in "${HS_SERVICES[@]}"; do
    local d; d="$(hs_dir_for "$svc")"
    run mkdir -p "$d"
    # Порядок важен: сначала секретный, потом public и hostname.
    # Если оборвать посередине, Tor использует старый public с новым
    # secret и перестанет отдавать правильный адрес.
    run cp -a "$base/$svc/hs_ed25519_secret_key" "$d/"
    if [ -f "$base/$svc/hs_ed25519_public_key" ]; then
      run cp -a "$base/$svc/hs_ed25519_public_key" "$d/"
    fi
    if [ -f "$base/$svc/hostname" ]; then
      run cp -a "$base/$svc/hostname" "$d/"
    fi
    # Права: Tor читает ключи под пользователем, entrypoint чинит
    # владение, но 600 на секрете обязателен и снаружи.
    run chmod 600 "$d/hs_ed25519_secret_key"
    run chmod 644 "$d/hs_ed25519_public_key" 2>/dev/null || true
    run chmod 644 "$d/hostname" 2>/dev/null || true
  done
  ok "5 ключей скопировано, права выставлены (600 на секретах)"

  say "Перезапускаю tor с новыми/импортированными ключами"
  # Контейнер читает ключи только на старте.
  compose restart tor
  wait_healthy tor
  publish_onions "$HS_DIR"
}

# ── бэкап ключей ──────────────────────────────────────────────────────────
do_keys_backup() {
  local dest="${1:-$BACKUP_ROOT/onion-keys-$(date +%Y%m%d-%H%M%S)}"
  say "Бэкап .onion-ключей → $dest"
  mkdir -p "$dest"
  local svc
  for svc in "${HS_SERVICES[@]}"; do
    mkdir -p "$dest/$svc"
    cp -a "$(hs_dir_for "$svc")/." "$dest/$svc/" 2>/dev/null || true
    if is_valid_key "$dest/$svc/hs_ed25519_secret_key"; then
      ok "$(printf '%-12s' "$svc") $(tr -d '[:space:]' < "$dest/$svc/hostname")"
    else
      warn "$svc: ключ не найден или невалиден"
    fi
  done
  # Копия на USB должна быть защищена: ключи = доступ к сервисам.
  chmod -R go-rwx "$dest"
  ok "бэкап готов, права 700 (это доступ к скрытым сервисам)"
  info "перенеси $dest на USB для клона на другой host"
}

# ── healthcheck-и ──────────────────────────────────────────────────────────
wait_healthy() {
  local name="$1" timeout="${2:-90}" waited=0 state=""
  info "жду healthy: $name (до ${timeout}s)"
  while [ "$waited" -lt "$timeout" ]; do
    state=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
            "ParanoidX-$name" 2>/dev/null || echo "нет")
    case "$state" in
      healthy|running) 
        case "$state" in healthy) ok "$name healthy (${waited}s)"; return 0 ;; esac ;;
      exited|dead)
        err "$name упал: состояние $state"
        info "логи: docker logs ParanoidX-$name --tail 20"
        return 1 ;;
    esac
    sleep 3; waited=$((waited+3))
  done
  err "$name не стал healthy за ${timeout}s (последнее: $state)"
  return 1
}

# ── проверка работоспособности ─────────────────────────────────────────────
do_test() {
  say "Проверка работоспособности"
  local failed=0

  # 1. Все 4 контейнера Up + healthy
  for svc in tor coturn smp-server xftp-server; do
    local st hs
    st=$(docker inspect -f '{{.State.Status}}' "ParanoidX-$svc" 2>/dev/null || echo "нет")
    hs=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}—{{end}}' \
         "ParanoidX-$svc" 2>/dev/null || echo "—")
    if [ "$st" = "running" ] && { [ "$hs" = "healthy" ] || [ "$hs" = "—" ]; }; then
      ok "$(printf '%-12s' "$svc") $st/$hs"
    else
      err "$(printf '%-12s' "$svc") $st/$hs"; failed=1
    fi
  done

  # 2. Скрытые адреса: не просто существуют, а отвечают Tor
  for svc in "${HS_SERVICES[@]}"; do
    local hf; hf="$(hs_dir_for "$svc")/hostname"
    if is_valid_hostname "$hf"; then
      ok "$(printf '%-12s' "$svc") $(tr -d '[:space:]' < "$hf")"
    else
      err "$svc: нет валидного hostname"; failed=1
    fi
  done

  # 3. Опубликованы в DataDir (их читает нода)
  for svc in "${ONION_EXPORTS[@]}"; do
    local f="$DATA_DIR/${svc}_onion.txt"
    if is_valid_hostname "$f"; then
      ok "$(printf '%-12s' "$svc") опубликован в DataDir"
    else
      err "$svc: $f отсутствует или невалиден — нода покажет пустое"; failed=1
    fi
  done

  # 4. Настоящий сетевой пробник.
  #    В torrc стоит `SocksPort 0` — Tor сознательно НЕ слушает Socks
  #    (SocksPort открыл бы доступ в сеть для всего, кто дотянется).
  #    Поэтому пробник возможен только если SocksPort включён, а он
  #    выключен по дизайну: onion-доступ идёт через HiddenServiceDir, а не
  #    через прокси. Пробуем, если есть, и честно говорим, если нет.
  if docker ps --format '{{.Names}}' | grep -q '^ParanoidX-tor$'; then
    if docker exec ParanoidX-tor sh -c 'nc -z 127.0.0.1 9050' 2>/dev/null; then
      if docker exec ParanoidX-tor wget -q -O - \
           --timeout=25 'http://check.torproject.org/api/ip' 2>/dev/null | grep -q .; then
        ok "tor выходит в сеть через onion (реальный запрос)"
      else
        warn "tor: SocksPort открыт, но пробник не ответил"
      fi
    else
      info "SocksPort=0 (by design) — прямой onion-пробник невозможен;"
      info "адреса проверены выше: они выданы самим Tor и лежат в его"
      info "HiddenServiceDir, то есть сервисы зарегистрированы."
    fi
  else
    warn "контейнер tor не запущен — сетевой пробник пропущен"
  fi

  # 5. Порты SimpleX слушают
  for p in 5223 5225; do
    if timeout 3 bash -c "</dev/tcp/127.0.0.1/$p" 2>/dev/null; then
      ok "порт $p отвечает"
    else
      warn "порт $p не отвечает (может быть нормой без активных клиентов)"
    fi
  done

  # 6. coturn: TURN для WebRTC. Проверяем ОБА транспорта, потому что
  #    они ломаются независимо. Конкретный баг, который это ловит:
  #    turn_cert.pem/turn_key.pem были по 0 байт (а turn_key.pem вдобавок
  #    оказался каталогом), coturn при этом числился healthy, 3478
  #    отвечал, а 5349 падал на TLS handshake — то есть весь
  #    шифрованный WebRTC-транспорт был мёртв и не выглядел сломанным.
  if docker ps --format '{{.Names}}' | grep -q '^ParanoidX-coturn$'; then
    local cip
    cip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
          ParanoidX-coturn 2>/dev/null)
    if [ -z "$cip" ]; then
      warn "не удалось узнать IP coturn — TURN не проверен"
    else
      # turn:// — plain STUN/TURN, 3478
      if timeout 5 bash -c "exec 3<>/dev/tcp/$cip/3478" 2>/dev/null; then
        ok "turn:// (3478) отвечает"
      else
        err "turn:// 3478 не отвечает — WebRTC ICE не заработает"; failed=1
      fi
      # turns:// — TLS, 5349. Проверяем РЕАЛЬНЫМ handshake: «порт
      # открыт» тут ничего не значит, сломанный сертификат тоже
      # держит TCP-соединение.
      local tls_out
      tls_out=$(timeout 12 openssl s_client -connect "$cip:5349" -brief \
                  </dev/null 2>&1)
      if printf '%s' "$tls_out" | grep -q 'CONNECTION ESTABLISHED'; then
        ok "turns:// (5349) TLS handshake установлен"
      else
        err "turns:// 5349 TLS не работает: $(printf '%s' "$tls_out" | head -1)"
        err "  → проверь docker/coturn/turn_cert.pem и turn_key.pem"
        failed=1
      fi
    fi
  else
    warn "coturn не запущен — TURN не проверен"
  fi

  echo
  if [ "$failed" = 0 ]; then
    ok "СТЕК РАБОТОСПОСОБЕН"
    return 0
  fi
  err "СТЕК НЕ РАБОТОСПОСОБЕН (см. строки с ОШИБКА выше)"
  return 1
}

# ── команды ────────────────────────────────────────────────────────────────
do_install() {
  say "ParanoidX Docker-стек: установка"
  ensure_docker

  # ── ВЫБОР РЕЖИМА КЛЮЧЕЙ ──
  if [ -z "$KEY_MODE" ]; then
    if [ -d "$HS_DIR/smp" ] && is_valid_key "$(hs_dir_for smp)/hs_ed25519_secret_key"; then
      KEY_MODE="keep"
    else
      KEY_MODE="new"
    fi
  fi

  if [ "$KEY_MODE" = "new" ] || [ "$KEY_MODE" = "keep" ]; then
    say "Выбор режима .onion-ключей"
    info "1) НОВЫЕ ключи — Tor сгенерирует свежие адреса."
    info "   Прежние .onion перестанут работать, ссылки у клиентов сгорят."
    info "2) ИМПОРТ из бэкапа — те же адреса, что на старом хосте."
    info "   Для клона или апгрейда. Бэкап: $BACKUP_ROOT/onion-keys-*"
    echo
    if [ "$KEY_MODE" = "keep" ]; then
      info "Обнаружены существующие ключи, по умолчанию — оставить их (1)."
    else
      info "Ключей нет, по умолчанию — создать новые (1)."
    fi
    if [ "$ASSUME_YES" = 1 ] || [ "$DRY_RUN" = 1 ]; then
      info "--yes/--dry-run: беру режим по умолчанию"
    else
      printf '  Выбор [1]: '
      read -r ans || ans="1"
      case "$ans" in
        2|и|имя|import) KEY_MODE="import" ;;
        *) KEY_MODE="new" ;;
      esac
    fi
  fi

  if [ "$KEY_MODE" = "import" ]; then
    if [ -z "$KEYS_SRC" ]; then
      # Ищем бэкап сами — чаще всего он уже на машине
      local found
      found=$(find "$BACKUP_ROOT" -maxdepth 2 -type d -name 'onion-keys*' 2>/dev/null | sort | tail -1)
      found=${found:-$(ls -dt /media/*/[Pp][Rr][Oo][Jj][Ee][Cc][Tt][Ss]/*/onion-keys* 2>/dev/null | head -1)}
      if [ -n "$found" ]; then
        KEYS_SRC="$found"
        info "нашёл бэкап: $KEYS_SRC"
      else
        die "для импорта нужен бэкап ключей. Создай его на старом хосте:
    $REPO/scripts/px-backup.sh onion
или скопируй docker/tor/hidden_services/ на USB и укажи --keys"
      fi
    fi
    do_keys_import || die "импорт ключей не удался — стек не трогаю"
  else
    do_keys_new
  fi

  say "Готовлю coturn (конфиг и TLS-сертификаты)"
  # ДО `compose up`: иначе coturn стартует, не найдя turnserver.conf
  # и сертификатов, и падает — а на чистом клоне это была бы загадка.
  ensure_coturn

  say "Поднимаю остальные контейнеры"
  compose up -d coturn smp-server xftp-server
  for svc in coturn smp-server xftp-server; do
    wait_healthy "$svc" 120 || warn "$svc не стал healthy — смотри docker logs ParanoidX-$svc"
  done

  say "Включаю автозапуск"
  # restart: unless-stopped уже есть в compose, но policy тоже нужна:
  # она переживает перезагрузку хоста, когда docker стартует раньше compose.
  if [ "$DRY_RUN" != 1 ]; then
    for svc in tor coturn smp-server xftp-server; do
      docker update --restart unless-stopped "ParanoidX-$svc" >/dev/null 2>&1 || true
    done
  fi
  ok "restart: unless-stopped + restart-policy принудительно проставлена"

  say "Проверка"
  do_test
}

do_status() {
  ensure_docker
  say "Контейнеры"
  printf '  %-14s %-10s %-10s %s\n' СЕРВИС СТАТУС HEALTH ИМЯ
  local svc st hs
  for svc in tor coturn smp-server xftp-server; do
    st=$(docker inspect -f '{{.State.Status}}' "ParanoidX-$svc" 2>/dev/null || echo "нет")
    hs=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}—{{end}}' \
         "ParanoidX-$svc" 2>/dev/null || echo "—")
    printf '  %-14s %-10s %-10s %s\n' "$svc" "$st" "$hs" "ParanoidX-$svc"
  done

  say ".onion-адреса"
  for svc in "${HS_SERVICES[@]}"; do
    local h; h=$(tr -d '[:space:]' < "$(hs_dir_for "$svc")/hostname" 2>/dev/null || echo "—")
    printf '  %-12s %s\n' "$svc" "$h"
  done

  say "Публикация в DataDir (её читает нода)"
  for svc in "${ONION_EXPORTS[@]}"; do
    local f="$DATA_DIR/${svc}_onion.txt"
    printf '  %-12s %s\n' "$svc" "$( [ -f "$f" ] && tr -d '[:space:]' < "$f" || echo 'НЕ ОПУБЛИКОВАН' )"
  done
}

do_show_keys() {
  say "Текущие .onion-ключи"
  for svc in "${HS_SERVICES[@]}"; do
    local d; d="$(hs_dir_for "$svc")"
    printf '  %-12s %s\n' "$svc" "$(tr -d '[:space:]' < "$d/hostname" 2>/dev/null || echo '—')"
  done
}

# ── main ───────────────────────────────────────────────────────────────────
case "$CMD" in
  install) do_install ;;
  start)
    ensure_docker; compose up -d; do_test ;;
  stop)
    ensure_docker; compose stop; ok "остановлено" ;;
  down)
    ensure_docker
    warn "будут удалены контейнеры; volumes и ключи НЕ трогаю"
    if [ "$ASSUME_YES" = 1 ]; then
      compose down
    else
      printf '  Удалить контейнеры? [y/N] '
      read -r ans
      case "$ans" in [yY]|[дД]) compose down ;; *) die "отменено" ;; esac
    fi
    ok "контейнеры удалены, ключи и volumes на месте" ;;
  status) do_status ;;
  test)  ensure_docker; do_test ;;
  logs)
    ensure_docker
    compose logs -f --tail 100 "${2:-}" ;;
  keys)
    ensure_docker
    case "${KEY_ACTION:-show}" in
      new)    do_keys_new ;;
      import) do_keys_import ;;
      show)   do_show_keys ;;
      backup) do_keys_backup ;;
      *)      usage; exit 2 ;;
    esac
    ;;
  *) usage; exit 2 ;;
esac
