#!/usr/bin/env bash
# Downloads libvosk for CI/release builds into ./third_party/vosk
set -euo pipefail
VER="0.3.42"
DIR="$(cd "$(dirname "$0")/.." && pwd)/third_party/vosk"
mkdir -p "$DIR"
case "$(uname -s)" in
  Darwin) NAME="vosk-osx-$VER" ;;
  MINGW*|MSYS*|CYGWIN*) NAME="vosk-win64-$VER" ;;
  *) echo "unsupported OS"; exit 1 ;;
esac
curl -fsSL -o "$DIR/vosk.zip" "https://github.com/alphacep/vosk-api/releases/download/v$VER/$NAME.zip"
unzip -oq "$DIR/vosk.zip" -d "$DIR"
mv "$DIR/$NAME"/* "$DIR/" && rmdir "$DIR/$NAME" && rm "$DIR/vosk.zip"
echo "CGO_CPPFLAGS=-I$DIR"
echo "CGO_LDFLAGS=-L$DIR"
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "CGO_CPPFLAGS=-I$DIR" >> "$GITHUB_ENV"
  echo "CGO_LDFLAGS=-L$DIR" >> "$GITHUB_ENV"
fi
