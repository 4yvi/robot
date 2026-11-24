//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func shortcutPath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA is not set")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "Robot.lnk"), nil
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// SetAutostart creates or removes a shortcut in the user's Startup folder.
func SetAutostart(enable bool) error {
	p, err := shortcutPath()
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
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s); $s.TargetPath=%s; $s.WorkingDirectory=%s; $s.Save()`,
		psQuote(p), psQuote(exe), psQuote(filepath.Dir(exe)))
	if out, err := hidden(exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)).CombinedOutput(); err != nil {
		return fmt.Errorf("create shortcut: %v: %s", err, out)
	}
	return nil
}

// AutostartEnabled reports whether the Startup shortcut exists.
func AutostartEnabled() bool {
	p, err := shortcutPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	createMutex = kernel32.NewProc("CreateMutexW")
	closeHandle = kernel32.NewProc("CloseHandle")
)

const errorAlreadyExists = 183

// Lock takes a named mutex; ok is false if another instance holds it.
func Lock(name string) (release func(), ok bool, err error) {
	n, err := syscall.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return nil, false, err
	}
	h, _, callErr := createMutex.Call(0, 1, uintptr(unsafe.Pointer(n)))
	if h == 0 {
		return nil, false, callErr
	}
	if errno, _ := callErr.(syscall.Errno); errno == errorAlreadyExists {
		_, _, _ = closeHandle.Call(h)
		return nil, false, nil
	}
	return func() { _, _, _ = closeHandle.Call(h) }, true, nil
}

// OpenPath opens a file or folder with the default application.
func OpenPath(p string) error {
	return hidden(exec.Command("cmd", "/c", "start", "", p)).Start()
}

// PromptText shows an input dialog. ok is false if the user cancelled.
func PromptText(title, message, def string) (text string, ok bool) {
	script := fmt.Sprintf(`Add-Type -AssemblyName Microsoft.VisualBasic; [Console]::OutputEncoding=[Text.Encoding]::UTF8; [Microsoft.VisualBasic.Interaction]::InputBox(%s, %s, %s)`,
		psQuote(message), psQuote(title), psQuote(def))
	out, err := hidden(exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)).Output()
	if err != nil {
		return "", false
	}
	text = strings.TrimSpace(string(out))
	return text, text != ""
}
