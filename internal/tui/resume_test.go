package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// resumeSpawnRunner records tmux Output calls so the test can assert the spawn
// command, and returns a valid new-window target.
type resumeSpawnRunner struct{ outputs [][]string }

func (r *resumeSpawnRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string(nil), args...))
	switch args[0] {
	case "display-message": // ResolveTarget(self) — no self pane in the test
		return nil, fmt.Errorf("no self pane")
	case "new-window":
		return []byte("%99\tmain:3.0\n"), nil
	}
	return []byte(""), nil
}
func (r *resumeSpawnRunner) Run(_ context.Context, _ ...string) error { return nil }

// resumeSession re-spawns an orphan with `--resume <sid>` in its stored dir and
// removes the stale orphan state file on success.
func TestResumeSession(t *testing.T) {
	stateDir := t.TempDir()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	orphan := domain.Agent{SessionID: "orphan-sid", Harness: "claude", State: "running", Cwd: folder, TmuxPaneID: "%9"}
	data, _ := json.Marshal(orphan)
	if err := os.WriteFile(filepath.Join(agentsDir, "orphan-sid.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	runner := &resumeSpawnRunner{}
	t.Cleanup(tmux.SetTestRunner(runner))

	cfg := testConfig(stateDir)
	cmd := resumeSession(orphan, nil, "", cfg.Profile, cfg.Settings)
	if cmd == nil {
		t.Fatal("expected a resume command")
	}
	msg := cmd()
	res, ok := msg.(createSessionMsg)
	if !ok {
		t.Fatalf("expected createSessionMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("unexpected resume error: %v", res.err)
	}
	if res.target != "main:3.0" {
		t.Errorf("target = %q, want main:3.0", res.target)
	}

	var sawResume bool
	for _, out := range runner.outputs {
		if out[0] == "new-window" && strings.Contains(out[len(out)-1], "--resume 'orphan-sid'") {
			sawResume = true
		}
	}
	if !sawResume {
		t.Errorf("new-window command should carry --resume 'orphan-sid'; got %v", runner.outputs)
	}

	if _, err := os.Stat(filepath.Join(agentsDir, "orphan-sid.json")); !os.IsNotExist(err) {
		t.Error("stale orphan state file should be removed after resume")
	}
}

// resumeSession is harness-agnostic: a codex orphan re-spawns via
// `codex resume <sid>` (no --resume flag).
func TestResumeSessionCodex(t *testing.T) {
	stateDir := t.TempDir()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	orphan := domain.Agent{SessionID: "orphan-cx", Harness: "codex", State: "running", Cwd: folder, TmuxPaneID: "%9"}
	data, _ := json.Marshal(orphan)
	if err := os.WriteFile(filepath.Join(agentsDir, "orphan-cx.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	runner := &resumeSpawnRunner{}
	t.Cleanup(tmux.SetTestRunner(runner))

	cfg := testConfig(stateDir)
	cmd := resumeSession(orphan, nil, "", cfg.Profile, cfg.Settings)
	if cmd == nil {
		t.Fatal("expected a resume command")
	}
	res, ok := cmd().(createSessionMsg)
	if !ok || res.err != nil {
		t.Fatalf("resume failed: %+v", res)
	}

	var sawResume bool
	for _, out := range runner.outputs {
		if out[0] == "new-window" && strings.Contains(out[len(out)-1], "codex resume 'orphan-cx'") {
			sawResume = true
		}
	}
	if !sawResume {
		t.Errorf("new-window command should carry `codex resume 'orphan-cx'`; got %v", runner.outputs)
	}
}
