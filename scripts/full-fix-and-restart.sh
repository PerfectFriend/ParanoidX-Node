#!/bin/bash
# ParanoidX FULL FIX + RESTART
# Run: sudo bash scripts/full-fix-and-restart.sh
# Stops root-owned processes, fixes ownership, rebuilds, restarts as tomas.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(dirname "$SCRIPT_DIR")"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
BIN_DIR="$HOME/bin"
BIN="$BIN_DIR/ParanoidX"
TARGET_USER="${SUDO_USER:-tomas}"
TARGET_GROUP="${SUDO_GID:-$(id -g "$TARGET_USER")}"

echo "=== ParanoidX FULL FIX ==="
echo "Running as root, will drop to $TARGET_USER for build/launch."
echo "Data: $DATA_DIR"
echo "Src:   $SRC_DIR"

# 1. Stop root-owned node and CLI
echo ""
echo "[1/6] Stopping root-owned processes..."
pkill -f ParanoidX -listen" 2>/dev/null || true
pkill -f "simplex-chat-island" 2>/dev/null || true
sleep 1
pgrep -a -f simplex-node 2>/dev/null && echo "WARN: simplex-node still running" || echo "Node stopped."
pgrep -a -f simplex-chat-island 2>/dev/null && echo "WARN: CLI still running" || echo "CLI stopped."

# 2. Free port 5230 if stuck
echo ""
echo "[2/6] Cleaning port 5230..."
fuser -k 5230/tcp 2>/dev/null || true
sleep 0.5
ss -tnp | grep 5230 && echo "WARN: port still in use" || echo "Port 5230 free."

# 3. Fix ownership
echo ""
echo "[3/6] Fixing ownership..."
chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR" "$SRC_DIR/docker" 2>/dev/null || true
for d in "$HOME"/.local/share/simplex-node-A*; do
  [ -d "$d" ] && chown -R "$TARGET_USER:$TARGET_GROUP" "$d" 2>/dev/null || true
done
echo "Ownership fixed."

# 4. Ensure dirs exist
echo ""
echo "[4/6] Ensuring directories..."
mkdir -p "$DATA_DIR/vault" "$DATA_DIR/logs" "$DATA_DIR/island-bot" "$BIN_DIR"
chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR/vault" "$DATA_DIR/logs" "$DATA_DIR/island-bot" "$BIN_DIR" 2>/dev/null || true
echo "Directories ready."

# 5. Rebuild binary as tomas
echo ""
echo "[5/6] Rebuilding Go binary..."
if command -v go >/dev/null 2>&1; then
  su -s /bin/bash -c "cd $SRC_DIR && go build -o $BIN ./cmd/ParanoidX" "$TARGET_USER"
  echo "Build OK: $BIN"
else
  echo "ERROR: go not found"
  exit 1
fi

# 6. Restart node via canonical launcher as tomas
echo ""
echo "[6/6] Restarting node..."
if [ -x "$SCRIPT_DIR/launch-node.sh" ]; then
  su -s /bin/bash -c "$SCRIPT_DIR/launch-node.sh" "$TARGET_USER"
  echo "Launch finished."
else
  echo "ERROR: launch-node.sh not found"
  exit 1
fi

echo ""
echo "=== FIX COMPLETE ==="
echo "Verify:"
echo "  curl -s http://127.0.0.1:8080/api/status | python3 -m json.tool"
echo "  ss -tnp | grep 5230"
echo "  pgrep -a -f simplex-chat-island"
