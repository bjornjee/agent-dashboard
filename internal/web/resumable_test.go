package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// GET /api/agents must flag restart-survivor orphans (dead pane, active
// session) with resumable:true so the frontend Cmd+K palette can mark and
// resume them. Finished agents (done/pr/merged) on dead panes stay false.
func TestHandleAgents_ResumableFlag(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m) // empty list-panes ⇒ no live panes, every pane is dead

	ts, _ := createTestServer(t,
		domain.Agent{SessionID: "running-orphan", State: "running", TmuxPaneID: "%2", Cwd: "/tmp/a"},
		domain.Agent{SessionID: "done-orphan", State: "done", TmuxPaneID: "%3", Cwd: "/tmp/b"},
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
