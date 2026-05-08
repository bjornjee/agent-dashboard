package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/stretchr/testify/mock"
)

// withMockTmuxRunner swaps the tmux package-level runner with a mock
// and restores the original on test cleanup.
func withMockTmuxRunner(t *testing.T) *mocks.MockRunner {
	t.Helper()
	m := mocks.NewMockRunner(t)
	restore := tmux.SetTestRunner(m)
	t.Cleanup(restore)
	return m
}

// mockReadAgentState sets up the tmux mock expectations needed by
// readAgentState: TmuxIsAvailable, TmuxListPaneTargets, TmuxListPaneCwds.
func mockReadAgentState(m *mocks.MockRunner) {
	// TmuxIsAvailable (Run: list-sessions)
	m.On("Run", mock.Anything, "list-sessions").Return(nil)

	// TmuxListPaneTargets (Output: list-panes -a -F ...)
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}",
	).Return([]byte(""), nil)

	// TmuxListPaneCwds (Output: list-panes -a -F ...)
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{pane_current_path}",
	).Return([]byte(""), nil)
}

// createTestServer sets up a test server with agent state files.
func createTestServer(t *testing.T, agents ...domain.Agent) (*httptest.Server, string) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Profile.Command = "claude"
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	for _, agent := range agents {
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, agent.SessionID+".json"), data, 0600)
	}

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, stateDir
}

func postCreate(t *testing.T, ts *httptest.Server, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/create",
		strings.NewReader(body))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/agents/create: %v", err)
	}
	return resp
}

// Skills that declare `effort: <level>` in their SKILL.md frontmatter are not
// consistently honored by Claude Code once permission_mode flips on
// EnterPlanMode. The dashboard pins effort at spawn via two channels:
//   - --effort <level> CLI flag (CC reads it directly)
//   - CLAUDE_CODE_EFFORT_LEVEL=<level> env-var prefix (so the SessionStart
//     hook can persist the level to the agent state file for display)
//
// Baseline for feature/fix/refactor is "high" — the dynamic dispatcher in
// agent-state-fast.js bumps to "max" while permission_mode='plan'.
func TestBuildAgentCommand_EffortHighSkillsGetFlagAndEnv(t *testing.T) {
	for _, skill := range []string{"feature", "fix", "refactor"} {
		t.Run(skill, func(t *testing.T) {
			got := buildAgentCommand("claude", skill, "")
			want := "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/" + skill + "'"
			if got != want {
				t.Errorf("buildAgentCommand(claude, %q, \"\") = %q, want %q", skill, got, want)
			}
		})
	}
}

func TestBuildAgentCommand_NonOptedSkillsOmitFlagAndEnv(t *testing.T) {
	for _, skill := range []string{"chore", "investigate", "rca", "pr"} {
		t.Run(skill, func(t *testing.T) {
			got := buildAgentCommand("claude", skill, "")
			want := "claude '/" + skill + "'"
			if got != want {
				t.Errorf("buildAgentCommand(claude, %q, \"\") = %q, want %q", skill, got, want)
			}
		})
	}
}

func TestBuildAgentCommand_EmptySkillNoFlag(t *testing.T) {
	got := buildAgentCommand("claude", "", "")
	want := "claude"
	if got != want {
		t.Errorf("buildAgentCommand(claude, \"\", \"\") = %q, want %q", got, want)
	}
}

func TestBuildAgentCommand_EmptySkillWithMessage(t *testing.T) {
	got := buildAgentCommand("claude", "", "do the thing")
	want := "claude 'do the thing'"
	if got != want {
		t.Errorf("buildAgentCommand(claude, \"\", \"do the thing\") = %q, want %q", got, want)
	}
}

func TestBuildAgentCommand_FeatureWithMessage(t *testing.T) {
	got := buildAgentCommand("claude", "feature", "add login")
	want := "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/feature add login'"
	if got != want {
		t.Errorf("buildAgentCommand(claude, feature, add login) = %q, want %q", got, want)
	}
}

func TestCreateNewWindow(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	// firstTmuxSession → TmuxListSessions
	m.On("Output", mock.Anything, "list-sessions", "-F", "#{session_name}").
		Return([]byte("main\n"), nil)

	// FindWindowByName fallback: TmuxListWindows
	m.On("Output", mock.Anything,
		"list-windows", "-t", "main", "-F", "#{window_index}\t#{window_name}",
	).Return([]byte("0\tdashboard\n"), nil)

	// No matching window → TmuxNewWindow
	m.On("Output", mock.Anything,
		"new-window", "-t", "main:", "-n", mock.AnythingOfType("string"),
		"-c", mock.AnythingOfType("string"), "-d", "-P", "-F",
		"#{session_name}:#{window_index}.#{pane_index}", mock.AnythingOfType("string"),
	).Return([]byte("main:1.0\n"), nil)

	folder := t.TempDir()
	ts, _ := createTestServer(t)

	resp := postCreate(t, ts, `{"folder":"`+folder+`"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["target"] != "main:1.0" {
		t.Errorf("expected target main:1.0, got %s", result["target"])
	}
}

func TestCreateSplitsIntoExistingWindow(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()

	existingAgent := domain.Agent{
		SessionID: "existing-1",
		Session:   "main",
		Window:    2,
		State:     "running",
		Cwd:       folder,
	}
	ts, _ := createTestServer(t, existingAgent)

	// FindWindowForRepo matches → "main:2"
	// TmuxCountPanes
	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:2", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n"), nil)

	// TmuxSplitWindow
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:2", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.AnythingOfType("string"),
	).Return([]byte("main:2.2\n"), nil)

	// TmuxEvenLayout after split
	m.On("Run", mock.Anything, "select-layout", "-t", "main:2", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["target"] != "main:2.2" {
		t.Errorf("expected target main:2.2, got %s", result["target"])
	}
}

func TestCreatePaneLimitReached(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()

	existingAgent := domain.Agent{
		SessionID: "full-1",
		Session:   "main",
		Window:    3,
		State:     "running",
		Cwd:       folder,
	}
	ts, _ := createTestServer(t, existingAgent)

	// TmuxCountPanes → 8 panes (at limit)
	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:3", "-F", "#{pane_index}",
	).Return([]byte("0\n1\n2\n3\n4\n5\n6\n7\n"), nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 for pane limit, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if !strings.Contains(result["error"], "pane limit") {
		t.Errorf("expected pane limit error, got %s", result["error"])
	}
}

func TestCreateWorktreeMatchesSameRepo(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	// Existing agent in a worktree of the same repo
	existingAgent := domain.Agent{
		SessionID:   "wt-1",
		Session:     "main",
		Window:      1,
		State:       "running",
		Cwd:         "/tmp/worktrees/myrepo/branch-a",
		WorktreeCwd: "/tmp/worktrees/myrepo/branch-a",
	}

	// Need a real directory for os.Stat validation
	folder := t.TempDir()
	worktreeDir := filepath.Join(folder, "worktrees", "myrepo", "branch-b")
	os.MkdirAll(worktreeDir, 0700)

	ts, _ := createTestServer(t, existingAgent)

	// FindWindowForRepo pass 2 matches → "main:1"
	// TmuxCountPanes
	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:1", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	// TmuxSplitWindow
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:1", "-c", worktreeDir,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.AnythingOfType("string"),
	).Return([]byte("main:1.1\n"), nil)

	// TmuxEvenLayout
	m.On("Run", mock.Anything, "select-layout", "-t", "main:1", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+worktreeDir+`"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body map[string]string
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["target"] != "main:1.1" {
		t.Errorf("expected target main:1.1, got %s", result["target"])
	}
}
