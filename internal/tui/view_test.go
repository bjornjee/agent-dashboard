package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func TestAgentListContentClampsWidth(t *testing.T) {
	// Force ASCII output so lipgloss.Width is predictable.
	t.Setenv("NO_COLOR", "1")

	fixedTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)

	makeModel := func(panelWidth int) model {
		m := model{
			leftWidth: panelWidth,
			agents: []domain.Agent{
				{
					Target:         "1:1.0",
					Window:         1,
					Pane:           1,
					State:          "running",
					Cwd:            "/Users/someone/Code/very-long-repository-name-that-overflows",
					Branch:         "feat/extremely-long-branch-name-that-should-be-truncated-properly",
					UpdatedAt:      fixedTime,
					PermissionMode: "bypassPermissions",
					CurrentTool:    "Bash(very long tool description here)",
					SubagentCount:  3,
				},
			},
			treeNodes: []treeNode{
				{AgentIdx: -1, GroupHeader: 3},
				{AgentIdx: 0},
				{AgentIdx: 0, Sub: &domain.SubagentInfo{
					AgentType:   "general-purpose",
					Description: "A very long subagent description that will definitely overflow the panel width",
					Completed:   false,
				}},
			},
			agentSubagents:  map[string][]domain.SubagentInfo{},
			collapsed:       map[string]bool{},
			collapsedGroups: map[int]bool{},
		}
		return m
	}

	t.Run("lines clamped to panel width", func(t *testing.T) {
		const panelWidth = 30
		m := makeModel(panelWidth)
		content := m.agentListContent()
		for i, line := range strings.Split(content, "\n") {
			w := lipgloss.Width(line)
			if w > panelWidth {
				t.Errorf("line %d exceeds panel width %d (got %d): %q", i, panelWidth, w, line)
			}
		}
	})

	t.Run("zero width does not corrupt lines", func(t *testing.T) {
		m := makeModel(0)
		content := m.agentListContent()
		// With zero width, truncation should be skipped entirely;
		// lines should still contain recognizable content.
		if !strings.Contains(content, "RUNNING") {
			t.Error("expected group header in output with zero leftWidth")
		}
	})
}

// Diagram badge (📑) must render on the same line as the parent agent's
// title (paneID + repo), not on a separate badges row underneath. This
// regression keeps the badge visible when the metadata badges line is
// missing or scrolled off.
func TestAgentListContent_DiagramBadgeOnTitleLine(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := model{
		leftWidth: 60,
		agents: []domain.Agent{
			{
				Target:       "1:1.0",
				Window:       3,
				Pane:         1,
				State:        "running",
				Cwd:          "/tmp/agent-dashboard",
				SessionID:    "sess-A",
				DiagramCount: 2,
			},
		},
		treeNodes: []treeNode{
			{AgentIdx: -1, GroupHeader: 3},
			{AgentIdx: 0},
		},
		agentSubagents:       map[string][]domain.SubagentInfo{},
		collapsed:            map[string]bool{},
		collapsedGroups:      map[int]bool{},
		lastSeenDiagramCount: map[string]int{"sess-A": 0},
	}

	content := m.agentListContent()
	lines := strings.Split(content, "\n")

	// Find the line containing the repo title.
	var titleLine string
	for _, l := range lines {
		if strings.Contains(l, "agent-dashboard") {
			titleLine = l
			break
		}
	}
	if titleLine == "" {
		t.Fatalf("could not find title line in:\n%s", content)
	}
	if !strings.Contains(titleLine, "📑") {
		t.Errorf("diagram badge 📑 must appear on title line, got:\n%q\nfull:\n%s", titleLine, content)
	}

	// Standalone badges row must NOT carry the diagram badge any more.
	for _, l := range lines {
		if l == titleLine {
			continue
		}
		if strings.Contains(l, "📑") {
			t.Errorf("📑 badge should only be on title line, also found on:\n%q", l)
		}
	}
}

func TestRenderHelpOverlayContainsSections(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 100
	m.height = 40

	content := m.renderHelpOverlay()

	sections := []string{"Navigation", "Agent Actions", "View Controls", "Diff Mode", "Diagram Mode", "Input Modes"}
	for _, s := range sections {
		if !strings.Contains(content, s) {
			t.Errorf("help overlay missing section %q", s)
		}
	}
}

func TestRenderHelpOverlayDocumentsDiagramKeys(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 100
	m.height = 40

	content := m.renderHelpOverlay()

	// View Controls should advertise the capital-D toggle.
	if !strings.Contains(content, "Toggle diagram view") {
		t.Error("help overlay should describe the D toggle for the diagram view")
	}

	// Diagram Mode section should mention browser open + delete + close.
	for _, want := range []string{"Open diagram in browser", "Delete diagram", "Close diagram view"} {
		if !strings.Contains(content, want) {
			t.Errorf("help overlay Diagram Mode section missing entry %q", want)
		}
	}
}

func TestRenderHelpOverlayTwoColumnsWide(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 200
	m.height = 40

	content := m.renderHelpOverlay()

	// At wide width the layout must put two distinct section headers on the
	// same visual line (proof of 2-col layout).
	headers := []string{"Navigation", "Agent Actions", "View Controls", "Diff Mode", "Diagram Mode", "Input Modes"}
	twoOnOneLine := false
	for _, l := range strings.Split(content, "\n") {
		hits := 0
		for _, h := range headers {
			if strings.Contains(l, h) {
				hits++
			}
		}
		if hits >= 2 {
			twoOnOneLine = true
			break
		}
	}
	if !twoOnOneLine {
		t.Error("expected at least one rendered line to contain two section headers (2-col layout)")
	}
}

func TestRenderHelpOverlayOneColumnNarrow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 70
	m.height = 40

	content := m.renderHelpOverlay()

	// At narrow width, every line must contain at most one section header.
	headers := []string{"Navigation", "Agent Actions", "View Controls", "Diff Mode", "Diagram Mode", "Input Modes"}
	for _, l := range strings.Split(content, "\n") {
		hits := 0
		for _, h := range headers {
			if strings.Contains(l, h) {
				hits++
			}
		}
		if hits > 1 {
			t.Errorf("narrow layout should not stack two headers on one line: %q", l)
		}
	}
}

func TestSlimHelpBarContainsHHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.tmuxAvailable = true

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "help") {
		t.Error("slim help bar should contain 'help' hint")
	}
	// Should contain lifecycle essentials
	if !strings.Contains(bar, "new") {
		t.Error("slim help bar should contain 'new' hint")
	}
	if !strings.Contains(bar, "close") {
		t.Error("slim help bar should contain 'close' hint")
	}
	// Should NOT contain the old verbose hints
	if strings.Contains(bar, "editor") {
		t.Error("slim help bar should not contain 'editor' — moved to overlay")
	}
	if strings.Contains(bar, "collapse") {
		t.Error("slim help bar should not contain 'collapse' — moved to overlay")
	}
}

func TestHelpBarWhenHelpVisible(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.helpVisible = true

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "close") {
		t.Error("help bar when helpVisible should contain 'close' hint")
	}
}

func TestHelpBarWhenUsageMode(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.mode = modeUsage

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "esc") {
		t.Error("help bar in modeUsage should show esc to dismiss")
	}
	// Should NOT show normal mode hints
	if strings.Contains(bar, "new") {
		t.Error("help bar in modeUsage should NOT show normal mode 'new' hint")
	}
}

func TestHelpBarWhenConfirmClose(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.mode = modeConfirmClose

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "close") {
		t.Error("help bar in modeConfirmClose should show 'close' hint")
	}
	if !strings.Contains(bar, "cancel") {
		t.Error("help bar in modeConfirmClose should show 'cancel' hint")
	}
}

func TestHelpBarWhenConfirmDeleteDiagram(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.mode = modeConfirmDeleteDiagram

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "delete") {
		t.Error("help bar in modeConfirmDeleteDiagram should show 'delete' hint")
	}
	if !strings.Contains(bar, "cancel") {
		t.Error("help bar in modeConfirmDeleteDiagram should show 'cancel' hint")
	}
}

func TestModeBadge_ReplyMode(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.mode = modeReply

	badge := m.modeBadge()
	if !strings.Contains(badge, "REPLY") {
		t.Errorf("mode badge in modeReply should contain 'REPLY', got %q", badge)
	}
}

func TestModeBadge_NormalMode(t *testing.T) {
	m := NewModel(testConfig(""), nil)
	m.mode = modeNormal

	badge := m.modeBadge()
	if badge != "" {
		t.Errorf("mode badge in modeNormal should be empty, got %q", badge)
	}
}

func TestModeBadge_AppearsInHelpBar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.mode = modeReply

	bar := m.renderHelpBar()
	if !strings.Contains(bar, "REPLY") {
		t.Error("help bar should contain mode badge 'REPLY' when in reply mode")
	}
}

func TestReplyInput_VisibleWhenAgentRunning(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(t.TempDir()), nil)
	m.tmuxAvailable = true
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running", TmuxPaneID: "%5"},
	}
	m.buildTree()
	selectFirstAgent(&m)
	m.conversation = []domain.ConversationEntry{
		{Role: "assistant", Content: "Working on the task..."},
	}
	m.mode = modeReply

	m.updateRightContent()
	content := m.messageVP.View()

	if !strings.Contains(content, "Reply") {
		t.Error("reply input should be visible in message viewport when agent is running and mode is reply")
	}
}

func TestStatusLine_SuccessVsError(t *testing.T) {
	m := NewModel(testConfig(""), nil)

	// Success message should not be empty
	m.statusMsg = "Reply sent"
	m.statusIsError = false
	s := m.statusLine()
	if s == "" {
		t.Error("expected non-empty status line for success message")
	}
	if !strings.Contains(s, "Reply sent") {
		t.Errorf("expected status line to contain 'Reply sent', got %q", s)
	}

	// Error message should not be empty
	m.statusMsg = "Key send failed: pane gone"
	m.statusIsError = true
	s = m.statusLine()
	if s == "" {
		t.Error("expected non-empty status line for error message")
	}
	if !strings.Contains(s, "Key send failed") {
		t.Errorf("expected status line to contain 'Key send failed', got %q", s)
	}
}

func TestHelpBarContainsStatusMessage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true

	// Set a success status message
	m.statusMsg = "Reply sent"
	m.statusIsError = false

	bar := m.renderHelpBar()
	if !strings.Contains(bar, "Reply sent") {
		t.Errorf("help bar should contain status message 'Reply sent', got %q", bar)
	}
}

func TestHelpBarContainsSpawningSpinner(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.tmuxAvailable = true

	m.statusMsg = "spawning"
	m.statusMsgTick = -1

	bar := m.renderHelpBar()
	if !strings.Contains(bar, "Spawning agent") {
		t.Errorf("help bar should contain 'Spawning agent', got %q", bar)
	}
}

func TestHelpBarShowsWeeklyCost(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.tmuxAvailable = true

	// Use dynamic dates so the test stays valid regardless of when it runs.
	// Monday-of-this-week and a day before Monday (should be excluded from
	// the Monday-anchored aggregate).
	weekStart := startOfWeek(time.Now())
	beforeWeek := weekStart.AddDate(0, 0, -1).Format("2006-01-02")
	inWeek1 := weekStart.Format("2006-01-02")
	inWeek2 := weekStart.AddDate(0, 0, 1).Format("2006-01-02")

	m.dbDailyUsage = []db.DayUsage{
		{Date: beforeWeek, CostUSD: 99.00}, // must be excluded
		{Date: inWeek1, CostUSD: 1.50},
		{Date: inWeek2, CostUSD: 2.25},
	}
	m.codexDailyUsage = []db.DayUsage{
		{Date: beforeWeek, CostUSD: 42.00}, // must be excluded
		{Date: inWeek1, CostUSD: 0.75},
	}

	bar := m.renderHelpBar()
	if !strings.Contains(bar, "Week:") {
		t.Errorf("help bar should contain 'Week:', got %q", bar)
	}
	if !strings.Contains(bar, "$4.50") {
		t.Errorf("help bar should contain '$4.50' weekly cost, got %q", bar)
	}
	if strings.Contains(bar, "$141.00") || strings.Contains(bar, "$146.50") {
		t.Errorf("help bar should not include pre-week rows in weekly cost, got %q", bar)
	}
	if strings.Contains(bar, "All-time") {
		t.Error("help bar should not contain 'All-time' anymore")
	}
}

func TestRightPanelDoesNotContainStatusLine(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.startupDone = true
	m.resizeViewports()

	m.statusMsg = "Reply sent"
	m.statusIsError = false

	panel := m.renderRightPanel()
	if strings.Contains(panel, "Reply sent") {
		t.Error("right panel should NOT contain status message — it should be in the help bar")
	}
}

func TestPanelRenderedDimensions(t *testing.T) {
	// lipgloss v2 includes borders in Width/Height, so rendered panels must
	// account for the 2-char border frame. This test ensures the rendered
	// left panel width equals leftWidth + 2 (content + borders).
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 120
	m.height = 40
	m.startupDone = true
	m.resizeViewports()

	panel := m.renderLeftPanel()
	lines := strings.Split(panel, "\n")
	if len(lines) == 0 {
		t.Fatal("rendered left panel has no lines")
	}

	expectedWidth := m.leftWidth + 2
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w != expectedWidth {
			t.Errorf("left panel line %d: width %d, want %d", i, w, expectedWidth)
			break
		}
	}

	expectedHeight := m.height - 5 - m.bannerHeight() + 2
	if len(lines) != expectedHeight {
		t.Errorf("left panel height: got %d lines, want %d", len(lines), expectedHeight)
	}
}

// -- MouseMode tests --

func TestView_MouseModeCellMotion_InNormalMode(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()

	v := m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("expected MouseModeCellMotion in normal mode, got %v", v.MouseMode)
	}
}

func TestView_MouseModeNone_InTextInputModes(t *testing.T) {
	textInputModes := []struct {
		name string
		mode int
	}{
		{"reply", modeReply},
		{"createFolder", modeCreateFolder},
		{"createSkill", modeCreateSkill},
		{"createMessage", modeCreateMessage},
	}
	for _, tt := range textInputModes {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(testConfig(t.TempDir()), nil)
			m.width = 120
			m.height = 40
			m.resizeViewports()
			m.mode = tt.mode

			v := m.View()
			if v.MouseMode != tea.MouseModeNone {
				t.Errorf("expected MouseModeNone in %s mode, got %v", tt.name, v.MouseMode)
			}
		})
	}
}

func TestView_MouseModeCellMotion_InConfirmMode(t *testing.T) {
	m := NewModel(testConfig(t.TempDir()), nil)
	m.width = 120
	m.height = 40
	m.resizeViewports()
	m.mode = modeConfirmClose

	v := m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("expected MouseModeCellMotion in confirm mode, got %v", v.MouseMode)
	}
}
