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

// mockExpector is the subset of *mocks.MockRunner we use to register
// expectations — keeps the case table free of the full mock type import.
type mockExpector interface {
	On(method string, arguments ...interface{}) *mock.Call
}
