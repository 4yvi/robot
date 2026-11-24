//go:build darwin

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func platformOpen(ctx context.Context, target string, isURL bool) error {
	args := []string{target}
	if !isURL {
		if _, err := os.Stat(target); err != nil {
			args = []string{"-a", target} // application name such as "Safari"
		}
	}
	return exec.CommandContext(ctx, "open", args...).Run()
}

func osascript(ctx context.Context, script string) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// playerScript sends a command to whichever of Spotify or Music is running.
func playerScript(cmd string) string {
	return fmt.Sprintf(`
if application "Spotify" is running then
	tell application "Spotify" to %[1]s
else if application "Music" is running then
	tell application "Music" to %[1]s
else
	error "no media player is running"
end if`, cmd)
}

func platformMedia(ctx context.Context, action string, level int) error {
	var script string
	switch action {
	case "volume_up":
		script = "set volume output volume (((output volume of (get volume settings)) + 10) as integer) with output muted false"
	case "volume_down":
		script = "set volume output volume (((output volume of (get volume settings)) - 10) as integer)"
	case "mute":
		script = "set volume output muted not (output muted of (get volume settings))"
	case "set_volume":
		script = fmt.Sprintf("set volume output volume %d", level)
	case "play_pause":
		script = playerScript("playpause")
	case "next":
		script = playerScript("next track")
	case "previous":
		script = playerScript("previous track")
	default:
		return fmt.Errorf("unsupported action %s", action)
	}
	if out, err := osascript(ctx, script); err != nil {
		return fmt.Errorf("osascript: %v %s", err, out)
	}
	return nil
}

func clipboardRead(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "pbpaste").Output()
	return string(out), err
}

func clipboardWrite(ctx context.Context, text string) error {
	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = bytes.NewBufferString(text)
	return cmd.Run()
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/zsh", "-c", command)
}
