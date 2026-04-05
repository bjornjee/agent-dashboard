package tui

import (
	"context"
	"os"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// noopTmuxRunner is a stub Runner that returns empty/nil for all calls,
// preventing any real tmux subprocess from being spawned during tests.
type noopTmuxRunner struct{}

func (r *noopTmuxRunner) Output(_ context.Context, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func (r *noopTmuxRunner) Run(_ context.Context, _ ...string) error {
	return nil
}

// noopBranchRunner is a stub BranchRunner that returns empty/nil for all calls.
type noopBranchRunner struct{}

func (r *noopBranchRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func TestMain(m *testing.M) {
	restoreTmux := tmux.SetTestRunner(&noopTmuxRunner{})
	restoreState := state.SetTestRunner(&noopBranchRunner{})
	restoreGit := setTestGitRunner(&noopGitRunner{})
	code := m.Run()
	restoreGit()
	restoreState()
	restoreTmux()
	os.Exit(code)
}

// noopGitRunner is a stub GitRunner that returns empty/nil for all calls.
type noopGitRunner struct{}

func (r *noopGitRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func (r *noopGitRunner) CombinedOutputDir(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func (r *noopGitRunner) RunDir(_ context.Context, _, _ string, _ ...string) error {
	return nil
}

func (r *noopGitRunner) SilentRun(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (r *noopGitRunner) Start(_ string, _ ...string) error {
	return nil
}
