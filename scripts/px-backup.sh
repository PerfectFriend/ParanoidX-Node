#!/usr/bin/env bash
# px-backup.sh — резервное копирование и восстановление ноды ParanoidX.
#
#   ./scripts/px-backup.sh save [DIR]   # всё: конфиги, ключи, состояние
#   ./scripts/px-backup.sh onion [DIR]  # только .onion-ключи (для клона)
#   ./scripts/px-backup.sh list         # что есть в BACKUP_ROOT
#   ./scripts/px-backup.sh restore SRC  # развернуть бэкап
#   ./scripts/px-backup.sh verify SRC   # проверить целостность, ничего не трогая
#
# ── Почему onion вынесен отдельной командой ────────────────────────────────
# Ключи скрытых сервисов — это идентичность ноды. Перенести её на другой
# host нужно при апгрейде, и для этого НЕ нужен ни Go, ни Flutter, ни
# Docker. Импорт делает deploy/px-docker.sh keys import. Секрет не должен
# попадать в общий бэкап: он идёт в отдельный каталог с правами 700.
#
# Все бэкапы пишутся в BACKUP_ROOT (по умолчанию ~/A1-backups), который
# по замыслу живёт ВНЕ git и (на этой машине) копируется на USB.
set -uo pipefail

REPO="${REPO:-$(cd "$(dirname "$0")/.." && pwd)}"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
# Конфиг ноды лежит РЯМО в DATA_DIR, а не в ~/.config — так его запускает
# systemd: ParanoidX-dashboard.service -> -config
# ~/.local/share/simplex-node/simplex-node.json. Раньше здесь стоял
# ~/.config/simplex-node/, и save молча писал «конфиг не найден», а
# verify — «нет конфига ноды»: бэкап без конфига проходил как годный,
# и на новом host нода поднималась бы с настройками по умолчанию.
CONFIG_FILE="${CONFIG_FILE:-$DATA_DIR/simplex-node.json}"
HS_DIR="${HS_DIR:-$REPO/docker/tor/hidden_services}"
BACKUP_ROOT="${BACKUP_ROOT:-$HOME/A1-backups}"
DOCKER_DIR="${DOCKER_DIR:-$REPO/docker}"
HS_SERVICES=(smp xftp dashboard ice auditor)

DRY_RUN="${DRY_RUN:-0}"
ASSUME_YES="${ASSUME_YES:-0}"

say()  { printf '\n\033[1;36m▸ %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mOK\033[0m   %s\n' "$*"; }
info() { printf '       %s\n' "$*"; }
warn() { printf '  \033[33mWARN\033[0m %s\n' "$*"; }
err()  { printf '  \033[31mFAIL\033[0m %s\n' "$*"; }
die()  { err "$*"; exit 1; }

now() { date +%Y%m%d-%H%M%S; }
run() { if [ "$DRY_RUN" = 1 ]; then info "DRY: $*"; else "$@"; fi; }

# ── onion: ключи скрытых сервисов ───────────────────────────────────────────
# Ключ Tor ed25519v1: 96 байт, первые 32 — заголовок
# "== ed25519v1-secret: type0 ==", остальные 64 — собственно ключ.
is_valid_key() {
  local f="$1"
  [ -f "$f" ] || return 1
  [ "$(stat -c%s "$f" 2>/dev/null || echo 0)" = 96 ] || return 1
  head -c 32 "$f" 2>/dev/null | grep -q 'ed25519v1-secret'
}
is_valid_hostname() {
  local f="$1"
  [ -f "$f" ] || return 1
  grep -qE '^[a-z2-7]{56}\.onion$' "$f" 2>/dev/null
}

# Копирует hidden_services, попутно проверяя каждый ключ. Молча скопировать
# битый ключ нельзя: на новом host он даст ноду, которая не резолвится, и
# об этом узнаёшь только через неделю, когда клиенты не подключатся.
copy_onion() {
  local src="$1" dst="$2" svc n=0 bad=0
  for svc in "${HS_SERVICES[@]}"; do
    local sd="$src/$svc"
    [ -d "$sd" ] || { warn "$svc: каталога нет в источнике"; bad=1; continue; }
    if is_valid_key "$sd/hs_ed25519_secret_key"; then
      mkdir -p "$dst/$svc"
      cp -a "$sd/hs_ed25519_secret_key" "$dst/$svc/"
      chmod 600 "$dst/$svc/hs_ed25519_secret_key"
      if is_valid_hostname "$sd/hostname"; then
        cp -a "$sd/hostname" "$dst/$svc/"
      else
        warn "$svc: нет валидного hostname — скопирую ключ, адрес Tor пересоздаст"
      fi
      n=$((n + 1))
    else
      err "$svc: ключ отсутствует или невалиден (ожидается 96 байт)"
      bad=1
    fi
  done
  [ "$bad" = 0 ] || return 1
  [ "$n" -eq "${#HS_SERVICES[@]}" ] || { err "скопировано $n из ${#HS_SERVICES[@]}"; return 1; }
  ok "onion-ключи: $n/${#HS_SERVICES[@]} сервисов"
  return 0
}

cmd_onion() {
  local dest="${1:-$BACKUP_ROOT/onion-keys-$(now)}"
  say "Бэкап .onion-ключей"
  info "источник: $HS_DIR"
  info "назначение: $dest"
  # Проверяем ДО копирования: если хоть один ключ битый, бэкап неполон, и
  # импортировать его на другой host опасно.
  local pre=0
  for svc in "${HS_SERVICES[@]}"; do
    is_valid_key "$HS_DIR/$svc/hs_ed25519_secret_key" || { err "$svc: ключ невалиден"; pre=1; }
  done
  [ "$pre" = 0 ] || die "бэкап прерван: сначала почини ключи (или восстанови из прошлого бэкапа)"
  if [ "$DRY_RUN" = 1 ]; then info "DRY: скопирую в $dest"; return 0; fi
  mkdir -p "$dest" || die "не создать $dest"
  chmod 700 "$dest"
  # Раскладываем в hidden_services/, а не в корень. Тот же формат, что у
  # полного бэкапа (save) и который ждёт verify/restore. Раньше здесь был
  # корень, и скрипт не мог восстановить СВОЙ бэкап: verify искал
  # hidden_services/ и рапортовал «onion-ключей в бэкапе нет».
  copy_onion "$HS_DIR" "$dest/hidden_services" || { rm -rf "$dest"; die "бэкап неполон, каталог удалён"; }
  # Публичные ключи — для сверки, что это те же адреса.
  for svc in "${HS_SERVICES[@]}"; do
    [ -f "$HS_DIR/$svc/hs_ed25519_public_key" ] && \
      cp -a "$HS_DIR/$svc/hs_ed25519_public_key" "$dest/hidden_services/$svc/"
  done
  cat > "$dest/MANIFEST.txt" <<EOF
ParanoidX onion key backup
Создан:      $(date)
Источник:    $HS_DIR
Сервисов:    ${#HS_SERVICES[@]}
Адреса:
$(for svc in "${HS_SERVICES[@]}"; do
    [ -f "$dest/hidden_services/$svc/hostname" ] && printf '  %-12s %s\n' "$svc" "$(tr -d '[:space:]' < "$dest/hidden_services/$svc/hostname")"
  done)
Импорт на новом host:
  ./deploy/px-docker.sh keys import --keys $dest
  ./scripts/px-backup.sh restore $dest
EOF
  ok "бэкап готов: $dest"
  info "восстановление: px-docker.sh keys import --keys $dest"
}

# ── save: полный бэкап ─────────────────────────────────────────────────────
cmd_save() {
  local dest="${1:-$BACKUP_ROOT/node-backup-$(now)}"
  say "Полный бэкап ноды"
  info "назначение: $dest"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: конфиг, ключи, состояние, coturn, сертификаты"
    return 0
  fi
  mkdir -p "$dest"; chmod 700 "$dest"

  # 1. .onion-ключи — критичны, и их проверяем.
  copy_onion "$HS_DIR" "$dest/hidden_services" || die "onion-ключи битые — бэкап не делаю"

  # 2. Конфиг ноды. Копируем дважды: рабочий и шаблон, чтобы восстановить
  #    не только текущие значения, но и саму возможность настройки.
  #
  #    Рабочий конфиг кладём под ФИКСИРОВАННЫМ именем simplex-node.json,
  #    а не под его собственным basename: при CONFIG_FILE=elsewhere.json
  #    он попадал в бэкап как elsewhere.json, и verify/restore его не
  #    находили — бэкап выглядел исправным, а восстановление молчало
  #    ничего не делать. Имя в бэкапе не должно зависеть от того,
  #    как файл назван на диске.
  if [ -f "$CONFIG_FILE" ]; then
    mkdir -p "$dest/config"
    cp -a "$CONFIG_FILE" "$dest/config/simplex-node.json"
    [ -f "$REPO/deploy/simplex-node.json.template" ] && \
      cp -a "$REPO/deploy/simplex-node.json.template" "$dest/config/"
    ok "конфиг ноды → config/simplex-node.json"
  else
    warn "конфиг ноды не найден ($CONFIG_FILE) — нода восстановится с настройками по умолчанию"
  fi

  # 3. Состояние ноды: кошельки, чат, транспорт. Без этого нода поднимется,
  #    но будет пустой — а это не «восстановление».
  if [ -d "$DATA_DIR" ]; then
    mkdir -p "$dest/data"
    for item in "$DATA_DIR"/*; do
      [ -e "$item" ] || continue
      bn=$(basename "$item")
      case "$bn" in
        # Тяжёлое и воспроизводимое — не копируем.
        logs|cache|*.log) continue ;;
      esac
      cp -a "$item" "$dest/data/" 2>/dev/null || warn "не скопировал $bn"
    done
    ok "состояние ноды ($(du -sh "$dest/data" 2>/dev/null | cut -f1))"
  else
    warn "каталог состояния не найден ($DATA_DIR)"
  fi

  # 4. Docker-конфигурация: coturn-конфиг и TLS. Ключи СЕРТИФИКАТОВ — не
  #    onion, но без них coturn не поднимет turns://.
  if [ -d "$DOCKER_DIR/coturn" ]; then
    mkdir -p "$dest/coturn"
    for f in turnserver.conf turn_cert.pem turn_key.pem; do
      [ -f "$DOCKER_DIR/coturn/$f" ] && { cp -a "$DOCKER_DIR/coturn/$f" "$dest/coturn/"; chmod 600 "$dest/coturn/$f"; }
    done
    [ -f "$DOCKER_DIR/coturn/turnserver.conf" ] && ok "coturn (конфиг + TLS)"
  fi

  # 5. Приватные TLS-ключи SMP/XFTP — без них серверы не примут клиентов,
  #    которым отпечаток уже зашит в приложение.
  for d in smp_configs xftp_configs; do
    if [ -d "$DOCKER_DIR/$d" ]; then
      mkdir -p "$dest/$d"
      ( cd "$DOCKER_DIR/$d" && find . -name '*.key' -o -name 'fingerprint' -o -name 'ca.srl' ) | while read -r f; do
        [ -f "$f" ] || continue
        mkdir -p "$dest/$d/$(dirname "$f")"
        cp -a "$f" "$dest/$d/$f"
      done
      found=$(find "$dest/$d" -type f 2>/dev/null | wc -l)
      [ "$found" -gt 0 ] && ok "$d: $found ключевых файлов"
    fi
  done

  # 6. Состояние контейнеров (volume-имя, restart policy) — нужно, чтобы
  #    восстановленный хост поднялся так же.
  if command -v docker >/dev/null 2>&1; then
    docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' > "$dest/docker-ps.txt" 2>/dev/null || true
    docker inspect ParanoidX-tor > "$dest/docker-tor.json" 2>/dev/null || true
  fi

  cat > "$dest/MANIFEST.txt" <<EOF
ParanoidX node backup
Создан:     $(date)
Хост:       $(hostname)
Repo:       $REPO
Размер:     $(du -sh "$dest" 2>/dev/null | cut -f1)

Восстановление:
  ./scripts/px-backup.sh restore $dest

Onion-адреса (из hidden_services/*/hostname):
$(for svc in "${HS_SERVICES[@]}"; do
    [ -f "$dest/hidden_services/$svc/hostname" ] && printf '  %-12s %s\n' "$svc" "$(tr -d '[:space:]' < "$dest/hidden_services/$svc/hostname")"
  done)
EOF
  ok "бэкап готов: $dest"
  info "размер: $(du -sh "$dest" 2>/dev/null | cut -f1)"
  warn "этот каталог содержит ПРИВАТНЫЕ КЛЮЧИ. Не класть в git."
}

cmd_list() {
  say "Бэкапы в $BACKUP_ROOT"
  if [ ! -d "$BACKUP_ROOT" ]; then
    warn "каталога нет — бэкапов пока не было"
    return 0
  fi
  local d found=0
  while IFS= read -r d; do
    found=$((found + 1))
    printf '  %-52s %s\n' "$(basename "$d")" "$(du -sh "$d" 2>/dev/null | cut -f1)"
  done < <(find "$BACKUP_ROOT" -maxdepth 1 -mindepth 1 -type d | sort -r)
  [ "$found" = 0 ] && info "(пусто)"
  ok "всего каталогов: $found"
}

# ── verify: проверка бэкапа, ничего не меняя ────────────────────────────────
cmd_verify() {
  local src="${1:-}"
  [ -n "$src" ] || die "укажи каталог бэкапа"
  [ -d "$src" ] || die "нет каталога $src"
  say "Проверка бэкапа $src"
  local bad=0
  local hsd="$src/hidden_services"
  if [ -d "$hsd" ]; then
    for svc in "${HS_SERVICES[@]}"; do
      if is_valid_key "$hsd/$svc/hs_ed25519_secret_key"; then
        local h="(нет hostname)"
        [ -f "$hsd/$svc/hostname" ] && h=$(tr -d '[:space:]' < "$hsd/$svc/hostname")
        ok "$svc: ключ валиден, адрес $h"
      else
        err "$svc: ключ отсутствует или невалиден"; bad=1
      fi
    done
  else
    err "нет hidden_services/ — onion-ключей в бэкапе нет"; bad=1
  fi
  if [ -f "$src/config/simplex-node.json" ]; then ok "конфиг ноды"; else warn "нет конфига ноды — нода поднимется с настройками по умолчанию"; fi
  [ -f "$src/MANIFEST.txt" ] && ok "манифест" || warn "нет MANIFEST.txt"
  if [ "$bad" = 0 ]; then ok "бэкап пригоден к восстановлению"; return 0; fi
  err "бэкап НЕ пригоден"; return 1
}

# ── restore ────────────────────────────────────────────────────────────────
cmd_restore() {
  local src="${1:-}"
  [ -n "$src" ] || die "укажи каталог бэкапа"
  [ -d "$src" ] || die "нет каталога $src"

  say "Восстановление из $src"
  # Отказоустойчивость: перед раскладкой проверяем, что восстанавливать
  # есть, иначе можно затереть живое состояние и остаться ни с чем.
  cmd_verify "$src" || die "бэкап неполон — не восстанавливаю, живое состояние не тронуто"

  if [ "$ASSUME_YES" != 1 ]; then
    warn "текущие onion-ключи и конфиги будут ЗАМЕНЕНЫ содержимым бэкапа."
    printf '  Продолжить? [y/N] '
    read -r a
    case "$a" in y|Y|yes|YES) ;; *) die "отменено" ;; esac
  fi

  # Страховка: сначала снимаем текущее состояние, чтобы откат был возможен
  # даже если бэкап окажется битым в том, что проверка не охватила.
  # Раскладываем в hidden_services/ — тем же путём, что и restore читает,
  # чтобы откат делался копированием «как есть».
  local stash="$BACKUP_ROOT/pre-restore-$(now)"
  say "Страховочная копия текущего состояния"
  mkdir -p "$stash"; chmod 700 "$stash"
  if [ -d "$HS_DIR" ]; then cp -a "$HS_DIR" "$stash/hidden_services"; ok "текущие onion-ключи → $stash"; fi
  [ -f "$CONFIG_FILE" ] && { mkdir -p "$stash/config"; cp -a "$CONFIG_FILE" "$stash/config/"; ok "текущий конфиг сохранён"; }
  info "откат: cp -a $stash/hidden_services/. $HS_DIR/"

  say "Раскладываю onion-ключи"
  mkdir -p "$HS_DIR"
  for svc in "${HS_SERVICES[@]}"; do
    [ -f "$src/hidden_services/$svc/hs_ed25519_secret_key" ] || continue
    mkdir -p "$HS_DIR/$svc"
    cp -a "$src/hidden_services/$svc/hs_ed25519_secret_key" "$HS_DIR/$svc/"
    chmod 600 "$HS_DIR/$svc/hs_ed25519_secret_key"
    ok "$svc"
  done

  say "Публикую адреса (нода читает *_onion.txt из DataDir)"
  if [ -d "$DATA_DIR" ]; then
    for svc in "${HS_SERVICES[@]}"; do
      f="$src/hidden_services/$svc/hostname"
      [ -f "$f" ] && cp -a "$f" "$DATA_DIR/${svc}_onion.txt"
    done
    ok "адреса опубликованы в $DATA_DIR"
  else
    warn "DataDir $DATA_DIR не существует — адреса опубликуются при старте"
  fi

  if [ -f "$src/config/simplex-node.json" ]; then
    say "Восстанавливаю конфиг ноды"
    if [ "$DRY_RUN" = 1 ]; then info "DRY: скопирую в $CONFIG_FILE"
    elif mkdir -p "$(dirname "$CONFIG_FILE")" \
      && cp -a "$src/config/simplex-node.json" "$CONFIG_FILE"; then
      # Проверяем ФАКТ, а не код возврата: mkdir может создать каталог,
      # а cp молча упасть (нет места, прав, битый источник) — и раньше
      # скрипт в этом случае рапортовал «конфиг восстановлен».
      if [ -f "$CONFIG_FILE" ]; then ok "конфиг восстановлен"
      else err "конфиг НЕ восстановлен — не могу записать $CONFIG_FILE"; return 1; fi
    else
      err "не удалось создать $(dirname "$CONFIG_FILE") или скопировать конфиг"; return 1
    fi
  fi

  if [ -d "$src/coturn" ] && [ -d "$DOCKER_DIR/coturn" ]; then
    say "Восстанавливаю coturn"
    for f in turnserver.conf turn_cert.pem turn_key.pem; do
      [ -f "$src/coturn/$f" ] || continue
      if [ "$DRY_RUN" = 1 ]; then info "DRY: $f"; else
        cp -a "$src/coturn/$f" "$DOCKER_DIR/coturn/"; chmod 600 "$DOCKER_DIR/coturn/$f"
      fi
    done
    ok "coturn: конфиг и TLS восстановлены"
  fi

  for d in smp_configs xftp_configs; do
    [ -d "$src/$d" ] && [ -d "$DOCKER_DIR/$d" ] || continue
    say "Восстанавливаю $d"
    ( cd "$src/$d" && find . -type f ) | while read -r f; do
      mkdir -p "$DOCKER_DIR/$d/$(dirname "$f")"
      [ "$DRY_RUN" = 1 ] && info "DRY: $d/$f" || cp -a "$src/$d/$f" "$DOCKER_DIR/$d/$f"
    done
    ok "$d восстановлен"
  done

  if [ -d "$src/data" ]; then
    say "Состояние ноды (кошельки, чат, транспорт)"
    if [ "$DRY_RUN" = 1 ]; then info "DRY: данные в $DATA_DIR"
    else mkdir -p "$DATA_DIR"; cp -a "$src/data/." "$DATA_DIR/"; ok "состояние восстановлено"; fi
  fi

  echo
  warn "Контейнеры и сервисы НЕ перезапущены (это неявное действие)."
  info "Перезапусти вручную:"
  info "  cd $DOCKER_DIR && docker compose up -d"
  info "  systemctl --user restart simplex-node"
  info "Затем проверь: ./deploy/px-docker.sh test"
}

usage() {
  cat <<EOF
px-backup.sh — бэкап и восстановление ноды ParanoidX

  save [DIR]     полный бэкап (onion, конфиг, состояние, coturn, TLS)
  onion [DIR]    только .onion-ключи — для переноса на другой host
  list           что есть в \$BACKUP_ROOT
  verify SRC     проверить бэкап, ничего не меняя
  restore SRC    развернуть бэкап (с вопросом и страховкой)

Переменные окружения:
  BACKUP_ROOT   куда писать (по умолчанию ~/A1-backups)
  DATA_DIR      состояние ноды
  HS_DIR        каталог onion-ключей
  DRY_RUN=1     показать, что будет сделано, и не делать
  ASSUME_YES=1  не спрашивать подтверждение (restore)
EOF
}

cmd="${1:-}"; shift 2>/dev/null || true
case "$cmd" in
  save)           cmd_save "$@" ;;
  onion|keys)     cmd_onion "$@" ;;
  list)           cmd_list "$@" ;;
  verify)         cmd_verify "$@" ;;
  restore)        cmd_restore "$@" ;;
  -h|--help|help|"") usage ;;
  *)              printf 'неизвестная команда: %s\n\n' "$cmd" >&2; usage >&2; exit 2 ;;
esac
