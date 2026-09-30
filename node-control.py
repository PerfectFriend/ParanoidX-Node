#!/usr/bin/env python3
"""node-control — управление компонентами ноды: ON / OFF / TEST, стоп-всё,
выгрузка демона до перезагрузки, ротация xray.

Импортируется node-monitor.py. Каждый компонент умеет start/stop/restart/test
и сообщает своё состояние. Все действия логируются и уходят в Telegram.

Компоненты:
  daemon   — ParanoidX-демон (systemd system-scope: ParanoidX-dashboard.service)
  xray     — нативный xray, SOCKS5 :10810 (не systemd, процесс)
  docker   — 4 контейнера ParanoidX-*
  monitor  — сам node-monitor (user-unit node-monitor.service)
"""

import json
import os
import socket
import subprocess
import sys
import threading
import time

HOME = os.path.expanduser("~")
XRAY_DIR = os.path.join(HOME, "bin", "v2ray")
XRAY_BIN = os.path.join(XRAY_DIR, "xray")
XRAY_CFG = os.path.join(XRAY_DIR, "config.json")
XRAY_SERVERS = os.path.join(XRAY_DIR, "servers.json")
XRAY_LAST_GOOD = os.path.join(XRAY_DIR, "xray-last-good.json")
XRAY_STATE = os.path.join(XRAY_DIR, "failover_state.json")
SUB_SCRIPT = os.path.join(HOME, "simplex-node", "xray-sub.py")

DAEMON_UNIT = "ParanoidX-dashboard.service"
DOCKER_PREFIX = "ParanoidX-"
SOCKS_PORT = 10810
COMPOSE_DIR = os.path.join(HOME, "simplex-node", "docker")

# ── демон: почему нельзя просто systemctl stop ────────────────────────────
# ParanoidX-dashboard.service — system-scope, systemctl stop требует polkit,
# а у монитора нет пароля. Но есть два рабочих пути, и оба без root:
#   • /api/restart — демон шлёт себе SIGTERM, а Restart=always его поднимает.
#     Это единственный способ РЕСТАРТА без root.
#   • OFF — снять Restart=always, потом kill. Иначе systemd поднимет процесс
#     обратно за 5 секунд и кнопка OFF ничего не делает.
#
# ON/OFF меняют политику перезапуска в drop-in'е. Это обратимо: drop-in
# перезаписывается, а remove оставляет юнит в исходном состоянии.
DROPIN_DIR = f"/etc/systemd/system/{DAEMON_UNIT}.d"
DROPIN_FILE = os.path.join(DROPIN_DIR, "90-node-control.conf")

# Маркер «демон выгружен до перебута». /run/user/<uid> очищается при ребуте,
# поэтому демон вернётся сам — ровно как просил пользователь.
SUSPEND_MARKER = f"/run/user/{os.getuid()}/px-daemon-suspended"


def _run(cmd, timeout=60, cwd=None):
    """Запускает команду без shell, возвращает (rc, stdout+stderr)."""
    try:
        p = subprocess.run(
            cmd, capture_output=True, timeout=timeout, cwd=cwd, text=True
        )
        return p.returncode, (p.stdout or "") + (p.stderr or "")
    except subprocess.TimeoutExpired:
        return 124, f"timeout after {timeout}s"
    except Exception as e:
        return 1, str(e)


def _socks_alive(port=SOCKS_PORT, timeout=3):
    try:
        s = socket.create_connection(("127.0.0.1", port), timeout=timeout)
        s.close()
        return True
    except Exception:
        return False


def _systemctl(unit, action, user=False, timeout=12):
    """systemctl start/stop. ВАЖНО: различает отказ polkit и успех.

    ParanoidX-dashboard.service — system-scope, и systemctl stop без polkit
    НЕ возвращает ошибку: команда висит до таймаута и печатает
    «Method call timed out», а rc при этом может быть 0. Если это не
    отличать, монитор рапортует об успехе, когда демон всё ещё работает.
    """
    cmd = ["systemctl"]
    if user:
        cmd.append("--user")
    cmd += [action, unit]
    rc, out = _run(cmd, timeout=timeout)
    low = out.lower()
    if "timed out" in low or "interactive authentication" in low:
        return 1, (
            f"{unit}: systemctl {action} требует polkit/root "
            f"(system-scope юнит). Таймаут — действие НЕ выполнено."
        )
    if rc != 0:
        return rc, out.strip()[:200] or f"systemctl {action} rc={rc}"
    return 0, out.strip()


# ── компоненты ─────────────────────────────────────────────────────────────

class Component:
    """Базовый компонент. start/stop/restart/test переопределяются."""

    name = "base"
    label = "base"
    kind = "generic"

    def is_running(self):
        return False

    def start(self):
        return False, "not implemented"

    def stop(self):
        return False, "not implemented"

    def restart(self):
        ok1, m1 = self.stop()
        time.sleep(0.8)
        ok2, m2 = self.start()
        return (ok1 or ok2), f"stop: {m1} | start: {m2}"

    def test(self):
        running = self.is_running()
        return running, ("running" if running else "not running")

    def status_text(self):
        return "running" if self.is_running() else "stopped"


PKEXEC_CTL = "/usr/local/bin/px-node-control"


def _pxctl(action, timeout=25):
    """Управление демоном через polkit — без пароля root.

    Правило /etc/polkit-1/rules.d/10-px-node-control.rules разрешает
    org.freedesktop.policykit.exec только для этой программы, активной
    сессии пользователя tomas. Wrapper внутри принимает только
    ParanoidX-dashboard.service и только start|stop|restart|status.
    """
    if not os.path.isfile(PKEXEC_CTL):
        return 1, (
            f"{PKEXEC_CTL} не установлен — нужен polkit-правило "
            f"(см. /etc/polkit-1/rules.d/10-px-node-control.rules)"
        )
    return _run(["pkexec", PKEXEC_CTL, action], timeout=timeout)


class DaemonComponent(Component):
    name = "daemon"
    label = "Node daemon"
    kind = "systemd"

    def is_running(self):
        rc, out = _run(["systemctl", "is-active", DAEMON_UNIT], timeout=15)
        return out.strip() == "active"

    def is_suspended(self):
        return os.path.exists(SUSPEND_MARKER)

    def _settle(self, want_running, timeout=25.0):
        """Ждёт фактического состояния. Код возврата systemctl НЕ доверяем.

        Почему: TimeoutStopUSec у юнита 90 с, поэтому systemctl stop
        упирается в наш таймаут, процесс не досылает код возврата, а его
        kill падает с EPERM (потомок уже root). Действие при этом
        ВЫПОЛНЕНО. Критерий успеха — состояние, а не rc.
        """
        deadline = time.time() + timeout
        while time.time() < deadline:
            if self.is_running() == want_running:
                return True
            time.sleep(0.5)
        return self.is_running() == want_running

    def start(self):
        if self.is_suspended():
            # Ручной старт снимает выгрузку: пользователь попросил вернуть ноду.
            try:
                os.unlink(SUSPEND_MARKER)
            except OSError:
                pass
        rc, out = _pxctl("start", timeout=100)
        if self._settle(True):
            return True, f"{DAEMON_UNIT} запущен"
        return False, out.strip()[:250] or "systemd не подтвердил запуск"

    def stop(self):
        rc, out = _pxctl("stop", timeout=100)
        if self._settle(False):
            return True, f"{DAEMON_UNIT} остановлен"
        return False, out.strip()[:250] or f"{DAEMON_UNIT} всё ещё активен"

    def suspend_until_reboot(self):
        """Выгружает демона до следующей перезагрузки хоста.

        Не disable: /run очищается при ребуте, поэтому демон вернётся сам.
        Сначала останавливаем, и только при успехе ставим маркер — иначе
        получим «выгружено», а демон продолжает работать.
        """
        ok, msg = self.stop()
        if not ok:
            return False, (
                f"не выгружен: {msg} "
                f"(нужен polkit/root для {DAEMON_UNIT})"
            )
        try:
            os.makedirs(os.path.dirname(SUSPEND_MARKER), exist_ok=True)
            with open(SUSPEND_MARKER, "w") as f:
                f.write(f"suspended at {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
        except OSError as e:
            return False, f"демон остановлен, но маркер не создан: {e}"
        return True, f"выгружен до ребута ({msg})"

    def test(self):
        if not self.is_running():
            if self.is_suspended():
                return False, "suspended until reboot"
            return False, f"{DAEMON_UNIT} не активен"
        # Реальная проверка: отвечает ли API.
        try:
            import urllib.request
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            r = opener.open("http://127.0.0.1:8080/api/health", timeout=6)
            return r.status == 200, f"API /api/health → {r.status}"
        except Exception as e:
            return False, f"API не отвечает: {type(e).__name__}"

    def status_text(self):
        # Маркер сам по себе НЕ означает «выгружен»: он мог остаться от
        # неудачной попытки, когда демон на самом деле работает. Поэтому
        # «suspended» показываем только если демон действительно не запущен.
        if self.is_suspended() and not self.is_running():
            return "suspended"
        return "running" if self.is_running() else "stopped"


class XrayComponent(Component):
    name = "xray"
    label = "xray proxy"
    kind = "process"

    def is_running(self):
        rc, out = _run(["pgrep", "-x", "xray"], timeout=10)
        return rc == 0

    def _spawn(self):
        logp = os.path.join(HOME, ".local", "share", "ParanoidX.logs", "xray.log")
        os.makedirs(os.path.dirname(logp), exist_ok=True)
        with open(logp, "ab") as lf:
            subprocess.Popen(
                [XRAY_BIN, "run", "-c", XRAY_CFG],
                stdout=lf, stderr=lf, start_new_session=True,
            )

    def start(self):
        if not (os.path.isfile(XRAY_BIN) and os.path.isfile(XRAY_CFG)):
            return False, "нет бинаря или конфига"
        if self.is_running() and _socks_alive():
            return True, "уже работает"
        self.stop()
        self._spawn()
        for _ in range(20):
            time.sleep(0.3)
            if _socks_alive():
                return True, f"SOCKS5 :{SOCKS_PORT} поднят"
        return False, "xray не поднял SOCKS за 6 с"

    def stop(self):
        _run(["pkill", "-x", "xray"], timeout=15)
        time.sleep(0.8)
        return not self.is_running(), "xray остановлен"

    def test(self):
        if not self.is_running():
            return False, "процесс не запущен"
        alive = _socks_alive()
        if not alive:
            return False, f"SOCKS5 :{SOCKS_PORT} не отвечает"
        # Реальный запрос через прокси — единственная честная проверка.
        rc, out = _run([
            "curl", "-sS", "--socks5-hostname", f"127.0.0.1:{SOCKS_PORT}",
            "--max-time", "20", "-o", "/dev/null",
            "-w", "%{http_code} %{speed_download}",
            "https://speed.cloudflare.com/__down?bytes=500000",
        ], timeout=30)
        parts = out.split()
        if rc == 0 and parts and parts[0] == "200":
            return True, f"прокси работает, {int(parts[1])/1e6:.2f} МБ/с"
        return False, f"прокси не даёт трафик (rc={rc}) {out.strip()[:80]}"

    def status_text(self):
        if not self.is_running():
            return "stopped"
        return "running" if _socks_alive() else "no socks"


class DockerComponent(Component):
    name = "docker"
    label = "Docker stack"
    kind = "docker"

    def containers(self, running_only=True):
        """Имена контейнеров ParanoidX.

        running_only=True (по умолчанию) берёт только ЗАПУЩЕННЫЕ. Раньше
        здесь стоял `docker ps -a`, и остановленный контейнер считался
        работающим: is_running() возвращал True при четырёх мёртвых
        контейнерах, а cli state писал «docker=4 up». Список для docker
        start по-прежнему нужен полный — за это отвечает all_containers().
        """
        cmd = ["docker", "ps"] + ([] if running_only else ["-a"]) + \
              ["--format", "{{.Names}}"]
        rc, out = _run(cmd, timeout=20)
        if rc != 0:
            return []
        return [l.strip() for l in out.splitlines() if l.strip().startswith(DOCKER_PREFIX)]

    def all_containers(self):
        return self.containers(running_only=False)

    def is_running(self):
        return bool(self.containers())

    def start(self):
        # Список для старта — ВСЕ контейнеры, включая остановленные.
        # С running-only списком остановленный стек было бы нечего
        # запускать: start возвращал бы «нет контейнеров ParanoidX»
        # именно тогда, когда он нужен.
        names = self.all_containers()
        if not names:
            return False, "нет контейнеров ParanoidX (нужен docker compose up -d)"
        up_before = set(self.containers())
        todo = [n for n in names if n not in up_before]
        if not todo:
            return True, f"уже работает ({len(names)}/{len(names)})"
        rc, out = _run(["docker", "start"] + todo, timeout=180)
        if rc != 0:
            return False, f"docker start не удался: {out.strip()[:80]}"
        time.sleep(3.0)
        up = set(self.containers())
        still = [n for n in todo if n not in up]
        if still:
            return False, (f"запустилось {len(todo) - len(still)}/{len(todo)}; "
                           f"не поднялись: {', '.join(still)}")
        return True, f"запущено {len(todo)}/{len(todo)}"

    def stop(self):
        names = self.containers()          # останавливать нужно только живые
        if not names:
            return True, "нечего останавливать"
        rc, out = _run(["docker", "stop", "-t", "10"] + names, timeout=180)
        time.sleep(1.5)
        left = self.containers()
        if left:
            return False, f"не остановились: {', '.join(left)}"
        return True, f"остановлено {len(names)}/{len(names)}"

    def test(self):
        names = self.containers()
        if not names:
            return False, "нет контейнеров"
        rc, out = _run(
            ["docker", "ps", "--format", "{{.Names}}\t{{.Status}}"], timeout=20
        )
        bad = []
        for line in out.splitlines():
            if not line.strip().startswith(DOCKER_PREFIX):
                continue
            name, _, status = line.partition("\t")
            if "healthy" not in status and "Up" not in status:
                bad.append(f"{name}: {status.strip()}")
        if bad:
            return False, "нездоровы: " + ", ".join(bad[:3])
        return True, f"все {len(names)} контейнеров здоровы"

    def status_text(self):
        n = len(self.containers())
        return f"{n} up" if n else "stopped"


class MonitorComponent(Component):
    name = "monitor"
    label = "node-monitor"
    kind = "systemd-user"

    def is_running(self):
        rc, out = _run(["systemctl", "--user", "is-active", "node-monitor.service"], timeout=15)
        return out.strip() == "active"

    def start(self):
        rc, out = _systemctl("node-monitor.service", "start", user=True)
        time.sleep(1.5)
        return self.is_running(), "node-monitor запущен" if self.is_running() else out.strip()[:150]

    def stop(self):
        rc, out = _systemctl("node-monitor.service", "stop", user=True)
        return not self.is_running(), "node-monitor остановлен"

    def test(self):
        if not self.is_running():
            return False, "юнит не активен"
        rc, out = _run(
            ["systemctl", "--user", "show", "node-monitor.service",
             "-p", "MainPID", "-p", "NRestarts", "-p", "ActiveState"],
            timeout=15,
        )
        if rc != 0:
            return False, "не удалось прочитать состояние юнита"
        return "active" in out, out.strip().replace("\n", " ")

    def status_text(self):
        return "running" if self.is_running() else "stopped"


# ── реестр и операции ──────────────────────────────────────────────────────

class NodeControl:
    def __init__(self, on_event=None):
        self.on_event = on_event or (lambda *a: None)
        self.components = {
            c.name: c
            for c in (DaemonComponent(), XrayComponent(), DockerComponent(), MonitorComponent())
        }
        self._op_lock = threading.Lock()
        self._busy = set()

    def emit(self, kind, name, action, ok, msg):
        try:
            self.on_event(kind, name, action, ok, msg)
        except Exception:
            pass

    def snapshot(self):
        out = {}
        for name, c in self.components.items():
            try:
                out[name] = c.status_text()
            except Exception as e:
                out[name] = f"error: {e}"
        return out

    def is_busy(self, name):
        with self._op_lock:
            return name in self._busy

    def _guard(self, name):
        with self._op_lock:
            if name in self._busy:
                return False
            self._busy.add(name)
        return True

    def _release(self, name):
        with self._op_lock:
            self._busy.discard(name)

    def act(self, name, action, background=True):
        """Выполняет start/stop/restart/test у компонента."""
        c = self.components.get(name)
        if not c:
            self.emit("error", name, action, False, "нет такого компонента")
            return
        if not self._guard(name):
            self.emit("busy", name, action, False, "операция уже выполняется")
            return

        def run():
            self.emit("start", name, action, True, "выполняется…")
            try:
                fn = getattr(c, action, None)
                if not fn:
                    ok, msg = False, f"действие {action} не поддерживается"
                else:
                    ok, msg = fn()
            except Exception as e:
                ok, msg = False, f"{type(e).__name__}: {e}"
            finally:
                self._release(name)
            self.emit("done" if ok else "error", name, action, ok, msg)
            return ok, msg

        if background:
            threading.Thread(target=run, daemon=True).start()
        else:
            return run()

    # ── стоп-всё ─────────────────────────────────────────────────────────
    def stop_all(self, include_docker=True, include_monitor=False, background=True):
        """Останавливает всё и освобождает память. Возвращает отчёт."""
        if not self._guard("__all__"):
            self.emit("busy", "all", "stop-all", False, "уже выполняется")
            return

        def run():
            def mem_mb():
                try:
                    with open("/proc/meminfo") as f:
                        for line in f:
                            if line.startswith("MemAvailable:"):
                                return int(line.split()[1]) // 1024
                except Exception:
                    return None
                return None

            before = mem_mb()
            report = [f"Доступно памяти до: {before} МБ" if before else "Доступно памяти до: ?"]
            self.emit("start", "all", "stop-all", True, "останавливаю всё…")

            order = ["daemon", "xray"]
            if include_docker:
                order.append("docker")
            for name in order:
                c = self.components[name]
                try:
                    ok, msg = c.stop()
                except Exception as e:
                    ok, msg = False, str(e)
                report.append(f"{c.label}: {'✓' if ok else '✗'} {msg}")
                self.emit("progress", name, "stop-all", ok, msg)

            # Освобождение памяти: prune и сброс кэша.
            rc, out = _run(
                ["docker", "system", "prune", "-f",
                 "--filter", "until=24h"],
                timeout=300,
            )
            if rc == 0:
                report.append("docker prune: выполнен")
            else:
                report.append(f"docker prune: {out.strip()[:80]}")
            _run(["sync"], timeout=30)
            _run(["sudo", "-n", "sh", "-c", "echo 3 > /proc/sys/vm/drop_caches"],
                 timeout=30)
            time.sleep(2)
            after = mem_mb()
            freed = (after - before) if (before is not None and after is not None) else None
            report.append(
                f"Доступно памяти после: {after} МБ"
                + (f" (освобождено {freed} МБ)" if freed is not None else "")
            )
            self._release("__all__")
            self.emit("done", "all", "stop-all", True, "\n".join(report))

        if background:
            threading.Thread(target=run, daemon=True).start()
        else:
            return run()

    # ── ротация xray ─────────────────────────────────────────────────────
    def xray_rotate(self, background=True):
        """Переключает xray на следующий сервер из топ-100."""
        if not self._guard("__rotate__"):
            self.emit("busy", "xray", "rotate", False, "ротация уже идёт")
            return

        def run():
            try:
                with open(XRAY_SERVERS) as f:
                    servers = json.load(f)
            except Exception as e:
                self._release("__rotate__")
                self.emit("error", "xray", "rotate", False, f"нет списка серверов: {e}")
                return
            if len(servers) < 2:
                self._release("__rotate__")
                self.emit("error", "xray", "rotate", False, "в списке меньше двух серверов")
                return

            try:
                with open(XRAY_STATE) as f:
                    st = json.load(f)
            except Exception:
                st = {"index": 0}
            idx = int(st.get("index", 0)) + 1
            if idx >= len(servers):
                idx = 0
            target = servers[idx]

            self.emit("start", "xray", "rotate", True,
                      f"переключаюсь на {target['host']}:{target['port']}")
            ok, msg = self._xray_switch_to(target)
            try:
                with open(XRAY_STATE, "w") as f:
                    json.dump({"index": idx, "switched": time.time()}, f)
            except OSError:
                pass
            self._release("__rotate__")
            self.emit("done" if ok else "error", "xray", "rotate", ok,
                      f"{msg} → {target['host']}:{target['port']} "
                      f"({target.get('rtt_ms', '?')} мс)")

        if background:
            threading.Thread(target=run, daemon=True).start()
        else:
            return run()

    def _xray_switch_to(self, target):
        """Собирает конфиг с одним outbound = target, валидирует, применяет."""
        import importlib.util
        try:
            spec = importlib.util.spec_from_file_location("xsub", SUB_SCRIPT)
            xs = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(xs)
        except Exception as e:
            return False, f"не удалось загрузить xray-sub: {e}"

        try:
            ob = xs.outbound_for(target)
            if not ob:
                return False, "неизвестная схема"
            ob["tag"] = "px-failover"
            cfg = {
                "log": {"loglevel": "warning"},
                "inbounds": [xs.INBOUND],
                "outbounds": [ob],
            }
            ok, err = xs.validate(cfg)
            if not ok:
                return False, f"xray отверг конфиг: {err.strip()[-120:]}"
            xs.backup_config("failover")
            xs.write_config(cfg)
            alive = xs.reload_xray()
            if alive:
                xs.save_last_good()
                return True, "конфиг переключён, SOCKS поднят"
            # откат
            return False, "SOCKS не поднялся после переключения"
        except Exception as e:
            return False, f"{type(e).__name__}: {e}"

    # ── протокол подписки ────────────────────────────────────────────────
    def sub_update(self, apply_new=True, background=True):
        """Запускает протокол обновления конфигов по подписке."""
        if not self._guard("__sub__"):
            self.emit("busy", "xray", "sub-update", False, "протокол уже идёт")
            return

        def run():
            self.emit("start", "xray", "sub-update", True, "запускаю протокол подписки…")
            cmd = ["python3", SUB_SCRIPT, "update"]
            if not apply_new:
                cmd.append("--dry")
            rc, out = _run(cmd, timeout=900)
            try:
                st = json.loads(out[out.index("{"):]) if "{" in out else {}
            except Exception:
                st = {}
            summary = (
                f"кандидатов {st.get('total_candidates', '?')}, "
                f"проверено {st.get('tested', '?')}, "
                f"мёртвых {st.get('dead', '?')}, "
                f"годных {st.get('speed_ok', '?')}, "
                f"в топе {st.get('kept', '?')}"
            )
            ok = rc == 0 and st.get("ok")
            self._release("__sub__")
            self.emit("done" if ok else "error", "xray", "sub-update", ok,
                      st.get("message", out.strip()[-150:]) + f" | {summary}")

        if background:
            threading.Thread(target=run, daemon=True).start()
        else:
            return run()


# Одиночный экземпляр для импорта из монитора.
_CONTROL = None


def get_control(on_event=None):
    global _CONTROL
    if _CONTROL is None:
        _CONTROL = NodeControl(on_event)
    return _CONTROL


# ── CLI: точка входа для HTTP-моста (cmd/ParanoidX/control_api.go) ────────

def _cli_print(ok, msg):
    """Печатает результат одной строкой. Код возврата — для вызывающего."""
    print(("OK " if ok else "FAIL ") + msg, flush=True)
    return 0 if ok else 1


def cli(argv):
    """CLI node-control. Используется HTTP-мостом, не интерактивно.

    Формат:
      node-control.py cli <компонент> <start|stop|restart|test>
      node-control.py cli --global <stop-all|rotate|suspend|subscription>
      node-control.py cli state
    """
    args = list(argv)
    if not args:
        return _cli_print(False, "usage: cli <component> <action> | --global <action> | state")
    control = get_control()

    if args[0] == "state":
        snap = control.snapshot()
        line = " ".join(f"{k}={v}" for k, v in sorted(snap.items()))
        print(line, flush=True)
        return 0

    if args[0] == "--global":
        if len(args) < 2:
            return _cli_print(False, "missing global action")
        act = args[1]
        if act == "stop-all":
            ok, msg = control.stop_all(background=False)
            return _cli_print(ok, msg.replace("\n", " | "))
        if act == "rotate":
            ok, msg = control.xray_rotate(background=False)
            return _cli_print(ok, msg)
        if act == "subscription":
            ok, msg = control.sub_update(apply_new=True, background=False)
            return _cli_print(ok, msg)
        if act == "suspend":
            c = control.components.get("daemon")
            if c is None:
                return _cli_print(False, "нет демона")
            ok, msg = c.suspend_until_reboot()
            return _cli_print(ok, msg)
        return _cli_print(False, f"unknown global action: {act}")

    component = args[0]
    action = args[1] if len(args) > 1 else ""
    if component not in control.components:
        return _cli_print(False, f"unknown component: {component}")
    if action not in ("start", "stop", "restart", "test"):
        return _cli_print(False, f"unknown action: {action}")
    ok, msg = control.act(component, action, background=False)
    return _cli_print(ok, msg)


if __name__ == "__main__":
    try:
        # Первый аргумент — уже имя команды, поэтому отбрасываем его,
        # иначе cli() увидит "cli" как имя компонента и всё отвергнет.
        sys.exit(cli(sys.argv[2:] if len(sys.argv) > 1 and sys.argv[1] == "cli"
                     else sys.argv[1:]))
    except Exception as exc:  # CLI обязан не падать трейсбеком в HTTP-мост
        print(f"FAIL {type(exc).__name__}: {exc}", flush=True)
        sys.exit(1)
