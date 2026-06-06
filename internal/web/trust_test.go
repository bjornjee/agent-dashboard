package web

import (
	"context"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/stretchr/testify/mock"
)

func newTrustTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	return NewServer(cfg, nil, ServerOptions{})
}

// TestServer_TrustPaneRoundtrip locks the in-memory trust tracker:
// markTrustPane / isTrustPane / clearTrustPane behave like a set keyed
// by tmux pane_id.
func TestServer_TrustPaneRoundtrip(t *testing.T) {
	s := newTrustTestServer(t)

	if s.isTrustPane("%7") {
		t.Fatalf("expected fresh server to have no trust panes")
	}
	s.markTrustPane("%7")
	if !s.isTrustPane("%7") {
		t.Fatalf("markTrustPane should set the flag")
	}
	s.clearTrustPane("%7")
	if s.isTrustPane("%7") {
		t.Fatalf("clearTrustPane should remove the flag")
	}
}

// TestServer_ApplyTrustFlags asserts that applyTrustFlags stamps
// TrustPromptDetected onto agents whose TmuxPaneID is in the trust map
// and leaves others untouched.
func TestServer_ApplyTrustFlags(t *testing.T) {
	s := newTrustTestServer(t)
	s.markTrustPane("%2")

	agents := []domain.Agent{
		{SessionID: "a", TmuxPaneID: "%1"},
		{SessionID: "b", TmuxPaneID: "%2"},
		{SessionID: "c", TmuxPaneID: ""},
	}
	out := s.applyTrustFlags(agents)
	// Find each by SessionID — applyTrustFlags may reorder via placeholder
	// injection for unmatched trust panes.
	byID := map[string]domain.Agent{}
	for _, a := range out {
		byID[a.SessionID] = a
	}
	if byID["a"].TrustPromptDetected {
		t.Errorf("agent a (pane %%1) should NOT be flagged")
	}
	if !byID["b"].TrustPromptDetected {
		t.Errorf("agent b (pane %%2) SHOULD be flagged")
	}
	if byID["c"].TrustPromptDetected {
		t.Errorf("agent c (no pane id) should NOT be flagged")
	}
}

// TestServer_ApplyTrustFlagsSynthesizesPlaceholder asserts that when a
// trust pane has no matching agent (because the harness blocks on the
// trust dialog before writing its SessionStart hook payload), a
// placeholder Agent is injected so the dashboard can render the trust
// chip + toast even without a real state file.
func TestServer_ApplyTrustFlagsSynthesizesPlaceholder(t *testing.T) {
	s := newTrustTestServer(t)
	// Stage a spawn pin so the placeholder can pick up the folder.
	if err := state.WriteSpawnPin(s.cfg.Profile.StateDir, state.SpawnPin{
		PaneID:      "%99",
		Target:      "main:1.0",
		WorktreeCwd: "/Users/me/Library/Sounds",
	}); err != nil {
		t.Fatalf("write spawn pin: %v", err)
	}
	s.markTrustPane("%99")

	out := s.applyTrustFlags(nil)
	if len(out) != 1 {
		t.Fatalf("want 1 synthesized agent, got %d", len(out))
	}
	a := out[0]
	if a.TmuxPaneID != "%99" {
		t.Errorf("placeholder pane id = %q, want %%99", a.TmuxPaneID)
	}
	if !a.TrustPromptDetected {
		t.Errorf("placeholder should have TrustPromptDetected=true")
	}
	if a.Cwd != "/Users/me/Library/Sounds" {
		t.Errorf("placeholder cwd = %q, want /Users/me/Library/Sounds", a.Cwd)
	}
	if a.Target != "main:1.0" {
		t.Errorf("placeholder target = %q, want main:1.0", a.Target)
	}
	if a.State != "running" {
		t.Errorf("placeholder state = %q, want running", a.State)
	}
	if a.SessionID == "" {
		t.Errorf("placeholder must carry a synthetic SessionID for routing")
	}
}

// TestServer_ApplyTrustFlagsNoPlaceholderWhenAgentExists asserts that
// once the harness writes its state file (post-trust-accept), the
// placeholder is NOT injected — the real agent gets stamped instead.
func TestServer_ApplyTrustFlagsNoPlaceholderWhenAgentExists(t *testing.T) {
	s := newTrustTestServer(t)
	_ = state.WriteSpawnPin(s.cfg.Profile.StateDir, state.SpawnPin{
		PaneID: "%99", Target: "main:1.0", WorktreeCwd: "/x",
	})
	s.markTrustPane("%99")

	agents := []domain.Agent{
		{SessionID: "real", TmuxPaneID: "%99", State: "running"},
	}
	out := s.applyTrustFlags(agents)
	if len(out) != 1 {
		t.Fatalf("want 1 agent (no placeholder), got %d", len(out))
	}
	if !out[0].TrustPromptDetected {
		t.Errorf("real agent should be stamped")
	}
	if out[0].SessionID != "real" {
		t.Errorf("real agent should remain, got %q", out[0].SessionID)
	}
}

// TestServer_WatchTrustPromptDetectsAndBroadcasts asserts the polling
// loop marks the pane and broadcasts an SSE message when the harness's
// trust prompt appears in the captured pane buffer.
func TestServer_WatchTrustPromptDetectsAndBroadcasts(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Output", mock.Anything,
		"capture-pane", "-p", "-t", "main:1.0", "-S", "-30",
	).Return([]byte("Claude Code\nYes, I trust this folder\nNo, exit\n"), nil)
	// On detection the watcher broadcasts a fresh readAgentState snapshot,
	// which hits TmuxIsAvailable + TmuxListPanes.
	mockReadAgentState(m)

	s := newTrustTestServer(t)
	// Subscribe to the SSE hub so we can assert the broadcast.
	ch := s.hub.register()
	defer s.hub.unregister(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.watchTrustPrompt(ctx, "%9", "main:1.0", 1*time.Second, 50*time.Millisecond)

	if !s.isTrustPane("%9") {
		t.Fatalf("expected pane %%9 to be marked trust after watchTrustPrompt detected the prompt")
	}

	select {
	case <-ch:
		// got the broadcast
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected SSE broadcast after trust detection, none received")
	}
}

// TestServer_WatchTrustPromptTimesOutCleanly asserts that when no trust
// prompt appears within the budget, watchTrustPrompt exits without
// marking the pane.
func TestServer_WatchTrustPromptTimesOutCleanly(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Output", mock.Anything,
		"capture-pane", "-p", "-t", "main:2.0", "-S", "-30",
	).Return([]byte("Just running...\nNo prompt here.\n"), nil)

	s := newTrustTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.watchTrustPrompt(ctx, "%11", "main:2.0", 300*time.Millisecond, 50*time.Millisecond)

	if s.isTrustPane("%11") {
		t.Fatalf("expected pane %%11 to NOT be marked when prompt never appears")
	}
}
