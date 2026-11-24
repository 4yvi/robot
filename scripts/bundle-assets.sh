#!/usr/bin/env bash
# Collects the small runtime pieces that ship inside release archives:
# Vosk model, Piper + Russian voice, whisper-server. The large whisper model is
# NOT bundled; the app downloads it on first run.
# Usage: bundle-assets.sh <windows|macos> <amd64|arm64> <dest-dir>
set -euo pipefail
OS="$1"; ARCH="$2"; DEST="$3"
WHISPER_TAG="v1.9.6"
PIPER_VER="2023.11.14-2"
VOICE="https://huggingface.co/rhasspy/piper-voices/resolve/main/ru/ru_RU/irina/medium"
TMP="$(mktemp -d)"
mkdir -p "$DEST/models/whisper"

echo "==> Vosk model"
curl -fsSL -o "$TMP/vosk.zip" https://alphacephei.com/vosk/models/vosk-model-small-ru-0.22.zip
unzip -q "$TMP/vosk.zip" -d "$TMP"
mv "$TMP/vosk-model-small-ru-0.22" "$DEST/models/vosk-model-small-ru"

echo "==> Piper"
if [ "$OS" = windows ]; then
  curl -fsSL -o "$TMP/piper.zip" "https://github.com/rhasspy/piper/releases/download/$PIPER_VER/piper_windows_amd64.zip"
  unzip -q "$TMP/piper.zip" -d "$DEST/models"
else
  [ "$ARCH" = arm64 ] && PA=aarch64 || PA=x64
  curl -fsSL -o "$TMP/piper.tgz" "https://github.com/rhasspy/piper/releases/download/$PIPER_VER/piper_macos_$PA.tar.gz"
  tar -xzf "$TMP/piper.tgz" -C "$DEST/models"
fi
for f in ru_RU-irina-medium.onnx ru_RU-irina-medium.onnx.json; do
  curl -fsSL -o "$DEST/models/piper/$f" "$VOICE/$f"
done

echo "==> whisper-server $WHISPER_TAG"
git clone -q --depth 1 --branch "$WHISPER_TAG" https://github.com/ggml-org/whisper.cpp "$TMP/whisper.cpp"
CMAKE_ARGS=(-DBUILD_SHARED_LIBS=OFF -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON -DCMAKE_BUILD_TYPE=Release)
if [ "$OS" = macos ]; then
  if [ "$ARCH" = arm64 ]; then
    CMAKE_ARGS+=(-DCMAKE_OSX_ARCHITECTURES=arm64 -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON)
  else
    CMAKE_ARGS+=(-DCMAKE_OSX_ARCHITECTURES=x86_64 -DGGML_METAL=OFF -DGGML_NATIVE=OFF)
  fi
fi
cmake -S "$TMP/whisper.cpp" -B "$TMP/build" "${CMAKE_ARGS[@]}"
cmake --build "$TMP/build" --config Release --target whisper-server -j
BIN="$(find "$TMP/build" -name 'whisper-server*' -type f \( -name whisper-server -o -name whisper-server.exe \) | head -1)"
[ -n "$BIN" ] || { echo "whisper-server was not built"; exit 1; }
cp "$BIN" "$DEST/models/whisper/"
rm -rf "$TMP"
du -sh "$DEST/models"/*
