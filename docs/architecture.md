# Architecture

Robot is a background voice assistant. Everything runs locally: wake word (Vosk), speech to text (whisper.cpp server), LLM (LM Studio), text to speech (Piper), storage (SQLite).

```
 mic ─► audio.Recorder ─► wake.Detector (Vosk, grammar ["робот","[unk]"])
                               │ wake word
                               ▼
                         session.Manager (FSM)
        ┌──────────────────────┼───────────────────────┐
        ▼                      ▼                       ▼
 audio.Capture+VAD ─► stt.Client ──► clarify.Checker ─► llm.Client ◄─► tools.Registry
 (until silence)     (whisper-server)  (JSON schema)   (LM Studio)      (11 tools)
                                                            │
                                                            ▼
                                   tts.Speaker (Piper, sentence queue) ─► audio.Player
                                                            │
                                                  memory.Store (SQLite)
```

## Packages

| Package | Interface | Notes |
|---|---|---|
| `internal/audio` | `Recorder`, `Player`, `VAD` | malgo, 16 kHz mono s16; energy-based silence detector |
| `internal/wake` | `Detector` | Vosk with restricted grammar; build tag `vosk` |
| `internal/stt` | `Transcriber` | HTTP multipart WAV to whisper-server, `language=ru` |
| `internal/llm` | `Client` | OpenAI-compatible, SSE streaming, tool calls, JSON schema |
| `internal/clarify` | `Checker` | structured output `{needs_clarification, missing, question}` |
| `internal/tools` | `Tool`, `Registry` | platform code in `*_windows.go` / `*_darwin.go` |
| `internal/tts` | `Synthesizer`, `Speaker` | Piper via stdin, sentence splitting, interruptible |
| `internal/memory` | `Store` | modernc.org/sqlite (no cgo), sessions, messages, summaries, facts |
| `internal/session` | `Manager` | dialog state machine |
| `internal/config` | `Config` | YAML load/save, path resolution |

## Session state machine

`idle` (waiting for wake word) → `listening` (record until silence) → `thinking` (clarify, LLM, tools) → `speaking` → `follow-up` (listen `follow_up_seconds` without wake word) → `idle` (session stays resumable for `session_timeout_minutes`).
Stop phrases end the session immediately; on close the session is saved, summarized by the model, facts are extracted, and the last three summaries plus facts are injected into the next session's system prompt.

## Platform layer

Build tags separate OS code: media/volume (Windows API / `osascript`), clipboard, open (`start` / `open`), shell (PowerShell / zsh, only after spoken "да"), autostart (Startup shortcut / LaunchAgent), tray/single instance.

## Decisions

- Name is `robot` everywhere (binary, repo, `Robot.app`).
- Config, logs, DB: config is looked up next to the executable first, then in `os.UserConfigDir()/robot`. Logs and DB live in `os.UserConfigDir()/robot`, with no hard-coded paths.
- The wake word is Russian, default `робот`.
- Not a Windows service: services have no access to the user session microphone.
- whisper.cpp and Piper are external processes to avoid more cgo dependencies. Only malgo, Vosk and the tray need cgo.
- Only the wake detector needs Vosk. Packages are built so the rest compiles and tests without cgo.
