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

func TestDailyUsageDaysParam(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, days := range []string{"7", "30", "90", "0"} {
		resp, err := http.Get(ts.URL + "/api/usage/daily?days=" + days)
		if err != nil {
			t.Fatalf("GET daily usage days=%s: %v", days, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("days=%s: expected 200, got %d", days, resp.StatusCode)
		}
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

	// Point to empty dirs so no real ~/.z or sessions are loaded
	cfg.Profile.SessionsDir = t.TempDir()
	cfg.Profile.HomeDir = t.TempDir()

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

	// Create real directories so DirExists passes
	dirA := filepath.Join(t.TempDir(), "project-a")
	dirB := filepath.Join(t.TempDir(), "project-b")
	os.MkdirAll(dirA, 0700)
	os.MkdirAll(dirB, 0700)

	// Point home dir at a temp dir with a .z file
	homeDir := t.TempDir()
	cfg.Profile.HomeDir = homeDir
	os.WriteFile(filepath.Join(homeDir, ".z"), []byte(
		fmt.Sprintf("%s|100|1700000000\n%s|50|1700000000\n", dirA, dirB),
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
	if err := json.NewDecoder(resp.Body).Decode(&suggestions); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d: %v", len(suggestions), suggestions)
	}
	// Higher rank should come first (both in same time bucket)
	if suggestions[0] != dirA {
		t.Errorf("expected %s first, got %s", dirA, suggestions[0])
	}
}

func TestSuggestionsFiltersSensitivePaths(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	cfg.Profile.SessionsDir = t.TempDir()

	// Create directories including sensitive ones
	base := t.TempDir()
	safeDir := filepath.Join(base, "my-project")
	sshDir := filepath.Join(base, ".ssh")
	awsDir := filepath.Join(base, ".aws")
	os.MkdirAll(safeDir, 0700)
	os.MkdirAll(sshDir, 0700)
	os.MkdirAll(awsDir, 0700)

	homeDir := t.TempDir()
	cfg.Profile.HomeDir = homeDir
	os.WriteFile(filepath.Join(homeDir, ".z"), []byte(
		fmt.Sprintf("%s|100|1700000000\n%s|90|1700000000\n%s|80|1700000000\n", safeDir, sshDir, awsDir),
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
	if err := json.NewDecoder(resp.Body).Decode(&suggestions); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion (sensitive filtered), got %d: %v", len(suggestions), suggestions)
	}
	if suggestions[0] != safeDir {
		t.Errorf("expected %s, got %s", safeDir, suggestions[0])
	}
}

func TestSkillsEndpointEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	cfg.Profile.PluginCacheDir = "/nonexistent/plugin/cache"

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/skills")
	if err != nil {
		t.Fatalf("GET /api/skills: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var skills []string
	if err := json.NewDecoder(resp.Body).Decode(&skills); err != nil {
		t.Fatalf("decode skills: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d: %v", len(skills), skills)
	}
}

func TestSkillsEndpointWithSkills(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	// Create fake plugin cache structure:
	// <cacheDir>/agent-dashboard/agent-dashboard/0.1.0/skills/{bugfix,feature}/
	cacheDir := t.TempDir()
	cfg.Profile.PluginCacheDir = cacheDir
	skillsBase := filepath.Join(cacheDir, "agent-dashboard", "agent-dashboard", "0.1.0", "skills")
	os.MkdirAll(filepath.Join(skillsBase, "bugfix"), 0700)
	os.MkdirAll(filepath.Join(skillsBase, "feature"), 0700)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/skills")
	if err != nil {
		t.Fatalf("GET /api/skills: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var skills []string
	if err := json.NewDecoder(resp.Body).Decode(&skills); err != nil {
		t.Fatalf("decode skills: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %v", len(skills), skills)
	}
	if skills[0] != "bugfix" || skills[1] != "feature" {
		t.Errorf("expected [bugfix feature], got %v", skills)
	}
}

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/Users/me/.ssh", true},
		{"/Users/me/.ssh/keys", true},
		{"/Users/me/.aws", true},
		{"/Users/me/.aws/config", true},
		{"/Users/me/.gnupg", true},
		{"/Users/me/.docker", true},
		{"/Users/me/.kube", true},
		{"/Users/me/.gitconfig", true},
		{"/Users/me/.vimrc", true},
		{"/Users/me/.vim", true},
		{"/Users/me/.vim/plugged", true},
		{"/Users/me/.zshrc", true},
		{"/Users/me/.netrc", true},
		{"/Users/me/.npmrc", true},
		{"/Users/me/.gemini", true},
		{"/Users/me/.Trash", true},
		{"/Users/me/.Trash/old-stuff", true},
		{"/Users/me/code/project", false},
		{"/Users/me/.dotfiles", false},
		{"/Users/me/.claude/plugins", false},
		{"/Users/me/.config/ghostty", false},
		{"/Users/me/.config/gcloud", true},
		{"/Users/me/.config/gcloud/configs", true},
	}
	for _, tt := range tests {
		if got := isSensitivePath(tt.path); got != tt.want {
			t.Errorf("isSensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPRURLEndpoint(t *testing.T) {
	t.Run("returns stored pr_url when set", func(t *testing.T) {
		cfg := config.DefaultConfig()
		stateDir := t.TempDir()
		cfg.Profile.StateDir = stateDir

		agentsDir := filepath.Join(stateDir, "agents")
		os.MkdirAll(agentsDir, 0700)

		agent := domain.Agent{
			SessionID: "pr-1",
			State:     "pr",
			Cwd:       "/tmp/repo",
			Branch:    "feat/test",
			PRURL:     "https://github.com/owner/repo/pull/42",
		}
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, "pr-1.json"), data, 0600)

		srv := NewServer(cfg, nil, ServerOptions{})
		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/agents/pr-1/pr-url")
		if err != nil {
			t.Fatalf("GET pr-url: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["url"] != "https://github.com/owner/repo/pull/42/files" {
			t.Errorf("expected stored PR url with /files, got %q", result["url"])
		}
	})

	t.Run("resolves via gh pr view when pr_url empty", func(t *testing.T) {
		m := withMockCommandRunner(t)

		cfg := config.DefaultConfig()
		stateDir := t.TempDir()
		cfg.Profile.StateDir = stateDir

		agentsDir := filepath.Join(stateDir, "agents")
		os.MkdirAll(agentsDir, 0700)

		agentDir := t.TempDir()
		agent := domain.Agent{
			SessionID: "pr-2",
			State:     "pr",
			Cwd:       agentDir,
			Branch:    "feat/test",
		}
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, "pr-2.json"), data, 0600)

		// Mock gh pr view returning an existing PR URL
		m.On("CombinedOutput", mock.Anything, agentDir, "gh", "pr", "view", "feat/test",
			"--json", "url", "-q", ".url").
			Return([]byte("https://github.com/owner/repo/pull/99\n"), nil)

		srv := NewServer(cfg, nil, ServerOptions{})
		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/agents/pr-2/pr-url")
		if err != nil {
			t.Fatalf("GET pr-url: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["url"] != "https://github.com/owner/repo/pull/99/files" {
			t.Errorf("expected gh-resolved PR url, got %q", result["url"])
		}
	})

	t.Run("detects branch from git when not stored", func(t *testing.T) {
		m := withMockCommandRunner(t)

		cfg := config.DefaultConfig()
		stateDir := t.TempDir()
		cfg.Profile.StateDir = stateDir

		agentsDir := filepath.Join(stateDir, "agents")
		os.MkdirAll(agentsDir, 0700)

		agentDir := t.TempDir()
		agent := domain.Agent{
			SessionID: "pr-4",
			State:     "pr",
			Cwd:       agentDir,
			// Branch intentionally empty
		}
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, "pr-4.json"), data, 0600)

		// Mock git branch --show-current
		m.On("CombinedOutput", mock.Anything, agentDir, "git", "branch", "--show-current").
			Return([]byte("feat/detected\n"), nil)

		// Mock gh pr view returning an existing PR
		m.On("CombinedOutput", mock.Anything, agentDir, "gh", "pr", "view", "feat/detected",
			"--json", "url", "-q", ".url").
			Return([]byte("https://github.com/owner/repo/pull/55\n"), nil)

		srv := NewServer(cfg, nil, ServerOptions{})
		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/agents/pr-4/pr-url")
		if err != nil {
			t.Fatalf("GET pr-url: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["url"] != "https://github.com/owner/repo/pull/55/files" {
			t.Errorf("expected detected branch PR url, got %q", result["url"])
		}
	})

	t.Run("falls back to compare URL when gh fails", func(t *testing.T) {
		m := withMockCommandRunner(t)

		cfg := config.DefaultConfig()
		stateDir := t.TempDir()
		cfg.Profile.StateDir = stateDir

		agentsDir := filepath.Join(stateDir, "agents")
		os.MkdirAll(agentsDir, 0700)

		agentDir := t.TempDir()
		agent := domain.Agent{
			SessionID: "pr-3",
			State:     "pr",
			Cwd:       agentDir,
			Branch:    "feat/test",
		}
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, "pr-3.json"), data, 0600)

		// gh pr view fails (no existing PR)
		m.On("CombinedOutput", mock.Anything, agentDir, "gh", "pr", "view", "feat/test",
			"--json", "url", "-q", ".url").
			Return(nil, fmt.Errorf("no PRs found"))

		// git remote get-url origin returns the remote URL
		m.On("CombinedOutput", mock.Anything, agentDir, "git", "remote", "get-url", "origin").
			Return([]byte("https://github.com/myowner/myrepo.git\n"), nil)

		// git symbolic-ref for default branch
		m.On("CombinedOutput", mock.Anything, agentDir, "git", "symbolic-ref", "refs/remotes/origin/HEAD").
			Return([]byte("refs/remotes/origin/main\n"), nil)

		srv := NewServer(cfg, nil, ServerOptions{})
		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/agents/pr-3/pr-url")
		if err != nil {
			t.Fatalf("GET pr-url: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		expected := "https://github.com/myowner/myrepo/compare/main...feat%2Ftest?expand=1"
		if result["url"] != expected {
			t.Errorf("expected compare URL %q, got %q", expected, result["url"])
		}
	})
}

func TestCleanupPerformsFullPostMergeFlow(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID:   "cleanup-1",
		State:       "merged",
		Cwd:         "/tmp/repo",
		WorktreeCwd: "/tmp/worktree",
		Branch:      "feat/test-cleanup",
	}
	data, _ := json.Marshal(agent)
	stateFile := filepath.Join(agentsDir, "cleanup-1.json")
	os.WriteFile(stateFile, data, 0600)

	m := withMockCommandRunner(t)

	// Mock: resolve default branch
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "symbolic-ref", "refs/remotes/origin/HEAD").
		Return([]byte("refs/remotes/origin/main\n"), nil)
	// Mock: remove worktree
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "worktree", "remove", "--force", "/tmp/worktree").
		Return([]byte(""), nil)
	// Mock: prune worktree
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "worktree", "prune").
		Return([]byte(""), nil)
	// Mock: checkout main
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "checkout", "main").
		Return([]byte(""), nil)
	// Mock: pull
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "pull", "origin", "main").
		Return([]byte(""), nil)
	// Mock: delete branch
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "branch", "-d", "feat/test-cleanup").
		Return([]byte(""), nil)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/cleanup-1/cleanup", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cleanup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["ok"] != "cleaned up" {
		t.Errorf("expected ok='cleaned up', got %v", result)
	}

	// State file should be removed
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("expected state file to be removed after cleanup")
	}

	m.AssertExpectations(t)
}

func TestCleanupRejectsInvalidCwd(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	// Agent with no cwd
	agent := domain.Agent{SessionID: "cleanup-bad", State: "merged", Cwd: ""}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "cleanup-bad.json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/cleanup-bad/cleanup", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cleanup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cwd, got %d", resp.StatusCode)
	}
}

func TestCleanupWorktreeRemoveFailureReturns500(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agent := domain.Agent{
		SessionID:   "cleanup-fail",
		State:       "merged",
		Cwd:         "/tmp/repo",
		WorktreeCwd: "/tmp/worktree",
		Branch:      "feat/fail",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "cleanup-fail.json"), data, 0600)

	m := withMockCommandRunner(t)

	// Mock: resolve default branch
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "symbolic-ref", "refs/remotes/origin/HEAD").
		Return([]byte("refs/remotes/origin/main\n"), nil)
	// Mock: worktree remove fails
	m.On("CombinedOutput", mock.Anything, "/tmp/repo", "git", "-C", "/tmp/repo", "worktree", "remove", "--force", "/tmp/worktree").
		Return([]byte("error: failed"), fmt.Errorf("exit status 1"))

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/cleanup-fail/cleanup", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cleanup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 when worktree remove fails, got %d", resp.StatusCode)
	}
}

func TestCleanupNotFoundReturns404(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/nonexistent/cleanup", nil)
	req.Header.Set("X-Requested-With", "dashboard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cleanup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
