package tmux

import "strings"

// ContainsTrustPrompt returns true if the pane buffer contains a folder
// trust dialog from a supported harness. Claude Code uses the select-menu
// option "Yes, I trust this folder"; codex (codex-rs/tui/src/onboarding/
// trust_directory.rs) renders the question "Do you trust the contents of
// this directory?". Either is enough to surface the "trust prompt
// detected" signal — the user resolves the prompt with the harness's own
// keybinds once they're inside the pane.
func ContainsTrustPrompt(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "Yes, I trust this folder") ||
			strings.Contains(line, "Do you trust the contents of this directory?") {
			return true
		}
	}
	return false
}
