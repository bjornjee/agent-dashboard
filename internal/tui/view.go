package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/usage"
	"github.com/charmbracelet/x/ansi"
)

// -- Content Builders --

func (m *model) updateLeftContent() {
	m.agentListVP.SetContent(m.agentListContent())
}

func (m *model) updateRightContent() {
	// Override modes use the full panel height since they replace all three viewports.
	// Normal mode restores the standard message viewport height.
	panelHeight := m.height - 5 - m.bannerHeight() // matches resizeViewports
	if m.mode == modeCreateFolder || m.mode == modeCreateSkill || m.mode == modeCreateMessage || (m.planVisible && m.renderedPlan != "") {
		fullHeight := panelHeight - defaultHeaderLines - 1 // -1 for section label
		if fullHeight < minMessageHeight {
			fullHeight = minMessageHeight
		}
		m.messageVP.SetHeight(fullHeight)
	} else {
		_, _, msgHeight := panelHeights(panelHeight, defaultHeaderLines)
		m.messageVP.SetHeight(msgHeight)
	}

	// Create folder mode overrides right panel (works even with no agents)
	if m.mode == modeCreateFolder {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, "  "+titleStyle.Render(" CREATE NEW SESSION "))
		lines = append(lines, "")
		lines = append(lines, "  "+boldStyle.Render("Git folder path:"))
		lines = append(lines, "  "+renderWrappedInput(m.textInput.Value(), m.textInput.Position(), m.rightWidth-4, true, nil, "  "))
		lines = append(lines, "")
		// Show z-plugin suggestions
		if len(m.suggestions) > 0 {
			for i, s := range m.suggestions {
				prefix := "  "
				if i == m.selectedSugg {
					lines = append(lines, prefix+selectedStyle.Render(" "+s+" "))
				} else {
					lines = append(lines, prefix+helpStyle.Render(" "+s))
				}
			}
			lines = append(lines, "")
		}
		lines = append(lines, "  "+helpStyle.Render("Enter to create │ Tab to accept │ ↑↓ cycle │ Esc to cancel"))
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent(strings.Join(lines, "\n"))
		return
	}

	// Create skill selection mode
	if m.mode == modeCreateSkill {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, "  "+titleStyle.Render(" CREATE NEW SESSION "))
		lines = append(lines, "")
		lines = append(lines, "  "+helpStyle.Render("Folder: "+m.createFolder))
		lines = append(lines, "")
		lines = append(lines, "  "+boldStyle.Render("Select skill:"))
		lines = append(lines, "")
		for i, s := range m.availableSkills {
			prefix := "  "
			if i == m.selectedCreateSkill {
				lines = append(lines, prefix+selectedStyle.Render(" "+s+" "))
			} else {
				lines = append(lines, prefix+helpStyle.Render(" "+s))
			}
		}
		lines = append(lines, "")
		lines = append(lines, "  "+helpStyle.Render("Enter to select │ ↑↓ cycle │ Esc back │ ^C cancel"))
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent(strings.Join(lines, "\n"))
		return
	}

	// Create message input mode
	if m.mode == modeCreateMessage {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, "  "+titleStyle.Render(" CREATE NEW SESSION "))
		lines = append(lines, "")
		lines = append(lines, "  "+helpStyle.Render("Folder: "+m.createFolder))
		if m.createSkillName != "" {
			lines = append(lines, "  "+helpStyle.Render("Skill:  /"+m.createSkillName))
		}
		lines = append(lines, "")
		lines = append(lines, "  "+boldStyle.Render("Message:"))
		lines = append(lines, "  "+renderWrappedInput(m.textInput.Value(), m.textInput.Position(), m.rightWidth-4, true, m.availableSkills, "  "))
		lines = append(lines, "")
		lines = append(lines, "  "+helpStyle.Render("Enter to launch │ Esc back │ ^C cancel"))
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent(strings.Join(lines, "\n"))
		return
	}

	// domain.Usage mode overrides right panel content (works even with no agents)
	if m.mode == modeUsage {
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent(m.usageContent())
		return
	}

	agent := m.selectedAgent()
	if agent == nil {
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent("  No agents found")
		return
	}

	sub := m.selectedSubagent()
	if sub != nil {
		// Subagent right panel: files touched + activity + output
		m.filesVP.SetContent(m.subagentFilesContent())
		m.historyVP.SetContent(m.subagentActivityContent())
		m.messageVP.SetContent(m.subagentOutputContent())
		return
	}

	// Parent agent right panel
	m.filesVP.SetContent(m.filesContent(*agent))
	m.historyVP.SetContent(m.historyContent())

	effState := agent.State

	// Auto-show plan when agent is in plan-review state (but not in reply mode)
	if effState == "plan" && m.renderedPlan != "" && m.mode != modeReply {
		m.planVisible = true
	}

	if m.planVisible && m.renderedPlan != "" && m.mode != modeReply {
		m.messageVP.SetContent(m.renderedPlan)
	} else if isBlocked(effState) || isWaiting(effState) {
		m.messageVP.SetContent(m.waitingMessageContent())
	} else if isReview(effState) || isPR(effState) || isMerged(effState) {
		if m.mode == modeReply {
			m.messageVP.SetContent(m.waitingMessageContent())
		} else {
			m.messageVP.SetContent(m.finalMessageContent())
		}
	} else if m.tmuxAvailable && hasContent(m.capturedLines) {
		w := m.rightWidth - 4
		var lines []string
		for _, l := range m.capturedLines {
			for _, wl := range wrapText(l, w) {
				lines = append(lines, " "+wl)
			}
		}
		m.messageVP.SetContent(strings.Join(lines, "\n"))
	} else {
		m.messageVP.SetContent("")
	}
}

func (m model) agentListContent() string {
	var lines []string

	if len(m.treeNodes) == 0 {
		if !m.startupDone {
			lines = append(lines, "  "+m.startupSpinner.View()+" Reticulating splines...")
		} else {
			lines = append(lines, "  No agents found")
		}
		return strings.Join(lines, "\n")
	}

	lastGroup := -1
	for nodeIdx, node := range m.treeNodes {
		agent := m.agents[node.AgentIdx]

		if node.Sub != nil {
			// Subagent node
			isLast := true
			// Check if this is the last subagent in the list
			for nextIdx := nodeIdx + 1; nextIdx < len(m.treeNodes); nextIdx++ {
				next := m.treeNodes[nextIdx]
				if next.AgentIdx != node.AgentIdx {
					break
				}
				if next.Sub != nil {
					isLast = false
					break
				}
			}

			prefix := "├─"
			if isLast {
				prefix = "└─"
			}

			var subIcon string
			if node.Sub.Completed {
				subIcon = lipgloss.NewStyle().Foreground(doneColor).Render("✓")
			} else {
				subIcon = lipgloss.NewStyle().Foreground(runningColor).Render("▶")
			}
			subLabel := node.Sub.AgentType
			if node.Sub.Description != "" {
				maxDesc := m.leftWidth - 9 - len(subLabel)
				desc := node.Sub.Description
				if maxDesc > 0 && len(desc) > maxDesc {
					desc = desc[:maxDesc-1] + "…"
				}
				subLabel += ": " + desc
			}

			line := fmt.Sprintf("    %s %s %s", helpStyle.Render(prefix), subIcon, subLabel)
			if nodeIdx == m.selected {
				line = highlightLine(line, m.leftWidth)
			}
			lines = append(lines, line)
			continue
		}

		// Parent agent node
		effState := agent.State
		group := domain.StatePriority[effState]
		if group == 0 {
			group = 3
		}

		if group != lastGroup {
			if lastGroup != -1 {
				lines = append(lines, "")
			}
			hdr := groupHeaders[group]
			// Use state-specific header for plan (shares priority 1 with permission)
			if effState == "plan" {
				hdr = struct {
					label string
					color color.Color
				}{"PLAN", planColor}
			}
			lines = append(lines, " "+lipgloss.NewStyle().
				Foreground(hdr.color).Bold(true).Render(hdr.label))
			lastGroup = group
		}

		si := stateIcons[effState]
		if si.icon == "" {
			si = stateIcons["idle_prompt"]
		}

		paneID := fmt.Sprintf("%d.%d", agent.Window, agent.Pane)

		repo := agentRepo(agent)
		repoStyled := agentRepoStyled(agent)

		duration := ""
		if effState == "running" {
			duration = state.FormatDuration(agent.UpdatedAt)
		}

		// Truncate repo name if needed (repo only, no branch on this line)
		plainRepo := repo
		if plainRepo == "" {
			plainRepo = agent.Session
		}
		maxRepo := m.leftWidth - 5 - len(paneID) - 2 - len(duration)
		displayRepo := repoStyled
		repoRunes := []rune(plainRepo)
		if maxRepo > 0 && len(repoRunes) > maxRepo {
			displayRepo = string(repoRunes[:maxRepo-1]) + "…"
		}

		icon := lipgloss.NewStyle().Foreground(si.color).Render(si.icon)
		line := fmt.Sprintf("   %s %s %s  %s", icon, paneID, displayRepo, duration)

		if nodeIdx == m.selected {
			line = highlightLine(line, m.leftWidth)
		}

		lines = append(lines, line)

		// Branch on its own line, indented to align under repo name
		if agent.Branch != "" {
			branchIndent := strings.Repeat(" ", 5+len(paneID)+1) // "   " + icon + " " + paneID + " "
			maxBranch := m.leftWidth - len(branchIndent)
			branchStr := agent.Branch
			if maxBranch > 0 && len([]rune(branchStr)) > maxBranch {
				branchStr = string([]rune(branchStr)[:maxBranch-1]) + "…"
			}
			branchLine := branchIndent + styledBranch(branchStr)
			if nodeIdx == m.selected {
				branchLine = highlightLine(branchLine, m.leftWidth)
			}
			lines = append(lines, branchLine)
		}

		// Metadata badges
		badges := agentBadges(agent)
		if badges != "" {
			lines = append(lines, "    "+badges)
		}

		// Collapse indicator if has subagents
		if subs := m.agentSubagents[agent.Target]; len(subs) > 0 && m.collapsed[agent.Target] {
			lines = append(lines, helpStyle.Render(fmt.Sprintf("       ▸ %d subagents (c to expand)", len(subs))))
		}
	}

	// Clamp all lines to panel width to prevent layout overflow
	if m.leftWidth > 0 {
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.leftWidth, "…")
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) filesContent(agent domain.Agent) string {
	if len(agent.FilesChanged) == 0 {
		return helpStyle.Render("  No files changed")
	}
	w := m.rightWidth - 4
	var lines []string
	for _, f := range agent.FilesChanged {
		var clr color.Color
		switch {
		case strings.HasPrefix(f, "+"):
			clr = doneColor
		case strings.HasPrefix(f, "-"):
			clr = errorColor
		default:
			clr = textInputColor
		}
		style := lipgloss.NewStyle().Foreground(clr)
		for _, wl := range wrapText(f, w) {
			lines = append(lines, "  "+style.Render(wl))
		}
	}
	return strings.Join(lines, "\n")
}

// renderHistoryEntry renders a single conversation entry as styled line(s).
func renderHistoryEntry(entry domain.ConversationEntry, w int) string {
	ts := ""
	if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
		ts = t.Local().Format("15:04")
	}

	role := entry.Role
	rStyle := lipgloss.NewStyle().Foreground(runningColor).Bold(true)
	if entry.IsNotification {
		role = "sub-agent"
		rStyle = lipgloss.NewStyle().Foreground(doneColor)
	} else if entry.Role == "human" {
		rStyle = lipgloss.NewStyle().Foreground(textInputColor).Bold(true)
	}

	preview := strings.Split(entry.Content, "\n")[0]
	header := fmt.Sprintf(" %s %s ",
		helpStyle.Render("["+ts+"]"),
		rStyle.Render(role+":"))
	// Wrap the preview text, indenting continuation lines
	wrapped := wrapText(preview, w-len(ts)-len(role)-6)
	var lines []string
	for i, wl := range wrapped {
		if i == 0 {
			lines = append(lines, header+wl)
		} else {
			lines = append(lines, strings.Repeat(" ", len(ts)+len(role)+6)+wl)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) historyContent() string {
	if len(m.conversation) == 0 {
		return helpStyle.Render("  No conversation history")
	}

	w := m.rightWidth - 4

	// Layer 2: return cached string if nothing changed
	if m.renderedHistory != "" &&
		m.historyConvLen == len(m.conversation) &&
		m.historyRightWidth == w {
		return m.renderedHistory
	}

	// Layer 3: incremental append when conversation grew
	if m.renderedHistory != "" &&
		len(m.conversation) > m.historyConvLen &&
		m.historyRightWidth == w {
		var newLines []string
		for _, entry := range m.conversation[m.historyConvLen:] {
			newLines = append(newLines, renderHistoryEntry(entry, w))
		}
		m.renderedHistory = m.renderedHistory + "\n" + strings.Join(newLines, "\n")
		m.historyConvLen = len(m.conversation)
		m.historyRightWidth = w
		return m.renderedHistory
	}

	// Full re-render (first load, agent switch, width change, conversation shrunk)
	var lines []string
	for _, entry := range m.conversation {
		lines = append(lines, renderHistoryEntry(entry, w))
	}
	m.renderedHistory = strings.Join(lines, "\n")
	m.historyConvLen = len(m.conversation)
	m.historyRightWidth = w
	return m.renderedHistory
}

func (m model) waitingMessageContent() string {
	var lines []string
	w := m.rightWidth - 4

	// Always show last assistant message from conversation
	var lastAssistant *domain.ConversationEntry
	for i := len(m.conversation) - 1; i >= 0; i-- {
		if m.conversation[i].Role == "assistant" && !m.conversation[i].IsNotification {
			lastAssistant = &m.conversation[i]
			break
		}
	}
	if lastAssistant == nil {
		return helpStyle.Render("  Waiting for agent message...")
	}
	wrapped := wrapText(lastAssistant.Content, w)
	for _, wl := range wrapped {
		lines = append(lines, "  "+wl)
	}

	lines = append(lines, "")
	if m.mode == modeReply {
		lines = append(lines, " "+lipgloss.NewStyle().Foreground(textInputColor).Bold(true).
			Render("Reply: ")+renderWrappedInput(m.textInput.Value(), m.textInput.Position(), m.rightWidth-12, true, m.availableSkills, "        "))
	} else {
		agent := m.selectedAgent()
		if agent != nil && agent.State == "question" {
			lines = append(lines, " "+helpStyle.Render("Press r to reply, enter to jump to agent"))
		} else {
			lines = append(lines, " "+helpStyle.Render("Press r to reply, y/n for quick answer"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) finalMessageContent() string {
	var lastAssistant *domain.ConversationEntry
	for i := len(m.conversation) - 1; i >= 0; i-- {
		if m.conversation[i].Role == "assistant" && !m.conversation[i].IsNotification {
			lastAssistant = &m.conversation[i]
			break
		}
	}

	if lastAssistant == nil {
		return ""
	}

	var lines []string
	wrapped := wrapText(lastAssistant.Content, m.rightWidth-3)
	for _, wl := range wrapped {
		lines = append(lines, "  "+wl)
	}
	return strings.Join(lines, "\n")
}

func (m model) usageContent() string {
	var lines []string
	lines = append(lines, costStyle.Render("  USAGE BREAKDOWN"))
	lines = append(lines, "")

	// Per-agent usage
	for _, agent := range m.agents {
		u, ok := m.agentUsage[agent.Target]
		if !ok || u.OutputTokens == 0 {
			continue
		}

		label := agentLabelStyled(agent)
		paneID := fmt.Sprintf("%d.%d", agent.Window, agent.Pane)

		lines = append(lines, fmt.Sprintf("  %s %s %s",
			boldStyle.Render(paneID), label, costStyle.Render(usage.FormatCost(u.CostUSD))))
		lines = append(lines, fmt.Sprintf("    in: %s  out: %s  cache-r: %s  cache-w: %s",
			usage.FormatTokens(u.InputTokens),
			usage.FormatTokens(u.OutputTokens),
			usage.FormatTokens(u.CacheReadTokens),
			usage.FormatTokens(u.CacheWriteTokens)))
		if u.Model != "" {
			lines = append(lines, fmt.Sprintf("    model: %s", helpStyle.Render(u.Model)))
		}
		lines = append(lines, "")
	}

	// Daily cost from DB
	if m.db != nil {
		days := m.db.CostByDay(time.Now().AddDate(0, 0, -7))
		if len(days) > 0 {
			const maxBarWidth = 30
			var maxCost float64
			for _, d := range days {
				if d.CostUSD > maxCost {
					maxCost = d.CostUSD
				}
			}

			lines = append(lines, boldStyle.Render("  DAILY COST (7d)"))
			lines = append(lines, "")
			for _, d := range days {
				width := 0
				if maxCost > 0 {
					width = int(float64(maxBarWidth) * d.CostUSD / maxCost)
				}
				bar := strings.Repeat("█", width)
				lines = append(lines, fmt.Sprintf("  %s  %s %s",
					helpStyle.Render(d.Date),
					costStyle.Render(bar),
					usage.FormatCost(d.CostUSD)))
			}
			lines = append(lines, "")
		}
	}

	// Today's accumulated cost
	if m.db != nil && m.dbTodayCost > 0 {
		lines = append(lines, fmt.Sprintf("  Today: %s  │  Session: in %s  out %s",
			costStyle.Render(usage.FormatCost(m.dbTodayCost)),
			usage.FormatTokens(m.totalUsage.InputTokens),
			usage.FormatTokens(m.totalUsage.OutputTokens)))
		lines = append(lines, "")
	}

	// All-time total
	if m.db != nil && m.dbTotalCost > 0 {
		lines = append(lines, fmt.Sprintf("  All-time: %s",
			costStyle.Render(usage.FormatCost(m.dbTotalCost))))
	} else {
		lines = append(lines, fmt.Sprintf("  Session: %s  │  in: %s  out: %s",
			costStyle.Render(usage.FormatCost(m.totalUsage.CostUSD)),
			usage.FormatTokens(m.totalUsage.InputTokens),
			usage.FormatTokens(m.totalUsage.OutputTokens)))
	}

	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("  Press u to close"))

	return strings.Join(lines, "\n")
}

// -- Subagent content builders --

func (m model) subagentFilesContent() string {
	// Extract unique files from tool activity
	seen := make(map[string]bool)
	var files []string
	for _, e := range m.subActivity {
		if e.Kind != "tool" {
			continue
		}
		// Parse "→ ToolName: path" format
		content := e.Content
		if !strings.HasPrefix(content, "→ ") {
			continue
		}
		content = content[len("→ "):]
		parts := strings.SplitN(content, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		tool, detail := parts[0], parts[1]
		if tool == "Read" || tool == "Edit" || tool == "Write" {
			if !seen[detail] {
				seen[detail] = true
				files = append(files, detail)
			}
		}
	}
	if len(files) == 0 {
		return helpStyle.Render("  No files touched")
	}
	w := m.rightWidth - 4
	var lines []string
	style := lipgloss.NewStyle().Foreground(textInputColor)
	for _, f := range files {
		for _, wl := range wrapText(f, w) {
			lines = append(lines, "  "+style.Render(wl))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) subagentActivityContent() string {
	if len(m.subActivity) == 0 {
		return helpStyle.Render("  No activity yet")
	}
	w := m.rightWidth - 4
	var lines []string
	for _, e := range m.subActivity {
		ts := ""
		if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
			ts = t.Local().Format("15:04")
		}
		indent := len(ts) + 10 // approximate prefix width for continuation
		switch e.Kind {
		case "tool":
			header := fmt.Sprintf(" %s ", helpStyle.Render("["+ts+"]"))
			toolStyle := lipgloss.NewStyle().Foreground(themeOverlay0)
			wrapped := wrapText(e.Content, w-indent)
			for i, wl := range wrapped {
				if i == 0 {
					lines = append(lines, header+toolStyle.Render(wl))
				} else {
					lines = append(lines, strings.Repeat(" ", indent)+toolStyle.Render(wl))
				}
			}
		case "human":
			header := fmt.Sprintf(" %s %s ",
				helpStyle.Render("["+ts+"]"),
				lipgloss.NewStyle().Foreground(textInputColor).Bold(true).Render("prompt:"))
			wrapped := wrapText(e.Content, w-indent-8)
			for i, wl := range wrapped {
				if i == 0 {
					lines = append(lines, header+wl)
				} else {
					lines = append(lines, strings.Repeat(" ", indent+8)+wl)
				}
			}
		case "assistant":
			header := fmt.Sprintf(" %s %s ",
				helpStyle.Render("["+ts+"]"),
				lipgloss.NewStyle().Foreground(runningColor).Bold(true).Render("text:"))
			preview := strings.Split(e.Content, "\n")[0]
			wrapped := wrapText(preview, w-indent-6)
			for i, wl := range wrapped {
				if i == 0 {
					lines = append(lines, header+wl)
				} else {
					lines = append(lines, strings.Repeat(" ", indent+6)+wl)
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) subagentOutputContent() string {
	// Find the last assistant text block
	var lastText string
	for i := len(m.subActivity) - 1; i >= 0; i-- {
		if m.subActivity[i].Kind == "assistant" {
			lastText = m.subActivity[i].Content
			break
		}
	}
	if lastText == "" {
		return helpStyle.Render("  No output yet")
	}
	var lines []string
	for _, wl := range wrapText(lastText, m.rightWidth-3) {
		lines = append(lines, "  "+wl)
	}
	return strings.Join(lines, "\n")
}

// -- View --

func (m model) View() tea.View {
	makeView := func(content string) tea.View {
		v := tea.NewView(content)
		v.AltScreen = true
		switch m.mode {
		case modeReply, modeCreateFolder, modeCreateSkill, modeCreateMessage:
			v.MouseMode = tea.MouseModeNone
		default:
			v.MouseMode = tea.MouseModeCellMotion
		}
		return v
	}

	if m.width == 0 || m.height == 0 {
		return makeView("Loading...")
	}

	banner := m.renderBanner()

	if m.helpVisible {
		overlay := m.renderHelpOverlay()
		helpBar := m.renderHelpBar()
		return makeView(lipgloss.JoinVertical(lipgloss.Left, banner, overlay, helpBar))
	}

	var left, right string
	if m.diffVisible {
		left = m.renderDiffFilePanel()
		right = m.renderDiffContentPanel()
	} else {
		left = m.renderLeftPanel()
		right = m.renderRightPanel()
	}
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	help := m.renderHelpBar()

	return makeView(lipgloss.JoinVertical(lipgloss.Left, banner, main, help))
}

func (m model) renderLeftPanel() string {
	panelHeight := m.height - 5 - m.bannerHeight()
	style := borderStyle
	if m.focusedVP == focusAgentList {
		style = style.BorderForeground(themeSapphire)
	}

	if m.petEnabled {
		content := lipgloss.JoinVertical(lipgloss.Left,
			m.agentListVP.View(),
			m.pet.View(),
		)
		return style.
			Width(m.leftWidth + 2).
			Height(panelHeight + 2).
			Render(content)
	}

	return style.
		Width(m.leftWidth + 2).
		Height(panelHeight + 2).
		Render(m.agentListVP.View())
}

func (m model) renderRightPanel() string {
	panelHeight := m.height - 5 - m.bannerHeight()

	// Create wizard modes: simple form
	if m.mode == modeCreateFolder || m.mode == modeCreateSkill || m.mode == modeCreateMessage {
		return borderStyle.
			Width(m.rightWidth + 2).
			Height(panelHeight + 2).
			Render(m.messageVP.View())
	}

	agent := m.selectedAgent()
	if agent == nil {
		return borderStyle.
			Width(m.rightWidth + 2).
			Height(panelHeight + 2).
			Render(m.messageVP.View())
	}

	sub := m.selectedSubagent()

	// Header (not in a viewport — static)
	var header []string

	if sub != nil {
		// Subagent header
		header = append(header, titleStyle.Render(fmt.Sprintf(" %s: %s ", sub.AgentType, sub.Description)))
		header = append(header, "")
		header = append(header, fmt.Sprintf(" Parent: %d.%d %s", agent.Window, agent.Pane, agent.Branch))
		header = append(header, "")
	} else {
		// Parent agent header
		projectTitleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(themeText)

		repo := agentRepo(*agent)
		if repo == "" {
			repo = agent.Target
		}
		header = append(header, " "+projectTitleStyle.Render(repo))
		header = append(header, "")

		effState := agent.State
		si := stateIcons[effState]
		if si.icon == "" {
			si = stateIcons["idle_prompt"]
		}
		stateLabel := stateLabels[effState]
		if stateLabel == "" {
			stateLabel = effState
		}
		stateStr := lipgloss.NewStyle().Foreground(si.color).Bold(true).
			Render(fmt.Sprintf("%s %s", si.icon, stateLabel))

		metaParts := []string{stateStr}
		if agent.Model != "" {
			metaParts = append(metaParts, helpStyle.Render(agent.Model))
		}
		if agent.PermissionMode != "" && agent.PermissionMode != "default" {
			metaParts = append(metaParts, permissionModeStyle(agent.PermissionMode))
		}
		header = append(header, " "+strings.Join(metaParts, helpStyle.Render(" | ")))
		header = append(header, "")

		const metaLabelWidth = 9

		if agent.Branch != "" {
			header = append(header, fmt.Sprintf(" %s %s", padLabel("branch", metaLabelWidth), styledBranch(agent.Branch)))
		}
		if dir := agent.EffectiveDir(); dir != "" {
			header = append(header, wrapMetaLine("dir", metaLabelWidth, dir, m.rightWidth-4)...)
		}
		header = append(header, "")

		if u, ok := m.agentUsage[agent.Target]; ok && u.OutputTokens > 0 {
			costValue := fmt.Sprintf("%s  (in: %s  out: %s  cache: %s)",
				costStyle.Render(usage.FormatCost(u.CostUSD)),
				usage.FormatTokens(u.InputTokens),
				usage.FormatTokens(u.OutputTokens),
				usage.FormatTokens(u.CacheReadTokens+u.CacheWriteTokens))
			header = append(header, wrapMetaLine("cost", metaLabelWidth, costValue, m.rightWidth-4)...)
		}

		if agent.SubagentCount > 0 {
			header = append(header, fmt.Sprintf(" %s %s active",
				padLabel("agents", metaLabelWidth),
				lipgloss.NewStyle().Foreground(runningColor).Bold(true).
					Render(fmt.Sprintf("%d", agent.SubagentCount))))
		}
		header = append(header, "")
	}

	// Dynamically compute viewport heights based on actual header size.
	// This value receiver works on a copy, so mutations are render-local.
	headerStr := strings.Join(header, "\n")
	actualHeaderLines := strings.Count(headerStr, "\n") + 1
	filesH, historyH, msgH := panelHeights(panelHeight, actualHeaderLines)
	m.filesVP.SetHeight(filesH)
	m.historyVP.SetHeight(historyH)
	m.messageVP.SetHeight(msgH)

	// Section labels + viewports
	focusMarker := func(vp int) string {
		if m.focusedVP == vp {
			return lipgloss.NewStyle().Foreground(themeSapphire).Render(" ◆")
		}
		return ""
	}

	scrollHint := func(vp viewport.Model) string {
		var hints []string
		if !vp.AtTop() {
			hints = append(hints, "▲")
		}
		if !vp.AtBottom() {
			hints = append(hints, "▼")
		}
		if len(hints) == 0 {
			return ""
		}
		return " " + helpStyle.Render(strings.Join(hints, " "))
	}

	var filesLabel, historyLabel, messageLabel string

	if m.mode == modeUsage {
		filesLabel = ""
		historyLabel = ""
		messageLabel = " " + lipgloss.NewStyle().Foreground(themePeach).Bold(true).
			Render("── domain.Usage") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
			" " + helpStyle.Render(strings.Repeat("─", 20))
	} else if sub != nil {
		filesLabel = " " + boldStyle.Render("── Files Touched") + focusMarker(focusFiles) + scrollHint(m.filesVP) +
			" " + helpStyle.Render(strings.Repeat("─", 12))
		historyLabel = " " + boldStyle.Render("── Activity") + focusMarker(focusHistory) + scrollHint(m.historyVP) +
			" " + helpStyle.Render(strings.Repeat("─", 17))
		messageLabel = " " + boldStyle.Render("── Output") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
			" " + helpStyle.Render(strings.Repeat("─", 19))
	} else {
		rpEffState := agent.State

		filesLabel = " " + boldStyle.Render("Files:") + focusMarker(focusFiles) + scrollHint(m.filesVP)
		historyLabel = " " + boldStyle.Render("── History") + focusMarker(focusHistory) + scrollHint(m.historyVP) +
			" " + helpStyle.Render(strings.Repeat("─", 18))

		if m.planVisible && m.renderedPlan != "" {
			label := "── Plan (p to close)"
			if rpEffState == "plan" {
				label = "── Plan ready (y approve, r feedback)"
			}
			messageLabel = " " + planLabelStyle.Render(label) + focusMarker(focusMessage) + scrollHint(m.messageVP)
		} else if isBlocked(rpEffState) {
			blockColor := permissionColor
			blockLabel := "── Agent is blocked"
			if rpEffState == "plan" {
				blockColor = planColor
				blockLabel = "── Plan ready for review"
			}
			messageLabel = " " + lipgloss.NewStyle().Foreground(blockColor).Bold(true).
				Render(blockLabel) + focusMarker(focusMessage) + scrollHint(m.messageVP)
		} else if isWaiting(rpEffState) {
			messageLabel = " " + lipgloss.NewStyle().Foreground(questionColor).Bold(true).
				Render("── Agent is waiting") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 9))
		} else if isReview(rpEffState) || isPR(rpEffState) || isMerged(rpEffState) {
			if m.mode == modeReply {
				messageLabel = " " + lipgloss.NewStyle().Foreground(questionColor).Bold(true).
					Render("── Agent is waiting") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
					" " + helpStyle.Render(strings.Repeat("─", 9))
			} else if isMerged(rpEffState) {
				messageLabel = " " + lipgloss.NewStyle().Foreground(mergedColor).Bold(true).
					Render("── Merged (x to close)") + focusMarker(focusMessage) + scrollHint(m.messageVP)
			} else if isPR(rpEffState) {
				messageLabel = " " + lipgloss.NewStyle().Foreground(prColor).Bold(true).
					Render("── PR open (m to merge)") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
					" " + helpStyle.Render(strings.Repeat("─", 6))
			} else {
				messageLabel = " " + lipgloss.NewStyle().Foreground(doneColor).Bold(true).
					Render("── Final message") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
					" " + helpStyle.Render(strings.Repeat("─", 12))
			}
		} else {
			messageLabel = " " + boldStyle.Render("── Live") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 21))
		}
	}

	// Compose right panel (with blank-line buffers between sections)
	var parts []string
	if m.mode == modeUsage {
		// Single-section layout: header + label + messageVP
		m.messageVP.SetHeight(max(panelHeight-actualHeaderLines-1, minMessageHeight))
		parts = []string{
			headerStr,
			messageLabel,
			m.messageVP.View(),
		}
	} else if m.planVisible && m.renderedPlan != "" {
		m.messageVP.SetHeight(max(panelHeight-actualHeaderLines-1, minMessageHeight))
		parts = []string{
			headerStr,
			messageLabel,
			m.messageVP.View(),
		}
	} else {
		parts = []string{
			headerStr,
			filesLabel,
			m.filesVP.View(),
			"",
			historyLabel,
			m.historyVP.View(),
			"",
			messageLabel,
			m.messageVP.View(),
		}
	}
	content := strings.Join(parts, "\n")

	return borderStyle.
		Width(m.rightWidth + 2).
		Height(panelHeight + 2).
		Render(content)
}

// statusLine returns the status message line (spawning spinner, errors, etc).
func (m model) statusLine() string {
	if m.statusMsg == "spawning" {
		return " " +
			m.spawningSpinner.View() +
			" " +
			lipgloss.NewStyle().Foreground(themeSapphire).Render("Spawning agent...")
	}
	if m.statusMsg != "" {
		clr := themeGreen
		if m.statusIsError {
			clr = errorColor
		}
		return " " + lipgloss.NewStyle().Foreground(clr).Render(m.statusMsg)
	}
	return ""
}

func (m model) renderHelpBar() string {
	var parts []string

	// Today's accumulated cost
	todayCost := m.dbTodayCost
	if todayCost > 0 {
		todayStr := lipgloss.NewStyle().Foreground(themePeach).Bold(true).
			Render(usage.FormatCost(todayCost))
		parts = append(parts, fmt.Sprintf("Today: %s", todayStr))
		parts = append(parts, "│")
	}

	// All-time total
	totalCost := m.dbTotalCost
	if totalCost == 0 {
		totalCost = m.totalUsage.CostUSD
	}
	if totalCost > 0 {
		costStr := lipgloss.NewStyle().Foreground(themePeach).Bold(true).
			Render(usage.FormatCost(totalCost))
		parts = append(parts, fmt.Sprintf("All-time: %s", costStr))
		parts = append(parts, "│")
	}

	// Help overlay active: minimal bar
	if m.helpVisible {
		parts = append(parts, boldStyle.Render("h/esc")+" close help")
		return m.composeHelpBarWithStatus(helpStyle.Render("  " + strings.Join(parts, "  ")))
	}

	if m.diffVisible {
		parts = append(parts, boldStyle.Render("^u/^d")+" scroll")
		parts = append(parts, boldStyle.Render("J/K")+" line scroll")
		parts = append(parts, boldStyle.Render("q/d/esc")+" close")
		parts = append(parts, boldStyle.Render("h")+" help")
		return m.composeHelpBarWithStatus(helpStyle.Render("  " + strings.Join(parts, "  ")))
	}

	if m.mode == modeConfirmMerge {
		parts = append(parts, boldStyle.Render("y")+" merge")
		parts = append(parts, boldStyle.Render("n/esc")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeConfirmCleanup {
		parts = append(parts, boldStyle.Render("y")+" cleanup")
		parts = append(parts, boldStyle.Render("n/esc")+" skip")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeConfirmSend {
		parts = append(parts, boldStyle.Render("enter")+" confirm")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeConfirmJump {
		parts = append(parts, boldStyle.Render("y/enter")+" jump")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeCreateFolder {
		parts = append(parts, boldStyle.Render("enter")+" create")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeCreateSkill {
		parts = append(parts, boldStyle.Render("enter")+" select")
		parts = append(parts, boldStyle.Render("↑↓")+" cycle")
		parts = append(parts, boldStyle.Render("esc")+" back")
		parts = append(parts, boldStyle.Render("^c")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeCreateMessage {
		parts = append(parts, boldStyle.Render("enter")+" launch")
		parts = append(parts, boldStyle.Render("esc")+" back")
		parts = append(parts, boldStyle.Render("^c")+" cancel")
		return m.truncateHelpBar(parts)
	}

	if m.mode == modeReply {
		parts = append(parts, boldStyle.Render("enter")+" send")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return m.truncateHelpBar(parts)
	}

	// Plan state: show plan-specific hints
	if agent := m.selectedAgent(); agent != nil && agent.State == "plan" {
		parts = append(parts, boldStyle.Render("y")+" approve")
		parts = append(parts, boldStyle.Render("r")+" feedback")
		parts = append(parts, boldStyle.Render("p")+" toggle plan")
		parts = append(parts, boldStyle.Render("h")+" help")
		return m.truncateHelpBar(parts)
	}

	// Normal mode: essential lifecycle hints for first-timers
	parts = append(parts, boldStyle.Render("a")+" new")
	if m.tmuxAvailable {
		parts = append(parts, boldStyle.Render("enter")+" jump")
	}
	parts = append(parts, boldStyle.Render("x")+" close")
	parts = append(parts, boldStyle.Render("d")+" diff")
	parts = append(parts, boldStyle.Render("o")+" open")
	parts = append(parts, boldStyle.Render("g")+" PR")
	parts = append(parts, boldStyle.Render("h")+" help")
	parts = append(parts, boldStyle.Render("q")+" quit")

	return m.truncateHelpBar(parts)
}

// composeHelpBarWithStatus takes the left-aligned help text and appends
// the status line right-aligned, padding to fill the terminal width.
func (m model) composeHelpBarWithStatus(leftContent string) string {
	status := m.statusLine()
	if status == "" {
		return leftContent
	}

	leftW := lipgloss.Width(leftContent)
	statusW := lipgloss.Width(status)
	gap := m.width - leftW - statusW
	if gap < 2 {
		gap = 2
	}
	return leftContent + strings.Repeat(" ", gap) + status
}

// truncateHelpBar joins help bar parts and truncates to fit within the terminal width.
// It reserves space for the status line on the right side.
func (m model) truncateHelpBar(parts []string) string {
	statusText := m.statusLine()
	statusW := lipgloss.Width(statusText)
	reserveRight := 0
	if statusW > 0 {
		reserveRight = statusW + 2 // +2 for gap
	}

	maxWidth := m.width - 2 - reserveRight // leave room for leading padding and status
	if maxWidth <= 0 {
		return ""
	}

	var included []string
	totalWidth := 0
	for _, p := range parts {
		rendered := helpStyle.Render(p)
		w := lipgloss.Width(rendered) + 2 // +2 for "  " separator
		if totalWidth+w > maxWidth && len(included) > 0 {
			break
		}
		included = append(included, p)
		totalWidth += w
	}

	leftContent := helpStyle.Render("  " + strings.Join(included, "  "))
	return m.composeHelpBarWithStatus(leftContent)
}

// renderHelpOverlay renders a full-screen help legend with all keybindings grouped by context.
func (m model) renderHelpOverlay() string {
	panelHeight := m.height - 5 - m.bannerHeight() // matches resizeViewports
	contentWidth := m.width - 4                    // account for border

	headerStyle := titleStyle
	keyStyle := boldStyle
	descStyle := helpStyle

	line := func(key, desc string) string {
		return fmt.Sprintf("  %s  %s", keyStyle.Render(fmt.Sprintf("%-12s", key)), descStyle.Render(desc))
	}

	var lines []string

	// Navigation
	lines = append(lines, headerStyle.Render("  Navigation"))
	lines = append(lines, line("↑ / k", "Previous agent"))
	lines = append(lines, line("↓ / j", "Next agent"))
	lines = append(lines, line("⇧↑ / ⇧↓", "Jump to parent agent"))
	lines = append(lines, line("tab", "Cycle focus forward"))
	lines = append(lines, line("⇧tab", "Cycle focus backward"))
	lines = append(lines, line("^u / ^d", "Half-page scroll"))
	lines = append(lines, line("J / K", "Line scroll (plan/diff)"))
	lines = append(lines, "")

	// Agent Actions
	lines = append(lines, headerStyle.Render("  Agent Actions"))
	lines = append(lines, line("enter", "Jump to agent pane"))
	lines = append(lines, line("r", "Reply to agent"))
	lines = append(lines, line("e", "Open editor"))
	lines = append(lines, line("o", "Open dir in tmux window"))
	lines = append(lines, line("a", "Create new session"))
	lines = append(lines, line("x", "Close/dismiss agent"))
	if m.ghAvailable {
		lines = append(lines, line("m", "Merge PR (squash) + cleanup"))
	} else {
		lines = append(lines, line("m", "Mark merged + send cleanup"))
	}
	lines = append(lines, line("c", "Collapse/expand subagents"))
	lines = append(lines, "")

	// View Controls
	lines = append(lines, headerStyle.Render("  View Controls"))
	lines = append(lines, line("p", "Toggle plan view"))
	lines = append(lines, line("u", "Toggle usage view"))
	lines = append(lines, line("d", "View diff"))
	lines = append(lines, line("g", "Open PR in browser"))
	lines = append(lines, line("h", "Toggle this help"))
	lines = append(lines, "")

	// Diff Mode
	lines = append(lines, headerStyle.Render("  Diff Mode"))
	lines = append(lines, line("↑ / k", "Previous file"))
	lines = append(lines, line("↓ / j", "Next file"))
	lines = append(lines, line("e", "Toggle expand all"))
	lines = append(lines, line("^u / ^d", "Scroll"))
	lines = append(lines, line("J / K", "Line scroll"))
	lines = append(lines, line("d / esc", "Close diff"))
	lines = append(lines, "")

	// Input Modes
	lines = append(lines, headerStyle.Render("  Input Modes"))
	lines = append(lines, line("enter", "Send reply / create session"))
	lines = append(lines, line("tab", "Auto-complete (create mode)"))
	lines = append(lines, line("esc", "Cancel"))
	lines = append(lines, "")

	// Quit
	lines = append(lines, line("q / ^c", "Quit dashboard"))

	content := strings.Join(lines, "\n")

	style := borderStyle.
		Width(contentWidth + 2).
		Height(panelHeight + 2)

	return style.Render(content)
}
