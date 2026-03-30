package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// -- Tree node --

// treeNode is a flat entry in the navigation tree (agent or subagent).
type treeNode struct {
	AgentIdx int           // index into m.agents
	Sub      *SubagentInfo // nil for parent agent nodes
}

// -- Model --

type model struct {
	agents        []Agent
	selected      int // index into treeNodes
	treeNodes     []treeNode
	width, height int
	mode          int
	textInput     textinput.Model
	tmuxAvailable bool
	statePath     string
	selfTarget    string
	statusMsg     string
	statusMsgTick int // tick when statusMsg was set; clears after 3s
	capturedLines []string
	conversation  []ConversationEntry
	tickCount     int
	agentUsage    map[string]Usage
	totalUsage    Usage
	db            *DB
	dbTotalCost   float64
	dbTodayCost   float64

	// Viewports
	agentListVP viewport.Model
	filesVP     viewport.Model
	historyVP   viewport.Model
	messageVP   viewport.Model
	focusedVP   int

	// Layout cache (for mouse routing)
	leftWidth  int
	rightWidth int

	// Subagent tree
	agentSubagents map[string][]SubagentInfo // parentTarget → subagents
	collapsed      map[string]bool           // parentTarget → collapsed state
	dismissed      map[string]bool           // "parentTarget:agentID" → dismissed
	subActivity    []ActivityEntry           // activity log for selected subagent

	// Plan content for selected agent
	planContent  string
	planVisible  bool   // true when plan is shown in message VP
	renderedPlan string // glamour-rendered plan markdown

	// Previous effective state per agent — used to detect transitions
	// and fire desktop notifications on needs-attention.
	prevEffState map[string]string // agentTarget → last effectiveState result

	// Close confirmation
	confirmTarget string // tmux target pending close confirmation

	// Z-plugin suggestions for create folder mode
	zEntries     []zEntry // cached z entries from ~/.z
	suggestions  []string // filtered suggestions for current input
	selectedSugg int      // index of highlighted suggestion

	// Banner
	quote   string           // random quote selected at startup
	nowFunc func() time.Time // injectable clock for testability

	// Path validation for z suggestions (injectable for testing)
	pathExists func(string) bool
}

// buildTree rebuilds the flat tree node list from agents and their subagents.
func (m *model) buildTree() {
	m.treeNodes = nil
	for i, agent := range m.agents {
		m.treeNodes = append(m.treeNodes, treeNode{AgentIdx: i})
		if !m.collapsed[agent.Target] {
			for _, sub := range m.agentSubagents[agent.Target] {
				key := agent.Target + ":" + sub.AgentID
				if m.dismissed[key] {
					continue
				}
				s := sub // copy
				m.treeNodes = append(m.treeNodes, treeNode{AgentIdx: i, Sub: &s})
			}
		}
	}
}

// nextParentIndex finds the next parent agent node in the given direction (1 or -1).
// Returns the index of the next parent, or stays at current if none found.
func (m model) nextParentIndex(dir int) int {
	for i := m.selected + dir; i >= 0 && i < len(m.treeNodes); i += dir {
		if m.treeNodes[i].Sub == nil {
			return i
		}
	}
	return m.selected
}

// selectedAgent returns the parent agent for the current selection.
func (m model) selectedAgent() *Agent {
	if m.selected >= len(m.treeNodes) {
		return nil
	}
	idx := m.treeNodes[m.selected].AgentIdx
	if idx >= len(m.agents) {
		return nil
	}
	return &m.agents[idx]
}

// selectedSubagent returns the subagent for the current selection, or nil if parent is selected.
func (m model) selectedSubagent() *SubagentInfo {
	if m.selected >= len(m.treeNodes) {
		return nil
	}
	return m.treeNodes[m.selected].Sub
}

func newModel(statePath, selfTarget string, db *DB) model {
	ti := textinput.New()
	ti.Placeholder = "Type reply..."
	ti.CharLimit = 4096

	return model{
		agents:         nil,
		statePath:      statePath,
		selfTarget:     selfTarget,
		tmuxAvailable:  TmuxIsAvailable(),
		textInput:      ti,
		mode:           modeNormal,
		db:             db,
		agentListVP:    viewport.New(0, 0),
		filesVP:        viewport.New(0, 0),
		historyVP:      viewport.New(0, 0),
		messageVP:      viewport.New(0, 0),
		focusedVP:      focusAgentList,
		agentSubagents: make(map[string][]SubagentInfo),
		collapsed:      make(map[string]bool),
		dismissed:      make(map[string]bool),
		prevEffState:   make(map[string]string),
		quote:          pickQuote(db),
		nowFunc:        time.Now,
		pathExists:     dirExists,
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		loadState(m.statePath),
		tickEvery(),
		m.captureSelected(),
		loadUsage(m.agents),
	}
	if m.db != nil {
		cmds = append(cmds, loadDBCost(m.db))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		return m, nil

	case stateUpdatedMsg:
		m.agents = SortedAgents(msg.state, m.selfTarget)
		// Prune maps for agents no longer present
		live := make(map[string]bool, len(m.agents))
		for _, a := range m.agents {
			live[a.Target] = true
		}
		for target := range m.prevEffState {
			if !live[target] {
				delete(m.prevEffState, target)
			}
		}
		for target := range m.agentSubagents {
			if !live[target] {
				delete(m.agentSubagents, target)
			}
		}
		for target := range m.collapsed {
			if !live[target] {
				delete(m.collapsed, target)
			}
		}
		for key := range m.dismissed {
			// dismissed keys are "session:window.pane:agentID" (constructed in keys.go).
			// parentTarget is "session:window.pane" — extract by finding the last colon,
			// which is safe as long as agentID contains no colons (UUIDs don't).
			parentTarget := key
			if idx := strings.LastIndex(key, ":"); idx > 0 {
				parentTarget = key[:idx]
			}
			if !live[parentTarget] {
				delete(m.dismissed, key)
			}
		}
		m.buildTree()
		if m.selected >= len(m.treeNodes) {
			m.selected = max(0, len(m.treeNodes)-1)
		}
		m.updateLeftContent()
		m.updateRightContent()
		cmds := []tea.Cmd{m.captureSelected(), m.loadConversation(), loadUsage(m.agents), m.loadPlan()}
		cmds = append(cmds, m.loadAllSubagents()...)
		if cmd := m.checkNeedsAttentionTransition(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case conversationMsg:
		prevLen := len(m.conversation)
		m.conversation = msg.entries
		m.updateRightContent()
		// On first load, scroll history to end
		if prevLen == 0 {
			m.historyVP.GotoBottom()
		}
		return m, nil

	case planMsg:
		m.planContent = msg.content
		if msg.content != "" {
			m.renderedPlan = renderPlanMarkdown(msg.content, m.rightWidth-4)
		} else {
			m.renderedPlan = ""
			m.planVisible = false
		}
		m.updateRightContent()
		return m, nil

	case tickMsg:
		m.tickCount++
		// Auto-clear status message after 3 seconds
		if m.statusMsg != "" && m.statusMsgTick >= 0 && m.tickCount-m.statusMsgTick >= 3 {
			m.statusMsg = ""
		}
		cmds := []tea.Cmd{tickEvery(), m.captureSelected(), m.loadConversation()}
		if m.selectedSubagent() != nil {
			cmds = append(cmds, m.loadSubagentActivity())
		}
		if m.tickCount%5 == 0 {
			cmds = append(cmds, m.loadAllSubagents()...)
			cmds = append(cmds, m.loadPlan())
		}
		if m.tickCount%10 == 0 {
			cmds = append(cmds, pruneDead(m.statePath), loadUsage(m.agents))
		}
		return m, tea.Batch(cmds...)

	case usageMsg:
		m.agentUsage = msg.perAgent
		m.totalUsage = msg.total
		m.updateRightContent()
		var cmds []tea.Cmd
		if m.db != nil {
			cmds = append(cmds, persistUsage(m.db, m.agents, msg.perAgent))
			cmds = append(cmds, loadDBCost(m.db))
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case persistResultMsg:
		return m, nil

	case dbCostMsg:
		m.dbTotalCost = msg.total
		m.dbTodayCost = msg.todayCost
		return m, nil

	case activityMsg:
		if m.selectedSubagent() != nil {
			m.subActivity = msg.entries
			m.updateRightContent()
		}
		return m, nil

	case subagentsMsg:
		m.agentSubagents[msg.parentTarget] = msg.agents
		m.buildTree()
		if m.selected >= len(m.treeNodes) {
			m.selected = max(0, len(m.treeNodes)-1)
		}
		m.updateLeftContent()
		return m, nil

	case createSessionMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Create failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			m.mode = modeNormal
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("Session created: %s", msg.target)
		m.statusMsgTick = m.tickCount
		m.updateRightContent()
		return m, tea.Batch(loadState(m.statePath), selectPane(msg.target))

	case closeResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Close failed: %v", msg.err)
		} else {
			m.statusMsg = "Pane closed"
		}
		m.statusMsgTick = m.tickCount
		// Renames already applied to state file inside closePane; pruneDead
		// only needs to catch agents that were already dead before this kill.
		return m, tea.Batch(loadState(m.statePath), pruneDead(m.statePath))

	case pruneDeadMsg:
		if msg.removed > 0 {
			return m, loadState(m.statePath)
		}
		return m, nil

	case captureResultMsg:
		m.capturedLines = msg.lines
		m.updateRightContent()
		return m, nil

	case jumpResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Jump failed: %v", msg.err)
		} else {
			m.statusMsg = "Jumped — switch back to this window for dashboard"
		}
		m.statusMsgTick = m.tickCount
		return m, nil

	case sendResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Reply failed: %v", msg.err)
		} else {
			m.statusMsg = "Reply sent"
		}
		m.statusMsgTick = m.tickCount
		return m, nil

	case selectPaneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Focus failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
		}
		return m, nil

	case notifyResultMsg:
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.mode == modeReply {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) resizeViewports() {
	m.leftWidth = m.width*30/100 - 2
	m.rightWidth = m.width - m.leftWidth - 4
	panelHeight := m.height - 5 - bannerHeight

	m.agentListVP.Width = m.leftWidth
	m.agentListVP.Height = panelHeight

	m.filesVP.Width = m.rightWidth
	m.filesVP.Height = filesVPHeight

	m.historyVP.Width = m.rightWidth
	m.historyVP.Height = historyVPHeight

	msgHeight := panelHeight - headerLines - filesVPHeight - historyVPHeight - sectionGaps
	if msgHeight < 3 {
		msgHeight = 3
	}
	m.messageVP.Width = m.rightWidth
	m.messageVP.Height = msgHeight

	if m.planContent != "" && m.planVisible {
		m.renderedPlan = renderPlanMarkdown(m.planContent, m.rightWidth-4)
	}
	m.updateLeftContent()
	m.updateRightContent()
}
