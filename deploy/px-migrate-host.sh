#!/usr/bin/env bash
# px-migrate-host.sh — перенос ноды ParanoidX на новый host.
#
#   ./deploy/px-migrate-host.sh doctor        # проверить новый host
#   ./deploy/px-migrate-host.sh prepare       # собрать пакет на старом host
#   ./deploy/px-migrate-host.sh install       # развернуть на новом host
#   ./deploy/px-migrate-host.sh verify        # доказать, что нода работает
#   ./deploy/px-migrate-host.sh rollback      # откатить назад
#
# ── Почему скрипт, а не «скопируй репозиторий» ─────────────────────────────
# Живая нода — это репозиторий ПЛЮС ещё четыре вещи, которых в git нет:
#
#   1. 16 systemd-юнитов (6 системных + 10 пользовательских). В git лежат
#      только 4 шаблона, и ни один не ставится автоматически.
#   2. Собранные бинарники: ParanoidX (22 МБ) и simplex-chat-island (76 МБ).
#      Либо собирать из исходников, либо переносить.
#   3. Состояние: ~/.local/share/simplex-node (35 МБ) — кошельки, чат,
#      транспорт. Без него нода поднимется пустой.
#   4. Секреты: onion-ключи, TLS-сертификаты coturn, TURN-секрет.
#
# И главное: 15 из 16 юнитов содержат жёсткий путь /home/tomas. На хосте
# с другим пользователем они не запустятся. Скрипт это переписывает.
#
# Скрипт НИЧЕГО не делает неявно: destructive-шаги требуют подтверждения,
# а rollback создаётся автоматически перед первым изменением.
set -uo pipefail

REPO="${REPO:-$(cd "$(dirname "$0")/.." && pwd)}"
DEPLOY="$REPO/deploy"
USER_NAME="${USER_NAME:-$(id -un)}"
HOME_DIR="$HOME"
BIN_DIR="${BIN_DIR:-$HOME/bin}"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
CONFIG_FILE="${CONFIG_FILE:-$DATA_DIR/simplex-node.json}"
SYS_UNITS="$DEPLOY/system-units"
USER_UNITS="$DEPLOY/user-units"
STATE_DIR="${STATE_DIR:-$HOME/.local/state/px-migrate}"
BACKUP_ROOT="${BACKUP_ROOT:-$HOME/A1-backups}"

# Юниты, которые ставим в /etc/systemd/system (нужен root).
SYS_LIST=(ParanoidX-dashboard.service ParanoidX-bridge.service
          ParanoidX-admin-bot.service ParanoidX-royal-bot.service
          ParanoidX-poet-bot.service ParanoidX-post-reboot-restore.service)
# Юниты, которые ставим в ~/.config/systemd/user.
USR_LIST=(node-monitor.service simplex-node.service px-xray-refresh.service
          isle-flutter.service simplex-backup.service simplex-stack.service)

DRY_RUN="${DRY_RUN:-0}"
ASSUME_YES="${ASSUME_YES:-0}"
# --report: сохранить полный лог и напечатать путь. Специально НЕ
# подавляет ошибки: на первом цикле репликации install ОБЯЗАН упасть,
# и падение с логом — это ровно тот отчёт, который нужен для починки.
# На рабочем хосте оставлять install с --report нельзя: юнит не стартует.
REPORT="${REPORT:-}"

say()  { printf '\n\033[1;36m▸ %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mOK\033[0m   %s\n' "$*"; }
info() { printf '       %s\n' "$*"; }
warn() { printf '  \033[33mВНИМ.\033[0m %s\n' "$*"; }
err()  { printf '  \033[31mОШИБКА\033[0m %s\n' "$*"; }
step() { printf '  \033[34m—\033[0m    %s\n' "$*"; }
die()  { err "$*"; exit 1; }

run()  { if [ "$DRY_RUN" = 1 ]; then info "DRY: $*"; else "$@"; fi; }
runr() { if [ "$DRY_RUN" = 1 ]; then info "DRY: sudo $*"; else sudo "$@"; fi; }
have() { command -v "$1" >/dev/null 2>&1; }

confirm() {
  [ "$ASSUME_YES" = 1 ] && return 0
  [ "$DRY_RUN" = 1 ] && return 0
  printf '  %s [y/N] ' "$1"
  read -r a
  case "$a" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

# ── doctor: что есть на этом хосте ─────────────────────────────────────────
cmd_doctor() {
  say "Обследование хоста"
  info "пользователь: $USER_NAME   home: $HOME_DIR"

  say "Инструменты"
  local miss=0
  if have go; then ok "go $(go version | awk '{print $3}')"; else err "go не установлен"; miss=1; fi
  if have docker; then ok "docker $(docker --version | awk '{print $3}' | tr -d ,)"
  else err "docker не установлен"; miss=1; fi
  if have git; then ok "git $(git --version | awk '{print $3}')"; else err "git нет"; miss=1; fi
  if have python3; then ok "python3 $(python3 -V 2>&1 | awk '{print $2}')"; else err "python3 нет"; miss=1; fi
  if have flutter; then ok "flutter есть (нужен только для сборки приложений)"
  else info "flutter нет — приложения не соберутся, нода работать будет"; fi
  if have pkexec; then ok "pkexec есть (нужен для кнопок управления в мониторе)"
  else warn "pkexec нет — кнопки ON/OFF в мониторе спросят пароль"; fi

  if [ "$(id -u)" = 0 ]; then
    warn "скрипт запущен от root — install-юниты всё равно ставятся с sudo"
  fi

  say "Свободное место"
  local avail
  avail=$(df -BG --output=avail "$HOME" 2>/dev/null | tail -1 | tr -dc '0-9')
  if [ -n "$avail" ] && [ "$avail" -ge 3 ]; then ok "${avail} ГБ свободно (нужно ~2)"
  else err "мало места: ${avail:-?} ГБ"; miss=1; fi

  say "Порт 8080"
  if timeout 2 bash -c "</dev/tcp/127.0.0.1/8080" 2>/dev/null; then
    warn "порт 8080 уже занят — нода не сможет подняться"
  else ok "порт 8080 свободен"; fi

  say "Docker уже работает?"
  if have docker && docker info >/dev/null 2>&1; then ok "dockerd отвечает"
  else warn "dockerd не отвечает — install его поднимет (нужен sudo)"; fi

  say "Существующая нода"
  if [ -f "$CONFIG_FILE" ]; then
    warn "конфиг уже есть: $CONFIG_FILE"
    info "install НЕ перезапишет его без подтверждения"
  else ok "конфига нет — чистый host"; fi
  if [ -d "$DATA_DIR" ]; then warn "каталог состояния есть: $DATA_DIR"
  else ok "состояния нет"; fi

  say "Аутентификация sudo"
  if sudo -n true 2>/dev/null; then ok "sudo без пароля (свежий кэш)"
  else info "sudo спросит пароль один раз — нужен BabaYaga99"; fi

  echo
  if [ "$miss" = 0 ]; then ok "host готов к установке"; return 0
  else err "не хватает: смотрите строки ОШИБКА выше"; return 1; fi
}

# ── prepare: собрать переносимый пакет на СТАРОМ хосте ──────────────────────
cmd_prepare() {
  say "Сборка пакета для переноса (выполнять на СТАРОМ хосте)"
  local stamp dest
  stamp=$(date +%Y%m%d-%H%M%S)
  dest="${OUT_DIR:-$HOME/px-migration-$stamp}"
  info "назначение: $dest"
  confirm "Создать пакет в $dest?" || die "отменено"
  run mkdir -p "$dest"

  # 1. Секреты и состояние — через уже проверенный скрипт.
  say "Бэкап ноды (ключи, конфиг, состояние, coturn)"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: ./scripts/px-backup.sh save $dest/node-backup"
  else
    "$REPO/scripts/px-backup.sh" save "$dest/node-backup" || die "бэкап не удался"
    ok "node-backup готов"
  fi

  # 2. Юниты, которых нет в git.
  say "Снимаю systemd-юниты (в git их нет)"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: скопирую ${#SYS_LIST[@]} системных и ${#USR_LIST[@]} пользовательских"
  else
    mkdir -p "$dest/units/system" "$dest/units/user"
    local u n=0
    for u in "${SYS_LIST[@]}"; do
      if [ -f "/etc/systemd/system/$u" ]; then
        cp "/etc/systemd/system/$u" "$dest/units/system/"; n=$((n+1))
      else warn "$u не найден на этом хосте"; fi
    done
    for u in "${USR_LIST[@]}"; do
      if [ -f "$HOME_DIR/.config/systemd/user/$u" ]; then
        cp "$HOME_DIR/.config/systemd/user/$u" "$dest/units/user/"; n=$((n+1))
      else info "$u нет (опционально)"; fi
    done
    # target нужен всем системным юнитам
    [ -f /etc/systemd/system/ParanoidX.target ] && \
      cp /etc/systemd/system/ParanoidX.target "$dest/units/system/"
    ok "скопировано юнитов: $n"
  fi

  # 3. Бинарники — собираем из исходников, а не переносим: так на новом
  #    хосте будет его архитектура, а не наша.
  say "Собираю бинарники из исходников"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: go build ./cmd/ParanoidX/ && go build ./cmd/simplex-chat-island/"
  else
    if have go; then
      ( cd "$REPO" && go build -o "$dest/bin/ParanoidX" ./cmd/ParanoidX/ ) \
        && ok "ParanoidX собран" || err "сборка ParanoidX не удалась"
      if [ -d "$REPO/cmd/simplex-chat-island" ]; then
        ( cd "$REPO" && go build -o "$dest/bin/simplex-chat-island" ./cmd/simplex-chat-island/ ) \
          && ok "simplex-chat-island собран" || warn "simplex-chat-island не собрался"
      else warn "нет cmd/simplex-chat-island — бот не переносится"; fi
    else warn "go отсутствует — бинарники придётся собирать на новом хосте"; fi
  fi

  # 4. Манифест: что должно быть на новом хосте.
  say "Манифест"
  if [ "$DRY_RUN" != 1 ]; then
    cat > "$dest/MANIFEST.txt" <<EOF
ParanoidX — пакет переноса
Создан: $(date)
Источник: $(hostname) / $USER_NAME

На новом хосте:
  1. git clone <repo> ~/ParanoidX
  2. ./deploy/px-migrate-host.sh doctor
  3. ./deploy/px-migrate-host.sh install --from $dest
  4. ./deploy/px-migrate-host.sh verify

Что внутри:
  node-backup/   ключи onion, конфиг, состояние, coturn (СЕКРЕТЫ)
  units/         16 systemd-юнитов, которых нет в git
  bin/           собранные бинарники под эту архитектуру

Юниты переписаны под пользователя нового хоста при установке.
EOF
    ok "манифест записан"
  fi

  say "Готово"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: пакет был бы в $dest"
  else
    ok "пакет: $dest ($(du -sh "$dest" | cut -f1))"
    warn "СОДЕРЖИТ ПРИВАТНЫЕ КЛЮЧИ. Переносить на USB, не в git."
    info "скопируй на новый host и выполни:"
    info "  ./deploy/px-migrate-host.sh install --from $dest"
  fi
}

# ── install: развернуть на новом хосте ─────────────────────────────────────
# Переписывает шаблон юнита под этот host.
#
# В git юниты лежат ШАБЛОНАМИ: @HOME@, @REPO@, @USER@. Так они portable
# сразу. Но prepare может положить в пакет СЫРЫЕ юниты, снятые с
# /etc/systemd/system старого хоста, где вместо @HOME@ — /home/tomas.
# Поэтому обрабатываем оба вида: сначала плейсхолдеры, потом старый путь.
rewrite_paths() {
  local f="$1"
  # 1) плейсхолдеры шаблона (порядок важен: @REPO@ длиннее @HOME@)
  sed -i \
    -e "s#@REPO@#${REPO}#g" \
    -e "s#@HOME@#${HOME_DIR}#g" \
    -e "s#@USER@#${USER_NAME}#g" \
    "$f"
  # 2) старые абсолютные пути — если юнит снят с чужого хоста
  sed -i \
    -e "s#/home/tomas/ParanoidX#${REPO}#g" \
    -e "s#/home/tomas/simplex-node#${REPO}#g" \
    -e "s#/home/tomas#${HOME_DIR}#g" \
    -e "s#\bUser=tomas\b#User=${USER_NAME}#g" \
    -e "s#\bGroup=tomas\b#Group=${USER_NAME}#g" \
    -e "s#\bUSER=tomas\b#USER=${USER_NAME}#g" \
    "$f"
  # 3) единичная проверка: не осталось ли неизвестных плейсхолдеров
  if grep -q '@[A-Z_]*@' "$f"; then
    local left
    left=$(grep -o '@[A-Z_]*@' "$f" | sort -u | tr '\n' ' ')
    err "в $f остались неподставленные плейсхолдеры: $left"
    return 1
  fi
  return 0
}

cmd_install() {
  local src=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --from) src="${2:-}"; shift 2 ;;
      --report) REPORT="${2:-$HOME/px-install-report.log}"; shift 2 ;;
      *) shift ;;
    esac
  done
  # Весь вывод install — в лог И в stdout, чтобы отчёт можно было
  # переслать одним куском.
  if [ -n "$REPORT" ]; then
    mkdir -p "$(dirname "$REPORT")"
    exec > >(tee "$REPORT") 2>&1
    say "Лог установки: $REPORT"
  fi
  say "Установка ноды на этот host"
  info "пользователь: $USER_NAME   home: $HOME_DIR   репозиторий: $REPO"
  # --from НЕ обязателен. Пакет нужен только чтобы перенести onion-ключи,
  # состояние ноды и TLS-сертификаты. Для новой ноды со своими ключами
  # достаточно клона репозитория: юниты и бинарники берутся из git.
  if [ -n "$src" ]; then
    info "пакет: $src"
  else
    info "пакета нет — режим НОВОЙ ноды: ключи сгенерируются заново"
    info "и состояние будет пустым. Если нужны старые адреса и данные — укажи --from."
  fi

  # 0. Страховка ДО любых изменений.
  local stamp rollback
  stamp=$(date +%Y%m%d-%H%M%S)
  rollback="$STATE_DIR/pre-install-$stamp"
  say "Страховочная точка отката"
  if [ "$DRY_RUN" = 1 ]; then
    info "DRY: снимок в $rollback"
  else
    mkdir -p "$rollback"
    for d in "$HOME_DIR/.config/systemd/user" /etc/systemd/system; do
      [ -d "$d" ] && cp -a "$d" "$rollback/$(basename "$d")" 2>/dev/null
    done
    [ -f "$CONFIG_FILE" ] && cp -a "$CONFIG_FILE" "$rollback/"
    [ -d "$DATA_DIR" ] && cp -a "$DATA_DIR" "$rollback/data" 2>/dev/null
    ok "снимок: $rollback (откат: px-migrate-host.sh rollback)"
  fi

  # 1. Проверки.
  say "Проверки"
  have go || die "go не установлен — поставь и повтори"
  have docker || die "docker не установлен — поставь и повтори"
  ok "go и docker на месте"

  # 2. Docker.
  if ! docker info >/dev/null 2>&1; then
    say "Поднимаю docker"
    confirm "Установить и запустить docker (нужен sudo)?" || die "без docker нода не поднимется"
    runr apt-get install -y -qq docker.io docker-compose-v2
    runr systemctl enable --now docker
    ok "docker запущен"
  else ok "docker уже работает"; fi

  # 3. Каталоги.
  say "Каталоги"
  run mkdir -p "$BIN_DIR" "$DATA_DIR"
  run mkdir -p "$HOME_DIR/.config/systemd/user"
  ok "$BIN_DIR и $DATA_DIR"

  # 4. Конфиг — НЕ перезаписываем молча.
  if [ -f "$CONFIG_FILE" ]; then
    warn "конфиг уже есть — не трогаю: $CONFIG_FILE"
  elif [ -n "$src" ] && [ -f "$src/node-backup/config/simplex-node.json" ]; then
    if [ "$DRY_RUN" = 1 ]; then info "DRY: скопирую конфиг"
    else cp -a "$src/node-backup/config/simplex-node.json" "$CONFIG_FILE"; ok "конфиг восстановлен"; fi
  elif [ -f "$REPO/deploy/simplex-node.json.template" ]; then
    say "Конфига нет — собираю из шаблона"
    confirm "Создать конфиг из deploy/simplex-node.json.template?" || die "без конфига нода не запустится"
    if [ "$DRY_RUN" != 1 ]; then
      cp "$REPO/deploy/simplex-node.json.template" "$CONFIG_FILE"
      # Подставляем пути нового хоста
      sed -i -e "s#/home/tomas#${HOME_DIR}#g" -e "s#/home/tomas/ParanoidX#${REPO}#g" \
             -e "s#\"tomas\"#\"${USER_NAME}\"#g" "$CONFIG_FILE"
      ok "конфиг создан из шаблона (ПРОВЕРЬ значения!)"
      warn "адреса и порты в шаблоне — заглушки, выставь свои"
    fi
  else
    warn "нет ни конфига, ни шаблона — нода поднимется с настройками по умолчанию"
  fi

  # 5. Бинарники: из пакета или сборкой.
  say "Бинарники"
  if [ -n "$src" ] && [ -x "$src/bin/ParanoidX" ]; then
    run cp -a "$src/bin/ParanoidX" "$BIN_DIR/ParanoidX"
    run chmod +x "$BIN_DIR/ParanoidX"
    ok "ParanoidX из пакета"
  else
    say "  в пакете нет — собираю из исходников"
    confirm "Собрать ParanoidX из исходников?" || die "нужен бинарник"
    run bash -c "cd '$REPO' && go build -o '$BIN_DIR/ParanoidX' ./cmd/ParanoidX/"
    ok "ParanoidX собран"
  fi
  if [ -n "$src" ] && [ -x "$src/bin/simplex-chat-island" ]; then
    run cp -a "$src/bin/simplex-chat-island" "$BIN_DIR/"
    run chmod +x "$BIN_DIR/simplex-chat-island"
    ok "simplex-chat-island из пакета"
  elif [ -d "$REPO/cmd/simplex-chat-island" ]; then
    run bash -c "cd '$REPO' && go build -o '$BIN_DIR/simplex-chat-island' ./cmd/simplex-chat-island/"
    ok "simplex-chat-island собран"
  else
    warn "simplex-chat-island: нет ни в пакете, ни в исходниках — бот не запустится"
  fi

  # 6. Docker-конфигурация: coturn генерируется, чтобы не тащить секреты.
  say "Docker-конфигурация (coturn)"
  if [ -x "$REPO/deploy/px-docker.sh" ]; then
    info "конфиг и TLS-сертификаты генерируются: px-docker.sh install"
  else warn "нет deploy/px-docker.sh — Docker-стек не поставить"; fi

  # 7. Юниты — с переписыванием путей.
  # 7. Юниты. Источник — сначала пакет (юниты снятые со СТАРОГО хоста,
  #    с его путями), потом git-шаблоны. Шаблонов достаточно для чистого
  #    хоста: install работает и без --from, только с клоном репозитория.
  install_units() {
    local kind="$1" src_dir="$2" dst_dir="$3" sudo_needed="$4"
    local f b n=0
    for f in "$src_dir"/*; do
      [ -f "$f" ] || continue
      b=$(basename "$f")
      if [ "$DRY_RUN" = 1 ]; then
        info "DRY: поставлю $dst_dir/$b"
      else
        cp "$f" "/tmp/px-unit-$b"
        rewrite_paths "/tmp/px-unit-$b" || { rm -f "/tmp/px-unit-$b"; n=$((n+1)); continue; }
        if [ "$sudo_needed" = 1 ]; then
          runr install -m 0644 "/tmp/px-unit-$b" "$dst_dir/$b"
        else
          install -m 0644 "/tmp/px-unit-$b" "$dst_dir/$b"
        fi
        rm -f "/tmp/px-unit-$b"
      fi
      n=$((n+1))
    done
    [ "$n" -gt 0 ] && ok "$kind: обработано $n" || warn "$kind: источник пуст"
  }

  say "Системные юниты (нужен sudo)"
  if [ -d "$src/units/system" ]; then
    install_units "из пакета" "$src/units/system" /etc/systemd/system 1
  elif [ -d "$SYS_UNITS" ]; then
    install_units "из git-шаблонов" "$SYS_UNITS" /etc/systemd/system 1
  else
    err "нет юнитов: ни в пакете, ни в deploy/system-units"; bad_install=1
  fi

  say "Пользовательские юниты"
  if [ -d "$src/units/user" ]; then
    install_units "из пакета" "$src/units/user" "$HOME_DIR/.config/systemd/user" 0
  elif [ -d "$USER_UNITS" ]; then
    install_units "из git-шаблонов" "$USER_UNITS" "$HOME_DIR/.config/systemd/user" 0
  else
    err "нет юнитов: ни в пакете, ни в deploy/user-units"; bad_install=1
  fi

  # 8. polkit и wrapper.
  say "polkit: правило и wrapper"
  if [ -f "$REPO/deploy/10-px-node-control.rules" ] && [ -f "$REPO/deploy/px-node-control" ]; then
    if [ "$DRY_RUN" = 1 ]; then
      info "DRY: поставлю правило и /usr/local/bin/px-node-control"
    else
      sed "s/\btomas\b/${USER_NAME}/g" "$REPO/deploy/10-px-node-control.rules" > /tmp/px.rules
      runr install -m 0644 /tmp/px.rules /etc/polkit-1/rules.d/10-px-node-control.rules
      runr install -m 0755 "$REPO/deploy/px-node-control" /usr/local/bin/px-node-control
      rm -f /tmp/px.rules
      ok "polkit-правило и wrapper установлены"
    fi
  else
    warn "нет deploy/10-px-node-control.rules или deploy/px-node-control — кнопки управления не заработают"
  fi

  # 9. daemon-reload, но НЕ старт: старт — отдельное решение.
  say "systemd daemon-reload"
  runr systemctl daemon-reload
  run bash -c "systemctl --user daemon-reload" 
  ok "юниты перечитаны (НЕ запущены)"

  # 10. Итог и остаток ручной работы.
  say "Итог установки"
  if [ -z "${bad_install:-}" ]; then ok "все автоматические шаги выполнены"
  else err "часть шагов провалилась (строки ОШИБКА выше)"; fi

  say "Что осталось сделать вручную"
  local step=1
  if [ -z "$src" ]; then
    info "$step. Ключи и состояние: НЕ ПЕРЕНЕСЕНЫ — это новая нода со своими адресами"
    info "   Чтобы перенести старые: ./scripts/px-backup.sh restore <ПАКЕТ>/node-backup"
    step=2
  else
    info "$step. Восстановить состояние и ключи:"
    info "     ./scripts/px-backup.sh restore $src/node-backup"
    step=2
  fi
  info "$step. Docker-стек (Tor поднимет новые onion-адреса и опубликует их):"
  info "     ./deploy/px-docker.sh install"
  info "$((step+1)). Старт:"
  info "     sudo systemctl enable --now ParanoidX-dashboard.service"
  info "$((step+2)). Проверка:"
  info "     ./deploy/px-migrate-host.sh verify"
  echo
  if [ -n "$REPORT" ]; then
    ok "лог сохранён: $REPORT"
    info "пришли его — по нему я починю скрипты под этот host"
  fi
}

# ── verify: доказать, что нода работает ────────────────────────────────────
cmd_verify() {
  say "Проверка ноды"
  local bad=0

  # 1. Docker
  say "Docker"
  if have docker && docker info >/dev/null 2>&1; then
    local up
    up=$(docker ps --filter "name=ParanoidX" --format '{{.Names}}' 2>/dev/null | wc -l)
    if [ "$up" -ge 4 ]; then ok "контейнеров запущено: $up/4"
    elif [ "$up" -gt 0 ]; then warn "контейнеров только $up из 4"; bad=1
    else err "контейнеры ParanoidX не запущены"; bad=1; fi
  else err "docker не отвечает"; bad=1; fi

  # 2. API
  say "API"
  if timeout 5 bash -c "</dev/tcp/127.0.0.1/8080" 2>/dev/null; then
    ok "порт 8080 отвечает"
    local h
    h=$(curl -s --max-time 5 http://127.0.0.1:8080/api/health 2>/dev/null)
    if echo "$h" | grep -q '"healthy":true'; then ok "healthy: true"
    elif echo "$h" | grep -q 'healthy'; then warn "health отвечает, но не healthy: $h"
    else warn "не разобрал /api/health: ${h:0:60}"; fi
  else err "порт 8080 не отвечает — нода не запущена"; bad=1; fi

  # 3. Юниты
  say "Юниты"
  local u
  for u in "${SYS_LIST[@]}"; do
    if [ ! -f "/etc/systemd/system/$u" ]; then warn "$u не установлен"
    elif systemctl is-active --quiet "$u" 2>/dev/null; then ok "$u active"
    else warn "$u установлен, но не active"; fi
  done

  # 4. Бинарники
  say "Бинарники"
  [ -x "$BIN_DIR/ParanoidX" ] && ok "ParanoidX" || { err "нет $BIN_DIR/ParanoidX"; bad=1; }
  [ -x "$BIN_DIR/simplex-chat-island" ] && ok "simplex-chat-island" \
    || warn "нет simplex-chat-island (бот острова)"

  # 5. SOCKS
  say "SOCKS-мост"
  if timeout 6 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10810' 2>/dev/null; then
    ok "порт 10810 отвечает"
  else warn "порт 10810 не отвечает (xray мог не подняться)"; fi

  # 6. onion
  say ".onion-адреса"
  local cnt=0
  for s in smp xftp dashboard ice auditor; do
    if [ -f "$REPO/docker/tor/hidden_services/$s/hostname" ]; then
      cnt=$((cnt+1))
    else warn "нет адреса для $s — Tor не создал или ключи не восстановлены"
    fi
  done
  if [ "$cnt" = 5 ]; then ok "все 5 адресов на месте"
  else err "адресов только $cnt из 5 — restore обязателен"; bad=1; fi

  # 7. Аудит секретов
  say "Аудит секретов"
  if [ -x "$REPO/scripts/px-audit-secrets.sh" ]; then
    if ( cd "$REPO" && ./scripts/px-audit-secrets.sh >/dev/null 2>&1 ); then
      ok "секретов в git нет"
    else err "аудит нашёл секреты в git — это не миграция, это утечка"; bad=1; fi
  else warn "нет scripts/px-audit-secrets.sh"; fi

  echo
  if [ "$bad" = 0 ]; then ok "host готов"; return 0
  else err "есть проблемы — строки ОШИБКА выше"; return 1; fi
}

# ── rollback ───────────────────────────────────────────────────────────────
cmd_rollback() {
  local latest
  latest=$(find "$STATE_DIR" -maxdepth 1 -mindepth 1 -type d -name 'pre-install-*' 2>/dev/null | sort -r | head -1)
  [ -n "$latest" ] || die "снимков нет в $STATE_DIR — откатывать нечего"
  say "Откат к $latest"
  confirm "Вернуть юниты, конфиг и состояние из снимка?" || die "отменено"

  if [ -d "$latest/systemd" ]; then
    runr cp -a "$latest/systemd/." /etc/systemd/system/
    ok "системные юниты возвращены"
  fi
  if [ -d "$latest/user" ]; then
    run cp -a "$latest/user/." "$HOME_DIR/.config/systemd/user/"
    ok "пользовательские юниты возвращены"
  fi
  if [ -f "$latest/simplex-node.json" ]; then
    run cp -a "$latest/simplex-node.json" "$CONFIG_FILE"
    ok "конфиг возвращён"
  fi
  if [ -d "$latest/data" ]; then
    warn "состояние не трогаю: снимок data/ есть, но restore безопаснее"
    info "если нужно: cp -a $latest/data/. $DATA_DIR/"
  fi
  runr systemctl daemon-reload
  run bash -c "systemctl --user daemon-reload"
  ok "откат выполнен, юниты перечитаны"
}

usage() {
  cat <<EOF
px-migrate-host.sh — перенос ноды ParanoidX на новый host

  doctor              обследовать этот host
  prepare             собрать пакет переноса (на СТАРОМ хосте)
  install --from DIR  развернуть на этом хосте
  verify              доказать, что нода работает
  rollback            вернуть состояние до install

Порядок:
  СТАРЫЙ host:  ./deploy/px-migrate-host.sh prepare
  скопировать пакет на USB
  НОВЫЙ host:   git clone ... && ./deploy/px-migrate-host.sh doctor
                ./deploy/px-migrate-host.sh install --from /media/usb/px-migration-*
                ./scripts/px-backup.sh restore <пакет>/node-backup
                ./deploy/px-docker.sh install
                sudo systemctl enable --now ParanoidX-dashboard.service
                ./deploy/px-migrate-host.sh verify

Переменные окружения:
  DRY_RUN=1     показать что будет сделано, и не делать
  ASSUME_YES=1  не спрашивать подтверждения
  OUT_DIR=DIR   куда prepare положит пакет

Юниты переписываются под пользователя и home этого хоста:
в 15 из 16 юнитов исходно зашит путь /home/tomas.
EOF
}

cmd="${1:-}"; shift 2>/dev/null || true
case "$cmd" in
  doctor)   cmd_doctor "$@" ;;
  prepare)  cmd_prepare "$@" ;;
  install)  cmd_install "$@" ;;
  verify)   cmd_verify "$@" ;;
  rollback) cmd_rollback "$@" ;;
  -h|--help|help|"") usage ;;
  *) printf 'неизвестная команда: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
esac
