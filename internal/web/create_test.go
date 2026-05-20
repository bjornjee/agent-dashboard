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
	return createTestServerWithCfg(t, nil, agents...)
}

// createTestServerWithCfg lets a test mutate cfg after the defaults are loaded
// before the server is built.
func createTestServerWithCfg(t *testing.T, mutate func(*domain.Config), agents ...domain.Agent) (*httptest.Server, string) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Profile.Command = "claude"
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	if mutate != nil {
		mutate(&cfg)
	}

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

// TestCreate_DefaultHarnessIsClaude confirms the spawn command falls through
// the unchanged claude path when no request harness override is provided.
func TestCreate_DefaultHarnessIsClaude(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}
	ts, _ := createTestServer(t, existingAgent)

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	var capturedCmd string
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.MatchedBy(func(s string) bool { capturedCmd = s; return true }),
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","skill":"feature"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if !strings.HasPrefix(capturedCmd, "CLAUDE_CODE_EFFORT_LEVEL=") {
		t.Errorf("expected claude effort prefix, got %q", capturedCmd)
	}
	if !strings.Contains(capturedCmd, " claude --effort ") {
		t.Errorf("expected claude binary invocation, got %q", capturedCmd)
	}
}

func TestCreate_HarnessOverridePiReturns400(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil)

	folder := t.TempDir()
	ts, _ := createTestServer(t)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"pi","message":"hello"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestCreate_HarnessOverrideCodex proves the per-request Harness="codex"
// override flows settings.Harness.Codex.* into the spawn-command builder.
// This locks the codex flag surface so drift in CodexHarnessSettings or web
// wiring shows up here.
func TestCreate_HarnessOverrideCodex(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}

	mutate := func(c *domain.Config) {
		c.Settings.Harness.Codex.Model = "gpt-5.5"
		c.Settings.Harness.Codex.Approval = "on-request"
		c.Settings.Harness.Codex.Sandbox = "workspace-write"
		c.Settings.Harness.Codex.DefaultReasoningEffort = "high"
	}
	ts, _ := createTestServerWithCfg(t, mutate, existingAgent)

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	var capturedCmd string
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.MatchedBy(func(s string) bool { capturedCmd = s; return true }),
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"codex","message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	want := "codex --model 'gpt-5.5' -a 'on-request' -s 'workspace-write' 'hi'"
	if capturedCmd != want {
		t.Errorf("captured cmd = %q, want %q", capturedCmd, want)
	}
}

func TestCreate_CodexAllowsSupportedSkill(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}
	ts, _ := createTestServer(t, existingAgent)

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	var capturedCmd string
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.MatchedBy(func(s string) bool { capturedCmd = s; return true }),
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"codex","skill":"feature","message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedCmd != "codex '/feature hi'" {
		t.Errorf("captured cmd = %q, want %q", capturedCmd, "codex '/feature hi'")
	}
}

func TestCreate_CodexAllowsCustomSkill(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}
	ts, _ := createTestServer(t, existingAgent)

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)

	var capturedCmd string
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.MatchedBy(func(s string) bool { capturedCmd = s; return true }),
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"codex","skill":"custom-maintained","message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedCmd != "codex '/custom-maintained hi'" {
		t.Errorf("captured cmd = %q, want %q", capturedCmd, "codex '/custom-maintained hi'")
	}
}

func TestCreate_CodexRejectsBlockedSkill(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil)

	folder := t.TempDir()
	ts, _ := createTestServer(t)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"codex","skill":"implement","message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// Empty skill is allowed for codex — free-prompt spawn doesn't touch
// EnterPlanMode/AskUserQuestion. Regression guard against over-eager
// blocking.
func TestCreate_CodexAllowsEmptySkill(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	folder := t.TempDir()
	existingAgent := domain.Agent{SessionID: "x", Session: "main", Window: 0, State: "running", Cwd: folder}
	ts, _ := createTestServer(t, existingAgent)

	m.On("Output", mock.Anything,
		"list-panes", "-t", "main:0", "-F", "#{pane_index}",
	).Return([]byte("0\n"), nil)
	m.On("Output", mock.Anything,
		"split-window", "-t", "main:0", "-c", folder,
		"-d", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}",
		mock.Anything,
	).Return([]byte("main:0.1\n"), nil)
	m.On("Run", mock.Anything, "select-layout", "-t", "main:0", "tiled").Return(nil)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"codex","message":"just chat"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (empty skill is OK for codex), got %d", resp.StatusCode)
	}
}

// TestCreate_HarnessUnknownIs400 proves the registry error surfaces as a
// 400 rather than silently coercing to claude (regression guard for the
// silent-fallback anti-pattern called out by go-reviewer-strict).
func TestCreate_HarnessUnknownIs400(t *testing.T) {
	m := withMockTmuxRunner(t)
	// Only TmuxIsAvailable fires before the 400 — skip readAgentState mocks.
	m.On("Run", mock.Anything, "list-sessions").Return(nil)

	folder := t.TempDir()
	ts, _ := createTestServer(t)

	resp := postCreate(t, ts, `{"folder":"`+folder+`","harness":"not-a-real-harness"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body["error"], "unknown harness") {
		t.Errorf("expected unknown-harness error, got %q", body["error"])
	}
}
