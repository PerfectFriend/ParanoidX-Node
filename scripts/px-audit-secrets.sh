#!/usr/bin/env bash
# px-audit-secrets.sh — проверка того, что секреты не попали в git.
#
# Запускать перед КАЖДЫМ push и в CI. Только чтение, ничего не меняет.
#
#   ./scripts/px-audit-secrets.sh
#
# Проверяет три вещи, потому что секрет может утёчь тремя разными
# путями:
#   1. ИНДЕКС   — файл подозрительного имени отслеживается прямо сейчас.
#   2. ИСТОРИЯ  — секрет остался в коммитах (git rm --cached не помогает,
#                 нужен filter-repo).
#   3. СОДЕРЖИМОЕ — файл с безобидным именем, но внутри приватный ключ.
#                 Ловушка, на которую попадают почти все: config.json,
#                 settings.py, .env.example.
#
# Нулевой код возврата = чисто.

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2
REPO="${REPO:-$PWD}"

FAILED=0
ok()   { printf '  \033[32mOK\033[0m   %s\n' "$*"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$*"; FAILED=1; }
warn() { printf '  \033[33mWARN\033[0m %s\n' "$*"; }
head_() { printf '\n\033[1m%s\033[0m\n' "$*"; }

# ── Что считаем секретом ────────────────────────────────────────────────────
# Имена файлов. Расширение .key — приватный по определению.
NAME_PATTERNS='\.key$
hs_ed25519_secret_key$
^Gemma4Token$
(^|/)fingerprint$
(^|/)ca\.srl$
\.token$'

# Признаки внутри содержимого.
#
# ВАЖНО: эти шаблоны НЕ должны совпадать с комментариями и со строками
# кода. Первая версия аудита дала два ложных срабатывания именно так:
#   * deploy/px-docker.sh содержит комментарий «== ed25519v1-secret: type0 ==»
#     и команду `grep -q 'ed25519v1-secret'` — то есть сам скрипт, который
#     ВАЛИДИРУЕТ ключ, попадал в список «файлов с ключом».
#   * docker/v2ray/certs и hidden_services упоминаются в коммите, где
#     они были УДАЛЕНЫ, — git log показывает коммит и для удалённого пути.
# Поэтому: (а) ключевые слова ловятся только в начале строки без '#',
# (б) для истории спрашиваем про конкретные файлы, а не про каталоги.
CONTENT_PATTERNS='^-----BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY
^[0-9]{8,10}:[A-Za-z0-9_-]{30,}
^static-auth-secret=[A-Za-z0-9+/]{20,}
^secret[_-]?key["'"'"' ]*[:=][^<{$[:space:]]{16,}
^api_key["'"'"' ]*[:=][^<{$[:space:]]{16,}'

head_ "1. Индекс: нет ли отслеживаемых секретов"

tracked=$(git ls-files 2>/dev/null)
hits=$(printf '%s\n' "$tracked" | grep -E "$NAME_PATTERNS" 2>/dev/null)
if [ -n "$hits" ]; then
  bad "в индексе $(printf '%s\n' "$hits" | wc -l) секрет(ов):"
  printf '%s\n' "$hits" | sed 's/^/         /'
  printf '         исправить: git rm --cached <файл>\n'
else
  ok "ни один секрет не отслеживается"
fi

head_ "2. Содержимое: нет ли ключей в безобидных файлах"

# Самое недооценённое место. Ограничиваем размер, чтобы не читать
# бинарники (ключ .key — 96 байт, но .pub рядом тоже попадёт).
leaked=0
for f in $tracked; do
  case "$f" in
    *.png|*.jpg|*.jpeg|*.gif|*.ico|*.webp|*.pdf|*.zip|*.tar|*.gz|*.xz) continue ;;
    install-pack/bin/*) continue ;;
  esac
  [ -f "$f" ] || continue
  if head -c 20000 "$f" 2>/dev/null | grep -qE "$CONTENT_PATTERNS"; then
    # Отличаем «файл и есть приватный ключ» (уже пойман в п.1) от
    # «секрет спрятан внутри» — второе и есть находка.
    if printf '%s' "$f" | grep -qE "$NAME_PATTERNS"; then
      :
    else
      bad "секрет внутри безобидно названного файла: $f"
      leaked=$((leaked + 1))
    fi
  fi
done
[ "$leaked" = 0 ] && ok "секретов в содержимом файлов не найдено"

head_ "3. История: не осталось ли секретов в коммитах"

hist_bad=0
# Спрашиваем про КОНКРЕТНЫЕ файлы, а не про каталоги. `git log -- <dir>`
# показывает и коммиты, где каталог был УДАЛЁН, — а это не утечка.
for path in 'Gemma4Token' 'install-pack/xray/config.json'; do
  n=$(git log --all --oneline -- "$path" 2>/dev/null | wc -l)
  if [ "$n" -gt 0 ]; then
    bad "история: $path упоминается в $n коммитах — нужен filter-repo"
    hist_bad=1
  fi
done

# Секретные ключи: точное имя файла + glob по всем 5 сервисам.
keyfiles=$(git log --all --name-only --format= 2>/dev/null \
           | grep -E 'hs_ed25519_secret_key$|\.key$' | sort -u)
if [ -n "$keyfiles" ]; then
  bad "история: приватные ключи всё ещё в $(printf '%s\n' "$keyfiles" | wc -l) файл(ах):"
  printf '%s\n' "$keyfiles" | head -6 | sed 's/^/         /'
  hist_bad=1
fi

# Объекты в pack-файле: самый надёжный признак. Именно он ловит
# секрет, который формально не привязан к коммиту (dangling blob).
# `grep -c` может выдать несколько строк, если поток не пуст — берём
# последнее значение через tr, иначе `[: 0\n0: integer expected`.
keyobjs=$(git rev-list --all --objects 2>/dev/null \
          | grep -cE '\.key$|hs_ed25519_secret_key' | tail -1)
keyobjs=${keyobjs:-0}
case "$keyobjs" in ''|*[!0-9]*) keyobjs=0 ;; esac
if [ "$keyobjs" -gt 0 ]; then
  bad "в объектах репозитория: $keyobjs секрет(ов) — нужен filter-repo"
  hist_bad=1
fi
[ "$hist_bad" = 0 ] && ok "в истории секретов нет"

head_ "4. Собранные образы/бэкапы не коммитятся"

# 21 МБ Go-бинарника в git означает, что репозиторий не клонируется
# на другой host и раздувает каждый клон.
if [ -f install-pack/bin/simplex-node ]; then
  if git ls-files --error-unmatch install-pack/bin/simplex-node >/dev/null 2>&1; then
    bad "install-pack/bin/simplex-node в git — собирать на хосте (go build)"
  else
    ok "бинарник не в git"
  fi
else
  ok "бинарника нет в рабочем дереве"
fi

head_ "5. .gitignore держит новые секреты"
# Проверяем правила, а не только наличие файлов: после `git add -f`
# правило не сработает, и это должно быть видно.
for probe in docker/xftp_configs/ca.key docker/tor/hidden_services/smp/hs_ed25519_secret_key \
             docker/coturn/turn_key.pem docker/coturn/turnserver.conf Gemma4Token; do
  if git check-ignore -q "$probe" 2>/dev/null; then
    ok "$probe игнорируется"
  else
    bad "$probe НЕ игнорируется — секрет может утечь"
  fi
done

echo
if [ "$FAILED" = 0 ]; then
  printf '\033[32mСЕКРЕТОВ В GIT НЕТ\033[0m\n'
  exit 0
fi
printf '\033[31mНАЙДЕНЫ ПРОБЛЕМЫ С СЕКРЕТАМИ — см. FAIL выше\033[0m\n'
exit 1
