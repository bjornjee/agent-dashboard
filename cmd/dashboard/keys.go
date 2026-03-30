package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	leftBorderEnd := m.leftWidth + 2

	if msg.X < leftBorderEnd {
		var cmd tea.Cmd
		m.agentListVP, cmd = m.agentListVP.Update(msg)
		return m, cmd
	}

	// Route to inner right viewport based on Y position
	// Header takes ~headerLines rows + 1 border
	rightStart := 1 + bannerHeight // top border + banner
	filesStart := rightStart + headerLines
	historyStart := filesStart + filesVPHeight + 2     // +1 label +1 buffer
	messageStart := historyStart + historyVPHeight + 2 // +1 label +1 buffer

	var cmd tea.Cmd
	if msg.Y >= messageStart {
		m.messageVP, cmd = m.messageVP.Update(msg)
	} else if msg.Y >= historyStart {
		m.historyVP, cmd = m.historyVP.Update(msg)
	} else if msg.Y >= filesStart {
		m.filesVP, cmd = m.filesVP.Update(msg)
	}
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Create folder mode
	if m.mode == modeCreateFolder {
		switch key {
		case "enter":
			folder := m.textInput.Value()
			if folder == "" && len(m.suggestions) > 0 && m.selectedSugg < len(m.suggestions) {
				folder = m.suggestions[m.selectedSugg]
			}
			m.mode = modeNormal
			m.textInput.Reset()
			m.textInput.Placeholder = "Type reply..."
			m.suggestions = nil
			m.selectedSugg = 0
			if folder != "" {
				return m, createSession(folder, m.agents, m.selfTarget)
			}
			return m, nil
		case "esc":
			m.mode = modeNormal
			m.textInput.Reset()
			m.textInput.Placeholder = "Type reply..."
			m.suggestions = nil
			m.selectedSugg = 0
			m.updateRightContent()
			return m, nil
		case "tab":
			if len(m.suggestions) > 0 && m.selectedSugg < len(m.suggestions) {
				m.textInput.SetValue(m.suggestions[m.selectedSugg])
				m.textInput.CursorEnd()
				m.suggestions = nil
				m.selectedSugg = 0
			}
			m.updateRightContent()
			return m, nil
		case "down":
			if len(m.suggestions) > 0 {
				m.selectedSugg = (m.selectedSugg + 1) % len(m.suggestions)
				m.updateRightContent()
			}
			return m, nil
		case "up":
			if len(m.suggestions) > 0 {
				m.selectedSugg = (m.selectedSugg - 1 + len(m.suggestions)) % len(m.suggestions)
				m.updateRightContent()
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			m.suggestions = filterZSuggestions(m.textInput.Value(), m.zEntries)
			m.selectedSugg = 0
			m.updateRightContent()
			return m, cmd
		}
	}

	// Reply mode
	if m.mode == modeReply {
		switch key {
		case "enter":
			text := m.textInput.Value()
			m.mode = modeNormal
			m.textInput.Reset()
			m.updateRightContent()
			if text != "" {
				if agent := m.selectedAgent(); agent != nil {
					return m, sendReply(agent.Target, text)
				}
			}
			return m, nil
		case "esc":
			m.mode = modeNormal
			m.textInput.Reset()
			m.updateRightContent()
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			m.updateRightContent()
			return m, cmd
		}
	}

	// Confirm close mode
	if m.mode == modeConfirmClose {
		switch key {
		case "y":
			target := m.confirmTarget
			m.confirmTarget = ""
			m.mode = modeNormal
			return m, closePane(target, m.statePath)
		case "n", "esc":
			m.confirmTarget = ""
			m.mode = modeNormal
			m.statusMsg = ""
			return m, nil
		}
		return m, nil
	}

	// Normal mode
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
			m.statusMsg = ""
			m.mode = modeNormal
			m.conversation = nil
			m.subActivity = nil
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "down", "j":
		if m.selected < len(m.treeNodes)-1 {
			m.selected++
			m.statusMsg = ""
			m.mode = modeNormal
			m.conversation = nil
			m.subActivity = nil
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "c":
		// Toggle collapse on current agent's subagent tree
		if agent := m.selectedAgent(); agent != nil {
			m.collapsed[agent.Target] = !m.collapsed[agent.Target]
			m.buildTree()
			if m.selected >= len(m.treeNodes) {
				m.selected = max(0, len(m.treeNodes)-1)
			}
			m.updateLeftContent()
			return m, nil
		}
	case "x":
		if sub := m.selectedSubagent(); sub != nil {
			// Dismiss selected subagent from tree
			agent := m.selectedAgent()
			if agent != nil {
				dismissKey := agent.Target + ":" + sub.AgentID
				m.dismissed[dismissKey] = true
				m.buildTree()
				if m.selected >= len(m.treeNodes) {
					m.selected = max(0, len(m.treeNodes)-1)
				}
				m.updateLeftContent()
				m.updateRightContent()
				return m, m.loadSelectionData()
			}
		} else if agent := m.selectedAgent(); agent != nil && m.tmuxAvailable {
			// Parent agent: confirm close
			m.mode = modeConfirmClose
			m.confirmTarget = agent.Target
			m.statusMsg = fmt.Sprintf("Close pane %s? (y/n)", agent.Target)
			m.statusMsgTick = -1 // pinned: don't auto-clear
			return m, nil
		}
	case "shift+down":
		// Jump to next parent agent (skip subagents)
		next := m.nextParentIndex(1)
		if next != m.selected {
			m.selected = next
			m.statusMsg = ""
			m.mode = modeNormal
			m.conversation = nil
			m.subActivity = nil
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "shift+up":
		// Jump to previous parent agent (skip subagents)
		prev := m.nextParentIndex(-1)
		if prev != m.selected {
			m.selected = prev
			m.statusMsg = ""
			m.mode = modeNormal
			m.conversation = nil
			m.subActivity = nil
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "tab":
		m.focusedVP = (m.focusedVP + 1) % focusCount
		return m, nil
	case "shift+tab":
		m.focusedVP = (m.focusedVP - 1 + focusCount) % focusCount
		return m, nil
	case "ctrl+u":
		return m.scrollFocused(msg)
	case "ctrl+d":
		return m.scrollFocused(msg)
	case "enter":
		if !m.tmuxAvailable {
			m.statusMsg = "Cannot jump: tmux not detected"
			return m, nil
		}
		if agent := m.selectedAgent(); agent != nil {
			return m, jumpToAgent(agent.Target)
		}
	case "r":
		if !m.tmuxAvailable {
			m.statusMsg = "Cannot reply: tmux not detected"
			return m, nil
		}
		if m.selectedAgent() != nil && m.selectedSubagent() == nil {
			m.mode = modeReply
			m.textInput.Focus()
			m.updateRightContent()
			return m, textinput.Blink
		}
	case "u":
		if m.mode == modeUsage {
			m.mode = modeNormal
			m.updateRightContent()
		} else {
			m.mode = modeUsage
			m.updateRightContent()
		}
		return m, nil
	case "a":
		if !m.tmuxAvailable {
			m.statusMsg = "Cannot create session: tmux not detected"
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		m.mode = modeCreateFolder
		m.textInput.Placeholder = "Git folder path (e.g. ~/code/myrepo)..."
		m.textInput.Focus()
		if m.zEntries == nil {
			m.zEntries = loadZEntries()
		}
		m.suggestions = filterZSuggestions("", m.zEntries)
		m.selectedSugg = 0
		m.updateRightContent()
		return m, textinput.Blink
	case "i":
		if !m.tmuxAvailable {
			m.statusMsg = "Cannot focus: tmux not detected"
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil {
			return m, selectPane(agent.Target)
		}
		m.statusMsg = "Select an agent to focus"
		m.statusMsgTick = m.tickCount
		return m, nil
	case "y", "n":
		if agent := m.selectedAgent(); m.tmuxAvailable && agent != nil && m.selectedSubagent() == nil {
			es := m.effectiveState(*agent)
			if es == "input" || es == "error" {
				return m, sendRawKey(agent.Target, key)
			}
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if agent := m.selectedAgent(); m.tmuxAvailable && agent != nil && m.selectedSubagent() == nil {
			es := m.effectiveState(*agent)
			if es == "input" || es == "error" {
				return m, sendRawKey(agent.Target, key)
			}
		}
	}

	return m, nil
}

func (m model) scrollFocused(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusedVP {
	case focusAgentList:
		m.agentListVP, cmd = m.agentListVP.Update(msg)
	case focusFiles:
		m.filesVP, cmd = m.filesVP.Update(msg)
	case focusHistory:
		m.historyVP, cmd = m.historyVP.Update(msg)
	case focusMessage:
		m.messageVP, cmd = m.messageVP.Update(msg)
	}
	return m, cmd
}
