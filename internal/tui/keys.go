package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjornjee/agent-dashboard/internal/zsuggest"
)

// debugLogKey writes a line to the debug key log file (if open).
// Format: timestamp | mode | key_string | key_type | runes_hex | mouse_age
// Note: phantom keys are swallowed by PhantomFilter before reaching handleKey,
// so any key logged here has already passed the filter.
func (m model) debugLogKey(msg tea.KeyPressMsg) {
	if m.DebugKeyLog == nil {
		return
	}
	var mouseAge string
	if m.lastEscapeAt.IsZero() {
		mouseAge = "never"
	} else {
		mouseAge = fmt.Sprintf("%dms", time.Since(m.lastEscapeAt).Milliseconds())
	}
	fmt.Fprintf(m.DebugKeyLog, "%s | mode=%d | key=%q | code=%d | text=%q | mouse_age=%s\n",
		time.Now().Format("15:04:05.000"), m.mode, msg.String(), msg.Key().Code, msg.Key().Text, mouseAge)
}

// confirmCooldown is the minimum time between entering a confirmation mode
// and accepting a confirmation key. Phantom keystrokes from terminal escape
// sequences arrive within microseconds; real users take at least 200-300ms.
const confirmCooldown = 300 * time.Millisecond

// escapeKeyCooldown is the minimum gap between a terminal escape sequence
// event (mouse, focus) and a key event for the key to be treated as genuine.
// Fragmented escape sequences produce phantom key events within ~1ms.
// 50ms is conservative — real transitions take 150ms+.
const escapeKeyCooldown = 50 * time.Millisecond

// modeResetCooldown is the minimum gap between a key event processed in a
// non-normal mode and a mode-entry key in normal mode. When Enter (or another
// destructive key) causes a mode transition back to normal, phantom repeats
// from key-release events or terminal artefacts can arrive within a few ms.
// 100ms is safe — real intentional keypresses take 150ms+.
const modeResetCooldown = 100 * time.Millisecond

// phantomGuardedKeys is the set of keys in normal mode that should be
// rejected when they arrive within escape or mode-reset cooldown windows.
// These are "destructive" keys that trigger actions; navigation keys are
// intentionally excluded so scrolling works immediately after mouse events.
var phantomGuardedKeys = map[string]bool{
	"x": true, "enter": true, "r": true, "m": true,
	"y": true, "n": true,
	"1": true, "2": true, "3": true, "4": true, "5": true,
	"6": true, "7": true, "8": true, "9": true,
}

// PhantomFilter is a tea.WithFilter callback that swallows phantom keystrokes
// before they reach Update(). It centralizes the escape-sequence cooldown,
// mode-reset cooldown, and confirmation cooldown guards that were previously
// scattered across individual key handlers.
func PhantomFilter(m tea.Model, msg tea.Msg) tea.Msg {
	mdl, ok := m.(model)
	if !ok {
		return msg
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return msg
	}
	keyStr := key.String()

	// Normal mode: swallow destructive keys during escape/mode-reset cooldown.
	if mdl.mode == modeNormal && phantomGuardedKeys[keyStr] {
		if !mdl.lastEscapeAt.IsZero() && time.Since(mdl.lastEscapeAt) < escapeKeyCooldown {
			return nil
		}
		if !mdl.modeResetAt.IsZero() && time.Since(mdl.modeResetAt) < modeResetCooldown {
			return nil
		}
	}

	// Confirmation modes: swallow confirming keys during cooldown.
	if !mdl.confirmEnteredAt.IsZero() && time.Since(mdl.confirmEnteredAt) < confirmCooldown {
		switch mdl.mode {
		case modeConfirmClose, modeConfirmMerge, modeConfirmCleanup:
			if keyStr == "y" {
				return nil
			}
		case modeConfirmSend:
			if keyStr == "enter" {
				return nil
			}
		case modeConfirmJump:
			if keyStr == "y" || keyStr == "enter" {
				return nil
			}
		}
	}

	return msg
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.lastEscapeAt = time.Now()
	mouse := msg.Mouse()
	m.mouseY = mouse.Y
	if m.DebugKeyLog != nil {
		fmt.Fprintf(m.DebugKeyLog, "%s | MOUSE | button=%d x=%d y=%d type=%T\n",
			time.Now().Format("15:04:05.000"), mouse.Button, mouse.X, mouse.Y, msg)
	}

	// Help overlay: swallow mouse events
	if m.helpVisible {
		return m, nil
	}

	// Diff mode: route mouse to diff viewports
	if m.diffVisible {
		leftBorderEnd := m.diffLeftWidth + 2
		var cmd tea.Cmd
		if mouse.X < leftBorderEnd {
			// Left-click selects a file/dir entry in the file list panel.
			if click, ok := msg.(tea.MouseClickMsg); ok && click.Mouse().Button == tea.MouseLeft {
				// Calculate which visible entry was clicked.
				// Panel layout: 1 border + 1 header + (1 filter if active) + 1 blank = offset rows
				headerRows := 3 // border + "FILES CHANGED" + blank line
				if m.diffFilterActive || m.diffFilterText != "" {
					headerRows++ // filter input line
				}
				clickedLine := mouse.Y - m.bannerHeight() - headerRows + m.diffFileVP.YOffset()
				if clickedLine >= 0 {
					vis := m.visibleDiffEntries()
					// Map clickedLine to a visible entry index, accounting for
					// multi-line wrapped entries.
					lineCount := 0
					for visIdx, entryIdx := range vis {
						entry := m.diffTreeEntries[entryIdx]
						entryLines := 1
						if !entry.isDir {
							// Count wrapped lines for long file names.
							maxWidth := m.diffLeftWidth - 4
							if maxWidth < 10 {
								maxWidth = 10
							}
							// Must match render path: prefix = indentStr + " icon "
							// where icon is 1 visible char = indent*3 + 3
							prefixWidth := entry.indent*3 + 3
							nameWidth := maxWidth - prefixWidth
							if nameWidth > 0 && len([]rune(entry.label)) > nameWidth {
								entryLines = (len([]rune(entry.label)) + nameWidth - 1) / nameWidth
							}
						}
						if clickedLine < lineCount+entryLines {
							m.diffCursor = visIdx
							if entry.isDir {
								m.toggleDiffDir()
							} else {
								m.selectedDiffFile = entry.fileIdx
								m.diffExpandedAll = false
								m.updateDiffContent()
							}
							return m, nil
						}
						lineCount += entryLines
					}
				}
				return m, nil
			}
			// Scroll wheel events still handled by viewport.
			m.diffFileVP, cmd = m.diffFileVP.Update(msg)
		} else {
			m.diffContentVP, cmd = m.diffContentVP.Update(msg)
		}
		return m, cmd
	}

	leftBorderEnd := m.leftWidth + 2

	if mouse.X < leftBorderEnd {
		var cmd tea.Cmd
		m.agentListVP, cmd = m.agentListVP.Update(msg)
		return m, cmd
	}

	// When plan is visible, the right panel is just header + messageVP (full height).
	// Route all right-panel mouse events directly to messageVP.
	if m.planVisible && m.renderedPlan != "" {
		var cmd tea.Cmd
		m.messageVP, cmd = m.messageVP.Update(msg)
		return m, cmd
	}

	// Route to inner right viewport based on Y position
	historyStart := m.historyViewportStartY()
	messageStart := m.messageViewportStartY()

	var cmd tea.Cmd
	if mouse.Y >= messageStart {
		m.messageVP, cmd = m.messageVP.Update(msg)
	} else if mouse.Y >= historyStart {
		m.historyVP, cmd = m.historyVP.Update(msg)
	} else if mouse.Y >= m.filesViewportStartY() {
		m.filesVP, cmd = m.filesVP.Update(msg)
	}
	return m, cmd
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.debugLogKey(msg)
	key := msg.String()

	// Track when keys are processed in non-normal modes so that phantom
	// repeats arriving right after a mode transition can be rejected.
	if m.mode != modeNormal && !m.helpVisible && !m.diffVisible {
		m.modeResetAt = time.Now()
	}

	// Dino game mode: route all keys to game
	if m.mode == modeDinoGame {
		var cmd tea.Cmd
		m.dino, cmd = m.dino.Update(msg)
		return m, cmd
	}

	// Create folder mode
	if m.mode == modeCreateFolder {
		switch key {
		case "enter":
			folder := m.textInput.Value()
			// When suggestions are visible, always use the highlighted entry.
			// The view highlights suggestions[selectedSugg] even without
			// arrow-key navigation, so Enter should honour that selection.
			if len(m.suggestions) > 0 && m.selectedSugg < len(m.suggestions) {
				folder = m.suggestions[m.selectedSugg]
			}
			m.suggestions = nil
			m.selectedSugg = 0

			if folder == "" {
				m.mode = modeNormal
				m.textInput.Reset()
				m.textInput.Placeholder = "Type reply..."
				return m, nil
			}

			m.createFolder = folder

			if m.skillsAvailable {
				// Advance to skill selection
				m.mode = modeCreateSkill
				m.selectedCreateSkill = 0
				m.textInput.Reset()
				m.updateRightContent()
				return m, nil
			}

			// Skip skill, advance to message input
			m.createSkillName = ""
			m.mode = modeCreateMessage
			m.textInput.Reset()
			m.textInput.Placeholder = "Message for agent (optional, Enter to skip)..."
			focusCmd := m.textInput.Focus()
			m.updateRightContent()
			return m, focusCmd
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
			m.suggestions = zsuggest.FilterZSuggestions(m.textInput.Value(), m.zEntries, m.pathExists)
			m.selectedSugg = 0
			m.updateRightContent()
			return m, cmd
		}
	}

	// Create skill selection mode
	if m.mode == modeCreateSkill {
		switch key {
		case "enter":
			if m.selectedCreateSkill == 0 || m.selectedCreateSkill >= len(m.availableSkills) {
				m.createSkillName = "" // "(none)" or out of bounds
			} else {
				m.createSkillName = m.availableSkills[m.selectedCreateSkill]
			}
			m.mode = modeCreateMessage
			m.textInput.Placeholder = "Message for agent (optional, Enter to skip)..."
			focusCmd := m.textInput.Focus()
			m.updateRightContent()
			return m, focusCmd
		case "esc":
			// Back to folder selection
			m.mode = modeCreateFolder
			m.selectedCreateSkill = 0
			m.textInput.SetValue(m.createFolder)
			m.textInput.CursorEnd()
			focusCmd := m.textInput.Focus()
			m.suggestions = zsuggest.FilterZSuggestions(m.createFolder, m.zEntries, m.pathExists)
			m.selectedSugg = 0
			m.updateRightContent()
			return m, focusCmd
		case "ctrl+c":
			m.mode = modeNormal
			m.textInput.Reset()
			m.textInput.Placeholder = "Type reply..."
			m.createFolder = ""
			m.selectedCreateSkill = 0
			m.updateRightContent()
			return m, nil
		case "down":
			if m.selectedCreateSkill < len(m.availableSkills)-1 {
				m.selectedCreateSkill++
				m.updateRightContent()
			}
			return m, nil
		case "up":
			if m.selectedCreateSkill > 0 {
				m.selectedCreateSkill--
				m.updateRightContent()
			}
			return m, nil
		}
		return m, nil
	}

	// Create message input mode
	if m.mode == modeCreateMessage {
		switch key {
		case "enter":
			message := m.textInput.Value()
			folder := m.createFolder
			skill := m.createSkillName

			// Reset wizard state
			m.mode = modeNormal
			m.textInput.Reset()
			m.textInput.Placeholder = "Type reply..."
			m.createFolder = ""
			m.createSkillName = ""
			m.selectedCreateSkill = 0

			m.statusMsg = "spawning"
			m.statusMsgTick = -1 // don't auto-clear
			m.spawningFolder = folder
			m.spawningTick = m.tickCount
			return m, tea.Batch(
				createSessionWithPrompt(folder, m.agents, m.selfPaneID, m.cfg.Profile, skill, message),
				m.spawningSpinner.Tick,
			)
		case "esc":
			// Back to skill selection (if available) or folder selection
			m.textInput.Reset()
			if m.skillsAvailable {
				m.mode = modeCreateSkill
				m.createSkillName = ""
				m.updateRightContent()
				return m, nil
			}
			// No skills — back to folder selection
			m.mode = modeCreateFolder
			m.textInput.SetValue(m.createFolder)
			m.textInput.CursorEnd()
			focusCmd := m.textInput.Focus()
			m.suggestions = zsuggest.FilterZSuggestions(m.createFolder, m.zEntries, m.pathExists)
			m.selectedSugg = 0
			m.updateRightContent()
			return m, focusCmd
		case "ctrl+c":
			m.mode = modeNormal
			m.textInput.Reset()
			m.textInput.Placeholder = "Type reply..."
			m.createFolder = ""
			m.createSkillName = ""
			m.selectedCreateSkill = 0
			m.updateRightContent()
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
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
					return m, sendReply(agent.TmuxPaneID, text, m.selfPaneID)
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
			m.messageVP.GotoBottom()
			return m, cmd
		}
	}

	// Confirm close mode
	if m.mode == modeConfirmClose {
		switch key {
		case "y":
			paneID := m.confirmPaneID
			sessionID := m.confirmSessionID
			m.confirmPaneID = ""
			m.confirmSessionID = ""
			m.mode = modeNormal
			return m, closePane(paneID, sessionID, m.statePath)
		case "n", "esc":
			m.confirmPaneID = ""
			m.confirmSessionID = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, nil
		}
		return m, nil
	}

	// Confirm merge mode
	if m.mode == modeConfirmMerge {
		switch key {
		case "y":
			sessionID := m.confirmMergeSessionID
			paneID := m.confirmMergePaneID
			dir := m.confirmMergeDir
			branch := m.confirmMergeBranch
			cwd := m.confirmMergeCwd
			m.confirmMergeSessionID = ""
			m.confirmMergePaneID = ""
			m.confirmMergeDir = ""
			m.confirmMergeBranch = ""
			m.confirmMergeCwd = ""
			m.mode = modeNormal
			if m.ghAvailable {
				m.mergeSessionID = sessionID
				m.mergePaneID = paneID
				m.mergeCwd = cwd
				m.mergeBranch = branch
				if cwd != dir {
					m.mergeWorktreeCwd = dir
				}
				m.setStatus("Merging PR...", false)
				return m, mergePR(dir, branch)
			}
			m.setStatus("Marked as merged", false)
			return m, pinAgentStateCmd(m.statePath, sessionID, "merged")
		case "n", "esc":
			m.confirmMergeSessionID = ""
			m.confirmMergePaneID = ""
			m.confirmMergeDir = ""
			m.confirmMergeBranch = ""
			m.confirmMergeCwd = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, nil
		}
		return m, nil
	}

	// Confirm cleanup mode (post-merge)
	if m.mode == modeConfirmCleanup {
		switch key {
		case "y":
			sessionID := m.cleanupSessionID
			paneID := m.cleanupPaneID
			cwd := m.cleanupCwd
			worktreeCwd := m.cleanupWorktreeCwd
			branch := m.cleanupBranch
			m.cleanupSessionID = ""
			m.cleanupPaneID = ""
			m.cleanupCwd = ""
			m.cleanupWorktreeCwd = ""
			m.cleanupBranch = ""
			m.mode = modeNormal
			m.setStatus("Cleaning up...", false)
			return m, postMergeCleanup(paneID, sessionID, m.statePath, cwd, worktreeCwd, branch)
		case "n", "esc":
			m.cleanupSessionID = ""
			m.cleanupPaneID = ""
			m.cleanupCwd = ""
			m.cleanupWorktreeCwd = ""
			m.cleanupBranch = ""
			m.mode = modeNormal
			m.setStatus("PR merged", false)
			return m, nil
		}
		return m, nil
	}

	// Confirm send-key mode
	if m.mode == modeConfirmSend {
		switch key {
		case "enter":
			paneID := m.confirmSendPaneID
			sendKey := m.confirmSendKey
			label := m.confirmSendLabel
			m.confirmSendPaneID = ""
			m.confirmSendKey = ""
			m.confirmSendLabel = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, sendRawKey(paneID, sendKey, label)
		case "esc":
			m.confirmSendPaneID = ""
			m.confirmSendKey = ""
			m.confirmSendLabel = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, nil
		}
		return m, nil
	}

	// Confirm jump mode (guards enter key against phantom keystrokes)
	if m.mode == modeConfirmJump {
		switch key {
		case "y", "enter":
			paneID := m.confirmJumpPaneID
			m.confirmJumpPaneID = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, jumpToAgent(paneID)
		case "n", "esc":
			m.confirmJumpPaneID = ""
			m.mode = modeNormal
			m.clearStatus()
			return m, nil
		}
		return m, nil
	}

	// Help overlay
	if m.helpVisible {
		switch key {
		case "h", "esc":
			m.helpVisible = false
		default:
			// swallow all other keys
		}
		return m, nil
	}

	// Diff viewer mode
	if m.diffVisible {
		// Filter input active — forward keys to text input
		if m.diffFilterActive {
			switch key {
			case "esc":
				m.diffFilterActive = false
				m.diffFilterText = ""
				m.diffFilterInput.Reset()
				m.diffFilterInput.Blur()
				m.applyTreeVisibility()
				m.diffCursor = 0
				m.updateDiffContent()
				return m, nil
			case "enter":
				m.diffFilterActive = false
				m.diffFilterInput.Blur()
				m.diffFilterText = m.diffFilterInput.Value()
				m.applyTreeVisibility()
				m.diffCursor = 0
				// Select the first visible file
				vis := m.visibleDiffEntries()
				for _, idx := range vis {
					e := m.diffTreeEntries[idx]
					if !e.isDir {
						m.selectedDiffFile = e.fileIdx
						break
					}
				}
				m.updateDiffContent()
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.diffFilterInput, cmd = m.diffFilterInput.Update(msg)
				m.diffFilterText = m.diffFilterInput.Value()
				m.applyTreeVisibility()
				m.diffCursor = 0
				m.updateDiffContent()
				return m, cmd
			}
		}

		switch key {
		case "d", "q", "esc":
			m.diffVisible = false
			m.diffExpandedAll = false
			m.diffTreeEntries = nil
			m.diffFilterText = ""
			m.diffFilterActive = false
			m.diffFilterInput.Reset()
			return m, nil
		case "up", "k":
			m.moveDiffCursor(-1)
			return m, nil
		case "down", "j":
			m.moveDiffCursor(1)
			return m, nil
		case "g":
			// Jump to first visible entry.
			vis := m.visibleDiffEntries()
			if len(vis) > 0 {
				m.diffCursor = 0
				entry := m.diffTreeEntries[vis[0]]
				if !entry.isDir {
					m.selectedDiffFile = entry.fileIdx
					m.diffExpandedAll = false
				}
				m.updateDiffContent()
			}
			return m, nil
		case "G":
			// Jump to last visible entry.
			vis := m.visibleDiffEntries()
			if len(vis) > 0 {
				m.diffCursor = len(vis) - 1
				entry := m.diffTreeEntries[vis[m.diffCursor]]
				if !entry.isDir {
					m.selectedDiffFile = entry.fileIdx
					m.diffExpandedAll = false
				}
				m.updateDiffContent()
			}
			return m, nil
		case "{":
			// Move cursor up by half viewport height.
			half := m.diffFileVP.Height() / 2
			if half < 1 {
				half = 1
			}
			m.moveDiffCursor(-half)
			return m, nil
		case "}":
			// Move cursor down by half viewport height.
			half := m.diffFileVP.Height() / 2
			if half < 1 {
				half = 1
			}
			m.moveDiffCursor(half)
			return m, nil
		case "enter", "space":
			m.toggleDiffDir()
			return m, nil
		case "/":
			m.diffFilterActive = true
			focusCmd := m.diffFilterInput.Focus()
			return m, focusCmd
		case "e":
			m.diffExpandedAll = !m.diffExpandedAll
			m.updateDiffContent()
			return m, nil
		case "ctrl+d":
			m.diffContentVP.HalfPageDown()
			return m, nil
		case "ctrl+u":
			m.diffContentVP.HalfPageUp()
			return m, nil
		case "J":
			m.diffContentVP.ScrollDown(1)
			return m, nil
		case "K":
			m.diffContentVP.ScrollUp(1)
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Normal mode
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.saveCurrentCache()
			m.selected--
			// Skip nodes in collapsed groups
			for m.selected > 0 && m.isNodeInCollapsedGroup(m.selected) {
				m.selected--
			}
			// If we landed on a collapsed node at index 0, move forward instead
			if m.isNodeInCollapsedGroup(m.selected) {
				m.selected++
				for m.selected < len(m.treeNodes) && m.isNodeInCollapsedGroup(m.selected) {
					m.selected++
				}
			}
			m.clearStatus()
			m.mode = modeNormal
			m.restoreCurrentCache()
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "down", "j":
		if m.selected < len(m.treeNodes)-1 {
			m.saveCurrentCache()
			m.selected++
			// Skip nodes in collapsed groups
			for m.selected < len(m.treeNodes)-1 && m.isNodeInCollapsedGroup(m.selected) {
				m.selected++
			}
			// If we landed on a collapsed node at the end, move backward instead
			if m.isNodeInCollapsedGroup(m.selected) {
				m.selected--
				for m.selected > 0 && m.isNodeInCollapsedGroup(m.selected) {
					m.selected--
				}
			}
			m.clearStatus()
			m.mode = modeNormal
			m.restoreCurrentCache()
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
	case "C":
		// Toggle collapse on the status group — works on both group headers and agents
		group := m.selectedGroupHeader()
		if group == 0 {
			if agent := m.selectedAgent(); agent != nil {
				group = agentGroup(*agent)
			}
		}
		if group > 0 {
			m.collapsedGroups[group] = !m.collapsedGroups[group]
			// If we just collapsed, move selection to the group's header node
			if m.collapsedGroups[group] && m.isNodeInCollapsedGroup(m.selected) {
				m.saveCurrentCache()
				// Find the header node for this group (scan backward)
				for i := m.selected - 1; i >= 0; i-- {
					if m.treeNodes[i].GroupHeader == group {
						m.selected = i
						break
					}
				}
				m.restoreCurrentCache()
			}
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
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
			m.confirmEnteredAt = time.Now()
			m.confirmPaneID = agent.TmuxPaneID
			m.confirmSessionID = agent.SessionID
			m.statusMsg = fmt.Sprintf("Close pane %s? (y/n)", agent.Target)
			m.statusMsgTick = -1 // pinned: don't auto-clear
			return m, nil
		}
	case "shift+down":
		// Jump to next parent agent (skip subagents)
		next := m.nextParentIndex(1)
		if next != m.selected {
			m.saveCurrentCache()
			m.selected = next
			m.clearStatus()
			m.mode = modeNormal
			m.restoreCurrentCache()
			m.updateLeftContent()
			m.updateRightContent()
			return m, m.loadSelectionData()
		}
	case "shift+up":
		// Jump to previous parent agent (skip subagents)
		prev := m.nextParentIndex(-1)
		if prev != m.selected {
			m.saveCurrentCache()
			m.selected = prev
			m.clearStatus()
			m.mode = modeNormal
			m.restoreCurrentCache()
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
			m.setStatus("Cannot jump: tmux not detected", true)
			return m, nil
		}
		if agent := m.selectedAgent(); agent != nil {
			m.mode = modeConfirmJump
			m.confirmEnteredAt = time.Now()
			m.confirmJumpPaneID = agent.TmuxPaneID
			m.statusMsg = "Jump to agent? (y/Enter to confirm, Esc to cancel)"
			m.statusMsgTick = -1 // pinned
			return m, nil
		}
	case "r":
		if !m.tmuxAvailable {
			m.setStatus("Cannot reply: tmux not detected", true)
			return m, nil
		}
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil {
			var cmds []tea.Cmd
			// Plan state: send "3" (feedback option) before entering reply mode
			if agent.State == "plan" {
				cmds = append(cmds, sendRawKey(agent.TmuxPaneID, "3", "Plan feedback selected"))
			}
			m.mode = modeReply
			focusCmd := m.textInput.Focus()
			m.updateRightContent()
			m.messageVP.GotoBottom()
			cmds = append(cmds, focusCmd)
			return m, tea.Batch(cmds...)
		}
	case "p":
		if m.selectedAgent() != nil && m.selectedSubagent() == nil && m.planContent != "" {
			m.planVisible = !m.planVisible
			if m.planVisible {
				m.focusedVP = focusMessage
			}
			m.updateRightContent()
			if m.planVisible {
				m.messageVP.GotoTop()
			}
		}
		return m, nil
	case "J":
		if m.planVisible && m.renderedPlan != "" {
			m.messageVP.ScrollDown(1)
		}
		return m, nil
	case "K":
		if m.planVisible && m.renderedPlan != "" {
			m.messageVP.ScrollUp(1)
		}
		return m, nil
	case "o":
		if !m.tmuxAvailable {
			m.setStatus("Cannot open window: tmux not detected", true)
			return m, nil
		}
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil && agent.EffectiveDir() != "" {
			m.statusMsg = "Opening window..."
			m.statusIsError = false
			m.statusMsgTick = -1 // pinned until async result
			return m, openWorktreeWindowCmd(agent.Session, agent.Branch, agent.EffectiveDir())
		}
	case "e":
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil && agent.EffectiveDir() != "" {
			m.statusMsg = "Opening editor..."
			m.statusIsError = false
			m.statusMsgTick = -1 // pinned until async result
			return m, openEditor(m.cfg.Editor, agent.EffectiveDir())
		}
	case "d":
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil && agent.EffectiveDir() != "" {
			m.statusMsg = "Loading diff..."
			m.statusIsError = false
			m.statusMsgTick = -1 // pinned until async result
			return m, loadDiffCmd(agent.EffectiveDir())
		}
	case "g":
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil && agent.EffectiveDir() != "" && agent.Branch != "" {
			m.statusMsg = "Opening PR..."
			m.statusIsError = false
			m.statusMsgTick = -1 // pinned until async result
			cmds := []tea.Cmd{openPR(agent.EffectiveDir(), agent.Branch)}
			if m.ghAvailable {
				// Defer pinning to openPRMsg handler — only pin when PR actually exists
				m.openPRSessionID = agent.SessionID
			} else {
				// No gh: pin immediately (manual workflow, backward compat)
				cmds = append(cmds, pinAgentStateCmd(m.statePath, agent.SessionID, "pr"))
			}
			return m, tea.Batch(cmds...)
		}
	case "m":
		if agent := m.selectedAgent(); agent != nil && m.selectedSubagent() == nil && m.tmuxAvailable &&
			isPR(agent.State) && agent.EffectiveDir() != "" && agent.Branch != "" {
			m.mode = modeConfirmMerge
			m.confirmEnteredAt = time.Now()
			m.confirmMergeSessionID = agent.SessionID
			m.confirmMergePaneID = agent.TmuxPaneID
			m.confirmMergeDir = agent.EffectiveDir()
			m.confirmMergeBranch = agent.Branch
			m.confirmMergeCwd = agent.Cwd
			m.statusMsg = fmt.Sprintf("Merge %s? (y/n)", agent.Branch)
			m.statusMsgTick = -1 // pinned
			return m, nil
		}
	case "h":
		m.helpVisible = true
		return m, nil
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
			m.setStatus("Cannot create session: tmux not detected", true)
			return m, nil
		}
		m.mode = modeCreateFolder
		m.textInput.Placeholder = "Git folder path (e.g. ~/code/myrepo)..."
		focusCmd := m.textInput.Focus()
		if m.zEntries == nil {
			m.zEntries = zsuggest.LoadZEntries(m.cfg.Profile.SessionsDir)
		}
		m.suggestions = zsuggest.FilterZSuggestions("", m.zEntries, m.pathExists)
		m.selectedSugg = 0
		m.updateRightContent()
		return m, focusCmd
	case "G":
		if m.dinoEnabled {
			m.mode = modeDinoGame
			m.dino = newDinoGameModel(m.leftWidth, dinoGameHeight)
			return m, nil
		}
	case "y", "n":
		if agent := m.selectedAgent(); m.tmuxAvailable && agent != nil && m.selectedSubagent() == nil {
			es := agent.State
			if isBlocked(es) || isWaiting(es) {
				sendKey := key
				label := fmt.Sprintf("Sent '%s'", key)
				// Plan state: y→"1" (approve+bypass), n stays as "n"
				if es == "plan" && key == "y" {
					sendKey = "1"
					label = "Plan approved"
				}
				m.mode = modeConfirmSend
				m.confirmEnteredAt = time.Now()
				m.confirmSendPaneID = agent.TmuxPaneID
				m.confirmSendKey = sendKey
				m.confirmSendLabel = label
				m.statusMsg = fmt.Sprintf("Send '%s' to agent? (Enter to confirm, Esc to cancel)", key)
				m.statusMsgTick = -1 // pinned
				return m, nil
			}
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if agent := m.selectedAgent(); m.tmuxAvailable && agent != nil && m.selectedSubagent() == nil {
			es := agent.State
			if isBlocked(es) || isWaiting(es) {
				m.mode = modeConfirmSend
				m.confirmEnteredAt = time.Now()
				m.confirmSendPaneID = agent.TmuxPaneID
				m.confirmSendKey = key
				m.confirmSendLabel = fmt.Sprintf("Sent '%s'", key)
				m.statusMsg = fmt.Sprintf("Send '%s' to agent? (Enter to confirm, Esc to cancel)", key)
				m.statusMsgTick = -1 // pinned
				return m, nil
			}
		}
	}

	return m, nil
}

func (m model) scrollFocused(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
