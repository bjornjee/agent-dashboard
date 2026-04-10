package db

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const versionTable = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);
`

// runMigrations applies all embedded SQL migration files that haven't been run yet.
// Each migration runs in its own transaction; partial failure leaves the DB at the
// last successfully applied version.
func runMigrations(conn *sqlx.DB) error {
	// Ensure the version tracking table exists.
	if _, err := conn.Exec(versionTable); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	// Read current version.
	var current int
	_ = conn.Get(&current, "SELECT COALESCE(MAX(version), 0) FROM schema_version")

	// Bootstrap: if schema_version is empty but tables already exist,
	// detect the current schema state and set the starting version.
	if current == 0 {
		current = detectExistingVersion(conn)
		if current > 0 {
			// Record all versions up to detected as already applied.
			for v := 1; v <= current; v++ {
				_, _ = conn.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (?)", v)
			}
		}
	}

	// Discover migration files.
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename to ensure numeric ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for i, entry := range entries {
		version := i + 1 // 001_initial.sql → version 1, etc.
		if version <= current {
			continue
		}

		sql, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if err := execMigration(conn, version, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// execMigration runs a single migration's SQL statements in a transaction,
// then records the version.
func execMigration(conn *sqlx.DB, version int, sql string) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}

	// Split on semicolons and execute each statement individually.
	// This avoids driver-specific multi-statement behavior.
	for _, stmt := range splitStatements(sql) {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", truncate(stmt, 80), err)
		}
	}

	if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", version); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// splitStatements splits SQL text on semicolons, trimming whitespace and
// discarding empty/comment-only fragments.
func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	var stmts []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		// Skip empty or comment-only fragments
		if s == "" || isCommentOnly(s) {
			continue
		}
		stmts = append(stmts, s)
	}
	return stmts
}

// isCommentOnly returns true if the string contains only SQL comments and whitespace.
func isCommentOnly(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		return false
	}
	return true
}

// detectExistingVersion inspects the database schema to determine which
// migrations have already been applied before the migration system existed.
// Returns 0 for a fresh DB, 1 if the original schema exists, 2 if the
// provider column is already present.
func detectExistingVersion(conn *sqlx.DB) int {
	// Check if daily_usage table exists at all.
	var tableCount int
	_ = conn.Get(&tableCount, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='daily_usage'")
	if tableCount == 0 {
		return 0 // fresh DB
	}

	// Check if provider column exists (migration 002).
	var providerCount int
	_ = conn.Get(&providerCount, "SELECT COUNT(*) FROM pragma_table_info('daily_usage') WHERE name='provider'")
	if providerCount > 0 {
		return 2 // provider column already added
	}

	return 1 // original schema without provider
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
