// Command recompute-claude-usage is a one-shot fixup for claude usage rows in
// daily_usage. Three modes:
//
//	default              : re-read every claude session's JSONL with the deduped
//	                       parser and rewrite per-date rows preserving the
//	                       existing per-date ratio. Sessions whose JSONL is no
//	                       longer locatable under ~/.claude/projects are left
//	                       untouched.
//
//	--prune-duplicates   : within UUIDv7 clusters (rows sharing the same 13-char
//	                       session_id prefix on the same date), keep the max-cost
//	                       row and delete the rest. These are non-canonical
//	                       snapshots of one in-progress session captured under
//	                       multiple ephemeral IDs.
//
//	--prune-tests        : delete rows whose session_id matches known synthetic
//	                       test names (evidence-test, test-codex-session,
//	                       test-codex-debug).
//
// Usage:
//
//	go run ./cmd/recompute-claude-usage [--db PATH] [--projects DIR] [--dry-run]
//	go run ./cmd/recompute-claude-usage --prune-duplicates [--dry-run]
//	go run ./cmd/recompute-claude-usage --prune-tests [--dry-run]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/usage"
)

type sessionInfo struct {
	SessionID string `db:"session_id"`
	Model     string `db:"model"`
}

type dayRow struct {
	Date    string  `db:"date"`
	CostUSD float64 `db:"cost_usd"`
}

// knownTestSessionIDs is the closed set of synthetic session names left in
// daily_usage from earlier debugging. Listed explicitly so we don't accidentally
// nuke a real session whose ID happens to contain "test".
var knownTestSessionIDs = []string{
	"evidence-test",
	"test-codex-session",
	"test-codex-debug",
}

func main() {
	home, _ := os.UserHomeDir()
	defaultDB := home + "/.agent-dashboard/dashboard.db"
	defaultProj := home + "/.claude/projects"

	dbPath := flag.String("db", defaultDB, "path to dashboard.db")
	projectsDir := flag.String("projects", defaultProj, "Claude Code projects dir")
	dryRun := flag.Bool("dry-run", false, "report what would change but do not write")
	pruneDuplicates := flag.Bool("prune-duplicates", false, "within UUIDv7 clusters (same 13-char prefix on same date), keep max-cost row, delete rest")
	pruneTests := flag.Bool("prune-tests", false, "delete rows whose session_id matches known synthetic test names")
	pruneOrphans := flag.Bool("prune-orphans", false, "strict-accuracy: delete any provider='claude' row whose session_id can't be resolved to a JSONL under --projects. Surviving totals undercount real spend but every row is provably verifiable.")
	flag.Parse()

	conn, err := sqlx.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.MustExec("PRAGMA journal_mode=WAL")

	// Modes are mutually exclusive.
	modes := 0
	if *pruneDuplicates {
		modes++
	}
	if *pruneTests {
		modes++
	}
	if *pruneOrphans {
		modes++
	}
	if modes > 1 {
		fmt.Fprintln(os.Stderr, "pick at most one of --prune-duplicates, --prune-tests, --prune-orphans")
		os.Exit(2)
	}

	switch {
	case *pruneOrphans:
		deleted, reclaimed, err := runPruneOrphans(conn, *projectsDir, *dryRun, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune-orphans: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("pruned %d orphan rows, $%.2f reclaimed%s\n",
			deleted, reclaimed, dryRunSuffix(*dryRun))
	case *pruneDuplicates:
		deleted, reclaimed, err := runPruneDuplicates(conn, *dryRun, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune-duplicates: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("pruned %d duplicate rows, $%.2f reclaimed%s\n",
			deleted, reclaimed, dryRunSuffix(*dryRun))
	case *pruneTests:
		deleted, reclaimed, err := runPruneTests(conn, *dryRun, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune-tests: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("pruned %d test rows, $%.2f reclaimed%s\n",
			deleted, reclaimed, dryRunSuffix(*dryRun))
	default:
		if err := runRecompute(conn, *projectsDir, *dryRun, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "recompute: %v\n", err)
			os.Exit(1)
		}
	}
}

func dryRunSuffix(dryRun bool) string {
	if dryRun {
		return "   [DRY RUN — no rows deleted]"
	}
	return ""
}

// runPruneDuplicates removes non-canonical rows from UUIDv7 clusters. Within
// each (13-char prefix, date) group of two-or-more provider='claude' rows
// whose session_id starts with "019", the highest-cost row is kept and the
// rest are deleted. Returns the count of deleted rows and total reclaimed cost.
func runPruneDuplicates(conn *sqlx.DB, dryRun bool, w io.Writer) (int, float64, error) {
	type row struct {
		Prefix    string  `db:"prefix"`
		Date      string  `db:"date"`
		SessionID string  `db:"session_id"`
		CostUSD   float64 `db:"cost_usd"`
	}
	var rows []row
	if err := conn.Select(&rows, `
		SELECT substr(session_id, 1, 13) AS prefix, date, session_id, cost_usd
		FROM daily_usage
		WHERE provider = 'claude' AND session_id LIKE '019%'
		ORDER BY prefix, date, cost_usd DESC`); err != nil {
		return 0, 0, fmt.Errorf("list candidates: %w", err)
	}

	// Group by (prefix, date). The first row of each group is the highest-cost
	// row (kept); all subsequent rows in the group are duplicates (deleted).
	type key struct{ prefix, date string }
	groupSize := make(map[key]int)
	for _, r := range rows {
		groupSize[key{r.Prefix, r.Date}]++
	}

	seen := make(map[key]bool)
	var deleted int
	var reclaimed float64
	for _, r := range rows {
		k := key{r.Prefix, r.Date}
		if groupSize[k] < 2 {
			continue // singleton — not a cluster
		}
		if !seen[k] {
			seen[k] = true // first (highest-cost) → keep
			continue
		}
		fmt.Fprintf(w, "[prune-dup]%s %s  -$%.2f  (cluster %s on %s)\n",
			dryRunTag(dryRun), r.SessionID, r.CostUSD, r.Prefix, r.Date)
		if !dryRun {
			if _, err := conn.Exec(
				`DELETE FROM daily_usage WHERE provider='claude' AND session_id=?`,
				r.SessionID); err != nil {
				return deleted, reclaimed, fmt.Errorf("delete %s: %w", r.SessionID, err)
			}
		}
		deleted++
		reclaimed += r.CostUSD
	}
	return deleted, reclaimed, nil
}

// runPruneTests deletes rows whose session_id matches a closed list of known
// synthetic test names left over from debugging.
func runPruneTests(conn *sqlx.DB, dryRun bool, w io.Writer) (int, float64, error) {
	var deleted int
	var reclaimed float64
	for _, sid := range knownTestSessionIDs {
		var cost float64
		if err := conn.Get(&cost, `
			SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage
			WHERE session_id=?`, sid); err != nil {
			return deleted, reclaimed, fmt.Errorf("sum %s: %w", sid, err)
		}
		var count int
		if err := conn.Get(&count, `SELECT COUNT(*) FROM daily_usage WHERE session_id=?`, sid); err != nil {
			return deleted, reclaimed, fmt.Errorf("count %s: %w", sid, err)
		}
		if count == 0 {
			continue
		}
		fmt.Fprintf(w, "[prune-test]%s %s  -$%.2f  (%d rows)\n",
			dryRunTag(dryRun), sid, cost, count)
		if !dryRun {
			if _, err := conn.Exec(`DELETE FROM daily_usage WHERE session_id=?`, sid); err != nil {
				return deleted, reclaimed, fmt.Errorf("delete %s: %w", sid, err)
			}
		}
		deleted += count
		reclaimed += cost
	}
	return deleted, reclaimed, nil
}

func dryRunTag(dryRun bool) string {
	if dryRun {
		return " [dry-run]"
	}
	return ""
}

// runPruneOrphans deletes every provider='claude' row whose session_id can't
// be resolved to a JSONL under projectsDir. Use when accuracy beats history:
// surviving rows are exactly verifiable; deleted rows represent unrecoverable
// estimates (typically sessions Claude Code retention has removed). Returns
// the number of pruned rows and the total reclaimed cost.
func runPruneOrphans(conn *sqlx.DB, projectsDir string, dryRun bool, w io.Writer) (int, float64, error) {
	var sessions []sessionInfo
	if err := conn.Select(&sessions, `
		SELECT DISTINCT session_id, COALESCE(model, '') AS model
		FROM daily_usage
		WHERE provider = 'claude'`); err != nil {
		return 0, 0, fmt.Errorf("list sessions: %w", err)
	}

	var deleted int
	var reclaimed float64

	for _, sess := range sessions {
		if sess.SessionID == "" {
			continue
		}
		if conversation.FindProjDirByScan(projectsDir, sess.SessionID) != "" {
			continue
		}

		var rows int
		if err := conn.Get(&rows, `
			SELECT COUNT(*) FROM daily_usage WHERE provider='claude' AND session_id=?`,
			sess.SessionID); err != nil {
			return deleted, reclaimed, fmt.Errorf("count %s: %w", sess.SessionID, err)
		}
		var cost float64
		if err := conn.Get(&cost, `
			SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE provider='claude' AND session_id=?`,
			sess.SessionID); err != nil {
			return deleted, reclaimed, fmt.Errorf("sum %s: %w", sess.SessionID, err)
		}
		if rows == 0 {
			continue
		}

		fmt.Fprintf(w, "[prune-orphan]%s %s  -$%.2f  (%d rows)\n",
			dryRunTag(dryRun), sess.SessionID, cost, rows)
		if !dryRun {
			if _, err := conn.Exec(
				`DELETE FROM daily_usage WHERE provider='claude' AND session_id=?`,
				sess.SessionID); err != nil {
				return deleted, reclaimed, fmt.Errorf("delete %s: %w", sess.SessionID, err)
			}
		}
		deleted += rows
		reclaimed += cost
	}
	return deleted, reclaimed, nil
}

// runRecompute walks every claude session in daily_usage, re-reads its JSONL
// with the deduped parser, and rewrites per-date rows preserving the existing
// per-date ratio.
func runRecompute(conn *sqlx.DB, projectsDir string, dryRun bool, w io.Writer) error {
	var sessions []sessionInfo
	if err := conn.Select(&sessions, `
		SELECT DISTINCT session_id, COALESCE(model, '') AS model
		FROM daily_usage
		WHERE provider = 'claude'`); err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	var resolved, skipped int
	var totalBefore, totalAfter float64

	for _, sess := range sessions {
		if sess.SessionID == "" || sess.SessionID == "codex-daily" {
			skipped++
			continue
		}
		projDir := conversation.FindProjDirByScan(projectsDir, sess.SessionID)
		if projDir == "" {
			skipped++
			continue
		}
		resolved++

		corrected := usage.ReadUsage(projDir, sess.SessionID)

		var rows []dayRow
		if err := conn.Select(&rows, `
			SELECT date, cost_usd
			FROM daily_usage
			WHERE session_id = ? AND provider = 'claude'
			ORDER BY date`, sess.SessionID); err != nil {
			return fmt.Errorf("load rows for %s: %w", sess.SessionID, err)
		}
		if len(rows) == 0 {
			continue
		}

		oldTotal := 0.0
		for _, r := range rows {
			oldTotal += r.CostUSD
		}
		totalBefore += oldTotal
		totalAfter += corrected.CostUSD

		model := sess.Model
		if model == "" {
			model = corrected.Model
		}

		if dryRun {
			fmt.Fprintf(w, "[dry-run] %s  $%.2f -> $%.2f  (%d rows)\n",
				sess.SessionID, oldTotal, corrected.CostUSD, len(rows))
			continue
		}

		for i, r := range rows {
			scaled := scaleRow(corrected, oldTotal, r.CostUSD, i == len(rows)-1, model)
			if err := upsert(conn, r.Date, sess.SessionID, model, scaled); err != nil {
				return fmt.Errorf("upsert %s/%s: %w", r.Date, sess.SessionID, err)
			}
		}
	}

	fmt.Fprintf(w, "sessions: %d total, %d resolved (rewritten), %d skipped (unresolvable or codex)\n",
		len(sessions), resolved, skipped)
	fmt.Fprintf(w, "claude total: $%.2f -> $%.2f", totalBefore, totalAfter)
	if dryRun {
		fmt.Fprint(w, "   [DRY RUN — no rows written]")
	}
	fmt.Fprintln(w)
	return nil
}

// scaleRow returns the new domain.Usage for one row, scaled from `corrected`
// (the deduped cumulative for the whole session) by this row's share of the
// old cumulative cost. When oldTotal is zero, the corrected total lands on
// the most-recent row and the rest are zeroed.
func scaleRow(corrected domain.Usage, oldTotal, oldRowCost float64, isLast bool, model string) domain.Usage {
	if oldTotal <= 0 {
		if !isLast {
			return domain.Usage{Model: model}
		}
		corrected.Model = model
		return corrected
	}
	ratio := oldRowCost / oldTotal
	return domain.Usage{
		InputTokens:      int(float64(corrected.InputTokens) * ratio),
		OutputTokens:     int(float64(corrected.OutputTokens) * ratio),
		CacheReadTokens:  int(float64(corrected.CacheReadTokens) * ratio),
		CacheWriteTokens: int(float64(corrected.CacheWriteTokens) * ratio),
		CostUSD:          corrected.CostUSD * ratio,
		Model:            model,
	}
}

func upsert(conn *sqlx.DB, date, sessionID, model string, u domain.Usage) error {
	_, err := conn.Exec(`
		INSERT OR REPLACE INTO daily_usage
			(date, session_id, provider, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, updated_at)
		VALUES (?, ?, 'claude', ?, ?, ?, ?, ?, ?, datetime('now'))`,
		date, sessionID, model,
		u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.CostUSD)
	return err
}
