#!/usr/bin/env bash
# mut-nodeid-test.sh — мутационная проверка контракта GenerateNodeID.
#
# Зачем (пункт 1 плана + правило 1): доказать, что тесты
# nodeid_test.go не пустые. Каждая мутация ниже ломает ровно одну
# часть контракта — и скрипт проверяет, что тест её замечает
# утверждением (или паникой), а НЕ ошибкой компиляции и НЕ молчаливо.
#
# Правила классификации (уроки слотов 10-03 и 10-04):
#   * мутация считается применённой только если файл реально
#     изменился (git diff) И в нём найден маркер мутации — иначе
#     зелёный тест гоняется на чистой копии и результат ложный;
#   * «поймана» = --- FAIL: или panic: в выводе go test;
#     build failed — невидимо: мутация-в-компиляцию ничего не
#     доказывает;
#   * PASS=<мутации, пойманные тестом> FAIL=<не пойманы или
#     не применились>; финальный гейт плана: -count=100.
#
# Использование: bash scripts/mut-nodeid-test.sh
set -u
cd "$(git rev-parse --show-toplevel)"

FILE=internal/registry/registry.go
RUN='TestGenerateNodeIDFormat|TestGenerateNodeIDKeyBinding|TestGenerateNodeIDSalt|TestGenerateNodeIDUniqueness|TestGenerateNodeIDEmptyKey'

PASS=0
FAIL=0

revert() { git checkout -- "$FILE"; }

# run_and_classify: прогоняет целевые тесты и классифицирует вывод.
#   0 = поймана (утверждение или паника в целевом тесте)
#   1 = ошибка компиляции (мутация ничего не доказывает)
#   2 = зелёный (мутация не поймана — тест пустой)
#   3 = не классифицировано
run_and_classify() {
  local out
  out="$(go test ./internal/registry/ -run "$RUN" -count=1 2>&1)"
  if grep -q 'build failed' <<<"$out"; then
    echo "   результат: ОШИБКА КОПИЛЯЦИИ — мутация не доказывает ничего"
    grep -E '\.go:[0-9]+:' <<<"$out" | sed 's/^/     /' | head -5
    return 1
  fi
  if grep -qE '^--- FAIL:|panic:' <<<"$out"; then
    echo "   результат: ПОЙМАНА — $(grep -oE '^--- FAIL: [A-Za-z]+' <<<"$out" | head -1)"
    grep -m1 -E '^    .*_test\.go' <<<"$out" | sed 's/^/     /' || true
    return 0
  fi
  if grep -q '^ok ' <<<"$out"; then
    echo "   результат: ЗЕЛЁНЫЙ — мутация НЕ поймана (тест пустой)"
    return 2
  fi
  echo "   результат: НЕ КЛАССИФИЦИРОВАНО — вывод ниже"
  sed 's/^/     /' <<<"$out"
  return 3
}

# mutation ИМЯ SED-ВЫРАЖЕНИЕ МАРКЕР: применяет одну строчную мутацию,
# проверяет применение, гоняет тесты, откатывает файл.
mutation() {
  local name="$1" expr="$2" marker="$3"
  echo "== $name"
  revert
  sed -i "$expr" "$FILE"
  if git diff --quiet -- "$FILE" || ! grep -qF "$marker" "$FILE"; then
    echo "   $name: мутация не применилась — считаю FAIL"
    revert
    FAIL=$((FAIL+1))
    return
  fi
  if run_and_classify; then
    PASS=$((PASS+1))
  else
    FAIL=$((FAIL+1))
  fi
  revert
}

echo "== sanity: зелёный базовый прогон (до мутаций)"
if go test ./internal/registry/ -run "$RUN" -count=5 2>&1 | grep -q '^ok '; then
  echo "   база: ok (5 прогонов)"
else
  echo "   база красная — останавливаюсь: сначала чинить дерево"
  echo "PASS=0 FAIL=1"
  exit 1
fi

# M1: сузить тело до 32 бит (8 hex) — гейт-мутация из пункта 1 плана.
mutation "M1: тело 32 бита (8 hex)" \
  's/hex\.EncodeToString(body\[:\])/hex.EncodeToString(body[:4])/' \
  'hex.EncodeToString(body[:4])'

# M2: ключ не участвует (нулевое копирование хвоста-кписка ключа).
mutation "M2: ключ не привязан" \
  's/copy(body\[8:\], sum\[:8\])/copy(body[8:], sum[:0])/' \
  'copy(body[8:], sum[:0])'

# M3: соль отключена — идентификатор прибит к ключу.
mutation "M3: без соли" \
  's/rand\.Read(body\[:8\])/rand.Read(body[:0])/' \
  'rand.Read(body[:0])'

# M4: возврат к исходной buggy-реализации (math/rand, 32 бита,
# ключ не участвует). Мультисимвольная — отдельно от mutation().
echo "== M4: исходная реализация (math/rand, 32 бита, без ключа)"
revert
sed -i -e '/"crypto\/rand"/d' \
       -e '/"crypto\/sha256"/d' \
       -e '/"encoding\/hex"/d' \
       -e 's/"encoding\/json"/"encoding\/json"\n\t"fmt"\n\t"math\/rand"/' "$FILE"
perl -0pi -e 's/func GenerateNodeID\(pubkey string\) string \{\n\tvar body \[16\]byte\n.*?\n\}/func GenerateNodeID(pubkey string) string {\n\th := fmt.Sprintf("%x", struct{ uint32 }{rand.Uint32()})\n\treturn "node-" + h[:8]\n}/s' "$FILE"
if git diff --quiet -- "$FILE" || ! grep -qF 'rand.Uint32()' "$FILE"; then
  echo "   M4: мутация не применилась — считаю FAIL"
  revert
  FAIL=$((FAIL+1))
else
  if run_and_classify; then
    PASS=$((PASS+1))
  else
    FAIL=$((FAIL+1))
  fi
  revert
fi

echo "== sanity: после отката дерево снова зелёное"
if [ -n "$(git status --porcelain -- "$FILE")" ]; then
  echo "   файл не восстановлен — считаю FAIL"
  FAIL=$((FAIL+1))
elif go test ./internal/registry/ -run "$RUN" -count=5 2>&1 | grep -q '^ok '; then
  echo "   откат: ok (5 прогонов)"
else
  echo "   откат красный — считаю FAIL"
  FAIL=$((FAIL+1))
fi

echo "== гейт плана: go test ./internal/registry/ -count=100"
if go test ./internal/registry/ -count=100 2>&1 | grep -q '^ok '; then
  echo "   -count=100: ok, ноль падений"
else
  echo "   -count=100: ПАДЕНИЯ — гейт не пройден"
  FAIL=$((FAIL+1))
fi

echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
