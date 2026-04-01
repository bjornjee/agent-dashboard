package main

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type diffMsg struct {
	files []*gitdiff.File
	err   error
}

func loadDiffCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff").Output()
		if err != nil {
			return diffMsg{err: err}
		}
		files, _, err := gitdiff.Parse(bytes.NewReader(out))
		return diffMsg{files: files, err: err}
	}
}
