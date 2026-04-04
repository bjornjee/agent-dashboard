package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/zsuggest"
)

func TestBuildTree_DismissedSubagentsHidden(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "aaa", AgentType: "Explore", Description: "first"},
		{AgentID: "bbb", AgentType: "Bash", Description: "second"},
		{AgentID: "ccc", AgentType: "Plan", Description: "third"},
	}

	// No dismissals — all 4 nodes (1 parent + 3 subs)
	m.buildTree()
	if len(m.treeNodes) != 4 {
		t.Fatalf("expected 4 tree nodes, got %d", len(m.treeNodes))
	}

	// Dismiss "bbb"
	m.dismissed["main:1.0:bbb"] = true
	m.buildTree()
	if len(m.treeNodes) != 3 {
		t.Fatalf("expected 3 tree nodes after dismiss, got %d", len(m.treeNodes))
	}

	// Verify dismissed node is not present
	for _, node := range m.treeNodes {
		if node.Sub != nil && node.Sub.AgentID == "bbb" {
			t.Error("dismissed subagent 'bbb' should not appear in tree")
		}
	}
}

func TestBuildTree_CollapsedHidesSubs(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "aaa", AgentType: "Explore", Description: "first"},
	}

	m.collapsed["main:1.0"] = true
	m.buildTree()
	if len(m.treeNodes) != 1 {
		t.Fatalf("expected 1 tree node when collapsed, got %d", len(m.treeNodes))
	}
}

func TestCurrentTool_InAgentStruct(t *testing.T) {
	// Verify CurrentTool field is available and serializes correctly
	agent := domain.Agent{
		Target:      "a:0.1",
		State:       "running",
		CurrentTool: "Bash",
	}
	if agent.CurrentTool != "Bash" {
		t.Errorf("expected CurrentTool=Bash, got %q", agent.CurrentTool)
	}
}

func TestNextParentAgent(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "aaa", AgentType: "Explore", Description: "sub1"},
		{AgentID: "bbb", AgentType: "Bash", Description: "sub2"},
	}
	m.buildTree()
	// Tree: [parent0, sub-aaa, sub-bbb, parent1]

	// From parent0 (idx 0), next parent should be parent1 (idx 3)
	m.selected = 0
	next := m.nextParentIndex(1)
	if next != 3 {
		t.Errorf("from parent0, expected next parent at index 3, got %d", next)
	}

	// From sub-aaa (idx 1), next parent should be parent1 (idx 3)
	m.selected = 1
	next = m.nextParentIndex(1)
	if next != 3 {
		t.Errorf("from sub-aaa, expected next parent at index 3, got %d", next)
	}

	// From parent1 (idx 3), next parent going down should stay at 3 (no more parents)
	m.selected = 3
	next = m.nextParentIndex(1)
	if next != 3 {
		t.Errorf("from last parent, expected to stay at 3, got %d", next)
	}

	// From parent1 (idx 3), prev parent should be parent0 (idx 0)
	m.selected = 3
	next = m.nextParentIndex(-1)
	if next != 0 {
		t.Errorf("from parent1, expected prev parent at index 0, got %d", next)
	}
}

func TestCloseResult_TriggersPruneDead(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
		{Target: "main:2.1", Window: 2, Pane: 1, State: "running"},
	}
	m.buildTree()

	// Simulate a successful close result
	result, cmd := m.Update(closeResultMsg{err: nil})
	_ = result

	if cmd == nil {
		t.Fatal("expected commands after closeResultMsg, got nil")
	}

	// Execute the batch to get individual commands
	// The batch should produce both loadState and pruneDead messages
	msgs := executeBatch(t, cmd)

	hasStateUpdate := false
	hasPruneDead := false
	for _, msg := range msgs {
		switch msg.(type) {
		case stateUpdatedMsg:
			hasStateUpdate = true
		case pruneDeadMsg:
			hasPruneDead = true
		}
	}

	if !hasStateUpdate {
		t.Error("closeResultMsg should trigger loadState (stateUpdatedMsg)")
	}
	if !hasPruneDead {
		t.Error("closeResultMsg should trigger pruneDead (pruneDeadMsg)")
	}
}

func TestWaitingMessage_ShowsLastAssistantMessage(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true
	m.capturedLines = []string{
		"Bash command",
		"",
		"   mkdir -p /tmp/.vscode",
		"   Create .vscode directory",
		"",
		" Claude requested permissions to edit /tmp/.vscode",
		"",
		" Do you want to proceed?",
		" > 1. Yes",
		"   2. Yes, and always allow",
		"   3. No",
	}
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "Let me create that directory.", Timestamp: "2026-03-29T10:00:00Z"},
	}

	// Test waitingMessageContent directly (viewport clipping may hide content)
	content := m.waitingMessageContent()

	// Should always show the last assistant message, not tmux capture
	if !strings.Contains(content, "Let me create that directory") {
		t.Errorf("waiting message should show last assistant message, got:\n%s", content)
	}
	// Should still show the reply hint (question state shows "r to reply, enter to jump")
	if !strings.Contains(content, "r to reply") {
		t.Errorf("waiting message should still show reply hint, got:\n%s", content)
	}
}

func TestWaitingMessage_FallsBackToConversation(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true
	m.capturedLines = nil // no tmux capture yet
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "What should I do?", Timestamp: "2026-03-29T10:00:00Z"},
	}

	content := m.waitingMessageContent()

	// With no capture, should fall back to conversation text
	if !strings.Contains(content, "What should I do") {
		t.Error("waiting message should fall back to conversation when no tmux capture")
	}
}

func TestReplyMode_ShowsInputBar(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "What should I do?", Timestamp: "2026-03-29T10:00:00Z"},
	}
	m.updateRightContent()

	// Enter reply mode
	result, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = result.(model)

	if m.mode != modeReply {
		t.Fatalf("expected modeReply, got %d", m.mode)
	}

	// The message viewport should contain "Reply:" after entering reply mode
	content := m.messageVP.View()
	if !strings.Contains(content, "Reply:") {
		t.Error("message viewport should show 'Reply:' input bar after entering reply mode")
	}
}

func TestReplyMode_KeystrokesUpdateViewport(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "What should I do?", Timestamp: "2026-03-29T10:00:00Z"},
	}
	m.updateRightContent()

	// Enter reply mode
	result, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = result.(model)

	// Type "hello"
	for _, ch := range "hello" {
		result, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = result.(model)
	}

	// The viewport should contain the typed text
	content := m.messageVP.View()
	if !strings.Contains(content, "hello") {
		t.Error("message viewport should show typed text 'hello' during reply mode")
	}
}

func TestReplyMode_EscRestoresView(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "What should I do?", Timestamp: "2026-03-29T10:00:00Z"},
	}
	m.updateRightContent()

	// Enter reply mode
	result, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = result.(model)

	// Press esc
	result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %d", m.mode)
	}

	// Viewport should show the normal prompt hint, not the reply input
	content := m.messageVP.View()
	if strings.Contains(content, "Reply:") {
		t.Error("message viewport should not show 'Reply:' after esc")
	}
}

func TestReplyMode_PlanStateNoPrematureReplySent(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "plan", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true

	// Enter reply mode on a plan-state agent (presses "r")
	result, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = result.(model)

	if m.mode != modeReply {
		t.Fatalf("expected modeReply, got %d", m.mode)
	}

	// Process the sendRawKey("3") result — this simulates the raw key completing
	result, _ = m.Update(rawKeySentMsg{err: nil})
	m = result.(model)

	// The status should NOT say "Reply sent" — the user hasn't typed yet
	if m.statusMsg == "Reply sent" {
		t.Error("status should not be 'Reply sent' after raw key send; user hasn't typed yet")
	}
}

func TestReplyMode_RawKeySendFailureShowsError(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "plan", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true

	// Enter reply mode on plan-state agent
	result, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = result.(model)

	// Simulate raw key send failure
	result, _ = m.Update(rawKeySentMsg{err: fmt.Errorf("pane %s no longer exists", "main:2.0")})
	m = result.(model)

	if !strings.Contains(m.statusMsg, "failed") {
		t.Errorf("expected error status for raw key failure, got %q", m.statusMsg)
	}
	// Should exit reply mode on failure since the pane isn't available
	if m.mode != modeNormal {
		t.Errorf("expected modeNormal after raw key failure, got %d", m.mode)
	}
}

func TestReplyMode_SendReplyFailureShowsError(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question", Cwd: "/tmp"},
	}
	m.buildTree()
	m.tmuxAvailable = true

	// Simulate a reply failure
	result, _ := m.Update(sendResultMsg{err: fmt.Errorf("pane main:2.0 no longer exists")})
	m = result.(model)

	if !strings.Contains(m.statusMsg, "Reply failed") {
		t.Errorf("expected 'Reply failed' status, got %q", m.statusMsg)
	}
}

func TestFindWindowForRepo_MatchesByFolder(t *testing.T) {
	agents := []domain.Agent{
		{Target: "main:1.0", Session: "main", Window: 1, Pane: 0, Cwd: "/home/user/code/skills"},
		{Target: "main:2.0", Session: "main", Window: 2, Pane: 0, Cwd: "/home/user/code/other"},
	}

	sw, found := findWindowForRepo(agents, "/home/user/code/skills", "%0")
	if !found {
		t.Fatal("expected to find window for matching folder")
	}
	if sw != "main:1" {
		t.Errorf("expected session:window main:1, got %q", sw)
	}
}

func TestFindWindowForRepo_NoMatch(t *testing.T) {
	agents := []domain.Agent{
		{Target: "main:1.0", Session: "main", Window: 1, Pane: 0, Cwd: "/home/user/code/skills"},
	}

	_, found := findWindowForRepo(agents, "/home/user/code/newrepo", "%0")
	if found {
		t.Error("expected no match for different folder")
	}
}

func TestFindWindowForRepo_EmptyAgents(t *testing.T) {
	_, found := findWindowForRepo(nil, "/home/user/code/skills", "%0")
	if found {
		t.Error("expected no match with empty agents")
	}
}

func TestCreateSessionMsg_Success(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.selfPaneID = "%0"
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()

	m.statusMsg = "spawning" // set by keys.go before createSession fires
	result, _ := m.Update(createSessionMsg{target: "main:2.0", err: nil})
	rm := result.(model)

	// After successful create, mode stays normal (pane is selected directly)
	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after successful create, got %d", rm.mode)
	}
	// statusMsg stays as "spawning" — it expires naturally via 3s auto-clear
	if rm.statusMsg != "spawning" {
		t.Errorf("expected spawning status to persist, got %q", rm.statusMsg)
	}
}

func TestCreateSessionMsg_Error(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.selfPaneID = "%0"
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true

	result, _ := m.Update(createSessionMsg{target: "", err: fmt.Errorf("8-pane limit reached")})
	rm := result.(model)

	if rm.mode != modeNormal {
		t.Errorf("expected modeNormal after failed create, got %d", rm.mode)
	}
	if !strings.Contains(rm.statusMsg, "8-pane limit") {
		t.Errorf("expected error in statusMsg, got %q", rm.statusMsg)
	}
}

func TestCreateFolderMode_SuggestionsShown(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.pathExists = func(string) bool { return true } // accept all fake paths in test
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()

	// Pre-load z entries
	m.zEntries = []zsuggest.Entry{
		zsuggest.Entry{Path: "/Users/bjornjee/Code/skills", Rank: 100, Timestamp: 1774000000},
		zsuggest.Entry{Path: "/Users/bjornjee/Code/other", Rank: 50, Timestamp: 1773000000},
		zsuggest.Entry{Path: "/tmp/unrelated", Rank: 10, Timestamp: 1770000000},
	}

	// Enter create folder mode
	result, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = result.(model)

	// Type partial path
	for _, ch := range "skills" {
		result, _ = m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = result.(model)
	}

	// Should have suggestions filtered to match "skills"
	if len(m.suggestions) == 0 {
		t.Fatal("expected suggestions matching 'skills'")
	}
	if !strings.Contains(m.suggestions[0], "skills") {
		t.Errorf("expected first suggestion to contain 'skills', got %q", m.suggestions[0])
	}
}

func TestCreateFolderMode_TabAcceptsSuggestion(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()

	m.zEntries = []zsuggest.Entry{
		zsuggest.Entry{Path: "/Users/bjornjee/Code/skills", Rank: 100, Timestamp: 1774000000},
	}

	// Enter create folder mode and type partial
	m.mode = modeCreateFolder
	m.textInput.Focus()
	m.textInput.SetValue("ski")
	m.suggestions = zsuggest.FilterZSuggestions("ski", m.zEntries, nil)

	// Press tab to accept
	result, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	rm := result.(model)

	if rm.textInput.Value() != "/Users/bjornjee/Code/skills" {
		t.Errorf("expected tab to accept suggestion, got %q", rm.textInput.Value())
	}
	if len(rm.suggestions) != 0 {
		t.Error("expected suggestions to be cleared after tab accept")
	}
}

func TestCreateFolderMode_SuggestionsInView(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()

	m.zEntries = []zsuggest.Entry{
		zsuggest.Entry{Path: "/Users/bjornjee/Code/skills", Rank: 100, Timestamp: 1774000000},
		zsuggest.Entry{Path: "/Users/bjornjee/Code/other", Rank: 50, Timestamp: 1773000000},
	}

	// Enter create folder mode
	m.mode = modeCreateFolder
	m.textInput.Focus()
	m.textInput.SetValue("Code")
	m.suggestions = zsuggest.FilterZSuggestions("Code", m.zEntries, nil)
	m.updateRightContent()

	content := m.messageVP.View()
	if !strings.Contains(content, "skills") {
		t.Error("viewport should show suggestion paths matching query")
	}
}

func TestStateUpdate_PrunesAllMaps(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.selfPaneID = "%0"
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "done"},
	}
	m.buildTree()

	// Populate maps for both agents
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{{AgentID: "sub1"}}
	m.agentSubagents["main:2.0"] = []domain.SubagentInfo{{AgentID: "sub2"}}
	m.collapsed["main:1.0"] = true
	m.collapsed["main:2.0"] = false
	m.dismissed["main:1.0:sub1"] = true
	m.dismissed["main:2.0:sub2"] = true

	// Simulate state update where main:2.0 is removed
	sf := domain.StateFile{
		Agents: map[string]domain.Agent{
			"main:1.0": {Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		},
	}
	result, _ := m.Update(stateUpdatedMsg{state: sf})
	rm := result.(model)

	// main:1.0 maps should survive
	if _, ok := rm.agentSubagents["main:1.0"]; !ok {
		t.Error("agentSubagents for main:1.0 should survive")
	}

	// main:2.0 maps should be pruned
	if _, ok := rm.agentSubagents["main:2.0"]; ok {
		t.Error("agentSubagents for main:2.0 should be pruned")
	}
	if _, ok := rm.collapsed["main:2.0"]; ok {
		t.Error("collapsed for main:2.0 should be pruned")
	}
	if _, ok := rm.dismissed["main:2.0:sub2"]; ok {
		t.Error("dismissed for main:2.0:sub2 should be pruned")
	}
	// dismissed for main:1.0 should survive
	if _, ok := rm.dismissed["main:1.0:sub1"]; !ok {
		t.Error("dismissed for main:1.0:sub1 should survive")
	}
}

func TestPlanToggle(t *testing.T) {
	setup := func() model {
		m := NewModel(testConfig(""), nil)
		m.width = 120
		m.height = 40
		m.resizeViewports()
		m.agents = []domain.Agent{
			{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
		}
		m.buildTree()
		m.tmuxAvailable = true
		m.planContent = "# Test Plan\n\n## Steps\n1. Do A"
		m.renderedPlan = renderPlanMarkdown(m.planContent, m.rightWidth-4)
		m.planVisible = true
		m.updateRightContent()
		return m
	}

	t.Run("p toggles plan off", func(t *testing.T) {
		m := setup()
		if !m.planVisible {
			t.Fatal("planVisible should start true")
		}
		result, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
		rm := result.(model)
		if rm.planVisible {
			t.Error("p should toggle planVisible off")
		}
	})

	t.Run("p toggles plan back on", func(t *testing.T) {
		m := setup()
		m.planVisible = false
		result, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
		rm := result.(model)
		if !rm.planVisible {
			t.Error("p should toggle planVisible on when plan content exists")
		}
	})

	t.Run("p ignored when no plan", func(t *testing.T) {
		m := setup()
		m.planContent = ""
		m.renderedPlan = ""
		m.planVisible = false
		result, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
		rm := result.(model)
		if rm.planVisible {
			t.Error("p should not enable planVisible when there is no plan content")
		}
	})

	t.Run("navigation clears planVisible", func(t *testing.T) {
		m := setup()
		// Add second agent for navigation
		m.agents = append(m.agents, domain.Agent{Target: "main:2.0", Window: 2, Pane: 0, State: "running", Cwd: "/tmp"})
		m.buildTree()
		if !m.planVisible {
			t.Fatal("planVisible should be true before navigation")
		}
		result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		rm := result.(model)
		if rm.planVisible {
			t.Error("navigation should reset planVisible")
		}
	})

	t.Run("p ignored on subagent", func(t *testing.T) {
		m := setup()
		m.planVisible = false // start off
		m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
			{AgentID: "sub1", AgentType: "Explore", Description: "test"},
		}
		m.buildTree()
		m.selected = 1 // select subagent
		result, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
		rm := result.(model)
		if rm.planVisible {
			t.Error("p should not toggle plan when subagent is selected")
		}
	})

	t.Run("J scrolls plan down one line", func(t *testing.T) {
		m := setup()
		m.messageVP.SetContent(scrollableContent(100))
		before := m.messageVP.YOffset()
		result, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
		rm := result.(model)
		if rm.messageVP.YOffset() != before+1 {
			t.Errorf("J should scroll plan down by 1 line, got offset %d (was %d)", rm.messageVP.YOffset(), before)
		}
	})

	t.Run("K scrolls plan up one line", func(t *testing.T) {
		m := setup()
		m.messageVP.SetContent(scrollableContent(100))
		// Scroll down first so we can scroll up
		m.messageVP.ScrollDown(5)
		before := m.messageVP.YOffset()
		result, _ := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
		rm := result.(model)
		if rm.messageVP.YOffset() != before-1 {
			t.Errorf("K should scroll plan up by 1 line, got offset %d (was %d)", rm.messageVP.YOffset(), before)
		}
	})

	t.Run("J ignored when plan not visible", func(t *testing.T) {
		m := setup()
		m.planVisible = false
		m.updateRightContent()
		before := m.messageVP.YOffset()
		result, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
		rm := result.(model)
		if rm.messageVP.YOffset() != before {
			t.Error("J should not scroll messageVP when plan is not visible")
		}
	})

	t.Run("K ignored when plan not visible", func(t *testing.T) {
		m := setup()
		m.planVisible = false
		m.updateRightContent()
		before := m.messageVP.YOffset()
		result, _ := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
		rm := result.(model)
		if rm.messageVP.YOffset() != before {
			t.Error("K should not scroll messageVP when plan is not visible")
		}
	})
}

func TestPlanMsg_NoAutoShow(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
	}
	m.buildTree()

	// planMsg with content should NOT auto-show (live/output is default)
	result, _ := m.Update(planMsg{content: "# Plan\n\n## Steps"})
	rm := result.(model)
	if rm.planVisible {
		t.Error("planMsg should not auto-show plan — live/output is default")
	}
	if rm.renderedPlan == "" {
		t.Error("planMsg should populate renderedPlan for when user presses p")
	}

	// User toggles on manually, then planMsg update should preserve that
	rm.planVisible = true
	result2, _ := rm.Update(planMsg{content: "# Updated Plan"})
	rm2 := result2.(model)
	if !rm2.planVisible {
		t.Error("planMsg should preserve planVisible when already true")
	}

	// Empty planMsg should clear everything
	result3, _ := rm2.Update(planMsg{content: ""})
	rm3 := result3.(model)
	if rm3.planVisible {
		t.Error("empty planMsg should clear planVisible")
	}
	if rm3.renderedPlan != "" {
		t.Error("empty planMsg should clear renderedPlan")
	}
}

func TestPlanVisible_AutoDismissOnStateChange(t *testing.T) {
	t.Run("cleared when agent leaves plan state", func(t *testing.T) {
		m := NewModel(testConfig(""), nil)
		m.width = 120
		m.height = 40
		m.resizeViewports()
		m.agents = []domain.Agent{
			{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", Cwd: "/tmp"},
		}
		m.buildTree()
		m.planVisible = true
		m.planContent = "# My Plan"
		m.renderedPlan = "rendered"

		// Agent transitions to running
		result, _ := m.Update(stateUpdatedMsg{state: domain.StateFile{
			Agents: map[string]domain.Agent{
				"main:1.0": {Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
			},
		}})
		rm := result.(model)
		if rm.planVisible {
			t.Error("planVisible should be auto-cleared when agent leaves plan state")
		}
	})

	t.Run("preserved when agent is still in plan state", func(t *testing.T) {
		m := NewModel(testConfig(""), nil)
		m.width = 120
		m.height = 40
		m.resizeViewports()
		m.agents = []domain.Agent{
			{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", Cwd: "/tmp"},
		}
		m.buildTree()
		m.planVisible = true
		m.planContent = "# My Plan"
		m.renderedPlan = "rendered"

		result, _ := m.Update(stateUpdatedMsg{state: domain.StateFile{
			Agents: map[string]domain.Agent{
				"main:1.0": {Target: "main:1.0", Window: 1, Pane: 0, State: "plan", Cwd: "/tmp"},
			},
		}})
		rm := result.(model)
		if !rm.planVisible {
			t.Error("planVisible should be preserved when agent is still in plan state")
		}
	})

	t.Run("cleared when selected agent disappears", func(t *testing.T) {
		m := NewModel(testConfig(""), nil)
		m.width = 120
		m.height = 40
		m.resizeViewports()
		m.agents = []domain.Agent{
			{Target: "main:1.0", Window: 1, Pane: 0, State: "plan", Cwd: "/tmp"},
		}
		m.buildTree()
		m.planVisible = true
		m.planContent = "# My Plan"

		// State update with no agents
		result, _ := m.Update(stateUpdatedMsg{state: domain.StateFile{
			Agents: map[string]domain.Agent{},
		}})
		rm := result.(model)
		if rm.planVisible {
			t.Error("planVisible should be cleared when selected agent disappears")
		}
	})
}

// scrollableContent returns n lines of text suitable for testing viewport scrolling.
func scrollableContent(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// executeBatch runs a tea.Cmd (expected to be a Batch) and collects messages.
func executeBatch(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	// tea.Batch returns a tea.BatchMsg ([]tea.Cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			if c != nil {
				msgs = append(msgs, c())
			}
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestPlanFlow_EndToEnd(t *testing.T) {
	// Setup: create temp dirs with JSONL and plan file
	dir := t.TempDir()
	slug := "-test-project"
	projDir := fmt.Sprintf("%s/projects/%s", dir, slug)
	plansDir := fmt.Sprintf("%s/plans", dir)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionID := "test-session-id"
	planSlugName := "my-test-plan"
	planContent := "# Test Plan\n\n## Steps\n1. Do thing A\n2. Do thing B"

	// Write JSONL with slug field
	jsonl := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T10:00:00Z","slug":"%s"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]},"timestamp":"2026-03-28T10:00:01Z","slug":"%s"}
`, planSlugName, planSlugName)
	if err := os.WriteFile(fmt.Sprintf("%s/%s.jsonl", projDir, sessionID), []byte(jsonl), 0644); err != nil {
		t.Fatal(err)
	}

	// Write plan file
	if err := os.WriteFile(fmt.Sprintf("%s/%s.md", plansDir, planSlugName), []byte(planContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify ReadPlanSlug finds the slug
	gotSlug := conversation.ReadPlanSlug(projDir, sessionID)
	if gotSlug != planSlugName {
		t.Fatalf("ReadPlanSlug: expected %q, got %q", planSlugName, gotSlug)
	}

	// Verify ReadPlanContent reads the file
	gotContent := conversation.ReadPlanContent(plansDir, planSlugName)
	if gotContent != planContent {
		t.Fatalf("ReadPlanContent: expected %q, got %q", planContent, gotContent)
	}

	// Now test the model flow
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	// Simulate state update with an agent
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/test/project", SessionID: sessionID},
	}
	m.buildTree()

	// Simulate planMsg with content
	result, _ := m.Update(planMsg{content: planContent})
	rm := result.(model)

	if rm.planContent != planContent {
		t.Errorf("planContent: expected %q, got %q", planContent, rm.planContent)
	}
	if rm.planVisible {
		t.Error("planMsg should not auto-show — live/output is default")
	}
	if rm.renderedPlan == "" {
		t.Error("planMsg should populate renderedPlan with glamour output")
	}

	// The plan content should contain parts of the plan
	if !strings.Contains(rm.planContent, "Test Plan") {
		t.Error("planContent should contain 'Test Plan'")
	}

	// Test that planMsg with empty content CLEARS planContent
	// (Bug fix: previously empty planMsg was silently ignored, causing stale plans)
	result2, _ := rm.Update(planMsg{content: ""})
	rm2 := result2.(model)
	if rm2.planContent != "" {
		t.Errorf("empty planMsg should clear planContent, but got %q", rm2.planContent)
	}
	if rm2.planVisible {
		t.Error("empty planMsg should clear planVisible")
	}
	if rm2.renderedPlan != "" {
		t.Error("empty planMsg should clear renderedPlan")
	}
}

func TestSpawningSpinner_TickAdvancesFrame(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	// Set spawning status
	m.statusMsg = "spawning"
	m.statusMsgTick = -1

	// Get initial spinner view
	view1 := m.spawningSpinner.View()

	// Send a spinner tick
	tickCmd := m.spawningSpinner.Tick
	tickMsg := tickCmd()
	result, _ := m.Update(tickMsg)
	m = result.(model)

	// Frame should have advanced
	view2 := m.spawningSpinner.View()
	if view1 == view2 {
		t.Error("spinner frame did not advance after tick")
	}
}

func TestSelectionPinnedOnReorder(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	// Start with two agents: AgentA (running), AgentB (input/needs attention)
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.buildTree()

	// Select AgentB (running) at index 1
	m.selected = 1
	selectedTarget := m.agents[m.treeNodes[m.selected].AgentIdx].Target
	if selectedTarget != "main:2.0" {
		t.Fatalf("expected selected target main:2.0, got %s", selectedTarget)
	}

	// Now AgentB changes to "question" — both agents are now in waiting group.
	// The reorder might change positions. Simulate via stateUpdatedMsg.
	// We'll directly call the identity-capture + rebuild logic.
	prevTarget, prevSubID := m.selectedIdentity()

	m.agents = []domain.Agent{
		{Target: "main:2.0", Window: 2, Pane: 0, State: "question"},
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question"},
	}
	m.buildTree()
	m.restoreSelection(prevTarget, prevSubID)

	// Selection should still point to main:2.0
	newTarget := m.agents[m.treeNodes[m.selected].AgentIdx].Target
	if newTarget != "main:2.0" {
		t.Errorf("expected selection pinned to main:2.0, got %s (index %d)", newTarget, m.selected)
	}
}

func TestSelectionPinned_SubagentPreserved(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.agentSubagents = map[string][]domain.SubagentInfo{
		"main:1.0": {{AgentID: "sub-abc", AgentType: "Explore", Description: "research"}},
	}
	m.buildTree()

	// Select the subagent (index 1: main:1.0's subagent)
	m.selected = 1
	node := m.treeNodes[m.selected]
	if node.Sub == nil || node.Sub.AgentID != "sub-abc" {
		t.Fatal("expected subagent sub-abc at index 1")
	}

	prevTarget, prevSubID := m.selectedIdentity()

	// Reorder: new agent enters needs-attention, shifting indices
	m.agents = []domain.Agent{
		{Target: "main:3.0", Window: 3, Pane: 0, State: "question"},
		{Target: "main:1.0", Window: 1, Pane: 0, State: "question"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:3.0"] = nil
	m.buildTree()
	m.restoreSelection(prevTarget, prevSubID)

	// Should still point to sub-abc under main:1.0
	node = m.treeNodes[m.selected]
	if node.Sub == nil || node.Sub.AgentID != "sub-abc" {
		t.Errorf("expected selection pinned to subagent sub-abc, got node at index %d (sub=%v)", m.selected, node.Sub)
	}
}

func TestSpawningSpinner_VisibleWithNoAgents(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	// No agents — dashboard is empty
	m.agents = nil
	m.buildTree()

	// Set spawning status (as keys.go does when user creates a session)
	m.statusMsg = "spawning"
	m.statusMsgTick = -1

	// The spawning spinner text should be visible in the help bar
	bar := m.renderHelpBar()
	if !strings.Contains(bar, "Spawning agent") {
		t.Errorf("spawning spinner should be visible in help bar when no agents exist, got:\n%s", bar)
	}
}

func TestHelpBar_FitsWithinWidth(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 80 // typical laptop terminal width
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp"},
	}
	m.buildTree()

	bar := m.renderHelpBar()

	// The help bar should not exceed the terminal width.
	// lipgloss.Width accounts for ANSI escape sequences.
	barWidth := lipgloss.Width(bar)
	if barWidth > m.width {
		t.Errorf("help bar width (%d) exceeds terminal width (%d)", barWidth, m.width)
	}
}

func TestSelectedSubagent_PreservesIcon(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")

	m := NewModel(testConfig(""), nil)
	m.width = 80
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents = map[string][]domain.SubagentInfo{
		"main:1.0": {
			{AgentID: "sub1", AgentType: "Explore", Description: "test", Completed: true},
		},
	}
	m.buildTree()
	// Select the subagent (index 1, after parent at index 0)
	m.selected = 1

	content := m.agentListContent()

	// A completed subagent should show checkmark, not arrow, even when selected
	if strings.Contains(content, "▶ Explore") {
		t.Error("selected completed subagent should show ✓ icon, not ▶")
	}
	if !strings.Contains(content, "✓") {
		t.Error("selected completed subagent should contain ✓ icon")
	}
}

func TestCreateSessionMsg_PlaceholderAgent(t *testing.T) {
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.selfPaneID = "%0"
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()

	m.statusMsg = "spawning"
	result, _ := m.Update(createSessionMsg{target: "main:2.0", err: nil})
	rm := result.(model)

	// A placeholder agent should be inserted immediately
	found := false
	for _, a := range rm.agents {
		if a.Target == "main:2.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected placeholder agent for main:2.0 to be present immediately after create")
	}
}

func TestCreateSessionMsg_PreservesSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := NewModel(testConfig("/tmp/test-state.json"), nil)
	m.selfPaneID = "%0"
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true

	// Agent at window 1 with file changes — user is looking at this agent
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running",
			FilesChanged: []string{"+old_file.go"}},
	}
	m.buildTree()
	m.selected = 0

	// Verify pre-condition: selected agent is main:1.0
	if agent := m.selectedAgent(); agent == nil || agent.Target != "main:1.0" {
		t.Fatal("pre-condition: expected main:1.0 to be selected")
	}

	// Create a new session — the placeholder sorts before or after the existing agent.
	// The bug: m.selected stays at 0 even though the agent at index 0 may change.
	m.statusMsg = "spawning"
	result, _ := m.Update(createSessionMsg{target: "main:0.0", err: nil})
	rm := result.(model)

	// After creating a new agent, the previously selected agent (main:1.0)
	// should still be selected — NOT the newly created placeholder.
	agent := rm.selectedAgent()
	if agent == nil {
		t.Fatal("expected a selected agent after create, got nil")
	}
	if agent.Target != "main:1.0" {
		t.Errorf("expected selection to stay on main:1.0, got %q (stale selection)", agent.Target)
	}
}

func TestSaveRestoreCache_PreservesConversation(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp/a"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running", Cwd: "/tmp/b"},
	}
	m.buildTree()
	m.selected = 0

	// Simulate conversation loaded for agent A
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "Hello from agent A"},
	}
	m.capturedLines = []string{"live output A"}
	m.planContent = "plan A"
	m.renderedPlan = "rendered plan A"

	// Save agent A's state, switch to agent B
	m.saveCurrentCache()
	m.selected = 1
	m.restoreCurrentCache()

	// Agent B should have empty state
	if len(m.conversation) != 0 {
		t.Errorf("expected empty conversation for uncached agent B, got %d entries", len(m.conversation))
	}
	if len(m.capturedLines) != 0 {
		t.Errorf("expected empty capturedLines for uncached agent B, got %d", len(m.capturedLines))
	}
	if m.planContent != "" {
		t.Errorf("expected empty planContent for uncached agent B, got %q", m.planContent)
	}

	// Switch back to agent A — state should be restored
	m.saveCurrentCache()
	m.selected = 0
	m.restoreCurrentCache()

	if len(m.conversation) != 1 || m.conversation[0].Content != "Hello from agent A" {
		t.Errorf("expected agent A conversation to be restored, got %v", m.conversation)
	}
	// capturedLines is ephemeral — not cached, re-captured every tick
	if len(m.capturedLines) != 0 {
		t.Errorf("capturedLines should not be restored from cache (ephemeral), got %v", m.capturedLines)
	}
	if m.planContent != "plan A" {
		t.Errorf("expected agent A planContent to be restored, got %q", m.planContent)
	}
}

func TestSaveRestoreCache_SubagentKey(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "sub1", AgentType: "Explore", Description: "test"},
	}
	m.buildTree()
	// Tree: [parent, sub1]

	// Select subagent
	m.selected = 1
	m.subActivity = []domain.ActivityEntry{{Kind: "tool", Content: "ls"}}
	m.saveCurrentCache()

	// Switch to parent
	m.selected = 0
	m.restoreCurrentCache()
	if len(m.subActivity) != 0 {
		t.Errorf("parent should not have subActivity, got %d entries", len(m.subActivity))
	}

	// Switch back to subagent — activity should be restored
	m.saveCurrentCache()
	m.selected = 1
	m.restoreCurrentCache()
	if len(m.subActivity) != 1 {
		t.Errorf("expected subagent activity to be restored, got %d entries", len(m.subActivity))
	}
}

func TestCreateSession_CallsResizeViewports(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	origRightWidth := m.rightWidth
	origFilesH := m.filesVP.Height()

	// Simulate createSessionMsg
	result, _ := m.Update(createSessionMsg{target: "main:2.0"})
	rm := result.(model)

	// Viewport dimensions should remain consistent (resizeViewports was called)
	if rm.rightWidth != origRightWidth {
		t.Errorf("rightWidth changed after createSession: %d → %d", origRightWidth, rm.rightWidth)
	}
	if rm.filesVP.Height() != origFilesH {
		t.Errorf("filesVP.Height changed after createSession: %d → %d", origFilesH, rm.filesVP.Height())
	}
}

func TestAgentCachePruned_OnStateUpdate(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running"},
	}
	m.buildTree()

	// Populate caches for both agents
	m.selected = 0
	m.conversation = []domain.ConversationEntry{{Role: "assistant", Content: "A"}}
	m.saveCurrentCache()
	m.selected = 1
	m.conversation = []domain.ConversationEntry{{Role: "assistant", Content: "B"}}
	m.saveCurrentCache()

	if len(m.agentCaches) != 2 {
		t.Fatalf("expected 2 caches, got %d", len(m.agentCaches))
	}

	// Simulate state update where agent main:2.0 is gone
	newState := domain.StateFile{Agents: map[string]domain.Agent{
		"main:1.0": {Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}}
	result, _ := m.Update(stateUpdatedMsg{state: newState})
	rm := result.(model)

	if len(rm.agentCaches) != 1 {
		t.Errorf("expected cache pruned to 1, got %d", len(rm.agentCaches))
	}
	if _, ok := rm.agentCaches["main:2.0"]; ok {
		t.Error("cache for removed agent main:2.0 should have been pruned")
	}
}

func TestNavigationDown_PreservesHistory(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp/a"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running", Cwd: "/tmp/b"},
	}
	m.buildTree()
	m.selected = 0

	// Load conversation for agent A
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "Agent A message"},
	}
	m.renderedHistory = "cached history"
	m.historyConvLen = 1

	// Navigate down to agent B
	result, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	rm := result.(model)

	if rm.selected != 1 {
		t.Fatalf("expected selected=1 after down, got %d", rm.selected)
	}

	// Navigate back up to agent A
	result, _ = rm.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	rm = result.(model)

	if rm.selected != 0 {
		t.Fatalf("expected selected=0 after up, got %d", rm.selected)
	}

	// Agent A's conversation should be restored from cache
	if len(rm.conversation) != 1 || rm.conversation[0].Content != "Agent A message" {
		t.Errorf("expected agent A conversation to be restored after navigate back, got %v", rm.conversation)
	}
}

func TestCacheDoesNotStoreDerivedFields(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp/a"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running", Cwd: "/tmp/b"},
	}
	m.buildTree()
	m.selected = 0

	// Populate with source and derived data
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "Hello"},
	}
	m.capturedLines = []string{"live output"}
	m.renderedHistory = "big rendered string"
	m.historyConvLen = 1
	m.historyRightWidth = 100
	m.planContent = "plan content"
	m.renderedPlan = "big rendered plan"
	m.planVisible = true

	m.saveCurrentCache()

	cache := m.agentCaches[m.cacheKey()]

	// Source data should be cached
	if len(cache.conversation) != 1 {
		t.Errorf("conversation should be cached, got %d entries", len(cache.conversation))
	}
	if cache.planContent != "plan content" {
		t.Errorf("planContent should be cached, got %q", cache.planContent)
	}
	if !cache.planVisible {
		t.Error("planVisible should be cached")
	}

	// Switch to agent B and back — verify derived fields are zeroed on restore
	m.selected = 1
	m.restoreCurrentCache()
	m.saveCurrentCache()
	m.selected = 0
	m.restoreCurrentCache()

	// Ephemeral data should NOT be restored from cache
	if len(m.capturedLines) != 0 {
		t.Errorf("capturedLines should not be restored, got %d", len(m.capturedLines))
	}
	if m.renderedHistory != "" {
		t.Errorf("renderedHistory should not be restored, got %q", m.renderedHistory)
	}
	if m.historyConvLen != 0 {
		t.Errorf("historyConvLen should not be restored, got %d", m.historyConvLen)
	}
	if m.historyRightWidth != 0 {
		t.Errorf("historyRightWidth should not be restored, got %d", m.historyRightWidth)
	}
}

func TestRestoreCache_RegeneratesPlan(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", Cwd: "/tmp/a"},
		{Target: "main:2.0", Window: 2, Pane: 0, State: "running", Cwd: "/tmp/b"},
	}
	m.buildTree()
	m.selected = 0

	// Set up plan content and save
	m.planContent = "# My Plan\n\nSome content"
	m.planVisible = true
	m.renderedPlan = "old rendered plan"
	m.saveCurrentCache()

	// Switch to agent B
	m.selected = 1
	m.restoreCurrentCache()

	// Switch back to agent A
	m.saveCurrentCache()
	m.selected = 0
	m.restoreCurrentCache()

	// planContent and planVisible should be restored
	if m.planContent != "# My Plan\n\nSome content" {
		t.Errorf("planContent should be restored, got %q", m.planContent)
	}
	if !m.planVisible {
		t.Error("planVisible should be restored")
	}
	// renderedPlan should be regenerated (non-empty) from planContent
	if m.renderedPlan == "" {
		t.Error("renderedPlan should be regenerated from planContent on restore")
	}
}

func TestCacheCapsSubActivity(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "sub1", AgentType: "Explore", Description: "test"},
	}
	m.buildTree()

	// Select subagent and set 500 activity entries
	m.selected = 1
	entries := make([]domain.ActivityEntry, 500)
	for i := range entries {
		entries[i] = domain.ActivityEntry{Kind: "tool", Content: fmt.Sprintf("entry %d", i)}
	}
	m.subActivity = entries
	m.saveCurrentCache()

	cache := m.agentCaches[m.cacheKey()]
	if len(cache.subActivity) > 300 {
		t.Errorf("subActivity should be capped at 300, got %d", len(cache.subActivity))
	}
	// Should keep the LAST 300 entries
	if cache.subActivity[0].Content != "entry 200" {
		t.Errorf("should keep last 300 entries, first entry is %q", cache.subActivity[0].Content)
	}
}

func TestDismissedSubagentCachePruned(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.agentSubagents["main:1.0"] = []domain.SubagentInfo{
		{AgentID: "sub1", AgentType: "Explore", Description: "test"},
	}
	m.buildTree()

	// Cache subagent state
	m.selected = 1
	m.subActivity = []domain.ActivityEntry{{Kind: "tool", Content: "ls"}}
	m.saveCurrentCache()

	// Verify cache exists for subagent
	subKey := "main:1.0:sub1"
	if _, ok := m.agentCaches[subKey]; !ok {
		t.Fatal("expected cache for subagent")
	}

	// Dismiss the subagent
	m.dismissed[subKey] = true

	// Simulate state update — parent agent still alive
	newState := domain.StateFile{Agents: map[string]domain.Agent{
		"main:1.0": {Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}}
	m.selected = 0
	result, _ := m.Update(stateUpdatedMsg{state: newState})
	rm := result.(model)

	if _, ok := rm.agentCaches[subKey]; ok {
		t.Error("cache for dismissed subagent should have been pruned")
	}
}

func TestTickHandler_PeriodicStateReload(t *testing.T) {
	// Create a temp state dir with an agent file so loadState returns real data.
	dir := t.TempDir()
	agentsPath := dir + "/agents"
	if err := os.MkdirAll(agentsPath, 0755); err != nil {
		t.Fatal(err)
	}
	agentJSON := `{"target":"main:1.0","session":"main","window":1,"pane":0,"state":"running","session_id":"abc123","tmux_pane_id":"%5","updated_at":"2026-04-02T00:00:00Z"}`
	if err := os.WriteFile(agentsPath+"/abc123.json", []byte(agentJSON), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(testConfig(dir), nil)
	m.width = 120
	m.height = 40
	m.tmuxAvailable = false // avoid real tmux calls
	m.resizeViewports()

	// Set tickCount so the next increment lands on the reload interval (30).
	// The tick handler does m.tickCount++ first, so start at 29.
	m.tickCount = 29
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("expected commands from tick handler, got nil")
	}

	// Execute batch and check for stateUpdatedMsg.
	// Filter out tickMsg (which sleeps 1s) by checking message types.
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatal("expected tea.BatchMsg from tick handler")
	}

	hasStateReload := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		result := c()
		if _, ok := result.(stateUpdatedMsg); ok {
			hasStateReload = true
		}
	}

	if !hasStateReload {
		t.Error("tick handler should trigger periodic loadState (stateUpdatedMsg) every 30 ticks, but none was found")
	}
}

func testModelWithAgent(focus int) model {
	m := NewModel(testConfig(""), nil)
	m.historyVP = viewport.New(viewport.WithWidth(40), viewport.WithHeight(3))
	m.messageVP = viewport.New(viewport.WithWidth(40), viewport.WithHeight(3))
	m.rightWidth = 44
	m.agents = []domain.Agent{{Target: "main:1.0", Window: 1, Pane: 0, State: "running"}}
	m.buildTree()
	m.focusedVP = focus
	return m
}

func TestAutoScrollHistory_WhenUnfocused(t *testing.T) {
	m := testModelWithAgent(focusAgentList) // not focused on history

	// First load — populate with initial conversation
	initial := []domain.ConversationEntry{{Role: "assistant", Content: "init"}}
	result, _ := m.Update(conversationMsg{entries: initial, sessionKey: "test"})
	m = result.(model)

	// Now deliver more messages (incremental update, prevLen > 0)
	entries := make([]domain.ConversationEntry, 20)
	for i := range entries {
		entries[i] = domain.ConversationEntry{Role: "assistant", Content: fmt.Sprintf("message %d with enough text to wrap", i)}
	}
	result, _ = m.Update(conversationMsg{entries: entries, sessionKey: "test"})
	m = result.(model)

	if !m.historyVP.AtBottom() {
		t.Error("history viewport should auto-scroll to bottom when unfocused")
	}
}

func TestAutoScrollHistory_PreservesPositionWhenFocused(t *testing.T) {
	m := testModelWithAgent(focusHistory) // focused on history

	// First load
	initial := []domain.ConversationEntry{{Role: "assistant", Content: "init"}}
	result, _ := m.Update(conversationMsg{entries: initial, sessionKey: "test"})
	m = result.(model)

	// Deliver more messages (incremental update)
	entries := make([]domain.ConversationEntry, 20)
	for i := range entries {
		entries[i] = domain.ConversationEntry{Role: "assistant", Content: fmt.Sprintf("message %d with enough text to wrap", i)}
	}
	result, _ = m.Update(conversationMsg{entries: entries, sessionKey: "test"})
	m = result.(model)

	if m.historyVP.YOffset() != 0 {
		t.Error("history viewport should NOT auto-scroll when user is focused on it")
	}
}

func TestAutoScrollLive_WhenUnfocused(t *testing.T) {
	m := testModelWithAgent(focusAgentList) // not focused on message
	m.tmuxAvailable = true

	// First capture — populate viewport
	result, _ := m.Update(captureResultMsg{lines: []string{"init"}})
	m = result.(model)

	// More output arrives (incremental)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d", i)
	}
	result, _ = m.Update(captureResultMsg{lines: lines})
	m = result.(model)

	if !m.messageVP.AtBottom() {
		t.Error("message viewport should auto-scroll to bottom when unfocused")
	}
}

func TestAutoScrollLive_PreservesPositionWhenFocused(t *testing.T) {
	m := testModelWithAgent(focusMessage) // focused on message
	m.tmuxAvailable = true

	// First capture
	result, _ := m.Update(captureResultMsg{lines: []string{"init"}})
	m = result.(model)
	m.messageVP.GotoTop()

	// More output arrives
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d", i)
	}
	result, _ = m.Update(captureResultMsg{lines: lines})
	m = result.(model)

	if m.messageVP.YOffset() != 0 {
		t.Error("message viewport should NOT auto-scroll when user is focused on it")
	}
}

func TestMergePRMsg_Success_PinsAndCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(tmpDir+"/agents", 0755)
	os.WriteFile(tmpDir+"/agents/sess1.json", []byte(`{"state":"pr","pinned_state":"pr"}`), 0644)

	m := NewModel(testConfig(tmpDir), nil)
	m.statePath = tmpDir
	m.tmuxAvailable = true
	m.mergeSessionID = "sess1"
	m.mergePaneID = "%5"
	m.mergeCwd = "/code/app"
	m.mergeWorktreeCwd = "/worktrees/app/feat-x"
	m.mergeBranch = "feat/test"

	result, cmd := m.Update(mergePRMsg{})
	updated := result.(model)

	if !strings.Contains(updated.statusMsg, "PR merged") {
		t.Errorf("expected status 'PR merged', got %q", updated.statusMsg)
	}
	if updated.mergeSessionID != "" {
		t.Error("mergeSessionID should be cleared after handling")
	}
	if updated.mode != modeConfirmCleanup {
		t.Errorf("expected modeConfirmCleanup, got %d", updated.mode)
	}
	if updated.cleanupSessionID != "sess1" {
		t.Errorf("expected cleanupSessionID='sess1', got %q", updated.cleanupSessionID)
	}
	if updated.cleanupCwd != "/code/app" {
		t.Errorf("expected cleanupCwd='/code/app', got %q", updated.cleanupCwd)
	}
	if updated.cleanupWorktreeCwd != "/worktrees/app/feat-x" {
		t.Errorf("expected cleanupWorktreeCwd, got %q", updated.cleanupWorktreeCwd)
	}
	if updated.cleanupBranch != "feat/test" {
		t.Errorf("expected cleanupBranch='feat/test', got %q", updated.cleanupBranch)
	}
	if cmd == nil {
		t.Fatal("expected cmd for pin, got nil")
	}
}

func TestMergePRMsg_Error_ShowsStatus(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.mergeSessionID = "sess1"
	m.mergePaneID = "%5"

	result, cmd := m.Update(mergePRMsg{err: fmt.Errorf("merge conflict")})
	updated := result.(model)

	if !strings.Contains(updated.statusMsg, "Merge failed") {
		t.Errorf("expected status 'Merge failed', got %q", updated.statusMsg)
	}
	if updated.mergeSessionID != "" {
		t.Error("mergeSessionID should be cleared after error")
	}
	if cmd != nil {
		t.Error("expected no cmd on merge error")
	}
}

func TestMergePRMsg_Error_ClearsAllMergeFields(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.mergeSessionID = "sess1"
	m.mergePaneID = "%5"
	m.mergeCwd = "/code/app"
	m.mergeWorktreeCwd = "/worktrees/app/feat-x"
	m.mergeBranch = "feat/test"

	result, _ := m.Update(mergePRMsg{err: fmt.Errorf("conflict")})
	updated := result.(model)

	if updated.mergeCwd != "" {
		t.Error("mergeCwd should be cleared after error")
	}
	if updated.mergeWorktreeCwd != "" {
		t.Error("mergeWorktreeCwd should be cleared after error")
	}
	if updated.mergeBranch != "" {
		t.Error("mergeBranch should be cleared after error")
	}
}

func TestPostMergeCleanupMsg_Success(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true

	result, cmd := m.Update(postMergeCleanupMsg{})
	updated := result.(model)

	if !strings.Contains(updated.statusMsg, "Cleaned up") {
		t.Errorf("expected status 'Cleaned up', got %q", updated.statusMsg)
	}
	if cmd == nil {
		t.Fatal("expected cmd for loadState + pruneDead after cleanup")
	}
}

func TestPostMergeCleanupMsg_Error(t *testing.T) {
	m := NewModel(testConfig(""), nil)

	result, cmd := m.Update(postMergeCleanupMsg{
		err:      fmt.Errorf("permission denied"),
		progress: "checkout main",
	})
	updated := result.(model)

	if !strings.Contains(updated.statusMsg, "Cleanup failed at checkout main") {
		t.Errorf("expected error status with step name, got %q", updated.statusMsg)
	}
	if cmd != nil {
		t.Error("expected no cmd on cleanup error")
	}
}

func TestAutoScrollLive_DisabledWhenPlanVisible(t *testing.T) {
	m := testModelWithAgent(focusAgentList) // not focused on message
	m.tmuxAvailable = true
	m.planVisible = true
	m.renderedPlan = "# My Plan\nStep 1\nStep 2\nStep 3\nStep 4\nStep 5"

	// First capture — populate viewport
	result, _ := m.Update(captureResultMsg{lines: []string{"init"}})
	m = result.(model)
	m.messageVP.GotoTop()

	// More output arrives while plan is visible
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("output line %d", i)
	}
	result, _ = m.Update(captureResultMsg{lines: lines})
	m = result.(model)

	if m.messageVP.YOffset() != 0 {
		t.Error("message viewport should NOT auto-scroll when plan is visible — user may be reading the plan")
	}
}

func TestRawKeySentMsg_SuccessLabel(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	result, _ := m.Update(rawKeySentMsg{err: nil, label: "Plan approved"})
	updated := result.(model)
	if updated.statusMsg != "Plan approved" {
		t.Errorf("expected status 'Plan approved', got %q", updated.statusMsg)
	}
	if updated.statusIsError {
		t.Error("expected statusIsError=false for success")
	}
}

func TestRawKeySentMsg_ErrorSetsIsError(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	result, _ := m.Update(rawKeySentMsg{err: fmt.Errorf("pane gone"), label: "Plan approved"})
	updated := result.(model)
	if !strings.Contains(updated.statusMsg, "failed") {
		t.Errorf("expected error status, got %q", updated.statusMsg)
	}
	if !updated.statusIsError {
		t.Error("expected statusIsError=true for error")
	}
}

func TestRawKeySentMsg_EmptyLabel_NoStatus(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.statusMsg = "previous"
	result, _ := m.Update(rawKeySentMsg{err: nil, label: ""})
	updated := result.(model)
	if updated.statusMsg != "previous" {
		t.Errorf("expected status unchanged, got %q", updated.statusMsg)
	}
}

func TestSpawningFolder_SurvivesCreateSessionSuccess(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.spawningFolder = "/Users/someone/Code/my-project"
	m.statusMsg = "spawning"
	m.statusMsgTick = -1

	updated, _ := m.Update(createSessionMsg{target: "main:1.0"})
	um := updated.(model)

	if um.spawningFolder == "" {
		t.Error("spawningFolder should persist after createSessionMsg success (cleared by stateUpdatedMsg)")
	}
}

func TestSpawningFolder_ClearedOnCreateSessionError(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.spawningFolder = "/Users/someone/Code/my-project"
	m.statusMsg = "spawning"
	m.statusMsgTick = -1

	updated, _ := m.Update(createSessionMsg{err: fmt.Errorf("tmux failed")})
	um := updated.(model)

	if um.spawningFolder != "" {
		t.Errorf("expected spawningFolder to be cleared on error, got %q", um.spawningFolder)
	}
}

func TestSpawningFolder_ClearedOnStateUpdateWithMatchingAgent(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.spawningFolder = "/Users/someone/Code/my-project"
	m.tmuxAvailable = true
	m.startupDone = true

	updated, _ := m.Update(stateUpdatedMsg{state: domain.StateFile{
		Agents: map[string]domain.Agent{
			"sess1": {Target: "main:1.0", State: "running", Cwd: "/Users/someone/Code/my-project"},
		},
	}})
	um := updated.(model)

	if um.spawningFolder != "" {
		t.Errorf("expected spawningFolder to be cleared when matching Cwd appears, got %q", um.spawningFolder)
	}
}

func TestSpawningFolder_PersistsOnStateUpdateWithoutMatch(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.spawningFolder = "/Users/someone/Code/my-project"
	m.tmuxAvailable = true
	m.startupDone = true

	updated, _ := m.Update(stateUpdatedMsg{state: domain.StateFile{
		Agents: map[string]domain.Agent{
			"sess1": {Target: "main:1.0", State: "running", Cwd: "/Users/someone/Code/other-project"},
		},
	}})
	um := updated.(model)

	if um.spawningFolder == "" {
		t.Error("spawningFolder should persist when no matching Cwd found")
	}
}

func TestSpawningFolder_SafetyExpiry(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.spawningFolder = "/Users/someone/Code/my-project"
	m.statusMsg = "spawning"
	m.statusMsgTick = -1
	m.spawningTick = 0
	m.tickCount = 28 // will become 29 after tickCount++ in handler

	// After increment: tickCount=29, 29-0=29 < 30, should persist
	updated, _ := m.Update(tickMsg{})
	um := updated.(model)
	if um.spawningFolder == "" {
		t.Error("spawningFolder should persist before 30s expiry")
	}
	if um.statusMsg != "spawning" {
		t.Error("statusMsg should still be 'spawning' before expiry")
	}

	// After increment: tickCount=30, 30-0=30 >= 30, should be cleared
	updated2, _ := um.Update(tickMsg{})
	um2 := updated2.(model)
	if um2.spawningFolder != "" {
		t.Errorf("expected spawningFolder to be cleared after 30s expiry, got %q", um2.spawningFolder)
	}
	if um2.statusMsg != "" {
		t.Errorf("expected statusMsg to be cleared after 30s expiry, got %q", um2.statusMsg)
	}
}

func TestSetStatus_SetsFields(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.tickCount = 42
	m.setStatus("hello", false)
	if m.statusMsg != "hello" || m.statusIsError || m.statusMsgTick != 42 {
		t.Errorf("setStatus(false) mismatch: msg=%q err=%v tick=%d", m.statusMsg, m.statusIsError, m.statusMsgTick)
	}
	m.setStatus("oops", true)
	if m.statusMsg != "oops" || !m.statusIsError || m.statusMsgTick != 42 {
		t.Errorf("setStatus(true) mismatch: msg=%q err=%v tick=%d", m.statusMsg, m.statusIsError, m.statusMsgTick)
	}
}
