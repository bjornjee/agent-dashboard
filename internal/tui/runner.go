package tui

import (
	"context"
	"os/exec"
)

// GitRunner abstracts command execution so tests can swap in a mock.
type GitRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execGitRunner is the production GitRunner that shells out via os/exec.
type execGitRunner struct{}

func (r *execGitRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// gitRunner is the package-level runner used by diff functions.
// Tests replace this with a mock.
var gitRunner GitRunner = &execGitRunner{}
