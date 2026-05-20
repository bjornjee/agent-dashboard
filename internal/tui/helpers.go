package tui

import (
	_ "embed"
	"fmt"
	"image/color"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/repowin"
)

//go:embed catppuccin-frappe.json
var catppuccinFrappeStyle []byte

// accentBarGlyph is the single-cell left-edge accent rendered on the selected
// row to give selection a strong peripheral cue independent of the background
// highlight.
const accentBarGlyph = "▌"

// withAccentBar replaces the first leading space of line with a styled accent
// bar tinted to harness. Net visual width is unchanged: the bar is one cell
// wide and replaces one space. Returns line unchanged when it has no leading
// space.
//
// Apply this BEFORE highlightLine so the bar participates in the background
// overlay; highlightLine is ANSI-aware and re-applies bg after the bar's
// foreground reset, so both colors persist.
func withAccentBar(line string, harness string) string {
	if !strings.HasPrefix(line, " ") {
		return line
	}
	bar := lipgloss.NewStyle().Foreground(harnessColor(harness)).Render(accentBarGlyph)
	return bar + line[1:]
}

// highlightLine applies a background highlight to a line while preserving
// inner ANSI foreground colors. It pads the line to the given width and
// re-applies the background after each SGR reset so colors aren't lost.
func highlightLine(line string, width int) string {
	// themeSurface1 = #51576d = RGB(81, 87, 109)
	const bgCode = "\x1b[48;2;81;87;109m"
	const boldCode = "\x1b[1m"
	const resetFull = "\x1b[0m"
	const resetShort = "\x1b[m"

	// Re-apply background after each inner reset so it persists
	// lipgloss v2 uses \x1b[m instead of \x1b[0m
	inner := strings.ReplaceAll(line, resetFull, resetFull+bgCode+boldCode)
	inner = strings.ReplaceAll(inner, resetShort, resetShort+bgCode+boldCode)

	// Pad to full width
	visWidth := lipgloss.Width(line)
	padding := ""
	if visWidth < width {
		padding = strings.Repeat(" ", width-visWidth)
	}

	return bgCode + boldCode + inner + padding + resetFull
}

// agentRepo extracts the repo name from an agent, preferring WorktreeCwd.
func agentRepo(agent domain.Agent) string {
	repo := repowin.RepoFromCwd(agent.WorktreeCwd)
	if repo == "" {
		repo = repowin.RepoFromCwd(agent.Cwd)
	}
	return repo
}

// branchColor returns the theme color for a branch based on its prefix.
func branchColor(branch string) color.Color {
	b := strings.ToLower(branch)
	switch {
	case b == "main" || b == "master":
		return themeText
	case strings.HasPrefix(b, "feat/") || strings.HasPrefix(b, "feature/"):
		return themeGreen
	case strings.HasPrefix(b, "fix/"):
		return themePeach
	case strings.HasPrefix(b, "hotfix/"):
		return themeRed
	case strings.HasPrefix(b, "chore/"):
		return themeLavender
	case strings.HasPrefix(b, "refactor/"):
		return themeYellow
	case strings.HasPrefix(b, "release/"):
		return themeMauve
	default:
		return themeSubtext0
	}
}

// styledBranch renders a branch name with prefix-appropriate color.
func styledBranch(branch string) string {
	return lipgloss.NewStyle().Foreground(branchColor(branch)).Render(branch)
}

// agentRepoStyled returns only the styled repo name (no branch), for the left panel first line.
func agentRepoStyled(agent domain.Agent) string {
	repo := agentRepo(agent)
	if repo != "" {
		return lipgloss.NewStyle().Foreground(themeSapphire).Bold(true).Render(repo)
	}
	return agent.Session
}

// wrapMetaLine renders a right-panel metadata line with a padded label and a value,
// wrapping long values with proper indentation so all values stay aligned.
func wrapMetaLine(label string, labelWidth int, value string, totalWidth int) []string {
	prefix := fmt.Sprintf(" %s ", padLabel(label, labelWidth))
	prefixWidth := 1 + labelWidth + 1 // leading space + label visual width + trailing space
	valueWidth := totalWidth - prefixWidth
	if valueWidth <= 0 {
		return []string{prefix + value}
	}

	wrapped := wrapText(value, valueWidth)
	if len(wrapped) == 0 {
		return []string{prefix + value}
	}

	indent := strings.Repeat(" ", prefixWidth)
	var lines []string
	lines = append(lines, prefix+wrapped[0])
	for _, w := range wrapped[1:] {
		lines = append(lines, indent+w)
	}
	return lines
}

// padLabel renders a label with dim style and pads it to a fixed visual width.
// If the rendered label is already wider than width, it is returned as-is.
func padLabel(label string, width int) string {
	dimLabel := lipgloss.NewStyle().Foreground(themeSubtext0)
	rendered := dimLabel.Render(label)
	renderedWidth := lipgloss.Width(rendered)
	if renderedWidth < width {
		rendered += strings.Repeat(" ", width-renderedWidth)
	}
	return rendered
}

// permissionModeColor returns the ANSI 256 color for a permission mode,
// matching Claude Code's visual language.
func permissionModeColor(mode string) color.Color {
	m := strings.ToLower(mode)
	switch {
	case strings.Contains(m, "plan"):
		return themeMauve
	case strings.Contains(m, "auto") && strings.Contains(m, "edit"):
		return themeYellow
	case strings.Contains(m, "full") && strings.Contains(m, "auto"):
		return themeGreen
	case strings.Contains(m, "bypass"):
		return themeRed
	default:
		return themeOverlay1
	}
}

// permissionModeStyle returns the permission mode string rendered with a
// color that matches Claude Code's visual language.
func permissionModeStyle(mode string) string {
	return lipgloss.NewStyle().Foreground(permissionModeColor(mode)).Render(mode)
}

// normalizeHarness collapses empty/unknown harness strings to "claude" to
// match the back-compat default documented at domain/types.go:73.
func normalizeHarness(harness string) string {
	if harness == "codex" {
		return "codex"
	}
	return "claude"
}

// harnessColor returns the theme color for a given harness name.
// Empty / unknown harnesses are treated as "claude".
func harnessColor(harness string) color.Color {
	if normalizeHarness(harness) == "codex" {
		return themePeach
	}
	return themeSapphire
}

// harnessBadgeToken renders the harness label for the metadata badges row.
// Selected rows render in the full harness color + bold; non-selected rows
// render in themeSubtext0 so the badge fades into the background while still
// being scannable on every row.
func harnessBadgeToken(harness string, selected bool) string {
	label := normalizeHarness(harness)
	if selected {
		return lipgloss.NewStyle().Foreground(harnessColor(harness)).Bold(true).Render(label)
	}
	return lipgloss.NewStyle().Foreground(themeSubtext0).Render(label)
}

// agentBadges returns a compact metadata string like "claude bypass max Bash [2]".
// The harness badge is always the first token; selected controls whether it is
// rendered in the full harness color (selected) or dimmed (unselected).
func agentBadges(agent domain.Agent, selected bool) string {
	parts := []string{harnessBadgeToken(agent.Harness, selected)}
	if agent.PermissionMode != "" && agent.PermissionMode != "default" {
		parts = append(parts, permissionModeStyle(agent.PermissionMode))
	}
	if agent.Effort != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(effortColor).
			Render(agent.Effort))
	}
	if agent.CurrentTool != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(themeSubtext0).
			Render(agent.CurrentTool))
	}
	if agent.SubagentCount > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(runningColor).
			Render(fmt.Sprintf("[%d]", agent.SubagentCount)))
	}
	return strings.Join(parts, " ")
}

// trimTrailingBlankLines removes trailing whitespace-only lines from a slice.
func trimTrailingBlankLines(lines []string) []string {
	i := len(lines)
	for i > 0 && strings.TrimSpace(lines[i-1]) == "" {
		i--
	}
	if i == 0 {
		return nil
	}
	return lines[:i]
}

func hasContent(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return true
		}
	}
	return false
}

// renderPlanMarkdown renders markdown content using glamour with syntax highlighting.
// Falls back to plain wrapText on error. Returns empty string for empty input.
func renderPlanMarkdown(content string, width int) string {
	if content == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(catppuccinFrappeStyle),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.Join(wrapText(content, width), "\n")
	}
	out, err := r.Render(content)
	if err != nil {
		return strings.Join(wrapText(content, width), "\n")
	}
	return strings.TrimRight(out, "\n")
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var result []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > width {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
}
