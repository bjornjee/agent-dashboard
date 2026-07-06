package codex_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/stretchr/testify/mock"
)

// Pane fixtures recorded from a live codex 0.142.5 tmux session
// (2026-07-06 injection test). The boot/dialog screens have no footer; the
// ready composer renders the "<model> <effort> <speed> · <cwd>" footer line —
// the " · " separator is the readiness marker paneShowsComposer keys on.
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
)

const bootTarget = "main:2.1"

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

// Happy path: the bootstrap polls until the composer footer renders, then
// injects the atomic "/plan <prompt>" paste (codex's /plan supports inline
// args: enters Plan mode AND submits the args as the user message in one
// dispatch — codex-rs slash_dispatch.rs SlashCommand::Plan).
func TestBootstrapPlanMode_WaitsForComposerThenInjects(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTarget, "-S", "-20").
		Return([]byte(paneHooksDialog), nil).Once()
	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTarget, "-S", "-20").
		Return([]byte(paneComposerReady), nil).Once()
	expectPasteWithSubmit(m, "/plan $agent-dashboard:feature add login")

	got := codex.BootstrapPlanModeForTest(bootTarget, "$agent-dashboard:feature add login")
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}

// Prompts pushing the paste over codex's 1000-char LARGE_PASTE_CHAR_THRESHOLD
// would become a placeholder element and defeat the leading-slash dispatch,
// so the bootstrap falls back to two pastes: bare "/plan" (enters Plan mode),
// then the prompt (large-paste placeholder expands on submit).
func TestBootstrapPlanMode_LargePromptUsesTwoStepInjection(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(time.Second)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	prompt := "$agent-dashboard:feature " + strings.Repeat("x", 1100)
	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTarget, "-S", "-20").
		Return([]byte(paneComposerReady), nil).Once()
	expectPasteWithSubmit(m, "/plan")
	expectPasteWithSubmit(m, prompt)

	got := codex.BootstrapPlanModeForTest(bootTarget, prompt)
	if got != codex.PlanbootInjectedForTest {
		t.Errorf("bootstrapPlanMode = %v, want injected", got)
	}
}

// Readiness timeout still injects: a session that never renders the composer
// marker degrades to today's behavior — the pasted prompt sits in (or
// submits into) the pane and the skill's own /plan hard-gate asks the user.
// A dead pane with a swallowed prompt is the failure mode this prevents.
func TestBootstrapPlanMode_TimeoutInjectsAnyway(t *testing.T) {
	defer codex.SetSleep(func(time.Duration) {})()
	defer codex.SetBootReadyBudget(0)()

	m := mocks.NewMockRunner(t)
	defer tmux.SetTestRunner(m)()

	m.On("Output", mock.Anything, "capture-pane", "-p", "-t", bootTarget, "-S", "-20").
		Return([]byte(paneHooksDialog), nil)
	expectPasteWithSubmit(m, "/plan $agent-dashboard:feature add login")

	got := codex.BootstrapPlanModeForTest(bootTarget, "$agent-dashboard:feature add login")
	if got != codex.PlanbootTimedOutForTest {
		t.Errorf("bootstrapPlanMode = %v, want timed out", got)
	}
}
