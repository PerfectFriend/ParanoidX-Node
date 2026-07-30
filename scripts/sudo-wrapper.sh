#!/bin/bash
# simplex-node SUDO WRAPPER
# Use this whenever you need to run simplex-node operations as root.
# Common operations: kill root-owned processes, free ports, fix ownership, restart.
#
# Usage:
#   sudo bash scripts/sudo-wrapper.sh kill
#   sudo bash scripts/sudo-wrapper.sh fix-ownership
#   sudo bash scripts/sudo-wrapper.sh full-restart
#   sudo bash scripts/sudo-wrapper.sh free-ports
#   sudo bash scripts/sudo-wrapper.sh status

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(dirname "$SCRIPT_DIR")"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
TARGET_USER="${SUDO_USER:-tomas}"
TARGET_GROUP="${SUDO_GID:-$(id -g "$TARGET_USER")}"

cmd="${1:-help}"

case "$cmd" in
  kill)
    echo "Stopping all simplex-node and simplex-chat-island processes..."
    pkill -f ParanoidX -listen" 2>/dev/null || true
    pkill -f "simplex-chat-island" 2>/dev/null || true
    sleep 1
    echo "Remaining:"
    pgrep -a -f simplex-node 2>/dev/null || echo "  no simplex-node"
    pgrep -a -f simplex-chat-island 2>/dev/null || echo "  no simplex-chat-island"
    ;;

  free-ports)
    echo "Freeing ports 8080, 5223, 5224, 5225, 5226, 5230..."
    fuser -k 8080/tcp 2>/dev/null || true
    fuser -k 5223/tcp 2>/dev/null || true
    fuser -k 5224/tcp 2>/dev/null || true
    fuser -k 5225/tcp 2>/dev/null || true
    fuser -k 5226/tcp 2>/dev/null || true
    fuser -k 5230/tcp 2>/dev/null || true
    sleep 1
    echo "Port status:"
    for p in 8080 5223 5224 5225 5226 5230; do
      ss -tnp | grep -q ":$p " && echo "  $p: OCCUPIED" || echo "  $p: free"
    done
    ;;

  fix-ownership)
    echo "Fixing ownership of simplex-node dirs to $TARGET_USER:$TARGET_GROUP..."
    chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR" "$SRC_DIR/docker" 2>/dev/null || true
    for d in "$HOME"/.local/share/simplex-node-A*; do
      [ -d "$d" ] && chown -R "$TARGET_USER:$TARGET_GROUP" "$d" 2>/dev/null || true
    done
    mkdir -p "$DATA_DIR/vault" "$DATA_DIR/logs" "$DATA_DIR/island-bot" "$HOME/bin"
    chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR/vault" "$DATA_DIR/logs" "$DATA_DIR/island-bot" "$HOME/bin" 2>/dev/null || true
    echo "Ownership fixed."
    ;;

  rebuild)
    echo "Rebuilding Go binary as $TARGET_USER..."
    if command -v go >/dev/null 2>&1; then
      su -s /bin/bash -c "cd $SRC_DIR && go build -o $HOME/bin/ParanoidX ./cmd/ParanoidX" "$TARGET_USER"
      echo "Build OK: $HOME/bin/ParanoidX"
    else
      echo "ERROR: go not found"
      exit 1
    fi
    ;;

  restart)
    echo "Restarting node as $TARGET_USER..."
    if [ -x "$SCRIPT_DIR/launch-node.sh" ]; then
      su -s /bin/bash -c "$SCRIPT_DIR/launch-node.sh" "$TARGET_USER"
      echo "Launch finished."
    else
      echo "ERROR: launch-node.sh not found"
      exit 1
    fi
    ;;

  full-restart)
    echo "=== FULL RESTART (kill + free-ports + fix-ownership + rebuild + restart) ==="
    "$0" kill
    "$0" free-ports
    "$0" fix-ownership
    "$0" rebuild
    "$0" restart
    echo ""
    echo "=== Verify ==="
    curl -s http://127.0.0.1:8080/api/status | python3 -m json.tool
    ss -tnp | grep 5230
    pgrep -a -f simplex-chat-island
    ;;

  status)
    echo "=== simplex-node STATUS ==="
    echo ""
    echo "Processes:"
    pgrep -a -f simplex-node 2>/dev/null || echo "  simplex-node: not running"
    pgrep -a -f simplex-chat-island 2>/dev/null || echo "  simplex-chat-island: not running"
    echo ""
    echo "Ports:"
    for p in 8080 5223 5224 5225 5226 5230; do
      ss -tnp | grep -q ":$p " && echo "  $p: OCCUPIED" || echo "  $p: free"
    done
    echo ""
    echo "Node API:"
    curl -s http://127.0.0.1:8080/api/status | python3 -m json.tool 2>/dev/null || echo "  API not responding"
    ;;

  help|*)
    echo "simplex-node SUDO WRAPPER"
    echo ""
    echo "Usage: sudo bash $0 <command>"
    echo ""
    echo "Commands:"
    echo "  kill            - Stop all simplex-node and CLI processes"
    echo "  free-ports      - Kill anything on ports 8080/5223-5226/5230"
    echo "  fix-ownership   - chown data + docker dirs to $TARGET_USER"
    echo "  rebuild         - go build -o ~/bin/ParanoidX ./cmd/ParanoidX"
    echo "  restart         - Run launch-node.sh as $TARGET_USER"
    echo "  full-restart    - kill + free-ports + fix-ownership + rebuild + restart"
    echo "  status          - Show processes, ports, and API status"
    echo ""
    echo "Example:"
    echo "  sudo bash $0 full-restart"
    ;;
esac
