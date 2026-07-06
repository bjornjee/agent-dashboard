package web

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/stretchr/testify/mock"
)

// The web surface must run the same prune/sweep cycle as the TUI: a headless
// web-only deployment otherwise never removes dead-pane files or records
// dead_pane tombstones.
func TestServerPruneDeadOnce(t *testing.T) {
	m := withMockTmuxRunner(t)
	// TmuxListLivePaneIDs: only %0 lives, under server PID 100.
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{pid}",
	).Return([]byte("%0\t100\n"), nil)

	srv, stateDir := createTestHandlerWithCfg(t, nil,
		domain.Agent{SessionID: "alive", State: "running", TmuxPaneID: "%0"},
		domain.Agent{SessionID: "dead", State: "done", TmuxPaneID: "%9"},
	)

	removed := srv.pruneDeadOnce()
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(state.AgentsDir(stateDir), "dead.json")); !os.IsNotExist(err) {
		t.Fatal("dead-pane agent file survived the web prune cycle")
	}
	if _, err := os.Stat(filepath.Join(state.AgentsDir(stateDir), "alive.json")); err != nil {
		t.Fatalf("live agent file removed: %v", err)
	}
}

// A failed or empty enumeration must skip the cycle — mirroring the TUI's
// guard — instead of treating every agent as dead.
func TestServerPruneDeadOnce_SkipsOnFailedEnumeration(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{pid}",
	).Return([]byte(""), nil)

	srv, stateDir := createTestHandlerWithCfg(t, nil,
		domain.Agent{SessionID: "dead", State: "done", TmuxPaneID: "%9"},
	)

	if removed := srv.pruneDeadOnce(); removed != 0 {
		t.Fatalf("removed = %d, want 0 on empty enumeration", removed)
	}
	if _, err := os.Stat(filepath.Join(state.AgentsDir(stateDir), "dead.json")); err != nil {
		t.Fatalf("agent file pruned despite failed enumeration: %v", err)
	}
}
