package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
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

// checkDeps probes each known dependency in parallel (3s per probe).
// Bounds the worst case to ~3s instead of ~12s sequential. Order in the
// returned slice matches depProbes for stable rendering.
func checkDeps() []depStatus {
	out := make([]depStatus, len(depProbes))
	var wg sync.WaitGroup
	for i, p := range depProbes {
		wg.Add(1)
		go func(idx int, probe struct {
			name string
			args []string
			hint string
		}) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := gitRunner.SilentRun(ctx, probe.args[0], probe.args[1:]...)
			ds := depStatus{name: probe.name}
			if err == nil {
				ds.ok = true
			} else {
				ds.hint = probe.hint
			}
			out[idx] = ds
		}(i, p)
	}
	wg.Wait()
	return out
}

// checkDepsCmd runs checkDeps in a goroutine and dispatches a depsReadyMsg.
// Use this in handleKey so the TUI Update loop is not blocked on subprocess
// calls.
func checkDepsCmd() tea.Cmd {
	return func() tea.Msg {
		return depsReadyMsg{deps: checkDeps()}
	}
}

// renderDepsView returns the full-screen content for modeDepsStatus.
func renderDepsView(deps []depStatus, width, _ int) string {
	title := lipgloss.NewStyle().Foreground(themeSapphire).Bold(true).Render("Dependencies")
	okStyle := lipgloss.NewStyle().Foreground(themeGreen)
	bad := lipgloss.NewStyle().Foreground(themeRed)
	dim := lipgloss.NewStyle().Foreground(themeOverlay1)

	if len(deps) == 0 {
		body := dim.Render("Checking dependencies…")
		out := lipgloss.JoinVertical(lipgloss.Left, title, "", body)
		if width > 0 {
			return lipgloss.NewStyle().Width(width).Render(out)
		}
		return out
	}

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
