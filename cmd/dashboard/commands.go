package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// -- Commands --

func (m model) captureSelected() tea.Cmd {
	agent := m.selectedAgent()
	if !m.tmuxAvailable || agent == nil {
		return nil
	}
	target := agent.Target
	return func() tea.Msg {
		lines, err := TmuxCapture(target, 15)
		if err != nil {
			return captureResultMsg{lines: nil}
		}
		return captureResultMsg{lines: lines}
	}
}

func (m model) loadConversation() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil || m.selectedSubagent() != nil {
		return nil // don't load conversation for subagent nodes
	}
	if agent.Cwd == "" {
		return nil
	}
	slug := ProjectSlug(agent.Cwd)
	projDir := filepath.Join(ConversationsDir(), slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = FindSessionID(cwd)
		}
		if sessionID == "" {
			return conversationMsg{entries: nil}
		}
		entries := ReadConversation(projDir, sessionID, 50)
		return conversationMsg{entries: entries}
	}
}

// loadSelectionData loads the right data for the current tree selection.
func (m model) loadSelectionData() tea.Cmd {
	if m.selectedSubagent() != nil {
		return m.loadSubagentActivity()
	}
	return tea.Batch(m.captureSelected(), m.loadConversation(), m.loadPlan())
}

// loadSubagentActivity loads activity log for the selected subagent.
func (m model) loadSubagentActivity() tea.Cmd {
	agent := m.selectedAgent()
	sub := m.selectedSubagent()
	if agent == nil || sub == nil || agent.Cwd == "" {
		return nil
	}
	slug := ProjectSlug(agent.Cwd)
	projDir := filepath.Join(ConversationsDir(), slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd
	agentID := sub.AgentID

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = FindSessionID(cwd)
		}
		if sessionID == "" {
			return activityMsg{entries: nil}
		}
		jsonlPath := SubagentJSONLPath(projDir, sessionID, agentID)
		entries := ReadActivityLog(jsonlPath, 500)
		return activityMsg{entries: entries}
	}
}

// loadAllSubagents loads subagent info for all agents.
func (m model) loadAllSubagents() []tea.Cmd {
	var cmds []tea.Cmd
	for _, agent := range m.agents {
		if agent.Cwd == "" {
			continue
		}
		a := agent // copy for closure
		cmds = append(cmds, func() tea.Msg {
			sid := a.SessionID
			if sid == "" {
				sid = FindSessionID(a.Cwd)
			}
			if sid == "" {
				return subagentsMsg{parentTarget: a.Target, agents: nil}
			}
			slug := ProjectSlug(a.Cwd)
			projDir := filepath.Join(ConversationsDir(), slug)
			subs := FindSubagents(projDir, sid)
			return subagentsMsg{parentTarget: a.Target, agents: subs}
		})
	}
	return cmds
}

func pruneDead(statePath string) tea.Cmd {
	return pruneDeadWithRenames(statePath, nil)
}

func pruneDeadWithRenames(statePath string, renames map[string]string) tea.Cmd {
	return func() tea.Msg {
		livePanes := TmuxListPanes()
		if len(livePanes) == 0 {
			return pruneDeadMsg{removed: 0}
		}
		removed := PruneDead(statePath, livePanes, renames)
		return pruneDeadMsg{removed: removed}
	}
}

func persistUsage(db *DB, agents []Agent, perAgent map[string]Usage) tea.Cmd {
	today := time.Now().Format("2006-01-02")
	type entry struct {
		sessionID string
		model     string
		usage     Usage
	}
	var entries []entry
	for _, agent := range agents {
		u, ok := perAgent[agent.Target]
		if !ok || u.OutputTokens == 0 {
			continue
		}
		sid := agent.SessionID
		if sid == "" {
			continue
		}
		entries = append(entries, entry{sessionID: sid, model: u.Model, usage: u})
	}

	return func() tea.Msg {
		for _, e := range entries {
			// Calculate delta: cumulative cost from JSONL minus what's already
			// stored for this session on previous days. This prevents double-counting
			// when a session spans multiple days.
			prevCost, err := db.SessionCostExcludingDate(e.sessionID, today)
			if err != nil {
				// Skip this entry — writing the full cumulative would cause double-counting
				continue
			}

			ratio := 1.0
			if e.usage.CostUSD > 0 && prevCost > 0 {
				ratio = (e.usage.CostUSD - prevCost) / e.usage.CostUSD
				if ratio < 0 {
					ratio = 0
				}
			}

			deltaUsage := Usage{
				InputTokens:      int(float64(e.usage.InputTokens) * ratio),
				OutputTokens:     int(float64(e.usage.OutputTokens) * ratio),
				CacheReadTokens:  int(float64(e.usage.CacheReadTokens) * ratio),
				CacheWriteTokens: int(float64(e.usage.CacheWriteTokens) * ratio),
				CostUSD:          e.usage.CostUSD - prevCost,
				Model:            e.usage.Model,
			}
			if deltaUsage.CostUSD < 0 {
				deltaUsage.CostUSD = 0
			}

			_ = db.UpsertUsage(today, e.sessionID, e.model, deltaUsage)
		}
		return persistResultMsg{}
	}
}

func loadDBCost(db *DB) tea.Cmd {
	return func() tea.Msg {
		today := time.Now().Format("2006-01-02")
		return dbCostMsg{
			total:     db.TotalCost(),
			todayCost: db.CostForDate(today),
		}
	}
}

func closePane(target, statePath string) tea.Cmd {
	return func() tea.Msg {
		// Snapshot pane IDs before kill to detect window renumbering
		beforePanes := TmuxListPanesWithID()

		err := TmuxKillPane(target)
		if err != nil {
			return closeResultMsg{err: err}
		}

		// Snapshot after kill to detect renumbered targets
		afterPanes := TmuxListPanesWithID()
		// If beforePanes is nil (tmux timeout), no renames will be detected.
		// PruneDead will clean up stale targets on the next tick.
		renames := BuildTargetRenames(beforePanes, afterPanes, target)

		// Remove killed agent and apply renames for surviving agents.
		// Best-effort: if the write fails, PruneDead on the next tick will clean up.
		sf := ReadState(statePath)
		delete(sf.Agents, target)
		for oldTarget, newTarget := range renames {
			if agent, ok := sf.Agents[oldTarget]; ok {
				delete(sf.Agents, oldTarget)
				agent.Target = newTarget
				sf.Agents[newTarget] = agent
			}
		}
		data, _ := json.Marshal(sf)
		_ = os.WriteFile(statePath, data, 0644)

		return closeResultMsg{err: nil, renames: renames}
	}
}

func loadUsage(agents []Agent) tea.Cmd {
	agentsCopy := make([]Agent, len(agents))
	copy(agentsCopy, agents)
	return func() tea.Msg {
		perAgent, total := ReadAllUsage(agentsCopy)
		return usageMsg{perAgent: perAgent, total: total}
	}
}

// notifyNeedsAttention sends a desktop notification when an agent transitions
// to "needs attention" state. Uses terminal-notifier if available, falls back
// to osascript.
func notifyNeedsAttention(agent Agent) tea.Cmd {
	title := "Claude Code"
	body := "Agent needs attention"
	if agent.LastMessagePreview != "" {
		body = agent.LastMessagePreview
		runes := []rune(body)
		if len(runes) > 100 {
			body = string(runes[:100]) + "..."
		}
	}
	subtitle := ""
	if agent.Branch != "" {
		subtitle = agent.Branch
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try terminal-notifier first
		if _, err := exec.LookPath("terminal-notifier"); err == nil {
			args := []string{"-title", title, "-message", body, "-group", "claude-dashboard-" + agent.Target}
			if subtitle != "" {
				args = append(args, "-subtitle", subtitle)
			}
			args = append(args, "-sound", "default")

			// Add tmux click action — ValidateTarget guarantees target contains
			// only [a-zA-Z0-9_.\-:] so no shell metacharacters are possible.
			if ValidateTarget(agent.Target) == nil {
				sw := extractSessionWindow(agent.Target)
				action := fmt.Sprintf("tmux select-window -t %s && tmux select-pane -t %s", sw, agent.Target)
				args = append(args, "-execute", action)
			}

			_ = exec.CommandContext(ctx, "terminal-notifier", args...).Run()
			return notifyResultMsg{}
		}

		// Fallback: osascript
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		if subtitle != "" {
			script = fmt.Sprintf(`display notification %q with title %q subtitle %q sound name "default"`, body, title, subtitle)
		}
		_ = exec.CommandContext(ctx, "osascript", "-e", script).Run()
		return notifyResultMsg{}
	}
}

func loadState(path string) tea.Cmd {
	return func() tea.Msg {
		return stateUpdatedMsg{state: ReadState(path)}
	}
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func jumpToAgent(target string) tea.Cmd {
	return func() tea.Msg {
		return jumpResultMsg{err: TmuxJump(target)}
	}
}

func selectPane(target string) tea.Cmd {
	return func() tea.Msg {
		return selectPaneMsg{err: TmuxSelectPane(target)}
	}
}

func sendReply(target, text string) tea.Cmd {
	return func() tea.Msg {
		return sendResultMsg{err: TmuxSendKeys(target, text)}
	}
}

// findWindowForRepo finds an existing tmux session:window for a given folder
// by scanning existing agents' working directories.
// It first tries an exact path match, then falls back to repo-name matching
// only when at least one side is a worktree path (to avoid false collisions
// between unrelated repos that share the same basename).
func findWindowForRepo(agents []Agent, folder, selfTarget string) (string, bool) {
	// Pass 1: exact path match
	for _, agent := range agents {
		if agent.Target == selfTarget {
			continue
		}
		if agent.Cwd == folder {
			return fmt.Sprintf("%s:%d", agent.Session, agent.Window), true
		}
	}

	// Pass 2: repo-name match, only when at least one side is a worktree
	folderRepo := repoFromCwd(folder)
	if folderRepo == "" {
		return "", false
	}
	folderIsWorktree := strings.Contains(folder, "/worktrees/")
	for _, agent := range agents {
		if agent.Target == selfTarget {
			continue
		}
		agentIsWorktree := strings.Contains(agent.Cwd, "/worktrees/")
		if !folderIsWorktree && !agentIsWorktree {
			continue
		}
		if repoFromCwd(agent.Cwd) == folderRepo {
			return fmt.Sprintf("%s:%d", agent.Session, agent.Window), true
		}
	}
	return "", false
}

// findWindowByName returns the first window whose Name equals repoName,
// excluding the window that contains the dashboard pane (selfTarget).
func findWindowByName(windows []TmuxWindowInfo, repoName, session, selfTarget string) (string, bool) {
	dashboardSW := extractSessionWindow(selfTarget)
	for _, w := range windows {
		candidate := fmt.Sprintf("%s:%d", session, w.Index)
		if w.Name == repoName && candidate != dashboardSW {
			return candidate, true
		}
	}
	return "", false
}

// expandPath expands ~ and resolves to an absolute path.
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}

const maxPanesPerWindow = 4

// validateFolder expands and validates a folder path, returning the absolute path.
func validateFolder(path string) (string, error) {
	absFolder, err := expandPath(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	info, err := os.Stat(absFolder)
	if err != nil {
		return "", fmt.Errorf("folder not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", absFolder)
	}
	return absFolder, nil
}

// createSession creates a new Claude Code session in a tmux pane.
func createSession(folder string, agents []Agent, selfTarget string) tea.Cmd {
	return func() tea.Msg {
		absFolder, err := validateFolder(folder)
		if err != nil {
			return createSessionMsg{err: err}
		}

		session := extractSession(selfTarget)
		repoName := sanitizeWindowName(repoFromCwd(absFolder))
		if repoName == "" {
			repoName = "claude"
		}

		var newTarget string

		// Check for existing window
		sw, found := findWindowForRepo(agents, absFolder, selfTarget)
		if !found {
			// Fallback: check window names
			windows, wErr := TmuxListWindows(session)
			if wErr == nil {
				sw, found = findWindowByName(windows, repoName, session, selfTarget)
			}
		}

		if found {
			// Check pane limit
			count, cErr := TmuxCountPanes(sw)
			if cErr != nil {
				return createSessionMsg{err: fmt.Errorf("cannot count panes: %w", cErr)}
			}
			if count >= maxPanesPerWindow {
				return createSessionMsg{err: fmt.Errorf("4-pane limit reached for %s", repoName)}
			}
			newTarget, err = TmuxSplitWindow(sw, absFolder)
		} else {
			newTarget, err = TmuxNewWindow(session, repoName, absFolder)
		}

		if err != nil {
			return createSessionMsg{err: err}
		}

		// Launch Claude in the new pane
		if sendErr := TmuxSendKeys(newTarget, "claude"); sendErr != nil {
			return createSessionMsg{err: fmt.Errorf("failed to launch claude: %w", sendErr)}
		}

		return createSessionMsg{target: newTarget}
	}
}

// PlansDir returns the Claude plans directory (~/.claude/plans).
func PlansDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return filepath.Join(home, ".claude", "plans")
}

func (m model) loadPlan() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil || agent.Cwd == "" {
		return nil
	}
	slug := ProjectSlug(agent.Cwd)
	projDir := filepath.Join(ConversationsDir(), slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = FindSessionID(cwd)
		}
		if sessionID == "" {
			return planMsg{content: ""}
		}
		planSlug := ReadPlanSlug(projDir, sessionID)
		if planSlug == "" {
			return planMsg{content: ""}
		}
		content := ReadPlanContent(PlansDir(), planSlug)
		return planMsg{content: content}
	}
}

func sendRawKey(target, key string) tea.Cmd {
	return func() tea.Msg {
		return sendResultMsg{err: TmuxSendRaw(target, key)}
	}
}

func watchStateFile(path string, p *tea.Program) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					p.Send(stateUpdatedMsg{state: ReadState(path)})
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	if err := watcher.Add(path); err != nil {
		dir := filepath.Dir(path)
		if dirErr := watcher.Add(dir); dirErr != nil {
			watcher.Close()
			return nil, fmt.Errorf("cannot watch %s: %w", path, err)
		}
	}

	return watcher, nil
}
