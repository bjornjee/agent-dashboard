package web

import (
	"context"
	"os"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// noopTmuxRunner prevents any real tmux subprocess from being spawned during tests.
type noopTmuxRunner struct{}

func (r *noopTmuxRunner) Output(_ context.Context, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func (r *noopTmuxRunner) Run(_ context.Context, _ ...string) error {
	return nil
}

// noopBranchRunner prevents any real git subprocess from being spawned during tests.
type noopBranchRunner struct{}

func (r *noopBranchRunner) Output(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(""), nil
}

func TestMain(m *testing.M) {
	restoreTmux := tmux.SetTestRunner(&noopTmuxRunner{})
	restoreState := state.SetTestRunner(&noopBranchRunner{})
	code := m.Run()
	restoreState()
	restoreTmux()
	os.Exit(code)
}
