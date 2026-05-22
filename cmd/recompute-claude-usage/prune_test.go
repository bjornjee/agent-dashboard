package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// setupDB creates a temp daily_usage DB with the production schema.
func setupDB(t *testing.T) (*sqlx.DB, string) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	conn, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.MustExec(`CREATE TABLE daily_usage (
		date TEXT NOT NULL, session_id TEXT NOT NULL,
		provider TEXT NOT NULL DEFAULT 'claude',
		model TEXT DEFAULT '',
		input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
		cache_read_tokens INTEGER DEFAULT 0, cache_write_tokens INTEGER DEFAULT 0,
		cost_usd REAL DEFAULT 0, updated_at DATETIME,
		PRIMARY KEY (date, session_id, provider)
	)`)
	return conn, dbPath
}

func insertRow(t *testing.T, conn *sqlx.DB, date, sessionID, provider string, cost float64) {
	t.Helper()
	_, err := conn.Exec(`INSERT INTO daily_usage (date, session_id, provider, cost_usd) VALUES (?, ?, ?, ?)`,
		date, sessionID, provider, cost)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func countRows(t *testing.T, conn *sqlx.DB, sessionID, provider string) int {
	t.Helper()
	var n int
	err := conn.Get(&n, "SELECT COUNT(*) FROM daily_usage WHERE session_id=? AND provider=?", sessionID, provider)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// writeClaudeJSONL creates an empty JSONL at <projectsDir>/<slug>/<sid>.jsonl
// so FindProjDirByScan can locate the session.
func writeClaudeJSONL(t *testing.T, projectsDir, slug, sessionID string) {
	t.Helper()
	dir := filepath.Join(projectsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestRunPrune_DeletesOrphan(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir() // empty: no JSONL anywhere
	insertRow(t, conn, "2026-05-22", "orphan-no-jsonl", "claude", 99.99)

	var buf bytes.Buffer
	count, reclaimed, err := runPrune(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if count != 1 {
		t.Errorf("pruned count: got %d, want 1", count)
	}
	if reclaimed < 99.98 || reclaimed > 100.0 {
		t.Errorf("reclaimed: got %f, want ~99.99", reclaimed)
	}
	if got := countRows(t, conn, "orphan-no-jsonl", "claude"); got != 0 {
		t.Errorf("orphan still present: %d rows", got)
	}
}

func TestRunPrune_PreservesResolvable(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	writeClaudeJSONL(t, projects, "-Users-foo", "real-sess")
	insertRow(t, conn, "2026-05-22", "real-sess", "claude", 12.34)

	var buf bytes.Buffer
	count, _, err := runPrune(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if count != 0 {
		t.Errorf("resolvable session pruned: count=%d", count)
	}
	if got := countRows(t, conn, "real-sess", "claude"); got != 1 {
		t.Errorf("resolvable row deleted: %d rows", got)
	}
}

func TestRunPrune_IgnoresCodexProvider(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	insertRow(t, conn, "2026-05-22", "codex-daily", "codex", 50.0)

	var buf bytes.Buffer
	count, _, err := runPrune(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if count != 0 {
		t.Errorf("codex row pruned: count=%d", count)
	}
	if got := countRows(t, conn, "codex-daily", "codex"); got != 1 {
		t.Errorf("codex row deleted: %d rows", got)
	}
}

func TestRunPrune_DryRunNoDelete(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	insertRow(t, conn, "2026-05-22", "orphan-dry", "claude", 7.0)

	var buf bytes.Buffer
	count, reclaimed, err := runPrune(conn, projects, true, &buf)
	if err != nil {
		t.Fatalf("runPrune: %v", err)
	}
	if count != 1 {
		t.Errorf("dry-run count: got %d, want 1", count)
	}
	if reclaimed < 6.99 || reclaimed > 7.01 {
		t.Errorf("dry-run reclaimed: got %f, want ~7.0", reclaimed)
	}
	if got := countRows(t, conn, "orphan-dry", "claude"); got != 1 {
		t.Errorf("dry-run deleted row: %d remaining", got)
	}
}
