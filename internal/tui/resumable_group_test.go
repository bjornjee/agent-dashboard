package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// Restart-survivors group under RESUMABLE (priority 7), never under the group
// of their stale state field.
func TestAgentGroupResumable(t *testing.T) {
	running := domain.Agent{State: "running"}
	if got := agentGroup(running); got != 3 {
		t.Fatalf("running group = %d, want 3", got)
	}
	running.Resumable = true
	if got := agentGroup(running); got != domain.ResumablePriority {
		t.Errorf("resumable group = %d, want %d", got, domain.ResumablePriority)
	}
}

// The tree renders survivors under a RESUMABLE header at the bottom, after
// live groups, even when their stale state says "running".
func TestBuildTreeResumableGroupLast(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", SessionID: "live-1", State: "running", TmuxPaneID: "%1"},
		{Target: "main:2.0", SessionID: "orph-1", State: "running", TmuxPaneID: "%9", Branch: "feat/x", Resumable: true},
	}
	m.buildTree()

	var groups []int
	for _, n := range m.treeNodes {
		if n.GroupHeader > 0 {
			groups = append(groups, n.GroupHeader)
		}
	}
	if len(groups) != 2 || groups[0] != 3 || groups[1] != domain.ResumablePriority {
		t.Errorf("group headers = %v, want [3 %d]", groups, domain.ResumablePriority)
	}
}

// x on a resumable row confirms with dismiss wording, not "close pane" — the
// pane is already dead.
func TestDismissResumableWording(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:2.0", SessionID: "orph-1", State: "running", TmuxPaneID: "%9", Cwd: "/repo/beta", Branch: "feat/x", Resumable: true},
	}
	m.buildTree()
	selectFirstAgent(&m)

	res, _ := m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m2 := res.(model)
	if m2.mode != modeConfirmClose {
		t.Fatalf("x on orphan should enter confirm mode, got %d", m2.mode)
	}
	if !strings.Contains(m2.statusMsg, "Dismiss resumable session") {
		t.Errorf("confirm prompt = %q, want dismiss wording", m2.statusMsg)
	}
}

// x on the RESUMABLE group header dismisses every survivor's state file after
// a y confirmation.
func TestDismissAllResumableFromHeader(t *testing.T) {
	stateDir := t.TempDir()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	orphans := []domain.Agent{
		{Target: "main:2.0", SessionID: "orph-1", State: "running", TmuxPaneID: "%8", Branch: "feat/x", Resumable: true},
		{Target: "main:3.0", SessionID: "orph-2", State: "idle_prompt", TmuxPaneID: "%9", Branch: "feat/y", Resumable: true},
	}
	for _, o := range orphans {
		data, _ := json.Marshal(o)
		if err := os.WriteFile(filepath.Join(agentsDir, o.SessionID+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	m := NewModel(testConfig(stateDir), nil)
	m.tmuxAvailable = true
	m.agents = orphans
	m.buildTree()
	m.selected = 0 // the RESUMABLE group header node

	if m.selectedGroupHeader() != domain.ResumablePriority {
		t.Fatalf("expected RESUMABLE header selected, got %d", m.selectedGroupHeader())
	}

	res, _ := m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m2 := res.(model)
	if m2.mode != modeConfirmDismissAll {
		t.Fatalf("x on RESUMABLE header should enter dismiss-all confirm, got mode %d", m2.mode)
	}
	if !strings.Contains(m2.statusMsg, "Dismiss all 2 resumable sessions") {
		t.Errorf("confirm prompt = %q, want dismiss-all wording with count", m2.statusMsg)
	}

	m2.confirmEnteredAt = time.Now().Add(-time.Second) // skip the y cooldown
	res2, cmd := m2.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if res2.(model).mode != modeNormal {
		t.Fatalf("y should leave confirm mode, got %d", res2.(model).mode)
	}
	if cmd == nil {
		t.Fatal("y should dispatch the dismiss-all command")
	}
	if msg, ok := cmd().(closeResultMsg); !ok || msg.err != nil {
		t.Fatalf("dismiss-all failed: %+v", msg)
	}

	for _, o := range orphans {
		if _, err := os.Stat(filepath.Join(agentsDir, o.SessionID+".json")); !os.IsNotExist(err) {
			t.Errorf("state file for %s should be removed", o.SessionID)
		}
	}
}

// paneNoPIDRunner simulates malformed list-panes output: a pane line without
// the #{pid} column, yielding a non-empty pane set but an unknown server PID.
type paneNoPIDRunner struct{}

func (paneNoPIDRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		return []byte("%1\n"), nil
	}
	return []byte(""), nil
}
func (paneNoPIDRunner) Run(_ context.Context, _ ...string) error { return nil }

// An unknown server PID must skip the prune cycle entirely — otherwise every
// survivor fails IsResumableOrphan's fence and gets deleted as dead.
func TestPruneDeadSkipsWhenServerPIDUnknown(t *testing.T) {
	stateDir := t.TempDir()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	survivor := domain.Agent{SessionID: "surv", State: "running", TmuxPaneID: "%9", TmuxServerPID: "99", Branch: "feat/x", Cwd: t.TempDir()}
	data, _ := json.Marshal(survivor)
	if err := os.WriteFile(filepath.Join(agentsDir, "surv.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(tmux.SetTestRunner(paneNoPIDRunner{}))

	msg := pruneDead(stateDir)()
	if got := msg.(pruneDeadMsg).removed; got != 0 {
		t.Errorf("prune with unknown server PID removed %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "surv.json")); err != nil {
		t.Error("survivor must not be pruned when the server PID is unknown")
	}
}
