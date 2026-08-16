#!/usr/bin/env python3
"""Import v2ray/xray share links (vless:// vmess:// trojan:// ss://) into
ParanoidX bridge client pool.

Output: ~/.local/share/simplex-node/paranoidx/v2ray/outbounds.json
Each outbound is a ready xray outbound object with a stable tag.
"""
import base64, binascii, json, os, re, sys, urllib.parse

POOL_DIR = os.path.expanduser("~/.local/share/simplex-node/paranoidx/v2ray")
OUT = os.path.join(POOL_DIR, "outbounds.json")


def b64u(s: str) -> str:
    s = s.strip()
    pad = "=" * (-len(s) % 4)
    try:
        return base64.urlsafe_b64decode(s + pad).decode()
    except Exception:
        try:
            return base64.b64decode(s + pad).decode()
        except Exception:
            return ""


def parse_vless(rest: str, frag: str) -> dict:
    """vless://uuid@host:port?params#name"""
    userinfo, _, hostpart = rest.partition("@")
    host, _, port = hostpart.rpartition(":")
    q = urllib.parse.parse_qs(urllib.parse.urlparse("?" + (hostpart.split("?", 1)[1] if "?" in hostpart else "")).query) if "?" in hostpart else {}
    # simpler: split query manually
    hp, _, qs = hostpart.partition("?")
    host, _, port = hp.rpartition(":")
    q = urllib.parse.parse_qs(qs) if qs else {}
    sec = q.get("security", ["none"])[0]
    type_ = q.get("type", ["tcp"])[0]
    stream = {"network": type_, "security": sec}
    if type_ == "ws":
        ws = {}
        if q.get("path"): ws["path"] = urllib.parse.unquote(q["path"][0])
        if q.get("host"): ws["headers"] = {"Host": q["host"][0]}
        stream["wsSettings"] = ws
    if sec == "tls" or q.get("sni"):
        tls = {}
        if q.get("sni"): tls["serverName"] = q["sni"][0]
        if q.get("fp"): tls["fingerprint"] = q["fp"][0]
        if q.get("alpn"): tls["alpn"] = [urllib.parse.unquote(a) for a in q["alpn"][0].split(",")]
        if q.get("insecure", ["0"])[0] == "1": tls["allowInsecure"] = True
        stream["tlsSettings"] = tls
    vnext = [{"address": host, "port": int(port), "users": [{
        "id": userinfo, "encryption": q.get("encryption", ["none"])[0],
        "flow": q.get("flow", [""])[0],
    }]}]
    return {"protocol": "vless", "tag": f"vless-{host}-{port}", "settings": {"vnext": vnext}, "streamSettings": stream}


def parse_vmess(b64: str, frag: str) -> dict:
    """vmess://base64json"""
    raw = b64u(b64)
    d = json.loads(raw)
    sec = d.get("tls", "") or ""
    type_ = d.get("net", "tcp")
    stream = {"network": type_, "security": sec or "none"}
    if type_ == "ws":
        ws = {}
        if d.get("path"): ws["path"] = d["path"]
        if d.get("host"): ws["headers"] = {"Host": d["host"]}
        stream["wsSettings"] = ws
    if sec == "tls":
        tls = {}
        if d.get("sni"): tls["serverName"] = d["sni"]
        if d.get("fp"): tls["fingerprint"] = d["fp"]
        if d.get("alpn"): tls["alpn"] = [d["alpn"]]
        if str(d.get("insecure", "0")) == "1": tls["allowInsecure"] = True
        stream["tlsSettings"] = tls
    vnext = [{"address": d["add"], "port": int(d["port"]), "users": [{
        "id": d["id"], "alterId": int(d.get("aid", "0") or 0), "security": d.get("scy", "auto"),
    }]}]
    return {"protocol": "vmess", "tag": f"vmess-{d['add']}-{d['port']}", "settings": {"vnext": vnext}, "streamSettings": stream}


def parse_trojan(rest: str, frag: str) -> dict:
    """trojan://pass@host:port?params#name"""
    userinfo, _, hostpart = rest.partition("@")
    hp, _, qs = hostpart.partition("?")
    host, _, port = hp.rpartition(":")
    q = urllib.parse.parse_qs(qs) if qs else {}
    type_ = q.get("type", ["tcp"])[0]
    stream = {"network": type_, "security": "tls"}
    if type_ == "ws":
        ws = {}
        if q.get("path"): ws["path"] = urllib.parse.unquote(q["path"][0])
        if q.get("host"): ws["headers"] = {"Host": q["host"][0]}
        stream["wsSettings"] = ws
    tls = {}
    if q.get("sni"): tls["serverName"] = q["sni"][0]
    if q.get("fp"): tls["fingerprint"] = q["fp"][0]
    if q.get("alpn"): tls["alpn"] = [urllib.parse.unquote(a) for a in q["alpn"][0].split(",")]
    if q.get("insecure", ["0"])[0] == "1": tls["allowInsecure"] = True
    stream["tlsSettings"] = tls
    servers = [{"address": host, "port": int(port), "password": urllib.parse.unquote(userinfo)}]
    return {"protocol": "trojan", "tag": f"trojan-{host}-{port}", "settings": {"servers": servers}, "streamSettings": stream}


def parse_ss(rest: str, frag: str) -> dict:
    """ss://base64(method:pass)@host:port or ss://base64(method:pass@host:port)"""
    if "@" in rest:
        cred, _, hostport = rest.partition("@")
        try:
            method, _, pw = b64u(cred).partition(":")
        except Exception:
            method, pw = "aes-256-gcm", ""
    else:
        raw = b64u(rest)
        cred, _, hostport = raw.rpartition("@")
        method, _, pw = cred.partition(":")
    host, _, port = hostport.rpartition(":")
    servers = [{"address": host, "port": int(port), "method": method, "password": pw}]
    return {"protocol": "shadowsocks", "tag": f"ss-{host}-{port}", "settings": {"servers": servers},
            "streamSettings": {"network": "tcp", "security": "none"}}


PARSERS = {"vless": parse_vless, "vmess": parse_vmess, "trojan": parse_trojan, "ss": parse_ss}


def import_links(text: str) -> list:
    out = []
    seen = set()
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        m = re.match(r"^([a-z0-9]+)://(.+)$", line)
        if not m:
            print(f"SKIP (no scheme): {line[:60]}")
            continue
        scheme, rest = m.group(1), m.group(2)
        frag = ""
        if "#" in rest:
            rest, _, frag = rest.partition("#")
        if scheme not in PARSERS:
            print(f"SKIP (unsupported {scheme}): {line[:60]}")
            continue
        try:
            ob = PARSERS[scheme](rest, frag)
        except Exception as e:
            print(f"SKIP ({scheme} parse error {e}): {line[:60]}")
            continue
        key = ob["tag"]
        if key in seen:
            continue
        seen.add(key)
        name = urllib.parse.unquote(frag).strip() if frag else key
        out.append({"outbound": ob, "name": name, "source": "user-import"})
        print(f"OK: {scheme} {ob['tag']} ({name})")
    return out


def main():
    src = sys.argv[1] if len(sys.argv) > 1 else "/tmp/v2ray_links.txt"
    with open(src) as f:
        text = f.read()
    items = import_links(text)
    os.makedirs(POOL_DIR, exist_ok=True)
    # merge with existing
    existing = []
    if os.path.exists(OUT):
        try:
            existing = json.load(open(OUT))
        except Exception:
            existing = []
    known = {it["outbound"]["tag"] for it in existing}
    for it in items:
        if it["outbound"]["tag"] not in known:
            existing.append(it)
    with open(OUT, "w") as f:
        json.dump(existing, f, indent=2, ensure_ascii=False)
    print(f"\nTotal in pool: {len(existing)} (added {len(items)})")
    print(f"Pool: {OUT}")


if __name__ == "__main__":
    main()
