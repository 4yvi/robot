// Package config loads and saves robot's YAML configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all user settings.
type Config struct {
	WakeWord              string            `yaml:"wake_word"`
	MicDevice             string            `yaml:"mic_device"`
	SpeakerDevice         string            `yaml:"speaker_device"`
	VoskModelPath         string            `yaml:"vosk_model_path"`
	WhisperURL            string            `yaml:"whisper_url"`
	WhisperServerPath     string            `yaml:"whisper_server_path"`
	WhisperModelPath      string            `yaml:"whisper_model_path"`
	WhisperModelURL       string            `yaml:"whisper_model_url"`
	LMStudioURL           string            `yaml:"lmstudio_url"`
	ModelName             string            `yaml:"model_name"`
	TTSEngine             string            `yaml:"tts_engine"` // auto, sherpa, piper or say (macOS)
	SayVoice              string            `yaml:"say_voice"`
	SherpaPath            string            `yaml:"sherpa_path"`
	SherpaVoiceDir        string            `yaml:"sherpa_voice_dir"`
	SpeechSpeed           float64           `yaml:"speech_speed"`
	BargeIn               bool              `yaml:"barge_in"`
	PiperPath             string            `yaml:"piper_path"`
	PiperVoice            string            `yaml:"piper_voice"`
	PromptPath            string            `yaml:"prompt_path"`
	SilenceMS             int               `yaml:"silence_ms"`
	MaxPhraseSeconds      int               `yaml:"max_phrase_seconds"`
	FollowUpSeconds       int               `yaml:"follow_up_seconds"`
	SessionTimeoutMinutes int               `yaml:"session_timeout_minutes"`
	MaxClarifications     int               `yaml:"max_clarifications"`
	MaxToolCalls          int               `yaml:"max_tool_calls"`
	StopPhrases           []string          `yaml:"stop_phrases"`
	Autostart             bool              `yaml:"autostart"`
	AllowedFolders        []string          `yaml:"allowed_folders"`
	NotesFile             string            `yaml:"notes_file"`
	Apps                  map[string]string `yaml:"apps"`

	mu   sync.Mutex
	path string
}

// Default returns the default configuration.
func Default() *Config {
	return &Config{
		WakeWord:              "робот",
		VoskModelPath:         "models/vosk-model-small-ru",
		WhisperURL:            "http://127.0.0.1:8080",
		WhisperServerPath:     "models/whisper/whisper-server" + exeSuffix(),
		WhisperModelPath:      "models/whisper/ggml-small.bin",
		WhisperModelURL:       "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin",
		LMStudioURL:           "http://localhost:1234/v1",
		TTSEngine:             "auto",
		SherpaPath:            "models/sherpa/bin/sherpa-onnx-offline-tts" + exeSuffix(),
		SherpaVoiceDir:        "models/sherpa/voice",
		SpeechSpeed:           1.0,
		BargeIn:               true,
		SayVoice:              "Milena",
		PiperPath:             "models/piper/piper" + exeSuffix(),
		PiperVoice:            "models/piper/ru_RU-irina-medium.onnx",
		PromptPath:            "prompt.md",
		SilenceMS:             1200,
		MaxPhraseSeconds:      30,
		FollowUpSeconds:       8,
		SessionTimeoutMinutes: 10,
		MaxClarifications:     2,
		MaxToolCalls:          6,
		StopPhrases:           []string{"хватит", "пока", "стоп"},
		Apps:                  map[string]string{},
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// Dir returns the per-user data directory (config, logs, database).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "robot")
	return d, os.MkdirAll(d, 0o755)
}

// FindPath returns the config file path: next to the executable if it exists
// there, otherwise in the user config directory.
func FindPath() (string, error) {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p := "config.yaml"; fileExists(p) {
		return filepath.Abs(p)
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// Load reads the config at path. A missing file yields defaults.
func Load(path string) (*Config, error) {
	c := Default()
	c.path = path
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Apps == nil {
		c.Apps = map[string]string{}
	}
	return c, nil
}

// Path returns the file the config was loaded from.
func (c *Config) Path() string { return c.path }

// Update applies fn under the lock and saves the config to disk.
func (c *Config) Update(fn func(*Config)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c)
	return c.saveLocked()
}

// Save writes the config to disk.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

func (c *Config) saveLocked() error {
	if c.path == "" {
		return fmt.Errorf("config has no path")
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// ResolvePath resolves a relative path. It is looked up next to the config
// file, next to the executable, in the app bundle's Resources folder and in
// the working directory; the first existing candidate wins. If none exists the
// path is placed next to the config file.
func (c *Config) ResolvePath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	var dirs []string
	if c.path != "" {
		dirs = append(dirs, filepath.Dir(c.path))
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, d, filepath.Join(d, "..", "Resources"))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		cand := filepath.Join(d, p)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if len(dirs) > 0 {
		return filepath.Join(dirs[0], p)
	}
	return p
}
