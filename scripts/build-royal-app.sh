#!/bin/bash
# Build Royal Flutter Client on Linux
# Run from ParanoidX directory

set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/apps/royal_app"

echo "=== Royal App Build (C1-C20) ==="

# Unset proxy for flutter (Tor proxy breaks pub get)
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

echo "[1/3] Pub get..."
flutter pub get

echo "[2/3] Analyze..."
flutter analyze

echo "[3/3] Build web..."
flutter build web --no-sound-null-safety

echo "=== Build successful ==="
echo "Output: build/web/"
