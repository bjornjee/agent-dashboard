package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
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

func TestRenderHelpOverlayContainsSections(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 100
	m.height = 40

	content := m.renderHelpOverlay()

	sections := []string{"Navigation", "Agent Actions", "View Controls", "Diff Mode", "Input Modes"}
	for _, s := range sections {
		if !strings.Contains(content, s) {
			t.Errorf("help overlay missing section %q", s)
		}
	}
}

func TestSlimHelpBarContainsHHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewModel(testConfig(""), nil)
	m.width = 80
	m.height = 24
	m.agents = []domain.Agent{
		{Target: "main:1.0", Window: 1, Pane: 0, State: "running"},
	}
	m.buildTree()
	selectFirstAgent(&m)

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "h") {
		t.Error("normal help bar should contain 'h' hint for help")
	}
}
