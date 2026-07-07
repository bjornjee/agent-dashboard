package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/stretchr/testify/mock"
)

// GET /api/agents must flag restart-survivors (pane died with a previous
// tmux server, active session, real branch, existing workdir) with
// resumable:true so the frontend Cmd+K palette can mark and resume them.
// Finished agents (done/pr/merged) on dead panes stay false.
func TestHandleAgents_ResumableFlag(t *testing.T) {
	m := withMockTmuxRunner(t)
	// TmuxIsAvailable + list-panes: only the dashboard pane %0 is live under
	// server PID 100, so %2/%3 are dead.
	m.On("Run", mock.Anything, "list-sessions").Return(nil)
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pid}\t#{pane_current_path}\t#{pane_current_command}",
	).Return([]byte("%0\tmain\t0\t0\t100\t/tmp\n"), nil)

	// WorktreeCwd + Branch is the pinned case: ResolveAgentBranches leaves the
	// stamped branch authoritative (a bare Cwd would be re-resolved via git
	// and cleared because the temp dir is not a repo).
	dir := t.TempDir()
	ts, _ := createTestServer(t,
		domain.Agent{SessionID: "running-orphan", State: "running", TmuxPaneID: "%2", TmuxServerPID: "99", Branch: "feat/x", WorktreeCwd: dir},
		domain.Agent{SessionID: "done-orphan", State: "done", TmuxPaneID: "%3", TmuxServerPID: "99", Branch: "feat/y", WorktreeCwd: dir},
	)

	req, _ := http.NewRequest("GET", ts.URL+"/api/agents", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var agents []domain.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatalf("decode: %v", err)
	}

	got := map[string]bool{}
	for _, a := range agents {
		got[a.SessionID] = a.Resumable
	}
	if !got["running-orphan"] {
		t.Error("running agent with dead pane should be resumable")
	}
	if got["done-orphan"] {
		t.Error("done agent should not be resumable")
	}
}
