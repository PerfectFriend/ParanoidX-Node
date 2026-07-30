#!/bin/bash
# ParanoidX FIX SCRIPT v2
# Fixes permissions, then rebuilds and restarts as the correct user.
#
# Run the WHOLE script as root/sudo:
#   sudo bash scripts/fix-permissions-and-restart.sh
#
# OR run only the chown part with sudo, then the rest normally:
#   sudo bash scripts/fix-permissions-and-restart.sh --chown-only
#   bash scripts/fix-permissions-and-restart.sh --build-only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(dirname "$SCRIPT_DIR")"
DATA_DIR="${DATA_DIR:-$HOME/.local/share/simplex-node}"
BIN_DIR="$HOME/bin"
BIN="$BIN_DIR/ParanoidX"

# Detect target user (the one who should own the files)
TARGET_USER="${SUDO_USER:-$USER}"
TARGET_GROUP="${SUDO_GID:-$(id -g "$TARGET_USER")}"

echo "=== ParanoidX FIX v2 ==="
echo "Running as: $(whoami)"
echo "Target user: $TARGET_USER"
echo "Data dir: $DATA_DIR"
echo "Source:   $SRC_DIR"
echo "Binary:   $BIN"

# If running as root, drop privileges for non-sudo steps
if [ "$(id -u)" -eq 0 ]; then
  echo ""
  echo "[INFO] Running as root. Will drop to $TARGET_USER for build/launch."
  RUN_AS_USER="su -s /bin/bash -c '$SCRIPT_DIR/launch-node.sh' '$TARGET_USER'"
  BUILD_AS_USER="su -s /bin/bash -c 'cd $SRC_DIR && go build -o $BIN ./cmd/ParanoidX' '$TARGET_USER'"
else
  RUN_AS_USER="bash $SCRIPT_DIR/launch-node.sh"
  BUILD_AS_USER="cd $SRC_DIR && go build -o $BIN ./cmd/ParanoidX"
fi

MODE="${1:-full}"

case "$MODE" in
  --chown-only)
    echo ""
    echo "[1/1] Fixing ownership (sudo required)..."
    chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR" "$SRC_DIR/docker" 2>/dev/null || true
    for d in "$HOME"/.local/share/simplex-node-A*; do
      [ -d "$d" ] && chown -R "$TARGET_USER:$TARGET_GROUP" "$d" 2>/dev/null || true
    done
    echo "Ownership fixed."
    echo ""
    echo "Now run WITHOUT sudo:"
    echo "  bash $0 --build-only"
    exit 0
    ;;

  --build-only)
    echo ""
    echo "[1/2] Rebuilding Go binary as $TARGET_USER..."
    eval "$BUILD_AS_USER"
    echo "Build OK."
    echo ""
    echo "[2/2] Restarting node as $TARGET_USER..."
    eval "$RUN_AS_USER"
    echo "Launch finished."
    ;;

  full|*)
    echo ""
    echo "[1/4] Fixing ownership..."
    chown -R "$TARGET_USER:$TARGET_GROUP" "$DATA_DIR" "$SRC_DIR/docker" 2>/dev/null || true
    for d in "$HOME"/.local/share/simplex-node-A*; do
      [ -d "$d" ] && chown -R "$TARGET_USER:$TARGET_GROUP" "$d" 2>/dev/null || true
    done
    echo "Ownership fixed."

    echo ""
    echo "[2/4] Ensuring directories..."
    mkdir -p "$DATA_DIR/vault" "$DATA_DIR/logs" "$DATA_DIR/island-bot" "$BIN_DIR"
    echo "Directories ready."

    echo ""
    echo "[3/4] Rebuilding Go binary as $TARGET_USER..."
    eval "$BUILD_AS_USER"
    echo "Build OK."

    echo ""
    echo "[4/4] Restarting node as $TARGET_USER..."
    eval "$RUN_AS_USER"
    echo "Launch finished."
    ;;
