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
	name    string
	purpose string // what the dashboard uses this for
	ok      bool
	hint    string // remediation, shown when !ok
}

type depProbe struct {
	name    string
	args    []string
	purpose string
	hint    string
}

// depProbes is the canonical list of deps the dashboard surfaces. All probes
// go through gitRunner so tests can swap a mock.
var depProbes = []depProbe{
	{"gh", []string{"gh", "auth", "status"}, "PR create, merge, auth", "run 'gh auth login' or install gh (brew install gh)"},
	{"tmux", []string{"tmux", "-V"}, "Agent pane control & jumps", "install tmux (brew install tmux)"},
	{"git", []string{"git", "--version"}, "Repository state & diffs", "install git"},
	{"codex", []string{"codex", "--version"}, "Codex delegation", "install codex CLI (https://github.com/openai/codex)"},
}

// checkDeps probes each known dependency in parallel (3s per probe).
// Bounds the worst case to ~3s instead of ~12s sequential. Order in the
// returned slice matches depProbes for stable rendering.
func checkDeps() []depStatus {
	out := make([]depStatus, len(depProbes))
	var wg sync.WaitGroup
	for i, p := range depProbes {
		wg.Add(1)
		go func(idx int, probe depProbe) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := gitRunner.SilentRun(ctx, probe.args[0], probe.args[1:]...)
			ds := depStatus{name: probe.name, purpose: probe.purpose}
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
	title := lipgloss.NewStyle().Foreground(themeSapphire).Bold(true).Render("Dependency Status")
	subtitle := lipgloss.NewStyle().Foreground(themeSubtext0).Render(
		"External tools the dashboard relies on. Missing tools disable specific features.")
	okStyle := lipgloss.NewStyle().Foreground(themeGreen)
	bad := lipgloss.NewStyle().Foreground(themeRed)
	dim := lipgloss.NewStyle().Foreground(themeOverlay1)
	hintStyle := lipgloss.NewStyle().Foreground(themePeach)

	if len(deps) == 0 {
		body := dim.Render("Checking dependencies…")
		out := lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", "  "+body)
		if width > 0 {
			return lipgloss.NewStyle().Width(width).Render(out)
		}
		return out
	}

	var rows []string
	okCount := 0
	for _, d := range deps {
		rowStyle := bad
		mark := "✗"
		if d.ok {
			rowStyle = okStyle
			mark = "✓"
			okCount++
		}
		rows = append(rows, fmt.Sprintf("   %s  %s  %s",
			rowStyle.Render(mark),
			rowStyle.Render(fmt.Sprintf("%-6s", d.name)),
			dim.Render(d.purpose),
		))
		if !d.ok {
			rows = append(rows, fmt.Sprintf("            %s %s",
				dim.Render("→"),
				hintStyle.Render(d.hint),
			))
		}
	}

	var summary string
	if okCount == len(deps) {
		summary = okStyle.Render(fmt.Sprintf("All %d of %d dependencies available.", okCount, len(deps)))
	} else {
		summary = bad.Render(fmt.Sprintf("%d of %d dependencies available.", okCount, len(deps)))
	}

	footer := dim.Render("  r refresh · esc / q close")
	body := strings.Join(rows, "\n")
	out := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		body,
		"",
		"  "+summary,
		"",
		footer,
	)
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(out)
	}
	return out
}
