package codex_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/stretchr/testify/mock"
)

// Pane fixtures recorded from a live codex 0.142.5 tmux session
// (2026-07-06 injection test). The boot/dialog screens have no footer; the
// ready composer renders the "<model> <effort> <speed> · <cwd>" footer line —
// the " · " separator is the readiness marker paneShowsComposer keys on.
// After /plan dispatches, the footer gains the "Plan mode" indicator.
const (
	paneHooksDialog = `  Hooks need review
  6 hooks are new or changed.
› 1. Review hooks
  2. Trust all and continue
  Press enter to confirm or esc to go back`

	paneComposerReady = `╭───────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.142.5)                │
╰───────────────────────────────────────────╯
› Implement {feature}
  gpt-5.5 high fast · ~/Code/bjornjee/agent-dashboard`

	paneComposerPlanMode = `• Model changed to gpt-5.5 medium for Plan mode.
› Implement {feature}
  gpt-5.5 medium fast · ~/Code/bjornjee/agent-dashboard                    Plan mode (shift+tab to cycle)`
)

const (
	bootTarget = "main:2.1"
	bootPaneID = "%42"
)

// stateDirWithPane creates a dashboard state dir whose agents/ holds one
// state file claiming paneID — the signal the SessionStart hook writes once
// codex's session is configured.
func stateDirWithPane(t *testing.T, paneID string) string {
	t.Helper()
	dir := t.TempDir()
	agents := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(domain.Agent{SessionID: "s1", TmuxPaneID: paneID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "s1.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// expectPasteWithSubmit registers the tmux call sequence
// TmuxPasteKeysClearingInput produces: C-u clear, set-buffer, bracketed
// paste, Enter. Bracketed paste + separate Enter is the injection recipe
// proven against codex's paste-burst suppression (codex-rs paste_burst.rs).
func expectPasteWithSubmit(m *mocks.MockRunner, text string) {
	m.On("Run", mock.Anything, "send-keys", "-t", bootTarget, "C-u").Return(nil).Once()
	m.On("Run", mock.Anything, "set-buffer", "-b", "agent-dashboard-reply", "--", text).Return(nil).Once()
	m.On("Run", mock.Anything, "paste-buffer", "-p", "-r", "-d", "-b", "agent-dashboard-reply", "-t", bootTarget).Return(nil).Once()
	m.On("Run", mock.Anything, "send-keys", "-t", bootTarget, "Enter").Return(nil).Once()
}

func expectCapture(m *mocks.MockRunner, pane string) {
	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTarget, "-S", "-20").
		Return([]byte(pane), nil).Once()
}

// Happy path: the bootstrap gates on (1) the composer footer, (2) the
// session-up signal — the agent state file the SessionStart hook writes for
// this pane. Codex clobbers a user-set Plan mask when SessionConfigured
// lands (codex-rs session_flow.rs resets active_collaboration_mask), so
// injecting before the state file exists loses plan mode. Only then does it
// paste bare /plan, verify the "Plan mode" footer, and paste the prompt.
func TestBootstrapPlanMode_GatesOnSessionUpThenInjects(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()
	stateDir := stateDirWithPane(t, bootPaneID)

	expectCapture(m, paneHooksDialog)
	expectCapture(m, paneComposerReady)
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneComposerPlanMode)
	expectPasteWithSubmit(m, "$agent-dashboard:feature add login")

	got := codex.BootstrapPlanModeForTest(bootTarget, bootPaneID, stateDir, "$agent-dashboard:feature add login")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}

// The session-up gate must hold while the composer is already rendered but
// the pane's state file has not appeared yet — injecting in that window is
// exactly the SessionConfigured mask-reset race.
func TestBootstrapPlanMode_WaitsForStateFileBeforeInjecting(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	// State file appears only after the first session-up poll misses.
	stateDir := t.TempDir()
	polled := false
	restore := codex.SetSessionUpProbe(func(dir, paneID string) bool {
		if !polled {
			polled = true
			return false
		}
		return dir == stateDir && paneID == bootPaneID
	})
	defer restore()

	expectCapture(m, paneComposerReady)
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneComposerPlanMode)
	expectPasteWithSubmit(m, "prompt")

	got := codex.BootstrapPlanModeForTest(bootTarget, bootPaneID, stateDir, "prompt")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
	if !polled {
		t.Error("session-up probe was never consulted")
	}
}

// All gates exhausted still injects: a session that never renders the
// composer, never writes a state file, or never confirms Plan mode degrades
// to today's behavior — the skill's own /plan hard-gate asks the user. A
// dead pane with a swallowed prompt is the failure mode this prevents.
func TestBootstrapPlanMode_TimeoutInjectsAnyway(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(0)()
	defer codex.SetPlanVerifyBudget(0)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	expectCapture(m, paneHooksDialog) // composer gate: one look, then deadline
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneHooksDialog) // verify gate: one look, then deadline
	expectPasteWithSubmit(m, "prompt")

	got := codex.BootstrapPlanModeForTest(bootTarget, bootPaneID, t.TempDir(), "prompt")
	if got != codex.PlanbootTimedOutForTest {
		t.Errorf("bootstrapPlanMode = %v, want timed out", got)
	}
}

// Plan-mode verification failure (e.g. collaboration modes disabled in the
// user's codex config — /plan dispatch shows an info message and does
// nothing) still submits the prompt and reports the timeout.
func TestBootstrapPlanMode_PlanVerifyFailureStillSubmitsPrompt(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()
	defer codex.SetPlanVerifyBudget(0)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()
	stateDir := stateDirWithPane(t, bootPaneID)

	expectCapture(m, paneComposerReady)
	expectPasteWithSubmit(m, "/plan")
	expectCapture(m, paneComposerReady) // no "Plan mode" marker, deadline 0
	expectPasteWithSubmit(m, "prompt")

	got := codex.BootstrapPlanModeForTest(bootTarget, bootPaneID, stateDir, "prompt")
	if got != codex.PlanbootTimedOutForTest {
		t.Errorf("bootstrapPlanMode = %v, want timed out", got)
	}
}
