// Command recompute-claude-usage is a one-shot fixup for the message.id
// double-counting bug. It walks every Claude session referenced in
// daily_usage, re-reads its JSONL with the (now-deduped) parser, and rewrites
// each session's per-date rows preserving the existing per-date ratio.
//
// Sessions whose JSONL is no longer locatable under ~/.claude/projects are
// left untouched. Codex provider rows are ignored.
//
// Usage:
//
//	go run ./cmd/recompute-claude-usage [--db PATH] [--projects DIR] [--dry-run]
package main

import (
	"flag"
	"fmt"
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
	flag.Parse()

	conn, err := sqlx.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	conn.MustExec("PRAGMA journal_mode=WAL")

	var sessions []sessionInfo
	if err := conn.Select(&sessions, `
		SELECT DISTINCT session_id, COALESCE(model, '') AS model
		FROM daily_usage
		WHERE provider = 'claude'`); err != nil {
		fmt.Fprintf(os.Stderr, "list sessions: %v\n", err)
		os.Exit(1)
	}

	var processed, resolved, skipped int
	var totalBefore, totalAfter float64

	for _, sess := range sessions {
		if sess.SessionID == "" || sess.SessionID == "codex-daily" {
			skipped++
			continue
		}
		projDir := conversation.FindProjDirByScan(*projectsDir, sess.SessionID)
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
			fmt.Fprintf(os.Stderr, "load rows for %s: %v\n", sess.SessionID, err)
			os.Exit(1)
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

		if *dryRun {
			fmt.Printf("[dry-run] %s  $%.2f -> $%.2f  (%d rows)\n",
				sess.SessionID, oldTotal, corrected.CostUSD, len(rows))
			processed++
			continue
		}

		for i, r := range rows {
			scaled := scaleRow(corrected, oldTotal, r.CostUSD, i == len(rows)-1, model)
			if err := upsert(conn, r.Date, sess.SessionID, model, scaled); err != nil {
				fmt.Fprintf(os.Stderr, "upsert %s/%s: %v\n", r.Date, sess.SessionID, err)
				os.Exit(1)
			}
		}
		processed++
	}

	fmt.Printf("sessions: %d total, %d resolved (rewritten), %d skipped (unresolvable or codex)\n",
		len(sessions), resolved, skipped)
	fmt.Printf("claude total: $%.2f -> $%.2f", totalBefore, totalAfter)
	if *dryRun {
		fmt.Print("   [DRY RUN — no rows written]")
	}
	fmt.Println()
	_ = processed
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
