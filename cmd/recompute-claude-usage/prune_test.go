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

func countSession(t *testing.T, conn *sqlx.DB, sessionID string) int {
	t.Helper()
	var n int
	if err := conn.Get(&n, "SELECT COUNT(*) FROM daily_usage WHERE session_id=?", sessionID); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// ---- Tier 1: --prune-duplicates ----

func TestRunPruneDuplicates_KeepsMaxPerCluster(t *testing.T) {
	conn, _ := setupDB(t)
	// Cluster: 3 rows under prefix '019aaaaa-bbbb' on same date.
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7111-aaaa-111111111111", "claude", 100.0)
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7222-aaaa-222222222222", "claude", 200.0)
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7333-aaaa-333333333333", "claude", 300.0)

	var buf bytes.Buffer
	deleted, reclaimed, err := runPruneDuplicates(conn, false, &buf)
	if err != nil {
		t.Fatalf("runPruneDuplicates: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted: got %d, want 2", deleted)
	}
	if reclaimed < 299.99 || reclaimed > 300.01 {
		t.Errorf("reclaimed: got %f, want ~300", reclaimed)
	}
	// Only the $300 row should survive.
	if got := countSession(t, conn, "019aaaaa-bbbb-7333-aaaa-333333333333"); got != 1 {
		t.Errorf("max-cost row deleted: %d remaining", got)
	}
	if got := countSession(t, conn, "019aaaaa-bbbb-7111-aaaa-111111111111"); got != 0 {
		t.Errorf("smaller row still present")
	}
}

func TestRunPruneDuplicates_IgnoresSingleton(t *testing.T) {
	conn, _ := setupDB(t)
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7111-aaaa-111111111111", "claude", 100.0)

	var buf bytes.Buffer
	deleted, _, err := runPruneDuplicates(conn, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("singleton pruned: %d", deleted)
	}
	if got := countSession(t, conn, "019aaaaa-bbbb-7111-aaaa-111111111111"); got != 1 {
		t.Errorf("singleton deleted")
	}
}

func TestRunPruneDuplicates_ScopedByDate(t *testing.T) {
	conn, _ := setupDB(t)
	// Same prefix, different dates → not a cluster.
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7111-aaaa-111111111111", "claude", 100.0)
	insertRow(t, conn, "2026-05-23", "019aaaaa-bbbb-7222-aaaa-222222222222", "claude", 200.0)

	var buf bytes.Buffer
	deleted, _, err := runPruneDuplicates(conn, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("cross-date prune occurred: %d", deleted)
	}
}

func TestRunPruneDuplicates_IgnoresUUIDv4(t *testing.T) {
	conn, _ := setupDB(t)
	// UUIDv4 rows must never be pruned by cluster rule.
	insertRow(t, conn, "2026-05-22", "abcdef12-3456-4789-9abc-def012345678", "claude", 100.0)
	insertRow(t, conn, "2026-05-22", "abcdef12-3456-4111-9abc-def012345001", "claude", 200.0)

	var buf bytes.Buffer
	deleted, _, err := runPruneDuplicates(conn, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("UUIDv4 pruned: %d", deleted)
	}
}

func TestRunPruneDuplicates_DryRun(t *testing.T) {
	conn, _ := setupDB(t)
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7111-aaaa-111111111111", "claude", 100.0)
	insertRow(t, conn, "2026-05-22", "019aaaaa-bbbb-7222-aaaa-222222222222", "claude", 200.0)

	var buf bytes.Buffer
	deleted, reclaimed, err := runPruneDuplicates(conn, true, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 1 {
		t.Errorf("dry-run reported wrong delete count: %d", deleted)
	}
	if reclaimed < 99.99 || reclaimed > 100.01 {
		t.Errorf("dry-run reclaimed: %f", reclaimed)
	}
	// Nothing actually deleted.
	var n int
	conn.Get(&n, "SELECT COUNT(*) FROM daily_usage WHERE session_id LIKE '019%'")
	if n != 2 {
		t.Errorf("dry-run deleted rows: %d remaining (want 2)", n)
	}
}

// ---- Tier 2: --prune-tests ----

func TestRunPruneTests_DeletesKnownNames(t *testing.T) {
	conn, _ := setupDB(t)
	insertRow(t, conn, "2026-05-22", "evidence-test", "claude", 61.73)
	insertRow(t, conn, "2026-05-22", "test-codex-session", "claude", 9.17)
	insertRow(t, conn, "2026-05-22", "test-codex-debug", "claude", 2.51)
	insertRow(t, conn, "2026-05-22", "real-session-keep", "claude", 100.0)

	var buf bytes.Buffer
	deleted, reclaimed, err := runPruneTests(conn, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted: got %d, want 3", deleted)
	}
	want := 61.73 + 9.17 + 2.51
	if reclaimed < want-0.01 || reclaimed > want+0.01 {
		t.Errorf("reclaimed: got %f, want %f", reclaimed, want)
	}
	if got := countSession(t, conn, "real-session-keep"); got != 1 {
		t.Errorf("real session deleted")
	}
	if got := countSession(t, conn, "evidence-test"); got != 0 {
		t.Errorf("test row still present")
	}
}

func TestRunPruneTests_DryRun(t *testing.T) {
	conn, _ := setupDB(t)
	insertRow(t, conn, "2026-05-22", "evidence-test", "claude", 61.73)

	var buf bytes.Buffer
	deleted, _, err := runPruneTests(conn, true, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 1 {
		t.Errorf("dry-run delete count: %d", deleted)
	}
	if got := countSession(t, conn, "evidence-test"); got != 1 {
		t.Errorf("dry-run deleted row")
	}
}

// ---- Strict-accuracy: --prune-orphans (delete any unverifiable claude row) ----

// writeClaudeJSONL creates an empty JSONL at <projectsDir>/<slug>/<sid>.jsonl
// so FindProjDirByScan locates the session.
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

func TestRunPruneOrphans_DeletesUnverifiable(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	insertRow(t, conn, "2026-05-22", "no-jsonl-anywhere", "claude", 99.99)

	var buf bytes.Buffer
	deleted, reclaimed, err := runPruneOrphans(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted: got %d, want 1", deleted)
	}
	if reclaimed < 99.98 || reclaimed > 100.0 {
		t.Errorf("reclaimed: got %f, want ~99.99", reclaimed)
	}
	if got := countSession(t, conn, "no-jsonl-anywhere"); got != 0 {
		t.Errorf("orphan still present")
	}
}

func TestRunPruneOrphans_PreservesResolvable(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	writeClaudeJSONL(t, projects, "-Users-foo", "verified-sess")
	insertRow(t, conn, "2026-05-22", "verified-sess", "claude", 12.34)

	var buf bytes.Buffer
	deleted, _, err := runPruneOrphans(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("verified row deleted: %d", deleted)
	}
	if got := countSession(t, conn, "verified-sess"); got != 1 {
		t.Errorf("verified row missing")
	}
}

func TestRunPruneOrphans_IgnoresCodexProvider(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	insertRow(t, conn, "2026-05-22", "codex-daily", "codex", 50.0)

	var buf bytes.Buffer
	deleted, _, err := runPruneOrphans(conn, projects, false, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 0 {
		t.Errorf("codex row pruned: %d", deleted)
	}
	if got := countSession(t, conn, "codex-daily"); got != 1 {
		t.Errorf("codex row missing")
	}
}

func TestRunPruneOrphans_DryRun(t *testing.T) {
	conn, _ := setupDB(t)
	projects := t.TempDir()
	insertRow(t, conn, "2026-05-22", "no-jsonl-anywhere", "claude", 99.99)

	var buf bytes.Buffer
	deleted, _, err := runPruneOrphans(conn, projects, true, &buf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if deleted != 1 {
		t.Errorf("dry-run count: %d", deleted)
	}
	if got := countSession(t, conn, "no-jsonl-anywhere"); got != 1 {
		t.Errorf("dry-run deleted row")
	}
}
