//go:build !darwin && !windows

package platform

import "errors"

var errUnsupported = errors.New("not supported on this OS")

func SetAutostart(bool) error                          { return errUnsupported }
func AutostartEnabled() bool                           { return false }
func Lock(string) (func(), bool, error)                { return func() {}, true, nil }
func OpenPath(string) error                            { return errUnsupported }
func PromptText(string, string, string) (string, bool) { return "", false }
