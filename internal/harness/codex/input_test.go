package codex_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/stretchr/testify/mock"
)

// SubmitKeysAfterPaste decides the codex submit keystrokes by inspecting the
// live pane footer rather than trusting the (often stale) sidecar state. The
// "tab to queue" hint means a turn is in flight and the reply must be queued
// with Tab before Enter; a clean footer submits with Enter alone. This mirrors
// the logic that fixed the web send path in PR #377 and is shared by the TUI
// and web reply paths.
func TestSubmitKeysAfterPaste(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	const target = "main:0.0"
	cases := []struct {
		name  string
		state string
		pane  string
		want  []string
	}{
		{
			name:  "footer shows tab to queue hint queues with Tab,Enter",
			state: "running",
			pane:  "› hello\n\n tab to queue message",
			want:  []string{"Tab", "Enter"},
		},
		{
			name:  "clean footer with idle state submits with Enter",
			state: "idle_prompt",
			pane:  "› hello\n\ngpt-5.5 high fast",
			want:  []string{"Enter"},
		},
		{
			name:  "plan state with clean footer queues with Tab,Enter",
			state: "plan",
			pane:  "› hello\n\ngpt-5.5 high fast",
			want:  []string{"Tab", "Enter"},
		},
		{
			name:  "permission state with clean footer queues with Tab,Enter",
			state: "permission",
			pane:  "› hello\n\ngpt-5.5 high fast",
			want:  []string{"Tab", "Enter"},
		},
		{
			name:  "stale running state but idle pane submits with Enter only",
			state: "running",
			pane:  "› hello\n\ngpt-5.5 high fast",
			want:  []string{"Enter"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := mocks.NewMockRunner(t)
			defer tmux.SetTestRunner(m)()
			m.On("Output", mock.Anything,
				"capture-pane", "-p", "-t", target, "-S", "-20",
			).Return([]byte(tc.pane), nil).Once()

			got := codex.SubmitKeysAfterPaste(target, tc.state)
			if !slices.Equal(got, tc.want) {
				t.Errorf("SubmitKeysAfterPaste(%q) = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}

// When the pane capture fails the helper cannot read the footer, so it falls
// back to the state-only decision: queue-prone states (running/permission/plan)
// use Tab,Enter; everything else uses Enter.
func TestSubmitKeysAfterPaste_CaptureErrorFallsBackToState(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	const target = "main:0.0"
	cases := []struct {
		state string
		want  []string
	}{
		{"running", []string{"Tab", "Enter"}},
		{"permission", []string{"Tab", "Enter"}},
		{"plan", []string{"Tab", "Enter"}},
		{"idle_prompt", []string{"Enter"}},
	}

	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			m := mocks.NewMockRunner(t)
			defer tmux.SetTestRunner(m)()
			m.On("Output", mock.Anything,
				"capture-pane", "-p", "-t", target, "-S", "-20",
			).Return(nil, errors.New("capture boom")).Once()

			got := codex.SubmitKeysAfterPaste(target, tc.state)
			if !slices.Equal(got, tc.want) {
				t.Errorf("SubmitKeysAfterPaste(%q) fallback = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}
