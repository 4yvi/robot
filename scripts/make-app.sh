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
# Bundle libvosk next to the executable so the app runs without extra setup.
LIBVOSK="${LIBVOSK:-$ROOT/third_party/vosk/libvosk.dylib}"
if [ -f "$LIBVOSK" ]; then
  mkdir -p "$APP/Contents/Frameworks"
  cp "$LIBVOSK" "$APP/Contents/Frameworks/libvosk.dylib"
  install_name_tool -id "@rpath/libvosk.dylib" "$APP/Contents/Frameworks/libvosk.dylib" 2>/dev/null || true
  OLD="$(otool -L "$APP/Contents/MacOS/robot" | awk '/libvosk/ {print $1}')"
  [ -n "$OLD" ] && install_name_tool -change "$OLD" "@rpath/libvosk.dylib" "$APP/Contents/MacOS/robot"
  install_name_tool -add_rpath "@executable_path/../Frameworks" "$APP/Contents/MacOS/robot" 2>/dev/null || true
fi
# Ad-hoc sign so macOS keeps the microphone permission for the bundle.
codesign --force --deep --sign - "$APP" >/dev/null 2>&1 || true
echo "$APP"
