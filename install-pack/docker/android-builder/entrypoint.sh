#!/bin/bash
# Entrypoint for Android APK builder
# Builds the isle_app APK inside Docker container

set -e

cd /app/isle_app

echo "=== Running flutter pub get ==="
flutter pub get

echo "=== Running flutter analyze ==="
flutter analyze || true

echo "=== Building Android APK ==="
flutter build apk --release

echo "=== APK built successfully ==="
ls -lh build/app/outputs/flutter-apk/app-release.apk

# Copy to output
mkdir -p /app/build-output
cp build/app/outputs/flutter-apk/app-release.apk /app/build-output/isle-app-release.apk

echo "=== Output: /app/build-output/isle-app-release.apk ==="
