#!/bin/bash
# Install The Island Linux Desktop App
# Run: sudo ./install.sh
# Works from the release tarball or source tree

set -euo pipefail

APP_NAME="the-island"
APP_TITLE="The Island"
BINARY="isle_app"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/opt/$APP_NAME"
ICON_DIR="/usr/share/icons/hicolor/512x512/apps"
DESKTOP_DIR="/usr/share/applications"

if [ "$EUID" -ne 0 ]; then
  echo "Please run with sudo"
  echo "  sudo ./install.sh"
  exit 1
fi

# Look for binary in build directory (source tree) or script directory (release tarball)
BUILD_DIR="$SCRIPT_DIR/build/linux/x64/release/bundle"
if [ -f "$BUILD_DIR/$BINARY" ]; then
  BUNDLE_SRC="$BUILD_DIR"
elif [ -f "$SCRIPT_DIR/$BINARY" ]; then
  BUNDLE_SRC="$SCRIPT_DIR"
else
  echo "Error: $BINARY not found."
  echo "  Checked: $BUILD_DIR"
  echo "  Checked: $SCRIPT_DIR"
  echo ""
  echo "Run 'flutter build linux --release' first, or unpack a release tarball."
  exit 1
fi

echo "Installing $APP_TITLE..."

# Copy bundle
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cp -a "$BUNDLE_SRC"/* "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/$BINARY"

# Copy icon
mkdir -p "$ICON_DIR"
if [ -f "$SCRIPT_DIR/icons/$APP_NAME.png" ]; then
  cp "$SCRIPT_DIR/icons/$APP_NAME.png" "$ICON_DIR/$APP_NAME.png"
elif [ -f "$SCRIPT_DIR/icons/$APP_NAME.svg" ]; then
  cp "$SCRIPT_DIR/icons/$APP_NAME.svg" "$ICON_DIR/$APP_NAME.svg"
elif [ -f "$SCRIPT_DIR/linux/assets/icon.png" ]; then
  cp "$SCRIPT_DIR/linux/assets/icon.png" "$ICON_DIR/$APP_NAME.png"
elif [ -f "$SCRIPT_DIR/linux/assets/icon.svg" ]; then
  cp "$SCRIPT_DIR/linux/assets/icon.svg" "$ICON_DIR/$APP_NAME.svg"
elif [ -f "$BUNDLE_SRC/../icon.png" ]; then
  cp "$BUNDLE_SRC/../icon.png" "$ICON_DIR/$APP_NAME.png"
elif [ -f "$BUNDLE_SRC/../icon.svg" ]; then
  cp "$BUNDLE_SRC/../icon.svg" "$ICON_DIR/$APP_NAME.svg"
fi

# Create .desktop file
mkdir -p "$DESKTOP_DIR"
if [ -f "$BUNDLE_SRC/../$APP_NAME.desktop" ]; then
  cp "$BUNDLE_SRC/../$APP_NAME.desktop" "$DESKTOP_DIR/$APP_NAME.desktop"
elif [ -f "$SCRIPT_DIR/$APP_NAME.desktop" ]; then
  cp "$SCRIPT_DIR/$APP_NAME.desktop" "$DESKTOP_DIR/$APP_NAME.desktop"
else
  cat > "$DESKTOP_DIR/$APP_NAME.desktop" << DESKTOP_EOF
[Desktop Entry]
Type=Application
Name=$APP_TITLE
Comment=Saint Mary Liberty Island — Silver-Backed Private Sovereign Network
Exec=$INSTALL_DIR/$BINARY %F
Icon=/home/tomas/.local/share/icons/hicolor/512x512/apps/$APP_NAME.png
Terminal=false
Categories=Finance;Office;
StartupNotify=true
Keywords=silver;crypto;wallet;payment;
DESKTOP_EOF
fi

# Update icon cache
if command -v gtk-update-icon-cache &>/dev/null; then
  gtk-update-icon-cache /usr/share/icons/hicolor/ 2>/dev/null || true
fi

echo ""
echo "✅ $APP_TITLE installed successfully!"
echo ""
echo "Launch from application menu, or run:"
echo "  $INSTALL_DIR/$BINARY --server http://YOUR_SERVER:8080 --pubkey YOUR_PUBKEY"
echo ""
echo "Keyboard shortcuts:"
echo "  Ctrl+1-6  Switch tabs"
echo "  Ctrl+R    Reconnect"
echo "  Ctrl+Q    Quit"
echo "  Ctrl+,    Settings"
