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
// or `ref` as a fallback.
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

// isLocalDefault reports whether name is the conventional default branch.
// Used by resolveDiffRef to recognise a stale-default head value.
func isLocalDefault(name string) bool {
	return name == "main" || name == "master"
}

func currentGitBranch(ctx context.Context, dir string) string {
	out, err := gitRunner.Output(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveDiffRef picks the git ref to diff against base. It combines two
// signals already produced elsewhere in the dashboard:
//
//   - headBranch: the worktree's actual checked-out branch, resolved once
//     by state.ResolveAgentBranches and cached on agent.Branch.
//   - recordedBranch: the most recent gitBranch field in the agent's JSONL
//     (conversation.LastGitBranch). Useful only when HEAD is misleading.
//
// Priority is HEAD-first. Recorded is consulted only when HEAD is the local
// default (b594c2d's pane 5.1 case: HEAD switched off the agent's branch)
// or the worktree is detached. Returns "HEAD" when neither yields a usable
// ref so callers fall back to working-tree-vs-base behaviour.
func resolveDiffRef(headBranch, recordedBranch, dir string) string {
	if headBranch != "" && headBranch != "HEAD" && !isLocalDefault(headBranch) {
		return headBranch
	}
	if recordedBranch != "" && branchExists(dir, recordedBranch) {
		return recordedBranch
	}
	if headBranch != "" && headBranch != "HEAD" {
		return headBranch
	}
	return "HEAD"
}

// loadDiffWithRef runs git diff against the merge-base of `ref` and
// origin/main (falling back through origin/master, main, master). Always
// appends untracked files from the working tree so new files appear in the
// diff viewer.
//
// HEAD path: diff working tree against base (includes uncommitted changes).
// Branch path: diff base..ref (commits the agent landed on its branch,
// even if working tree has since switched off it).
//
// Callers should pre-resolve ref via resolveDiffRef.
func loadDiffWithRef(ctx context.Context, dir, ref string) ([]*gitdiff.File, error) {
	if ref == "" {
		ref = "HEAD"
	}
	base := findMergeBase(dir, ref)
	diffTarget := base
	if ref != "HEAD" && currentGitBranch(ctx, dir) != ref {
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

func loadDiffCmd(agent domain.Agent) tea.Cmd {
	dir := agent.EffectiveDir()
	headBranch := agent.Branch
	projDir := agent.ProjDir
	sessionID := agent.SessionID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		recorded := conversation.LastGitBranch(projDir, sessionID)
		ref := resolveDiffRef(headBranch, recorded, dir)
		files, err := loadDiffWithRef(ctx, dir, ref)
		return diffMsg{files: files, err: err}
	}
}
