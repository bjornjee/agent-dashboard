package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func newSearchModel(t *testing.T) model {
	t.Helper()
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	// Resumable arrives pre-flagged from resolveAgents (state.FlagResumable);
	// the model consumes the flag, it never re-derives orphan status.
	m.agents = []domain.Agent{
		{Target: "main:1.0", SessionID: "live-1", State: "running", TmuxPaneID: "%1", Cwd: "/repo/alpha", Branch: "main"},
		{Target: "main:2.0", SessionID: "orph-1", State: "running", TmuxPaneID: "%9", Cwd: "/repo/beta", Branch: "feat/login", Resumable: true},
	}
	m.buildTree()
	return m
}

func visibleAgentIDs(m model) []string {
	var ids []string
	for _, n := range m.treeNodes {
		if n.AgentIdx >= 0 && n.Sub == nil {
			ids = append(ids, m.agents[n.AgentIdx].SessionID)
		}
	}
	return ids
}

func TestSearchSlashOpensMode(t *testing.T) {
	m := newSearchModel(t)
	res, _ := m.handleKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	if res.(model).mode != modeSearch {
		t.Fatalf("'/' should open search mode, got mode %d", res.(model).mode)
	}
}

func TestSearchFiltersByText(t *testing.T) {
	m := newSearchModel(t)
	m.mode = modeSearch
	m.searchText = "login" // matches orph-1's branch only
	m.buildTree()
	ids := visibleAgentIDs(m)
	if len(ids) != 1 || ids[0] != "orph-1" {
		t.Errorf("expected only orph-1 visible, got %v", ids)
	}
}

func TestSearchOrphanToggleFilters(t *testing.T) {
	m := newSearchModel(t)
	res, _ := m.handleKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	res2, _ := res.(model).handleKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m3 := res2.(model)
	if !m3.searchOrphanOnly {
		t.Fatal("ctrl+o should enable orphan-only filter")
	}
	ids := visibleAgentIDs(m3)
	if len(ids) != 1 || ids[0] != "orph-1" {
		t.Errorf("orphan-only should show only orph-1, got %v", ids)
	}
}

func TestSearchEnterResumesOrphan(t *testing.T) {
	m := newSearchModel(t)
	m.mode = modeSearch
	m.searchOrphanOnly = true
	m.buildTree()
	selectFirstAgent(&m)
	res, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res.(model).mode == modeConfirmJump {
		t.Fatal("enter on an orphan should resume, not confirm-jump")
	}
	if cmd == nil {
		t.Fatal("enter on an orphan should dispatch a resume command")
	}
}

func TestSearchEnterJumpsLiveAgent(t *testing.T) {
	m := newSearchModel(t)
	m.mode = modeSearch
	m.searchText = "alpha" // matches live-1 only
	m.buildTree()
	selectFirstAgent(&m)
	res, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res.(model).mode != modeConfirmJump {
		t.Fatalf("enter on a live agent should confirm jump, got mode %d", res.(model).mode)
	}
}

func TestSearchEscClosesAndClears(t *testing.T) {
	m := newSearchModel(t)
	m.mode = modeSearch
	m.searchText = "login"
	m.searchOrphanOnly = true
	m.buildTree()
	res, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2 := res.(model)
	if m2.mode != modeNormal {
		t.Fatalf("esc should return to normal mode, got %d", m2.mode)
	}
	if m2.searchText != "" || m2.searchOrphanOnly {
		t.Error("esc should clear search text and orphan filter")
	}
	if len(visibleAgentIDs(m2)) != 2 {
		t.Error("esc should restore the full agent list")
	}
}
