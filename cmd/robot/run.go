package main

import (
	"fmt"
	"os"

	"github.com/4yvi/robot/internal/config"
)

// runCommand handles the default (background) mode and unknown commands.
func runCommand(cmd string, cfg *config.Config) {
	if cmd != "" {
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(2)
	}
	fmt.Println("robot", version, "- background mode is not implemented yet")
}
