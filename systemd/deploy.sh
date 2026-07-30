#!/bin/bash
# deploy.sh — Deploy new simplex-node and simplex-bridge binaries, restart services
# Run: sudo bash deploy.sh

set -euo pipefail

ME="tomas"
PROJECT_DIR="/home/$ME/simplex-node"
BIN_DIR="/home/$ME/bin"

echo "=== Deploying simplex-node binaries ==="

# 1. Stop services that depend on the binaries
echo "--- Stopping services ---"
for s in simplex-node-dashboard.service simplex-node-bridge.service; do
  if systemctl is-active "$s" &>/dev/null 2>&1; then
    systemctl stop "$s"
    echo "  stopped $s"
  fi
done
sleep 2

# 2. Build fresh binaries (as user)
echo "--- Building binaries ---"
sudo -u "$ME" bash -c "cd '$PROJECT_DIR' && go build -o simplex-node ./cmd/simplex-node/ && go build -o simplex-bridge ./cmd/simplex-bridge/ && echo '  build done'"

# 3. Copy to ~/bin
echo "--- Deploying to $BIN_DIR ---"
cp "$PROJECT_DIR/simplex-node" "$BIN_DIR/simplex-node"
cp "$PROJECT_DIR/simplex-bridge" "$BIN_DIR/simplex-bridge"
chown "$ME:$ME" "$BIN_DIR/simplex-node" "$BIN_DIR/simplex-bridge"
chmod 755 "$BIN_DIR/simplex-node" "$BIN_DIR/simplex-bridge"
echo "  deployed: $(du -h "$BIN_DIR/simplex-node" | cut -f1) + $(du -h "$BIN_DIR/simplex-bridge" | cut -f1)"

# 4. Ensure bridge service file exists (new service added in this refactor)
SYS_FILE="/etc/systemd/system/simplex-node-bridge.service"
if [ ! -f "$SYS_FILE" ]; then
  echo "--- Installing bridge service ---"
  cp "$PROJECT_DIR/systemd/simplex-node-bridge.service" "$SYS_FILE"
  systemctl daemon-reload
  systemctl enable simplex-node-bridge.service
  echo "  installed"
fi

# 5. Restart services
echo "--- Starting services ---"
systemctl start simplex-node-dashboard.service
sleep 3
systemctl start simplex-node-bridge.service

# 6. Verify
echo "--- Verification ---"
echo ""
echo "=== Dashboard ==="
curl -s --max-time 5 http://127.0.0.1:8080/api/status | python3 -m json.tool 2>/dev/null | head -5 || echo "  FAILED"
echo ""
echo "=== Health ==="
curl -s --max-time 5 http://127.0.0.1:8080/api/health | python3 -c "
import sys,json
d=json.load(sys.stdin)
print(f'Healthy: {d[\"healthy\"]}  Checks: {len(d[\"checks\"])}  Uptime: {d[\"uptime\"]}')
" 2>/dev/null || echo "  FAILED"
echo ""
echo "=== Service status ==="
for s in simplex-node-dashboard.service simplex-node-bridge.service simplex-node-admin-bot.service simplex-node-royal-bot.service; do
  status=$(systemctl is-active "$s" 2>/dev/null || echo "not found")
  echo "  $s: $status"
done

echo ""
echo "=== Deploy complete ==="
echo "New binaries:"
echo "  simplex-node   $(du -h "$BIN_DIR/simplex-node" | cut -f1)  (was 10.6 MB)"
echo "  simplex-bridge $(du -h "$BIN_DIR/simplex-bridge" | cut -f1)  (new service)"
echo ""
echo "To roll back:"
echo "  cp ~/simplex-node/simplex-node.bak.refactor ~/bin/simplex-node  # not yet deployed"
