#!/usr/bin/env python3
"""xray-sub — подписка, тестирование, топ-100 и генерация конфига xray.

Протокол (запускается при рестарте ноды и раз в 12 ч):
  1. скачать подписки (список источников в SUB_URLS)
  2. распарсить vless/vmess/ss/trojan (hy2/tuic xray не понимает — отсеиваем)
  3. добавить ТЕКУЩИЙ конфиг как полноправного участника рейтинга
  4. протестировать: TCP-connect, замерить RTT
  5. мёртвые удалить, отсортировать быстрые → медленные, оставить топ-100
  6. сгенерировать config.json, забэкапив текущий, и применить

Gate: если ни один сервер не прошёл порог — остаётся прежний конфиг,
запись в state помечается failed, монитор получит это в статусе.
Никогда не оставляем ноду без рабочего прокси.

Команды:
  xray-sub.py update   полный протокол (по умолчанию)
  xray-sub.py test     только прогон рейтинга по текущему списку
  xray-sub.py status   показать состояние (json для монитора)
  xray-sub.py servers  показать топ-N
"""

import base64
import json
import os
import socket
import ssl
import subprocess
import sys
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor

HOME = os.path.expanduser("~")
XRAY_DIR = os.path.join(HOME, "bin", "v2ray")
XRAY_BIN = os.path.join(XRAY_DIR, "xray")
CONFIG = os.path.join(XRAY_DIR, "config.json")
SERVERS = os.path.join(XRAY_DIR, "servers.json")
STATE = os.path.join(XRAY_DIR, "sub_state.json")
LAST_GOOD = os.path.join(XRAY_DIR, "xray-last-good.json")
BACKUP_DIR = os.path.join(HOME, "A1-backups", "xray-config")
SOCKS_PORT = 10810

# Публичные бесплатные подписки v2rayNG (проверены: HTTP 200, разбор вручную).
SUB_URLS = [
    "https://raw.githubusercontent.com/Epodonios/v2ray-configs/main/All_Configs_Sub.txt",
    "https://raw.githubusercontent.com/mahdibland/V2RayAggregator/master/Eternity.txt",
    "https://raw.githubusercontent.com/Pawdroid/Free-servers/main/sub",
]

TOP_N = 100              # сколько лучших оставляем
TEST_TIMEOUT = 3.0       # сек на TCP-connect
GOOD_RTT_MS = 1200       # порог «годного» сервера
# Параллельных TCP-connect. Подписки дают 3 500+ узлов, и по замыслу
# протокола ВСЕ должны быть проверены — список чистится ПО РЕЗУЛЬТАТУ
# замера, а не отсекается заранее. 64 потока на этой машине (3.2 ГБ RAM)
# держат 3 500 проб примерно за 25–40 с.
PROBE_WORKERS = 64
# Сколько кандидатов максимум. 0 = без ограничения (по умолчанию).
MAX_TEST = 0

# Порт для замера скорости. Основной бридж на :10810 — не трогаем, нода
# продолжает работать на старом проверенном конфиге, пока идёт отбор.
PROBE_PORT = 19999
# Параллельных замеров скорости: каждый поднимает свой xray на своём порту.
SPEED_WORKERS = 6


def log(msg):
    print(f"[xray-sub] {msg}", flush=True)


# ── загрузка ────────────────────────────────────────────────────────────────

def fetch(url, timeout=30):
    """Скачивание через curl, а не urllib.

    Почему curl: в окружении прописан socks5://127.0.0.1:9050 (Tor), а urllib
    не умеет SOCKS-прокси — он шлёт запрос в Tor как в HTTP-прокси и получает
    «501 Tor is not an HTTP Proxy». curl понимает socks5h и ходит через Tor.
    """
    try:
        p = subprocess.run(
            ["curl", "-sS", "-L", "--max-time", str(timeout),
             "-A", "clash.meta", url],
            capture_output=True, timeout=timeout + 10,
        )
        if p.returncode == 0 and p.stdout:
            return p.stdout.decode("utf-8", "replace")
        log(f"curl вернул {p.returncode}: {p.stderr.decode('utf-8', 'replace').strip()[:120]}")
        return ""
    except Exception as e:
        log(f"curl не сработал: {e}")
        return ""


def decode_sub(text):
    """Подписка может быть base64-обёрткой или plain-текстом."""
    lines = [l.strip() for l in text.splitlines() if l.strip()]
    joined = "".join(lines)
    looks_plain = any(
        l.startswith(("vmess://", "vless://", "ss://", "trojan://")) for l in lines
    )
    if looks_plain:
        return lines
    try:
        raw = base64.b64decode(joined + "=" * (-len(joined) % 4))
        return [l.strip() for l in raw.decode("utf-8", "replace").splitlines() if l.strip()]
    except Exception:
        return lines


# ── парсинг ────────────────────────────────────────────────────────────────
# Внутренний вид: {"scheme","host","port","name","raw","params"…}

def _split_hostport(netloc, default_port=443):
    netloc = netloc.rsplit("@", 1)[-1]  # убрать userinfo
    if netloc.startswith("["):  # IPv6
        host, _, rest = netloc.partition("]")
        return host.lstrip("["), int(rest.lstrip(":") or default_port)
    if ":" in netloc:
        host, _, port = netloc.rpartition(":")
        if port.isdigit():
            return host, int(port)
    return netloc, default_port


def parse_vmess(line):
    b = line[8:]
    b += "=" * (-len(b) % 4)
    try:
        d = json.loads(base64.b64decode(b))
    except Exception:
        return None
    host, port = d.get("add"), d.get("port")
    if not host:
        return None
    try:
        port = int(port)
    except (TypeError, ValueError):
        return None
    return {
        "scheme": "vmess",
        "host": host,
        "port": port,
        "name": d.get("ps") or d.get("remark") or f"{host}:{port}",
        "uuid": d.get("id"),
        "aid": int(d.get("aid") or 0),
        "scy": d.get("scy") or "auto",
        "net": d.get("net") or "tcp",
        "tls": d.get("tls") or "",
        "host_hdr": d.get("host") or "",
        "path": d.get("path") or "",
        "sni": d.get("sni") or d.get("host") or "",
        "raw": line,
    }


def parse_vless(line):
    try:
        body = line[8:]
        frag = ""
        if "#" in body:
            body, frag = body.split("#", 1)
        query = ""
        if "?" in body:
            body, query = body.split("?", 1)
        uuid, _, netloc = body.partition("@")
        if not netloc:
            return None
        host, port = _split_hostport(netloc)
        p = dict(urllib.parse.parse_qsl(query))
        return {
            "scheme": "vless",
            "host": host,
            "port": port,
            "name": urllib.parse.unquote(frag) or f"{host}:{port}",
            "uuid": uuid,
            "flow": p.get("flow") or "",
            "encryption": p.get("encryption") or "none",
            "security": p.get("security") or "none",
            "net": p.get("type") or "tcp",
            "sni": p.get("sni") or p.get("host") or "",
            "host_hdr": p.get("host") or "",
            "path": p.get("path") or "",
            "pbk": p.get("pbk") or "",
            "sid": p.get("sid") or "",
            "fp": p.get("fp") or "chrome",
            "raw": line,
        }
    except Exception:
        return None


def parse_trojan(line):
    try:
        body = line[9:]
        frag = ""
        if "#" in body:
            body, frag = body.split("#", 1)
        query = ""
        if "?" in body:
            body, query = body.split("?", 1)
        password, _, netloc = body.partition("@")
        if not netloc:
            return None
        host, port = _split_hostport(netloc)
        p = dict(urllib.parse.parse_qsl(query))
        return {
            "scheme": "trojan",
            "host": host,
            "port": port,
            "name": urllib.parse.unquote(frag) or f"{host}:{port}",
            "password": password,
            "security": p.get("security") or "tls",
            "net": p.get("type") or "tcp",
            "sni": p.get("sni") or p.get("host") or "",
            "host_hdr": p.get("host") or "",
            "path": p.get("path") or "",
            "raw": line,
        }
    except Exception:
        return None


def parse_ss(line):
    """ss:// base64(whole) | base64(method:pass)@host:port | method:pass@host:port"""
    try:
        body = line[5:]
        frag = ""
        if "#" in body:
            body, frag = body.split("#", 1)
        if "@" not in body:
            b = body + "=" * (-len(body) % 4)
            body = base64.b64decode(b).decode("utf-8", "replace")
        else:
            userinfo, _, hostport = body.rpartition("@")
            if ":" not in userinfo:
                userinfo = base64.b64decode(
                    userinfo + "=" * (-len(userinfo) % 4)
                ).decode("utf-8", "replace")
            body = f"{userinfo}@{hostport}"
        cred, _, netloc = body.rpartition("@")
        method, _, password = cred.partition(":")
        host, port = _split_hostport(netloc)
        if not method or not host:
            return None
        return {
            "scheme": "ss",
            "host": host,
            "port": port,
            "name": urllib.parse.unquote(frag) or f"{host}:{port}",
            "method": method,
            "password": urllib.parse.unquote(password),
            "raw": line,
        }
    except Exception:
        return None


PARSERS = {
    "vmess://": parse_vmess,
    "vless://": parse_vless,
    "trojan://": parse_trojan,
    "ss://": parse_ss,
}


def parse_line(line):
    for prefix, fn in PARSERS.items():
        if line.startswith(prefix):
            return fn(line)
    return None  # hysteria2/hy2/tuic/socks — xray не понимает


# ── текущий конфиг как участник рейтинга ───────────────────────────────────

def read_current_outbound():
    """Достаёт outbound из текущего config.json как кандидата рейтинга."""
    try:
        with open(CONFIG, encoding="utf-8") as f:
            cfg = json.load(f)
    except Exception as e:
        log(f"не читается текущий конфиг: {e}")
        return None
    for ob in cfg.get("outbounds", []):
        if ob.get("protocol") in ("freedom", "blackhole"):
            continue
        return {"config": ob, "label": f"current:{ob.get('protocol', '?')}"}
    return None


def current_candidate():
    """Кандидат из ручного конфига (обычно Reality на 127.0.0.1:10813).

    ВАЖНО: 127.0.0.1 — это локальный прокси, а не удалённый сервер. Он
    участвует в рейтинге на равных, но его «адрес» для TCP-теста — тот
    локальный порт; проверяется связность цепочки, а не пинг до удалённого
    хоста. Если локальный порт закрыт — кандидат мёртв и выпадает.
    """
    ob = read_current_outbound()
    if not ob:
        return None
    cfg = ob["config"]
    vnext = (cfg.get("settings") or {}).get("vnext") or []
    servers = (cfg.get("settings") or {}).get("servers") or []
    entry = None
    if vnext:
        entry = vnext[0]
    elif servers:
        entry = servers[0]
    if not entry:
        return None
    host = entry.get("address")
    port = int(entry.get("port") or 0)
    if not host or not port:
        return None
    users = entry.get("users") or []
    user = users[0] if users else {}
    ss = cfg.get("streamSettings") or {}
    reality = ss.get("realitySettings") or {}
    cand = {
        "scheme": "current",
        "host": host,
        "port": port,
        "name": f"CURRENT (last-known-good) {host}:{port}",
        "is_current": True,
        "net": ss.get("network") or "tcp",
        "security": ss.get("security") or "none",
        "sni": reality.get("serverName") or "",
        "fingerprint": reality.get("fingerprint") or "chrome",
        "publicKey": reality.get("publicKey") or "",
        "shortId": reality.get("shortId") or "",
        "uuid": user.get("id") or "",
        "flow": user.get("flow") or "",
        "encryption": user.get("encryption") or "none",
        "raw": "",
    }
    return cand


# ── тестирование ───────────────────────────────────────────────────────────

def is_public_host(host):
    """True только для настоящего публичного адреса.

    Отсекает заглушки ("127.0.0.1", "0.0.0.0"), локальную сеть и служебные
    адреса. Заглушки опасны тем, что connect к ним мгновенен и они
    побеждают реальные серверы по RTT — см. комментарий в probe().
    """
    h = (host or "").strip().strip("[]").lower()
    if not h:
        return False
    # Не IP-адрес — пропускаем (доменное имя проверить нечем на этом этапе).
    try:
        import ipaddress
        ip = ipaddress.ip_address(h)
    except ValueError:
        return True
    if ip.is_loopback or ip.is_unspecified or ip.is_link_local:
        return False
    if ip.is_private or ip.is_reserved or ip.is_multicast:
        return False
    # 100.64.0.0/10 — CGNAT, в подписках не встречается как прокси.
    if str(ip).startswith("100."):
        return False
    return True


def probe(cand, timeout=TEST_TIMEOUT):
    """TCP-connect НАПРЯМУЮ, мимо бриджа. Возвращает (alive, rtt_ms).

    Почему мимо бриджа: socket.create_connection не использует SOCKS из env
    и не перехватывается ядром (правил NAT/REDIRECT в системе нет) — нода
    сама dial'ит socks5://127.0.0.1:10810. Проверено замером: прямой connect
    49.9 мс против 2.2 мс через SOCKS (последнее — локальный хэндшейк).
    Значит замер отражает реальный путь до сервера, а не путь через прокси,
    который мы как раз и оцениваем.
    """
    host, port = cand["host"], int(cand["port"])
    if port <= 0 or port > 65535:
        return False, float("inf")
    # Локальные адреса НЕ являются кандидатами. Строка подписки вида
    # "127.0.0.1:8080" или "0.0.0.0:8080" — это заглушка/сбой парсера, а не
    # прокси-сервер. Но TCP-connect к ним мгновенен (0.8 мс), поэтому они
    # побеждали ВСЕ реальные серверы по RTT и попадали в топ-1. Дальше
    # build_config превращал 127.0.0.1:8080 в vless-outbound с UUID-заглушкой,
    # и xray отвергал весь конфиг: "failed to build outbound config with tag
    # px-000". Шлюз не проходил, старая нода продолжала работать — то есть
    # проявлялось как «подписка не применилась» без внятной причины.
    if not is_public_host(host):
        return False, float("inf")
    t0 = time.perf_counter()
    try:
        family = socket.AF_INET6 if ":" in host else socket.AF_INET
        sock = socket.socket(family, socket.SOCK_STREAM)
        sock.settimeout(timeout)
        sock.connect((host, port))
        sock.close()
        return True, (time.perf_counter() - t0) * 1000.0
    except Exception:
        return False, float("inf")


def throughput(cand, port=PROBE_PORT, bytes_wanted=3_000_000, timeout=22.0):
    """Реальная скорость: поднимаем xray с этим outbound и качаем через него.

    ПОЧЕМУ ТАК: узел из подписки — это ПРОКСИ, а не файловый сервер.
    Измерено на живых данных:
      • на сыром сокете сервер отвечает 414-байтовой страницей ошибки и
        закрывает соединение → скорость 0 для ЛЮБОГО узла;
      • RST на «живом» по TCP-connect узле (104.16.72.70, 8.47.69.0
        рабочие, 151.101.56.6 и 140.248.185.253 — нет).
    Значит пинг НЕ отличает рабочий прокси от мёртвого: успешный
    connect бывает и у негодных. Реальную скорость даёт только прогон
    трафика через сам прокси.

    Требование «мимо бриджа» соблюдено: поднимается ОТДЕЛЬНЫЙ xray на
    порту PROBE_PORT, основной бридж на :10810 не трогается — нода всё
    это время работает на старом проверенном конфиге.
    """
    ob = outbound_for(cand)
    if not ob:
        return 0.0, float("inf")
    ob["tag"] = "probe"
    cfg = {
        "log": {"loglevel": "error"},
        "inbounds": [{
            "port": port, "protocol": "socks",
            "settings": {"auth": "noaccess", "udp": False},
        }],
        "outbounds": [ob],
    }
    import tempfile
    tmp = None
    proc = None
    try:
        fd, tmp = tempfile.mkstemp(suffix=".json", prefix="xray-probe-")
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(cfg, f)
        proc = subprocess.Popen(
            [XRAY_BIN, "run", "-c", tmp],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        # ждём, пока SOCKS поднимется
        for _ in range(20):
            time.sleep(0.25)
            try:
                s = socket.create_connection(("127.0.0.1", port), timeout=1)
                s.close()
                break
            except Exception:
                if proc.poll() is not None:
                    return 0.0, float("inf")

        url = f"https://speed.cloudflare.com/__down?bytes={bytes_wanted}"
        p = subprocess.run(
            ["curl", "-sS", "--socks5-hostname", "127.0.0.1:%d" % port,
             "--max-time", str(int(timeout)), "-o", "/dev/null",
             "-w", "%{http_code} %{size_download} %{speed_download} %{time_starttransfer}",
             url],
            capture_output=True, timeout=timeout + 8,
        )
        out = (p.stdout or b"").decode().strip().split()
        if p.returncode != 0 or len(out) < 4:
            return 0.0, float("inf")
        code, size, speed, ttfb = out[0], int(out[1]), float(out[2]), float(out[3])
        if code != "200" or size < 100_000 or speed <= 0:
            return 0.0, ttfb * 1000
        return speed, ttfb * 1000
    except Exception:
        return 0.0, float("inf")
    finally:
        if proc is not None:
            try:
                proc.kill()
                proc.wait(timeout=5)
            except Exception:
                pass
        if tmp and os.path.exists(tmp):
            try:
                os.unlink(tmp)
            except Exception:
                pass


def test_candidates(cands, on_progress=None):
    """Параллельный прогон. on_progress(done, total)."""
    results = []
    total = len(cands)
    with ThreadPoolExecutor(max_workers=PROBE_WORKERS) as ex:
        futs = {ex.submit(probe, c): c for c in cands}
        done = 0
        for fut in futs:
            pass
        for fut, cand in futs.items():
            alive, rtt = fut.result()
            done += 1
            if on_progress and done % 50 == 0:
                on_progress(done, total)
            if alive:
                c = dict(cand)
                c["rtt_ms"] = round(rtt, 1)
                results.append(c)
    results.sort(key=lambda x: x["rtt_ms"])
    return results


# ── генерация xray-конфига ────────────────────────────────────────────────

INBOUND = {
    "port": SOCKS_PORT,
    "protocol": "socks",
    "settings": {"auth": "noaccess", "udp": True},
    "sniffing": {"enabled": True, "destOverride": ["http", "tls"]},
}


def _stream_settings(c):
    """Собирает streamSettings из кандидата."""
    net = c.get("net") or "tcp"
    st = {"network": net}
    security = c.get("security") or "none"

    if c["scheme"] == "current":
        # Точная копия исходного Reality-конфига.
        st["security"] = security if security != "none" else "reality"
        if st["security"] == "reality":
            st["realitySettings"] = {
                "show": False,
                "serverName": c.get("sni") or "",
                "fingerprint": c.get("fingerprint") or "chrome",
                "publicKey": c.get("publicKey") or "",
                "shortId": c.get("shortId") or "",
            }
        return st

    if security in ("tls", "reality"):
        # ВАЖНО: у reality параметры идут в realitySettings, а НЕ в tlsSettings.
        # Если положить publicKey/shortId в tlsSettings, xray не найдёт
        # realitySettings и скажет «REALITY: Empty realitySettings».
        st["security"] = security
        common = {
            "serverName": c.get("sni") or c.get("host") or "",
            "fingerprint": c.get("fingerprint") or "chrome",
        }
        if c.get("allowInsecure") is not None:
            common["allowInsecure"] = bool(c["allowInsecure"])
        if security == "reality":
            rs = dict(common)
            rs["publicKey"] = c.get("pbk") or ""
            rs["shortId"] = c.get("sid") or ""
            rs["show"] = False
            st["realitySettings"] = rs
        else:
            st["tlsSettings"] = common

    if net == "ws":
        st["wsSettings"] = {
            "path": c.get("path") or "/",
            "headers": {"Host": c.get("host_hdr") or c.get("host")},
        }
    elif net == "grpc":
        st["grpcSettings"] = {"serviceName": c.get("path") or ""}
    elif net == "xhttp":
        # XHTTP требует СВОЮ секцию; без неё xray падает на всём конфиге.
        st["xhttpSettings"] = {
            "path": c.get("path") or "/",
            "host": c.get("host_hdr") or c.get("host") or "",
        }
    return st


def outbound_for(c):
    """Один outbound из кандидата."""
    s = c["scheme"]
    if s == "current":
        return c.get("_outbound") or {
            "protocol": "vless",
            "settings": {
                "vnext": [{
                    "address": c["host"],
                    "port": c["port"],
                    "users": [{
                        "id": c.get("uuid") or "",
                        "flow": c.get("flow") or "",
                        "encryption": c.get("encryption") or "none",
                    }],
                }]
            },
            "streamSettings": _stream_settings(c),
        }

    if s == "vmess":
        user = {
            "id": c.get("uuid") or "",
            "alterId": int(c.get("aid") or 0),
            "security": c.get("scy") or "auto",
        }
        return {
            "protocol": "vmess",
            "settings": {"vnext": [{
                "address": c["host"], "port": c["port"], "users": [user]
            }]},
            "streamSettings": _stream_settings(c),
        }

    if s == "vless":
        user = {
            "id": c.get("uuid") or "",
            "encryption": c.get("encryption") or "none",
        }
        if c.get("flow"):
            user["flow"] = c["flow"]
        return {
            "protocol": "vless",
            "settings": {"vnext": [{
                "address": c["host"], "port": c["port"], "users": [user]
            }]},
            "streamSettings": _stream_settings(c),
        }

    if s == "trojan":
        return {
            "protocol": "trojan",
            "settings": {"servers": [{
                "address": c["host"], "port": c["port"],
                "password": c.get("password") or "",
            }]},
            "streamSettings": _stream_settings(c),
        }

    if s == "ss":
        return {
            "protocol": "shadowsocks",
            "settings": {"servers": [{
                "address": c["host"], "port": c["port"],
                "method": c.get("method") or "aes-256-gcm",
                "password": c.get("password") or "",
            }]},
            "streamSettings": {"network": "tcp"},
        }

    return None


def usable(c):
    """Кандидат пригоден для генерации конфига, иначе xray не поднимется.

    Проверено на реальных подписках: vless с security=reality, но без pbk/sid
    даёт «REALITY: Empty realitySettings» и валит ВЕСЬ конфиг, а не один узел.
    Такие отсеиваем; также режем мусорные пути и нечисловые порты.
    """
    if not c.get("host") or not c.get("port"):
        return False
    if c["scheme"] == "current":
        return True
    sec = c.get("security") or "none"
    if sec == "reality" and not c.get("pbk"):
        return False
    if sec in ("tls", "reality"):
        # TLS без SNI: xray возьмёт имя из адреса, но для IP-адресов без
        # домена это гарантированный отказ TLS-рукопожатия.
        if not (c.get("sni") or c.get("host_hdr")) and ":" in str(c.get("host", "")) \
           and not str(c.get("host", "")).replace(".", "").isdigit():
            return False
    if c["scheme"] in ("vmess", "vless") and not c.get("uuid"):
        return False
    if c["scheme"] == "trojan" and not c.get("password"):
        return False
    if c["scheme"] == "ss" and not c.get("method"):
        return False
    return True


def build_config(cands):
    """Конфиг с балансировщиком по топ-100 + эталонным direct-выбором.

    balancer даёт устойчивость к падению текущего сервера: xray сам выбирает
    живой из списка, а мы держим «быстрый» тег для контроля.
    """
    outbounds = []
    tags = []
    skipped = 0
    for i, c in enumerate(cands):
        if not usable(c):
            skipped += 1
            continue
        ob = outbound_for(c)
        if not ob:
            skipped += 1
            continue
        tag = f"px-{len(tags):03d}"   # теги сквозные, без дыр от пропусков
        ob["tag"] = tag
        ob.pop("sendThrough", None)
        outbounds.append(ob)
        tags.append(tag)

    outbounds.append({"tag": "direct", "protocol": "freedom", "settings": {}})

    cfg = {
        "log": {"loglevel": "warning"},
        "inbounds": [INBOUND],
        "outbounds": outbounds,
        # Балансировщик подключён тегом "px-balancer" в НИЖНЕМ outbound:
        # xray применяет balancer к запросу через outboundTag, поэтому
        # отдельное правило routing с "no effective fields" не нужно.
        "balancers": [{
            "tag": "px-balancer",
            "selector": tags,
            "strategy": {"type": "leastPing"},
        }],
    }
    # Первым в списке идёт балансировщик — весь трафик идёт через него.
    outbounds.insert(0, {"tag": "px-balancer", "protocol": "freedom", "settings": {}})
    outbounds.append({"tag": "fallback-direct", "protocol": "freedom", "settings": {}})
    return cfg, len(outbounds)

def validate(cfg):
    """xray -test: если конфиг невалиден, его нельзя применять.

    ВАЖНО: -c принимает ПУТЬ к файлу, а не JSON-строку. Передача строки даёт
    «failed to load config files» с обрывом сообщения об ошибке.
    """
    import tempfile
    tmp = None
    try:
        fd, tmp = tempfile.mkstemp(suffix=".json", prefix="xray-test-")
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(cfg, f)
        p = subprocess.run(
            [XRAY_BIN, "run", "-test", "-c", tmp],
            capture_output=True, timeout=30,
        )
        return p.returncode == 0, (p.stderr or p.stdout).decode("utf-8", "replace")
    except Exception as e:
        return False, str(e)
    finally:
        if tmp and os.path.exists(tmp):
            os.unlink(tmp)


def backup_config(tag="sub"):
    try:
        os.makedirs(BACKUP_DIR, exist_ok=True)
        stamp = time.strftime("%Y%m%d-%H%M%S")
        dst = os.path.join(BACKUP_DIR, f"config-{tag}-{stamp}.json")
        with open(CONFIG, "rb") as src, open(dst, "wb") as out:
            out.write(src.read())
        return dst
    except Exception as e:
        log(f"бэкап не удался: {e}")
        return None


def write_config(cfg):
    tmp = CONFIG + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(cfg, f, indent=2, ensure_ascii=False)
    os.replace(tmp, CONFIG)


def reload_xray():
    """Перезапуск нативного xray с новым конфигом."""
    subprocess.run(["pkill", "-x", "xray"], capture_output=True)
    time.sleep(0.6)
    logp = os.path.join(HOME, ".local", "share", "ParanoidX.logs", "xray.log")
    os.makedirs(os.path.dirname(logp), exist_ok=True)
    with open(logp, "ab") as lf:
        subprocess.Popen(
            [XRAY_BIN, "run", "-c", CONFIG],
            stdout=lf, stderr=lf, start_new_session=True,
        )
    time.sleep(1.5)
    return socks_alive()


def socks_alive():
    try:
        s = socket.create_connection(("127.0.0.1", SOCKS_PORT), timeout=3)
        s.close()
        return True
    except Exception:
        return False


def save_last_good():
    try:
        with open(CONFIG, "rb") as src, open(LAST_GOOD, "w", encoding="utf-8") as out:
            out.write(src.read().decode("utf-8", "replace"))
    except Exception:
        pass


# ── протокол ───────────────────────────────────────────────────────────────

def load_servers():
    try:
        with open(SERVERS, encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return []


def save_servers(cands):
    with open(SERVERS, "w", encoding="utf-8") as f:
        json.dump(cands, f, indent=1, ensure_ascii=False)


def save_state(st):
    with open(STATE, "w", encoding="utf-8") as f:
        json.dump(st, f, indent=2, ensure_ascii=False)


def gather(limit=MAX_TEST):
    """Скачать + распарсить + добавить текущий конфиг как кандидата."""
    cands, seen, stats = [], set(), {"subs_ok": 0, "subs_fail": 0, "raw": 0, "parsed": 0, "skipped_proto": 0}

    for url in SUB_URLS:
        try:
            lines = decode_sub(fetch(url))
            stats["subs_ok"] += 1
            got = 0
            for ln in lines:
                if not ln.startswith(("vmess://", "vless://", "ss://", "trojan://")):
                    if ln.startswith(("hysteria2://", "hy2://", "tuic://", "socks://")):
                        stats["skipped_proto"] += 1
                    continue
                stats["raw"] += 1
                c = parse_line(ln)
                if not c or not c.get("host"):
                    continue
                key = (c["host"], c["port"], c["scheme"])
                if key in seen:
                    continue
                seen.add(key)
                cands.append(c)
                got += 1
            stats["parsed"] += got
            log(f"{url.rsplit('/', 2)[-2]}: +{got} узлов")
        except Exception as e:
            stats["subs_fail"] += 1
            log(f"подписка не скачалась {url}: {e}")

    # Текущий конфиг — полноправный участник рейтинга.
    cur = current_candidate()
    if cur:
        ob = read_current_outbound()
        cur["_outbound"] = ob["config"] if ob else None
        cands.insert(0, cur)
        stats["current_added"] = True
        log(f"текущий конфиг добавлен в рейтинг: {cur['host']}:{cur['port']}")

    stats["total"] = len(cands)
    return cands[:limit] if limit else cands, stats


def rank(cands, top_n=TOP_N, rtt_limit=GOOD_RTT_MS, progress=None,
          speed_shortlist=40, min_speed=200_000):
    """Двухфазный отбор, как задумано.

    Фаза 1 (быстро, параллельно, ВСЕ кандидаты): TCP-connect + RTT.
             Мёртвые отсеиваются здесь, slow — уходят в хвост.
    Фаза 2 (короткий шортлист): реальная скорость НАПРЯМУЮ мимо бриджа.
             Скорость — настоящий критерий выбора, а RTT лишь предварительный.

    Порядок вывода: быстрые в начало, медленные в конец, мёртвые удалены,
    остаётся ровно top_n лучших.
    """
    # ── фаза 1: пинг всех ────────────────────────────────────────────────
    alive = test_candidates(cands, on_progress=progress)
    dead = len(cands) - len(alive)
    alive.sort(key=lambda x: x["rtt_ms"])

    # Годные по RTT; если их больше шортлиста — скорость меряем только там.
    rtt_ok = [c for c in alive if c["rtt_ms"] <= rtt_limit]
    shortlist = rtt_ok[:speed_shortlist] if rtt_ok else []

    # ── фаза 2: реальная скорость через отдельный xray, мимо бриджа ──────
    # Каждый замер поднимает СВОЙ xray на своём порту (PROBE_PORT + i),
    # поэтому замеры идут параллельно и не мешают друг другу.
    speeds = {}
    if shortlist:
        def sp(i_c):
            i, c = i_c
            bps, ttfb = throughput(c, port=PROBE_PORT + i)
            return (c["host"], c["port"]), bps, ttfb

        pairs = list(enumerate(shortlist))
        with ThreadPoolExecutor(max_workers=min(SPEED_WORKERS, len(pairs))) as ex:
            for key, bps, ttfb in ex.map(sp, pairs):
                speeds[key] = (bps, ttfb)

    measured = []
    for c in shortlist:
        bps, ttfb = speeds.get((c["host"], c["port"]), (0.0, float("inf")))
        if bps < min_speed:
            continue                      # не прошёл порог скорости
        c = dict(c)
        c["speed_bps"] = round(bps, 0)
        c["ttfb_ms"] = round(ttfb, 1)
        measured.append(c)

    # Основной критерий — скорость; при равенстве — RTT.
    measured.sort(key=lambda x: (-x["speed_bps"], x["rtt_ms"]))

    stats = {
        "tested": len(cands),
        "dead": dead,
        "alive": len(alive),
        "speed_measured": len(shortlist),
        "speed_ok": len(measured),
        "kept": min(len(measured), top_n),
    }
    if not measured:
        # Ничего не прошло порог скорости — откатываемся на RTT-рейтинг,
        # чтобы не оставлять ноду без прокси. Это НЕ гарантия работоспособности:
        # пинг не отличает рабочий прокси от мёртвого (см. throughput).
        fallback = [dict(c, speed_bps=0, ttfb_ms=0) for c in rtt_ok[:top_n]]
        stats["fallback_rtt"] = True
        return fallback, stats

    return measured[:top_n], stats


def protocol_update(apply_new=True, top_n=TOP_N):
    t0 = time.time()
    st = {
        "phase": "fetching", "started": t0,
        "ok": False, "applied": False, "message": "",
        "subs_ok": 0, "subs_fail": 0, "raw": 0, "parsed": 0,
        "tested": 0, "dead": 0, "alive": 0, "kept": 0,
        "skipped_proto": 0, "current_added": False,
    }

    cands, stats = gather()
    st.update({k: v for k, v in stats.items() if k != "total"})
    st["total_candidates"] = len(cands)
    save_state(st)

    if not cands:
        st["phase"] = "failed"
        st["message"] = "нет кандидатов (подписки недоступны)"
        save_state(st)
        return st

    st["phase"] = "testing"
    save_state(st)

    def prog(done, total):
        st["tested"] = done
        st["phase"] = f"testing {done}/{total}"
        save_state(st)

    top, rstats = rank(cands, top_n=top_n, progress=prog)
    st.update(rstats)
    st["phase"] = "building"
    save_state(st)

    if not top:
        # GATE: ни одного годного сервера — остаёмся на текущем конфиге.
        st["phase"] = "failed"
        st["ok"] = False
        st["message"] = f"нет серверов быстрее {GOOD_RTT_MS} мс — оставлен прежний конфиг"
        save_state(st)
        log(st["message"])
        return st

    save_servers(top)
    cfg, n_ob = build_config(top)

    ok, err = validate(cfg)
    if not ok:
        # НЕ обрезаем до 200 символов: в сообщении xray полезная часть
        # (сама ошибка) в самом конце, а начало — это баннер версии. Раньше
        # обрезка съедала причину, и сбой выглядел загадочно.
        detail = (err or "").strip()
        # Отбрасываем баннер xray, оставляя строку с ошибкой.
        lines = [ln for ln in detail.splitlines() if ln.strip()]
        reason = next(
            (ln for ln in reversed(lines)
             if not ln.startswith("2026/") and "Xray" not in ln
             and "anti-censorship" not in ln and "unified platform" not in ln),
            lines[-1] if lines else "неизвестная ошибка",
        )
        st["phase"] = "failed"
        st["message"] = f"xray отверг конфиг: {reason[:200]}"
        st["validate_error"] = reason[:400]
        save_state(st)
        log(st["message"])
        return st

    st["ok"] = True
    st["outbounds"] = n_ob
    st["best_rtt_ms"] = top[0]["rtt_ms"]
    st["best"] = f"{top[0]['host']}:{top[0]['port']} ({top[0]['rtt_ms']} мс)"

    if apply_new:
        b = backup_config()
        st["backup"] = b
        write_config(cfg)
        alive = reload_xray()
        st["applied"] = alive
        if alive:
            save_last_good()
        else:
            # Откат на забэкапированный конфиг.
            if b and os.path.exists(b):
                with open(b, "rb") as src, open(CONFIG, "wb") as out:
                    out.write(src.read())
                reload_xray()
                st["message"] = "новый конфиг не поднялся — откат на предыдущий"
        st["message"] = st["message"] or f"применён топ-1: {st['best']}"
    else:
        st["message"] = f"протокол выполнен (dry-run), топ-1 {st['best']}"

    st["phase"] = "done" if st["ok"] else "failed"
    st["elapsed"] = round(time.time() - t0, 1)
    save_state(st)
    log(st["message"])
    return st


def status():
    try:
        with open(STATE, encoding="utf-8") as f:
            st = json.load(f)
    except Exception:
        st = {"phase": "never-run", "ok": False, "message": "протокол ещё не запускался"}
    s = read_current_outbound()
    st["socks_alive"] = socks_alive()
    st["xray_running"] = bool(
        subprocess.run(["pgrep", "-x", "xray"], capture_output=True).returncode == 0
    )
    st["servers_saved"] = len(load_servers())
    st["current"] = (
        f"{s['config'].get('protocol')} → "
        f"{(s['config'].get('settings', {}).get('vnext') or [{}])[0].get('address')}"
        if s else "?"
    )
    return st


def main():
    cmd = sys.argv[1] if len(sys.argv) > 1 else "update"
    if cmd == "update":
        st = protocol_update(apply_new="--dry" not in sys.argv)
        print(json.dumps(st, indent=2, ensure_ascii=False))
        return 0 if st["ok"] else 1
    if cmd == "test":
        cands, stats = gather()
        top, r = rank(cands)
        print(json.dumps({"gather": stats, "rank": r, "top": top[:20]}, indent=2, ensure_ascii=False))
        return 0
    if cmd == "status":
        print(json.dumps(status(), indent=2, ensure_ascii=False))
        return 0
    if cmd == "servers":
        n = int(sys.argv[2]) if len(sys.argv) > 2 else 20
        c = load_servers()
        for i, s in enumerate(c[:n]):
            flag = " ← CURRENT" if s.get("is_current") else ""
            print(f"{i+1:3}. {s['rtt_ms']:7.1f} мс  {s['scheme']:8} {s['host']}:{s['port']}{flag}")
        return 0
    print(__doc__)
    return 1


if __name__ == "__main__":
    sys.exit(main())
