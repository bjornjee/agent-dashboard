package main

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	tea "github.com/charmbracelet/bubbletea"
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

func loadDiffCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		base := findMergeBase(dir)
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", base).Output()
		if err != nil {
			return diffMsg{err: err}
		}
		files, _, err := gitdiff.Parse(bytes.NewReader(out))
		return diffMsg{files: files, err: err}
	}
}
