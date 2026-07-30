#!/usr/bin/env bash
set -euo pipefail
# ============================================================================
# simplex-node — Install Pack for Ubuntu Linux (Beelink SER9)
# Services only (no Isle/Royal client applications)
# Saint Mary Liberty Island — Sovereign Network
# https://stmaria.org
# ============================================================================
# Usage:
#   tar xzf simplex-node-install-pack.tar.gz
#   cd simplex-node-install-pack
#   bash install.sh
# ============================================================================

# ─── Color helpers ──────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERR]${NC}   $*"; }

# ─── Paths ──────────────────────────────────────────────────────────────────
SIMPLEX_SRC="$(cd "$(dirname "$0")" && pwd)"
HOME_DIR="$HOME"
USER_NAME=$(whoami)
BIN_DEST="$HOME_DIR/bin"
DATA_DIR="$HOME_DIR/.local/share/simplex-node"
LOG_DIR="$DATA_DIR/logs"
SYSTEMD_USER_DIR="$HOME_DIR/.config/systemd/user"
DOCKER_DIR="$HOME_DIR/simplex-node-docker"
NODE_DIR="$HOME_DIR/simplex-node"

STEP=0
TOTAL_STEPS=9

# ─── Pre-flight ─────────────────────────────────────────────────────────────
clear
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║      simplex-node INSTALL PACK — Ubuntu / Beelink SER9      ║"
echo "║      Services only (no Isle/Royal client apps)              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Check Ubuntu
if ! grep -qi ubuntu /etc/os-release 2>/dev/null; then
  warn "Not Ubuntu — continuing anyway (Beelink SER9 recommended)"
fi

# Check x86_64
ARCH=$(uname -m)
if [ "$ARCH" != "x86_64" ]; then
  err "Expected x86_64, got $ARCH. Beelink SER9 (Intel N150) recommended."
  exit 1
fi

# Check RAM
TOTAL_RAM=$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo 2>/dev/null || echo 0)
if [ "$TOTAL_RAM" -lt 8 ]; then
  warn "Less than 8GB RAM detected (${TOTAL_RAM}GB). 16GB+ recommended for production."
fi

# Check disk
ROOT_AVAIL=$(df -m / 2>/dev/null | awk 'NR==2 {print $4}' || echo 0)
if [ "$ROOT_AVAIL" -lt 10240 ]; then
  warn "Less than 10GB free on root (${ROOT_AVAIL}MB). 128GB+ SSD recommended."
fi

# Check user
if [ "$(id -u)" = "0" ]; then
  err "Do NOT run as root. Run as normal user (will sudo when needed)."
  exit 1
fi

info "Installing as user: $USER_NAME"
info "Home: $HOME_DIR"
info "Architecture: $ARCH"
info "RAM: ${TOTAL_RAM}GB | Disk free: ${ROOT_AVAIL}MB"
echo ""

# ─── Step 1: System dependencies ────────────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Installing system dependencies..."
sudo apt-get update -qq || true
sudo apt-get install -y -qq \
  ca-certificates curl gnupg lsb-release \
  docker.io docker-compose-v2 \
  lsof net-tools jq \
  python3 python3-pip python3-venv \
  openssl wireguard-tools qrencode \
  unzip git ufw \
  || warn "Some packages failed to install (may already be present)"

# Ensure docker group
if ! groups "$USER_NAME" | grep -q docker; then
  sudo usermod -aG docker "$USER_NAME"
  warn "Added to docker group. RE-LOGIN or 'newgrp docker' required for docker access."
fi

# Start docker
sudo systemctl enable docker 2>/dev/null || true
sudo systemctl start docker 2>/dev/null || true

# Configure firewall
sudo ufw allow 8080/tcp 2>/dev/null || true
sudo ufw allow 5223/tcp 2>/dev/null || true
sudo ufw allow 5225/tcp 2>/dev/null || true
sudo ufw --force enable 2>/dev/null || true

ok "System dependencies installed"

# ─── Step 2: Create directories ────────────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Creating directories..."
mkdir -p "$BIN_DEST"
mkdir -p "$DATA_DIR"
mkdir -p "$LOG_DIR"
mkdir -p "$SYSTEMD_USER_DIR"
mkdir -p "$DOCKER_DIR"
mkdir -p "$NODE_DIR/scripts"
mkdir -p "$NODE_DIR/config"
ok "Directories created"

# ─── Step 3: Install simplex-node binary ────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Installing simplex-node binary..."
if [ -f "$SIMPLEX_SRC/bin/simplex-node" ]; then
  cp "$SIMPLEX_SRC/bin/simplex-node" "$BIN_DEST/simplex-node"
  chmod +x "$BIN_DEST/simplex-node"
  ok "Binary installed: $BIN_DEST/simplex-node ($(du -h "$BIN_DEST/simplex-node" | cut -f1))"
else
  warn "Binary not found — building from source..."
  if [ -d "$SIMPLEX_SRC/../" ] && [ -f "$SIMPLEX_SRC/../go.mod" ]; then
    cd "$SIMPLEX_SRC/../"
    go build -o "$BIN_DEST/simplex-node" ./cmd/simplex-node/ 2>/dev/null && \
      ok "Built from source: $BIN_DEST/simplex-node" || \
      err "Build failed. Install Go and build manually: go build -o ~/bin/simplex-node ./cmd/simplex-node/"
    cd "$SIMPLEX_SRC"
  else
    err "No binary and no source found. Place simplex-node binary in ./bin/ before running."
    exit 1
  fi
fi

# ─── Step 4: Install xray native proxy ──────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Installing xray (native proxy)..."
XRAY_DIR="$HOME_DIR/bin/v2ray"
mkdir -p "$XRAY_DIR"

if [ ! -f "$XRAY_DIR/xray" ]; then
  info "Downloading xray core..."
  XRAY_VER="1.8.24"
  XRAY_URL="https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VER}/Xray-linux-64.zip"
  TMP_DIR=$(mktemp -d)
  cd "$TMP_DIR"
  if curl -sL -o xray.zip "$XRAY_URL"; then
    unzip -q xray.zip && mv xray "$XRAY_DIR/" && chmod +x "$XRAY_DIR/xray"
    rm -rf "$TMP_DIR"
    ok "xray $XRAY_VER installed"
  else
    warn "Failed to download xray — continuing without proxy"
    rm -rf "$TMP_DIR"
  fi
  cd /
else
  ok "xray already installed at $XRAY_DIR/xray"
fi

# Install xray config
cp "$SIMPLEX_SRC/xray/config.json" "$XRAY_DIR/config.json" 2>/dev/null || cat > "$XRAY_DIR/config.json" << 'XRAYCONF'
{
  "log": {"loglevel": "warning"},
  "inbounds": [
    {"port": 10810, "protocol": "socks", "settings": {"auth": "no", "udp": true}, "tag": "socks"},
    {"port": 10811, "protocol": "dokodemo-door", "settings": {"network": "tcp,udp", "followRedirect": true}, "tag": "tproxy", "streamSettings": {"sockopt": {"tproxy": "redirect"}}}
  ],
  "outbounds": [
    {"protocol": "freedom", "tag": "direct"},
    {"protocol": "socks", "tag": "tor-out", "settings": {"servers": [{"address": "127.0.0.1", "port": 9050}]}}
  ],
  "routing": {"domainStrategy": "AsIs", "rules": [{"type": "field", "outboundTag": "direct", "ip": ["0.0.0.0/0", "::/0"]}]}
}
XRAYCONF
ok "xray config installed"

# ─── Step 5: Install Docker compose stack ───────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Deploying Docker compose stack..."

if [ -d "$SIMPLEX_SRC/docker" ]; then
  cp -r "$SIMPLEX_SRC/docker"/* "$DOCKER_DIR/"
  ok "Docker stack copied from pack"
else
  info "Creating minimal Docker stack..."
  mkdir -p "$DOCKER_DIR/tor/hidden_services"/{smp,xftp,dashboard,ice,auditor}
  mkdir -p "$DOCKER_DIR/coturn"
  mkdir -p "$DOCKER_DIR/smp_configs" "$DOCKER_DIR/smp_state"
  mkdir -p "$DOCKER_DIR/xftp_configs" "$DOCKER_DIR/xftp_state"

  # Create minimal docker-compose.yml
  cat > "$DOCKER_DIR/docker-compose.yml" << 'COMPOSE'
name: simplex-node
services:
  smp-server:
    image: simplexchat/smp-server:latest
    container_name: simplex-node-smp-server
    environment: [ADDR=simplex-node.local]
    volumes:
      - ./smp_configs:/etc/opt/simplex
      - ./smp_state:/var/opt/simplex
    ports: ["5223:5223", "127.0.0.1:5224:5224"]
    restart: unless-stopped
    depends_on: [tor]
  xftp-server:
    image: simplexchat/xftp-server:latest
    container_name: simplex-node-xftp-server
    environment: [ADDR=simplex-node.local, QUOTA=20gb]
    volumes:
      - ./xftp_configs:/etc/opt/simplex-xftp
      - ./xftp_state:/srv/xftp
    ports: ["5225:443", "127.0.0.1:5226:5226"]
    restart: unless-stopped
    depends_on: [tor]
  tor:
    build: ./tor
    container_name: simplex-node-tor
    restart: unless-stopped
    volumes:
      - ./tor/torrc:/etc/tor/torrc:ro
      - ./tor/hidden_services/smp:/var/lib/tor/smp
      - ./tor/hidden_services/xftp:/var/lib/tor/xftp
      - ./tor/hidden_services/dashboard:/var/lib/tor/dashboard
      - ./tor/hidden_services/ice:/var/lib/tor/ice
      - ./tor/hidden_services/auditor:/var/lib/tor/auditor
    extra_hosts: ["host.docker.internal:host-gateway"]
  coturn:
    image: coturn/coturn:latest
    container_name: simplex-node-coturn
    restart: unless-stopped
    user: "1000:1000"
    command: -c /etc/coturn/turnserver.conf --log-file=stdout
    volumes:
      - ./coturn/turnserver.conf:/etc/coturn/turnserver.conf:ro
      - ./coturn/turn_cert.pem:/etc/coturn/turn_cert.pem:ro
      - ./coturn/turn_key.pem:/etc/coturn/turn_key.pem:ro
    depends_on: [tor]
COMPOSE

  # Create minimal torrc
  cat > "$DOCKER_DIR/tor/torrc" << 'TORRC'
SocksPort 0
Log notice stdout
RunAsDaemon 0
DataDirectory /tmp/tor-data
HiddenServiceSingleHopMode 1
HiddenServiceNonAnonymousMode 1
HiddenServiceDir /var/lib/tor/dashboard
HiddenServicePort 80 host.docker.internal:8080
HiddenServiceDir /var/lib/tor/smp
HiddenServicePort 5223 smp-server:5223
HiddenServiceDir /var/lib/tor/xftp
HiddenServicePort 443 xftp-server:443
HiddenServiceDir /var/lib/tor/ice
HiddenServicePort 3478 coturn:3478
HiddenServicePort 5349 coturn:5349
HiddenServiceDir /var/lib/tor/auditor
HiddenServicePort 80 host.docker.internal:8080
TORRC

  # Create minimal Tor Dockerfile
  mkdir -p "$DOCKER_DIR/tor"
  cat > "$DOCKER_DIR/tor/Dockerfile" << 'TORDF'
FROM alpine:3.21
RUN apk add --no-cache tor su-exec
COPY torrc /etc/tor/torrc
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
TORDF

  cat > "$DOCKER_DIR/tor/entrypoint.sh" << 'TOREPS'
#!/bin/sh
set -e
PERSISTENT="/var/lib/tor"
RUNTIME="/tmp/tor-data"
TARGET_UID="${SIMPLEX_UID:-1000}"
TARGET_GID="${SIMPLEX_GID:-1000}"
mkdir -p "$PERSISTENT/smp" "$PERSISTENT/xftp" "$PERSISTENT/dashboard" "$PERSISTENT/ice" "$PERSISTENT/auditor"
chown -R "${TARGET_UID}:${TARGET_GID}" "$PERSISTENT" 2>/dev/null || true
chmod 755 "$PERSISTENT" 2>/dev/null || true
for d in smp xftp dashboard ice auditor; do
  chown -R "${TARGET_UID}:${TARGET_GID}" "$PERSISTENT/$d" 2>/dev/null || true
  chmod 700 "$PERSISTENT/$d" 2>/dev/null || true
done
chmod 600 "$PERSISTENT"/smp/hs_ed25519_secret_key "$PERSISTENT"/xftp/hs_ed25519_secret_key "$PERSISTENT"/dashboard/hs_ed25519_secret_key "$PERSISTENT"/ice/hs_ed25519_secret_key "$PERSISTENT"/auditor/hs_ed25519_secret_key 2>/dev/null || true
mkdir -p "$RUNTIME"
chown -R "${TARGET_UID}:${TARGET_GID}" "$RUNTIME" 2>/dev/null || true
chmod 700 "$RUNTIME" 2>/dev/null || true
exec su-exec "${TARGET_UID}:${TARGET_GID}" tor -f /etc/tor/torrc "$@"
TOREPS
  chmod +x "$DOCKER_DIR/tor/entrypoint.sh"

  ok "Minimal Docker stack created"
fi

# Generate TURN secret
ICE_SECRET_FILE="$DATA_DIR/ice_turn_secret.txt"
if [ ! -f "$ICE_SECRET_FILE" ]; then
  openssl rand -base64 32 | tr -d '\n' | head -c 40 > "$ICE_SECRET_FILE"
  chmod 600 "$ICE_SECRET_FILE"
  ok "TURN secret generated"
fi

# Start Docker stack
info "Starting Docker containers..."
cd "$DOCKER_DIR"
docker compose pull 2>/dev/null || true
docker compose build tor 2>/dev/null || true
docker compose up -d 2>&1 | head -5 || warn "Docker compose start had issues"
cd /
ok "Docker stack deployed"

# ─── Step 6: Install systemd services ──────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Installing systemd services..."

# Copy from pack if available
if [ -d "$SIMPLEX_SRC/systemd" ]; then
  # Template: replace /home/tomas with actual home
  for f in "$SIMPLEX_SRC/systemd"/*.service; do
    if [ -f "$f" ]; then
      BASENAME=$(basename "$f")
      sed "s|/home/tomas|$HOME_DIR|g; s|User=tomas|User=$USER_NAME|g; s|Group=tomas|Group=$USER_NAME|g" "$f" > "$SYSTEMD_USER_DIR/$BASENAME" 2>/dev/null || \
      sed "s|/home/tomas|$HOME_DIR|g" "$f" > "$SYSTEMD_USER_DIR/$BASENAME" 2>/dev/null
    fi
  done
  # Copy target file
  if [ -f "$SIMPLEX_SRC/systemd/simplex-node.target" ]; then
    cp "$SIMPLEX_SRC/systemd/simplex-node.target" "$SYSTEMD_USER_DIR/"
  fi
  ok "Systemd services copied from pack"
else
  # Create minimal dashboard service
  cat > "$SYSTEMD_USER_DIR/simplex-node-dashboard.service" << 'SVC1'
[Unit]
Description=simplex-node server (Go HTTP :8080)
After=network-online.target docker.service
Wants=network-online.target docker.service
PartOf=simplex-node.target
[Service]
Type=simple
User=%u
WorkingDirectory=%h/simplex-node
Environment=HOME=%h
Environment=USER=%u
ExecStartPre=/bin/bash -c 'pkill -x simplex-node 2>/dev/null; fuser -k 8080/tcp 2>/dev/null; exit 0'
ExecStart=%h/bin/simplex-node -listen 0.0.0.0:8080 -data %h/.local/share/simplex-node
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
PrivateTmp=true
ProtectSystem=strict
ProtectHome=false
NoNewPrivileges=true
ReadWritePaths=%h
[Install]
WantedBy=simplex-node.target
SVC1

  cat > "$SYSTEMD_USER_DIR/simplex-node.target" << 'TGT'
[Unit]
Description=simplex-node — full server stack
Documentation=https://stmaria.org
[Install]
WantedBy=default.target
TGT
  ok "Minimal systemd services created"
fi

systemctl --user daemon-reload
ok "Systemd services installed"

# ─── Step 7: Install scripts ────────────────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Installing scripts and configs..."

# Copy scripts from pack
if [ -d "$SIMPLEX_SRC/scripts" ]; then
  for f in "$SIMPLEX_SRC/scripts"/*.sh; do
    if [ -f "$f" ]; then
      BASENAME=$(basename "$f")
      sed "s|/home/tomas|$HOME_DIR|g; s|tomas|$USER_NAME|g" "$f" > "$NODE_DIR/scripts/$BASENAME" 2>/dev/null || \
      cp "$f" "$NODE_DIR/scripts/$BASENAME" 2>/dev/null
    fi
  done
  chmod +x "$NODE_DIR/scripts"/*.sh 2>/dev/null || true
  ok "Scripts installed"
fi

# Create launch-node.sh
cat > "$NODE_DIR/scripts/launch-node.sh" << 'LAUNCH'
#!/bin/bash
# simplex-node launcher — always use this for restarts
set -euo pipefail
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
mkdir -p "$DATA_DIR"
touch -t "$(date -d '+30 minutes' '+%Y%m%d%H%M.%S')" "$DATA_DIR/.maintenance" 2>/dev/null || touch "$DATA_DIR/.maintenance"

export NO_PROXY="localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.local,.onion,api.telegram.org"
export no_proxy="$NO_PROXY"

# Start native xray proxy
XRAY_BIN="$HOME/bin/v2ray/xray"
XRAY_CONFIG="$HOME/bin/v2ray/config.json"
if [ -x "$XRAY_BIN" ] && [ -f "$XRAY_CONFIG" ]; then
  if ! pgrep -x xray >/dev/null 2>&1; then
    nohup "$XRAY_BIN" run -c "$XRAY_CONFIG" > "$DATA_DIR/logs/xray.log" 2>&1 &
    echo "xray started (PID $!)"
    sleep 1
    ss -tlnp 2>/dev/null | grep -q :10810 && echo "xray SOCKS5 ✓ :10810" || echo "xray NOT on :10810"
  fi
fi

# Kill any existing node process
pkill -x simplex-node 2>/dev/null || true
sleep 0.5
fuser -k 8080/tcp 2>/dev/null || true

echo "Starting simplex-node on :8080..."
exec "$HOME/bin/simplex-node" -listen 0.0.0.0:8080 -data "$DATA_DIR"
LAUNCH
chmod +x "$NODE_DIR/scripts/launch-node.sh"
ok "launch-node.sh created"

# Copy node-monitor
if [ -f "$SIMPLEX_SRC/node-monitor.py" ]; then
  cp "$SIMPLEX_SRC/node-monitor.py" "$NODE_DIR/node-monitor.py"
  chmod +x "$NODE_DIR/node-monitor.py"
  ok "node-monitor.py installed"

  # Install monitor systemd service
  cat > "$SYSTEMD_USER_DIR/simplex-node-monitor.service" << 'MON'
[Unit]
Description=simplex-node monitor — auto-heal daemon
After=network-online.target simplex-node-dashboard.service
PartOf=simplex-node.target
[Service]
Type=simple
User=%u
Environment=HOME=%h
Environment=USER=%u
ExecStartPre=/bin/bash -c 'pkill -f "python3.*node-monitor" 2>/dev/null; exit 0'
ExecStart=%h/simplex-node/node-monitor.py
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
[Install]
WantedBy=simplex-node.target
MON
  systemctl --user daemon-reload
fi

# ─── Step 8: Create default config ──────────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Creating default configuration..."

cat > "$DATA_DIR/simplex-node.json" << 'CONFIG'
{
  "node_name": "simplex-node",
  "listen": "0.0.0.0:8080",
  "data_dir": "~/.local/share/simplex-node",
  "bridge_port": 17225,
  "p2p_port": 17001,
  "log_level": "info",
  "auto_accept_contacts": true,
  "features": {
    "bridge": true,
    "chat": true,
    "economy": true,
    "radio": true,
    "dc_cloud": true,
    "paranoidx": true,
    "transport_v1": true
  }
}
CONFIG

# Create environment file
cat > "$NODE_DIR/.env" << ENVEOF
# simplex-node environment — source this to set paths
export SIMPLEX_NODE_HOME="$HOME_DIR"
export SIMPLEX_NODE_DATA="$DATA_DIR"
export SIMPLEX_NODE_BIN="$BIN_DEST/simplex-node"
export SIMPLEX_NODE_SCRIPTS="$NODE_DIR/scripts"
export PATH="\$PATH:$BIN_DEST"
ENVEOF

# Create Python virtual environment
if command -v python3 >/dev/null 2>&1; then
  python3 -m venv "$NODE_DIR/.venv" 2>/dev/null || true
fi

ok "Default configuration created"

# ─── Step 9: Start services and verify ─────────────────────────────────────
STEP=$((STEP + 1))
info "Step $STEP/$TOTAL_STEPS: Starting services and verifying..."

# Enable services
systemctl --user enable simplex-node-dashboard.service 2>/dev/null || true
systemctl --user enable simplex-node-monitor.service 2>/dev/null || true
systemctl --user enable simplex-node.target 2>/dev/null || true

# Start dashboard
systemctl --user start simplex-node-dashboard.service 2>&1 | head -3 || warn "Service start had issues"
sleep 5

# Verify
HEALTH_OK=false
for i in {1..6}; do
  if curl -sf --max-time 3 http://127.0.0.1:8080/api/health > /dev/null 2>&1; then
    HEALTH_OK=true
    break
  fi
  sleep 3
done

if [ "$HEALTH_OK" = true ]; then
  ok "simplex-node is RUNNING on http://127.0.0.1:8080"
  # Show health info
  curl -s http://127.0.0.1:8080/api/health | python3 -m json.tool 2>/dev/null || true
else
  warn "simplex-node not responding yet — check logs:"
  journalctl --user -u simplex-node-dashboard.service --no-pager -n 20 2>/dev/null | tail -10
fi

# ─── Summary ────────────────────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║       simplex-node INSTALLATION COMPLETE                    ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""
echo "  Node name:    simplex-node (services only)"
echo "  API:          http://127.0.0.1:8080"
echo "  Dashboard:    http://127.0.0.1:8080"
echo "  SimpleX SMP:  :5223"
echo "  SimpleX XFTP: :5225"
echo "  P2P:          :17001 (TCP)"
echo "  Bridge:       :17225 (WS to simplex-chat CLI)"
echo "  Proxy SOCKS:  :10810 (xray), :9050 (Tor)"
echo ""
echo "  Binary:       $BIN_DEST/simplex-node"
echo "  Data:         $DATA_DIR"
echo "  Config:       $DATA_DIR/simplex-node.json"
echo "  Docker:       $DOCKER_DIR"
echo "  Scripts:      $NODE_DIR/scripts/"
echo "  Logs:         $LOG_DIR"
echo ""
echo "  Commands:"
echo "    systemctl --user status simplex-node-dashboard"
echo "    journalctl --user -u simplex-node-dashboard -f"
echo "    curl -s http://127.0.0.1:8080/api/health"
echo "    ~/simplex-node/scripts/launch-node.sh"
echo ""
echo "  Verify:"
echo "    curl -s http://127.0.0.1:8080/api/version"
echo "    curl -s http://127.0.0.1:8080/api/status"
echo ""
echo "  Saint Mary Liberty Island — Sovereign Network"
echo "  Beelink SER9 | Ubuntu | simplex-node"
echo "============================================================================"
