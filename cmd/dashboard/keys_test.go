package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModelWithAgents() model {
	m := newModel("", "", nil)
	m.tmuxAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []SubagentInfo{
		{AgentID: "aaa", AgentType: "Explore", Description: "sub1"},
	}
	m.buildTree()
	// Tree: [parent0(0), sub-aaa(1), parent1(2)]
	return m
}

func TestShiftDownJumpsToNextParent(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyShiftDown}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selected != 2 {
		t.Errorf("shift+down from parent0: expected selected=2, got %d", rm.selected)
	}
}

func TestShiftUpJumpsToPrevParent(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 2

	msg := tea.KeyMsg{Type: tea.KeyShiftUp}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selected != 0 {
		t.Errorf("shift+up from parent1: expected selected=0, got %d", rm.selected)
	}
}

func TestCtrlDownDoesNotJump(t *testing.T) {
	m := newTestModelWithAgents()
	// Start at parent0 (idx 0) — old code would jump to parent1 (idx 2)
	m.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyCtrlDown}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// ctrl+down should NOT jump (feature removed), selection stays at 0
	if rm.selected != 0 {
		t.Errorf("ctrl+down should not change selection, expected 0, got %d", rm.selected)
	}
}

func TestCtrlUpDoesNotJump(t *testing.T) {
	m := newTestModelWithAgents()
	// Start at parent1 (idx 2) — if ctrl+up still worked, it would jump to 0
	m.selected = 2

	msg := tea.KeyMsg{Type: tea.KeyCtrlUp}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// ctrl+up should NOT jump (feature removed), selection stays at 2
	if rm.selected != 2 {
		t.Errorf("ctrl+up should not change selection, expected 2, got %d", rm.selected)
	}
}

func TestAKeyEntersCreateFolderMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateFolder {
		t.Errorf("expected modeCreateFolder, got %d", rm.mode)
	}
}

func TestAKeyNoopWithoutTmux(t *testing.T) {
	m := newTestModelWithAgents()
	m.tmuxAvailable = false

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal when tmux unavailable, got %d", rm.mode)
	}
	if rm.statusMsg == "" {
		t.Error("expected status message about tmux not available")
	}
}

func TestCreateFolderMode_EscReturnsToNormal(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("/some/path")

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", rm.mode)
	}
	if rm.textInput.Value() != "" {
		t.Error("expected textInput to be reset after esc")
	}
}

func TestShiftSDoesNothing(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// "S" should not set any status message (feature removed)
	if rm.statusMsg != "" {
		t.Errorf("S key should not set statusMsg, got %q", rm.statusMsg)
	}
}

func TestCreateFolderMode_EnterAcceptsSuggestion(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.suggestions = []string{"/Users/test/code/myrepo", "/Users/test/code/other"}
	m.selectedSugg = 0
	// textInput is empty — user arrow-selected a suggestion without Tab

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after enter, got %d", rm.mode)
	}
	// A command should be returned (createSession) since suggestion was used
	if cmd == nil {
		t.Error("expected createSession command when suggestion available, got nil")
	}
}

func TestCreateFolderMode_EnterUsesHighlightedSuggestion(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("sales") // user typed partial query
	m.suggestions = []string{"/Users/test/code/sales-app", "/Users/test/code/sales-demo"}
	m.selectedSugg = 1

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleKey(msg)

	// Should use the highlighted suggestion, not the partial text "sales"
	if cmd == nil {
		t.Error("expected createSession command when suggestion highlighted, got nil")
	}
}

func TestCreateFolderMode_DownAdvancesSelection(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.suggestions = []string{"/Users/test/code/a", "/Users/test/code/b"}
	m.selectedSugg = 0

	msg := tea.KeyMsg{Type: tea.KeyDown}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selectedSugg != 1 {
		t.Errorf("expected selectedSugg=1 after down key, got %d", rm.selectedSugg)
	}
}

func TestCreateFolderMode_TypingResetsSelection(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.selectedSugg = 2
	m.suggestions = []string{"/Users/test/code/a", "/Users/test/code/b", "/Users/test/code/c"}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selectedSugg != 0 {
		t.Errorf("expected selectedSugg=0 after typing, got %d", rm.selectedSugg)
	}
}

func TestCreateFolderMode_EnterWithTextUsesSuggestionWhenVisible(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("typed")
	m.suggestions = []string{"/Users/test/code/suggestion"}
	m.selectedSugg = 0

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleKey(msg)

	// When suggestions are visible the highlighted entry is always used,
	// even if the user typed partial text without navigating.
	if cmd == nil {
		t.Error("expected createSession command using highlighted suggestion, got nil")
	}
}

func TestCreateFolderMode_EnterWithTextNoSuggestionsUsesText(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("/Users/test/code/typed-path")
	m.suggestions = nil // no suggestions visible
	m.selectedSugg = 0

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleKey(msg)

	// No suggestions visible — fall back to typed text.
	if cmd == nil {
		t.Error("expected createSession command for typed path, got nil")
	}
}

func TestCreateFolderMode_UpWrapsSelection(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.suggestions = []string{"/Users/test/code/a", "/Users/test/code/b"}
	m.selectedSugg = 0

	msg := tea.KeyMsg{Type: tea.KeyUp}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selectedSugg != 1 {
		t.Errorf("expected selectedSugg=1 after up wrap, got %d", rm.selectedSugg)
	}
}
