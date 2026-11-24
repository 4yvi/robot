package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/4yvi/robot/internal/memory"
)

// printHistory writes recent sessions. With full=true it includes every message.
func printHistory(w io.Writer, store memory.Store, limit int, full bool) error {
	ctx := context.Background()
	list, err := store.Sessions(ctx, limit)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Fprintln(w, "No sessions yet.")
		return nil
	}
	for _, s := range list {
		fmt.Fprintf(w, "# Session %d, %s, %d messages\n", s.ID, s.Started.Local().Format("2006-01-02 15:04"), s.Messages)
		if s.Summary != "" {
			fmt.Fprintf(w, "Summary: %s\n", s.Summary)
		}
		if full {
			msgs, err := store.Messages(ctx, s.ID)
			if err != nil {
				return err
			}
			for _, m := range msgs {
				label := m.Role
				if m.ToolName != "" {
					label += " " + m.ToolName
				}
				fmt.Fprintf(w, "  [%s] %s\n", label, strings.ReplaceAll(strings.TrimSpace(m.Content), "\n", " "))
			}
		}
		fmt.Fprintln(w)
	}
	facts, err := store.Facts(ctx)
	if err == nil && len(facts) > 0 {
		fmt.Fprintln(w, "# Known facts")
		for _, f := range facts {
			fmt.Fprintln(w, "- "+f)
		}
	}
	return nil
}

// cmdHistory implements `robot history [N] [--full]`.
func cmdHistory(args []string) {
	limit, full := 10, false
	for _, a := range args {
		if a == "--full" || a == "-f" {
			full = true
		} else if n, err := strconv.Atoi(a); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openStore()
	fatal(err)
	defer store.Close()
	fatal(printHistory(stdout(), store, limit, full))
}
