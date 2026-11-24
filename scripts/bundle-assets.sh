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

echo "==> Neural voice (sherpa-onnx + Piper VITS voice)"
# v1.12.x still accepts Piper voices with --vits-data-dir; v2.x dropped it.
SHERPA_VER="v1.12.28"
if [ "$OS" = windows ]; then
  SHERPA_PKG="sherpa-onnx-$SHERPA_VER-win-x64-shared-MD-Release"
else
  SHERPA_PKG="sherpa-onnx-$SHERPA_VER-osx-universal2-shared"
fi
curl -fsSL -o "$TMP/sherpa.tar.bz2" "https://github.com/k2-fsa/sherpa-onnx/releases/download/$SHERPA_VER/$SHERPA_PKG.tar.bz2"
tar -xjf "$TMP/sherpa.tar.bz2" -C "$TMP"
mkdir -p "$DEST/models/sherpa/bin" "$DEST/models/sherpa/lib"
cp "$TMP/$SHERPA_PKG"/bin/sherpa-onnx-offline-tts* "$DEST/models/sherpa/bin/"
if [ "$OS" = windows ]; then
  cp "$TMP/$SHERPA_PKG"/bin/*.dll "$DEST/models/sherpa/bin/"
else
  cp "$TMP/$SHERPA_PKG"/lib/*.dylib "$DEST/models/sherpa/lib/"
fi
curl -fsSL -o "$TMP/voice.tar.bz2" https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-piper-ru_RU-irina-medium.tar.bz2
tar -xjf "$TMP/voice.tar.bz2" -C "$TMP"
mv "$TMP/vits-piper-ru_RU-irina-medium" "$DEST/models/sherpa/voice"
rm -f "$DEST/models/sherpa/voice/MODEL_CARD"

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
