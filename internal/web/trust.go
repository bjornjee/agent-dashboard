package web

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// trustWatchBudget is how long the post-spawn poller waits for a folder
// trust prompt before giving up. Long enough for slow harness startups,
// short enough that a stalled goroutine doesn't outlive the user's
// patience with the spawn flow.
const trustWatchBudget = 30 * time.Second

// trustWatchTick is the polling cadence. Matches the watcher debounce
// (300ms) so a typical trust prompt surfaces within one or two ticks.
const trustWatchTick = 300 * time.Millisecond

// markTrustPane records that paneID is waiting on a folder-trust prompt.
func (s *Server) markTrustPane(paneID string) {
	if paneID == "" {
		return
	}
	s.trustMu.Lock()
	s.trustPanes[paneID] = struct{}{}
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
// TmuxPaneID is in the trust set. Returns the same slice (mutated in
// place) so callers can chain through state-resolution pipelines.
func (s *Server) applyTrustFlags(agents []domain.Agent) []domain.Agent {
	if len(agents) == 0 {
		return agents
	}
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	if len(s.trustPanes) == 0 {
		return agents
	}
	for i := range agents {
		if agents[i].TmuxPaneID == "" {
			continue
		}
		if _, ok := s.trustPanes[agents[i].TmuxPaneID]; ok {
			agents[i].TrustPromptDetected = true
		}
	}
	return agents
}

// watchTrustPrompt polls the pane's tmux buffer for up to budget,
// marking paneID and broadcasting a fresh state snapshot via the SSE
// hub on first detection. Exits early on context cancellation,
// detection, or repeated capture errors. Designed to be called as a
// goroutine right after a successful spawn.
func (s *Server) watchTrustPrompt(ctx context.Context, paneID, target string, budget, tick time.Duration) {
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
			s.markTrustPane(paneID)
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
