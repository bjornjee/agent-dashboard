// Package git is a thin layer over `git` subprocess invocations used by the
// dashboard for worktree discovery. The Runner interface mirrors
// state.BranchRunner so callers can pass either implementation; declaring it
// here avoids an import cycle with internal/state.
package git

import (
	"bufio"
	"bytes"
	"context"
	"strings"
)

// Runner abstracts `git` subprocess execution so tests can swap in a mock.
// Shape matches state.BranchRunner — duck-typed.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Worktree is one entry from `git worktree list --porcelain`, enriched with
// the per-worktree git-dir resolved via `git rev-parse --absolute-git-dir`.
// Branch is empty when the worktree is on a detached HEAD.
type Worktree struct {
	Path   string
	Branch string
	GitDir string
}

// ListWorktrees runs `git -C <dir> worktree list --porcelain`, parses the
// line-oriented blocks, then resolves each worktree's git-dir via a
// per-worktree `rev-parse --absolute-git-dir`. A per-worktree git-dir
// resolution failure leaves that entry's GitDir empty rather than failing
// the whole call — the caller can decide whether to skip it.
func ListWorktrees(ctx context.Context, runner Runner, dir string) ([]Worktree, error) {
	out, err := runner.Output(ctx, "git", "-C", dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	wts := parsePorcelain(out)

	for i := range wts {
		gd, err := runner.Output(ctx, "git", "-C", wts[i].Path, "rev-parse", "--absolute-git-dir")
		if err != nil {
			continue
		}
		wts[i].GitDir = strings.TrimSpace(string(gd))
	}

	return wts, nil
}

// parsePorcelain consumes blocks of the form:
//
//	worktree <path>
//	HEAD <sha>
//	branch refs/heads/<name>   (or `detached`, plus optional `locked` / `prunable`)
//
// Blocks are separated by a blank line. The `branch` line's
// `refs/heads/` prefix is stripped; other refs are kept verbatim so callers
// can see e.g. `refs/tags/v1` without misinterpreting it as a branch.
func parsePorcelain(data []byte) []Worktree {
	var out []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" {
			out = append(out, cur)
		}
		cur = Worktree{}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			// `branch refs/heads/<name>` is the common case; refs outside
			// refs/heads/ (e.g. refs/tags/v1) are kept verbatim so callers can
			// distinguish them from a real branch name.
			ref := strings.TrimPrefix(line, "branch ")
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
		// HEAD / detached / locked / prunable lines are intentionally ignored —
		// branch already captures the ref state we care about, and the others
		// are diagnostic flags the reconciler doesn't act on.
	}
	flush()
	return out
}

// IsBranchMerged reports whether branch is an ancestor of base in the repo at
// dir — i.e. the branch's commits are already reachable from base (true merge
// or fast-forward). Squash merges are invisible to ancestry and are covered
// by the resume TTL instead. Callers must not pass the repo's default branch
// as branch: a ref is always an ancestor of itself. The "--" terminator keeps
// an option-looking branch value from a state file (e.g. "--version") from
// being parsed as a git flag and faking a merged verdict.
func IsBranchMerged(ctx context.Context, runner Runner, dir, branch, base string) bool {
	_, err := runner.Output(ctx, "git", "-C", dir, "merge-base", "--is-ancestor", "--", branch, base)
	return err == nil
}
