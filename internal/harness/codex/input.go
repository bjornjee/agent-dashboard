package codex

import (
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// sleep is the settle delay used before capturing the codex footer. It is a
// package variable so tests can stub it out (see export_test.go) instead of
// paying the real wall-clock delay on every queue-prone subtest.
var sleep = time.Sleep

// SubmitKeysAfterPaste decides the keystrokes that submit a reply already
// pasted into a codex composer. It prefers the visible pane footer over the
// sidecar state: hooks can leave the state "running" after the prompt is
// already idle, while the codex footer explicitly says when Tab is required to
// queue a reply behind an in-flight turn. Returns "Tab","Enter" when the reply
// must queue, "Enter" otherwise. Falls back to the state-only decision when the
// pane cannot be captured.
func SubmitKeysAfterPaste(target, state string) []string {
	if stateMayQueue(state) {
		sleep(100 * time.Millisecond)
	}
	if lines, err := tmux.TmuxCapture(target, 20); err == nil && len(lines) > 0 {
		if paneShowsQueueHint(lines) {
			return []string{"Tab", "Enter"}
		}
		if state == "permission" || state == "plan" {
			return []string{"Tab", "Enter"}
		}
		return []string{"Enter"}
	}
	return fallbackSubmitKeys(state)
}

// stateMayQueue reports whether the sidecar state suggests a turn may be in
// flight, warranting a short settle before the footer is captured.
func stateMayQueue(state string) bool {
	switch state {
	case "running", "permission", "plan":
		return true
	default:
		return false
	}
}

// paneShowsQueueHint reports whether the captured codex footer instructs the
// user to press Tab to queue the reply behind an in-flight turn.
func paneShowsQueueHint(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "tab to queue") {
			return true
		}
	}
	return false
}

// fallbackSubmitKeys is the state-only decision used when the pane footer is
// unreadable: queue-prone states need Tab,Enter; everything else uses Enter.
func fallbackSubmitKeys(state string) []string {
	switch state {
	case "running", "permission", "plan":
		return []string{"Tab", "Enter"}
	default:
		return []string{"Enter"}
	}
}
