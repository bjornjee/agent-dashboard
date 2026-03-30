package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// -- Content Builders --

func (m *model) updateLeftContent() {
	m.agentListVP.SetContent(m.agentListContent())
}

func (m *model) updateRightContent() {
	// Override modes use the full panel height since they replace all three viewports.
	// Normal mode restores the standard message viewport height.
	panelHeight := m.height - 5 - bannerHeight // matches resizeViewports
	if m.mode == modeCreateFolder || (m.planVisible && m.renderedPlan != "") {
		fullHeight := panelHeight - headerLines - 2
		if fullHeight < 3 {
			fullHeight = 3
		}
		m.messageVP.Height = fullHeight
	} else {
		msgHeight := panelHeight - headerLines - filesVPHeight - historyVPHeight - sectionGaps
		if msgHeight < 3 {
			msgHeight = 3
		}
		m.messageVP.Height = msgHeight
	}

	// Create folder mode overrides right panel (works even with no agents)
	if m.mode == modeCreateFolder {
		var lines []string
		lines = append(lines, "")
		lines = append(lines, "  "+titleStyle.Render(" CREATE NEW SESSION "))
		lines = append(lines, "")
		lines = append(lines, "  "+boldStyle.Render("Git folder path:"))
		lines = append(lines, "  "+m.textInput.View())
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

	agent := m.selectedAgent()
	if agent == nil {
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent("  No agents found")
		return
	}

	// Usage mode overrides right panel content
	if m.mode == modeUsage {
		m.filesVP.SetContent("")
		m.historyVP.SetContent("")
		m.messageVP.SetContent(m.usageContent())
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

	effState := m.effectiveState(*agent)
	needsAttention := effState == "input" || effState == "error"
	isDone := effState == "done" || effState == "idle"

	if m.planVisible && m.renderedPlan != "" {
		m.messageVP.SetContent(m.renderedPlan)
	} else if needsAttention {
		m.messageVP.SetContent(m.waitingMessageContent())
	} else if isDone {
		m.messageVP.SetContent(m.finalMessageContent())
	} else if m.tmuxAvailable && hasContent(m.capturedLines) {
		var lines []string
		for _, l := range m.capturedLines {
			lines = append(lines, " "+l)
		}
		m.messageVP.SetContent(strings.Join(lines, "\n"))
	} else {
		m.messageVP.SetContent("")
	}
}

func (m model) agentListContent() string {
	var lines []string

	if len(m.treeNodes) == 0 {
		lines = append(lines, "  No agents found")
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
				maxDesc := m.leftWidth - 12 - len(subLabel)
				desc := node.Sub.Description
				if maxDesc > 0 && len(desc) > maxDesc {
					desc = desc[:maxDesc-1] + "…"
				}
				subLabel += ": " + desc
			}

			line := fmt.Sprintf("       %s %s %s", helpStyle.Render(prefix), subIcon, subLabel)
			if nodeIdx == m.selected {
				line = selectedStyle.Render(fmt.Sprintf("       %s ▶ %s", prefix, subLabel))
			}
			lines = append(lines, line)
			continue
		}

		// Parent agent node
		effState := m.effectiveState(agent)
		group := statePriority[effState]
		if group == 0 {
			group = 3
		}

		if group != lastGroup {
			if lastGroup != -1 {
				lines = append(lines, "")
			}
			hdr := groupHeaders[group]
			lines = append(lines, " "+lipgloss.NewStyle().
				Foreground(hdr.color).Bold(true).Render(hdr.label))
			lastGroup = group
		}

		si := stateIcons[effState]
		if si.icon == "" {
			si = stateIcons["idle"]
		}

		paneID := fmt.Sprintf("%d.%d", agent.Window, agent.Pane)

		label := agentLabel(agent)

		duration := ""
		if effState == "running" {
			duration = FormatDuration(agent.UpdatedAt)
		}

		maxLabel := m.leftWidth - 5 - len(paneID) - 2 - len(duration)
		if maxLabel > 0 && len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		icon := lipgloss.NewStyle().Foreground(si.color).Render(si.icon)
		line := fmt.Sprintf("   %s %s %s  %s", icon, paneID, label, duration)

		if nodeIdx == m.selected {
			line = selectedStyle.Render(fmt.Sprintf("  %s %s %s  %s", si.icon, paneID, label, duration))
		}

		lines = append(lines, line)

		// Metadata badges
		badges := agentBadges(agent)
		if badges != "" {
			lines = append(lines, "       "+badges)
		}

		// Collapse indicator if has subagents
		if subs := m.agentSubagents[agent.Target]; len(subs) > 0 && m.collapsed[agent.Target] {
			lines = append(lines, helpStyle.Render(fmt.Sprintf("       ▸ %d subagents (c to expand)", len(subs))))
		}
	}

	return strings.Join(lines, "\n")
}

func (m model) filesContent(agent Agent) string {
	if len(agent.FilesChanged) == 0 {
		return helpStyle.Render("  No files changed")
	}
	var lines []string
	for _, f := range agent.FilesChanged {
		var color lipgloss.Color
		switch {
		case strings.HasPrefix(f, "+"):
			color = doneColor
		case strings.HasPrefix(f, "-"):
			color = errorColor
		default:
			color = inputColor
		}
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(color).Render(f))
	}
	return strings.Join(lines, "\n")
}

func (m model) historyContent() string {
	if len(m.conversation) == 0 {
		return helpStyle.Render("  No conversation history")
	}

	var lines []string
	for _, entry := range m.conversation {
		ts := ""
		if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
			ts = t.Local().Format("15:04")
		}

		role := entry.Role
		roleStyle := lipgloss.NewStyle().Foreground(runningColor).Bold(true)
		if entry.IsNotification {
			role = "sub-agent"
			roleStyle = lipgloss.NewStyle().Foreground(doneColor)
		} else if entry.Role == "human" {
			roleStyle = lipgloss.NewStyle().Foreground(inputColor).Bold(true)
		}

		preview := strings.Split(entry.Content, "\n")[0]
		if len(preview) > 120 {
			preview = preview[:119] + "…"
		}

		lines = append(lines, fmt.Sprintf(" %s %s %s",
			helpStyle.Render("["+ts+"]"),
			roleStyle.Render(role+":"),
			preview))
	}
	return strings.Join(lines, "\n")
}

func (m model) waitingMessageContent() string {
	var lastAssistant *ConversationEntry
	for i := len(m.conversation) - 1; i >= 0; i-- {
		if m.conversation[i].Role == "assistant" && !m.conversation[i].IsNotification {
			lastAssistant = &m.conversation[i]
			break
		}
	}

	if lastAssistant == nil {
		return helpStyle.Render("  Waiting for agent message...")
	}

	var lines []string
	wrapped := wrapText(lastAssistant.Content, m.rightWidth-3)
	for _, wl := range wrapped {
		lines = append(lines, "  "+wl)
	}

	lines = append(lines, "")
	if m.mode == modeReply {
		lines = append(lines, " "+lipgloss.NewStyle().Foreground(inputColor).Bold(true).
			Render("Reply: ")+m.textInput.View())
	} else {
		lines = append(lines, " "+helpStyle.Render("Press r to reply, y/n for quick answer"))
	}

	return strings.Join(lines, "\n")
}

func (m model) finalMessageContent() string {
	var lastAssistant *ConversationEntry
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

		label := agentLabel(agent)
		paneID := fmt.Sprintf("%d.%d", agent.Window, agent.Pane)

		lines = append(lines, fmt.Sprintf("  %s %s %s",
			boldStyle.Render(paneID), label, costStyle.Render(FormatCost(u.CostUSD))))
		lines = append(lines, fmt.Sprintf("    in: %s  out: %s  cache-r: %s  cache-w: %s",
			FormatTokens(u.InputTokens),
			FormatTokens(u.OutputTokens),
			FormatTokens(u.CacheReadTokens),
			FormatTokens(u.CacheWriteTokens)))
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
					FormatCost(d.CostUSD)))
			}
			lines = append(lines, "")
		}
	}

	// Today's accumulated cost
	if m.db != nil && m.dbTodayCost > 0 {
		lines = append(lines, fmt.Sprintf("  Today: %s  │  Session: in %s  out %s",
			costStyle.Render(FormatCost(m.dbTodayCost)),
			FormatTokens(m.totalUsage.InputTokens),
			FormatTokens(m.totalUsage.OutputTokens)))
		lines = append(lines, "")
	}

	// All-time total
	if m.db != nil && m.dbTotalCost > 0 {
		lines = append(lines, fmt.Sprintf("  All-time: %s",
			costStyle.Render(FormatCost(m.dbTotalCost))))
	} else {
		lines = append(lines, fmt.Sprintf("  Session: %s  │  in: %s  out: %s",
			costStyle.Render(FormatCost(m.totalUsage.CostUSD)),
			FormatTokens(m.totalUsage.InputTokens),
			FormatTokens(m.totalUsage.OutputTokens)))
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
	var lines []string
	for _, f := range files {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(inputColor).Render(f))
	}
	return strings.Join(lines, "\n")
}

func (m model) subagentActivityContent() string {
	if len(m.subActivity) == 0 {
		return helpStyle.Render("  No activity yet")
	}
	var lines []string
	for _, e := range m.subActivity {
		ts := ""
		if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
			ts = t.Local().Format("15:04")
		}
		switch e.Kind {
		case "tool":
			lines = append(lines, fmt.Sprintf(" %s %s",
				helpStyle.Render("["+ts+"]"),
				lipgloss.NewStyle().Foreground(themeOverlay0).Render(e.Content)))
		case "human":
			lines = append(lines, fmt.Sprintf(" %s %s %s",
				helpStyle.Render("["+ts+"]"),
				lipgloss.NewStyle().Foreground(inputColor).Bold(true).Render("prompt:"),
				truncateLineStr(e.Content, m.rightWidth-20)))
		case "assistant":
			preview := strings.Split(e.Content, "\n")[0]
			lines = append(lines, fmt.Sprintf(" %s %s %s",
				helpStyle.Render("["+ts+"]"),
				lipgloss.NewStyle().Foreground(runningColor).Bold(true).Render("text:"),
				truncateLineStr(preview, m.rightWidth-20)))
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

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	banner := m.renderBanner()
	left := m.renderLeftPanel()
	right := m.renderRightPanel()
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	help := m.renderHelpBar()

	return lipgloss.JoinVertical(lipgloss.Left, banner, main, help)
}

func (m model) renderLeftPanel() string {
	panelHeight := m.height - 5 - bannerHeight
	style := borderStyle
	if m.focusedVP == focusAgentList {
		style = style.BorderForeground(themeSapphire)
	}
	return style.
		Width(m.leftWidth).
		Height(panelHeight).
		Render(m.agentListVP.View())
}

func (m model) renderRightPanel() string {
	panelHeight := m.height - 5 - bannerHeight

	// Create folder mode: simple form
	if m.mode == modeCreateFolder {
		return borderStyle.
			Width(m.rightWidth).
			Height(panelHeight).
			Render(m.messageVP.View())
	}

	agent := m.selectedAgent()
	if agent == nil {
		return borderStyle.
			Width(m.rightWidth).
			Height(panelHeight).
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
		name := agentLabel(*agent)
		if name == "" {
			name = agent.Target
		}
		header = append(header, titleStyle.Render(fmt.Sprintf(" PEEK: %s ", name)))
		header = append(header, "")

		effState := m.effectiveState(*agent)
		si := stateIcons[effState]
		if si.icon == "" {
			si = stateIcons["idle"]
		}
		stateLabel := map[string]string{
			"input": "Waiting for input", "error": "Error",
			"running": "Running", "done": "Done", "idle": "Idle",
		}[effState]
		if stateLabel == "" {
			stateLabel = agent.State
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
		header = append(header, " "+strings.Join(metaParts, helpStyle.Render(" │ ")))
		header = append(header, "")

		if agent.Branch != "" {
			header = append(header, fmt.Sprintf(" Branch: %s", boldStyle.Render(agent.Branch)))
		}
		if agent.Cwd != "" {
			header = append(header, fmt.Sprintf(" Dir: %s", agent.Cwd))
		}

		if u, ok := m.agentUsage[agent.Target]; ok && u.OutputTokens > 0 {
			header = append(header, fmt.Sprintf(" Cost: %s  (in: %s  out: %s  cache: %s)",
				boldStyle.Render(FormatCost(u.CostUSD)),
				FormatTokens(u.InputTokens),
				FormatTokens(u.OutputTokens),
				FormatTokens(u.CacheReadTokens+u.CacheWriteTokens)))
		}

		if agent.SubagentCount > 0 {
			header = append(header, fmt.Sprintf(" Subagents: %s active",
				lipgloss.NewStyle().Foreground(runningColor).Bold(true).
					Render(fmt.Sprintf("%d", agent.SubagentCount))))
		}
		header = append(header, "")
	}

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
			Render("── Usage") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
			" " + helpStyle.Render(strings.Repeat("─", 20))
	} else if sub != nil {
		filesLabel = " " + boldStyle.Render("── Files Touched") + focusMarker(focusFiles) + scrollHint(m.filesVP) +
			" " + helpStyle.Render(strings.Repeat("─", 12))
		historyLabel = " " + boldStyle.Render("── Activity") + focusMarker(focusHistory) + scrollHint(m.historyVP) +
			" " + helpStyle.Render(strings.Repeat("─", 17))
		messageLabel = " " + boldStyle.Render("── Output") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
			" " + helpStyle.Render(strings.Repeat("─", 19))
	} else {
		rpEffState := m.effectiveState(*agent)
		needsAttention := rpEffState == "input" || rpEffState == "error"
		isDone := rpEffState == "done" || rpEffState == "idle"

		filesLabel = " " + boldStyle.Render("Files:") + focusMarker(focusFiles) + scrollHint(m.filesVP)
		historyLabel = " " + boldStyle.Render("── History") + focusMarker(focusHistory) + scrollHint(m.historyVP) +
			" " + helpStyle.Render(strings.Repeat("─", 18))

		if m.planVisible && m.renderedPlan != "" {
			messageLabel = " " + planLabelStyle.Render("── Plan (p to close)") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 8))
		} else if needsAttention {
			messageLabel = " " + lipgloss.NewStyle().Foreground(inputColor).Bold(true).
				Render("── Agent is waiting") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 9))
		} else if isDone {
			messageLabel = " " + lipgloss.NewStyle().Foreground(doneColor).Bold(true).
				Render("── Final message") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 12))
		} else {
			messageLabel = " " + boldStyle.Render("── Live") + focusMarker(focusMessage) + scrollHint(m.messageVP) +
				" " + helpStyle.Render(strings.Repeat("─", 21))
		}
	}

	// Status message
	statusLine := ""
	if m.statusMsg != "" {
		statusLine = " " + lipgloss.NewStyle().Foreground(errorColor).Render(m.statusMsg)
	}

	// Compose right panel (with blank-line buffers between sections)
	var parts []string
	if m.mode == modeUsage {
		parts = []string{
			strings.Join(header, "\n"),
			messageLabel,
			m.messageVP.View(),
		}
	} else if m.planVisible && m.renderedPlan != "" {
		parts = []string{
			strings.Join(header, "\n"),
			messageLabel,
			m.messageVP.View(),
		}
	} else {
		parts = []string{
			strings.Join(header, "\n"),
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
	if statusLine != "" {
		parts = append(parts, statusLine)
	}

	content := strings.Join(parts, "\n")

	return borderStyle.
		Width(m.rightWidth).
		Height(panelHeight).
		Render(content)
}

func (m model) renderHelpBar() string {
	var parts []string

	// Today's accumulated cost
	todayCost := m.dbTodayCost
	if todayCost > 0 {
		todayStr := lipgloss.NewStyle().Foreground(themePeach).Bold(true).
			Render(FormatCost(todayCost))
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
			Render(FormatCost(totalCost))
		parts = append(parts, fmt.Sprintf("All-time: %s", costStr))
		parts = append(parts, "│")
	}

	parts = append(parts, boldStyle.Render("↑/↓")+" navigate")

	if m.mode == modeCreateFolder {
		parts = append(parts, boldStyle.Render("enter")+" create")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return helpStyle.Render("  " + strings.Join(parts, "  "))
	}

	if m.mode == modeReply {
		parts = append(parts, boldStyle.Render("enter")+" send")
		parts = append(parts, boldStyle.Render("esc")+" cancel")
		return helpStyle.Render("  " + strings.Join(parts, "  "))
	}

	if m.tmuxAvailable {
		parts = append(parts, boldStyle.Render("enter")+" jump")
		parts = append(parts, boldStyle.Render("r")+" reply")
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil {
			es := m.effectiveState(*agent)
			if es == "input" || es == "error" {
				parts = append(parts, boldStyle.Render("y/n")+" quick answer")
			}
		}
	} else {
		parts = append(parts, helpStyle.Render("enter")+" "+helpStyle.Render("jump"))
		parts = append(parts, helpStyle.Render("r")+" "+helpStyle.Render("reply"))
	}
	if m.planContent != "" && m.selectedSubagent() == nil {
		parts = append(parts, boldStyle.Render("p")+" plan")
	}
	parts = append(parts, boldStyle.Render("a")+" new")
	parts = append(parts, boldStyle.Render("u")+" usage")
	parts = append(parts, boldStyle.Render("c")+" collapse")
	parts = append(parts, boldStyle.Render("x")+" close/dismiss")
	parts = append(parts, boldStyle.Render("⇧↑/⇧↓")+" next agent")
	parts = append(parts, boldStyle.Render("tab")+" focus")
	parts = append(parts, boldStyle.Render("^u/^d")+" scroll")
	parts = append(parts, boldStyle.Render("q")+" quit")

	return helpStyle.Render("  " + strings.Join(parts, "  "))
}
