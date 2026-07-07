package codex

import (
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// bootReadyBudget bounds how long the bootstrap waits for the codex composer
// and the session-up signal. Codex may sit on hook/folder-trust dialogs the
// user has to answer first, so this mirrors the web trust watcher's budget
// (internal/web/trust.go).
var bootReadyBudget = 30 * time.Second

// planVerifyBudget bounds the wait for the "Plan mode" footer after /plan.
var planVerifyBudget = 10 * time.Second

// bootTick is the polling cadence, matching trustWatchTick.
const bootTick = 300 * time.Millisecond

// sessionUpProbe reports whether the SessionStart hook has written an agent
// state file for paneID under stateDir. Package var so tests can stub it.
var sessionUpProbe = agentPaneUp

// BootstrapPlanMode drives a freshly spawned codex pane into Plan mode and
// submits the deferred skill prompt. Blocking; callers run it in a goroutine.
//
// Gate order matters:
//
//  1. Composer chrome rendered (footer " · " marker).
//  2. Session configured — probed via the agent state file the SessionStart
//     hook writes for this pane. Codex resets the collaboration mask when
//     SessionConfigured lands (codex-rs chatwidget/session_flow.rs), so a
//     /plan dispatched earlier is silently clobbered and the queued prompt
//     submits in Default mode. The state file only exists after that reset.
//  3. Paste bare "/plan" + Enter and verify the footer shows "Plan mode".
//  4. Paste the prompt + Enter (large pastes become placeholder elements
//     that expand on submit, so any prompt length is safe here).
//
// Every injection is a bracketed paste plus a *separate* Enter
// (TmuxPasteKeysClearingInput): codex's composer treats rapid literal
// keystrokes as a paste burst and suppresses Enter into a newline for 120ms
// (codex-rs/tui/src/bottom_pane/paste_burst.rs PASTE_ENTER_SUPPRESS_WINDOW),
// which is why plain send-keys text+Enter never submits. A bracketed paste
// clears that suppression, and a leading '/' additionally bypasses it.
//
// Best-effort throughout: on any gate timeout it keeps going and injects
// anyway, degrading to the skill's own /plan hard-gate instead of leaving a
// dead pane.
func BootstrapPlanMode(target, paneID, stateDir, prompt string) {
	bootstrapPlanMode(target, paneID, stateDir, prompt)
}

type planbootResult int

const (
	planbootInjected planbootResult = iota
	planbootTimedOut
)

func bootstrapPlanMode(target, paneID, stateDir, prompt string) planbootResult {
	res := planbootInjected
	deadline := time.Now().Add(bootReadyBudget)
	if !waitUntil(deadline, func() bool { return paneShowsComposer(capture(target)) }) {
		res = planbootTimedOut
	}
	// Session-up gate is skipped when the caller has no pane identity (e.g.
	// tmux variants that don't report pane_id) — the composer gate plus the
	// plan-mode verification below still bound the race window.
	if res == planbootInjected && paneID != "" && stateDir != "" {
		if !waitUntil(deadline, func() bool { return sessionUpProbe(stateDir, paneID) }) {
			res = planbootTimedOut
		}
	}

	// Best-effort pastes: a failed paste leaves the skill's /plan gate to
	// ask the user.
	_ = tmux.TmuxPasteKeysClearingInput(target, "/plan", "Enter")
	if !waitUntil(time.Now().Add(planVerifyBudget), func() bool { return paneShowsPlanMode(capture(target)) }) {
		res = planbootTimedOut
	}
	_ = tmux.TmuxPasteKeysClearingInput(target, prompt, "Enter")
	return res
}

// waitUntil polls cond every bootTick until it holds or deadline passes.
func waitUntil(deadline time.Time, cond func() bool) bool {
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		sleep(bootTick)
	}
}

func capture(target string) []string {
	lines, err := tmux.TmuxCapture(target, 20)
	if err != nil {
		return nil
	}
	return lines
}

// paneShowsComposer reports whether the captured pane renders codex's ready
// composer. The readiness marker is the footer's " · " separator between the
// model summary and the cwd ("gpt-5.5 high fast · ~/repo") — boot banners and
// trust/hook dialogs render no footer (verified against codex 0.142.5).
func paneShowsComposer(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, " · ") {
			return true
		}
	}
	return false
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

// agentPaneUp reports whether any agent state file under stateDir claims
// paneID. The SessionStart hook writes it once the codex session is live —
// strictly after the SessionConfigured mask reset described above.
func agentPaneUp(stateDir, paneID string) bool {
	for _, agent := range state.ReadState(stateDir).Agents {
		if agent.TmuxPaneID == paneID {
			return true
		}
	}
	return false
}
