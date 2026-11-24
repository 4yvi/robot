//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const agentLabel = "io.github.robot-assistant"

func agentPath() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "Library", "LaunchAgents", agentLabel+".plist"), nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// appBundle returns the Robot.app path if exe lives inside a bundle.
func appBundle(exe string) string {
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+4]
	}
	return ""
}

// AutostartPlist renders the LaunchAgent for exe.
func AutostartPlist(exe string) string {
	args := fmt.Sprintf("\t\t<string>%s</string>\n", xmlEscape(exe))
	if app := appBundle(exe); app != "" {
		// start through `open` so macOS attributes microphone access to the bundle
		args = fmt.Sprintf("\t\t<string>/usr/bin/open</string>\n\t\t<string>-a</string>\n\t\t<string>%s</string>\n", xmlEscape(app))
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + agentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
` + args + `	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
}

// SetAutostart installs or removes the LaunchAgent in ~/Library/LaunchAgents.
func SetAutostart(enable bool) error {
	p, err := agentPath()
	if err != nil {
		return err
	}
	if !enable {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := ExePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(AutostartPlist(exe)), 0o644)
}

// AutostartEnabled reports whether the LaunchAgent exists.
func AutostartEnabled() bool {
	p, err := agentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Lock takes an exclusive per-user lock; ok is false if another instance holds it.
func Lock(name string) (release func(), ok bool, err error) {
	dir := os.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false, nil
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, true, nil
}

// OpenPath opens a file or folder with the default application.
func OpenPath(p string) error { return exec.Command("open", p).Start() }

func osaEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// PromptText shows a native input dialog. ok is false if the user cancelled.
func PromptText(title, message, def string) (text string, ok bool) {
	script := fmt.Sprintf(`text returned of (display dialog "%s" with title "%s" default answer "%s" buttons {"Отмена", "OK"} default button "OK")`,
		osaEscape(message), osaEscape(title), osaEscape(def))
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
