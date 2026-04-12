package repowin

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func TestRepoFromCwd(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"worktree path", "/Users/test/Code/worktrees/skills/dashboard-agent-naming", "skills"},
		{"normal repo path", "/Users/test/Code/skills", "skills"},
		{"worktree with deep path", "/home/user/worktrees/myapp/feature-branch", "myapp"},
		{"empty cwd", "", ""},
		{"root path", "/", ""},
		{"worktrees at end without branch dir", "/home/user/worktrees/repo", "repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RepoFromCwd(tt.cwd)
			if got != tt.want {
				t.Errorf("RepoFromCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
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
			got := SanitizeWindowName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeWindowName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindWindowForRepo_ExactPathMatch(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/agent-dashboard"},
	}
	sw, found := FindWindowForRepo(agents, "/Users/test/Code/agent-dashboard", "%0")
	if !found {
		t.Fatal("expected exact path match")
	}
	if sw != "main:1" {
		t.Errorf("got %s, want main:1", sw)
	}
}

func TestFindWindowForRepo_WorktreeToPlain(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/skills"},
	}
	sw, found := FindWindowForRepo(agents, "/Users/test/Code/worktrees/skills/feature-x", "%0")
	if !found {
		t.Fatal("expected worktree-to-plain match")
	}
	if sw != "main:1" {
		t.Errorf("got %s, want main:1", sw)
	}
}

func TestFindWindowForRepo_PlainToWorktree(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/worktrees/skills/feature-branch"},
	}
	sw, found := FindWindowForRepo(agents, "/Users/test/Code/skills", "%0")
	if !found {
		t.Fatal("expected plain-to-worktree match")
	}
	if sw != "main:1" {
		t.Errorf("got %s, want main:1", sw)
	}
}

func TestFindWindowForRepo_WorktreeToWorktree(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/worktrees/skills/branch-a"},
	}
	sw, found := FindWindowForRepo(agents, "/Users/test/Code/worktrees/skills/branch-b", "%0")
	if !found {
		t.Fatal("expected worktree-to-worktree match")
	}
	if sw != "main:1" {
		t.Errorf("got %s, want main:1", sw)
	}
}

func TestFindWindowForRepo_DifferentRepos(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/other-repo"},
	}
	_, found := FindWindowForRepo(agents, "/Users/test/Code/skills", "%0")
	if found {
		t.Error("should not match different repos")
	}
}

func TestFindWindowForRepo_SameBasenameDifferentParent(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, Cwd: "/Users/test/Code/project-a/app"},
	}
	_, found := FindWindowForRepo(agents, "/Users/test/Code/project-b/app", "%0")
	if found {
		t.Error("should not match different repos with same basename (neither is worktree)")
	}
}

func TestFindWindowForRepo_EmptyAgents(t *testing.T) {
	_, found := FindWindowForRepo(nil, "/Users/test/Code/skills", "%0")
	if found {
		t.Error("should not match with empty agents")
	}
}

func TestFindWindowForRepo_SkipsSelfPane(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, TmuxPaneID: "%5", Cwd: "/Users/test/Code/skills"},
	}
	_, found := FindWindowForRepo(agents, "/Users/test/Code/skills", "%5")
	if found {
		t.Error("should skip self pane")
	}
}

func TestFindWindowForRepo_EmptySelfPaneID(t *testing.T) {
	agents := []domain.Agent{
		{Session: "main", Window: 1, TmuxPaneID: "%5", Cwd: "/Users/test/Code/skills"},
	}
	sw, found := FindWindowForRepo(agents, "/Users/test/Code/skills", "")
	if !found {
		t.Fatal("empty selfPaneID should not exclude any agent")
	}
	if sw != "main:1" {
		t.Errorf("got %s, want main:1", sw)
	}
}

func TestFindWindowByName(t *testing.T) {
	windows := []domain.TmuxWindowInfo{
		{Index: 1, Name: "other-project"},
		{Index: 2, Name: "agent-dashboard"},
	}

	t.Run("matches by name", func(t *testing.T) {
		sw, found := FindWindowByName(windows, "other-project", "main", "main:2")
		if !found {
			t.Fatal("expected match")
		}
		if sw != "main:1" {
			t.Errorf("got %s, want main:1", sw)
		}
	})

	t.Run("skips excluded window", func(t *testing.T) {
		_, found := FindWindowByName(windows, "agent-dashboard", "main", "main:2")
		if found {
			t.Error("should skip excluded window")
		}
	})

	t.Run("empty windows", func(t *testing.T) {
		_, found := FindWindowByName(nil, "skills", "main", "")
		if found {
			t.Error("should not match empty list")
		}
	})
}
