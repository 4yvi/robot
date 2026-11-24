#!/usr/bin/env bash
# Usage: make-app.sh <binary> <version> <arch>
# Builds dist/app/<arch>/Robot.app around the compiled binary.
set -euo pipefail
BIN="$1"; VERSION="${2:-0.0.0}"; ARCH="${3:-$(uname -m)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="$ROOT/dist/app/$ARCH/Robot.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/robot"
sed "s/__VERSION__/$VERSION/g" "$ROOT/packaging/macos/Info.plist" > "$APP/Contents/Info.plist"
cp "$ROOT/prompt.md" "$ROOT/config.example.yaml" "$APP/Contents/Resources/"
# Ad-hoc sign so macOS keeps the microphone permission for the bundle.
codesign --force --deep --sign - "$APP" >/dev/null 2>&1 || true
echo "$APP"
