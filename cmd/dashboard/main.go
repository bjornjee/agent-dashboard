package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// ownPaneID returns the dashboard's own tmux pane ID (%N format)
// so we can exclude it from the agent list.
func ownPaneID() string {
	return os.Getenv("TMUX_PANE")
}

func main() {
	stateDir := DefaultStateDir()

	// Clean stale agents (>10 min since last update) on startup
	CleanStale(stateDir, 10*60)

	db, err := OpenDB(DefaultDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: usage DB not available: %v\n", err)
	}
	if db != nil {
		defer db.Close()
	}

	selfPane := ownPaneID()
	m := newModel(stateDir, selfPane, db)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start directory watcher for per-agent state files
	watcher, err := watchStateDir(stateDir, p)
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
