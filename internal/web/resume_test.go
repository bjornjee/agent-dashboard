package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/stretchr/testify/mock"
)

func postResume(t *testing.T, ts *httptest.Server, id string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/"+id+"/resume", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	return resp
}

// Resuming an orphan re-spawns it with `--resume <sid>` in its stored
// EffectiveDir, splitting into the repo's window, and clears the stale orphan
// state file so the resumed (live) agent replaces it.
func TestResumeOrphanSplitsIntoWindow(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	orphan := domain.Agent{
		SessionID:  "orphan-1",
		Harness:    "claude",
		State:      "running",
		Cwd:        folder,
		Session:    "main",
		Window:     1,
		TmuxPaneID: "%9", // dead pane (not in live set)
		// Survivor fields: stamped by a previous tmux server, real branch.
		TmuxServerPID: "99",
		Branch:        "feat/x",
	}
	ts, stateDir := createTestServer(t, orphan)

	// TmuxListLivePaneIDs — %9 (the orphan's pane) is NOT live, so it qualifies.
	m.On("Output", mock.Anything, "list-panes", "-a", "-F", "#{pane_id}\t#{pid}").
		Return([]byte("%0\t100\n%1\t100\n"), nil)

	// FindWindowForRepo Pass-1 matches the orphan's own dir → "main:1".
	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:1", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	// TmuxSplitWindow — assert the spawn command carries the resume flag.
	resumeCmd := mock.MatchedBy(func(s string) bool {
		return strings.Contains(s, "--resume 'orphan-1'")
	})
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:1", "-c", folder,
		"-d", "-P", "-F", "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}",
		resumeCmd,
	).Return([]byte("main:1.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:1", "tiled").Return(nil)

	resp := postResume(t, ts, "orphan-1")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["target"] != "main:1.1" {
		t.Errorf("target = %q, want main:1.1", result["target"])
	}

	// Stale orphan file should be gone (resumed agent re-creates it live).
	if _, err := os.Stat(filepath.Join(stateDir, "agents", "orphan-1.json")); !os.IsNotExist(err) {
		t.Error("stale orphan state file should be removed after resume")
	}
}

// A codex orphan resumes via `codex resume <sid>` (no --resume flag), proving
// the resume path is harness-agnostic: handleResume resolves the harness from
// agent.Harness.
func TestResumeCodexOrphanSplitsIntoWindow(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	orphan := domain.Agent{
		SessionID:  "orph-cx",
		Harness:    "codex",
		State:      "running",
		Cwd:        folder,
		Session:    "main",
		Window:     1,
		TmuxPaneID: "%9", // dead pane
		// Survivor fields: stamped by a previous tmux server, real branch.
		TmuxServerPID: "99",
		Branch:        "feat/x",
	}
	ts, stateDir := createTestServer(t, orphan)

	m.On("Output", mock.Anything, "list-panes", "-a", "-F", "#{pane_id}\t#{pid}").
		Return([]byte("%0\t100\n%1\t100\n"), nil) // %9 not live → orphan

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:1", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	codexResume := mock.MatchedBy(func(s string) bool {
		return strings.Contains(s, "codex resume 'orph-cx'")
	})
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:1", "-c", folder,
		"-d", "-P", "-F", "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}",
		codexResume,
	).Return([]byte("main:1.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:1", "tiled").Return(nil)

	resp := postResume(t, ts, "orph-cx")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "agents", "orph-cx.json")); !os.IsNotExist(err) {
		t.Error("stale codex orphan state file should be removed after resume")
	}
}

func TestResumeUnknownAgent(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil) // TmuxIsAvailable
	ts, _ := createTestServer(t)

	resp := postResume(t, ts, "does-not-exist")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown agent, got %d", resp.StatusCode)
	}
}

// Resuming an agent whose pane is still alive must be rejected — it would spawn
// a duplicate session and delete the live agent's state file.
func TestResumeLiveAgentRejected(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil) // TmuxIsAvailable
	// TmuxListLivePaneIDs reports %5 alive → the agent is not an orphan.
	m.On("Output", mock.Anything, "list-panes", "-a", "-F", "#{pane_id}\t#{pid}").
		Return([]byte("%5\t100\n"), nil)

	folder := t.TempDir()
	live := domain.Agent{
		SessionID:  "live-1",
		Harness:    "claude",
		State:      "running",
		Cwd:        folder,
		TmuxPaneID: "%5", // still alive
	}
	ts, stateDir := createTestServer(t, live)

	resp := postResume(t, ts, "live-1")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 resuming a live agent, got %d", resp.StatusCode)
	}
	// The live agent's state file must survive.
	if _, err := os.Stat(filepath.Join(stateDir, "agents", "live-1.json")); err != nil {
		t.Error("live agent's state file must not be removed on a rejected resume")
	}
}

func TestResumeMissingDir(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil) // TmuxIsAvailable
	orphan := domain.Agent{
		SessionID:  "orphan-2",
		Harness:    "claude",
		State:      "running",
		Cwd:        "/nonexistent/path/xyz",
		TmuxPaneID: "%9",
	}
	ts, _ := createTestServer(t, orphan)

	resp := postResume(t, ts, "orphan-2")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 when resume dir is gone, got %d", resp.StatusCode)
	}
}
