#!/usr/bin/env bash
# Mutation test for the TestChatArchiveLifecycle fix.
# Claim under test: the test is no longer calendar-dependent and still fails
# when archiving is genuinely broken.
#
# NOTE: go test must run from inside the module. Earlier revisions of this
# script used an absolute package path from /tmp, which fails with
# "go.mod file not found" on EVERY configuration — that produced a red
# baseline and false "OK" mutations.
set -uo pipefail
P=/mnt/data/ParaNodeX
W=/tmp/mut-archive.$$
PASS=0; FAIL=0
strip() { env -u http_proxy -u https_proxy -u ALL_PROXY -u all_proxy -u HTTP_PROXY -u HTTPS_PROXY "$@"; }
cleanup() { rm -rf "$W"; }
trap cleanup EXIT

# run test inside a module dir; $1=repo, $2=label, $3=expect(0=fail)
runtest() {
  local repo="$1"
  ( cd "$repo" && strip timeout 200 go test ./internal/api/ -run TestChatArchiveLifecycle -count=1 >/dev/null 2>&1 )
  echo $?
}

echo "=== sanity: go test must work from inside the module ==="
rc=$(runtest "$P")
if [[ "$rc" == "0" ]]; then
  echo "  OK   module-local go test works (exit 0)"; PASS=$((PASS+1))
else
  echo "  FAIL module-local go test broken (exit $rc) — harness is wrong"; FAIL=$((FAIL+1))
fi

echo
echo "=== M1: archiving disabled -> test must FAIL ==="
rm -rf "$W"; mkdir -p "$W"; cp -a "$P" "$W/repo"
python3 - "$W/repo/internal/api/chat.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old="cutoff := time.Now().AddDate(0, 0, -days)"
new="cutoff := time.Now().AddDate(0, 0, -days)\n\tcutoff = time.Now().AddDate(0, 0, 3650) // MUT: archive nothing"
assert old in s, "anchor not found"
open(p,"w").write(s.replace(old,new,1))
PY
rc=$(runtest "$W/repo")
[[ "$rc" != "0" ]] && { echo "  OK   test catches archiving disabled (exit $rc)"; PASS=$((PASS+1)); } \
                  || { echo "  FAIL test passed with archiving disabled — VACUOUS"; FAIL=$((FAIL+1)); }

echo
echo "=== M2: stale hard-coded dates -> test must FAIL (reproduces original rot) ==="
rm -rf "$W"; mkdir -p "$W"; cp -a "$P" "$W/repo"
python3 - "$W/repo/internal/api/chat_integration_test.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
a='oldTime := now.AddDate(0, 0, -200).Format(time.RFC3339)'
b='recentTime := now.AddDate(0, 0, -10).Format(time.RFC3339)'
assert a in s and b in s, "anchors not found"
s=s.replace(a,'oldTime := "2025-01-01T00:00:00Z"; _ = now',1)
s=s.replace(b,'recentTime := "2026-07-01T00:00:00Z"',1)
open(p,"w").write(s)
PY
rc=$(runtest "$W/repo")
[[ "$rc" != "0" ]] && { echo "  OK   stale dates reproduce the failure (exit $rc)"; PASS=$((PASS+1)); } \
                  || { echo "  FAIL stale dates pass — the rot was not real"; FAIL=$((FAIL+1)); }

echo
echo "=== M3: future timestamp guard -> test must FAIL (proves it validates age) ==="
rm -rf "$W"; mkdir -p "$W"; cp -a "$P" "$W/repo"
python3 - "$W/repo/internal/api/chat.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old="\t\tif err != nil || t.After(cutoff) {"
new="\t\tif err != nil { // MUT: keep everything\n\t\t\tkept = append(kept, m)"
assert old in s, "anchor not found"
open(p,"w").write(s.replace(old,new,1))
PY
rc=$(runtest "$W/repo")
[[ "$rc" != "0" ]] && { echo "  OK   test catches broken age filter (exit $rc)"; PASS=$((PASS+1)); } \
                  || { echo "  FAIL test passed with broken age filter"; FAIL=$((FAIL+1)); }

echo
echo "======================================"
echo "PASS=${PASS}  FAIL=${FAIL}"
if [[ $FAIL -eq 0 ]]; then
  echo "OK  fix is real: calendar-proof and not vacuous"
else
  echo "ERR vacuous or incomplete fix"
fi
exit $(( FAIL > 0 ? 1 : 0 ))