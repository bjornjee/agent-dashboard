package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/lock"
	"github.com/bjornjee/agent-dashboard/internal/tui"
)

// Version is set at build time via -ldflags "-X main.Version=..."
var Version string

func main() {
	if Version != "" {
		tui.Version = Version
	}

	cfg := config.DefaultConfig()
	stateDir := cfg.Profile.StateDir

	// Singleton lock — only one dashboard instance at a time.
	lockFile, err := lock.AcquireLock(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer lockFile.Close()

	dbPath := filepath.Join(stateDir, "usage.db")
	database, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: usage DB not available: %v\n", err)
	}
	if database != nil {
		defer database.Close()
	}

	m := tui.NewModel(cfg, database)

	// Debug key log — writes raw key/mouse events for diagnosing phantom keystrokes.
	// Enable with [debug] key_log = true in settings.toml.
	if cfg.Settings.Debug.KeyLog {
		debugLogPath := filepath.Join(stateDir, "debug-keys.log")
		if debugLog, err := os.OpenFile(debugLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
			m.DebugKeyLog = debugLog
			defer debugLog.Close()
			fmt.Fprintf(debugLog, "=== dashboard key debug log started %s ===\n", time.Now().Format(time.RFC3339))
		}
	}

	p := tea.NewProgram(m)

	// Start directory watcher for per-agent state files.
	// m.TmuxReady is an atomic.Bool updated by deferredStartup once the
	// real TmuxIsAvailable() check completes; the watcher reads it on
	// each event to decide whether to call tmux for target resolution.
	watcher, err := tui.WatchStateDir(stateDir, p, m.TmuxReady)
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
