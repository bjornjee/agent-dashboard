package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg := DefaultConfig()
	stateDir := cfg.Profile.StateDir

	dbPath := filepath.Join(stateDir, "usage.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: usage DB not available: %v\n", err)
	}
	if db != nil {
		defer db.Close()
	}

	m := newModel(cfg, db)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start directory watcher for per-agent state files.
	// m.tmuxReady is an atomic.Bool updated by deferredStartup once the
	// real TmuxIsAvailable() check completes; the watcher reads it on
	// each event to decide whether to call tmux for target resolution.
	watcher, err := watchStateDir(stateDir, p, m.tmuxReady)
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
