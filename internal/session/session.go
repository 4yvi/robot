// Package session runs the dialog state machine: wake word, listening,
// thinking, speaking, follow-up and session memory.
package session

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/4yvi/robot/internal/audio"
	"github.com/4yvi/robot/internal/clarify"
	"github.com/4yvi/robot/internal/config"
	"github.com/4yvi/robot/internal/llm"
	"github.com/4yvi/robot/internal/memory"
	"github.com/4yvi/robot/internal/stt"
	"github.com/4yvi/robot/internal/tools"
	"github.com/4yvi/robot/internal/tts"
	"github.com/4yvi/robot/internal/wake"
)

// State is what the assistant is doing right now (shown by the tray icon).
type State string

const (
	StateIdle      State = "idle"      // waiting for the wake word
	StateListening State = "listening" // recording the user
	StateThinking  State = "thinking"  // model / tools
	StateSpeaking  State = "speaking"
	StatePaused    State = "paused"
)

// Mic is a source of 16 kHz mono PCM chunks.
type Mic interface {
	Start(ctx context.Context) (<-chan []int16, error)
}

// Deps are the replaceable components the manager is built from.
type Deps struct {
	Cfg         *config.Config
	Mic         Mic
	Player      audio.Player
	NewDetector func(word string) (wake.Detector, error)
	STT         stt.Transcriber
	LLM         llm.Client
	Clarify     *clarify.Step
	Tools       *tools.Registry
	Speaker     *tts.Speaker
	Store       memory.Store
	OnState     func(State)
	OnUserText  func(string) // what the user just asked, for the status display
}

// Manager owns the dialog loop.
type Manager struct {
	d Deps

	detMu sync.Mutex
	det   wake.Detector

	routeMu sync.Mutex
	route   func([]int16)

	wakeCh   chan struct{}
	reloadCh chan struct{}
	speakMu  sync.Mutex

	paused atomic.Bool
	state  atomic.Value // State

	sessMu sync.Mutex
	cur    *sess
	busy   bool
}

// New creates a manager. Call Run to start it.
func New(d Deps) (*Manager, error) {
	det, err := d.NewDetector(d.Cfg.WakeWord)
	if err != nil {
		return nil, err
	}
	m := &Manager{d: d, det: det, wakeCh: make(chan struct{}, 1), reloadCh: make(chan struct{}, 1)}
	m.state.Store(StateIdle)
	return m, nil
}

// State returns the current state.
func (m *Manager) State() State { return m.state.Load().(State) }

func (m *Manager) setState(s State) {
	if m.paused.Load() && s == StateIdle {
		s = StatePaused
	}
	if old := m.state.Swap(s); old != s && m.d.OnState != nil {
		m.d.OnState(s)
	}
}

// SetPaused stops (or resumes) reacting to the wake word.
func (m *Manager) SetPaused(p bool) {
	m.paused.Store(p)
	if m.State() == StateIdle || m.State() == StatePaused {
		if p {
			m.setState(StatePaused)
		} else {
			m.setState(StateIdle)
		}
	}
}

// Paused reports whether the assistant is paused.
func (m *Manager) Paused() bool { return m.paused.Load() }

// SetWakeWord replaces the detector with one for the new word.
func (m *Manager) SetWakeWord(word string) error {
	det, err := m.d.NewDetector(word)
	if err != nil {
		return err
	}
	m.detMu.Lock()
	old := m.det
	m.det = det
	m.detMu.Unlock()
	old.Close()
	return nil
}

// SetOnUserText sets the callback that receives each recognized user phrase.
func (m *Manager) SetOnUserText(f func(string)) { m.d.OnUserText = f }

// SetOnState sets the callback invoked on every state change.
func (m *Manager) SetOnState(f func(State)) { m.d.OnState = f }

// Reload restarts microphone capture (after the microphone setting changed).
func (m *Manager) Reload() {
	select {
	case m.reloadCh <- struct{}{}:
	default:
	}
}

func (m *Manager) detAccept(c []int16) bool {
	m.detMu.Lock()
	defer m.detMu.Unlock()
	return m.det.Accept(c)
}

func (m *Manager) detReset() {
	m.detMu.Lock()
	defer m.detMu.Unlock()
	m.det.Reset()
}

// Close releases the detector.
func (m *Manager) Close() {
	m.detMu.Lock()
	defer m.detMu.Unlock()
	m.det.Close()
}

// setRoute directs microphone chunks to f and returns a function restoring the previous route.
func (m *Manager) setRoute(f func([]int16)) (restore func()) {
	m.routeMu.Lock()
	prev := m.route
	m.route = f
	m.routeMu.Unlock()
	return func() {
		m.routeMu.Lock()
		m.route = prev
		m.routeMu.Unlock()
	}
}

func (m *Manager) pump(chunks <-chan []int16) {
	for c := range chunks {
		m.routeMu.Lock()
		r := m.route
		m.routeMu.Unlock()
		if r != nil {
			r(c)
		}
	}
}

func (m *Manager) idleRoute(c []int16) {
	if m.paused.Load() {
		return
	}
	if m.detAccept(c) {
		select {
		case m.wakeCh <- struct{}{}:
		default:
		}
	}
}

// Run blocks, serving the wake word and dialogs until ctx ends.
func (m *Manager) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		micCtx, cancel := context.WithCancel(ctx)
		chunks, err := m.d.Mic.Start(micCtx)
		if err != nil {
			cancel()
			slog.Error("microphone", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
			continue
		}
		m.setRoute(m.idleRoute)
		go m.pump(chunks)
		m.setState(StateIdle)
		slog.Info("listening for wake word", "word", m.d.Cfg.WakeWord)
		m.loop(micCtx)
		cancel()
	}
	m.sessMu.Lock()
	cur := m.cur
	m.sessMu.Unlock()
	if cur != nil {
		m.finalize(cur) // save the session on shutdown
	}
	return ctx.Err()
}

func (m *Manager) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.reloadCh:
			return
		case <-m.wakeCh:
			slog.Info("wake word detected")
			m.handleSession(ctx)
			m.detReset()
			m.setRoute(m.idleRoute) // fresh route after the dialog
			m.setState(StateIdle)
			select { // drop wake hits caused by our own speech
			case <-m.wakeCh:
			default:
			}
		}
	}
}

// record captures one phrase from the microphone.
func (m *Manager) record(ctx context.Context, waitMS int) ([]int16, error) {
	ch := make(chan []int16, 256)
	restore := m.setRoute(func(c []int16) {
		select {
		case ch <- c:
		default:
		}
	})
	defer restore()
	vad := audio.NewVAD(m.d.Cfg.SilenceMS, m.d.Cfg.MaxPhraseSeconds*1000)
	return audio.RecordPhrase(ctx, ch, vad, waitMS)
}

// listen records a phrase and transcribes it. It returns "" if nobody spoke.
func (m *Manager) listen(ctx context.Context, waitMS int) (string, error) {
	m.setState(StateListening)
	pcm, err := m.record(ctx, waitMS)
	if errors.Is(err, audio.ErrNoSpeech) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	m.setState(StateThinking)
	text, err := m.d.STT.Transcribe(ctx, audio.EncodeWAV(pcm, audio.SampleRate, audio.Channels))
	if err != nil {
		return "", err
	}
	slog.Info("recognized", "text", text)
	return text, nil
}

func (m *Manager) beep(ctx context.Context) {
	if m.d.Player == nil {
		return
	}
	m.speakMu.Lock()
	defer m.speakMu.Unlock()
	_ = m.d.Player.Play(ctx, audio.Beep(880, 140))
	sleep(ctx, 120*time.Millisecond)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// Say speaks text on its own (timers, reminders). Safe to call from any goroutine.
func (m *Manager) Say(text string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		m.speakText(ctx, text)
	}()
}

func (m *Manager) speakText(ctx context.Context, text string) {
	m.speakMu.Lock()
	defer m.speakMu.Unlock()
	prev := m.State()
	m.setState(StateSpeaking)
	if err := m.d.Speaker.Speak(ctx, text); err != nil {
		slog.Error("speak", "err", err)
	}
	if prev == StateSpeaking {
		prev = StateIdle
	}
	m.setState(prev)
}

// Confirm asks a yes/no question by voice.
func (m *Manager) Confirm(ctx context.Context, question string) (bool, error) {
	drop := m.setRoute(func([]int16) {})
	m.speakText(ctx, question)
	sleep(ctx, 250*time.Millisecond)
	drop()
	text, err := m.listen(ctx, 8000)
	if err != nil {
		return false, err
	}
	yes := IsYes(text)
	slog.Info("confirmation", "answer", text, "yes", yes)
	return yes, nil
}

// IsStop reports whether text is one of the session-ending phrases.
func IsStop(text string, phrases []string) bool {
	words := fields(text)
	if len(words) == 0 || len(words) > 2 {
		return false
	}
	for _, p := range phrases {
		pw := fields(p)
		if len(pw) == 0 {
			continue
		}
		if strings.Join(words, " ") == strings.Join(pw, " ") {
			return true
		}
		for _, w := range words {
			if len(pw) == 1 && w == pw[0] {
				return true
			}
		}
	}
	return false
}

// IsYes reports whether the answer is an agreement.
func IsYes(text string) bool {
	yes := false
	for _, w := range fields(text) {
		switch w {
		case "нет", "не", "отмена", "отменить", "стоп":
			return false
		case "да", "давай", "ага", "конечно", "выполняй", "подтверждаю", "угу", "выполни":
			yes = true
		}
	}
	return yes
}

func fields(s string) []string {
	s = strings.ToLower(strings.ReplaceAll(s, "ё", "е"))
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
