#!/usr/bin/env python3
"""Проверка scripts/px-backup.sh — бэкапа, верификации и восстановления.

Живёт в репозитории, а не в /tmp: ad-hoc проверки в /tmp исчезают, и
через месяц никто не вспомнит, что бэкап вообще проверяли.

    python3 tests/verify_px_backup.py

Ничего живого не трогает: BACKUP_ROOT, HS_DIR, DATA_DIR и CONFIG_FILE
перенаправляются в песочницу. Мутации применяются к копиям и обязаны
ломать проверки — иначе они пустые.
"""
import os
import shutil
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPT = f"{REPO}/scripts/px-backup.sh"
SVCS = ["smp", "xftp", "dashboard", "ice", "auditor"]
N, F = [0], [0]


def ok(m):
    N[0] += 1
    print(f"  PASS  {m}")


def bad(m, d=""):
    N[0] += 1
    F[0] += 1
    print(f"  FAIL  {m}" + (f"  {d}" if d else ""))


def check(c, m, d=""):
    ok(m) if c else bad(m, d)


def make_key(path, valid=True):
    """Ключ Tor ed25519v1: 32-байтовый заголовок + 64 байта = 96."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(b"== ed25519v1-secret: type0 ==" + b"\x00" * 3 +
                (b"\x11" * 64 if valid else b"CORRUPT" * 12))


def sandbox():
    """Фейковая нода: 5 onion-сервисов + конфиг + состояние."""
    box = tempfile.mkdtemp(prefix="pxbak-")
    hs = os.path.join(box, "hs")
    for s in SVCS:
        make_key(os.path.join(hs, s, "hs_ed25519_secret_key"))
        with open(os.path.join(hs, s, "hostname"), "w") as f:
            f.write(f"{s[0] * 56}.onion\n")
    data = os.path.join(box, "data")
    os.makedirs(data)
    with open(os.path.join(data, "simplex-node.json"), "w") as f:
        f.write('{"node":"test"}')
    with open(os.path.join(data, "wallet.db"), "w") as f:
        f.write("WALLET")
    return box, hs, data


def run(box, hs, data, *args, dry=False, script=SCRIPT):
    env = dict(os.environ, BACKUP_ROOT=os.path.join(box, "backups"), HS_DIR=hs,
               DATA_DIR=data, CONFIG_FILE=os.path.join(data, "simplex-node.json"),
               REPO=REPO, DOCKER_DIR=os.path.join(box, "docker"))
    if dry:
        env["DRY_RUN"] = "1"
    p = subprocess.run(["bash", script, *args], capture_output=True, text=True,
                       env=env, timeout=180)
    return p


def first_backup(box):
    d = os.path.join(box, "backups")
    return os.path.join(d, os.listdir(d)[0]) if os.path.isdir(d) and os.listdir(d) else ""


# ── [A] синтаксис и CLI ────────────────────────────────────────────────────
print("[A] синтаксис и CLI")
r = subprocess.run(["bash", "-n", SCRIPT], capture_output=True, text=True)
check(r.returncode == 0, "bash -n проходит", r.stderr.strip()[:100])
p = subprocess.run(["bash", SCRIPT, "--help"], capture_output=True, text=True, timeout=60)
for c in ("save", "onion", "list", "verify", "restore"):
    check(c in p.stdout, f"команда {c} документирована")
check("CONFIG_FILE" in open(SCRIPT, encoding="utf-8").read(),
      "конфиг берётся из CONFIG_FILE (а не из ~/.config — там его нет)")

# ── [B] onion: полный бэкап ключей ─────────────────────────────────────────
print("[B] onion: 5 сервисов с проверкой ключей")
box, hs, data = sandbox()
try:
    p = run(box, hs, data, "onion")
    check(p.returncode == 0, "onion отработал", p.stderr[:150])
    d = first_backup(box)
    hsd = os.path.join(d, "hidden_services")
    check(all(os.path.isfile(os.path.join(hsd, s, "hs_ed25519_secret_key"))
              for s in SVCS), "все 5 ключей скопированы (в hidden_services/)")
    modes = {oct(os.stat(os.path.join(hsd, s, "hs_ed25519_secret_key")).st_mode & 0o777)
             for s in SVCS}
    check(modes == {"0o600"}, f"ключи 600 ({modes})")
    check(oct(os.stat(d).st_mode & 0o777) == "0o700", "каталог бэкапа 700")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [C] verify принимает годный бэкап ──────────────────────────────────────
print("[C] verify: принимает свой бэкап")
box, hs, data = sandbox()
try:
    run(box, hs, data, "onion")
    d = first_backup(box)
    p = run(box, hs, data, "verify", d)
    check(p.returncode == 0, "verify принял валидный бэкап")
    check("пригоден" in p.stdout, "verify явно говорит «пригоден»")
    # onion — это бэкап ТОЛЬКО ключей, конфиг копирует save. Поэтому
    # предупреждение о конфиге здесь корректно, и требовать его отсутствия
    # было ошибкой проверки, а не признаком бага.
    check("нет конфига ноды" in p.stdout,
          "verify честно предупреждает: в onion-бэкапе конфига нет (это save)")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [D] verify отвергает подделанный ключ ───────────────────────────────────
print("[D] verify: отвергает битый ключ")
box, hs, data = sandbox()
try:
    run(box, hs, data, "onion")
    d = first_backup(box)
    make_key(os.path.join(d, "hidden_services", "smp",
                          "hs_ed25519_secret_key"), valid=False)
    p = run(box, hs, data, "verify", d)
    check(p.returncode != 0, "битый ключ отвергнут")
    check("smp" in p.stdout, "назван проблемный сервис")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [E] бэкап без конфига — предупреждение, а не молчание ──────────────────
print("[E] verify: предупреждает об отсутствии конфига")
box, hs, data = sandbox()
try:
    run(box, hs, data, "onion")
    d = first_backup(box)
    shutil.rmtree(os.path.join(d, "hidden_services"))
    os.makedirs(os.path.join(d, "hidden_services"))
    for s in SVCS:
        make_key(os.path.join(d, "hidden_services", s, "hs_ed25519_secret_key"))
    p = run(box, hs, data, "verify", d)
    check("нет конфига" in p.stdout,
          "сказано, что нода поднимется с настройками по умолчанию")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [F] save: полный бэкап ────────────────────────────────────────────────
print("[F] save: конфиг, состояние, coturn")
box, hs, data = sandbox()
try:
    cd = os.path.join(box, "docker", "coturn")
    os.makedirs(cd)
    for f in ("turnserver.conf", "turn_cert.pem", "turn_key.pem"):
        with open(os.path.join(cd, f), "w") as fh:
            fh.write("X" * 200)
    p = run(box, hs, data, "save")
    check(p.returncode == 0, "save отработал", p.stderr[:150])
    d = first_backup(box)
    check(os.path.isfile(os.path.join(d, "config", "simplex-node.json")),
          "конфиг ноды сохранён (баг с ~/.config исправлен)")
    check(os.path.isfile(os.path.join(d, "data", "wallet.db")), "состояние сохранено")
    check(os.path.isfile(os.path.join(d, "coturn", "turn_key.pem")), "coturn TLS сохранён")
    check(oct(os.stat(os.path.join(d, "coturn", "turn_key.pem")).st_mode & 0o777)
          == "0o600", "coturn-ключ 600")
    check(run(box, hs, data, "verify", d).returncode == 0, "полный бэкап проходит verify")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [G] restore ────────────────────────────────────────────────────────────
print("[G] restore: ключи + адреса + страховка")
box, hs, data = sandbox()
try:
    os.makedirs(os.path.join(box, "docker", "coturn"), exist_ok=True)
    run(box, hs, data, "save")
    d = first_backup(box)
    # «Другая машина»: свои ключи и адреса.
    for s in SVCS:
        with open(os.path.join(hs, s, "hostname"), "w") as f:
            f.write(f"{'z' * 56}.onion\n")
        make_key(os.path.join(hs, s, "hs_ed25519_secret_key"))
    p = run(box, hs, data, "restore", d)
    env_patched = p
    # ASSUME_YES через отдельный вызов
    r2 = subprocess.run(["bash", SCRIPT, "restore", d], capture_output=True, text=True,
                        env=dict(os.environ, BACKUP_ROOT=os.path.join(box, "backups"),
                                 HS_DIR=hs, DATA_DIR=data,
                                 CONFIG_FILE=os.path.join(data, "simplex-node.json"),
                                 REPO=REPO, DOCKER_DIR=os.path.join(box, "docker"),
                                 ASSUME_YES="1"), timeout=180)
    check(r2.returncode == 0, "restore отработал", r2.stderr[:150])
    got = open(os.path.join(hs, "smp", "hs_ed25519_secret_key"), "rb").read()
    src = open(os.path.join(d, "hidden_services", "smp",
                            "hs_ed25519_secret_key"), "rb").read()
    check(got == src, "ключ взят ИЗ БЭКАПА")
    check(all(os.path.isfile(os.path.join(data, f"{s}_onion.txt")) for s in SVCS),
          "адреса опубликованы в DataDir (их читает нода)")
    check("docker compose up" in r2.stdout, "сказано, что рестарт за пользователем")
    bd = os.path.join(box, "backups")
    stashes = [x for x in os.listdir(bd) if x.startswith("pre-restore-")]
    check(stashes, "страховочная копия создана")
    check(os.path.isfile(os.path.join(bd, stashes[0], "hidden_services", "smp",
                                      "hs_ed25519_secret_key")),
          "в страховке ПРЕЖНИЙ ключ — откат возможен")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [H] restore отвергает неполный бэкап, ничего не ломая ─────────────────
print("[H] restore: неполный бэкап отвергнут ДО записи")
box, hs, data = sandbox()
try:
    run(box, hs, data, "onion")
    d = first_backup(box)
    shutil.rmtree(os.path.join(d, "hidden_services", "xftp"))
    before = open(os.path.join(hs, "smp", "hs_ed25519_secret_key"), "rb").read()
    p = run(box, hs, data, "restore", d)
    check(p.returncode != 0, "неполный бэкап отвергнут")
    check("не тронуто" in p.stdout or "не восстанавливаю" in p.stdout,
          "сказано, что живое состояние не тронуто")
    check(open(os.path.join(hs, "smp", "hs_ed25519_secret_key"), "rb").read() == before,
          "живой ключ НЕ изменён")
    check(not any(x.startswith("pre-restore-")
                  for x in os.listdir(os.path.join(box, "backups"))),
          "страховка не создавалась — отказ был до записи")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [I] DRY_RUN ничего не пишет ───────────────────────────────────────────
print("[I] DRY_RUN=1 не трогает файлы")
box, hs, data = sandbox()
try:
    run(box, hs, data, "onion", os.path.join(box, "dry"), dry=True)
    check(not os.path.exists(os.path.join(box, "dry")), "каталог не создан")
finally:
    shutil.rmtree(box, ignore_errors=True)

# ── [J] портируемость ─────────────────────────────────────────────────────
print("[J] нет жёстких путей")
src_text = open(SCRIPT, encoding="utf-8").read()
check("/home/tomas" not in src_text, "нет /home/tomas — переносимо")
# Путь к конфигу больше не собирается вручную из $HOME/.config — он
# приходит из DATA_DIR. Проверяем именно это, а не буквальную строку,
# которой в коде и не должно быть.
check('CONFIG_FILE="${CONFIG_FILE:-$DATA_DIR/simplex-node.json}"' in src_text,
      "CONFIG_FILE выводится из DATA_DIR, а не из ~/.config")
# Проверять сырую строку ".config/simplex-node" нельзя: она есть в
# КОММЕНТАРИИ, который объясняет именно этот баг. Ищем только в
# исполняемых строках (без ведущего #).
code_lines = [l for l in src_text.splitlines() if not l.lstrip().startswith("#")]
exec_text = "\n".join(code_lines)
check(".config" not in exec_text,
      "в исполняемом коде нет ручной ссылки на ~/.config — только DATA_DIR")
for v in ("BACKUP_ROOT", "HS_DIR", "DATA_DIR", "CONFIG_FILE", "DOCKER_DIR", "REPO"):
    check(f"{v}=" in src_text, f"{v} переопределяется через ENV")

# ── [K] мутации ────────────────────────────────────────────────────────────
print("[K] мутации: проверки не пустые")
mut_dir = tempfile.mkdtemp(prefix="pxbak-mut-")
try:
    mut = os.path.join(mut_dir, "px-backup.sh")
    shutil.copy(SCRIPT, mut)
    os.chmod(mut, 0o755)
    orig = open(mut, encoding="utf-8").read()

    # K1: убрать проверку битых ключей → verify перестанет отвергать.
    m1 = orig.replace('if is_valid_key "$hsd/$svc/hs_ed25519_secret_key"; then',
                      "if true; then")
    check(m1 != orig, "K1: паттерн проверки ключа найден")
    open(mut, "w", encoding="utf-8").write(m1)
    b, h, d_ = sandbox()
    try:
        run(b, h, d_, "onion")
        dd = first_backup(b)
        make_key(os.path.join(dd, "hidden_services", "smp",
                              "hs_ed25519_secret_key"), valid=False)
        r1 = run(b, h, d_, "verify", dd, script=mut)
        r2 = run(b, h, d_, "verify", dd)
        check(r1.returncode == 0 and r2.returncode != 0,
              f"K1: без проверки ключа verify пропускает битый бэкап "
              f"(мутация rc={r1.returncode}, оригинал rc={r2.returncode})")
    finally:
        shutil.rmtree(b, ignore_errors=True)

    # K2: убрать копирование ключей в страховку → откат невозможен.
    open(mut, "w", encoding="utf-8").write(orig)
    m2 = orig.replace(
        'if [ -d "$HS_DIR" ]; then cp -a "$HS_DIR" "$stash/hidden_services"; ok "текущие onion-ключи → $stash"; fi',
        ": # страховка отключена мутацией")
    check(m2 != orig, "K2: строка копирования в страховку найдена")
    open(mut, "w", encoding="utf-8").write(m2)
    b, h, d_ = sandbox()
    try:
        os.makedirs(os.path.join(b, "docker", "coturn"), exist_ok=True)
        run(b, h, d_, "save")
        dd = first_backup(b)
        subprocess.run(["bash", mut, "restore", dd], capture_output=True,
                       env=dict(os.environ, BACKUP_ROOT=os.path.join(b, "backups"),
                                HS_DIR=h, DATA_DIR=d_,
                                CONFIG_FILE=os.path.join(d_, "simplex-node.json"),
                                REPO=REPO, ASSUME_YES="1",
                                DOCKER_DIR=os.path.join(b, "docker")), timeout=180)
        bd = os.path.join(b, "backups")
        keys = [os.path.join(bd, x, "hidden_services", "smp", "hs_ed25519_secret_key")
                for x in os.listdir(bd) if x.startswith("pre-restore-")]
        check(not any(os.path.isfile(f) for f in keys),
              "K2: без копирования прежних ключей откат невозможен")
    finally:
        shutil.rmtree(b, ignore_errors=True)

    # K3: chmod 600 убрать → ключи станут 644.
    open(mut, "w", encoding="utf-8").write(orig)
    m3 = orig.replace('chmod 600 "$dst/$svc/hs_ed25519_secret_key"', ":")
    check(m3 != orig, "K3: строка chmod 600 найдена")
    open(mut, "w", encoding="utf-8").write(m3)
    b, h, d_ = sandbox()
    try:
        run(b, h, d_, "onion", script=mut)
        k = os.path.join(first_backup(b), "hidden_services", "smp",
                         "hs_ed25519_secret_key")
        mode = os.stat(k).st_mode & 0o777
        check(mode != 0o600, f"K3: без chmod ключ был бы {oct(mode)}")
    finally:
        shutil.rmtree(b, ignore_errors=True)

    # K4: вернуть неверный путь конфига → save перестанет его копировать.
    open(mut, "w", encoding="utf-8").write(orig)
    # Первая версия мутировала ЗНАЧЕНИЕ CONFIG_FILE — но run() всегда
    # передаёт CONFIG_FILE через ENV, поэтому подмена не влияла ни на что
    # и проверка падала на корректном коде. Мутировать нужно ТО, что
    # используется, когда переменной нет: здесь это путь к конфигу.
    m4 = orig.replace('if [ -f "$CONFIG_FILE" ]; then',
                      'if [ -f "$HOME/.config/simplex-node/simplex-node.json" ]; then')
    m4 = m4.replace('cp -a "$CONFIG_FILE" "$dest/config/"',
                    'cp -a "$HOME/.config/simplex-node/simplex-node.json" "$dest/config/"')
    check(m4 != orig, "K4: путь конфига в save найден")
    open(mut, "w", encoding="utf-8").write(m4)
    b, h, d_ = sandbox()
    try:
        run(b, h, d_, "save", script=mut)
        dd = first_backup(b)
        check(not os.path.isfile(os.path.join(dd, "config", "simplex-node.json")),
              "K4: со старым путём конфиг снова теряется — проверка это ловит")
    finally:
        shutil.rmtree(b, ignore_errors=True)
finally:
    shutil.rmtree(mut_dir, ignore_errors=True)

print("\n" + "=" * 64)
print(f"passed={N[0]} failed={F[0]}")
print("PASSED" if F[0] == 0 else "FAILED")
sys.exit(1 if F[0] else 0)
