#!/bin/bash
# setup-vless.sh — Generate VLESS+Reality server config, client config, systemd service
# Usage: ./scripts/setup-vless.sh [--uuid <UUID>] [--port 443] [--dest <domain>] [--sni <sni>]
# Replaces deprecated VMess with modern VLESS+XTLS-Reality (forward secrecy, no fingerprints)

set -euo pipefail

VLESS_PORT=10813
UUID=""
DEST="www.microsoft.com:443"
SNI="www.microsoft.com"
PRIV_KEY=""
PUB_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --uuid) UUID="$2"; shift 2 ;;
    --port) VLESS_PORT="$2"; shift 2 ;;
    --dest) DEST="$2"; shift 2 ;;
    --sni) SNI="$2"; shift 2 ;;
    --privkey) PRIV_KEY="$2"; shift 2 ;;
    *) echo "Usage: $0 [--uuid UUID] [--port PORT] [--dest DEST] [--sni SNI] [--privkey PRIVKEY]"; exit 1 ;;
  esac
done

if [ -z "$UUID" ]; then
  UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$(date +%s)-$$-$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')" | sha256sum | head -c 32 | sed 's/\(........\)\(....\)\(....\)\(....\)\(............\)/\1-\2-\3-\4-\5/')
fi

# Generate Reality keypair if not provided
if [ -z "$PRIV_KEY" ]; then
  REALITY_KEYS=$(/home/tomas/bin/v2ray/xray x25519)
  PRIV_KEY=$(echo "$REALITY_KEYS" | grep "PrivateKey:" | awk '{print $2}')
  PUB_KEY=$(echo "$REALITY_KEYS" | grep "Password (PublicKey):" | awk '{print $3}')
else
  PUB_KEY=$(echo "$PRIV_KEY" | /home/tomas/bin/v2ray/xray x25519 -i /dev/stdin | grep "Password (PublicKey):" | awk '{print $3}')
fi

DATA_DIR="${HOME}/.local/share/simplex-node"
VLESS_DIR="${DATA_DIR}/vless"
XRAY_BIN="/home/tomas/bin/v2ray/xray"
SHORT_ID=$(openssl rand -hex 8)

mkdir -p "${VLESS_DIR}" "${DATA_DIR}/logs"

# === VLESS+Reality Server Config (xray listens as VLESS server on :443) ===
cat > "${VLESS_DIR}/server.json" <<VLESSEOF
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": ${VLESS_PORT},
      "protocol": "vless",
      "settings": {
        "clients": [
          {
            "id": "${UUID}",
            "flow": "xtls-rprx-vision",
            "level": 0
          }
        ],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "show": false,
          "dest": "${DEST}",
          "xver": 0,
          "serverNames": ["${SNI}"],
          "privateKey": "${PRIV_KEY}",
          "shortIds": ["${SHORT_ID}"]
        }
      },
      "sniffing": {
        "enabled": true,
        "destOverride": ["http", "tls", "quic"]
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "freedom",
      "settings": {}
    }
  ]
}
VLESSEOF

# === VLESS Client Config Patch — replaces freedom outbound with VLESS+Reality ===
cat > "${VLESS_DIR}/client_patch.json" <<CLIEOF
{
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "127.0.0.1",
            "port": ${VLESS_PORT},
            "users": [
              {
                "id": "${UUID}",
                "flow": "xtls-rprx-vision",
                "encryption": "none",
                "level": 0
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "show": false,
          "serverName": "${SNI}",
          "fingerprint": "chrome",
          "publicKey": "${PUB_KEY}",
          "shortId": "${SHORT_ID}"
        }
      }
    }
  ]
}
CLIEOF

# Save metadata
cat > "${VLESS_DIR}/meta.json" <<METAEOF
{
  "uuid": "${UUID}",
  "port": ${VLESS_PORT},
  "dest": "${DEST}",
  "sni": "${SNI}",
  "privateKey": "${PRIV_KEY}",
  "publicKey": "${PUB_KEY}",
  "shortId": "${SHORT_ID}",
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "server_config": "${VLESS_DIR}/server.json",
  "client_patch": "${VLESS_DIR}/client_patch.json"
}
METAEOF

echo "✓ VLESS+Reality configs generated"
echo "  UUID:       ${UUID}"
echo "  Port:       ${VLESS_PORT}"
echo "  Dest:       ${DEST}"
echo "  SNI:        ${SNI}"
echo "  PublicKey:  ${PUB_KEY}"
echo "  ShortId:    ${SHORT_ID}"
echo "  Server:     ${VLESS_DIR}/server.json"
echo "  Client:     ${VLESS_DIR}/client_patch.json"

# === Apply client patch to main xray config ===
MAIN_CONFIG="/home/tomas/bin/v2ray/config.json"
if [ -f "${MAIN_CONFIG}" ]; then
  if command -v jq &>/dev/null; then
    CLIENT_OUTBOUND=$(cat "${VLESS_DIR}/client_patch.json")
    jq '.outbounds = '"${CLIENT_OUTBOUND}"'["outbounds"]' "${MAIN_CONFIG}" > "${MAIN_CONFIG}.tmp" && mv "${MAIN_CONFIG}.tmp" "${MAIN_CONFIG}"
    echo "✓ Main xray config patched (via jq)"
  else
    python3 -c "
import json
with open('${MAIN_CONFIG}') as f:
    cfg = json.load(f)
with open('${VLESS_DIR}/client_patch.json') as f:
    patch = json.load(f)
cfg['outbounds'] = patch['outbounds']
with open('${MAIN_CONFIG}', 'w') as f:
    json.dump(cfg, f, indent=2)
" 2>/dev/null || {
      echo "WARNING: could not patch main config (no jq/python). Manual edit needed."
      echo "  Replace outbounds in ${MAIN_CONFIG} with:"
      cat "${VLESS_DIR}/client_patch.json"
    }
    echo "✓ Main xray config patched"
  fi
else
  echo "WARNING: ${MAIN_CONFIG} not found — creating new main config from client_patch"
  cat > "${MAIN_CONFIG}" <<MAINEOF
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": 10810,
      "protocol": "socks",
      "settings": {
        "auth": "noaccess",
        "udp": true
      },
      "sniffing": {
        "enabled": true,
        "destOverride": ["http", "tls"]
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "127.0.0.1",
            "port": ${VLESS_PORT},
            "users": [
              {
                "id": "${UUID}",
                "flow": "xtls-rprx-vision",
                "encryption": "none",
                "level": 0
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "show": false,
          "serverName": "${SNI}",
          "fingerprint": "chrome",
          "publicKey": "${PUB_KEY}",
          "shortId": "${SHORT_ID}"
        }
      }
    }
  ]
}
MAINEOF
  echo "✓ New main config created with VLESS+Reality outbound"
fi

# === Create systemd user service ===
SERVICE_DIR="${HOME}/.config/systemd/user"
mkdir -p "${SERVICE_DIR}"

cat > "${SERVICE_DIR}/vless-server.service" <<SERVEOF
[Unit]
Description=ParanoidX VLESS+Reality Server (xray-core)
After=network.target

[Service]
Type=simple
ExecStart=${XRAY_BIN} run -c ${VLESS_DIR}/server.json
Restart=on-failure
RestartSec=5
StandardOutput=append:${DATA_DIR}/logs/vless-server.log
StandardError=append:${DATA_DIR}/logs/vless-server.log

[Install]
WantedBy=default.target
SERVEOF

systemctl --user daemon-reload
echo "✓ Systemd user service created: vless-server.service"

# === Start VLESS server ===
systemctl --user enable --now vless-server.service 2>/dev/null || {
  echo "WARNING: systemctl enable failed, starting manually"
  nohup "${XRAY_BIN}" run -c "${VLESS_DIR}/server.json" > "${DATA_DIR}/logs/vless-server.log" 2>&1 &
  echo "  VLESS server PID: $!"
}

sleep 2

# === Verify ===
if ss -tlnp 2>/dev/null | grep -q ":${VLESS_PORT}"; then
  echo "✓ VLESS+Reality server listening on :${VLESS_PORT}"
else
  echo "WARNING: VLESS server not yet listening on :${VLESS_PORT}"
  echo "  Check logs: ${DATA_DIR}/logs/vless-server.log"
fi

echo ""
echo "=== VLESS+Reality Setup Complete ==="
echo "UUID:         ${UUID}"
echo "Port:         ${VLESS_PORT}"
echo "Dest:         ${DEST}"
echo "SNI:          ${SNI}"
echo "PublicKey:    ${PUB_KEY}"
echo "ShortId:      ${SHORT_ID}"
echo "Configs:      ${VLESS_DIR}/"
echo "Service:      vless-server.service (user)"
echo ""
echo "Traffic flow: SOCKS5 :10810 → VLESS+Reality → VLESS Server :${VLESS_PORT} → ${DEST} (via Reality)"
echo ""
echo "Client config (VLESS URI):"
echo "vless://${UUID}@127.0.0.1:${VLESS_PORT}?type=tcp&security=reality&pbk=${PUB_KEY}&fp=chrome&sni=${SNI}&sid=${SHORT_ID}&flow=xtls-rprx-vision#ParanoidX-VLESS"