package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModelWithAgents() model {
	m := newModel(testConfig(""), nil)
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
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// With no skills available, folder Enter advances to message input
	if rm.mode != modeCreateMessage {
		t.Errorf("expected modeCreateMessage after enter, got %d", rm.mode)
	}
	if rm.createFolder != "/Users/test/code/myrepo" {
		t.Errorf("expected createFolder=/Users/test/code/myrepo, got %q", rm.createFolder)
	}
}

func TestCreateFolderMode_EnterUsesHighlightedSuggestion(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("sales") // user typed partial query
	m.suggestions = []string{"/Users/test/code/sales-app", "/Users/test/code/sales-demo"}
	m.selectedSugg = 1

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// Should use the highlighted suggestion, not the partial text "sales"
	if rm.createFolder != "/Users/test/code/sales-demo" {
		t.Errorf("expected createFolder=/Users/test/code/sales-demo, got %q", rm.createFolder)
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
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// When suggestions are visible the highlighted entry is always used,
	// even if the user typed partial text without navigating.
	if rm.createFolder != "/Users/test/code/suggestion" {
		t.Errorf("expected createFolder=/Users/test/code/suggestion, got %q", rm.createFolder)
	}
	if rm.mode != modeCreateMessage {
		t.Errorf("expected modeCreateMessage, got %d", rm.mode)
	}
}

func TestCreateFolderMode_EnterWithTextNoSuggestionsUsesText(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.SetValue("/Users/test/code/typed-path")
	m.suggestions = nil // no suggestions visible
	m.selectedSugg = 0

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// No suggestions visible — fall back to typed text.
	if rm.createFolder != "/Users/test/code/typed-path" {
		t.Errorf("expected createFolder=/Users/test/code/typed-path, got %q", rm.createFolder)
	}
	if rm.mode != modeCreateMessage {
		t.Errorf("expected modeCreateMessage, got %d", rm.mode)
	}
}

func TestUsageModeWorksWithNoAgents(t *testing.T) {
	m := newModel(testConfig(""), nil)
	m.agents = nil // no agents
	m.mode = modeUsage

	m.updateRightContent()

	content := m.messageVP.View()
	if !strings.Contains(content, "USAGE BREAKDOWN") {
		t.Errorf("expected usage content when mode is modeUsage with no agents, got %q", content)
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

func TestHKeyOpensHelp(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if !rm.helpVisible {
		t.Error("expected helpVisible=true after pressing h")
	}
}

func TestHKeyClosesHelp(t *testing.T) {
	m := newTestModelWithAgents()
	m.helpVisible = true

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.helpVisible {
		t.Error("expected helpVisible=false after pressing h in help overlay")
	}
}

func TestEscClosesHelp(t *testing.T) {
	m := newTestModelWithAgents()
	m.helpVisible = true

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.helpVisible {
		t.Error("expected helpVisible=false after pressing esc in help overlay")
	}
}

func TestHelpOverlaySwallowsKeys(t *testing.T) {
	m := newTestModelWithAgents()
	m.helpVisible = true

	// 'r' should not enter reply mode when help is visible
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal when help visible, got %d", rm.mode)
	}
	if !rm.helpVisible {
		t.Error("help should remain visible when pressing unrelated key")
	}
}

// -- Create wizard: skill selection + message input tests --

func TestCreateWizard_FolderToSkill(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.skillsAvailable = true
	m.availableSkills = []string{"(none)", "feature", "fix"}
	m.textInput.SetValue("/Users/test/code/myrepo")
	m.suggestions = nil

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateSkill {
		t.Errorf("expected modeCreateSkill when skills available, got %d", rm.mode)
	}
	if rm.createFolder != "/Users/test/code/myrepo" {
		t.Errorf("expected createFolder to be stashed, got %q", rm.createFolder)
	}
	if rm.selectedCreateSkill != 0 {
		t.Errorf("expected selectedCreateSkill=0, got %d", rm.selectedCreateSkill)
	}
}

func TestCreateWizard_SkillNavigation(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill
	m.availableSkills = []string{"(none)", "chore", "feature", "fix"}
	m.selectedCreateSkill = 0

	// Down
	msg := tea.KeyMsg{Type: tea.KeyDown}
	result, _ := m.handleKey(msg)
	rm := result.(model)
	if rm.selectedCreateSkill != 1 {
		t.Errorf("expected selectedCreateSkill=1 after down, got %d", rm.selectedCreateSkill)
	}

	// Up from 1 back to 0
	rm.mode = modeCreateSkill
	msg = tea.KeyMsg{Type: tea.KeyUp}
	result, _ = rm.handleKey(msg)
	rm = result.(model)
	if rm.selectedCreateSkill != 0 {
		t.Errorf("expected selectedCreateSkill=0 after up, got %d", rm.selectedCreateSkill)
	}

	// Up from 0 stays at 0 (no wrap)
	msg = tea.KeyMsg{Type: tea.KeyUp}
	result, _ = rm.handleKey(msg)
	rm = result.(model)
	if rm.selectedCreateSkill != 0 {
		t.Errorf("expected selectedCreateSkill=0 (clamped), got %d", rm.selectedCreateSkill)
	}
}

func TestCreateWizard_SkillToMessage(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill
	m.availableSkills = []string{"(none)", "feature", "fix"}
	m.selectedCreateSkill = 1 // "feature"
	m.createFolder = "/Users/test/repo"

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateMessage {
		t.Errorf("expected modeCreateMessage, got %d", rm.mode)
	}
	if rm.createSkillName != "feature" {
		t.Errorf("expected createSkillName=feature, got %q", rm.createSkillName)
	}
}

func TestCreateWizard_SkillNoneToMessage(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill
	m.availableSkills = []string{"(none)", "feature", "fix"}
	m.selectedCreateSkill = 0 // "(none)"

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateMessage {
		t.Errorf("expected modeCreateMessage, got %d", rm.mode)
	}
	if rm.createSkillName != "" {
		t.Errorf("expected empty createSkillName for (none), got %q", rm.createSkillName)
	}
}

func TestCreateWizard_EscFromSkillGoesBackToFolder(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill
	m.createFolder = "/Users/test/repo"
	m.selectedCreateSkill = 2

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateFolder {
		t.Errorf("expected modeCreateFolder after esc from skill, got %d", rm.mode)
	}
	// Folder should be preserved in the text input
	if rm.textInput.Value() != "/Users/test/repo" {
		t.Errorf("expected textInput to show previous folder, got %q", rm.textInput.Value())
	}
	if rm.selectedCreateSkill != 0 {
		t.Errorf("expected selectedCreateSkill reset, got %d", rm.selectedCreateSkill)
	}
}

func TestCreateWizard_CtrlCFromSkillCancels(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill
	m.createFolder = "/Users/test/repo"
	m.selectedCreateSkill = 2

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after ctrl+c, got %d", rm.mode)
	}
	if rm.createFolder != "" {
		t.Errorf("expected createFolder reset, got %q", rm.createFolder)
	}
}

func TestCreateWizard_EscFromMessageGoesBack(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.createFolder = "/Users/test/repo"
	m.createSkillName = "feature"
	m.skillsAvailable = true
	m.availableSkills = []string{"(none)", "feature", "fix"}
	m.textInput.SetValue("some message")

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// Should go back to skill selection since skills are available
	if rm.mode != modeCreateSkill {
		t.Errorf("expected modeCreateSkill after esc from message, got %d", rm.mode)
	}
	if rm.createSkillName != "" {
		t.Errorf("expected createSkillName reset for re-selection, got %q", rm.createSkillName)
	}
}

func TestCreateWizard_EscFromMessageGoesToFolderWhenNoSkills(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.createFolder = "/Users/test/repo"
	m.skillsAvailable = false
	m.textInput.SetValue("some message")

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// No skills — should go back to folder selection
	if rm.mode != modeCreateFolder {
		t.Errorf("expected modeCreateFolder after esc (no skills), got %d", rm.mode)
	}
	if rm.textInput.Value() != "/Users/test/repo" {
		t.Errorf("expected textInput to show previous folder, got %q", rm.textInput.Value())
	}
}

func TestCreateWizard_CtrlCFromMessageCancels(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.createFolder = "/Users/test/repo"
	m.createSkillName = "feature"
	m.textInput.SetValue("some message")

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after ctrl+c, got %d", rm.mode)
	}
	if rm.createFolder != "" {
		t.Errorf("expected createFolder reset, got %q", rm.createFolder)
	}
	if rm.createSkillName != "" {
		t.Errorf("expected createSkillName reset, got %q", rm.createSkillName)
	}
}

func TestCreateWizard_MessageLaunch(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.createFolder = "/Users/test/repo"
	m.createSkillName = "feature"
	m.textInput.SetValue("add login page")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after launch, got %d", rm.mode)
	}
	if rm.statusMsg != "spawning" {
		t.Errorf("expected statusMsg=spawning, got %q", rm.statusMsg)
	}
	if cmd == nil {
		t.Error("expected command batch for createSessionWithPrompt")
	}
	// Wizard state should be reset
	if rm.createFolder != "" || rm.createSkillName != "" || rm.selectedCreateSkill != 0 {
		t.Error("expected wizard state to be reset after launch")
	}
}

func TestCreateWizard_MessageEmptyLaunch(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.createFolder = "/Users/test/repo"
	m.createSkillName = ""
	// textInput is empty — no message

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after launch, got %d", rm.mode)
	}
	if cmd == nil {
		t.Error("expected command even with empty message (launches agent without prompt)")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello world", "'hello world'"},
		{"what does this do>", "'what does this do>'"},
		{"it's a test", "'it'\\''s a test'"},
		{"foo|bar&baz", "'foo|bar&baz'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		skill, message, want string
	}{
		{"feature", "add login", "/feature add login"},
		{"fix", "", "/fix"},
		{"", "do something", "do something"},
		{"", "", ""},
	}
	for _, tt := range tests {
		got := buildPrompt(tt.skill, tt.message)
		if got != tt.want {
			t.Errorf("buildPrompt(%q, %q) = %q, want %q", tt.skill, tt.message, got, tt.want)
		}
	}
}

func TestGKeyPinsPRState_NoGH(t *testing.T) {
	// When gh is not available, pressing "g" should still immediately pin
	// to "pr" state (backward compat — manual workflow).
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "sess1.json"), []byte(`{"state":"question"}`), 0644)

	m := newModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = false
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question",
			SessionID: "sess1", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 0

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if cmd == nil {
		t.Fatal("expected cmd from 'g' key, got nil")
	}

	// Execute batch commands and check for pinStateMsg
	var hasPinMsg bool
	batchResult := cmd()
	if batch, ok := batchResult.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			msg := c()
			if _, ok := msg.(pinStateMsg); ok {
				hasPinMsg = true
			}
		}
	}

	if !hasPinMsg {
		t.Error("pressing 'g' without gh should immediately pin agent to 'pr' state")
	}
}

func TestGKeyDefersPin_WithGH(t *testing.T) {
	// When gh is available, pressing "g" should NOT immediately pin —
	// pinning is deferred to the openPRMsg handler based on whether a PR exists.
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "sess1.json"), []byte(`{"state":"question"}`), 0644)

	m := newModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question",
			SessionID: "sess1", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 0

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if cmd == nil {
		t.Fatal("expected cmd from 'g' key, got nil")
	}

	// Should store session ID for deferred pinning
	updated := result.(model)
	if updated.openPRSessionID != "sess1" {
		t.Errorf("expected openPRSessionID='sess1', got %q", updated.openPRSessionID)
	}

	// Should NOT have pinStateMsg in the batch — pinning is deferred
	var hasPinMsg bool
	batchResult := cmd()
	if batch, ok := batchResult.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			msg := c()
			if _, ok := msg.(pinStateMsg); ok {
				hasPinMsg = true
			}
		}
	}

	if hasPinMsg {
		t.Error("pressing 'g' with gh available should NOT immediately pin — defer to openPRMsg")
	}
}

func TestMKey_WithGH_EntersConfirmMode(t *testing.T) {
	// When gh is available and agent is in "pr" state, pressing "m" should
	// enter modeConfirmMerge instead of merging immediately.
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "sess1.json"), []byte(`{"state":"pr","pinned_state":"pr"}`), 0644)

	m := newModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "pr",
			SessionID: "sess1", TmuxPaneID: "%5", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 0

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil {
		t.Fatal("expected nil cmd from 'm' key (should enter confirm mode, not execute)")
	}

	updated := result.(model)
	if updated.mode != modeConfirmMerge {
		t.Errorf("expected modeConfirmMerge, got %d", updated.mode)
	}
	if updated.confirmMergeSessionID != "sess1" {
		t.Errorf("expected confirmMergeSessionID='sess1', got %q", updated.confirmMergeSessionID)
	}
	if !strings.Contains(updated.statusMsg, "Merge") {
		t.Errorf("expected status to contain 'Merge', got %q", updated.statusMsg)
	}
}

func TestMKey_ConfirmMerge_Y_ExecutesMerge(t *testing.T) {
	// Confirming with 'y' in modeConfirmMerge should execute the merge.
	m := newModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.mode = modeConfirmMerge
	m.confirmMergeSessionID = "sess1"
	m.confirmMergePaneID = "%5"
	m.confirmMergeDir = t.TempDir()
	m.confirmMergeBranch = "feat/test"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected cmd after confirming merge")
	}

	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after confirm, got %d", updated.mode)
	}
	if updated.mergeSessionID != "sess1" {
		t.Errorf("expected mergeSessionID='sess1', got %q", updated.mergeSessionID)
	}
	if !strings.Contains(updated.statusMsg, "Merging") {
		t.Errorf("expected status to contain 'Merging', got %q", updated.statusMsg)
	}
}

func TestMKey_ConfirmMerge_Esc_Cancels(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmMergeSessionID = "sess1"
	m.statusMsg = "Merge feat/test? (y/n)"

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", updated.mode)
	}
	if updated.confirmMergeSessionID != "" {
		t.Error("expected confirmMergeSessionID to be cleared")
	}
	if updated.statusMsg != "" {
		t.Errorf("expected empty status after cancel, got %q", updated.statusMsg)
	}
}

func TestMKey_NoGH_ConfirmThenPin(t *testing.T) {
	// When gh is not available, pressing "m" should enter confirm mode.
	// Confirming with 'y' should pin to "merged" and send cleanup message.
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "sess1.json"), []byte(`{"state":"pr","pinned_state":"pr"}`), 0644)

	m := newModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = false
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "pr",
			SessionID: "sess1", TmuxPaneID: "%5", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 0

	// Step 1: press 'm' — should enter confirm mode
	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil {
		t.Fatal("expected nil cmd from 'm' (confirm mode)")
	}
	m = result.(model)
	if m.mode != modeConfirmMerge {
		t.Fatalf("expected modeConfirmMerge, got %d", m.mode)
	}

	// Step 2: confirm with 'y' — should execute merge
	result, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected cmd after confirming merge")
	}

	updated := result.(model)
	if !strings.Contains(updated.statusMsg, "Marked as merged") {
		t.Errorf("expected status to contain 'Marked as merged', got %q", updated.statusMsg)
	}

	// Should have pinStateMsg in the batch
	var hasPinMsg bool
	batchResult := cmd()
	if batch, ok := batchResult.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			msg := c()
			if _, ok := msg.(pinStateMsg); ok {
				hasPinMsg = true
			}
		}
	}

	if !hasPinMsg {
		t.Error("confirming merge without gh should pin to 'merged'")
	}
}

func TestYKey_BlockedAgent_EntersConfirmSend(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "permission", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 0

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		t.Fatal("expected nil cmd from 'y' (should enter confirm mode)")
	}

	updated := result.(model)
	if updated.mode != modeConfirmSend {
		t.Errorf("expected modeConfirmSend, got %d", updated.mode)
	}
	if updated.confirmSendKey != "y" {
		t.Errorf("expected confirmSendKey='y', got %q", updated.confirmSendKey)
	}
	if updated.confirmSendPaneID != "%5" {
		t.Errorf("expected confirmSendPaneID='%%5', got %q", updated.confirmSendPaneID)
	}
}

func TestYKey_PlanAgent_MapsTo1(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 0

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated := result.(model)
	if updated.confirmSendKey != "1" {
		t.Errorf("expected plan 'y' to map to '1', got %q", updated.confirmSendKey)
	}
}

func TestNumKey_BlockedAgent_EntersConfirmSend(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 0

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if cmd != nil {
		t.Fatal("expected nil cmd from '3' (should enter confirm mode)")
	}

	updated := result.(model)
	if updated.mode != modeConfirmSend {
		t.Errorf("expected modeConfirmSend, got %d", updated.mode)
	}
	if updated.confirmSendKey != "3" {
		t.Errorf("expected confirmSendKey='3', got %q", updated.confirmSendKey)
	}
}

func TestConfirmSend_Enter_Sends(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd after confirming send")
	}

	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after confirm, got %d", updated.mode)
	}
	if updated.confirmSendPaneID != "" {
		t.Error("expected confirmSendPaneID to be cleared")
	}
}

func TestConfirmSend_Esc_Cancels(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"
	m.statusMsg = "Send 'y' to agent?"

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", updated.mode)
	}
	if updated.confirmSendPaneID != "" {
		t.Error("expected confirmSendPaneID to be cleared")
	}
	if updated.statusMsg != "" {
		t.Errorf("expected empty status after cancel, got %q", updated.statusMsg)
	}
}

func TestConfirmSend_PhantomKey_Swallowed(t *testing.T) {
	// A phantom key like 'm' arriving during modeConfirmSend should be swallowed.
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil {
		t.Fatal("expected nil cmd for unrecognized key in confirm mode")
	}
	updated := result.(model)
	if updated.mode != modeConfirmSend {
		t.Errorf("expected to stay in modeConfirmSend, got %d", updated.mode)
	}
}

func TestEnterKey_EntersConfirmJump(t *testing.T) {
	// Pressing enter on a selected agent should enter modeConfirmJump,
	// NOT immediately jump — guards against phantom keystrokes.
	m := newModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 0

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected nil cmd from enter (should enter confirm mode, not jump)")
	}

	updated := result.(model)
	if updated.mode != modeConfirmJump {
		t.Errorf("expected modeConfirmJump, got %d", updated.mode)
	}
	if updated.confirmJumpPaneID != "%5" {
		t.Errorf("expected confirmJumpPaneID='%%5', got %q", updated.confirmJumpPaneID)
	}
}

func TestConfirmJump_Y_Jumps(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected cmd after confirming jump")
	}

	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after confirm, got %d", updated.mode)
	}
	if updated.confirmJumpPaneID != "" {
		t.Error("expected confirmJumpPaneID to be cleared")
	}
}

func TestConfirmJump_Enter_Jumps(t *testing.T) {
	// Enter should also confirm the jump (natural UX).
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd after confirming jump with enter")
	}

	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after confirm, got %d", updated.mode)
	}
}

func TestConfirmJump_Esc_Cancels(t *testing.T) {
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"
	m.statusMsg = "Jump to agent?"

	result, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", updated.mode)
	}
	if updated.confirmJumpPaneID != "" {
		t.Error("expected confirmJumpPaneID to be cleared")
	}
	if updated.statusMsg != "" {
		t.Errorf("expected empty status after cancel, got %q", updated.statusMsg)
	}
}

func TestConfirmJump_PhantomKey_Swallowed(t *testing.T) {
	// A phantom key arriving during modeConfirmJump should be swallowed.
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil {
		t.Fatal("expected nil cmd for unrecognized key in confirm jump mode")
	}
	updated := result.(model)
	if updated.mode != modeConfirmJump {
		t.Errorf("expected to stay in modeConfirmJump, got %d", updated.mode)
	}
}

func TestConfirmMerge_PhantomKey_Swallowed(t *testing.T) {
	// A phantom key arriving during modeConfirmMerge should be swallowed.
	m := newModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmMergeSessionID = "sess1"

	result, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd != nil {
		t.Fatal("expected nil cmd for unrecognized key in confirm merge mode")
	}
	updated := result.(model)
	if updated.mode != modeConfirmMerge {
		t.Errorf("expected to stay in modeConfirmMerge, got %d", updated.mode)
	}
}
