package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjornjee/agent-dashboard/internal/mocks"
	"github.com/stretchr/testify/mock"
)

func setMockGitRunner(t *testing.T) *mocks.MockGitRunner {
	t.Helper()
	gr := mocks.NewMockGitRunner(t)
	orig := gitRunner
	gitRunner = gr
	t.Cleanup(func() { gitRunner = orig })
	return gr
}

func mockAllDepsOK(gr *mocks.MockGitRunner) {
	gr.On("SilentRun", mock.Anything, "gh", "auth", "status").Return(nil)
	gr.On("SilentRun", mock.Anything, "tmux", "-V").Return(nil)
	gr.On("SilentRun", mock.Anything, "git", "--version").Return(nil)
	gr.On("SilentRun", mock.Anything, "codex", "--version").Return(nil)
}

func findDep(deps []depStatus, name string) *depStatus {
	for i := range deps {
		if deps[i].name == name {
			return &deps[i]
		}
	}
	return nil
}

func TestCheckDeps_AllAvailable(t *testing.T) {
	gr := setMockGitRunner(t)
	mockAllDepsOK(gr)

	deps := checkDeps()
	if len(deps) != 4 {
		t.Fatalf("expected 4 deps (gh, tmux, git, codex), got %d", len(deps))
	}
	for _, d := range deps {
		if !d.ok {
			t.Errorf("expected %s ok=true, got false (hint=%q)", d.name, d.hint)
		}
		if d.purpose == "" {
			t.Errorf("expected %s to have a non-empty purpose", d.name)
		}
	}
}

func TestCheckDeps_GHMissing(t *testing.T) {
	gr := setMockGitRunner(t)
	gr.On("SilentRun", mock.Anything, "gh", "auth", "status").Return(fmt.Errorf("not found"))
	gr.On("SilentRun", mock.Anything, "tmux", "-V").Return(nil)
	gr.On("SilentRun", mock.Anything, "git", "--version").Return(nil)
	gr.On("SilentRun", mock.Anything, "codex", "--version").Return(nil)

	deps := checkDeps()
	gh := findDep(deps, "gh")
	if gh == nil {
		t.Fatal("missing gh dep entry")
	}
	if gh.ok {
		t.Error("expected gh.ok=false")
	}
	if gh.hint == "" {
		t.Error("expected non-empty hint for missing gh")
	}
	if tm := findDep(deps, "tmux"); tm == nil || !tm.ok {
		t.Errorf("expected tmux ok=true")
	}
}

func TestCheckDeps_TmuxMissing(t *testing.T) {
	gr := setMockGitRunner(t)
	gr.On("SilentRun", mock.Anything, "gh", "auth", "status").Return(nil)
	gr.On("SilentRun", mock.Anything, "tmux", "-V").Return(fmt.Errorf("not found"))
	gr.On("SilentRun", mock.Anything, "git", "--version").Return(nil)
	gr.On("SilentRun", mock.Anything, "codex", "--version").Return(nil)

	deps := checkDeps()
	tm := findDep(deps, "tmux")
	if tm == nil || tm.ok || tm.hint == "" {
		t.Errorf("expected tmux ok=false with hint, got %+v", tm)
	}
}

func TestCheckDeps_GitMissing(t *testing.T) {
	gr := setMockGitRunner(t)
	gr.On("SilentRun", mock.Anything, "gh", "auth", "status").Return(nil)
	gr.On("SilentRun", mock.Anything, "tmux", "-V").Return(nil)
	gr.On("SilentRun", mock.Anything, "git", "--version").Return(fmt.Errorf("not found"))
	gr.On("SilentRun", mock.Anything, "codex", "--version").Return(nil)

	deps := checkDeps()
	g := findDep(deps, "git")
	if g == nil || g.ok || g.hint == "" {
		t.Errorf("expected git ok=false with hint, got %+v", g)
	}
}

func TestCheckDeps_CodexMissing(t *testing.T) {
	gr := setMockGitRunner(t)
	gr.On("SilentRun", mock.Anything, "gh", "auth", "status").Return(nil)
	gr.On("SilentRun", mock.Anything, "tmux", "-V").Return(nil)
	gr.On("SilentRun", mock.Anything, "git", "--version").Return(nil)
	gr.On("SilentRun", mock.Anything, "codex", "--version").Return(fmt.Errorf("not found"))

	deps := checkDeps()
	c := findDep(deps, "codex")
	if c == nil || c.ok || c.hint == "" {
		t.Errorf("expected codex ok=false with hint, got %+v", c)
	}
}

func TestRenderDepsView_AllOK(t *testing.T) {
	deps := []depStatus{
		{name: "gh", purpose: "PR create, merge, auth", ok: true},
		{name: "tmux", purpose: "Pane control", ok: true},
		{name: "git", purpose: "Repository state", ok: true},
		{name: "codex", purpose: "Codex delegation", ok: true},
	}
	out := renderDepsView(deps, 80, 24)
	for _, name := range []string{"gh", "tmux", "git", "codex"} {
		if !strings.Contains(out, name) {
			t.Errorf("render missing dep name %q", name)
		}
	}
	if !strings.Contains(out, "PR create") {
		t.Errorf("expected purpose to appear in render, got:\n%s", out)
	}
	if !strings.Contains(out, "Dependency Status") {
		t.Errorf("expected descriptive title 'Dependency Status', got:\n%s", out)
	}
	if !strings.Contains(out, "4 of 4") && !strings.Contains(out, "All 4") {
		t.Errorf("expected an availability summary in render, got:\n%s", out)
	}
}

func TestRenderDepsView_WithMissing(t *testing.T) {
	deps := []depStatus{
		{name: "gh", purpose: "PR create, merge, auth", ok: false, hint: "run 'gh auth login'"},
		{name: "tmux", purpose: "Pane control", ok: true},
		{name: "git", purpose: "Repository state", ok: true},
		{name: "codex", purpose: "Codex delegation", ok: true},
	}
	out := renderDepsView(deps, 80, 24)
	if !strings.Contains(out, "run 'gh auth login'") {
		t.Errorf("expected hint to appear in render, got:\n%s", out)
	}
	if !strings.Contains(out, "3 of 4") {
		t.Errorf("expected '3 of 4' summary in render, got:\n%s", out)
	}
}

func TestSKey_OpensDepsView(t *testing.T) {
	gr := setMockGitRunner(t)
	mockAllDepsOK(gr)

	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.mode = modeNormal

	// Press s — should enter modeDepsStatus and return a probe cmd
	// (async so the TUI goroutine doesn't block on up to 4×3s subprocess calls).
	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	updated := result.(model)
	if updated.mode != modeDepsStatus {
		t.Errorf("expected modeDepsStatus, got %d", updated.mode)
	}
	if cmd == nil {
		t.Fatal("expected probe cmd from 's' key")
	}

	// Executing the cmd runs the probes and yields a depsReadyMsg.
	msg := cmd()
	ready, ok := msg.(depsReadyMsg)
	if !ok {
		t.Fatalf("expected depsReadyMsg, got %T", msg)
	}
	if len(ready.deps) != 4 {
		t.Errorf("expected 4 deps in msg, got %d", len(ready.deps))
	}
}

func TestDepsReadyMsg_PopulatesModelDeps(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeDepsStatus

	msg := depsReadyMsg{deps: []depStatus{{name: "gh", ok: true}, {name: "tmux", ok: false, hint: "install"}}}
	result, _ := m.Update(msg)
	updated := result.(model)
	if len(updated.deps) != 2 {
		t.Fatalf("expected 2 deps populated, got %d", len(updated.deps))
	}
	if updated.deps[0].name != "gh" || !updated.deps[0].ok {
		t.Errorf("expected gh ok=true at index 0, got %+v", updated.deps[0])
	}
}

func TestDepsView_EscReturnsToNormal(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeDepsStatus

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", updated.mode)
	}
}

func TestDepsView_QReturnsToNormal(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeDepsStatus

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after q, got %d", updated.mode)
	}
}

func TestDepsView_RRefreshes(t *testing.T) {
	gr := setMockGitRunner(t)
	mockAllDepsOK(gr)

	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeDepsStatus
	m.deps = []depStatus{{name: "stale", ok: false}}

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	updated := result.(model)
	if updated.mode != modeDepsStatus {
		t.Errorf("expected to stay in modeDepsStatus, got %d", updated.mode)
	}
	if cmd == nil {
		t.Fatal("expected refresh cmd from 'r'")
	}

	// Executing the cmd dispatches a depsReadyMsg with fresh probe results.
	msg := cmd()
	ready, ok := msg.(depsReadyMsg)
	if !ok {
		t.Fatalf("expected depsReadyMsg, got %T", msg)
	}
	if len(ready.deps) != 4 {
		t.Errorf("expected 4 deps in refresh msg, got %d", len(ready.deps))
	}
	if findDep(ready.deps, "stale") != nil {
		t.Error("stale deps must not appear in fresh probe results")
	}
}
