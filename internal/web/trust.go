package web

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// trustPaneRecord carries the metadata needed to synthesize a
// placeholder agent for a trust-blocked pane. We track folder + target
// here rather than reading them back from the SpawnPin because
// StageSpawnPin only populates WorktreeCwd for git folders — non-git
// trust-prompt targets (Library/Sounds, mktemp dirs, fresh home subdirs)
// leave the pin's WorktreeCwd empty and the placeholder would render
// as "unknown".
type trustPaneRecord struct {
	Folder string
	Target string
}

// trustWatchBudget is how long the post-spawn poller waits for a folder
// trust prompt before giving up. Long enough for slow harness startups,
// short enough that a stalled goroutine doesn't outlive the user's
// patience with the spawn flow.
const trustWatchBudget = 30 * time.Second

// trustWatchTick is the polling cadence. Matches the watcher debounce
// (300ms) so a typical trust prompt surfaces within one or two ticks.
const trustWatchTick = 300 * time.Millisecond

// markTrustPane records that paneID is waiting on a folder-trust prompt.
// folder + target are needed so applyTrustFlags can synthesize a
// placeholder agent before the harness's SessionStart hook fires (which
// happens AFTER the user accepts trust — too late for the dashboard).
func (s *Server) markTrustPane(paneID, folder, target string) {
	if paneID == "" {
		return
	}
	s.trustMu.Lock()
	s.trustPanes[paneID] = trustPaneRecord{Folder: folder, Target: target}
	s.trustMu.Unlock()
}

// isTrustPane returns true if a trust prompt has been detected for
// paneID and has not been cleared.
func (s *Server) isTrustPane(paneID string) bool {
	if paneID == "" {
		return false
	}
	s.trustMu.Lock()
	_, ok := s.trustPanes[paneID]
	s.trustMu.Unlock()
	return ok
}

// clearTrustPane removes paneID from the trust set. No-op when absent.
func (s *Server) clearTrustPane(paneID string) {
	if paneID == "" {
		return
	}
	s.trustMu.Lock()
	delete(s.trustPanes, paneID)
	s.trustMu.Unlock()
}

// applyTrustFlags stamps TrustPromptDetected onto agents whose
// TmuxPaneID is in the trust set, and synthesizes placeholder agents
// for trust panes that have no matching state file. Claude Code and
// codex both block on the harness trust dialog BEFORE firing the
// SessionStart hook, so a real spawn in an untrusted folder has no
// agent record yet. Without the placeholder, the chip + toast surface
// has no row to land on and the user sees nothing.
//
// Placeholder data is read from the trustPaneRecord (folder + target)
// captured at markTrustPane time. Synthetic SessionID is derived from
// the pane_id so the placeholder remains stable across SSE ticks; when
// the real state file later appears with the same TmuxPaneID, this
// function stamps that agent directly and skips the placeholder.
func (s *Server) applyTrustFlags(agents []domain.Agent) []domain.Agent {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	if len(s.trustPanes) == 0 {
		return agents
	}
	matched := map[string]bool{}
	for i := range agents {
		if agents[i].TmuxPaneID == "" {
			continue
		}
		if _, ok := s.trustPanes[agents[i].TmuxPaneID]; ok {
			agents[i].TrustPromptDetected = true
			matched[agents[i].TmuxPaneID] = true
		}
	}
	var placeholders []domain.Agent
	for paneID, rec := range s.trustPanes {
		if matched[paneID] {
			continue
		}
		sess, win, pane, _ := tmux.ParseTarget(rec.Target)
		placeholders = append(placeholders, domain.Agent{
			SessionID:           "trust-pending-" + paneID,
			TmuxPaneID:          paneID,
			Target:              rec.Target,
			Session:             sess,
			Window:              win,
			Pane:                pane,
			Cwd:                 rec.Folder,
			State:               "running",
			TrustPromptDetected: true,
			Harness:             s.cfg.Harness.Name(),
		})
	}
	if len(placeholders) > 0 {
		// Prepend so trust-blocked agents are visually first — they're
		// the most urgent thing the user can act on.
		agents = append(placeholders, agents...)
	}
	return agents
}

// watchTrustPrompt polls the pane's tmux buffer for up to budget,
// marking paneID and broadcasting a fresh state snapshot via the SSE
// hub on first detection. Exits early on context cancellation,
// detection, or repeated capture errors. Designed to be called as a
// goroutine right after a successful spawn.
func (s *Server) watchTrustPrompt(ctx context.Context, paneID, target, folder string, budget, tick time.Duration) {
	if paneID == "" || target == "" {
		return
	}
	deadline := time.Now().Add(budget)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	check := func() bool {
		lines, err := tmux.TmuxCapture(target, 30)
		if err != nil {
			// Pane likely killed or tmux unavailable; bail.
			return true
		}
		if tmux.ContainsTrustPrompt(lines) {
			s.markTrustPane(paneID, folder, target)
			if data, mErr := json.Marshal(s.readAgentState()); mErr == nil {
				s.hub.broadcast(data)
			}
			return true
		}
		return false
	}

	if check() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) || check() {
				return
			}
		}
	}
}
