//go:build windows

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func hide(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

// platformOpen opens a URL or program. URLs go through rundll32 so characters
// like & in a query string are not interpreted by cmd.exe; programs from the
// config use `start`.
func platformOpen(ctx context.Context, target string, isURL bool) error {
	if isURL {
		return hide(exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target)).Run()
	}
	return hide(exec.CommandContext(ctx, "cmd", "/c", "start", "", target)).Run()
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	keybdEvt = user32.NewProc("keybd_event")
)

const (
	vkVolumeMute = 0xAD
	vkVolumeDown = 0xAE
	vkVolumeUp   = 0xAF
	vkNext       = 0xB0
	vkPrevious   = 0xB1
	vkPlayPause  = 0xB3
	keyUp        = 0x0002
)

func pressKey(vk uintptr, times int) {
	for i := 0; i < times; i++ {
		_, _, _ = keybdEvt.Call(vk, 0, 0, 0)
		_, _, _ = keybdEvt.Call(vk, 0, keyUp, 0)
	}
}

func platformMedia(ctx context.Context, action string, level int) error {
	switch action {
	case "volume_up":
		pressKey(vkVolumeUp, 5) // each press is 2%
	case "volume_down":
		pressKey(vkVolumeDown, 5)
	case "mute":
		pressKey(vkVolumeMute, 1)
	case "set_volume":
		pressKey(vkVolumeDown, 50)
		pressKey(vkVolumeUp, level/2)
	case "play_pause":
		pressKey(vkPlayPause, 1)
	case "next":
		pressKey(vkNext, 1)
	case "previous":
		pressKey(vkPrevious, 1)
	default:
		return fmt.Errorf("unsupported action %s", action)
	}
	return nil
}

func powershell(ctx context.Context, script string) *exec.Cmd {
	return hide(exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script))
}

func clipboardRead(ctx context.Context) (string, error) {
	out, err := powershell(ctx, "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw").Output()
	return strings.TrimRight(string(out), "\r\n"), err
}

func clipboardWrite(ctx context.Context, text string) error {
	cmd := powershell(ctx, "[Console]::InputEncoding=[Text.Encoding]::UTF8; $t=[Console]::In.ReadToEnd(); Set-Clipboard -Value $t")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return powershell(ctx, "[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+command)
}
