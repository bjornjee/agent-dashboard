package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func TestServerStartsAndServesRoutes(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Health check
	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var agents []domain.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestAuthMiddlewareDisabledOnLocalhost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	// No auth options = localhost mode
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 without auth, got %d", resp.StatusCode)
	}
}

func TestAuthMiddlewareBlocksUnauthenticated(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      "test-session-secret-32-bytes-ok!",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without session, got %d", resp.StatusCode)
	}
}

func TestAuthLoginPageAccessible(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      "test-session-secret-32-bytes-ok!",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Login page should be accessible without auth
	resp, err := http.Get(ts.URL + "/auth/login")
	if err != nil {
		t.Fatalf("GET /auth/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for login page, got %d", resp.StatusCode)
	}
}

func TestAuthGoogleRedirect(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      "test-session-secret-32-bytes-ok!",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/auth/google")
	if err != nil {
		t.Fatalf("GET /auth/google: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected 307 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("expected Location header for OAuth redirect")
	}
}

func TestSessionCookieValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	secret := "test-session-secret-32-bytes-ok!"
	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      secret,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create a valid session token
	token, err := srv.createSessionToken("allowed@gmail.com")
	if err != nil {
		t.Fatalf("createSessionToken: %v", err)
	}

	// Request with valid session cookie
	req, _ := http.NewRequest("GET", ts.URL+"/api/agents", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/agents with cookie: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with valid session, got %d", resp.StatusCode)
	}
}

func TestSessionRejectsWrongEmail(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	secret := "test-session-secret-32-bytes-ok!"
	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      secret,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create token for wrong email
	token, err := srv.createSessionToken("wrong@gmail.com")
	if err != nil {
		t.Fatalf("createSessionToken: %v", err)
	}

	req, _ := http.NewRequest("GET", ts.URL+"/api/agents", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with wrong email cookie: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong email, got %d", resp.StatusCode)
	}
}

func TestGetAgentsWithStateFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	// Create agent state file
	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID:          "test-session-1",
		State:              "running",
		Cwd:                "/tmp/myrepo",
		Branch:             "feat/test",
		Model:              "claude-opus-4-6",
		LastMessagePreview: "Working on it...",
		FilesChanged:       []string{"+main.go"},
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "test-session-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET /api/agents: %v", err)
	}
	defer resp.Body.Close()

	var agents []domain.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SessionID != "test-session-1" {
		t.Errorf("expected session test-session-1, got %s", agents[0].SessionID)
	}
	if agents[0].State != "running" {
		t.Errorf("expected state running, got %s", agents[0].State)
	}
}

func TestGetAgentDetail(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID: "detail-1",
		State:     "permission",
		Cwd:       "/tmp/repo",
		Branch:    "main",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "detail-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Found
	resp, err := http.Get(ts.URL + "/api/agents/detail-1")
	if err != nil {
		t.Fatalf("GET /api/agents/detail-1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Not found
	resp2, err := http.Get(ts.URL + "/api/agents/nonexistent")
	if err != nil {
		t.Fatalf("GET nonexistent: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp2.StatusCode)
	}
}

func TestConversationEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID: "conv-1",
		State:     "running",
		Cwd:       "/tmp/myproject",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "conv-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Should return empty array (no JSONL file)
	resp, err := http.Get(ts.URL + "/api/agents/conv-1/conversation")
	if err != nil {
		t.Fatalf("GET conversation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// withMockCommandRunner swaps the package-level cmdRunner with a mock
// and restores the original on test cleanup.
func withMockCommandRunner(t *testing.T) *mocks.MockCommandRunner {
	t.Helper()
	m := mocks.NewMockCommandRunner(t)
	orig := cmdRunner
	cmdRunner = m
	t.Cleanup(func() { cmdRunner = orig })
	return m
}

func TestDiffEndpoint(t *testing.T) {
	m := withMockCommandRunner(t)

	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agentDir := t.TempDir()
	agent := domain.Agent{
		SessionID: "diff-1",
		State:     "running",
		Cwd:       agentDir,
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "diff-1.json"), data, 0600)

	// Mock findMergeBase: all merge-base attempts fail -> returns "HEAD"
	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		m.On("Output", mock.Anything, "git", "-C", agentDir, "merge-base", "HEAD", base).
			Return(nil, fmt.Errorf("not a git repo"))
	}
	// Mock git diff HEAD
	m.On("Output", mock.Anything, "git", "-C", agentDir, "diff", "HEAD", "--no-color").
		Return([]byte{}, nil)
	// Mock untracked files
	m.On("Output", mock.Anything, "git", "-C", agentDir,
		"ls-files", "--others", "--exclude-standard").
		Return([]byte{}, nil)
	// Mock ignored files
	m.On("Output", mock.Anything, "git", "-C", agentDir,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory").
		Return([]byte{}, nil)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/diff-1/diff")
	if err != nil {
		t.Fatalf("GET diff: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var dr diffResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	if dr.Raw != "" {
		t.Errorf("expected empty diff, got %q", dr.Raw)
	}
}

func TestPlanEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()
	cfg.Profile.PlansDir = t.TempDir()

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID: "plan-1",
		State:     "plan",
		Cwd:       "/tmp/planrepo",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "plan-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/plan-1/plan")
	if err != nil {
		t.Fatalf("GET plan: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["content"]; !ok {
		t.Error("expected 'content' key in plan response")
	}
}

func TestUsageEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID: "usage-1",
		State:     "done",
		Cwd:       "/tmp/usagerepo",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "usage-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/usage-1/usage")
	if err != nil {
		t.Fatalf("GET usage: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDailyUsageEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	// No database — should still return 200 with empty data
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/usage/daily")
	if err != nil {
		t.Fatalf("GET daily usage: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["days"]; !ok {
		t.Error("expected 'days' key in response")
	}
}

func TestSSEEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Test that SSE endpoint responds with correct headers
	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}

	// Read the initial SSE data frame
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	frame := string(buf[:n])
	if len(frame) < 6 || frame[:6] != "data: " {
		t.Errorf("expected SSE data frame, got %q", frame)
	}
}

func TestSubagentsEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID: "sub-1",
		State:     "running",
		Cwd:       "/tmp/subrepo",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "sub-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/sub-1/subagents")
	if err != nil {
		t.Fatalf("GET subagents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCSRFRequired(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{SessionID: "csrf-1", State: "running", Cwd: "/tmp/repo"}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "csrf-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// POST without X-Requested-With header should be rejected
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/csrf-1/approve", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST approve: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 without CSRF header, got %d", resp.StatusCode)
	}
}

func TestActionNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// POST to nonexistent agent
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/nonexistent/approve", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestInputValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{SessionID: "input-1", State: "question", Cwd: "/tmp/repo"}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "input-1.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Invalid JSON
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/input-1/input",
		strings.NewReader("not json"))
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

func TestCreateValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Missing folder
	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/create",
		strings.NewReader(`{"folder":""}`))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty folder, got %d", resp.StatusCode)
	}
}

func TestCloseRemovesStateFile(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{SessionID: "close-1", State: "done", Cwd: "/tmp/repo"}
	data, _ := json.Marshal(agent)
	stateFile := filepath.Join(agentsDir, "close-1.json")
	os.WriteFile(stateFile, data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/close-1/close", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST close: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// State file should be removed
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("expected state file to be removed after close")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-secret",
		AllowedEmail:       "allowed@gmail.com",
		SessionSecret:      "test-session-secret-32-bytes-ok!",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/auth/logout", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/logout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Check that Set-Cookie clears the session
	for _, c := range resp.Cookies() {
		if c.Name == "session" && c.MaxAge < 0 {
			return // Cookie properly cleared
		}
	}
	t.Error("expected session cookie to be cleared")
}

func TestSuggestionsEndpoint(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	// Create a temporary z-file with test entries
	sessionsDir := t.TempDir()
	cfg.Profile.SessionsDir = sessionsDir

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/suggestions")
	if err != nil {
		t.Fatalf("GET /api/suggestions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var suggestions []string
	if err := json.NewDecoder(resp.Body).Decode(&suggestions); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	// With empty sessions dir, should return empty array
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestSuggestionsWithZFile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	cfg.Profile.SessionsDir = t.TempDir()

	// Point home dir at a temp dir with a .z file
	homeDir := t.TempDir()
	cfg.Profile.HomeDir = homeDir
	os.WriteFile(filepath.Join(homeDir, ".z"), []byte(
		"/tmp/project-a|100|1700000000\n/tmp/project-b|50|1700000000\n",
	), 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/suggestions")
	if err != nil {
		t.Fatalf("GET /api/suggestions: %v", err)
	}
	defer resp.Body.Close()

	var suggestions []string
	json.NewDecoder(resp.Body).Decode(&suggestions)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d: %v", len(suggestions), suggestions)
	}
	// Higher rank should come first
	if suggestions[0] != "/tmp/project-a" {
		t.Errorf("expected /tmp/project-a first, got %s", suggestions[0])
	}
}
