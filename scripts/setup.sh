#!/usr/bin/env bash
# macOS setup: downloads Vosk (model + lib), builds whisper.cpp with Metal,
# downloads Piper and a Russian voice. Everything goes to ./models and ./third_party.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
MODELS="$ROOT/models"
mkdir -p "$MODELS" third_party

ARCH="$(uname -m)"   # arm64 or x86_64
VOSK_MODEL="vosk-model-small-ru-0.22"
WHISPER_MODEL="ggml-small.bin"
PIPER_VER="2023.11.14-2"
VOICE_BASE="https://huggingface.co/rhasspy/piper-voices/resolve/main/ru/ru_RU/irina/medium"

echo "==> Vosk model"
if [ ! -d "$MODELS/vosk-model-small-ru" ]; then
  curl -fL -o "$MODELS/vosk.zip" "https://alphacephei.com/vosk/models/$VOSK_MODEL.zip"
  unzip -oq "$MODELS/vosk.zip" -d "$MODELS" && mv "$MODELS/$VOSK_MODEL" "$MODELS/vosk-model-small-ru" && rm "$MODELS/vosk.zip"
fi

echo "==> libvosk"
bash "$ROOT/scripts/fetch-libvosk.sh"

echo "==> whisper.cpp (Metal)"
if [ ! -x "$MODELS/whisper/whisper-server" ]; then
  command -v cmake >/dev/null || { echo "Install cmake: brew install cmake"; exit 1; }
  rm -rf third_party/whisper.cpp
  git clone --depth 1 https://github.com/ggml-org/whisper.cpp third_party/whisper.cpp
  cmake -S third_party/whisper.cpp -B third_party/whisper.cpp/build -DGGML_METAL=ON -DCMAKE_BUILD_TYPE=Release
  cmake --build third_party/whisper.cpp/build -j --config Release --target whisper-server
  mkdir -p "$MODELS/whisper"
  cp third_party/whisper.cpp/build/bin/whisper-server "$MODELS/whisper/"
fi
[ -f "$MODELS/whisper/$WHISPER_MODEL" ] || curl -fL -o "$MODELS/whisper/$WHISPER_MODEL" "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/$WHISPER_MODEL"

echo "==> Piper"
if [ ! -x "$MODELS/piper/piper" ]; then
  [ "$ARCH" = "arm64" ] && PA="aarch64" || PA="x64"
  curl -fL -o "$MODELS/piper.tgz" "https://github.com/rhasspy/piper/releases/download/$PIPER_VER/piper_macos_$PA.tar.gz"
  tar -xzf "$MODELS/piper.tgz" -C "$MODELS" && rm "$MODELS/piper.tgz"
fi
for f in ru_RU-irina-medium.onnx ru_RU-irina-medium.onnx.json; do
  [ -f "$MODELS/piper/$f" ] || curl -fL -o "$MODELS/piper/$f" "$VOICE_BASE/$f"
done

[ -f config.yaml ] || cp config.example.yaml config.yaml
cat <<EOF

Done. Start the whisper server:
  $MODELS/whisper/whisper-server -m $MODELS/whisper/$WHISPER_MODEL -l ru --port 8080
Build the app (needs the vosk tag and libvosk from third_party/vosk):
  CGO_CPPFLAGS=-I\$PWD/third_party/vosk CGO_LDFLAGS=-L\$PWD/third_party/vosk go build -tags vosk -o robot ./cmd/robot
EOF
