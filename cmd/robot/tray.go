package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/systray"

	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/platform"
	"github.com/4yvi/robot/internal/session"
	"github.com/4yvi/robot/internal/tray"
)

var stateTitles = map[session.State]string{
	session.StateIdle:      "Ждёт кодовое слово",
	session.StateListening: "Слушает",
	session.StateThinking:  "Думает",
	session.StateSpeaking:  "Говорит",
	session.StatePaused:    "На паузе",
}

// choiceMenu is a submenu of radio-style items rebuilt from a list.
type choiceMenu struct {
	parent *systray.MenuItem
	items  []*systray.MenuItem
}

func (c *choiceMenu) rebuild(names []string, current string, onPick func(name string)) {
	for _, it := range c.items {
		it.Remove()
	}
	c.items = nil
	for _, n := range names {
		it := c.parent.AddSubMenuItemCheckbox(n, "", n == current)
		c.items = append(c.items, it)
		go func(it *systray.MenuItem, name string) {
			for range it.ClickedCh {
				for _, o := range c.items {
					o.Uncheck()
				}
				it.Check()
				onPick(name)
			}
		}(it, n)
	}
}

func runTray(ctx context.Context, stop context.CancelFunc, app *App) {
	cfg := app.cfg
	systray.Run(func() {
		systray.SetIcon(tray.Icon(string(session.StateIdle)))
		systray.SetTooltip("Robot")

		head := systray.AddMenuItem("Robot "+version, "")
		head.Disable()
		status := systray.AddMenuItem("Состояние: "+stateTitles[session.StateIdle], "")
		status.Disable()
		question := systray.AddMenuItem("Вопрос: -", "")
		question.Disable()
		app.mgr.SetOnUserText(func(t string) {
			t = strings.Join(strings.Fields(t), " ")
			if r := []rune(t); len(r) > 90 {
				t = string(r[:90]) + "…"
			}
			question.SetTitle("Вопрос: " + t)
		})
		app.mgr.SetOnState(func(s session.State) {
			systray.SetIcon(tray.Icon(string(s)))
			systray.SetTooltip("Robot: " + stateTitles[s])
			status.SetTitle("Состояние: " + stateTitles[s])
		})

		pause := systray.AddMenuItemCheckbox("Пауза", "Не реагировать на кодовое слово", false)
		systray.AddSeparator()

		// LM Studio
		lmItem := systray.AddMenuItem("LM Studio: "+cfg.LMStudioURL+" (изменить)", "")
		check := systray.AddMenuItem("Проверить соединение / обновить список моделей", "")
		models := systray.AddMenuItem("Модель (выбрать): "+orDefault(cfg.ModelName, "авто"), "")
		modelMenu := &choiceMenu{parent: models}
		refreshModels := func() string {
			c, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			ids, err := app.llm.Models(c)
			if err != nil {
				return "нет связи: " + err.Error()
			}
			known := false
			for _, id := range ids {
				known = known || id == app.llm.Model
			}
			if !known && len(ids) > 0 { // saved model is not on this server: fall back to the first one
				app.llm.SetModel(ids[0])
				_ = cfg.Update(func(c *config.Config) { c.ModelName = ids[0] })
			}
			models.SetTitle("Модель (выбрать): " + orDefault(app.llm.Model, "авто"))
			modelMenu.rebuild(ids, app.llm.Model, func(name string) {
				app.llm.SetModel(name)
				_ = cfg.Update(func(c *config.Config) { c.ModelName = name })
				models.SetTitle("Модель (выбрать): " + name)
			})
			return fmt.Sprintf("доступно моделей: %d", len(ids))
		}
		go refreshModels()

		// Microphone
		mics := systray.AddMenuItem("Микрофон: "+orDefault(cfg.MicDevice, "по умолчанию"), "")
		micMenu := &choiceMenu{parent: mics}
		var names []string
		names = append(names, "По умолчанию")
		if devs, err := app.audio.Capture(); err == nil {
			for _, d := range devs {
				names = append(names, d.Name)
			}
		}
		cur := cfg.MicDevice
		if cur == "" {
			cur = "По умолчанию"
		}
		micMenu.rebuild(names, cur, func(name string) {
			if name == "По умолчанию" {
				name = ""
			}
			app.audio.SetMic(name)
			_ = cfg.Update(func(c *config.Config) { c.MicDevice = name })
			mics.SetTitle("Микрофон: " + orDefault(name, "по умолчанию"))
			app.mgr.Reload()
		})

		wakeItem := systray.AddMenuItem("Кодовое слово: "+cfg.WakeWord+" (изменить)", "")
		auto := systray.AddMenuItemCheckbox("Автозапуск при входе", "", platform.AutostartEnabled())
		systray.AddSeparator()
		hist := systray.AddMenuItem("Открыть историю диалогов", "")
		logs := systray.AddMenuItem("Открыть логи", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Выход", "")

		go func() {
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-pause.ClickedCh:
					p := !app.mgr.Paused()
					app.mgr.SetPaused(p)
					if p {
						pause.Check()
					} else {
						pause.Uncheck()
					}
				case <-lmItem.ClickedCh:
					go func() {
						v, ok := platform.PromptText("LM Studio", "Адрес LM Studio (OpenAI API):", cfg.LMStudioURL)
						if !ok {
							return
						}
						app.llm.SetBaseURL(v)
						v = app.llm.BaseURL // normalized, with /v1
						_ = cfg.Update(func(c *config.Config) { c.LMStudioURL = v })
						lmItem.SetTitle("LM Studio: " + v + " (изменить)")
						check.SetTitle("Проверка: " + refreshModels())
					}()
				case <-check.ClickedCh:
					go check.SetTitle("Проверка: " + refreshModels())
				case <-wakeItem.ClickedCh:
					go func() {
						v, ok := platform.PromptText("Кодовое слово", "Новое кодовое слово (по-русски):", cfg.WakeWord)
						if !ok || v == cfg.WakeWord {
							return
						}
						if err := app.mgr.SetWakeWord(v); err != nil {
							slog.Error("change wake word", "err", err)
							return
						}
						_ = cfg.Update(func(c *config.Config) { c.WakeWord = v })
						wakeItem.SetTitle("Кодовое слово: " + v + " (изменить)")
					}()
				case <-auto.ClickedCh:
					want := !auto.Checked()
					if err := platform.SetAutostart(want); err != nil {
						slog.Error("autostart", "err", err)
						continue
					}
					if want {
						auto.Check()
					} else {
						auto.Uncheck()
					}
					_ = cfg.Update(func(c *config.Config) { c.Autostart = want })
				case <-hist.ClickedCh:
					go openHistory(app)
				case <-logs.ClickedCh:
					if d, err := config.Dir(); err == nil {
						_ = platform.OpenPath(filepath.Join(d, "logs"))
					}
				case <-quit.ClickedCh:
					stop()
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		stop()
	})
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// openHistory exports recent sessions to history.txt and opens it.
func openHistory(app *App) {
	d, err := config.Dir()
	if err != nil {
		return
	}
	var buf bytes.Buffer
	if err := printHistory(&buf, app.store, 30, true); err != nil {
		slog.Error("export history", "err", err)
		return
	}
	p := filepath.Join(d, "history.txt")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		slog.Error("write history", "err", err)
		return
	}
	_ = platform.OpenPath(p)
}
