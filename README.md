# Robot

[![CI](https://github.com/4yvi/robot/actions/workflows/ci.yml/badge.svg)](https://github.com/4yvi/robot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](go.mod)

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
- Go 1.22+ and a C compiler (MinGW-w64 on Windows, Xcode CLT on macOS) to build from source
- [LM Studio](https://lmstudio.ai) with a model loaded and the local server started
- cmake (macOS, to build whisper.cpp with Metal)

## Installation

### From source
```sh
git clone https://github.com/4yvi/robot && cd robot
# macOS
bash scripts/setup.sh
# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/setup.ps1
```
The setup script downloads the Vosk model and library, whisper.cpp and its model, Piper and a Russian voice into `models/` and creates `config.yaml`.

Build with Vosk enabled:
```sh
CGO_CPPFLAGS=-I$PWD/third_party/vosk CGO_LDFLAGS=-L$PWD/third_party/vosk go build -tags vosk -o robot ./cmd/robot
```
On Windows add `-ldflags "-H=windowsgui"` and `-lvosk`. For macOS, `bash scripts/make-app.sh ./robot 0.1.0 $(uname -m)` produces `dist/app/<arch>/Robot.app` (required for microphone access).

### Run
1. Start whisper-server (the setup script prints the command).
2. Start LM Studio's local server and load a model.
3. Run `robot` (or open `Robot.app`). Pick the model from the tray menu.
4. Say **«робот»**.

## Configuration
Copy [config.example.yaml](config.example.yaml) to `config.yaml` (next to the executable or in the user config dir). Key options: `wake_word`, `mic_device`, `vosk_model_path`, `whisper_url`, `lmstudio_url`, `model_name`, `piper_path`, `piper_voice`, `silence_ms`, `follow_up_seconds`, `session_timeout_minutes`, `max_clarifications`, `max_tool_calls`, `allowed_folders`, `apps`. The system prompt lives in `prompt.md`.

## Usage
```
robot            run in the background
robot devices    list audio devices
robot history    show recent sessions
```
Stop phrases: «хватит», «пока», «стоп».

## Roadmap
- [ ] More languages and wake words
- [ ] Linux support
- [ ] Streaming STT
- [ ] Plugin API for tools

## Contributing
See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under [MIT](LICENSE).
