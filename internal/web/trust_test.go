package web

import (
	"context"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
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
	if out[0].TrustPromptDetected {
		t.Errorf("agent a (pane %%1) should NOT be flagged")
	}
	if !out[1].TrustPromptDetected {
		t.Errorf("agent b (pane %%2) SHOULD be flagged")
	}
	if out[2].TrustPromptDetected {
		t.Errorf("agent c (no pane id) should NOT be flagged")
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
