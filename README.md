# Robot

[![CI](https://github.com/4yvi/robot/actions/workflows/ci.yml/badge.svg)](https://github.com/4yvi/robot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](go.mod)

English | [Русский](README.ru.md)

A local, always-on voice assistant written in Go for Windows and macOS (Apple Silicon and Intel). Say the Russian wake word **«робот»**, ask a question, and get a spoken answer from a model running in [LM Studio](https://lmstudio.ai). No cloud, no web UI, no Python.

## Features
- Wake word detection with Vosk (restricted grammar: the wake word and `[unk]`)
- Speech to text via a local whisper.cpp server
- LLM via LM Studio (OpenAI-compatible API): streaming, tool calling, structured output
- Spoken answers with Piper, sentence by sentence, interruptible by the wake word
- Clarification step before answering (max 2 in a row)
- Tools: web search, page fetch, time, timers and reminders, open apps/URLs, media and volume, clipboard, file search/read in allowed folders, notes, shell (after voice confirmation), memory search
- Sessions and long-term memory in SQLite (summaries and facts)
- Tray icon (state colors) and menu, autostart, single instance

## Architecture

```
mic → audio → wake (Vosk) → session FSM → stt (whisper-server) → clarify → llm (LM Studio) ⇄ tools
                                                                                 ↓
                                                              tts (Piper) → speakers      memory (SQLite)
```

Details: [docs/architecture.md](docs/architecture.md).

## Requirements
- Windows 10/11 x64 or macOS 11+ (arm64 / x86_64)
- Go 1.26+ and a C compiler (MinGW-w64 on Windows, Xcode CLT on macOS) to build from source
- [LM Studio](https://lmstudio.ai) with a model loaded and the local server started
- cmake (macOS, to build whisper.cpp with Metal)

## Installation

### Quick start (release builds)
1. Install [LM Studio](https://lmstudio.ai), load a model and start its local server.
2. Download the archive for your system from [Releases](https://github.com/4yvi/robot/releases): `Robot_*_macos_*.zip` (unzip and open `Robot.app`) or `robot_*_windows_amd64.zip` (unzip and run `robot.exe`).
3. On the first run Robot downloads the large speech recognition model (about 470 MB) and starts `whisper-server` itself. It tells you by voice when it is ready, then say **«робот»**.

The Vosk model, Piper with a Russian voice and whisper-server are already inside the archive. macOS may ask for microphone access; allow it. If macOS blocks the unsigned app, right-click it and choose Open.

### From source
```sh
git clone https://github.com/4yvi/robot && cd robot
bash scripts/setup.sh        # macOS (needs cmake); Windows: powershell -File scripts/setup.ps1
CGO_CPPFLAGS=-I$PWD/third_party/vosk CGO_LDFLAGS=-L$PWD/third_party/vosk go build -tags vosk -o robot ./cmd/robot
```
On Windows add `-ldflags "-H=windowsgui"` and `-lvosk`. `bash scripts/make-app.sh ./robot 0.1.0 $(uname -m)` builds `Robot.app` (needed for microphone access on macOS).

## Configuration
Copy [config.example.yaml](config.example.yaml) to `config.yaml` (next to the executable, or `~/Library/Application Support/robot/` on macOS and `%AppData%\robot\` on Windows). Logs are in the `logs` folder next to it. Key options: `wake_word`, `mic_device`, `vosk_model_path`, `whisper_url`, `lmstudio_url`, `model_name`, `piper_path`, `piper_voice`, `silence_ms`, `follow_up_seconds`, `session_timeout_minutes`, `max_clarifications`, `max_tool_calls`, `allowed_folders`, `apps`. The system prompt lives in `prompt.md`.

## Usage
```
robot               run in the background with a tray icon
robot headless      run without the tray (logs to the terminal)
robot devices       list audio devices
robot record-test   record 5 seconds and play them back
robot listen        wake word + phrase recognition check
robot say "text"    speak text with Piper
robot history       show recent sessions (add --full for messages)
```
Stop phrases: «хватит», «пока», «стоп».

## Roadmap
- [ ] More languages and wake words
- [ ] Linux support
- [ ] Streaming STT
- [ ] Plugin API for tools

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under [MIT](LICENSE).
