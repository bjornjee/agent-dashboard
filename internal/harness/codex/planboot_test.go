package codex_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/stretchr/testify/mock"
)

// Pane fixtures recorded from live codex 0.142.5 tmux sessions (2026-07-06
// injection test; 2026-07-08 dashboard spawn %95/%96 post-mortem).
//
// The " · " composer footer renders while the model catalog is still
// loading, so it is NOT a configured signal on its own — codex renders the
// full session banner ("model: <name>", no "loading") only from its
// SessionConfigured handler, strictly after the collaboration-mask reset
// that clobbers early /plan dispatches.
const (
	paneHooksDialog = `  Hooks need review
  6 hooks are new or changed.
› 1. Review hooks
  2. Trust all and continue
  Press enter to confirm or esc to go back`

	paneModelLoading = `╭───────────────────────────────────────╮
│ >_ OpenAI Codex (v0.142.5)            │
│ model:     loading   /model to change │
╰───────────────────────────────────────╯
› Write tests for @filename
  gpt-5.5 default fast · ~/.dotfiles/dotfiles`

	paneConfigured = `╭───────────────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.142.5)                        │
│ model:     gpt-5.5 high   fast   /model to change │
╰───────────────────────────────────────────────────╯
  Tip: Try the Codex App.
› Implement {feature}
  gpt-5.5 high fast · ~/Code/bjornjee/agent-dashboard`

	paneConfiguredPlanMode = `│ model:     gpt-5.5 high   fast   /model to change │
• Model changed to gpt-5.5 medium for Plan mode.
› Implement {feature}
  gpt-5.5 medium fast · ~/Code/bjornjee/agent-dashboard                    Plan mode (shift+tab to cycle)`
)

const (
	bootTargetPositional = "main:2.1"
	bootPaneID           = "%42"
)

// expectPasteWithSubmit registers the tmux call sequence
// TmuxPasteKeysClearingInput produces: C-u clear, set-buffer, bracketed
// paste, Enter — addressed to the stable pane id, never the positional
// target (pane indexes renumber when panes close; a stale bootstrap once
// injected into a different agent's pane that inherited its position).
func expectPasteWithSubmit(m *mocks.MockRunner, text string) {
	m.On("Run", mock.Anything, "send-keys", "-t", bootPaneID, "C-u").Return(nil).Once()
	m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", text).Return(nil).Once()
	m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", bootPaneID).Return(nil).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", bootPaneID, "Enter").Return(nil).Once()
}

func expectCapture(m *mocks.MockRunner, pane string) {
	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootPaneID, "-S", "-20").
		Return([]byte(pane), nil).Once()
}

// Happy path: the bootstrap waits through the hooks dialog and the
// model-loading banner until the configured banner renders, then pastes
// /plan, verifies the "Plan mode" footer, and pastes the prompt.
func TestBootstrapPlanMode_WaitsForConfiguredBannerThenInjects(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	expectCapture(m, paneHooksDialog)
	expectCapture(m, paneModelLoading) // " · " footer alone must NOT pass
	expectCapture(m, paneConfigured)
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneConfiguredPlanMode)
	expectPasteWithSubmit(m, "$agent-dashboard:feature add login")

	got := codex.BootstrapPlanModeForTest(bootTargetPositional, bootPaneID, "$agent-dashboard:feature add login")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}

// A /plan that does not verify (e.g. dispatched into a transient popup, or
// swallowed by a dialog) is retried until the "Plan mode" footer confirms.
func TestBootstrapPlanMode_RetriesPlanUntilVerified(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	expectCapture(m, paneConfigured)
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneConfigured) // no Plan mode yet → retry
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneConfiguredPlanMode)
	expectPasteWithSubmit(m, "prompt")

	got := codex.BootstrapPlanModeForTest(bootTargetPositional, bootPaneID, "prompt")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}

// All gates exhausted still injects: a session that never renders the
// configured banner or never confirms Plan mode degrades to today's
// behavior — the skill's own /plan hard-gate asks the user. A dead pane
// with a swallowed prompt is the failure mode this prevents.
func TestBootstrapPlanMode_TimeoutInjectsAnyway(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(0)()
	defer codex.SetPlanVerifyBudget(0)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	expectCapture(m, paneModelLoading) // readiness: one look, then deadline
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneModelLoading) // verify: one look, then deadline
	expectPasteWithSubmit(m, "prompt")

	got := codex.BootstrapPlanModeForTest(bootTargetPositional, bootPaneID, "prompt")
	if got != codex.PlanbootTimedOutForTest {
		t.Errorf("bootstrapPlanMode = %v, want timed out", got)
	}
}

// A capture error means the pane is gone (killed or respawned). The
// bootstrap must abort without pasting anything — a stale bootstrap once
// injected its /plan and prompt into an unrelated agent's pane that had
// inherited the positional target.
func TestBootstrapPlanMode_PaneGoneAbortsWithoutInjecting(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootPaneID, "-S", "-20").
		Return(nil, errors.New("can't find pane %42")).Once()

	got := codex.BootstrapPlanModeForTest(bootTargetPositional, bootPaneID, "prompt")
	if got != codex.PlanbootAbortedForTest {
		t.Errorf("bootstrapPlanMode = %v, want aborted", got)
	}
}

// Without a pane id (tmux variants that omit it) the bootstrap falls back
// to the positional target rather than doing nothing.
func TestBootstrapPlanMode_FallsBackToPositionalTarget(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTargetPositional, "-S", "-20").
		Return([]byte(paneConfigured), nil).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", bootTargetPositional, "C-u").Return(nil).Twice()
	m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "/plan").Return(nil).Once()
	m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", "prompt").Return(nil).Once()
	m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", bootTargetPositional).Return(nil).Twice()
	m.On("Run", mock.Anything, "send-keys", "-t", bootTargetPositional, "Enter").Return(nil).Twice()
	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTargetPositional, "-S", "-20").
		Return([]byte(paneConfiguredPlanMode), nil).Once()

	got := codex.BootstrapPlanModeForTest(bootTargetPositional, "", "prompt")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}
