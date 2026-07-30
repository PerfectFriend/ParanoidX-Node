#!/bin/bash
# install-services.sh — Install and start simplex-node systemd services
# Run: sudo bash install-services.sh

set -euo pipefail

SYS_DIR="/home/tomas/simplex-node/systemd"

echo "=== Installing simplex-node systemd services ==="

# Disable old broken services if they exist
for s in simplex-node.service simplex-node-dashboard.service simplex-node-bridge.service; do
  if systemctl is-enabled "$s" &>/dev/null 2>&1; then
    systemctl disable "$s" 2>/dev/null || true
  fi
done

# Remove old service files
rm -f /etc/systemd/system/simplex-node.service
rm -f /etc/systemd/system/simplex-node-dashboard.service
rm -f /etc/systemd/system/simplex-node-bridge.service

# Copy new service files
cp "$SYS_DIR/simplex-node.target" /etc/systemd/system/
cp "$SYS_DIR/simplex-node-dashboard.service" /etc/systemd/system/
cp "$SYS_DIR/simplex-node-bridge.service" /etc/systemd/system/
cp "$SYS_DIR/simplex-node-admin-bot.service" /etc/systemd/system/
cp "$SYS_DIR/simplex-node-royal-bot.service" /etc/systemd/system/

# Optional: poet bot (enable if token configured)
if [ -f /home/tomas/.config/poet-bot.token ]; then
  cp "$SYS_DIR/simplex-node-poet-bot.service" /etc/systemd/system/
fi

# Reload systemd
systemctl daemon-reload

# Enable target (will auto-enable all part-of services)
systemctl enable simplex-node.target

# Enable individual services
systemctl enable simplex-node-dashboard.service
systemctl enable simplex-node-bridge.service
systemctl enable simplex-node-admin-bot.service
systemctl enable simplex-node-royal-bot.service

if [ -f /home/tomas/.config/poet-bot.token ]; then
  systemctl enable simplex-node-poet-bot.service
fi

echo "=== Starting services ==="
systemctl start simplex-node-dashboard.service
sleep 3
systemctl start simplex-node-bridge.service
sleep 2
systemctl start simplex-node-admin-bot.service
systemctl start simplex-node-royal-bot.service

if [ -f /home/tomas/.config/poet-bot.token ]; then
  systemctl start simplex-node-poet-bot.service
fi

echo "=== Status ==="
systemctl status simplex-node-dashboard.service --no-pager 2>&1 | head -12
echo "..."
systemctl status simplex-node-bridge.service --no-pager 2>&1 | head -8
echo "..."
systemctl status simplex-node-admin-bot.service --no-pager 2>&1 | head -8
echo "..."
systemctl status simplex-node-royal-bot.service --no-pager 2>&1 | head -8

echo ""
echo "=== Checking dashboard ==="
sleep 2
curl -s --max-time 5 http://127.0.0.1:8080/api/status | python3 -m json.tool 2>/dev/null || echo "Dashboard not yet responding"

echo ""
echo "=== Checking health ==="
curl -s --max-time 5 http://127.0.0.1:8080/api/health | python3 -m json.tool 2>/dev/null || echo "Health not yet responding"

echo ""
echo "=== Done ==="
echo "All services will auto-start on boot."
echo "Individual services:"
echo "  systemctl restart simplex-node-dashboard.service"
echo "  systemctl restart simplex-node-bridge.service"
echo "  systemctl restart simplex-node-admin-bot.service"
echo "  systemctl restart simplex-node-royal-bot.service"
echo "  systemctl start simplex-node.target       # start all"
