package web

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/stretchr/testify/mock"
)

// TestHandleInputSendsByHarness asserts that the input endpoint keeps the
// harness-specific tmux delivery paths straight: Claude uses literal text
// and Codex mirrors the TUI paste-buffer submit sequence.
func TestHandleInputSendsByHarness(t *testing.T) {
	cases := []struct {
		name    string
		harness string
		state   string
		text    string
		assert  func(*testing.T, mockExpector)
	}{
		{
			name:    "claude uses literal send-keys then Enter",
			harness: "",
			state:   "idle_prompt",
			text:    "hello",
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "hello").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name:    "codex running uses paste-buffer + Tab,Enter to queue",
			harness: "codex",
			state:   "running",
			text:    "hello",
			assert: func(t *testing.T, m mockExpector) {
				expectCodexPasteCaptureSubmit(m, "main:0.0", "hello", "› hello\n\n tab to queue message", "Tab", "Enter")
			},
		},
		{
			name:    "codex running dashboard skill command mirrors TUI queue sequence",
			harness: "codex",
			state:   "running",
			text:    "$agent-dashboard:pr",
			assert: func(t *testing.T, m mockExpector) {
				expectCodexPasteCaptureSubmit(m, "main:0.0", "$agent-dashboard:pr", "› $agent-dashboard:pr\n\n tab to queue message", "Tab", "Enter")
			},
		},
		{
			name:    "codex permission dashboard skill command queues before submit",
			harness: "codex",
			state:   "permission",
			text:    "$agent-dashboard:pr",
			assert: func(t *testing.T, m mockExpector) {
				expectCodexPasteCaptureSubmit(m, "main:0.0", "$agent-dashboard:pr", "› $agent-dashboard:pr\n\n tab to queue message", "Tab", "Enter")
			},
		},
		{
			name:    "codex stale running state but idle pane submits with Enter only",
			harness: "codex",
			state:   "running",
			text:    "$agent-dashboard:pr",
			assert: func(t *testing.T, m mockExpector) {
				expectCodexPasteCaptureSubmit(m, "main:0.0", "$agent-dashboard:pr", "› $agent-dashboard:pr\n\ngpt-5.5 high fast", "Enter")
			},
		},
		{
			name:    "codex idle uses paste-buffer + Enter only",
			harness: "codex",
			state:   "idle_prompt",
			text:    "hello",
			assert: func(t *testing.T, m mockExpector) {
				expectCodexPasteCaptureSubmit(m, "main:0.0", "hello", "› hello\n\ngpt-5.5 high fast", "Enter")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := withMockTmuxRunner(t)

			// lookupAgent → TmuxIsAvailable + TmuxListPanes
			m.On("Run", mock.Anything, "list-sessions").Return(nil)
			m.On("Output", mock.Anything,
				"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_current_path}",
			).Return([]byte(""), nil)

			// handleInput → TmuxIsAvailable (re-uses list-sessions mock above)
			// + ResolveTarget(paneID) → display-message
			m.On("Output", mock.Anything,
				"display-message", "-p", "-t", "%1",
				"#{session_name}:#{window_index}.#{pane_index}",
			).Return([]byte("main:0.0\n"), nil)
			tc.assert(t, m)

			agent := domain.Agent{
				SessionID:  "send-1",
				State:      tc.state,
				Harness:    tc.harness,
				TmuxPaneID: "%1",
				Cwd:        "/tmp/repo",
			}
			ts, _ := createTestServer(t, agent)

			body := `{"text":"` + tc.text + `"}`
			req, _ := http.NewRequest("POST", ts.URL+"/api/agents/send-1/input",
				strings.NewReader(body))
			req.Header.Set("X-Requested-With", "dashboard")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestHandleInputRejectsEmptyText asserts that an empty text body is
// rejected with 400 before any tmux call is attempted. Without the guard,
// the codex branch's set-buffer with empty data fails and surfaces a
// cryptic 500 "exit status 1" — see verification of PR #293 mirror.
func TestHandleInputRejectsEmptyText(t *testing.T) {
	m := withMockTmuxRunner(t)
	m.On("Run", mock.Anything, "list-sessions").Return(nil)
	m.On("Output", mock.Anything,
		"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_current_path}",
	).Return([]byte(""), nil)

	agent := domain.Agent{
		SessionID:  "send-empty",
		State:      "idle_prompt",
		Harness:    "codex",
		TmuxPaneID: "%1",
		Cwd:        "/tmp/repo",
	}
	ts, _ := createTestServer(t, agent)

	req, _ := http.NewRequest("POST", ts.URL+"/api/agents/send-empty/input",
		strings.NewReader(`{"text":""}`))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// mockExpector is the subset of *mocks.MockRunner we use to register
// expectations — keeps the case table free of the full mock type import.
type mockExpector interface {
	On(method string, arguments ...interface{}) *mock.Call
}

func expectCodexPasteCaptureSubmit(m mockExpector, target, text, pane string, keys ...string) {
	calls := []*mock.Call{
		m.On("Run", mock.Anything, "send-keys", "-t", target, "C-u").Return(nil).Once(),
		m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", text).Return(nil).Once(),
		m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", target).Return(nil).Once(),
		m.On("Output", mock.Anything, "capture-pane", "-p", "-t", target, "-S", "-20").Return([]byte(pane), nil).Once(),
	}
	for _, key := range keys {
		calls = append(calls, m.On("Run", mock.Anything, "send-keys", "-t", target, key).Return(nil).Once())
	}
	mock.InOrder(calls...)
}

// TestHandleAnswerQuestionDrivesPicker locks the picker-driving key sequence
// for AskUserQuestion answers. Claude Code's AskUserQuestion is a numbered
// picker; number keys are global option shortcuts (auto-advance on
// single-select, toggle on multi-select), and "Other" is the digit after the
// last labeled option. The endpoint translates a structured answer payload
// into that key sequence.
func TestHandleAnswerQuestionDrivesPicker(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		assert  func(*testing.T, mockExpector)
		wantErr bool
	}{
		{
			name: "single-select three questions sends digit per question + final Enter",
			body: `{"answers":[
				{"option_indices":[0],"freeform":"","multi":false},
				{"option_indices":[1],"freeform":"","multi":false},
				{"option_indices":[2],"freeform":"","multi":false}
			],"option_counts":[3,3,3]}`,
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "1").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "2").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "3").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "multi-select toggles each option then Tab advances",
			body: `{"answers":[
				{"option_indices":[0,2],"freeform":"","multi":true},
				{"option_indices":[1],"freeform":"","multi":false}
			],"option_counts":[3,3]}`,
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "1").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "3").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Tab").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "2").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "freeform via Other digit then typed text + Enter",
			body: `{"answers":[
				{"option_indices":[],"freeform":"my custom answer","multi":false}
			],"option_counts":[3]}`,
			assert: func(t *testing.T, m mockExpector) {
				// Other = option_count + 1 = 4
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "4").Return(nil).Once()
				// TmuxSendKeys: -l literal text, then Enter
				m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "my custom answer").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
				// Final Submit Enter
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "mismatched answers vs option_counts → 400, zero tmux send",
			body: `{"answers":[
				{"option_indices":[0],"freeform":"","multi":false}
			],"option_counts":[3,3]}`,
			assert:  func(t *testing.T, m mockExpector) {},
			wantErr: true,
		},
		{
			name: "empty answer (no pick, no freeform) → 400, zero tmux send",
			body: `{"answers":[
				{"option_indices":[],"freeform":"","multi":false}
			],"option_counts":[3]}`,
			assert:  func(t *testing.T, m mockExpector) {},
			wantErr: true,
		},
		{
			name: "option_index out of range → 400, zero tmux send",
			body: `{"answers":[
				{"option_indices":[5],"freeform":"","multi":false}
			],"option_counts":[3]}`,
			assert:  func(t *testing.T, m mockExpector) {},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := withMockTmuxRunner(t)
			mockReadAgentState(m)
			// ResolveTarget is only reached after request validation;
			// the wantErr cases short-circuit on 400 before any tmux dispatch.
			if !tc.wantErr {
				m.On("Output", mock.Anything,
					"display-message", "-p", "-t", "%1",
					"#{session_name}:#{window_index}.#{pane_index}",
				).Return([]byte("main:0.0\n"), nil)
			}

			tc.assert(t, m)

			agent := domain.Agent{
				SessionID:   "aq-1",
				State:       "question",
				CurrentTool: "AskUserQuestion",
				Harness:     "",
				TmuxPaneID:  "%1",
				Cwd:         "/tmp/repo",
				// Sidecar payload — the hook layer stamps state + tool +
				// PendingQuestion together; the test fixture mirrors that
				// so PausedOnQuestion's free path resolves without I/O.
				PendingQuestion: &domain.PendingQuestion{
					ToolUseID: "tool_aq",
					Questions: []domain.PendingQuestionPrompt{{Question: "Which?"}},
				},
			}
			ts, _ := createTestServer(t, agent)

			req, _ := http.NewRequest("POST",
				ts.URL+"/api/agents/aq-1/answer-question",
				strings.NewReader(tc.body))
			req.Header.Set("X-Requested-With", "dashboard")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()

			if tc.wantErr {
				if resp.StatusCode != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d", resp.StatusCode)
				}
			} else {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("expected 200, got %d", resp.StatusCode)
				}
			}
		})
	}
}

// TestHandleAnswerQuestion_CodexDrivesPicker locks codex's request_user_input
// picker driver. Codex's TUI footer reads "tab to add notes | enter to submit"
// and has no digit shortcuts — the driver navigates via Down arrows and
// commits with Enter. Each question is committed individually; codex
// advances to the next question on Enter, so no final Submit-tab Enter is
// needed (unlike claude's picker).
//
// Codex's request_user_input has no multi-select. Multi=true is ignored
// and option_indices is treated as single-select on the first entry.
func TestHandleAnswerQuestion_CodexDrivesPicker(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		assert func(*testing.T, mockExpector)
	}{
		{
			name: "single-select option 2 sends one Down + Enter",
			body: `{"answers":[
				{"option_indices":[1],"freeform":"","multi":false}
			],"option_counts":[3]}`,
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Down").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "single-select option 1 sends Enter only (no Down)",
			body: `{"answers":[
				{"option_indices":[0],"freeform":"","multi":false}
			],"option_counts":[3]}`,
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "two questions: Q1 pick option 2, Q2 pick option 3",
			body: `{"answers":[
				{"option_indices":[1],"freeform":"","multi":false},
				{"option_indices":[2],"freeform":"","multi":false}
			],"option_counts":[3,4]}`,
			assert: func(t *testing.T, m mockExpector) {
				// Q1: Down + Enter
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Down").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
				// Q2: Down Down + Enter
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Down").Return(nil).Twice()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name: "freeform navigates to last option, Tab to open notes, types text, Enter",
			body: `{"answers":[
				{"option_indices":[],"freeform":"my custom","multi":false}
			],"option_counts":[3]}`,
			assert: func(t *testing.T, m mockExpector) {
				// Navigate to the auto-added Other entry — codex appends it as the
				// last selectable row, so for 3 options we step Down 3 times.
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Down").Return(nil).Times(3)
				// Tab opens the notes input per the picker footer.
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Tab").Return(nil).Once()
				// Literal text via -l flag, then Enter to submit.
				m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "my custom").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := withMockTmuxRunner(t)
			mockReadAgentState(m)
			m.On("Output", mock.Anything,
				"display-message", "-p", "-t", "%1",
				"#{session_name}:#{window_index}.#{pane_index}",
			).Return([]byte("main:0.0\n"), nil)
			tc.assert(t, m)

			agent := domain.Agent{
				SessionID:   "aq-codex",
				State:       "question",
				CurrentTool: "request_user_input",
				Harness:     "codex",
				TmuxPaneID:  "%1",
				Cwd:         "/tmp/repo",
				PendingQuestion: &domain.PendingQuestion{
					ToolUseID: "tool_codex",
					Questions: []domain.PendingQuestionPrompt{{Question: "Which?"}},
				},
			}
			ts, _ := createTestServer(t, agent)

			req, _ := http.NewRequest("POST",
				ts.URL+"/api/agents/aq-codex/answer-question",
				strings.NewReader(tc.body))
			req.Header.Set("X-Requested-With", "dashboard")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestHandleAnswerQuestionRejectsNonQuestionState asserts the endpoint refuses
// to drive the picker when the agent isn't currently blocked on
// AskUserQuestion — guards against stale dashboard clicks.
func TestHandleAnswerQuestionRejectsNonQuestionState(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	agent := domain.Agent{
		SessionID:   "aq-running",
		State:       "running",
		CurrentTool: "Bash",
		Harness:     "",
		TmuxPaneID:  "%1",
		Cwd:         "/tmp/repo",
	}
	ts, _ := createTestServer(t, agent)

	req, _ := http.NewRequest("POST",
		ts.URL+"/api/agents/aq-running/answer-question",
		strings.NewReader(`{"answers":[{"option_indices":[0],"freeform":"","multi":false}],"option_counts":[3]}`))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestHandleAnswerQuestion_AcceptsPinnedStateOverride asserts the endpoint
// still drives the picker when ApplyPinnedStates has rewritten the agent's
// State from "question" to "pr". CurrentTool + PendingQuestion are the
// durable signal — agents with an open PR + a pending question must
// still be answerable.
func TestHandleAnswerQuestion_AcceptsPinnedStateOverride(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)
	m.On("Output", mock.Anything,
		"display-message", "-p", "-t", "%1",
		"#{session_name}:#{window_index}.#{pane_index}",
	).Return([]byte("main:0.0\n"), nil)
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "1").Return(nil).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

	agent := domain.Agent{
		SessionID:   "aq-pinned",
		State:       "pr", // post-ApplyPinnedStates: question → pr override
		PinnedState: "pr",
		CurrentTool: "AskUserQuestion",
		PendingQuestion: &domain.PendingQuestion{
			ToolUseID: "tool_x",
			Questions: []domain.PendingQuestionPrompt{{Question: "Confirm?"}},
		},
		Harness:    "",
		TmuxPaneID: "%1",
		Cwd:        "/tmp/repo",
	}
	ts, _ := createTestServer(t, agent)

	req, _ := http.NewRequest("POST",
		ts.URL+"/api/agents/aq-pinned/answer-question",
		strings.NewReader(`{"answers":[{"option_indices":[0],"freeform":"","multi":false}],"option_counts":[1]}`))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (body: %s)", resp.StatusCode, body)
	}
}

// TestHandleAnswerQuestion_AcceptsCodexStopOverride asserts the endpoint
// drives the picker for a codex agent whose Stop hook fired before the
// user answered the inline request_user_input picker. CurrentTool gets
// cleared and State becomes "done" — both sidecar paused-on-tool signals
// fail — but the codex rollout still carries the unanswered function_call.
// handleAnswerQuestion falls back to a JSONL scan (same logic as
// handlePendingQuestion), so the click is accepted and the picker is
// driven.
//
// This locks in the codex parity case where codex's Stop semantics
// preempt the request_user_input PreToolUse stamp.
func TestHandleAnswerQuestion_AcceptsCodexStopOverride(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)
	m.On("Output", mock.Anything,
		"display-message", "-p", "-t", "%1",
		"#{session_name}:#{window_index}.#{pane_index}",
	).Return([]byte("main:0.0\n"), nil)
	// Codex picker driving: Down arrow (option index 0 → no Down), Enter to submit.
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	sessionID := "019e9ba2-84c1-77a0-8bb4-ee9e88264f58"
	rolloutDir := filepath.Join(codexHome, "sessions", "2026", "06", "06")
	os.MkdirAll(rolloutDir, 0o755)
	rolloutPath := filepath.Join(rolloutDir, "rollout-2026-06-06T11-00-00-"+sessionID+".jsonl")
	jsonl := `{"timestamp":"2026-06-06T11:00:00Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"header\":\"Scope\",\"question\":\"Which?\",\"options\":[{\"label\":\"A\"},{\"label\":\"B\"}]}]}","call_id":"call_codex_stop"}}
`
	os.WriteFile(rolloutPath, []byte(jsonl), 0o644)

	agent := domain.Agent{
		SessionID:   sessionID,
		State:       "done", // Stop hook fired before user answered
		CurrentTool: "",     // cleared
		Harness:     "codex",
		TmuxPaneID:  "%1",
		Cwd:         "/tmp/repo",
	}
	ts, _ := createTestServer(t, agent)

	req, _ := http.NewRequest("POST",
		ts.URL+"/api/agents/"+sessionID+"/answer-question",
		strings.NewReader(`{"answers":[{"option_indices":[0],"freeform":"","multi":false}],"option_counts":[2]}`))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d (body: %s)", resp.StatusCode, body)
	}
}

// TestHandleAnswerQuestionMidSequenceFailureAbortsPicker asserts that when a
// tmux send fails mid-key-sequence, the handler dispatches Escape to abort
// Claude's picker (returning it to a clean cancelled state instead of
// half-answered) before surfacing the 500 to the FE.
func TestHandleAnswerQuestionMidSequenceFailureAbortsPicker(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)
	m.On("Output", mock.Anything,
		"display-message", "-p", "-t", "%1",
		"#{session_name}:#{window_index}.#{pane_index}",
	).Return([]byte("main:0.0\n"), nil)

	// First digit succeeds; second digit errors. Handler must follow up with Escape.
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "1").Return(nil).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "2").
		Return(http.ErrAbortHandler).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Escape").Return(nil).Once()

	agent := domain.Agent{
		SessionID:   "aq-fail",
		State:       "question",
		CurrentTool: "AskUserQuestion",
		Harness:     "",
		TmuxPaneID:  "%1",
		Cwd:         "/tmp/repo",
		PendingQuestion: &domain.PendingQuestion{
			ToolUseID: "tool_fail",
			Questions: []domain.PendingQuestionPrompt{{Question: "Which?"}},
		},
	}
	ts, _ := createTestServer(t, agent)

	body := `{"answers":[
		{"option_indices":[0],"freeform":"","multi":false},
		{"option_indices":[1],"freeform":"","multi":false}
	],"option_counts":[3,3]}`
	req, _ := http.NewRequest("POST",
		ts.URL+"/api/agents/aq-fail/answer-question",
		strings.NewReader(body))
	req.Header.Set("X-Requested-With", "dashboard")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// TestHandleApprove_HarnessRouting locks the approve/reject keystroke
// semantics per harness.
//
// Claude uses single-letter shortcuts at its permission/plan picker —
// "y" approves, "n" rejects. Codex has no equivalent picker; the plan
// is just a chat artifact and the user advances by sending a chat
// message. Sending "y" into a codex pane would just type a literal "y"
// into the composer. For codex, the approve/reject actions send a
// short literal text that codex's agent can interpret as the user's
// decision.
func TestHandleApprove_HarnessRouting(t *testing.T) {
	t.Run("claude sends y", func(t *testing.T) {
		m := withMockTmuxRunner(t)
		mockReadAgentState(m)
		m.On("Output", mock.Anything,
			"display-message", "-p", "-t", "%1",
			"#{session_name}:#{window_index}.#{pane_index}",
		).Return([]byte("main:0.0\n"), nil)
		m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "y").Return(nil).Once()
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

		agent := domain.Agent{
			SessionID:  "ap-claude",
			State:      "plan",
			Harness:    "",
			TmuxPaneID: "%1",
			Cwd:        "/tmp/repo",
		}
		ts, _ := createTestServer(t, agent)
		req, _ := http.NewRequest("POST", ts.URL+"/api/agents/ap-claude/approve", nil)
		req.Header.Set("X-Requested-With", "dashboard")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("codex sends Approve text via paste-buffer", func(t *testing.T) {
		m := withMockTmuxRunner(t)
		mockReadAgentState(m)
		m.On("Output", mock.Anything,
			"display-message", "-p", "-t", "%1",
			"#{session_name}:#{window_index}.#{pane_index}",
		).Return([]byte("main:0.0\n"), nil)
		// Codex needs paste-buffer + bracketed-paste so the text lands in
		// the input box. State="running" → Tab+Enter (queues the reply
		// behind the in-flight turn); idle → Enter alone. Mirrors
		// handleInput's codex path.
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "C-u").Return(nil).Once()
		m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "Approve").Return(nil).Once()
		m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", "main:0.0").Return(nil).Once()
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Tab").Return(nil).Once()
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

		agent := domain.Agent{
			SessionID:  "ap-codex",
			State:      "running",
			Harness:    "codex",
			TmuxPaneID: "%1",
			Cwd:        "/tmp/repo",
		}
		ts, _ := createTestServer(t, agent)
		req, _ := http.NewRequest("POST", ts.URL+"/api/agents/ap-codex/approve", nil)
		req.Header.Set("X-Requested-With", "dashboard")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestHandleReject_HarnessRouting(t *testing.T) {
	t.Run("claude sends n", func(t *testing.T) {
		m := withMockTmuxRunner(t)
		mockReadAgentState(m)
		m.On("Output", mock.Anything,
			"display-message", "-p", "-t", "%1",
			"#{session_name}:#{window_index}.#{pane_index}",
		).Return([]byte("main:0.0\n"), nil)
		m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "n").Return(nil).Once()
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

		agent := domain.Agent{
			SessionID:  "rj-claude",
			State:      "plan",
			Harness:    "",
			TmuxPaneID: "%1",
			Cwd:        "/tmp/repo",
		}
		ts, _ := createTestServer(t, agent)
		req, _ := http.NewRequest("POST", ts.URL+"/api/agents/rj-claude/reject", nil)
		req.Header.Set("X-Requested-With", "dashboard")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("codex sends Reject text via paste-buffer", func(t *testing.T) {
		m := withMockTmuxRunner(t)
		mockReadAgentState(m)
		m.On("Output", mock.Anything,
			"display-message", "-p", "-t", "%1",
			"#{session_name}:#{window_index}.#{pane_index}",
		).Return([]byte("main:0.0\n"), nil)
		// Idle state → Enter alone (no Tab queue).
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "C-u").Return(nil).Once()
		m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "Reject — please revise the plan").Return(nil).Once()
		m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", "main:0.0").Return(nil).Once()
		m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()

		agent := domain.Agent{
			SessionID:  "rj-codex",
			State:      "idle_prompt",
			Harness:    "codex",
			TmuxPaneID: "%1",
			Cwd:        "/tmp/repo",
		}
		ts, _ := createTestServer(t, agent)
		req, _ := http.NewRequest("POST", ts.URL+"/api/agents/rj-codex/reject", nil)
		req.Header.Set("X-Requested-With", "dashboard")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	})
}
