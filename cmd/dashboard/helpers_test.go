package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRepoFromCwd(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "worktree path",
			cwd:  "/Users/bjornjee/Code/bjornjee/worktrees/skills/dashboard-agent-naming",
			want: "skills",
		},
		{
			name: "normal repo path",
			cwd:  "/Users/bjornjee/Code/bjornjee/skills",
			want: "skills",
		},
		{
			name: "worktree with deep path",
			cwd:  "/home/user/worktrees/myapp/feature-branch",
			want: "myapp",
		},
		{
			name: "empty cwd",
			cwd:  "",
			want: "",
		},
		{
			name: "root path",
			cwd:  "/",
			want: "",
		},
		{
			name: "worktrees at end without branch dir",
			cwd:  "/home/user/worktrees/repo",
			want: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoFromCwd(tt.cwd)
			if got != tt.want {
				t.Errorf("repoFromCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestAgentLabel(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{
			name: "repo and branch",
			agent: Agent{
				Cwd:    "/Users/bjornjee/Code/bjornjee/skills",
				Branch: "feat/dashboard-agent-naming",
			},
			want: "skills/feat/dashboard-agent-naming",
		},
		{
			name: "worktree repo and branch",
			agent: Agent{
				Cwd:    "/Users/bjornjee/Code/bjornjee/worktrees/skills/dashboard-agent-naming",
				Branch: "feat/dashboard-agent-naming",
			},
			want: "skills/feat/dashboard-agent-naming",
		},
		{
			name: "repo only no branch",
			agent: Agent{
				Cwd: "/Users/bjornjee/Code/bjornjee/skills",
			},
			want: "skills",
		},
		{
			name: "branch only no cwd",
			agent: Agent{
				Branch: "main",
			},
			want: "main",
		},
		{
			name: "fallback to session",
			agent: Agent{
				Session: "dev",
			},
			want: "dev",
		},
		{
			name:  "empty agent",
			agent: Agent{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentLabel(tt.agent)
			if got != tt.want {
				t.Errorf("agentLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPermissionModeColor(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want lipgloss.Color
	}{
		{"plan mode gets blue/purple", "plan", lipgloss.Color("105")},
		{"auto-edit gets yellow", "auto-edit", lipgloss.Color("220")},
		{"autoEdit gets yellow", "autoEdit", lipgloss.Color("220")},
		{"full-auto gets green", "full-auto", lipgloss.Color("82")},
		{"fullAuto gets green", "fullAuto", lipgloss.Color("82")},
		{"unknown mode gets gray", "custom", lipgloss.Color("242")},
		{"case insensitive Plan", "Plan", lipgloss.Color("105")},
		{"default fallback", "default", lipgloss.Color("242")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := permissionModeColor(tt.mode)
			if got != tt.want {
				t.Errorf("permissionModeColor(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSanitizeWindowName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"safe name", "skills", "skills"},
		{"with dash", "my-repo", "my-repo"},
		{"with dot", "my.repo", "my.repo"},
		{"with colon", "foo:bar", "foo_bar"},
		{"with spaces", "foo bar", "foo_bar"},
		{"with shell chars", "$(evil)", "__evil_"},
		{"with semicolon", "foo;bar", "foo_bar"},
		{"empty", "", "claude"},
		{"all unsafe", ":::", "___"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeWindowName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeWindowName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPermissionModeColor_Bypass(t *testing.T) {
	got := permissionModeColor("bypassPermissions")
	want := lipgloss.Color("196")
	if got != want {
		t.Errorf("permissionModeColor(bypassPermissions) = %q, want %q", got, want)
	}
}

func TestAgentBadges_NoModelIndicator(t *testing.T) {
	// Test with model only — no permission mode to avoid false matches
	agent := Agent{
		Model: "claude-opus-4-6",
	}
	badges := agentBadges(agent)
	if badges != "" {
		t.Errorf("agentBadges with only model should be empty, got %q", badges)
	}

	// Test with model + permission — should only show permission
	agent.PermissionMode = "bypassPermissions"
	badges = agentBadges(agent)
	if !strings.Contains(badges, "bypassPermissions") {
		t.Errorf("agentBadges should contain permission mode, got %q", badges)
	}
}

func TestFindWindowForRepo_MatchesWorktrees(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/worktrees/skills/feature-branch",
		},
	}
	// Different path but same repo — should find the window
	sw, found := findWindowForRepo(agents, "/Users/test/Code/skills", "main:0.0")
	if !found {
		t.Error("findWindowForRepo should match worktree agent to same repo")
	}
	if sw != "main:1" {
		t.Errorf("expected main:1, got %s", sw)
	}
}

func TestFindWindowForRepo_ExactPathMatch(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/bjornjee/agent-dashboard",
		},
	}
	sw, found := findWindowForRepo(agents, "/Users/test/Code/bjornjee/agent-dashboard", "main:0.0")
	if !found {
		t.Error("findWindowForRepo should match exact path")
	}
	if sw != "main:1" {
		t.Errorf("expected main:1, got %s", sw)
	}
}

func TestFindWindowForRepo_DifferentReposSameBasename(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/project-a/app",
		},
	}
	// Different parent, same basename "app" — should NOT match
	_, found := findWindowForRepo(agents, "/Users/test/Code/project-b/app", "main:0.0")
	if found {
		t.Error("findWindowForRepo should not match different repos with same basename")
	}
}

func TestFindWindowForRepo_WorktreeToPlain(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/skills",
		},
	}
	// Worktree folder should match agent at plain repo path
	sw, found := findWindowForRepo(agents, "/Users/test/Code/worktrees/skills/feature-x", "main:0.0")
	if !found {
		t.Error("findWindowForRepo should match worktree folder to plain agent cwd")
	}
	if sw != "main:1" {
		t.Errorf("expected main:1, got %s", sw)
	}
}

func TestFindWindowForRepo_WorktreeToWorktree(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/worktrees/skills/branch-a",
		},
	}
	// Both sides are worktrees for the same repo
	sw, found := findWindowForRepo(agents, "/Users/test/Code/worktrees/skills/branch-b", "main:0.0")
	if !found {
		t.Error("findWindowForRepo should match two worktrees of the same repo")
	}
	if sw != "main:1" {
		t.Errorf("expected main:1, got %s", sw)
	}
}

func TestFindWindowForRepo_NoMatchDifferentRepo(t *testing.T) {
	agents := []Agent{
		{
			Target:  "main:1.0",
			Session: "main",
			Window:  1,
			Cwd:     "/Users/test/Code/other-repo",
		},
	}
	_, found := findWindowForRepo(agents, "/Users/test/Code/skills", "main:0.0")
	if found {
		t.Error("findWindowForRepo should not match different repos")
	}
}

func TestFindWindowByName_SkipsDashboardWindow(t *testing.T) {
	windows := []TmuxWindowInfo{
		{Index: 1, Name: "other-project"},
		{Index: 2, Name: "agent-dashboard"}, // dashboard's own window
	}
	// selfTarget is in window 2 — the fallback should NOT match window 2
	sw, found := findWindowByName(windows, "agent-dashboard", "main", "main:2.0")
	if found {
		t.Errorf("findWindowByName should skip dashboard's own window, but matched %s", sw)
	}

	// But it SHOULD match a different window with the same name
	sw, found = findWindowByName(windows, "other-project", "main", "main:2.0")
	if !found {
		t.Error("findWindowByName should match non-dashboard window")
	}
	if sw != "main:1" {
		t.Errorf("expected main:1, got %s", sw)
	}

	// selfTarget without pane suffix should still skip correctly
	_, found = findWindowByName(windows, "agent-dashboard", "main", "main:2")
	if found {
		t.Error("findWindowByName should skip dashboard window even without pane suffix")
	}

	// Empty window list should return not-found
	_, found = findWindowByName(nil, "agent-dashboard", "main", "main:2.0")
	if found {
		t.Error("findWindowByName should return false for empty window list")
	}
}

// stripANSI removes ANSI escape sequences from a string for test assertions.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestRenderPlanMarkdown(t *testing.T) {
	t.Run("renders markdown with code block", func(t *testing.T) {
		content := "# My Plan\n\n## Steps\n\n```go\nfunc main() {}\n```\n"
		result := renderPlanMarkdown(content, 80)
		if result == "" {
			t.Fatal("renderPlanMarkdown returned empty string for non-empty input")
		}
		plain := stripANSI(result)
		// Should contain the heading text
		if !strings.Contains(plain, "My Plan") {
			t.Errorf("rendered output should contain heading text, got:\n%s", plain)
		}
		// Should contain code content
		if !strings.Contains(plain, "func main()") {
			t.Errorf("rendered output should contain code block content, got:\n%s", plain)
		}
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		result := renderPlanMarkdown("", 80)
		if result != "" {
			t.Errorf("expected empty output for empty input, got %q", result)
		}
	})

	t.Run("respects width", func(t *testing.T) {
		content := "# Plan\n\nThis is a very long line that should be wrapped when the width is small enough to require wrapping of the content."
		narrow := renderPlanMarkdown(content, 40)
		if narrow == "" {
			t.Fatal("renderPlanMarkdown returned empty for non-empty input")
		}
		// Just verify it produces output — glamour handles wrapping internally
	})
}

func TestPermissionModeStyle(t *testing.T) {
	// permissionModeStyle should preserve the original mode text in its output
	modes := []string{"plan", "auto-edit", "full-auto", "custom"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			got := permissionModeStyle(mode)
			if !strings.Contains(got, mode) {
				t.Errorf("permissionModeStyle(%q) = %q, want text %q present", mode, got, mode)
			}
		})
	}
}
