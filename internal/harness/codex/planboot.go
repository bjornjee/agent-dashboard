package codex

import (
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// bootReadyBudget bounds how long the bootstrap waits for codex's configured
// session banner. Codex may sit on hook/folder-trust dialogs the user has to
// answer first, so this mirrors the web trust watcher's budget
// (internal/web/trust.go).
var bootReadyBudget = 30 * time.Second

// planVerifyBudget bounds the /plan paste-verify-retry loop.
var planVerifyBudget = 10 * time.Second

// bootTick is the polling cadence, matching trustWatchTick.
const bootTick = 300 * time.Millisecond

// planSettle is the pause between pasting /plan and checking the footer,
// giving codex a frame to dispatch and re-render.
const planSettle = 500 * time.Millisecond

// BootstrapPlanMode drives a freshly spawned codex pane into Plan mode and
// submits the deferred skill prompt. Blocking; callers run it in a goroutine.
//
// All tmux operations address the stable pane id when available — positional
// targets ("session:win.pane") renumber when panes close, and a stale
// bootstrap once injected into an unrelated agent's pane that had inherited
// its position. A capture failure means the pane is gone: abort, never
// inject blind.
//
// Gate order matters:
//
//  1. Configured banner rendered — the "model: <name>" session-info banner
//     (without "loading") that codex draws only from its SessionConfigured
//     handler. The " · " composer footer alone is NOT sufficient: it renders
//     while the model catalog is still loading, and codex resets the
//     collaboration mask when SessionConfigured lands (codex-rs
//     chatwidget/session_flow.rs), silently clobbering any earlier /plan.
//  2. Paste bare "/plan" + Enter and verify the footer shows "Plan mode";
//     retry the paste until it verifies or the budget lapses. /plan is
//     idempotent, so re-pasting is safe.
//  3. Paste the prompt + Enter (large pastes become placeholder elements
//     that expand on submit, so any prompt length is safe here).
//
// Every injection is a bracketed paste plus a *separate* Enter
// (TmuxPasteKeysClearingInput): codex's composer treats rapid literal
// keystrokes as a paste burst and suppresses Enter into a newline for 120ms
// (codex-rs/tui/src/bottom_pane/paste_burst.rs PASTE_ENTER_SUPPRESS_WINDOW),
// which is why plain send-keys text+Enter never submits. A bracketed paste
// clears that suppression, and a leading '/' additionally bypasses it.
//
// Best-effort throughout: on gate timeout it keeps going and injects anyway,
// degrading to the skill's own /plan hard-gate instead of leaving a dead
// pane.
func BootstrapPlanMode(target, paneID, prompt string) {
	bootstrapPlanMode(target, paneID, prompt)
}

type planbootResult int

const (
	planbootInjected planbootResult = iota
	planbootTimedOut
	planbootAborted
)

func bootstrapPlanMode(target, paneID, prompt string) planbootResult {
	tgt := paneID
	if tgt == "" {
		tgt = target
	}

	res := planbootInjected
	deadline := time.Now().Add(bootReadyBudget)
	for {
		lines, err := tmux.TmuxCapture(tgt, 20)
		if err != nil {
			return planbootAborted // pane gone — never inject blind
		}
		if paneConfigured(lines) {
			break
		}
		if time.Now().After(deadline) {
			res = planbootTimedOut
			break
		}
		sleep(bootTick)
	}

	// Best-effort pastes from here down: a failed paste leaves the skill's
	// /plan gate to ask the user.
	verified := false
	verifyDeadline := time.Now().Add(planVerifyBudget)
	for {
		_ = tmux.TmuxPasteKeysClearingInput(tgt, "/plan", "Enter")
		sleep(planSettle)
		lines, err := tmux.TmuxCapture(tgt, 20)
		if err != nil {
			return planbootAborted
		}
		if paneShowsPlanMode(lines) {
			verified = true
			break
		}
		if time.Now().After(verifyDeadline) {
			break
		}
		sleep(bootTick)
	}
	if !verified {
		res = planbootTimedOut
	}

	_ = tmux.TmuxPasteKeysClearingInput(tgt, prompt, "Enter")
	return res
}

// paneConfigured reports whether codex has rendered its post-configure
// session banner: the composer footer (" · " between model summary and cwd)
// plus a "model:" banner line that is no longer "loading". Verified against
// codex 0.142.5 — the loading-state banner shows "model:     loading" while
// the composer footer is already visible.
func paneConfigured(lines []string) bool {
	footer, model := false, false
	for _, line := range lines {
		if strings.Contains(line, " · ") {
			footer = true
		}
		if strings.Contains(line, "model:") && !strings.Contains(line, "loading") {
			model = true
		}
	}
	return footer && model
}

// paneShowsPlanMode reports whether the footer carries codex's Plan mode
// indicator ("Plan mode (shift+tab to cycle)" in codex 0.142.5).
func paneShowsPlanMode(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "Plan mode") {
			return true
		}
	}
	return false
}
