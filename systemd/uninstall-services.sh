#!/bin/bash
# uninstall-services.sh — Stop and remove all simplex-node systemd services
# Run: sudo bash uninstall-services.sh

set -euo pipefail

echo "=== Uninstalling simplex-node systemd services ==="

# Stop target and all services
systemctl stop simplex-node.target 2>/dev/null || true
for s in simplex-node.target simplex-node-dashboard.service simplex-node-bridge.service simplex-node-admin-bot.service simplex-node-royal-bot.service simplex-node-poet-bot.service simplex-node-post-reboot-restore.service; do
  systemctl disable "$s" 2>/dev/null || true
  rm -f "/etc/systemd/system/$s"
done

systemctl daemon-reload

echo "=== Done ==="
echo "All simplex-node services stopped and removed."
echo ""
echo "To also stop Docker containers:"
echo "  cd ~/simplex-node/docker && docker compose down"
