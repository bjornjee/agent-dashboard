package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/state"
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

func TestPendingQuestionEndpoint_ReturnsPayload(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	projectsDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = projectsDir

	cwd := "/tmp/pq-repo-1"
	projDir := filepath.Join(projectsDir, conversation.ProjectSlug(cwd))
	os.MkdirAll(projDir, 0755)
	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	sessionID := "pq-1"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool_abc","name":"AskUserQuestion","input":{"questions":[{"question":"Which?","header":"Pick","multiSelect":false,"options":[{"label":"A","description":"first"},{"label":"B","description":"second"}]}]}}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	agent := domain.Agent{
		SessionID: sessionID,
		ProjDir:   projDir,
		State:     "question",
		Cwd:       cwd,
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET pending-question: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var pq domain.PendingQuestion
	if err := json.NewDecoder(resp.Body).Decode(&pq); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pq.ToolUseID != "tool_abc" {
		t.Errorf("ToolUseID = %q, want tool_abc", pq.ToolUseID)
	}
	if len(pq.Questions) != 1 {
		t.Fatalf("len(Questions) = %d, want 1", len(pq.Questions))
	}
	if pq.Questions[0].Header != "Pick" {
		t.Errorf("Header = %q, want Pick", pq.Questions[0].Header)
	}
	if len(pq.Questions[0].Options) != 2 {
		t.Errorf("Options len = %d, want 2", len(pq.Questions[0].Options))
	}
}

func TestPendingQuestionEndpoint_ReturnsPayloadWhenStateOverridden(t *testing.T) {
	// Regression: when an agent has a PendingQuestion populated by the
	// fast hook AND its State has been overridden by ApplyPinnedStates
	// (e.g. pinned_state='pr' → state='pr'), the endpoint MUST still
	// return the question — the user is being asked something and the
	// detail view needs to render the card. The previous gate
	// (`agent.State != "question"`) silently dropped the payload.
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	projectsDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = projectsDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	sessionID := "pq-override-1"
	agent := domain.Agent{
		SessionID:   sessionID,
		State:       "pr", // overridden — pinned_state took precedence
		PinnedState: "pr",
		Cwd:         "/tmp/pq-override-repo",
		PendingQuestion: &domain.PendingQuestion{
			ToolUseID: "tool_xyz",
			Questions: []domain.PendingQuestionPrompt{{
				Question: "Confirm or override?",
				Header:   "Register",
				Options: []domain.PendingQuestionOption{
					{Label: "Confirm refined-minimal", Description: "match PRODUCT.md"},
					{Label: "Industrial", Description: "data-dense"},
				},
			}},
		},
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET pending-question: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var pq domain.PendingQuestion
	if err := json.NewDecoder(resp.Body).Decode(&pq); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pq.ToolUseID != "tool_xyz" {
		t.Errorf("ToolUseID = %q, want tool_xyz", pq.ToolUseID)
	}
	if len(pq.Questions) != 1 || pq.Questions[0].Question != "Confirm or override?" {
		t.Errorf("Questions = %+v, want Confirm or override?", pq.Questions)
	}
}

func TestPendingQuestionEndpoint_ReadsFromSidecarBeforeJSONL(t *testing.T) {
	// Claude Code doesn't flush the AskUserQuestion tool_use line to the JSONL
	// until the user answers; while the agent is paused on the question, the
	// payload only exists in the agent sidecar (written by agent-state-fast.js's
	// PreToolUse hook). The endpoint must read from there directly. This test
	// asserts that path by using a JSONL that contains NO AskUserQuestion at
	// all — the only place the data exists is the sidecar.
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	projectsDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = projectsDir

	cwd := "/tmp/pq-sidecar-repo"
	projDir := filepath.Join(projectsDir, conversation.ProjectSlug(cwd))
	os.MkdirAll(projDir, 0755)
	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	sessionID := "pq-sidecar"
	// JSONL exists (so ProjDir resolution succeeds) but has zero AskUserQuestion
	// tool_use blocks — mirrors the real bug where the hook stamped state=question
	// before Claude Code flushed the tool_use line.
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"thinking..."}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	agent := domain.Agent{
		SessionID: sessionID,
		ProjDir:   projDir,
		State:     "question",
		Cwd:       cwd,
		PendingQuestion: &domain.PendingQuestion{
			ToolUseID: "tool_sidecar_42",
			Questions: []domain.PendingQuestionPrompt{{
				Question:    "Pick one",
				Header:      "Sidecar",
				MultiSelect: false,
				Options: []domain.PendingQuestionOption{
					{Label: "X", Description: "from sidecar"},
					{Label: "Y", Description: "also from sidecar"},
				},
			}},
		},
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var pq domain.PendingQuestion
	if err := json.NewDecoder(resp.Body).Decode(&pq); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pq.ToolUseID != "tool_sidecar_42" {
		t.Errorf("ToolUseID = %q, want tool_sidecar_42 (sidecar path must win)", pq.ToolUseID)
	}
	if len(pq.Questions) != 1 || pq.Questions[0].Header != "Sidecar" {
		t.Errorf("Questions not propagated from sidecar: %+v", pq.Questions)
	}
	if len(pq.Questions[0].Options) != 2 {
		t.Errorf("Options len = %d, want 2", len(pq.Questions[0].Options))
	}
}

func TestPendingQuestionEndpoint_EmptyWhenNone(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	projectsDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = projectsDir

	cwd := "/tmp/pq-empty-repo"
	projDir := filepath.Join(projectsDir, conversation.ProjectSlug(cwd))
	os.MkdirAll(projDir, 0755)
	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	sessionID := "pq-empty"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	agent := domain.Agent{
		SessionID: sessionID,
		ProjDir:   projDir,
		State:     "running",
		Cwd:       cwd,
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	trimmed := strings.TrimSpace(string(buf[:n]))
	if trimmed != "null" && trimmed != "{}" {
		t.Errorf("expected null or {}, got %q", trimmed)
	}
}

func TestPendingQuestionEndpoint_ScansJSONLEvenWhenStateNotQuestion(t *testing.T) {
	// Regression: the previous version gated the JSONL fallback on
	// agent.State == "question" as a cost optimization. That gate locked
	// out codex agents whose Stop hook landed before the dashboard had
	// a chance to read the unanswered request_user_input — state stays
	// "done" but the rollout still carries the question. The endpoint
	// must now scan the JSONL regardless of state when sidecar
	// PendingQuestion is nil, so the detail view's question card can
	// still render.
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	projectsDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = projectsDir

	cwd := "/tmp/pq-stop-repo"
	projDir := filepath.Join(projectsDir, conversation.ProjectSlug(cwd))
	os.MkdirAll(projDir, 0755)
	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	sessionID := "pq-stop"
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool_stop","name":"AskUserQuestion","input":{"questions":[{"question":"Which approach?","options":[{"label":"A"},{"label":"B"}]}]}}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(jsonl), 0644)

	agent := domain.Agent{
		SessionID: sessionID,
		ProjDir:   projDir,
		State:     "done", // Stop hook landed before the question was surfaced
		Cwd:       cwd,
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var pq domain.PendingQuestion
	if err := json.NewDecoder(resp.Body).Decode(&pq); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pq.ToolUseID != "tool_stop" {
		t.Errorf("ToolUseID = %q, want tool_stop", pq.ToolUseID)
	}
	if len(pq.Questions) != 1 || pq.Questions[0].Question != "Which approach?" {
		t.Errorf("Questions = %+v, want Which approach?", pq.Questions)
	}
}

// Codex agents must surface their pending request_user_input through the
// same /api/agents/{id}/pending-question endpoint as claude — same JSON
// shape so the frontend doesn't need a harness-specific branch.
func TestPendingQuestionEndpoint_Codex(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()

	sessionID := "019e9ba2-84c1-77a0-8bb4-ee9e88264f58"
	rolloutDir := filepath.Join(codexHome, "sessions", "2026", "06", "06")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-06T14-32-59-"+sessionID+".jsonl")
	contents := `{"timestamp":"2026-06-06T06:39:26.092Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"fmt_target\",\"header\":\"Fmt target\",\"question\":\"Add fmt?\",\"options\":[{\"label\":\"Yes\",\"description\":\"good\"},{\"label\":\"No\"}]}]}","call_id":"call_codex_q"}}` + "\n"
	if err := os.WriteFile(rolloutPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)
	agent := domain.Agent{
		SessionID: sessionID,
		State:     "question",
		Cwd:       "/tmp/codex-repo",
		Harness:   "codex",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/pending-question")
	if err != nil {
		t.Fatalf("GET pending-question: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var pq domain.PendingQuestion
	if err := json.NewDecoder(resp.Body).Decode(&pq); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pq.ToolUseID != "call_codex_q" {
		t.Errorf("ToolUseID = %q, want call_codex_q", pq.ToolUseID)
	}
	if len(pq.Questions) != 1 || pq.Questions[0].ID != "fmt_target" {
		t.Errorf("Questions unexpected: %+v", pq.Questions)
	}
	if pq.Questions[0].Header != "Fmt target" || pq.Questions[0].Question != "Add fmt?" {
		t.Errorf("Question/Header mismatch: %+v", pq.Questions[0])
	}
	if len(pq.Questions[0].Options) != 2 || pq.Questions[0].Options[0].Label != "Yes" {
		t.Errorf("Options unexpected: %+v", pq.Questions[0].Options)
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

// stubBranchRunner returns a fixed branch for any directory passed to
// gitBranch, simulating "this dir resolves to <branch>". Used by tests that
// pre-populate Agent.Branch and need ResolveAgentBranches to resolve to the
// same value (otherwise it clears the field, since resolution failure is now
// treated as authoritative).
type stubBranchRunner struct{ branch string }

func (r *stubBranchRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(r.branch + "\n"), nil
}

// withStubBranchRunner makes ResolveAgentBranches resolve every directory to
// the given branch for the duration of the test.
func withStubBranchRunner(t *testing.T, branch string) {
	t.Helper()
	restore := state.SetTestRunner(&stubBranchRunner{branch: branch})
	t.Cleanup(restore)
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
	if dr.Status != "empty" {
		t.Errorf("expected status=empty for clean worktree, got %q", dr.Status)
	}
}

// resolveDiffRef matrix mirror of the TUI tests, exercising the web-side copy.

func TestResolveDiffRef_HeadIsRealFeatureBranch(t *testing.T) {
	if got := resolveDiffRef("feat/auto-statement-fetch", "main", "/fake"); got != "feat/auto-statement-fetch" {
		t.Fatalf("expected feat/auto-statement-fetch, got %q", got)
	}
}

func TestResolveDiffRef_HeadAndRecordedAgree(t *testing.T) {
	if got := resolveDiffRef("feat/z", "feat/z", "/fake"); got != "feat/z" {
		t.Fatalf("expected feat/z, got %q", got)
	}
}

func TestResolveDiffRef_HeadIsLocalDefault_FallsBackToRecorded(t *testing.T) {
	m := withMockCommandRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/fake", "rev-parse", "--verify", "chore/feat^{commit}").
		Return([]byte("abc\n"), nil)

	if got := resolveDiffRef("main", "chore/feat", "/fake"); got != "chore/feat" {
		t.Fatalf("expected chore/feat, got %q", got)
	}
}

func TestResolveDiffRef_HeadIsLocalDefault_RecordedDeleted_DegradesToHead(t *testing.T) {
	m := withMockCommandRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/fake", "rev-parse", "--verify", "feat/y^{commit}").
		Return(nil, fmt.Errorf("unknown revision"))

	if got := resolveDiffRef("main", "feat/y", "/fake"); got != "main" {
		t.Fatalf("expected main, got %q", got)
	}
}

func TestResolveDiffRef_HeadIsMaster_FallsBackToRecorded(t *testing.T) {
	m := withMockCommandRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/fake", "rev-parse", "--verify", "feat/x^{commit}").
		Return([]byte("abc\n"), nil)

	if got := resolveDiffRef("master", "feat/x", "/fake"); got != "feat/x" {
		t.Fatalf("expected feat/x, got %q", got)
	}
}

func TestResolveDiffRef_DetachedHead_FallsBackToRecorded(t *testing.T) {
	m := withMockCommandRunner(t)
	m.On("Output", mock.Anything, "git", "-C", "/fake", "rev-parse", "--verify", "feat/x^{commit}").
		Return([]byte("abc\n"), nil)

	if got := resolveDiffRef("HEAD", "feat/x", "/fake"); got != "feat/x" {
		t.Fatalf("expected feat/x, got %q", got)
	}
}

func TestResolveDiffRef_NoSignals(t *testing.T) {
	if got := resolveDiffRef("", "", "/fake"); got != "HEAD" {
		t.Fatalf("expected HEAD, got %q", got)
	}
}

func TestDiffEndpoint_StaleDefaultRecordedBranch_UsesAgentBranch(t *testing.T) {
	// Pane 4.1 live bug repro: agent.Branch="feat/x", JSONL recorded "main".
	// handleDiff must diff base..feat/x, not main..main.
	m := withMockCommandRunner(t)
	// state.ResolveAgentBranches recomputes Branch on every state load via
	// branchRunner; pin it to the agent's actual checked-out branch so the
	// JSON-loaded Branch survives lookupAgent.
	withStubBranchRunner(t, "feat/auto-statement-fetch")

	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()
	cfg.Profile.SessionsDir = t.TempDir()

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agentDir := t.TempDir()
	agent := domain.Agent{
		SessionID: "stale-1",
		State:     "running",
		Cwd:       agentDir,
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "stale-1.json"), data, 0600)

	// JSONL with stale recorded branch. PickProjDir uses agent.Cwd's slug.
	projSlug := conversation.ProjectSlug(agentDir)
	projDir := filepath.Join(cfg.Profile.ProjectsDir, projSlug)
	os.MkdirAll(projDir, 0700)
	os.WriteFile(filepath.Join(projDir, "stale-1.jsonl"),
		[]byte(`{"type":"user","gitBranch":"main"}`+"\n"), 0600)

	// resolveDiffRef path: head="feat/auto-statement-fetch" is non-default,
	// so we go straight to the head — no branchExists call.
	// findMergeBase(feat/auto-statement-fetch, origin/main) returns base.
	m.On("Output", mock.Anything, "git", "-C", agentDir, "merge-base",
		"feat/auto-statement-fetch", "origin/main").
		Return([]byte("base123\n"), nil)

	// git diff base..feat/auto-statement-fetch — non-empty, the agent's commits.
	branchDiff := "diff --git a/r.md b/r.md\n" +
		"--- a/r.md\n+++ b/r.md\n@@ -1 +1 @@\n-old\n+new\n"
	m.On("Output", mock.Anything, "git", "-C", agentDir, "diff",
		"base123..feat/auto-statement-fetch", "--no-color").
		Return([]byte(branchDiff), nil)

	m.On("Output", mock.Anything, "git", "-C", agentDir,
		"ls-files", "--others", "--exclude-standard").
		Return([]byte(""), nil)
	m.On("Output", mock.Anything, "git", "-C", agentDir,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory").
		Return([]byte(""), nil)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/stale-1/diff")
	if err != nil {
		t.Fatalf("GET diff: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var dr diffResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	if dr.Raw == "" {
		t.Fatalf("expected non-empty diff (the agent's commits), got empty — stale-default bug not fixed")
	}
	if !strings.Contains(dr.Raw, "r.md") {
		t.Fatalf("expected diff to mention r.md, got %q", dr.Raw)
	}
	if dr.Status != "ok" {
		t.Errorf("expected status=ok, got %q", dr.Status)
	}
}

func TestDiffEndpoint_StatusError_OnGitDiffFailure(t *testing.T) {
	m := withMockCommandRunner(t)

	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	cfg.Profile.StateDir = stateDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)

	agentDir := t.TempDir()
	agent := domain.Agent{SessionID: "err-1", State: "running", Cwd: agentDir}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, "err-1.json"), data, 0600)

	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		m.On("Output", mock.Anything, "git", "-C", agentDir, "merge-base", "HEAD", base).
			Return(nil, fmt.Errorf("not a git repo"))
	}
	m.On("Output", mock.Anything, "git", "-C", agentDir, "diff", "HEAD", "--no-color").
		Return(nil, fmt.Errorf("git diff failed"))

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/err-1/diff")
	if err != nil {
		t.Fatalf("GET diff: %v", err)
	}
	defer resp.Body.Close()
	var dr diffResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	if dr.Status != "error" {
		t.Errorf("expected status=error, got %q", dr.Status)
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

// Codex agents store plan content inside the rollout JSONL as
// item_completed events with item.type="Plan". handlePlan must route by
// harness and surface the latest plan text — claude's projDir-based
// helpers don't apply to codex sessions.
func TestPlanEndpoint_Codex(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	cfg.Profile.StateDir = stateDir
	cfg.Profile.ProjectsDir = t.TempDir()
	cfg.Profile.PlansDir = t.TempDir()

	sessionID := "019e9ba2-84c1-77a0-8bb4-ee9e88264f58"
	rolloutDir := filepath.Join(codexHome, "sessions", "2026", "06", "06")
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-06T14-32-59-"+sessionID+".jsonl")
	planText := "# Flatten checklist\n\nArchive non-table blocks."
	contents := `{"timestamp":"2026-06-06T06:39:26.092Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","text":` + jsonQuoted(planText) + `}}}` + "\n"
	if err := os.WriteFile(rolloutPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)
	agent := domain.Agent{
		SessionID: sessionID,
		State:     "plan",
		Cwd:       "/tmp/planrepo",
		Harness:   "codex",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, sessionID+".json"), data, 0600)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + sessionID + "/plan")
	if err != nil {
		t.Fatalf("GET plan: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["content"] != planText {
		t.Errorf("plan content = %q, want %q", result["content"], planText)
	}
}

func jsonQuoted(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

func TestSubagentsEndpointCodex(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	homeDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.HomeDir = homeDir
	parentID := "parent-codex"
	childID := "child-codex"

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)
	agent := domain.Agent{
		SessionID: parentID,
		State:     "running",
		Cwd:       "/tmp/subrepo",
		Harness:   "codex",
	}
	data, _ := json.Marshal(agent)
	os.WriteFile(filepath.Join(agentsDir, parentID+".json"), data, 0600)
	writeWebRollout(t, filepath.Join(homeDir, ".codex", "sessions"), childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-codex","agent_nickname":"Nietzsche","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Nietzsche","agent_role":"explorer"}}
{"timestamp":"2026-05-21T14:44:04.000Z","type":"turn_context","payload":{"approval_policy":"on-request","sandbox_policy":{"type":"workspace-write"},"collaboration_mode":{"mode":"plan"}}}
{"timestamp":"2026-05-21T14:44:05.000Z","type":"event_msg","payload":{"type":"user_message","message":"Inspect the Codex subagent display path."}}
`)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/" + parentID + "/subagents")
	if err != nil {
		t.Fatalf("GET subagents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got []domain.SubagentInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(got), got)
	}
	if got[0].AgentID != childID {
		t.Errorf("AgentID = %q, want %q", got[0].AgentID, childID)
	}
	if got[0].InstructionHead != "Inspect the Codex subagent display path." {
		t.Errorf("InstructionHead = %q, want rollout instruction", got[0].InstructionHead)
	}
	if got[0].Mode != "plan / on-request / workspace-write" {
		t.Errorf("Mode = %q, want compact Codex context", got[0].Mode)
	}
}

func TestAgentsEndpointFiltersCodexSubagentTopLevelRows(t *testing.T) {
	cfg := config.DefaultConfig()
	stateDir := t.TempDir()
	homeDir := t.TempDir()
	cfg.Profile.StateDir = stateDir
	cfg.Profile.HomeDir = homeDir

	agentsDir := filepath.Join(stateDir, "agents")
	os.MkdirAll(agentsDir, 0700)
	for _, agent := range []domain.Agent{
		{SessionID: "parent-codex", State: "running", Cwd: "/tmp/repo", Harness: "codex"},
		{SessionID: "child-codex", State: "running", Cwd: "/tmp/repo", Harness: "codex"},
	} {
		data, _ := json.Marshal(agent)
		os.WriteFile(filepath.Join(agentsDir, agent.SessionID+".json"), data, 0600)
	}
	writeWebRollout(t, filepath.Join(homeDir, ".codex", "sessions"), "child-codex", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-codex","agent_role":"explorer"}}},"thread_source":"subagent"}}
`)

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatalf("GET agents: %v", err)
	}
	defer resp.Body.Close()
	var got []domain.Agent
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d top-level agents, want only parent: %+v", len(got), got)
	}
	if got[0].SessionID != "parent-codex" {
		t.Errorf("top-level agent = %q, want parent-codex", got[0].SessionID)
	}
}

func writeWebRollout(t *testing.T, root, sessionID, contents string) string {
	t.Helper()

	dir := filepath.Join(root, "2026", "05", "21")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	path := filepath.Join(dir, "rollout-2026-05-21T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
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
	// <cacheDir>/agent-dashboard/agent-dashboard/0.1.0/skills/{bugfix,feature}/SKILL.md
	cacheDir := t.TempDir()
	cfg.Profile.PluginCacheDir = cacheDir
	skillsBase := filepath.Join(cacheDir, "agent-dashboard", "agent-dashboard", "0.1.0", "skills")
	for _, skill := range []string{"bugfix", "feature"} {
		skillDir := filepath.Join(skillsBase, skill)
		if err := os.MkdirAll(skillDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+skill+"\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

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

// /api/skills?harness=codex must scan the codex plugin cache and filter
// out skills the dashboard blocks for codex (implement, rca). Without
// the harness param, behavior is unchanged: scan the claude cache.
func TestSkillsEndpoint_HarnessCodex(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	claudeCache := t.TempDir()
	codexCache := t.TempDir()
	cfg.Profile.PluginCacheDir = claudeCache
	cfg.Profile.CodexPluginCacheDir = codexCache

	writeSkillDir(t, filepath.Join(claudeCache, "agent-dashboard", "agent-dashboard", "0.1.0", "skills"), "claude-only")
	codexSkills := filepath.Join(codexCache, "agent-dashboard", "agent-dashboard", "0.1.0", "skills")
	for _, name := range []string{"feature", "fix", "implement", "rca", "pr"} {
		writeSkillDir(t, codexSkills, name)
	}

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/skills?harness=codex")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var skills []string
	json.NewDecoder(resp.Body).Decode(&skills)
	want := []string{"feature", "fix", "pr"}
	if len(skills) != len(want) {
		t.Fatalf("got %v, want %v", skills, want)
	}
	for i := range want {
		if skills[i] != want[i] {
			t.Errorf("skills[%d]=%q, want %q", i, skills[i], want[i])
		}
	}
}

func TestSkillsEndpoint_NoHarnessParamScansClaudeCache(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()

	claudeCache := t.TempDir()
	codexCache := t.TempDir()
	cfg.Profile.PluginCacheDir = claudeCache
	cfg.Profile.CodexPluginCacheDir = codexCache

	claudeSkills := filepath.Join(claudeCache, "agent-dashboard", "agent-dashboard", "0.1.0", "skills")
	writeSkillDir(t, claudeSkills, "feature")
	writeSkillDir(t, claudeSkills, "implement")
	writeSkillDir(t, filepath.Join(codexCache, "agent-dashboard", "agent-dashboard", "0.1.0", "skills"), "codex-only")

	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/skills")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var skills []string
	json.NewDecoder(resp.Body).Decode(&skills)
	// implement stays — only codex blocks it.
	want := []string{"feature", "implement"}
	if len(skills) != len(want) {
		t.Fatalf("got %v, want %v", skills, want)
	}
}

// writeSkillDir creates skillsBase/<name>/SKILL.md so DiscoverSkills sees a
// valid skill directory (skills.go:DiscoverSkills filters on SKILL.md
// existence). Mirrors the writeSkill helper in internal/skills/skills_test.go.
func writeSkillDir(t *testing.T, skillsBase, name string) {
	t.Helper()
	dir := filepath.Join(skillsBase, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0600); err != nil {
		t.Fatal(err)
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
		withStubBranchRunner(t, "feat/test")

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
		withStubBranchRunner(t, "feat/test")

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
	withStubBranchRunner(t, "feat/test-cleanup")

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

// --- PWA installability ---

func TestPWAManifestServesValidJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/manifest.json")
	if err != nil {
		t.Fatalf("GET /manifest.json: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var m struct {
		Name            string `json:"name"`
		ShortName       string `json:"short_name"`
		ID              string `json:"id"`
		StartURL        string `json:"start_url"`
		Scope           string `json:"scope"`
		Display         string `json:"display"`
		Orientation     string `json:"orientation"`
		BackgroundColor string `json:"background_color"`
		ThemeColor      string `json:"theme_color"`
		Icons           []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
		Shortcuts []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"shortcuts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if m.Display != "standalone" {
		t.Errorf("display: got %q, want standalone", m.Display)
	}
	// Explicit orientation prevents Chrome on Android from rotating the
	// installed PWA when the OS auto-rotate lock is off. An absent
	// orientation defaults to "any" per the W3C App Manifest spec, which
	// bypasses the system lock — see PR #361 for the original (incomplete)
	// fix that only deleted "orientation": "any".
	if m.Orientation != "portrait" {
		t.Errorf("orientation: got %q, want portrait (respects OS auto-rotate)", m.Orientation)
	}
	if m.ThemeColor != "#000000" {
		t.Errorf("theme_color: got %q, want #000000 (must match index.html meta)", m.ThemeColor)
	}
	if m.BackgroundColor != "#000000" {
		t.Errorf("background_color: got %q, want #000000", m.BackgroundColor)
	}
	if m.StartURL != "/" {
		t.Errorf("start_url: got %q, want /", m.StartURL)
	}
	if m.ID != "/" {
		t.Errorf("id: got %q, want /", m.ID)
	}
	if m.Scope != "/" {
		t.Errorf("scope: got %q, want /", m.Scope)
	}

	var has192, has512, hasMaskable bool
	for _, ic := range m.Icons {
		if ic.Type != "image/png" {
			t.Errorf("icon %q: type %q, want image/png", ic.Src, ic.Type)
		}
		if ic.Sizes == "192x192" {
			has192 = true
		}
		if ic.Sizes == "512x512" && (ic.Purpose == "" || ic.Purpose == "any") {
			has512 = true
		}
		if ic.Purpose == "maskable" {
			hasMaskable = true
		}
	}
	if !has192 {
		t.Error("manifest missing 192x192 PNG icon")
	}
	if !has512 {
		t.Error("manifest missing 512x512 PNG icon with purpose any")
	}
	if !hasMaskable {
		t.Error("manifest missing maskable icon (required for Android adaptive launchers)")
	}

	var hasNewAgentShortcut bool
	for _, s := range m.Shortcuts {
		if s.Name == "New Agent" && strings.Contains(s.URL, "action=new-agent") {
			hasNewAgentShortcut = true
		}
	}
	if !hasNewAgentShortcut {
		t.Error("manifest missing 'New Agent' shortcut with ?action=new-agent URL")
	}
}

func TestPWAIconsServeAsPNG(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for _, path := range []string{
		"/icons/icon-192.png",
		"/icons/icon-512.png",
		"/icons/icon-512-maskable.png",
		"/icons/apple-touch-icon.png",
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Errorf("%s: status %d, want 200", path, resp.StatusCode)
			continue
		}
		head := make([]byte, len(pngMagic))
		_, err = io.ReadFull(resp.Body, head)
		resp.Body.Close()
		if err != nil {
			t.Errorf("%s: read magic bytes: %v", path, err)
			continue
		}
		if !bytes.Equal(head, pngMagic) {
			t.Errorf("%s: header %#x, want PNG magic %#x", path, head, pngMagic)
		}
	}
}

func TestPWAServiceWorkerCacheVersion(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sw.js")
	if err != nil {
		t.Fatalf("GET /sw.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read sw.js: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "agent-dashboard-v22") {
		t.Errorf("sw.js cache version: missing 'agent-dashboard-v22' (must bump when icon paths change)")
	}
	if strings.Contains(body, "icon-192.svg") {
		t.Errorf("sw.js still references icon-192.svg (should be /icons/icon-192.png)")
	}
}

func TestPWAIndexHTMLHasAppleTouchIcon(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `rel="apple-touch-icon"`) {
		t.Error(`index.html missing <link rel="apple-touch-icon">`)
	}
	if !strings.Contains(body, "/icons/apple-touch-icon.png") {
		t.Error(`index.html apple-touch-icon should point to /icons/apple-touch-icon.png`)
	}
}

func TestFaviconServesEmbeddedPNG(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Profile.StateDir = t.TempDir()
	srv := NewServer(cfg, nil, ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	indexResp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	indexRaw, err := io.ReadAll(indexResp.Body)
	indexResp.Body.Close()
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	indexBody := string(indexRaw)
	if strings.Contains(indexBody, "fill='%233b82f6'") || strings.Contains(indexBody, "EA%3C/text%3E") {
		t.Error("index.html still references the old blue 'A' placeholder favicon")
	}
	if !strings.Contains(indexBody, `href="/favicon.svg"`) {
		t.Error(`index.html should reference /favicon.svg`)
	}

	svgResp, err := http.Get(ts.URL + "/favicon.svg")
	if err != nil {
		t.Fatalf("GET /favicon.svg: %v", err)
	}
	defer svgResp.Body.Close()
	if svgResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /favicon.svg: status %d, want 200", svgResp.StatusCode)
	}
	svgRaw, err := io.ReadAll(svgResp.Body)
	if err != nil {
		t.Fatalf("read favicon.svg: %v", err)
	}
	svgBody := string(svgRaw)
	if !strings.Contains(svgBody, "<svg ") {
		t.Error("favicon.svg should be an SVG document")
	}
	if !strings.Contains(svgBody, "data:image/png;base64,") {
		t.Error("favicon.svg should embed a base64 PNG via <image>")
	}
}
