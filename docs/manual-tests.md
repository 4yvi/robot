# Manual tests

Everything below needs a real microphone, speakers or the external services, so it is not covered by `go test`.

| Stage | Steps | Expected |
|---|---|---|
| 1 Audio | `robot devices`; `robot record-test` | devices listed; 5 s recording plays back |
| 2 Wake | `robot listen`, say «робот», then unrelated speech | beep on the wake word only |
| 3 Phrase | say «робот», then a phrase, pause 1.2 s | recognized text in the log |
| 4 Voice | `robot say "Привет"`; long text; say «робот» while speaking | speech, first sentence fast, interrupted by wake word |
| 5 LLM | start LM Studio with a model; ask a question | short spoken answer |
| 6 Clarify | «какие диски подойдут на машину» | asks for the car model; max 2 clarifications |
| 7 Tools | ask for specs of a machine; «поставь таймер на минуту»; shell command | web search used; timer speaks; shell asks «да» |
| 8 Memory | chat, say «хватит», new session, ask about the previous topic | summary remembered; `robot history` lists sessions |
| 9 Tray | launch, check icon colors and menu, reboot | icon color follows state; autostart answers the wake word |
| macOS | run `Robot.app` | microphone permission prompt appears |

## Already covered by automated tests

Wake word on synthesized speech (macOS, `-tags vosk`), speech-length gate, VAD and phrase recording, WAV encoding, whisper client, sentence splitting and interruptible speaker, LM Studio client (SSE, tool calls, JSON schema), clarification policy, every tool except OS integrations, SQLite memory, session logic (clarify, tool loop, tool limit, summaries carried into the next session, expiry), autostart plist, single-instance lock, tray icons.
