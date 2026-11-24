package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// scheduler fires voice notifications at a given time.
type scheduler struct {
	env    *Env
	mu     sync.Mutex
	timers map[*time.Timer]struct{}
}

func newScheduler(env *Env) *scheduler {
	return &scheduler{env: env, timers: map[*time.Timer]struct{}{}}
}

func (s *scheduler) add(d time.Duration, message string) {
	var t *time.Timer
	t = time.AfterFunc(d, func() {
		s.mu.Lock()
		delete(s.timers, t)
		s.mu.Unlock()
		if s.env.Say != nil {
			s.env.Say(message)
		}
	})
	s.mu.Lock()
	s.timers[t] = struct{}{}
	s.mu.Unlock()
}

// ParseWhen interprets "HH:MM" (today, or tomorrow if already past) or RFC3339.
func ParseWhen(now time.Time, at string) (time.Time, error) {
	at = strings.TrimSpace(at)
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", at, now.Location()); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("15:04", at, now.Location()); err == nil {
		r := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if !r.After(now) {
			r = r.AddDate(0, 0, 1)
		}
		return r, nil
	}
	return time.Time{}, fmt.Errorf("cannot understand time %q, use HH:MM or YYYY-MM-DD HH:MM", at)
}

func (s *scheduler) timerTool() Tool {
	return &funcTool{
		name: "set_timer",
		desc: "Ставит таймер. Когда время выйдет, ассистент скажет об этом голосом.",
		props: map[string]any{
			"seconds": num("через сколько секунд сработать"),
			"label":   str("название таймера, необязательно"),
		},
		required: []string{"seconds"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Seconds float64
				Label   string
			}
			if err := decode(args, &a); err != nil {
				return "", err
			}
			if a.Seconds <= 0 || a.Seconds > 7*24*3600 {
				return "", fmt.Errorf("seconds must be between 1 and 604800")
			}
			msg := "Таймер сработал."
			if a.Label != "" {
				msg = "Таймер " + a.Label + " сработал."
			}
			s.add(time.Duration(a.Seconds*1000)*time.Millisecond, msg)
			return fmt.Sprintf("Таймер поставлен на %d сек.", int(a.Seconds)), nil
		},
	}
}

func (s *scheduler) reminderTool() Tool {
	return &funcTool{
		name: "set_reminder",
		desc: "Ставит напоминание на время. Укажи либо in_minutes, либо at в формате HH:MM или YYYY-MM-DD HH:MM.",
		props: map[string]any{
			"text":       str("о чём напомнить"),
			"in_minutes": num("через сколько минут"),
			"at":         str("точное время, HH:MM или YYYY-MM-DD HH:MM"),
		},
		required: []string{"text"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Text      string
				InMinutes float64 `json:"in_minutes"`
				At        string
			}
			if err := decode(args, &a); err != nil {
				return "", err
			}
			now := s.env.now()
			var when time.Time
			switch {
			case a.InMinutes > 0:
				when = now.Add(time.Duration(a.InMinutes * float64(time.Minute)))
			case a.At != "":
				t, err := ParseWhen(now, a.At)
				if err != nil {
					return "", err
				}
				when = t
			default:
				return "", fmt.Errorf("specify in_minutes or at")
			}
			if !when.After(now) {
				return "", fmt.Errorf("that time is in the past")
			}
			s.add(when.Sub(now), "Напоминание. "+a.Text)
			return "Напоминание поставлено на " + when.Format("02.01.2006 15:04"), nil
		},
	}
}
