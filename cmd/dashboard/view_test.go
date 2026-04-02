package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestAgentListContentClampsWidth(t *testing.T) {
	// Force ASCII output so lipgloss.Width is predictable.
	// All tests in this package inherit this profile.
	lipgloss.SetColorProfile(termenv.Ascii)

	fixedTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)

	makeModel := func(panelWidth int) model {
		return model{
			leftWidth: panelWidth,
			agents: []Agent{
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
				{AgentIdx: 0},
				{AgentIdx: 0, Sub: &SubagentInfo{
					AgentType:   "general-purpose",
					Description: "A very long subagent description that will definitely overflow the panel width",
					Completed:   false,
				}},
			},
			agentSubagents: map[string][]SubagentInfo{},
			collapsed:      map[string]bool{},
		}
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
	lipgloss.SetColorProfile(termenv.Ascii)

	m := newModel(testConfig(""), "", nil)
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
	lipgloss.SetColorProfile(termenv.Ascii)

	m := newModel(testConfig(""), "", nil)
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
	lipgloss.SetColorProfile(termenv.Ascii)

	m := newModel(testConfig(""), "", nil)
	m.width = 120
	m.helpVisible = true

	bar := m.renderHelpBar()

	if !strings.Contains(bar, "close") {
		t.Error("help bar when helpVisible should contain 'close' hint")
	}
}
