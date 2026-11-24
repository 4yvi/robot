# Robot

[![CI](https://github.com/4yvi/robot/actions/workflows/ci.yml/badge.svg)](https://github.com/4yvi/robot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](go.mod)

[English](README.md) | Русский

Локальный голосовой ассистент на Go для Windows и macOS (Apple Silicon и Intel), который работает в фоне. Скажите кодовое слово **«робот»**, задайте вопрос и услышите ответ от модели в [LM Studio](https://lmstudio.ai). Без облака, без веб-интерфейса, без Python.

## Возможности
- Кодовое слово через Vosk (грамматика из кодового слова и `[unk]`)
- Распознавание речи через локальный сервер whisper.cpp
- Модель в LM Studio (OpenAI-совместимый API): потоковый ответ, вызов инструментов, структурированный вывод
- Озвучка Piper по предложениям, кодовое слово прерывает речь
- Шаг уточнения перед ответом (не больше двух подряд)
- Инструменты: поиск в интернете, загрузка страниц, время, таймеры и напоминания, открытие программ и ссылок, громкость и медиа, буфер обмена, поиск и чтение файлов в разрешённых папках, заметки, консоль (после голосового подтверждения), поиск по прошлым диалогам
- Сессии и долгая память в SQLite (сводки и факты)
- Значок в трее, меню, автозапуск, защита от второго экземпляра

## Архитектура

```
микрофон → audio → wake (Vosk) → session FSM → stt (whisper-server) → clarify → llm (LM Studio) ⇄ tools
                                                                                      ↓
                                                                   tts (Piper) → динамики     memory (SQLite)
```

Подробнее: [docs/architecture.md](docs/architecture.md).

## Требования
- Windows 10/11 x64 или macOS 11+ (arm64 / x86_64)
- Go 1.26+ и C-компилятор (MinGW-w64 на Windows, Xcode CLT на macOS) для сборки из исходников
- [LM Studio](https://lmstudio.ai) с загруженной моделью и запущенным локальным сервером
- cmake (на macOS, для сборки whisper.cpp с Metal)

## Установка с нуля

```sh
git clone https://github.com/4yvi/robot && cd robot
# macOS
bash scripts/setup.sh
# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/setup.ps1
```
Скрипт скачивает модель и библиотеку Vosk, whisper.cpp с моделью, Piper и русский голос в `models/`, и создаёт `config.yaml`.

Сборка с Vosk:
```sh
CGO_CPPFLAGS=-I$PWD/third_party/vosk CGO_LDFLAGS=-L$PWD/third_party/vosk go build -tags vosk -o robot ./cmd/robot
```
На Windows добавьте `-ldflags "-H=windowsgui"` и `-lvosk`. На macOS `bash scripts/make-app.sh ./robot 0.1.0 $(uname -m)` создаёт `dist/app/<arch>/Robot.app` (без .app система не даст доступ к микрофону).

### Запуск
1. Запустите whisper-server (команду выводит скрипт установки).
2. Запустите локальный сервер LM Studio и загрузите модель.
3. Запустите `robot` (или `Robot.app`), выберите модель в меню трея.
4. Скажите **«робот»**.

## Конфигурация
Скопируйте [config.example.yaml](config.example.yaml) в `config.yaml` (рядом с exe, либо `~/Library/Application Support/robot/` на macOS и `%AppData%\robot\` на Windows). Логи лежат в папке `logs` рядом с ним. Основные параметры: `wake_word`, `mic_device`, `vosk_model_path`, `whisper_url`, `lmstudio_url`, `model_name`, `piper_path`, `piper_voice`, `silence_ms`, `follow_up_seconds`, `session_timeout_minutes`, `max_clarifications`, `max_tool_calls`, `allowed_folders`, `apps`. Системный промпт лежит в `prompt.md`.

## Использование
```
robot               запуск в фоне со значком в трее
robot headless      запуск без трея (логи в терминал)
robot devices       список аудиоустройств
robot record-test   записать 5 секунд и воспроизвести
robot listen        проверка кодового слова и распознавания
robot say "текст"   озвучить текст через Piper
robot history       последние сессии (--full с репликами)
```
Слова завершения: «хватит», «пока», «стоп».

## Roadmap
- [ ] Другие языки и кодовые слова
- [ ] Linux
- [ ] Потоковое распознавание
- [ ] API плагинов для инструментов

## Участие
См. [CONTRIBUTING.md](CONTRIBUTING.md). Лицензия [MIT](LICENSE).
