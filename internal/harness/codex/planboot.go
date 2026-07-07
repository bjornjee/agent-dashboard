package codex

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// bootReadyBudget bounds how long the bootstrap polls for the codex composer.
// Codex may sit on hook/folder-trust dialogs the user has to answer first, so
// this mirrors the web trust watcher's budget (internal/web/trust.go).
var bootReadyBudget = 30 * time.Second

// bootTick is the composer polling cadence, matching trustWatchTick.
const bootTick = 300 * time.Millisecond

// twoStepSettle is the pause between the bare /plan dispatch and the prompt
// paste in the large-prompt path, giving codex a frame to apply Plan mode.
const twoStepSettle = 500 * time.Millisecond

// largePasteCharThreshold mirrors codex's LARGE_PASTE_CHAR_THRESHOLD
// (codex-rs/tui/src/bottom_pane/chat_composer.rs). Pastes above it are
// replaced with a placeholder element, so a "/plan <prompt>" paste would no
// longer start with '/' and the slash command would not dispatch.
const largePasteCharThreshold = 1000

// BootstrapPlanMode waits for the codex composer in the pane at target, then
// injects "/plan <prompt>" so the session enters Plan mode and submits the
// skill prompt atomically — codex dispatches /plan inline args in one action
// (codex-rs/tui/src/chatwidget/slash_dispatch.rs SlashCommand::Plan).
//
// The injection must be a bracketed paste plus a *separate* Enter
// (TmuxPasteKeysClearingInput): codex's composer treats rapid literal
// keystrokes as a paste burst and suppresses Enter into a newline for 120ms
// (codex-rs/tui/src/bottom_pane/paste_burst.rs PASTE_ENTER_SUPPRESS_WINDOW),
// which is why plain send-keys text+Enter never submits. A bracketed paste
// clears that suppression, and a leading '/' additionally bypasses it.
//
// Blocking; callers run it in a goroutine. Best-effort: on readiness timeout
// it injects anyway, degrading to the skill's own /plan hard-gate instead of
// leaving a dead pane.
func BootstrapPlanMode(target, prompt string) {
	bootstrapPlanMode(target, prompt)
}

type planbootResult int

const (
	planbootInjected planbootResult = iota
	planbootTimedOut
)

func bootstrapPlanMode(target, prompt string) planbootResult {
	res := waitForComposer(target)

	atomic := "/plan " + prompt
	if utf8.RuneCountInString(atomic) <= largePasteCharThreshold {
		// Best-effort: a failed paste leaves the skill's /plan gate to ask the user.
		_ = tmux.TmuxPasteKeysClearingInput(target, atomic, "Enter")
		return res
	}
	// Over codex's large-paste threshold the paste becomes a placeholder
	// element and the leading '/' is lost, so enter Plan mode bare first,
	// then paste the prompt (placeholders expand on submit at any length).
	// Both pastes are best-effort for the same reason as above.
	_ = tmux.TmuxPasteKeysClearingInput(target, "/plan", "Enter")
	sleep(twoStepSettle)
	_ = tmux.TmuxPasteKeysClearingInput(target, prompt, "Enter")
	return res
}

// waitForComposer polls the pane until the codex composer chrome renders,
// or the budget lapses.
func waitForComposer(target string) planbootResult {
	deadline := time.Now().Add(bootReadyBudget)
	for {
		if lines, err := tmux.TmuxCapture(target, 20); err == nil && paneShowsComposer(lines) {
			return planbootInjected
		}
		if time.Now().After(deadline) {
			return planbootTimedOut
		}
		sleep(bootTick)
	}
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
