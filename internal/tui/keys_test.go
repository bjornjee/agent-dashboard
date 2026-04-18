package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// pastConfirmTime returns a time far enough in the past to bypass the cooldown.
var pastConfirmTime = time.Now().Add(-time.Second)

func newTestModelWithAgents() model {
	m := NewModel(testConfig(""), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "aaa", AgentType: "Explore", Description: "sub1"},
	}
	m.buildTree()
	// Tree: [header(0), parent0(1), sub-aaa(2), parent1(3)]
	return m
}

func TestShiftDownJumpsToNextParent(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 1 // parent0 (index 0 is group header)

	msg := tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selected != 3 {
		t.Errorf("shift+down from parent0: expected selected=3, got %d", rm.selected)
	}
}

func TestShiftUpJumpsToPrevParent(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 3 // parent1

	msg := tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selected != 1 {
		t.Errorf("shift+up from parent1: expected selected=1, got %d", rm.selected)
	}
}

func TestCtrlDownDoesNotJump(t *testing.T) {
	m := newTestModelWithAgents()
	// Start at parent0 (idx 1) — old code would jump to parent1
	m.selected = 1

	msg := tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// ctrl+down should NOT jump (feature removed), selection stays at 1
	if rm.selected != 1 {
		t.Errorf("ctrl+down should not change selection, expected 1, got %d", rm.selected)
	}
}

func TestCtrlUpDoesNotJump(t *testing.T) {
	m := newTestModelWithAgents()
	// Start at parent1 (idx 3) — if ctrl+up still worked, it would jump to 1
	m.selected = 3

	msg := tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	// ctrl+up should NOT jump (feature removed), selection stays at 3
	if rm.selected != 3 {
		t.Errorf("ctrl+up should not change selection, expected 3, got %d", rm.selected)
	}
}

func TestAKeyEntersCreateFolderMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.selected = 1

	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeCreateFolder {
		t.Errorf("expected modeCreateFolder, got %d", rm.mode)
	}
}

func TestAKeyNoopWithoutTmux(t *testing.T) {
	m := newTestModelWithAgents()
	m.tmuxAvailable = false

	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
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
	selectFirstAgent(&m)

	msg := tea.KeyPressMsg{Code: 'S', Text: "S"}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyDown}
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

	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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
	m := NewModel(testConfig(""), nil)
	m.agents = nil // no agents
	m.mode = modeUsage
	m.messageVP.SetWidth(80)
	m.messageVP.SetHeight(40)

	m.updateRightContent()

	content := m.messageVP.View()
	if !strings.Contains(content, "USAGE") {
		t.Errorf("expected usage content when mode is modeUsage with no agents, got %q", content)
	}
}

func TestUsageModeSwallowsKeys(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeUsage
	m.selected = 1
	m.messageVP.SetWidth(80)
	m.messageVP.SetHeight(40)

	msg := tea.KeyPressMsg{Code: 'j', Text: "j"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeUsage {
		t.Errorf("expected modeUsage after j in usage mode, got %d", rm.mode)
	}
	if rm.selected != 1 {
		t.Errorf("expected selected=1 unchanged, got %d", rm.selected)
	}
}

func TestUsageModeExitWithU(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeUsage

	msg := tea.KeyPressMsg{Code: 'u', Text: "u"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after u in usage mode, got %d", rm.mode)
	}
}

func TestUsageModeExitWithEsc(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeUsage

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after esc in usage mode, got %d", rm.mode)
	}
}

func TestCreateFolderMode_UpWrapsSelection(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.suggestions = []string{"/Users/test/code/a", "/Users/test/code/b"}
	m.selectedSugg = 0

	msg := tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.selectedSugg != 1 {
		t.Errorf("expected selectedSugg=1 after up wrap, got %d", rm.selectedSugg)
	}
}

func TestHKeyOpensHelp(t *testing.T) {
	m := newTestModelWithAgents()
	selectFirstAgent(&m)

	msg := tea.KeyPressMsg{Code: 'h', Text: "h"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if !rm.helpVisible {
		t.Error("expected helpVisible=true after pressing h")
	}
}

func TestHKeyClosesHelp(t *testing.T) {
	m := newTestModelWithAgents()
	m.helpVisible = true

	msg := tea.KeyPressMsg{Code: 'h', Text: "h"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if rm.helpVisible {
		t.Error("expected helpVisible=false after pressing h in help overlay")
	}
}

func TestEscClosesHelp(t *testing.T) {
	m := newTestModelWithAgents()
	m.helpVisible = true

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
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
	msg := tea.KeyPressMsg{Code: 'r', Text: "r"}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	result, _ := m.handleKey(msg)
	rm := result.(model)
	if rm.selectedCreateSkill != 1 {
		t.Errorf("expected selectedCreateSkill=1 after down, got %d", rm.selectedCreateSkill)
	}

	// Up from 1 back to 0
	rm.mode = modeCreateSkill
	msg = tea.KeyPressMsg{Code: tea.KeyUp}
	result, _ = rm.handleKey(msg)
	rm = result.(model)
	if rm.selectedCreateSkill != 0 {
		t.Errorf("expected selectedCreateSkill=0 after up, got %d", rm.selectedCreateSkill)
	}

	// Up from 0 stays at 0 (no wrap)
	msg = tea.KeyPressMsg{Code: tea.KeyUp}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
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

	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
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

	msg := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
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

	m := NewModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = false
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question",
			SessionID: "sess1", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
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

	m := NewModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question",
			SessionID: "sess1", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
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

	m := NewModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "pr",
			SessionID: "sess1", TmuxPaneID: "%5", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = pastConfirmTime
	m.confirmMergeSessionID = "sess1"
	m.confirmMergePaneID = "%5"
	m.confirmMergeDir = t.TempDir()
	m.confirmMergeBranch = "feat/test"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = pastConfirmTime
	m.confirmMergeSessionID = "sess1"
	m.statusMsg = "Merge feat/test? (y/n)"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
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
	// Confirming with 'y' should pin to "merged".
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "sess1.json"), []byte(`{"state":"pr","pinned_state":"pr"}`), 0644)

	m := NewModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.ghAvailable = false
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "pr",
			SessionID: "sess1", TmuxPaneID: "%5", Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	// Step 1: press 'm' — should enter confirm mode
	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd != nil {
		t.Fatal("expected nil cmd from 'm' (confirm mode)")
	}
	m = result.(model)
	if m.mode != modeConfirmMerge {
		t.Fatalf("expected modeConfirmMerge, got %d", m.mode)
	}

	// Step 2: confirm with 'y' — should pin to merged
	m.confirmEnteredAt = pastConfirmTime // bypass cooldown for test
	result, cmd = m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected cmd after confirming merge")
	}

	updated := result.(model)
	if !strings.Contains(updated.statusMsg, "Marked as merged") {
		t.Errorf("expected status to contain 'Marked as merged', got %q", updated.statusMsg)
	}

	// The returned cmd should be pinAgentStateCmd
	pinMsg := cmd()
	if _, ok := pinMsg.(pinStateMsg); !ok {
		t.Errorf("expected pinStateMsg from cmd, got %T", pinMsg)
	}

	// No-GH path should NOT enter cleanup confirm mode
	if updated.mode == modeConfirmCleanup {
		t.Error("no-GH path should not enter modeConfirmCleanup")
	}
	if updated.cleanupSessionID != "" {
		t.Error("cleanupSessionID should be empty in no-GH path")
	}
}

func TestMKey_ConfirmMerge_Y_StoresMergeCwdFields(t *testing.T) {
	// Confirming merge should propagate Cwd, branch, and worktreeCwd fields.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = pastConfirmTime
	m.confirmMergeSessionID = "sess1"
	m.confirmMergePaneID = "%5"
	m.confirmMergeDir = "/worktrees/app/feat-x" // EffectiveDir (worktree)
	m.confirmMergeBranch = "feat/test"
	m.confirmMergeCwd = "/code/app" // main repo Cwd

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := result.(model)
	if updated.mergeCwd != "/code/app" {
		t.Errorf("expected mergeCwd='/code/app', got %q", updated.mergeCwd)
	}
	if updated.mergeWorktreeCwd != "/worktrees/app/feat-x" {
		t.Errorf("expected mergeWorktreeCwd='/worktrees/app/feat-x', got %q", updated.mergeWorktreeCwd)
	}
	if updated.mergeBranch != "feat/test" {
		t.Errorf("expected mergeBranch='feat/test', got %q", updated.mergeBranch)
	}
	if updated.confirmMergeCwd != "" {
		t.Error("expected confirmMergeCwd to be cleared after confirm")
	}
}

func TestMKey_ConfirmMerge_Y_NonWorktree_NoWorktreeCwd(t *testing.T) {
	// When Cwd == EffectiveDir, mergeWorktreeCwd should be empty.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = pastConfirmTime
	m.confirmMergeSessionID = "sess1"
	m.confirmMergePaneID = "%5"
	m.confirmMergeDir = "/code/app" // same as Cwd
	m.confirmMergeBranch = "feat/test"
	m.confirmMergeCwd = "/code/app"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := result.(model)
	if updated.mergeWorktreeCwd != "" {
		t.Errorf("expected empty mergeWorktreeCwd for non-worktree, got %q", updated.mergeWorktreeCwd)
	}
}

func TestConfirmCleanup_Y_FiresCleanup(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmCleanup
	m.confirmEnteredAt = pastConfirmTime
	m.cleanupSessionID = "sess1"
	m.cleanupPaneID = "%5"
	m.cleanupCwd = "/code/app"
	m.cleanupWorktreeCwd = "/worktrees/app/feat-x"
	m.cleanupBranch = "feat/test"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected cmd after confirming cleanup")
	}
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal, got %d", updated.mode)
	}
	if updated.cleanupSessionID != "" {
		t.Error("expected cleanupSessionID to be cleared")
	}
	if !strings.Contains(updated.statusMsg, "Cleaning up") {
		t.Errorf("expected status to contain 'Cleaning up', got %q", updated.statusMsg)
	}
}

func TestConfirmCleanup_Esc_Cancels(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmCleanup
	m.confirmEnteredAt = pastConfirmTime
	m.cleanupSessionID = "sess1"
	m.cleanupPaneID = "%5"
	m.cleanupCwd = "/code/app"
	m.cleanupBranch = "feat/test"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after esc, got %d", updated.mode)
	}
	if updated.cleanupSessionID != "" {
		t.Error("expected cleanupSessionID to be cleared")
	}
	if !strings.Contains(updated.statusMsg, "PR merged") {
		t.Errorf("expected status 'PR merged', got %q", updated.statusMsg)
	}
}

func TestConfirmCleanup_PhantomGuard(t *testing.T) {
	// 'y' should be swallowed during cooldown period.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmCleanup
	m.confirmEnteredAt = time.Now() // just entered — within cooldown

	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	filtered := PhantomFilter(m, msg)
	if filtered != nil {
		t.Error("expected 'y' to be swallowed during cleanup confirm cooldown")
	}
}

func TestYKey_BlockedAgent_EntersConfirmSend(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "permission", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
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

func TestYKey_PlanAgent_SendsEnter(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := result.(model)
	// Plan approval sends Enter (to select default "Allow") rather than bare "y"
	// which Claude Code 6.1's ExitPlanMode prompt does not accept.
	if updated.confirmSendKey != "Enter" {
		t.Errorf("expected plan 'y' to send 'Enter', got %q", updated.confirmSendKey)
	}
}

func TestNumKey_BlockedAgent_EntersConfirmSend(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.selected = 1 // index 0 is group header

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: '3', Text: "3"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmEnteredAt = pastConfirmTime
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"
	m.statusMsg = "Send 'y' to agent?"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmEnteredAt = pastConfirmTime
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmEnteredAt = pastConfirmTime
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd after confirming jump with enter")
	}

	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal after confirm, got %d", updated.mode)
	}
}

func TestConfirmJump_Esc_Cancels(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"
	m.statusMsg = "Jump to agent?"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmJump
	m.confirmJumpPaneID = "%5"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'm', Text: "m"})
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
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmMergeSessionID = "sess1"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil {
		t.Fatal("expected nil cmd for unrecognized key in confirm merge mode")
	}
	updated := result.(model)
	if updated.mode != modeConfirmMerge {
		t.Errorf("expected to stay in modeConfirmMerge, got %d", updated.mode)
	}
}

func TestConfirmCooldown_RejectsPhantomConfirmation(t *testing.T) {
	// A confirmation arriving within the cooldown period should be swallowed
	// by PhantomFilter before reaching handleKey.
	tests := []struct {
		name string
		mode int
		key  tea.KeyPressMsg
	}{
		{"merge_y", modeConfirmMerge, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"close_y", modeConfirmClose, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"send_enter", modeConfirmSend, tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"jump_y", modeConfirmJump, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"jump_enter", modeConfirmJump, tea.KeyPressMsg{Code: tea.KeyEnter}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(testConfig(t.TempDir()), nil)
			m.mode = tt.mode
			m.confirmEnteredAt = time.Now() // just entered — within cooldown

			result := PhantomFilter(m, tt.key)
			if result != nil {
				t.Fatal("expected nil — confirmation should be swallowed by PhantomFilter during cooldown")
			}
		})
	}
}

func TestConfirmCooldown_AcceptsAfterCooldown(t *testing.T) {
	// A confirmation arriving after the cooldown passes PhantomFilter and
	// is processed normally by handleKey.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = time.Now().Add(-time.Second) // well past cooldown
	m.tmuxAvailable = true
	m.ghAvailable = true
	m.confirmMergeSessionID = "sess1"
	m.confirmMergePaneID = "%5"
	m.confirmMergeDir = t.TempDir()
	m.confirmMergeBranch = "feat/test"

	result, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected cmd after cooldown period")
	}
	updated := result.(model)
	if updated.mode != modeNormal {
		t.Errorf("expected modeNormal, got %d", updated.mode)
	}
}

func TestPhantomKey_EnterRejected(t *testing.T) {
	// A phantom Enter (from a fragmented mouse escape sequence) should be
	// swallowed by PhantomFilter.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.lastEscapeAt = time.Now() // mouse event just happened

	result := PhantomFilter(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if result != nil {
		t.Error("phantom Enter should be swallowed by PhantomFilter")
	}
}

func TestPhantomKey_MergeRejected(t *testing.T) {
	// A phantom "m" from a mouse escape sequence should be swallowed by PhantomFilter.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "pr", TmuxPaneID: "%5",
			Cwd: t.TempDir(), Branch: "feat/test"},
	}
	m.buildTree()
	m.lastEscapeAt = time.Now()

	result := PhantomFilter(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if result != nil {
		t.Error("phantom 'm' should be swallowed by PhantomFilter")
	}
}

func TestPhantomKey_CloseRejected(t *testing.T) {
	// A phantom "x" should be swallowed by PhantomFilter.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	m.lastEscapeAt = time.Now()

	result := PhantomFilter(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if result != nil {
		t.Error("phantom 'x' should be swallowed by PhantomFilter")
	}
}

func TestPhantomKey_AcceptedAfterCooldown(t *testing.T) {
	// A real Enter key arriving well after the mouse cooldown should be accepted.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	selectFirstAgent(&m)
	m.lastEscapeAt = time.Now().Add(-100 * time.Millisecond) // well past 50ms cooldown

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := result.(model)
	if updated.mode != modeConfirmJump {
		t.Errorf("real Enter should be accepted; got mode %d, want modeConfirmJump", updated.mode)
	}
}

func TestHandleMouse_DoesNotSetLastEscapeAt(t *testing.T) {
	// Mouse events in bubbletea v2 are fully parsed by the framework, so
	// they should NOT refresh lastEscapeAt (which would block guarded keys).
	m := NewModel(testConfig(t.TempDir()), nil)
	if !m.lastEscapeAt.IsZero() {
		t.Fatal("lastEscapeAt should be zero initially")
	}

	result, _ := m.handleMouse(tea.MouseMotionMsg{})
	updated := result.(model)
	if !updated.lastEscapeAt.IsZero() {
		t.Error("handleMouse should NOT set lastEscapeAt — bubbletea v2 fully parses mouse sequences")
	}
}

func TestFocusEvent_SetsLastEscapeAt(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	if !m.lastEscapeAt.IsZero() {
		t.Fatal("lastEscapeAt should be zero initially")
	}

	before := time.Now()
	result, _ := m.Update(tea.FocusMsg{})
	after := time.Now()

	updated := result.(model)
	if updated.lastEscapeAt.Before(before) || updated.lastEscapeAt.After(after) {
		t.Errorf("lastEscapeAt should be set by focus event; got %v", updated.lastEscapeAt)
	}
}

func TestPhantomKey_AfterFocus_EnterRejected(t *testing.T) {
	m := newTestModelWithAgents()
	m.lastEscapeAt = time.Now() // focus event just happened

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result := PhantomFilter(m, msg)
	if result != nil {
		t.Error("enter should be swallowed as phantom after focus event")
	}
}

func TestIsSelfPane_SamePaneBlocked(t *testing.T) {
	if !isSelfPane("%5", "%5") {
		t.Error("isSelfPane should return true when paneID == selfPaneID")
	}
}

func TestIsSelfPane_DifferentPaneAllowed(t *testing.T) {
	if isSelfPane("%5", "%0") {
		t.Error("isSelfPane should return false when paneID != selfPaneID")
	}
}

func TestIsSelfPane_EmptySelfSkipsGuard(t *testing.T) {
	if isSelfPane("%5", "") {
		t.Error("isSelfPane should return false when selfPaneID is empty")
	}
}

// -- Mode-transition phantom tests --

func TestPhantomEnter_AfterReplySubmit(t *testing.T) {
	// When Enter submits a reply in modeReply, transitioning to modeNormal,
	// a rapid follow-up Enter (phantom from key release or terminal artefact)
	// must be swallowed by PhantomFilter.
	m := newTestModelWithAgents()
	m.agents[0].TmuxPaneID = "%5"
	m.mode = modeReply
	m.textInput.SetValue("test reply")
	m.buildTree()

	// Press Enter to submit reply → mode returns to modeNormal.
	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(model)
	if rm.mode != modeNormal {
		t.Fatalf("expected modeNormal after reply submit, got %d", rm.mode)
	}

	// Immediately press Enter again (phantom) — filter should swallow it.
	filtered := PhantomFilter(rm, tea.KeyPressMsg{Code: tea.KeyEnter})
	if filtered != nil {
		t.Error("phantom Enter after reply submit should be swallowed by PhantomFilter")
	}
}

func TestPhantomEnter_AfterConfirmSend(t *testing.T) {
	// When Enter confirms a key send in modeConfirmSend, a rapid follow-up
	// Enter must be swallowed by PhantomFilter.
	m := newTestModelWithAgents()
	m.agents[0].TmuxPaneID = "%5"
	m.agents[0].State = "waiting"
	m.mode = modeConfirmSend
	m.confirmEnteredAt = pastConfirmTime // bypass confirm cooldown
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"
	m.buildTree()

	// Press Enter to confirm send → mode returns to modeNormal.
	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(model)
	if rm.mode != modeNormal {
		t.Fatalf("expected modeNormal after confirm send, got %d", rm.mode)
	}

	// Immediately press Enter again (phantom) — filter should swallow it.
	filtered := PhantomFilter(rm, tea.KeyPressMsg{Code: tea.KeyEnter})
	if filtered != nil {
		t.Error("phantom Enter after confirm-send should be swallowed by PhantomFilter")
	}
}

func TestPhantomEnter_AfterCreateMessage(t *testing.T) {
	// When Enter submits in modeCreateMessage, a rapid follow-up Enter
	// must be swallowed by PhantomFilter.
	m := newTestModelWithAgents()
	m.agents[0].TmuxPaneID = "%5"
	m.mode = modeCreateMessage
	m.createFolder = "/tmp/test"
	m.buildTree()

	// Press Enter to submit → mode returns to modeNormal.
	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(model)
	if rm.mode != modeNormal {
		t.Fatalf("expected modeNormal after create message submit, got %d", rm.mode)
	}

	// Immediately press Enter again (phantom) — filter should swallow it.
	filtered := PhantomFilter(rm, tea.KeyPressMsg{Code: tea.KeyEnter})
	if filtered != nil {
		t.Error("phantom Enter after create-message should be swallowed by PhantomFilter")
	}
}

func TestRealEnter_AfterCooldown(t *testing.T) {
	// A real Enter arriving well after a mode transition should work normally.
	m := newTestModelWithAgents()
	m.agents[0].TmuxPaneID = "%5"
	m.mode = modeReply
	m.textInput.SetValue("test")
	m.buildTree()
	selectFirstAgent(&m)

	// Press Enter to submit reply.
	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := result.(model)

	// Simulate time passing beyond the cooldown.
	rm.modeResetAt = time.Now().Add(-time.Second)

	// Verify PhantomFilter lets the key through.
	enterMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	filtered := PhantomFilter(rm, enterMsg)
	if filtered == nil {
		t.Fatal("real Enter after cooldown should pass through PhantomFilter")
	}

	// Now press Enter — should work normally.
	result2, _ := rm.handleKey(enterMsg)
	rm2 := result2.(model)
	if rm2.mode != modeConfirmJump {
		t.Errorf("real Enter after cooldown should trigger modeConfirmJump, got mode %d", rm2.mode)
	}
}

func TestMergeGH_PinsToMerged(t *testing.T) {
	m := newTestModelWithAgents()
	m.mergeSessionID = "sess-456"
	m.mergePaneID = "%7"

	result, cmd := m.Update(mergePRMsg{})
	rm := result.(model)

	if !strings.Contains(rm.statusMsg, "PR merged") {
		t.Errorf("expected status to contain 'PR merged', got %q", rm.statusMsg)
	}
	if cmd == nil {
		t.Fatal("expected pin cmd after merge")
	}
	pinMsg := cmd()
	if _, ok := pinMsg.(pinStateMsg); !ok {
		t.Errorf("expected pinStateMsg, got %T", pinMsg)
	}
}

func TestYKey_PlanState_SetsConfirmSendLabel(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", TmuxPaneID: "%5"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := result.(model)
	if updated.mode != modeConfirmSend {
		t.Fatalf("expected modeConfirmSend, got %d", updated.mode)
	}
	if updated.confirmSendLabel != "Plan approved" {
		t.Errorf("expected label 'Plan approved', got %q", updated.confirmSendLabel)
	}
	if updated.confirmSendKey != "Enter" {
		t.Errorf("expected sendKey 'Enter' for plan approve, got %q", updated.confirmSendKey)
	}
}

func TestNumberKey_BlockedState_SetsConfirmSendLabel(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "permission", TmuxPaneID: "%5"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, _ := m.handleKey(tea.KeyPressMsg{Code: '3', Text: "3"})
	updated := result.(model)
	if updated.mode != modeConfirmSend {
		t.Fatalf("expected modeConfirmSend, got %d", updated.mode)
	}
	if updated.confirmSendLabel != "Sent '3'" {
		t.Errorf("expected label \"Sent '3'\", got %q", updated.confirmSendLabel)
	}
}

func TestConfirmSend_Esc_ClearsLabel(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmSend
	m.confirmSendPaneID = "%5"
	m.confirmSendKey = "y"
	m.confirmSendLabel = "Plan approved"
	m.statusMsg = "Send 'y' to agent?"

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := result.(model)
	if updated.confirmSendLabel != "" {
		t.Errorf("expected confirmSendLabel cleared, got %q", updated.confirmSendLabel)
	}
}

func TestEditorKey_SetsInFlightStatus(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.cfg.Editor = "vim"
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	updated := result.(model)
	if updated.statusMsg != "Opening editor..." {
		t.Errorf("expected 'Opening editor...', got %q", updated.statusMsg)
	}
	if updated.statusIsError {
		t.Error("expected statusIsError=false for in-flight message")
	}
}

func TestDiffKey_SetsInFlightStatus(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated := result.(model)
	if updated.statusMsg != "Loading diff..." {
		t.Errorf("expected 'Loading diff...', got %q", updated.statusMsg)
	}
}

func TestPRKey_SetsInFlightStatus(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp", Branch: "feat/test"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	updated := result.(model)
	if updated.statusMsg != "Opening PR..." {
		t.Errorf("expected 'Opening PR...', got %q", updated.statusMsg)
	}
}

// -- PhantomFilter tests --

func TestPhantomFilter_SwallowsDestructiveKeyDuringEscapeCooldown(t *testing.T) {
	destructiveKeys := []tea.KeyPressMsg{
		{Code: 'x', Text: "x"},
		{Code: tea.KeyEnter},
		{Code: 'r', Text: "r"},
		{Code: 'm', Text: "m"},
		{Code: 'y', Text: "y"},
		{Code: 'n', Text: "n"},
		{Code: '1', Text: "1"},
		{Code: '5', Text: "5"},
		{Code: '9', Text: "9"},
	}
	for _, key := range destructiveKeys {
		t.Run(key.String(), func(t *testing.T) {
			m := newTestModelWithAgents()
			m.lastEscapeAt = time.Now() // mouse event just happened
			result := PhantomFilter(m, key)
			if result != nil {
				t.Errorf("expected nil (swallowed) for destructive key %q during escape cooldown", key.String())
			}
		})
	}
}

func TestPhantomFilter_AllowsNavigationKeyDuringEscapeCooldown(t *testing.T) {
	navKeys := []tea.KeyPressMsg{
		{Code: 'j', Text: "j"},
		{Code: 'k', Text: "k"},
		{Code: tea.KeyTab},
		{Code: 'q', Text: "q"},
		{Code: 'c', Text: "c"},
		{Code: 'h', Text: "h"},
	}
	for _, key := range navKeys {
		t.Run(key.String(), func(t *testing.T) {
			m := newTestModelWithAgents()
			m.lastEscapeAt = time.Now()
			result := PhantomFilter(m, key)
			if result == nil {
				t.Errorf("navigation key %q should NOT be swallowed", key.String())
			}
		})
	}
}

func TestPhantomFilter_SwallowsKeyDuringModeResetCooldown(t *testing.T) {
	m := newTestModelWithAgents()
	m.modeResetAt = time.Now() // just transitioned back to normal

	result := PhantomFilter(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if result != nil {
		t.Error("enter should be swallowed during mode reset cooldown")
	}
}

func TestPhantomFilter_AllowsKeyAfterCooldownExpires(t *testing.T) {
	m := newTestModelWithAgents()
	m.lastEscapeAt = time.Now().Add(-100 * time.Millisecond) // well past 50ms cooldown

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result := PhantomFilter(m, msg)
	if result == nil {
		t.Error("key should pass through after cooldown expires")
	}
}

func TestPhantomFilter_SwallowsConfirmKeyDuringConfirmCooldown(t *testing.T) {
	tests := []struct {
		name string
		mode int
		key  tea.KeyPressMsg
	}{
		{"close_y", modeConfirmClose, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"merge_y", modeConfirmMerge, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"send_enter", modeConfirmSend, tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"jump_y", modeConfirmJump, tea.KeyPressMsg{Code: 'y', Text: "y"}},
		{"jump_enter", modeConfirmJump, tea.KeyPressMsg{Code: tea.KeyEnter}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(testConfig(t.TempDir()), nil)
			m.mode = tt.mode
			m.confirmEnteredAt = time.Now() // just entered — within cooldown
			result := PhantomFilter(m, tt.key)
			if result != nil {
				t.Errorf("confirm key should be swallowed during cooldown in mode %d", tt.mode)
			}
		})
	}
}

func TestPhantomFilter_AllowsConfirmKeyAfterCooldown(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.mode = modeConfirmMerge
	m.confirmEnteredAt = time.Now().Add(-time.Second) // well past cooldown

	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	result := PhantomFilter(m, msg)
	if result == nil {
		t.Error("confirm key should pass through after cooldown")
	}
}

func TestPhantomFilter_PassesThroughNonKeyMessages(t *testing.T) {
	m := newTestModelWithAgents()
	m.lastEscapeAt = time.Now()

	// Mouse messages should pass through even during cooldown
	msg := tea.MouseMotionMsg{}
	result := PhantomFilter(m, msg)
	if result == nil {
		t.Error("non-key messages should always pass through")
	}
}

func TestPhantomFilter_PassesThroughNonNormalModeKeys(t *testing.T) {
	// In non-normal, non-confirm modes (e.g. reply), destructive keys pass through
	// because they're being used as text input, not actions.
	m := newTestModelWithAgents()
	m.mode = modeReply
	m.lastEscapeAt = time.Now()

	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
	result := PhantomFilter(m, msg)
	if result == nil {
		t.Error("keys in modeReply should not be phantom-filtered")
	}
}

func TestReplyMode_ViewportScrollsToBottom(t *testing.T) {
	// When typing in reply mode, the viewport must scroll to the bottom
	// so that the reply input (including wrapped continuation lines) stays visible.
	m := newTestModelWithAgents()
	m.agents[0].TmuxPaneID = "%5"
	m.agents[0].State = "done"
	m.buildTree()
	selectFirstAgent(&m)
	m.width = 80
	m.height = 40
	m.resizeViewports()

	// Add a long assistant message to push the reply input below the viewport
	longMsg := strings.Repeat("This is a long assistant message line.\n", 50)
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: longMsg},
	}

	// Enter reply mode and scroll to bottom (as the real code does)
	m.mode = modeReply
	m.textInput.Focus()
	m.updateRightContent()
	m.messageVP.GotoBottom()

	// Verify the content is taller than the viewport (precondition)
	vpHeight := m.messageVP.Height()
	totalLines := m.messageVP.TotalLineCount()
	if totalLines <= vpHeight {
		t.Fatalf("precondition: content (%d lines) must exceed viewport height (%d)", totalLines, vpHeight)
	}

	// Now scroll UP to simulate the viewport being away from bottom
	// (this simulates what happens when content grows without auto-scroll)
	m.messageVP.SetYOffset(0)
	if m.messageVP.AtBottom() {
		t.Fatal("precondition: viewport should not be at bottom after scrolling to top")
	}

	// Type enough text to cause wrapping
	longReply := strings.Repeat("a", 200)
	m.textInput.SetValue(longReply)
	m.textInput.SetCursor(len(longReply))

	// Simulate a key press to trigger the update path in reply mode
	result, _ := m.handleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	rm := result.(model)

	// The viewport should be at the bottom so the reply is visible
	if !rm.messageVP.AtBottom() {
		t.Error("viewport should scroll to bottom in reply mode after typing, so reply text is visible")
	}
}

func TestOpenWorktreeWindowWithWorktreeCwd(t *testing.T) {
	m := newTestModelWithAgents()
	m.agents[0].Cwd = "/home/user/code/myrepo"
	m.agents[0].WorktreeCwd = "/home/user/code/worktrees/myrepo/feature-branch"
	m.agents[0].Session = "main"
	m.agents[0].Branch = "feat/my-feature"
	m.buildTree()
	selectFirstAgent(&m)

	msg := tea.KeyPressMsg{Code: 'o', Text: "o"}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	if rm.statusMsg != "Opening window..." {
		t.Errorf("expected status 'Opening window...', got %q", rm.statusMsg)
	}
	if cmd == nil {
		t.Error("expected a command to be returned for opening worktree window")
	}
}

func TestOpenWorktreeWindowWithCwdOnly(t *testing.T) {
	m := newTestModelWithAgents()
	m.agents[0].Cwd = "/home/user/code/myrepo"
	m.agents[0].WorktreeCwd = ""
	m.agents[0].Session = "main"
	m.agents[0].Branch = "main"
	m.buildTree()
	selectFirstAgent(&m)

	msg := tea.KeyPressMsg{Code: 'o', Text: "o"}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	if rm.statusMsg != "Opening window..." {
		t.Errorf("expected status 'Opening window...', got %q", rm.statusMsg)
	}
	if cmd == nil {
		t.Error("expected a command to be returned for opening cwd window")
	}
}

func TestOpenWorktreeWindowNoTmux(t *testing.T) {
	m := newTestModelWithAgents()
	m.tmuxAvailable = false
	m.agents[0].Cwd = "/home/user/code/myrepo"
	m.buildTree()
	selectFirstAgent(&m)

	msg := tea.KeyPressMsg{Code: 'o', Text: "o"}
	result, _ := m.handleKey(msg)
	rm := result.(model)

	if !rm.statusIsError {
		t.Error("expected error status when tmux is not available")
	}
	if !strings.Contains(rm.statusMsg, "tmux") {
		t.Errorf("expected tmux error message, got %q", rm.statusMsg)
	}
}

func TestOpenWorktreeWindowNoAgent(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.tmuxAvailable = true
	m.buildTree()

	msg := tea.KeyPressMsg{Code: 'o', Text: "o"}
	result, cmd := m.handleKey(msg)
	rm := result.(model)

	// No agent selected — should be a no-op
	if rm.statusMsg != "" {
		t.Errorf("expected no status message, got %q", rm.statusMsg)
	}
	if cmd != nil {
		t.Error("expected no command when no agent is selected")
	}
}

func TestOpenWorktreeMsgSuccess(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	result, _ := m.Update(openWorktreeMsg{err: nil, dir: "/tmp/worktree"})
	rm := result.(model)

	if rm.statusIsError {
		t.Error("expected non-error status on success")
	}
	if !strings.Contains(rm.statusMsg, "/tmp/worktree") {
		t.Errorf("expected status to contain dir, got %q", rm.statusMsg)
	}
}

func TestOpenWorktreeMsgError(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	result, _ := m.Update(openWorktreeMsg{err: fmt.Errorf("new-window failed"), dir: "/tmp/worktree"})
	rm := result.(model)

	if !rm.statusIsError {
		t.Error("expected error status on failure")
	}
	if !strings.Contains(rm.statusMsg, "new-window failed") {
		t.Errorf("expected error message in status, got %q", rm.statusMsg)
	}
}

// --- Paste (Cmd+V / bracketed paste) tests ---

func TestPasteInReplyMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeReply
	m.textInput.Focus()

	msg := tea.PasteMsg{Content: "pasted text"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.textInput.Value() != "pasted text" {
		t.Errorf("expected textInput to contain 'pasted text', got %q", rm.textInput.Value())
	}
}

func TestPasteInCreateFolderMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateFolder
	m.textInput.Focus()

	msg := tea.PasteMsg{Content: "/some/path"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.textInput.Value() != "/some/path" {
		t.Errorf("expected textInput to contain '/some/path', got %q", rm.textInput.Value())
	}
}

func TestPasteInCreateMessageMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateMessage
	m.textInput.Focus()

	msg := tea.PasteMsg{Content: "agent message"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.textInput.Value() != "agent message" {
		t.Errorf("expected textInput to contain 'agent message', got %q", rm.textInput.Value())
	}
}

func TestPasteInDiffFilterMode(t *testing.T) {
	m := newTestModelWithAgents()
	m.diffVisible = true
	m.diffFilterActive = true
	m.diffFilterInput.Focus()

	msg := tea.PasteMsg{Content: "filter"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.diffFilterInput.Value() != "filter" {
		t.Errorf("expected diffFilterInput to contain 'filter', got %q", rm.diffFilterInput.Value())
	}
	if rm.diffFilterText != "filter" {
		t.Errorf("expected diffFilterText to be 'filter', got %q", rm.diffFilterText)
	}
}

func TestPasteInNormalModeIsNoop(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeNormal

	msg := tea.PasteMsg{Content: "should be ignored"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.textInput.Value() != "" {
		t.Errorf("expected textInput to be empty in normal mode, got %q", rm.textInput.Value())
	}
}

func TestPasteInCreateSkillModeIsNoop(t *testing.T) {
	m := newTestModelWithAgents()
	m.mode = modeCreateSkill

	msg := tea.PasteMsg{Content: "should be ignored"}
	result, _ := m.Update(msg)
	rm := result.(model)

	if rm.textInput.Value() != "" {
		t.Errorf("expected textInput to be empty in skill selection mode, got %q", rm.textInput.Value())
	}
}

func TestGuardedKey_NotSwallowedAfterMouseEvent(t *testing.T) {
	// After a mouse event, guarded keys like Enter should NOT be swallowed
	// by PhantomFilter. Mouse events in bubbletea v2 are fully parsed by
	// the framework — no phantom keys can leak from mouse escape sequences.
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()

	// Simulate a mouse event by calling handleMouse
	result, _ := m.handleMouse(tea.MouseClickMsg{})
	m = result.(model)

	// Now press Enter — it should pass through PhantomFilter
	filtered := PhantomFilter(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if filtered == nil {
		t.Error("Enter after mouse event should NOT be swallowed — mouse events in bubbletea v2 are fully parsed")
	}
}

func TestGuardedKey_NotSwallowedAfterMouseScroll(t *testing.T) {
	// Simulates the tmux-switch scenario: user scrolls with mouse (works),
	// then presses a guarded key (should also work, not be swallowed).
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()

	// Simulate multiple rapid mouse scroll events
	for i := 0; i < 5; i++ {
		result, _ := m.handleMouse(tea.MouseWheelMsg{})
		m = result.(model)
	}

	// Guarded keys should still work
	for _, key := range []string{"x", "enter", "r", "m"} {
		var msg tea.KeyPressMsg
		if key == "enter" {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		} else {
			msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
		}
		filtered := PhantomFilter(m, msg)
		if filtered == nil {
			t.Errorf("key %q should NOT be swallowed after mouse events", key)
		}
	}
}

func TestEnterKey_TrustDetected_JumpsDirectly(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.trustDetected = true
	m.spawningTarget = "main:2.0"
	m.mode = modeNormal
	m.startupDone = true

	_, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Should return a jump command (not nil, and not enter modeConfirmJump)
	if cmd == nil {
		t.Fatal("expected a jump command, got nil")
	}
	if m.mode == modeConfirmJump {
		t.Error("should skip confirm dialog and jump directly when trust is detected")
	}
}

func TestEnterKey_NoTrust_NormalBehavior(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.trustDetected = false
	m.spawningTarget = ""
	m.mode = modeNormal
	m.startupDone = true
	m.agents = []domain.Agent{{Target: "main:1.0", State: "running", TmuxPaneID: "%5"}}
	m.buildTree()
	// Select the agent (index 1 because index 0 is group header)
	m.selected = 1

	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := result.(model)

	// Should enter confirm mode as usual
	if um.mode != modeConfirmJump {
		t.Errorf("expected modeConfirmJump, got %d", um.mode)
	}
}
