package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// depStatus is one row in the deps view.
type depStatus struct {
	name string
	ok   bool
	hint string // shown when !ok; empty when ok
}

// depProbes is the canonical list of deps the dashboard surfaces. Each entry
// pairs a name with the command we run to detect availability, plus a hint
// shown when that command exits non-zero. All probes go through gitRunner
// so tests can swap a mock.
var depProbes = []struct {
	name string
	args []string
	hint string
}{
	{"gh", []string{"gh", "auth", "status"}, "run 'gh auth login' or install gh (brew install gh)"},
	{"tmux", []string{"tmux", "-V"}, "install tmux (brew install tmux)"},
	{"git", []string{"git", "--version"}, "install git"},
	{"codex", []string{"codex", "--version"}, "install codex CLI (https://github.com/openai/codex)"},
}

// checkDeps probes each known dependency synchronously, 3s per probe.
func checkDeps() []depStatus {
	out := make([]depStatus, 0, len(depProbes))
	for _, p := range depProbes {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := gitRunner.SilentRun(ctx, p.args[0], p.args[1:]...)
		cancel()
		ds := depStatus{name: p.name}
		if err == nil {
			ds.ok = true
		} else {
			ds.hint = p.hint
		}
		out = append(out, ds)
	}
	return out
}

// renderDepsView returns the full-screen content for modeDepsStatus.
func renderDepsView(deps []depStatus, width, _ int) string {
	title := lipgloss.NewStyle().Foreground(themeSapphire).Bold(true).Render("Dependencies")
	okStyle := lipgloss.NewStyle().Foreground(themeGreen)
	bad := lipgloss.NewStyle().Foreground(themeRed)
	dim := lipgloss.NewStyle().Foreground(themeOverlay1)

	var rows []string
	for _, d := range deps {
		var mark, name, detail string
		if d.ok {
			mark = okStyle.Render("✓")
			name = okStyle.Render(d.name)
			detail = dim.Render("OK")
		} else {
			mark = bad.Render("✗")
			name = bad.Render(d.name)
			detail = d.hint
		}
		rows = append(rows, fmt.Sprintf("  %s %-7s %s", mark, name, detail))
	}

	footer := dim.Render("r refresh · esc / q close")
	body := strings.Join(rows, "\n")
	out := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(out)
	}
	return out
}
