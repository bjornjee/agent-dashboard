package tui

import (
	"bytes"
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

type diffMsg struct {
	files []*gitdiff.File
	err   error
}

// findMergeBase returns the merge-base commit between `ref` and main/master,
// or `ref` as a fallback. Pass "HEAD" for the legacy behaviour.
// Prefers origin/ refs to avoid stale local branch refs.
func findMergeBase(dir, ref string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		out, err := gitRunner.Output(ctx, "git", "-C", dir, "merge-base", ref, base)
		if err == nil {
			if s := bytes.TrimSpace(out); len(s) > 0 {
				return string(s)
			}
		}
	}
	return ref
}

// branchExists reports whether `name` resolves to a commit in dir's repo.
// Used to validate a recovered WorkBranch before diffing against it.
func branchExists(dir, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := gitRunner.Output(ctx, "git", "-C", dir, "rev-parse", "--verify", name+"^{commit}")
	return err == nil
}

// loadDiffWithRef runs git diff against the merge-base of `workBranch` and
// origin/main, falling back to working-tree diff against HEAD's merge-base
// when workBranch is empty or doesn't exist locally. Always appends untracked
// files from the working tree so new files appear in the diff viewer.
//
// projDir + sessionID are accepted but unused at present; reserved for
// callers that already plumb them and may want to lazy-resolve workBranch.
func loadDiffWithRef(ctx context.Context, dir, workBranch, projDir, sessionID string) ([]*gitdiff.File, error) {
	_, _ = projDir, sessionID
	ref := "HEAD"
	if workBranch != "" && branchExists(dir, workBranch) {
		ref = workBranch
	}
	base := findMergeBase(dir, ref)

	// HEAD path: diff working tree against base (includes uncommitted changes).
	// Branch path: diff base..ref (commits the agent landed on its branch,
	// even if working tree has since switched off it).
	diffTarget := base
	if ref != "HEAD" {
		diffTarget = base + ".." + ref
	}
	out, err := gitRunner.Output(ctx, "git", "-C", dir, "diff", diffTarget)
	if err != nil {
		return nil, err
	}

	// Also collect untracked files so new files show in the diff.
	untrackedOut, err := gitRunner.Output(ctx, "git", "-C", dir,
		"ls-files", "--others", "--exclude-standard")
	if err == nil && len(bytes.TrimSpace(untrackedOut)) > 0 {
		// Generate a unified diff for each untracked file via git diff --no-index.
		for _, name := range strings.Split(strings.TrimSpace(string(untrackedOut)), "\n") {
			if name == "" {
				continue
			}
			// git diff --no-index /dev/null <file> exits 1 when there are diffs,
			// so we ignore the exit code and just use stdout.
			patch, _ := gitRunner.Output(ctx, "git", "-C", dir,
				"diff", "--no-index", "--", "/dev/null", name)
			if len(patch) > 0 {
				out = append(out, patch...)
			}
		}
	}

	files, _, err := gitdiff.Parse(bytes.NewReader(out))
	return files, err
}

// loadDiff is the HEAD-only convenience wrapper kept for test compatibility.
func loadDiff(ctx context.Context, dir string) ([]*gitdiff.File, error) {
	return loadDiffWithRef(ctx, dir, "", "", "")
}

func loadDiffCmd(agent domain.Agent) tea.Cmd {
	dir := agent.EffectiveDir()
	projDir := agent.ProjDir
	sessionID := agent.SessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		workBranch := conversation.LastGitBranch(projDir, sessionID)
		files, err := loadDiffWithRef(ctx, dir, workBranch, projDir, sessionID)
		return diffMsg{files: files, err: err}
	}
}
