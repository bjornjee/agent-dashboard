// Package repowin provides shared logic for matching repos to tmux windows.
// Used by both the TUI and web server when creating agent sessions.
package repowin

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// MaxPanesPerWindow is the upper limit of panes allowed in a single tmux window.
const MaxPanesPerWindow = 8

// safeWindowNameRe matches characters safe for tmux window names.
var safeWindowNameRe = regexp.MustCompile(`[^a-zA-Z0-9_.\-]`)

// RepoFromCwd extracts the repo name from a working directory path.
// For worktree paths like /foo/worktrees/skills/branch-name, returns "skills".
// For normal paths like /foo/skills, returns "skills" (filepath.Base).
func RepoFromCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	parts := strings.SplitN(cwd, "/worktrees/", 2)
	if len(parts) == 2 && parts[1] != "" {
		repo := strings.SplitN(parts[1], "/", 2)[0]
		return repo
	}
	base := filepath.Base(cwd)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

// SanitizeWindowName strips unsafe characters from a tmux window name.
// Characters like : would break target parsing (session:window.pane).
func SanitizeWindowName(name string) string {
	safe := safeWindowNameRe.ReplaceAllString(name, "_")
	if safe == "" {
		return "claude"
	}
	return safe
}

// FindWindowForRepo searches agents for an existing tmux window running in the
// same repo as folder. Two-pass: exact path match, then worktree-aware repo
// name match (only when at least one side is a worktree, to avoid false matches
// between unrelated repos that share the same basename).
// selfPaneID is the dashboard's own pane ID (%N) to exclude; pass "" from web.
func FindWindowForRepo(agents []domain.Agent, folder, selfPaneID string) (string, bool) {
	// Pass 1: exact path match
	for _, agent := range agents {
		if selfPaneID != "" && agent.TmuxPaneID == selfPaneID {
			continue
		}
		if agent.Cwd == folder {
			return fmt.Sprintf("%s:%d", agent.Session, agent.Window), true
		}
	}

	// Pass 2: repo-name match, only when at least one side is a worktree
	folderRepo := RepoFromCwd(folder)
	if folderRepo == "" {
		return "", false
	}
	folderIsWorktree := strings.Contains(folder, "/worktrees/")
	for _, agent := range agents {
		if selfPaneID != "" && agent.TmuxPaneID == selfPaneID {
			continue
		}
		agentIsWorktree := strings.Contains(agent.Cwd, "/worktrees/") || agent.WorktreeCwd != ""
		if !folderIsWorktree && !agentIsWorktree {
			continue
		}
		if RepoFromCwd(agent.Cwd) == folderRepo {
			return fmt.Sprintf("%s:%d", agent.Session, agent.Window), true
		}
	}
	return "", false
}

// FindWindowByName returns the first window whose Name equals repoName,
// excluding the window identified by excludeSW (e.g. "main:2").
func FindWindowByName(windows []domain.TmuxWindowInfo, repoName, session, excludeSW string) (string, bool) {
	for _, w := range windows {
		candidate := fmt.Sprintf("%s:%d", session, w.Index)
		if w.Name == repoName && candidate != excludeSW {
			return candidate, true
		}
	}
	return "", false
}
