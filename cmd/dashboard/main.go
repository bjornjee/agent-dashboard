package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ownTarget resolves the dashboard's own tmux pane to a target string
// so we can exclude it from the agent list.
func ownTarget() string {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane,
		"#{session_name}:#{window_index}.#{pane_index}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func main() {
	statePath := DefaultStatePath()

	// Clean stale agents (>10 min since last update) on startup
	CleanStale(statePath, 10*60)

	db, err := OpenDB(DefaultDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: usage DB not available: %v\n", err)
	}
	if db != nil {
		defer db.Close()
	}

	self := ownTarget()
	m := newModel(statePath, self, db)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start file watcher
	watcher, err := watchStateFile(statePath, p)
	if err != nil {
		// Non-fatal: dashboard works without live updates
		fmt.Fprintf(os.Stderr, "warning: file watcher not available: %v\n", err)
	}
	if watcher != nil {
		defer watcher.Close()
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
