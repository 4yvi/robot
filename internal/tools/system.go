package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/4yvi/robot/internal/config"
)

var weekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

func currentTimeTool(env *Env) Tool {
	return &funcTool{
		name:  "current_time",
		desc:  "Возвращает текущие дату и время.",
		props: map[string]any{},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			n := env.now()
			return fmt.Sprintf("%s, %s", weekdays[n.Weekday()], n.Format("02.01.2006 15:04")), nil
		},
	}
}

func openTool(env *Env) Tool {
	return &funcTool{
		name: "open_app_or_url",
		desc: "Открывает программу из списка в настройках или ссылку http/https.",
		props: map[string]any{
			"target": str("имя программы из списка или адрес ссылки"),
		},
		required: []string{"target"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Target string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			target := strings.TrimSpace(a.Target)
			if u, err := url.Parse(target); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				return "Открыто.", platformOpen(ctx, u.String(), true)
			}
			for name, cmd := range env.Config.Apps {
				if strings.EqualFold(name, target) {
					return "Открыто.", platformOpen(ctx, expandHome(cmd), false)
				}
			}
			names := make([]string, 0, len(env.Config.Apps))
			for n := range env.Config.Apps {
				names = append(names, n)
			}
			return "", fmt.Errorf("unknown app %q; known apps: %s", target, strings.Join(names, ", "))
		},
	}
}

// MediaActions lists the supported media_control actions.
var MediaActions = []string{"volume_up", "volume_down", "mute", "set_volume", "play_pause", "next", "previous"}

func mediaTool(env *Env) Tool {
	return &funcTool{
		name: "media_control",
		desc: "Управление громкостью и воспроизведением: " + strings.Join(MediaActions, ", ") + ".",
		props: map[string]any{
			"action": map[string]any{"type": "string", "enum": MediaActions},
			"level":  num("громкость от 0 до 100, только для set_volume"),
		},
		required: []string{"action"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Action string
				Level  float64
			}
			if err := decode(args, &a); err != nil {
				return "", err
			}
			ok := false
			for _, m := range MediaActions {
				ok = ok || m == a.Action
			}
			if !ok {
				return "", fmt.Errorf("unknown action %q", a.Action)
			}
			if a.Action == "set_volume" && (a.Level < 0 || a.Level > 100) {
				return "", fmt.Errorf("level must be 0..100")
			}
			if err := platformMedia(ctx, a.Action, int(a.Level)); err != nil {
				return "", err
			}
			return "Готово.", nil
		},
	}
}

func clipboardReadTool() Tool {
	return &funcTool{
		name:  "clipboard_read",
		desc:  "Читает текст из буфера обмена.",
		props: map[string]any{},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			t, err := clipboardRead(ctx)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(t) == "" {
				return "Буфер обмена пуст.", nil
			}
			return truncate(t, 4000), nil
		},
	}
}

func clipboardWriteTool() Tool {
	return &funcTool{
		name:     "clipboard_write",
		desc:     "Записывает текст в буфер обмена.",
		props:    map[string]any{"text": str("текст")},
		required: []string{"text"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Text string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			if err := clipboardWrite(ctx, a.Text); err != nil {
				return "", err
			}
			return "Скопировано.", nil
		},
	}
}

// NotesPath returns the notes file path.
func NotesPath(cfg *config.Config) (string, error) {
	if cfg.NotesFile != "" {
		return expandHome(cfg.NotesFile), nil
	}
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "notes.md"), nil
}

func noteAddTool(env *Env) Tool {
	return &funcTool{
		name:     "note_add",
		desc:     "Записывает заметку в файл заметок.",
		props:    map[string]any{"text": str("текст заметки")},
		required: []string{"text"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Text string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			if strings.TrimSpace(a.Text) == "" {
				return "", fmt.Errorf("note is empty")
			}
			p, err := NotesPath(env.Config)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return "", err
			}
			defer f.Close()
			line := fmt.Sprintf("- %s %s\n", env.now().Format("2006-01-02 15:04"), strings.TrimSpace(a.Text))
			if _, err := f.WriteString(line); err != nil {
				return "", err
			}
			return "Заметка сохранена.", nil
		},
	}
}

const shellTimeout = 30 * time.Second

func shellTool(env *Env) Tool {
	return &funcTool{
		name: "run_shell",
		desc: "Выполняет команду в консоли (PowerShell на Windows, zsh на macOS). Перед запуском ассистент спросит у пользователя голосовое подтверждение.",
		props: map[string]any{
			"command": str("команда"),
		},
		required: []string{"command"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Command string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			cmdline := strings.TrimSpace(a.Command)
			if cmdline == "" {
				return "", fmt.Errorf("command is empty")
			}
			if env.Confirm == nil {
				return "", fmt.Errorf("confirmation is not available, command not run")
			}
			ok, err := env.Confirm(ctx, "Выполнить команду: "+truncate(cmdline, 200)+"? Скажите да или нет.")
			if err != nil {
				return "", err
			}
			if !ok {
				return "Пользователь не подтвердил, команда не выполнена.", nil
			}
			cctx, cancel := context.WithTimeout(ctx, shellTimeout)
			defer cancel()
			cmd := shellCommand(cctx, cmdline)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err = cmd.Run()
			res := truncate(strings.TrimSpace(out.String()), 4000)
			if err != nil {
				return res + "\n(ошибка: " + err.Error() + ")", nil
			}
			if res == "" {
				res = "Команда выполнена, вывода нет."
			}
			return res, nil
		},
	}
}

func memorySearchTool(env *Env) Tool {
	return &funcTool{
		name:     "memory_search",
		desc:     "Ищет по прошлым диалогам с пользователем.",
		props:    map[string]any{"query": str("о чём искать")},
		required: []string{"query"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Query string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			if env.Memory == nil {
				return "Память недоступна.", nil
			}
			res, err := env.Memory.Search(ctx, a.Query, 8)
			if err != nil {
				return "", err
			}
			if len(res) == 0 {
				return "В прошлых диалогах ничего не найдено.", nil
			}
			return strings.Join(res, "\n"), nil
		},
	}
}
