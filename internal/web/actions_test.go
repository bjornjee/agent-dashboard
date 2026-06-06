package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/stretchr/testify/mock"
)

// TestHandleInputSendsByHarness asserts that the input endpoint uses the
// codex-aware paste-buffer + Tab/Enter submit sequence for codex agents,
// matching the TUI fix in PR #293. Claude agents keep the literal send-keys
// path.
func TestHandleInputSendsByHarness(t *testing.T) {
	cases := []struct {
		name    string
		harness string
		state   string
		assert  func(*testing.T, mockExpector)
	}{
		{
			name:    "claude uses literal send-keys then Enter",
			harness: "",
			state:   "idle_prompt",
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-l", "-t", "main:0.0", "hello").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name:    "codex running uses paste-buffer + Tab,Enter to queue",
			harness: "codex",
			state:   "running",
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "C-u").Return(nil).Once()
				m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "hello").Return(nil).Once()
				m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", "main:0.0").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Tab").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
			},
		},
		{
			name:    "codex idle uses paste-buffer + Enter only",
			harness: "codex",
			state:   "idle_prompt",
			assert: func(t *testing.T, m mockExpector) {
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "C-u").Return(nil).Once()
				m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "hello").Return(nil).Once()
				m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", "main:0.0").Return(nil).Once()
				m.On("Run", mock.Anything, "send-keys", "-t", "main:0.0", "Enter").Return(nil).Once()
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

			req, _ := http.NewRequest("POST", ts.URL+"/api/agents/send-1/input",
				strings.NewReader(`{"text":"hello"}`))
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

// TestHandleAnswerQuestionRejectsWrongHarness asserts that codex agents are
// rejected from /answer-question — codex has no AskUserQuestion picker, so
// driving keys would land in its chat composer.
func TestHandleAnswerQuestionRejectsWrongHarness(t *testing.T) {
	m := withMockTmuxRunner(t)
	mockReadAgentState(m)

	agent := domain.Agent{
		SessionID:   "aq-codex",
		State:       "question",
		CurrentTool: "AskUserQuestion",
		Harness:     "codex",
		TmuxPaneID:  "%1",
		Cwd:         "/tmp/repo",
	}
	ts, _ := createTestServer(t, agent)

	req, _ := http.NewRequest("POST",
		ts.URL+"/api/agents/aq-codex/answer-question",
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
