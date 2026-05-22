package usage

import (
	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// claudeRecomputeV1Marker is the meta key recording that the one-time fixup
// for the message.id double-counting bug has already run on this DB.
const claudeRecomputeV1Marker = "claude_recompute_v1"

// ProjDirResolver maps (projectsDir, sessionID) → the absolute path of the
// projects/<slug> directory that contains <sessionID>.jsonl, or "" when it
// cannot be located (e.g. the slug has been deleted).
type ProjDirResolver func(projectsDir, sessionID string) string

// RecomputeClaudeUsageOnce runs RecomputeClaudeUsage exactly once per DB. The
// first successful run writes a marker into the meta table; subsequent calls
// short-circuit. Safe to call on every dashboard startup.
func RecomputeClaudeUsageOnce(database *db.DB, projectsDir string, resolver ProjDirResolver) error {
	if database == nil {
		return nil
	}
	if database.MetaGet(claudeRecomputeV1Marker) != "" {
		return nil
	}
	if err := RecomputeClaudeUsage(database, projectsDir, resolver); err != nil {
		return err
	}
	return database.MetaSet(claudeRecomputeV1Marker, "done")
}

// RecomputeClaudeUsage re-reads every Claude session referenced in daily_usage
// using the corrected (message.id-deduped) parser and rewrites each session's
// per-date rows, preserving the existing per-date ratio. Sessions whose JSONL
// can no longer be located are left untouched.
func RecomputeClaudeUsage(database *db.DB, projectsDir string, resolver ProjDirResolver) error {
	if database == nil {
		return nil
	}
	for _, sess := range database.DistinctSessionsForProvider("claude") {
		// Defensive: skip the codex sentinel even if it leaked into the claude
		// provider (it shouldn't, but daily_usage uses a (date, session_id) PK
		// without provider, so historical rows could have crossed).
		if sess.SessionID == "" || sess.SessionID == "codex-daily" {
			continue
		}
		projDir := resolver(projectsDir, sess.SessionID)
		if projDir == "" {
			continue
		}
		corrected := ReadUsage(projDir, sess.SessionID)
		rows := database.SessionDaysForProvider(sess.SessionID, "claude")
		if len(rows) == 0 {
			continue
		}

		oldTotalCost := 0.0
		for _, r := range rows {
			oldTotalCost += r.CostUSD
		}

		model := sess.Model
		if model == "" {
			model = corrected.Model
		}

		// When the seeded total is zero we can't proportion-scale, so place
		// the corrected cumulative on the most-recent row and zero the rest.
		if oldTotalCost <= 0 {
			for i, r := range rows {
				u := domain.Usage{Model: model}
				if i == len(rows)-1 {
					u = domain.Usage{
						InputTokens:      corrected.InputTokens,
						OutputTokens:     corrected.OutputTokens,
						CacheReadTokens:  corrected.CacheReadTokens,
						CacheWriteTokens: corrected.CacheWriteTokens,
						CostUSD:          corrected.CostUSD,
						Model:            model,
					}
				}
				if err := database.UpsertUsageWithProvider(r.Date, sess.SessionID, "claude", model, u); err != nil {
					return err
				}
			}
			continue
		}

		for _, r := range rows {
			ratio := r.CostUSD / oldTotalCost
			u := domain.Usage{
				InputTokens:      int(float64(corrected.InputTokens) * ratio),
				OutputTokens:     int(float64(corrected.OutputTokens) * ratio),
				CacheReadTokens:  int(float64(corrected.CacheReadTokens) * ratio),
				CacheWriteTokens: int(float64(corrected.CacheWriteTokens) * ratio),
				CostUSD:          corrected.CostUSD * ratio,
				Model:            model,
			}
			if err := database.UpsertUsageWithProvider(r.Date, sess.SessionID, "claude", model, u); err != nil {
				return err
			}
		}
	}
	return nil
}
