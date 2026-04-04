package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type diffMsg struct {
	files []*gitdiff.File
	err   error
}

// findMergeBase returns the merge-base commit between HEAD and main/master,
// or "HEAD" as a fallback (which shows only uncommitted changes).
// Prefers origin/ refs to avoid stale local branch refs.
func findMergeBase(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, base := range []string{"origin/main", "origin/master", "main", "master"} {
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "merge-base", "HEAD", base).Output()
		if err == nil {
			if s := bytes.TrimSpace(out); len(s) > 0 {
				return string(s)
			}
		}
	}
	return "HEAD"
}

// loadDiff runs git diff against the merge-base and also includes untracked
// files so that newly created files appear in the diff viewer.
func loadDiff(ctx context.Context, dir string) ([]*gitdiff.File, error) {
	base := findMergeBase(dir)
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", base).Output()
	if err != nil {
		return nil, err
	}

	// Also collect untracked files so new files show in the diff.
	untrackedOut, err := exec.CommandContext(ctx, "git", "-C", dir,
		"ls-files", "--others", "--exclude-standard").Output()
	if err == nil && len(bytes.TrimSpace(untrackedOut)) > 0 {
		// Generate a unified diff for each untracked file via git diff --no-index.
		for _, name := range strings.Split(strings.TrimSpace(string(untrackedOut)), "\n") {
			if name == "" {
				continue
			}
			// git diff --no-index /dev/null <file> exits 1 when there are diffs,
			// so we ignore the exit code and just use stdout.
			patch, _ := exec.CommandContext(ctx, "git", "-C", dir,
				"diff", "--no-index", "--", "/dev/null", name).Output()
			if len(patch) > 0 {
				out = append(out, patch...)
			}
		}
	}

	files, _, err := gitdiff.Parse(bytes.NewReader(out))
	return files, err
}

func loadDiffCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		files, err := loadDiff(ctx, dir)
		return diffMsg{files: files, err: err}
	}
}
