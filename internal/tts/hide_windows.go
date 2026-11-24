//go:build windows

package tts

import (
	"os/exec"
	"syscall"
)

// hideWindow stops a console window flashing when piper starts.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
