package tui

import (
	"context"
	"io"
	"os/exec"
)

// GitRunner abstracts command execution so tests can swap in a mock.
// All external command calls in the tui package go through this interface.
type GitRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	CombinedOutputDir(ctx context.Context, dir, name string, args ...string) ([]byte, error)
	RunDir(ctx context.Context, dir, name string, args ...string) error
	SilentRun(ctx context.Context, name string, args ...string) error
	Start(name string, args ...string) error
}

// execGitRunner is the production GitRunner that shells out via os/exec.
type execGitRunner struct{}

func (r *execGitRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func (r *execGitRunner) CombinedOutputDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func (r *execGitRunner) RunDir(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Run()
}

func (r *execGitRunner) SilentRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (r *execGitRunner) Start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

// gitRunner is the package-level runner used by all tui command functions.
// Tests replace this with a mock.
var gitRunner GitRunner = &execGitRunner{}
