// Package platform holds OS-specific helpers: autostart, single instance,
// native dialogs and opening files.
package platform

import (
	"os"
	"path/filepath"
)

// ExePath returns the real path of the running executable. On macOS inside an
// app bundle this is the binary in Robot.app/Contents/MacOS.
func ExePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}
