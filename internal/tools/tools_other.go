//go:build !darwin && !windows

package tools

import (
	"context"
	"errors"
	"os/exec"
)

var errUnsupported = errors.New("not supported on this OS")

func platformOpen(context.Context, string, bool) error { return errUnsupported }
func platformMedia(context.Context, string, int) error { return errUnsupported }
func clipboardRead(context.Context) (string, error)    { return "", errUnsupported }
func clipboardWrite(context.Context, string) error     { return errUnsupported }
func shellCommand(ctx context.Context, c string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", c)
}
