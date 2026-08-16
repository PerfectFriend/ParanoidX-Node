#!/usr/bin/env python3
"""Import .ovpn files into ParanoidX OpenVPN pool.

Output: ~/.local/share/simplex-node/paranoidx/openvpn/pool.json
Each entry is a ready OpenVPN client config with metadata.
"""
import json, os, re, sys, hashlib
from pathlib import Path

POOL_DIR = os.path.expanduser("~/.local/share/simplex-node/paranoidx/openvpn")
OUT = os.path.join(POOL_DIR, "pool.json")


def parse_ovpn(content: str, filename: str) -> dict:
    """Extract key fields from .ovpn file."""
    lines = content.splitlines()

    # Extract remotes (host:port, protocol)
    remotes = []
    for line in lines:
        m = re.match(r"^remote\s+(\S+)\s+(\d+)", line.strip())
        if m:
            remotes.append({"host": m.group(1), "port": int(m.group(2))})

    # Protocol
    proto = "udp"
    for line in lines:
        if line.strip().startswith("proto "):
            proto = line.strip().split()[1]
            break

    # Cipher
    cipher = "AES-128-GCM"
    for line in lines:
        if line.strip().startswith("cipher "):
            cipher = line.strip().split()[1]
            break

    # Auth
    auth = "SHA1"
    for line in lines:
        if line.strip().startswith("auth "):
            auth = line.strip().split()[1]
            break

    # TLS auth / crypt
    tls_auth = None
    tls_crypt = None
    for line in lines:
        if line.strip().startswith("tls-auth "):
            tls_auth = line.strip().split()[1]
        elif line.strip().startswith("tls-crypt "):
            tls_crypt = line.strip().split()[1]

    # Cert/key inline?
    has_ca = "<ca>" in content
    has_cert = "<cert>" in content
    has_key = "<key>" in content
    has_tls_auth_key = "<tls-auth>" in content

    # Friendly name from filename
    name = Path(filename).stem
    # Clean up common prefixes
    name = re.sub(r"^(doc_[a-f0-9]+_)?", "", name)

    # Generate stable ID from content hash
    config_hash = hashlib.sha256(content.encode()).hexdigest()[:16]

    return {
        "id": config_hash,
        "name": name,
        "filename": filename,
        "remotes": remotes,
        "protocol": proto,
        "cipher": cipher,
        "auth": auth,
        "tls_auth": tls_auth,
        "tls_crypt": tls_crypt,
        "inline": {
            "ca": has_ca,
            "cert": has_cert,
            "key": has_key,
            "tls_auth": has_tls_auth_key,
        },
        "content": content,  # store full content for later use
    }


def import_folder(folder: str) -> list:
    items = []
    for f in Path(folder).glob("*.ovpn"):
        try:
            content = f.read_text(encoding="utf-8", errors="ignore")
            parsed = parse_ovpn(content, f.name)
            items.append(parsed)
            print(f"OK: {parsed['name']} ({len(parsed['remotes'])} remotes, {parsed['protocol']})")
        except Exception as e:
            print(f"SKIP ({f.name}: {e})")
    return items


def main():
    if len(sys.argv) < 2:
        print("Usage: import-ovpn.py <folder>")
        sys.exit(1)

    folder = sys.argv[1]
    if not os.path.isdir(folder):
        print(f"Folder not found: {folder}")
        sys.exit(1)

    items = import_folder(folder)
    os.makedirs(POOL_DIR, exist_ok=True)

    # merge with existing
    existing = []
    if os.path.exists(OUT):
        try:
            existing = json.load(open(OUT))
        except Exception:
            existing = []

    known = {it["id"] for it in existing}
    added = 0
    for it in items:
        if it["id"] not in known:
            existing.append(it)
            added += 1

    with open(OUT, "w") as f:
        json.dump(existing, f, indent=2, ensure_ascii=False)

    print(f"\nTotal in pool: {len(existing)} (added {added})")
    print(f"Pool: {OUT}")


if __name__ == "__main__":
    main()