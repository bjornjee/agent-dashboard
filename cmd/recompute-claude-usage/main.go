// Command recompute-claude-usage is a one-shot fixup for claude usage rows in
// daily_usage. Two modes:
//
//	default            : re-read every claude session's JSONL with the deduped
//	                     parser and rewrite per-date rows preserving the existing
//	                     per-date ratio. Sessions whose JSONL is no longer locatable
//	                     under ~/.claude/projects are left untouched.
//
//	--prune-orphans    : delete every provider='claude' row whose session_id no
//	                     longer resolves to a JSONL under ~/.claude/projects.
//	                     These rows are unrecoverable noise (stale renames,
//	                     deleted sessions) — pruning them gives the dashboard
//	                     an honest total.
//
// Usage:
//
//	go run ./cmd/recompute-claude-usage [--db PATH] [--projects DIR] [--dry-run]
//	go run ./cmd/recompute-claude-usage --prune-orphans [--db PATH] [--projects DIR] [--dry-run]
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

func main() {
	home, _ := os.UserHomeDir()
	defaultDB := home + "/.agent-dashboard/usage.db"
	defaultProj := home + "/.claude/projects"

	dbPath := flag.String("db", defaultDB, "path to usage.db")
	projectsDir := flag.String("projects", defaultProj, "Claude Code projects dir")
	dryRun := flag.Bool("dry-run", false, "report what would change but do not write")
	prune := flag.Bool("prune-orphans", false, "delete claude rows whose session_id can't be resolved to a JSONL (mutually exclusive with default recompute)")
	flag.Parse()

	conn, err := sqlx.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.MustExec("PRAGMA journal_mode=WAL")

	if *prune {
		count, reclaimed, err := runPrune(conn, *projectsDir, *dryRun, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prune: %v\n", err)
			os.Exit(1)
		}
		suffix := ""
		if *dryRun {
			suffix = "   [DRY RUN — no rows deleted]"
		}
		fmt.Printf("pruned: %d sessions, $%.2f reclaimed%s\n", count, reclaimed, suffix)
		return
	}

	if err := runRecompute(conn, *projectsDir, *dryRun, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "recompute: %v\n", err)
		os.Exit(1)
	}
}

// runPrune deletes provider='claude' rows whose session_id can't be resolved
// to a JSONL under projectsDir. Returns the number of pruned sessions and the
// total reclaimed cost.
func runPrune(conn *sqlx.DB, projectsDir string, dryRun bool, w io.Writer) (int, float64, error) {
	var sessions []sessionInfo
	if err := conn.Select(&sessions, `
		SELECT DISTINCT session_id, COALESCE(model, '') AS model
		FROM daily_usage
		WHERE provider = 'claude'`); err != nil {
		return 0, 0, fmt.Errorf("list sessions: %w", err)
	}

	var count int
	var reclaimed float64

	for _, sess := range sessions {
		if sess.SessionID == "" {
			continue
		}
		if conversation.FindProjDirByScan(projectsDir, sess.SessionID) != "" {
			continue
		}

		var (
			rows int
			cost float64
		)
		if err := conn.Get(&rows, `
			SELECT COUNT(*) FROM daily_usage WHERE provider='claude' AND session_id=?`,
			sess.SessionID); err != nil {
			return count, reclaimed, fmt.Errorf("count rows for %s: %w", sess.SessionID, err)
		}
		if err := conn.Get(&cost, `
			SELECT COALESCE(SUM(cost_usd), 0) FROM daily_usage WHERE provider='claude' AND session_id=?`,
			sess.SessionID); err != nil {
			return count, reclaimed, fmt.Errorf("sum cost for %s: %w", sess.SessionID, err)
		}
		if rows == 0 {
			continue
		}

		tag := ""
		if dryRun {
			tag = " [dry-run]"
		}
		fmt.Fprintf(w, "[prune]%s %s  -$%.2f  (%d rows)\n", tag, sess.SessionID, cost, rows)

		if !dryRun {
			if _, err := conn.Exec(`
				DELETE FROM daily_usage WHERE provider='claude' AND session_id=?`,
				sess.SessionID); err != nil {
				return count, reclaimed, fmt.Errorf("delete %s: %w", sess.SessionID, err)
			}
		}
		count++
		reclaimed += cost
	}
	return count, reclaimed, nil
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
