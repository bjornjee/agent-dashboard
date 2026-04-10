package db

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func openRawConn(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestRunMigrations_FreshDB(t *testing.T) {
	conn := openRawConn(t)
	if err := runMigrations(conn); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Verify schema_version has 2 entries
	var count int
	conn.Get(&count, "SELECT COUNT(*) FROM schema_version")
	if count != 2 {
		t.Errorf("schema_version count = %d, want 2", count)
	}

	// Verify provider column exists
	var provCount int
	conn.Get(&provCount, "SELECT COUNT(*) FROM pragma_table_info('daily_usage') WHERE name='provider'")
	if provCount != 1 {
		t.Errorf("provider column not found after migrations")
	}

	// Verify quotes table exists
	var quotesCount int
	conn.Get(&quotesCount, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='quotes'")
	if quotesCount != 1 {
		t.Errorf("quotes table not found after migrations")
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	conn := openRawConn(t)

	// Run twice — second run should be a no-op.
	if err := runMigrations(conn); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runMigrations(conn); err != nil {
		t.Fatalf("second run: %v", err)
	}

	var count int
	conn.Get(&count, "SELECT COUNT(*) FROM schema_version")
	if count != 2 {
		t.Errorf("schema_version count = %d after 2 runs, want 2", count)
	}
}

func TestRunMigrations_BootstrapV1(t *testing.T) {
	conn := openRawConn(t)

	// Simulate a pre-migration DB with the original schema (no provider column).
	conn.MustExec(`
		CREATE TABLE daily_usage (
			date TEXT NOT NULL, session_id TEXT NOT NULL,
			model TEXT DEFAULT '', input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0, cache_read_tokens INTEGER DEFAULT 0,
			cache_write_tokens INTEGER DEFAULT 0, cost_usd REAL DEFAULT 0,
			updated_at DATETIME, PRIMARY KEY (date, session_id)
		)
	`)
	conn.MustExec(`INSERT INTO daily_usage (date, session_id, cost_usd) VALUES ('2026-01-01', 'sess-1', 5.0)`)

	if err := runMigrations(conn); err != nil {
		t.Fatalf("runMigrations on V1 DB: %v", err)
	}

	// Should have detected V1 and only run migration 002.
	var version int
	conn.Get(&version, "SELECT MAX(version) FROM schema_version")
	if version != 2 {
		t.Errorf("max version = %d, want 2", version)
	}

	// Provider column should exist.
	var provCount int
	conn.Get(&provCount, "SELECT COUNT(*) FROM pragma_table_info('daily_usage') WHERE name='provider'")
	if provCount != 1 {
		t.Errorf("provider column not found after bootstrap+migration")
	}

	// Existing data should be preserved with provider='claude'.
	var cost float64
	conn.Get(&cost, "SELECT cost_usd FROM daily_usage WHERE session_id='sess-1' AND provider='claude'")
	if cost != 5.0 {
		t.Errorf("existing row cost = %f, want 5.0", cost)
	}
}

func TestRunMigrations_BootstrapV2(t *testing.T) {
	conn := openRawConn(t)

	// Simulate a DB that was already migrated by the old migrateProvider code
	// (has provider column but no schema_version table).
	conn.MustExec(`
		CREATE TABLE daily_usage (
			date TEXT NOT NULL, session_id TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT 'claude',
			model TEXT DEFAULT '', input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0, cache_read_tokens INTEGER DEFAULT 0,
			cache_write_tokens INTEGER DEFAULT 0, cost_usd REAL DEFAULT 0,
			updated_at DATETIME, PRIMARY KEY (date, session_id, provider)
		)
	`)
	conn.MustExec(`INSERT INTO daily_usage (date, session_id, provider, cost_usd) VALUES ('2026-01-01', 'sess-1', 'claude', 3.0)`)

	if err := runMigrations(conn); err != nil {
		t.Fatalf("runMigrations on V2 DB: %v", err)
	}

	// Should detect V2 and skip all migrations.
	var version int
	conn.Get(&version, "SELECT MAX(version) FROM schema_version")
	if version != 2 {
		t.Errorf("max version = %d, want 2", version)
	}

	// Existing data should be untouched.
	var cost float64
	conn.Get(&cost, "SELECT cost_usd FROM daily_usage WHERE session_id='sess-1'")
	if cost != 3.0 {
		t.Errorf("existing row cost = %f, want 3.0", cost)
	}
}

func TestSplitStatements(t *testing.T) {
	sql := `
-- Comment only
CREATE TABLE foo (id INTEGER);

-- Another comment
INSERT INTO foo VALUES (1);
INSERT INTO foo VALUES (2);
`
	stmts := splitStatements(sql)
	if len(stmts) != 3 {
		t.Fatalf("got %d statements, want 3: %v", len(stmts), stmts)
	}
}

func TestSplitStatements_EmptyInput(t *testing.T) {
	stmts := splitStatements("")
	if len(stmts) != 0 {
		t.Errorf("got %d statements for empty input, want 0", len(stmts))
	}
}
