package main

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

//go:embed catppuccin-frappe.json
var catppuccinFrappeStyle []byte

// safeWindowNameRe matches characters safe for tmux window names.
var safeWindowNameRe = regexp.MustCompile(`[^a-zA-Z0-9_.\-]`)

// sanitizeWindowName strips unsafe characters from a tmux window name.
// Characters like : would break target parsing (session:window.pane).
func sanitizeWindowName(name string) string {
	safe := safeWindowNameRe.ReplaceAllString(name, "_")
	if safe == "" {
		return "claude"
	}
	return safe
}

// repoFromCwd extracts the repo name from a working directory path.
// For worktree paths like /foo/worktrees/skills/branch-name, returns "skills".
// For normal paths like /foo/skills, returns "skills" (filepath.Base).
func repoFromCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	parts := strings.SplitN(cwd, "/worktrees/", 2)
	if len(parts) == 2 && parts[1] != "" {
		// Worktree: repo is the first component after /worktrees/
		repo := strings.SplitN(parts[1], "/", 2)[0]
		return repo
	}
	base := filepath.Base(cwd)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

// agentLabel returns a display label for an agent: "repo/branch" with fallbacks.
// When WorktreeCwd is set, it is preferred over Cwd for deriving the repo name
// (the agent may be operating in a worktree whose path differs from the launch dir).
func agentLabel(agent Agent) string {
	repo := repoFromCwd(agent.WorktreeCwd)
	if repo == "" {
		repo = repoFromCwd(agent.Cwd)
	}
	branch := agent.Branch

	if repo != "" && branch != "" {
		return repo + "/" + branch
	}
	if repo != "" {
		return repo
	}
	if branch != "" {
		return branch
	}
	return agent.Session
}

// modelShort returns a single-letter model indicator with color.
func modelShort(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return lipgloss.NewStyle().Foreground(themePink).Render("O")
	case strings.Contains(m, "sonnet"):
		return lipgloss.NewStyle().Foreground(themeSapphire).Render("S")
	case strings.Contains(m, "haiku"):
		return lipgloss.NewStyle().Foreground(themeTeal).Render("H")
	}
	return ""
}

// permissionModeColor returns the ANSI 256 color for a permission mode,
// matching Claude Code's visual language.
func permissionModeColor(mode string) lipgloss.Color {
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

// agentBadges returns a compact metadata string like "auto Bash [2]".
func agentBadges(agent Agent) string {
	var parts []string
	if agent.PermissionMode != "" && agent.PermissionMode != "default" {
		parts = append(parts, permissionModeStyle(agent.PermissionMode))
	}
	if agent.CurrentTool != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(themeSubtext0).
			Render(agent.CurrentTool))
	}
	if agent.SubagentCount > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(runningColor).
			Render(fmt.Sprintf("[%d]", agent.SubagentCount)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
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
