#!/bin/bash
ISLE_HOME="${ISLE_HOME:-$HOME}"  # переносимый дом проекта: переопредели env-ом, дефолт $HOME
# setup-vmess.sh — Generate VMess server config, client config, systemd service
# Usage: ./scripts/setup-vmess.sh [--uuid <UUID>] [--port 10812]
set -euo pipefail

VMESS_PORT=10812
UUID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --uuid) UUID="$2"; shift 2 ;;
    --port) VMESS_PORT="$2"; shift 2 ;;
    *) echo "Usage: $0 [--uuid UUID] [--port PORT]"; exit 1 ;;
  esac
done

if [ -z "$UUID" ]; then
  UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$(date +%s)-$$-$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')" | sha256sum | head -c 32 | sed 's/\(........\)\(....\)\(....\)\(....\)\(............\)/\1-\2-\3-\4-\5/')
fi

DATA_DIR="${HOME}/.local/share/simplex-node"
VMESS_DIR="${DATA_DIR}/vmess"
XRAY_BIN="$ISLE_HOME/bin/v2ray/xray"

mkdir -p "${VMESS_DIR}" "${DATA_DIR}/logs"

# === VMess Server Config (xray listens as VMess server on :10812) ===
cat > "${VMESS_DIR}/server.json" <<VMESSEOF
{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "port": ${VMESS_PORT},
      "protocol": "vmess",
      "settings": {
        "clients": [
          {
            "id": "${UUID}",
            "level": 0,
            "alterId": 0
          }
        ],
        "disableInsecureEncryption": true
      },
      "streamSettings": {
        "network": "tcp",
        "security": "none"
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
VMESSEOF

# === VMess Client Config Patch — replaces freedom outbound with VMess ===
# The main xray config ($ISLE_HOME/bin/v2ray/config.json) will have its
# outbound changed from freedom to VMess pointing at the local VMess server.
cat > "${VMESS_DIR}/client_patch.json" <<CLIEOF
{
  "outbounds": [
    {
      "protocol": "vmess",
      "settings": {
        "vnext": [
          {
            "address": "127.0.0.1",
            "port": ${VMESS_PORT},
            "users": [
              {
                "id": "${UUID}",
                "alterId": 0,
                "security": "auto"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "none"
      }
    }
  ]
}
CLIEOF

# Save metadata
cat > "${VMESS_DIR}/meta.json" <<METAEOF
{
  "uuid": "${UUID}",
  "port": ${VMESS_PORT},
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "server_config": "${VMESS_DIR}/server.json",
  "client_patch": "${VMESS_DIR}/client_patch.json"
}
METAEOF

echo "✓ VMess configs generated"
echo "  UUID: ${UUID}"
echo "  Port: ${VMESS_PORT}"
echo "  Server config: ${VMESS_DIR}/server.json"
echo "  Client patch:  ${VMESS_DIR}/client_patch.json"

# === Apply client patch to main xray config ===
MAIN_CONFIG="$ISLE_HOME/bin/v2ray/config.json"
if [ -f "${MAIN_CONFIG}" ]; then
  # Use jq if available, otherwise patch manually
  if command -v jq &>/dev/null; then
    CLIENT_OUTBOUND=$(cat "${VMESS_DIR}/client_patch.json")
    jq '.outbounds = '"${CLIENT_OUTBOUND}"'["outbounds"]' "${MAIN_CONFIG}" > "${MAIN_CONFIG}.tmp" && mv "${MAIN_CONFIG}.tmp" "${MAIN_CONFIG}"
    echo "✓ Main xray config patched (via jq)"
  else
    # Fallback: simple JSON replace using python
    python3 -c "
import json
with open('${MAIN_CONFIG}') as f:
    cfg = json.load(f)
with open('${VMESS_DIR}/client_patch.json') as f:
    patch = json.load(f)
cfg['outbounds'] = patch['outbounds']
with open('${MAIN_CONFIG}', 'w') as f:
    json.dump(cfg, f, indent=2)
" 2>/dev/null || {
      echo "WARNING: could not patch main config (no jq/python). Manual edit needed."
      echo "  Replace outbounds in ${MAIN_CONFIG} with:"
      cat "${VMESS_DIR}/client_patch.json"
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
      "protocol": "vmess",
      "settings": {
        "vnext": [
          {
            "address": "127.0.0.1",
            "port": ${VMESS_PORT},
            "users": [
              {
                "id": "${UUID}",
                "alterId": 0,
                "security": "auto"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "none"
      }
    }
  ]
}
MAINEOF
  echo "✓ New main config created with VMess outbound"
fi

# === Create systemd user service ===
SERVICE_DIR="${HOME}/.config/systemd/user"
mkdir -p "${SERVICE_DIR}"

cat > "${SERVICE_DIR}/vmess-server.service" <<SERVEOF
[Unit]
Description=ParanoidX VMess Server (xray-core)
After=network.target

[Service]
Type=simple
ExecStart=${XRAY_BIN} run -c ${VMESS_DIR}/server.json
Restart=on-failure
RestartSec=5
StandardOutput=append:${DATA_DIR}/logs/vmess-server.log
StandardError=append:${DATA_DIR}/logs/vmess-server.log

[Install]
WantedBy=default.target
SERVEOF

systemctl --user daemon-reload
echo "✓ Systemd user service created: vmess-server.service"

# === Start VMess server ===
systemctl --user enable --now vmess-server.service 2>/dev/null || {
  echo "WARNING: systemctl enable failed, starting manually"
  nohup "${XRAY_BIN}" run -c "${VMESS_DIR}/server.json" > "${DATA_DIR}/logs/vmess-server.log" 2>&1 &
  echo "  VMess server PID: $!"
}

sleep 1

# === Verify ===
if ss -tlnp 2>/dev/null | grep -q ":${VMESS_PORT}"; then
  echo "✓ VMess server listening on :${VMESS_PORT}"
else
  echo "WARNING: VMess server not yet listening on :${VMESS_PORT}"
  echo "  Check logs: ${DATA_DIR}/logs/vmess-server.log"
fi

echo ""
echo "=== VMess Setup Complete ==="
echo "UUID:      ${UUID}"
echo "Port:      ${VMESS_PORT}"
echo "Configs:   ${VMESS_DIR}/"
echo "Service:   vmess-server.service (user)"
echo ""
echo "Traffic flow: SOCKS5 :10810 → VMess → VMess Server :${VMESS_PORT} → Internet"
