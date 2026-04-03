package main

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// -- Tree node --

// treeNode is a flat entry in the navigation tree (agent or subagent).
type treeNode struct {
	AgentIdx int           // index into m.agents
	Sub      *SubagentInfo // nil for parent agent nodes
}

// -- Per-agent state cache --

// agentCache stores per-agent UI state so switching between agents preserves
// conversation history, live output, plan content, etc.
type agentCache struct {
	conversation   []ConversationEntry
	convFileOffset int64
	convSessionKey string
	planContent    string
	planVisible    bool
	subActivity    []ActivityEntry
}

// -- Model --

type model struct {
	cfg             Config
	agents          []Agent
	selected        int // index into treeNodes
	treeNodes       []treeNode
	width, height   int
	mode            int
	textInput       textinput.Model
	tmuxAvailable   bool
	ghAvailable     bool
	openPRSessionID string       // stored for deferred pin in openPRMsg handler
	mergeSessionID  string       // stored for async merge callback
	mergePaneID     string       // stored for async merge callback
	tmuxReady       *atomic.Bool // shared with watcher goroutine
	statePath       string
	selfPaneID      string
	statusMsg       string
	statusMsgTick   int           // tick when statusMsg was set; clears after 3s
	spawningSpinner spinner.Model // bouncing-ball spinner for "Spawning agent..."
	startupDone     bool          // set true when startupMsg arrives
	startupSpinner  spinner.Model // shown in agent list until startupMsg
	capturedLines   []string
	conversation    []ConversationEntry
	tickCount       int
	agentUsage      map[string]Usage
	totalUsage      Usage
	db              *DB
	dbTotalCost     float64
	dbTodayCost     float64

	// History render cache (Layers 2+3)
	renderedHistory   string // cached output of historyContent()
	historyConvLen    int    // len(m.conversation) when cache was built
	historyRightWidth int    // m.rightWidth when cache was built

	// Conversation file offset (Layer 4)
	convFileOffset int64  // byte offset after last JSONL read
	convSessionKey string // projDir+sessionID for cache invalidation

	// Viewports
	agentListVP viewport.Model
	filesVP     viewport.Model
	historyVP   viewport.Model
	messageVP   viewport.Model
	focusedVP   int

	// Layout cache (for mouse routing)
	leftWidth  int
	rightWidth int

	// Diff-specific layout (narrower left, wider right)
	diffLeftWidth  int
	diffRightWidth int

	// Per-agent state cache (keyed by cacheKey)
	agentCaches map[string]*agentCache

	// Subagent tree
	agentSubagents map[string][]SubagentInfo // parentTarget → subagents
	collapsed      map[string]bool           // parentTarget → collapsed state
	dismissed      map[string]bool           // "parentTarget:agentID" → dismissed
	subActivity    []ActivityEntry           // activity log for selected subagent

	// Plan content for selected agent
	planContent  string
	planVisible  bool   // true when plan is shown in message VP
	renderedPlan string // glamour-rendered plan markdown

	// Diff viewer
	diffVisible       bool
	diffFiles         []*gitdiff.File
	diffTreeEntries   []diffTreeEntry
	selectedDiffFile  int
	diffCursor        int             // cursor position in visible tree entries
	diffCollapsedDirs map[string]bool // dirKey → collapsed
	diffExpandedAll   bool            // expand/collapse all context blocks
	diffFileVP        viewport.Model
	diffContentVP     viewport.Model
	diffFuncCtx       []string // per-row function context for sticky header
	diffFilterInput   textinput.Model
	diffFilterActive  bool
	diffFilterText    string

	// Help overlay
	helpVisible bool

	// Close confirmation
	confirmPaneID    string // tmux pane ID (%N) pending close
	confirmSessionID string // session_id of agent pending close

	// Send-key confirmation (guards against phantom keystrokes from mouse escape sequences)
	confirmSendPaneID string // tmux pane ID for pending key send
	confirmSendKey    string // key to send (y, n, 1-9)

	// Jump confirmation (guards against phantom enter from mouse escape sequences)
	confirmJumpPaneID string // tmux pane ID for pending jump

	// Merge confirmation
	confirmMergeSessionID string
	confirmMergePaneID    string
	confirmMergeDir       string
	confirmMergeBranch    string

	// Confirmation cooldown — reject confirmations arriving within 300ms of
	// entering a confirm mode. Phantom keystrokes from escape sequences arrive
	// within microseconds; real users take at least 200-300ms.
	confirmEnteredAt time.Time

	// Z-plugin suggestions for create folder mode
	zEntries     []zEntry // cached z entries from ~/.z
	suggestions  []string // filtered suggestions for current input
	selectedSugg int      // index of highlighted suggestion

	// Skill-aware create wizard state
	availableSkills     []string // display list: ["(none)", "chore", "feature", ...]
	skillsAvailable     bool     // true if skills found AND agent is Claude
	createFolder        string   // folder selected in step 1
	selectedCreateSkill int      // index into availableSkills
	createSkillName     string   // selected skill name ("" if none)

	// Banner
	quote       string           // random quote text selected at startup
	quoteAuthor string           // quote author (empty for fallback quotes)
	nowFunc     func() time.Time // injectable clock for testability

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

// selectedIdentity returns the identity of the currently selected tree node:
// the agent Target and (if a subagent is selected) the SubagentInfo.AgentID.
func (m model) selectedIdentity() (target string, subID string) {
	if m.selected < 0 || m.selected >= len(m.treeNodes) {
		return "", ""
	}
	node := m.treeNodes[m.selected]
	if node.AgentIdx < len(m.agents) {
		target = m.agents[node.AgentIdx].Target
	}
	if node.Sub != nil {
		subID = node.Sub.AgentID
	}
	return target, subID
}

// restoreSelection scans the tree for a node matching the given identity
// and sets m.selected to that position. Falls back to clamping if not found.
func (m *model) restoreSelection(target, subID string) {
	for i, node := range m.treeNodes {
		nodeTarget := ""
		if node.AgentIdx < len(m.agents) {
			nodeTarget = m.agents[node.AgentIdx].Target
		}
		if nodeTarget != target {
			continue
		}
		if subID == "" && node.Sub == nil {
			m.selected = i
			return
		}
		if subID != "" && node.Sub != nil && node.Sub.AgentID == subID {
			m.selected = i
			return
		}
	}
	// Not found — clamp to valid range
	if m.selected >= len(m.treeNodes) {
		m.selected = max(0, len(m.treeNodes)-1)
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

// cacheKey returns the map key for the currently selected node's cache.
func (m model) cacheKey() string {
	target, subID := m.selectedIdentity()
	if subID != "" {
		return target + ":" + subID
	}
	return target
}

// saveCurrentCache persists the current agent's UI state into agentCaches.
// Only source data is cached; derived renders (renderedHistory, renderedPlan)
// and ephemeral data (capturedLines) are regenerated on restore.
func (m *model) saveCurrentCache() {
	key := m.cacheKey()
	if key == "" {
		return // empty tree or out-of-range selection — nothing to save
	}
	// Cap subActivity to reduce memory for long-running subagents.
	// Copy the slice to avoid aliasing with the model's live slice.
	const maxCachedActivity = 300
	var activity []ActivityEntry
	if len(m.subActivity) > maxCachedActivity {
		activity = make([]ActivityEntry, maxCachedActivity)
		copy(activity, m.subActivity[len(m.subActivity)-maxCachedActivity:])
	} else if len(m.subActivity) > 0 {
		activity = make([]ActivityEntry, len(m.subActivity))
		copy(activity, m.subActivity)
	}
	m.agentCaches[key] = &agentCache{
		conversation:   m.conversation,
		convFileOffset: m.convFileOffset,
		convSessionKey: m.convSessionKey,
		planContent:    m.planContent,
		planVisible:    m.planVisible,
		subActivity:    activity,
	}
}

// restoreCurrentCache loads cached UI state for the newly selected agent,
// or zeros out the fields if no cache exists. Derived renders are regenerated
// synchronously from source data to avoid empty panels.
func (m *model) restoreCurrentCache() {
	key := m.cacheKey()
	if c, ok := m.agentCaches[key]; ok && c != nil {
		m.conversation = c.conversation
		m.convFileOffset = c.convFileOffset
		m.convSessionKey = c.convSessionKey
		m.planContent = c.planContent
		m.planVisible = c.planVisible
		m.subActivity = c.subActivity
	} else {
		m.conversation = nil
		m.convFileOffset = 0
		m.convSessionKey = ""
		m.planContent = ""
		m.planVisible = false
		m.subActivity = nil
	}

	// Zero ephemeral/derived fields — regenerated on demand
	m.capturedLines = nil
	m.renderedHistory = ""
	m.historyConvLen = 0
	m.historyRightWidth = 0

	// Regenerate plan render synchronously to avoid empty plan panel
	if m.planContent != "" && m.planVisible {
		m.renderedPlan = renderPlanMarkdown(m.planContent, m.rightWidth-4)
	} else {
		m.renderedPlan = ""
	}
}

func newModel(cfg Config, db *DB) model {
	ti := textinput.New()
	ti.Placeholder = "Type reply..."
	ti.CharLimit = 4096

	dfi := textinput.New()
	dfi.Placeholder = "Filter files..."
	dfi.CharLimit = 256

	s := spinner.New()
	s.Spinner = spinner.Jump
	s.Style = lipgloss.NewStyle().Foreground(textInputColor)

	ss := spinner.New()
	ss.Spinner = spinner.Dot
	ss.Style = lipgloss.NewStyle().Foreground(textInputColor)

	// Discover skills from agent-dashboard plugin cache
	rawSkills := discoverSkills(cfg.Profile.PluginCacheDir)
	skillList := buildSkillList(rawSkills)
	hasSkills := len(skillList) > 0 && strings.Contains(cfg.Profile.Command, "claude")

	return model{
		cfg:               cfg,
		agents:            nil,
		statePath:         cfg.Profile.StateDir,
		selfPaneID:        "",
		tmuxAvailable:     false,
		tmuxReady:         &atomic.Bool{},
		textInput:         ti,
		spawningSpinner:   s,
		startupSpinner:    ss,
		startupDone:       false,
		mode:              modeNormal,
		db:                db,
		agentListVP:       viewport.New(0, 0),
		filesVP:           viewport.New(0, 0),
		historyVP:         viewport.New(0, 0),
		messageVP:         viewport.New(0, 0),
		focusedVP:         focusAgentList,
		diffFileVP:        viewport.New(0, 0),
		diffContentVP:     viewport.New(0, 0),
		diffCollapsedDirs: make(map[string]bool),
		diffFilterInput:   dfi,
		agentCaches:       make(map[string]*agentCache),
		agentSubagents:    make(map[string][]SubagentInfo),
		collapsed:         make(map[string]bool),
		dismissed:         make(map[string]bool),
		quote:             "",
		quoteAuthor:       "",
		nowFunc:           time.Now,
		pathExists:        dirExists,
		availableSkills:   skillList,
		skillsAvailable:   hasSkills,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		deferredStartup(m.statePath, m.db, m.cfg),
		deferredQuote(m.db, m.cfg.Settings.Banner.ShowQuote),
		tickEvery(),
		checkGHAvailable(),
		m.startupSpinner.Tick,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case startupMsg:
		m.startupDone = true
		m.tmuxAvailable = msg.tmuxAvailable
		m.selfPaneID = msg.selfPaneID
		m.tmuxReady.Store(msg.tmuxAvailable)
		cmds := []tea.Cmd{
			loadState(m.statePath, m.tmuxAvailable),
			m.captureSelected(),
		}
		if m.db != nil {
			cmds = append(cmds, loadDBCost(m.db))
		}
		return m, tea.Batch(cmds...)

	case quoteMsg:
		m.quote = msg.text
		m.quoteAuthor = msg.author
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		return m, nil

	case stateUpdatedMsg:
		ApplyIdleOverrides(&msg.state, m.cfg.Profile.ProjectsDir)
		prevTarget, prevSubID := m.selectedIdentity()
		m.agents = SortedAgents(msg.state, m.selfPaneID)
		// Prune maps for agents no longer present
		live := make(map[string]bool, len(m.agents))
		for _, a := range m.agents {
			live[a.Target] = true
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
		for key := range m.agentCaches {
			// Cache keys use "target" for parents or "target:subID" for subagents.
			// Check if key is a live target first (parent agent cache).
			if live[key] {
				continue
			}
			// For subagent caches ("target:subID"), extract parent target.
			parentTarget := key
			if idx := strings.LastIndex(key, ":"); idx > 0 {
				parentTarget = key[:idx]
			}
			if !live[parentTarget] {
				delete(m.agentCaches, key)
				continue
			}
			// Prune dismissed subagent caches
			if m.dismissed[key] {
				delete(m.agentCaches, key)
			}
		}
		m.buildTree()
		m.restoreSelection(prevTarget, prevSubID)
		m.renderedHistory = "" // invalidate cache on state update
		m.convFileOffset = 0   // reset offset — agent list may have changed
		// Also invalidate the current agent's cached entry so restoreCurrentCache
		// doesn't restore stale offsets after this reset.
		if key := m.cacheKey(); key != "" {
			if c, ok := m.agentCaches[key]; ok {
				c.convFileOffset = 0
			}
		}
		m.updateLeftContent()
		m.updateRightContent()
		cmds := []tea.Cmd{m.captureSelected(), m.loadConversation(), loadUsage(m.agents, m.cfg.Profile.ProjectsDir, m.cfg.Profile.SessionsDir), m.loadPlan()}
		cmds = append(cmds, m.loadAllSubagents()...)
		return m, tea.Batch(cmds...)

	case conversationMsg:
		// Only update offset tracking when we have a valid session
		if msg.sessionKey != "" {
			m.convFileOffset = msg.fileOffset
			m.convSessionKey = msg.sessionKey
		}
		if conversationEqual(m.conversation, msg.entries) {
			return m, nil // nothing changed — skip re-render
		}
		m.conversation = msg.entries
		m.renderedHistory = "" // invalidate cache (Layer 2)
		m.updateRightContent()
		// Auto-scroll history to latest when user isn't focused on it
		if m.focusedVP != focusHistory {
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
		// Auto-clear status message: errors get 6s, others 3s
		if m.statusMsg != "" && m.statusMsgTick >= 0 {
			ttl := 3
			if strings.HasPrefix(m.statusMsg, "Create failed") || strings.HasPrefix(m.statusMsg, "Close failed") {
				ttl = 6
			}
			if m.tickCount-m.statusMsgTick >= ttl {
				m.statusMsg = ""
			}
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
			cmds = append(cmds, pruneDead(m.statePath), loadUsage(m.agents, m.cfg.Profile.ProjectsDir, m.cfg.Profile.SessionsDir))
		}
		if m.tickCount%30 == 0 {
			cmds = append(cmds, loadState(m.statePath, m.tmuxAvailable))
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmds []tea.Cmd
		if m.statusMsg == "spawning" {
			var cmd tea.Cmd
			m.spawningSpinner, cmd = m.spawningSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		if !m.startupDone {
			var cmd tea.Cmd
			m.startupSpinner, cmd = m.startupSpinner.Update(msg)
			m.updateLeftContent()
			cmds = append(cmds, cmd)
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

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
		prevTarget2, prevSubID2 := m.selectedIdentity()
		m.agentSubagents[msg.parentTarget] = msg.agents
		m.buildTree()
		m.restoreSelection(prevTarget2, prevSubID2)
		m.updateLeftContent()
		return m, nil

	case createSessionMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Create failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			m.mode = modeNormal
			return m, nil
		}
		m.statusMsgTick = m.tickCount // let "spawning" expire naturally via 3s auto-clear

		// Insert a placeholder agent immediately so the panel doesn't jump
		// when the state file appears on the next tick. The placeholder is
		// naturally replaced when loadState returns with real data.
		if sess, win, pane, ok := parseTarget(msg.target); ok {
			already := false
			for _, a := range m.agents {
				if a.Target == msg.target {
					already = true
					break
				}
			}
			if !already {
				prevTarget, prevSubID := m.selectedIdentity()
				m.agents = append(m.agents, Agent{
					Target:  msg.target,
					Session: sess,
					Window:  win,
					Pane:    pane,
					State:   "running",
				})
				// Re-sort so placeholder appears in correct position
				sort.Slice(m.agents, func(i, j int) bool {
					pi := statePriority[m.agents[i].State]
					pj := statePriority[m.agents[j].State]
					if pi == 0 {
						pi = 99
					}
					if pj == 0 {
						pj = 99
					}
					if pi != pj {
						return pi < pj
					}
					if m.agents[i].Window != m.agents[j].Window {
						return m.agents[i].Window < m.agents[j].Window
					}
					return m.agents[i].Pane < m.agents[j].Pane
				})
				m.buildTree()
				m.restoreSelection(prevTarget, prevSubID)
				m.resizeViewports() // recalculate viewport dimensions for new agent count
			}
		}

		m.updateRightContent()
		return m, tea.Batch(loadState(m.statePath, m.tmuxAvailable), selectPane(msg.target))

	case closeResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Close failed: %v", msg.err)
		} else {
			m.statusMsg = "Pane closed"
		}
		m.statusMsgTick = m.tickCount
		return m, tea.Batch(loadState(m.statePath, m.tmuxAvailable), pruneDead(m.statePath))

	case pruneDeadMsg:
		if msg.removed > 0 {
			return m, loadState(m.statePath, m.tmuxAvailable)
		}
		return m, nil

	case captureResultMsg:
		m.capturedLines = msg.lines
		m.updateRightContent()
		// Auto-scroll live output to latest when user isn't focused on it
		// Skip when plan is visible — the user may be reading the plan
		if m.focusedVP != focusMessage && !m.planVisible {
			m.messageVP.GotoBottom()
		}
		return m, nil

	case jumpResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Jump failed: %v", msg.err)
		} else {
			m.statusMsg = "Jumped — switch back to this window for dashboard"
		}
		m.statusMsgTick = m.tickCount
		return m, nil

	case openEditorMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Editor failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Opened %s", m.cfg.Editor)
		}
		m.statusMsgTick = m.tickCount
		return m, nil

	case openPRMsg:
		sessionID := m.openPRSessionID
		m.openPRSessionID = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("PR link failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		// Deferred pin: when gh detected an existing PR, pin to "pr" now
		if sessionID != "" && msg.hasPR {
			m.statusMsg = "Opened PR page in browser"
			m.statusMsgTick = m.tickCount
			return m, pinAgentStateCmd(m.statePath, sessionID, "pr")
		}
		if msg.hasPR {
			m.statusMsg = "Opened PR page in browser"
		} else {
			m.statusMsg = "Opened compare page in browser"
		}
		m.statusMsgTick = m.tickCount
		return m, nil

	case ghAvailableMsg:
		m.ghAvailable = msg.available
		return m, nil

	case mergePRMsg:
		sessionID := m.mergeSessionID
		paneID := m.mergePaneID
		m.mergeSessionID = ""
		m.mergePaneID = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Merge failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		m.statusMsg = "PR merged — cleanup sent"
		m.statusMsgTick = m.tickCount
		cmds := []tea.Cmd{
			pinAgentStateCmd(m.statePath, sessionID, "merged"),
		}
		if paneID != "" {
			cmds = append(cmds, sendReply(paneID,
				"The PR has been merged. Please clean up: remove any worktrees and temporary branches."))
		}
		return m, tea.Batch(cmds...)

	case pinStateMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Pin state failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
		}
		return m, nil

	case rawKeySentMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Key send failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			// Exit reply mode since the pane is unreachable
			if m.mode == modeReply {
				m.mode = modeNormal
				m.textInput.Reset()
				m.updateRightContent()
			}
		}
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

	case diffMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Diff failed: %v", msg.err)
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		if len(msg.files) == 0 {
			m.statusMsg = "No changes"
			m.statusMsgTick = m.tickCount
			return m, nil
		}
		m.diffFiles = msg.files
		m.diffCollapsedDirs = make(map[string]bool)
		m.diffFilterText = ""
		m.diffFilterActive = false
		m.diffFilterInput.Reset()
		m.buildDiffTreeEntries()
		m.selectedDiffFile = 0
		m.diffCursor = 0
		m.diffVisible = true
		m.updateDiffContent()
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
	panelHeight := m.height - 5 - m.bannerHeight()

	m.agentListVP.Width = m.leftWidth
	m.agentListVP.Height = panelHeight

	filesH, historyH, msgH := panelHeights(panelHeight, defaultHeaderLines)

	m.filesVP.Width = m.rightWidth
	m.filesVP.Height = filesH

	m.historyVP.Width = m.rightWidth
	m.historyVP.Height = historyH

	m.messageVP.Width = m.rightWidth
	m.messageVP.Height = msgH

	m.textInput.Width = m.rightWidth - 12 // account for "Reply: " prefix + padding

	// Diff viewer viewports (narrower left panel for file list)
	m.diffLeftWidth = m.width*20/100 - 2
	if m.diffLeftWidth < 20 {
		m.diffLeftWidth = 20
	}
	m.diffRightWidth = m.width - m.diffLeftWidth - 4
	diffPanelHeight := panelHeight - 4 // header + padding
	if diffPanelHeight < 3 {
		diffPanelHeight = 3
	}
	m.diffFileVP.Width = m.diffLeftWidth
	m.diffFileVP.Height = diffPanelHeight
	m.diffContentVP.Width = m.diffRightWidth
	m.diffContentVP.Height = diffPanelHeight

	if m.planContent != "" && m.planVisible {
		m.renderedPlan = renderPlanMarkdown(m.planContent, m.rightWidth-4)
	}
	m.renderedHistory = "" // invalidate cache on resize
	m.updateLeftContent()
	m.updateRightContent()
}
