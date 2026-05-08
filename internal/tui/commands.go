package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/db"
	"github.com/bjornjee/agent-dashboard/internal/diagrams"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/gh"
	"github.com/bjornjee/agent-dashboard/internal/repo"
	"github.com/bjornjee/agent-dashboard/internal/repowin"
	"github.com/bjornjee/agent-dashboard/internal/state"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
	"github.com/bjornjee/agent-dashboard/internal/usage"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

// -- Deferred startup commands --

// deferredStartup runs all blocking startup work (tmux probes, stale cleanup)
// concurrently via errgroup so the TUI can render immediately.
func deferredStartup(stateDir string, database *db.DB, cfg domain.Config) tea.Cmd {
	return func() tea.Msg {
		var (
			tmuxAvail   bool
			selfPane    string
			livePaneIDs map[string]bool
		)

		g := new(errgroup.Group)

		g.Go(func() error {
			tmuxAvail = tmux.TmuxIsAvailable()
			return nil
		})

		g.Go(func() error {
			selfPane = tmux.TmuxResolvePaneID()
			return nil
		})

		g.Go(func() error {
			livePaneIDs = tmux.TmuxListLivePaneIDs()
			return nil
		})

		_ = g.Wait()

		state.CleanStale(stateDir, 10*60, livePaneIDs)

		return startupMsg{tmuxAvailable: tmuxAvail, selfPaneID: selfPane}
	}
}

// deferredQuote fetches the daily quote in the background so the banner
// renders instantly with an empty quote, then fills in once ready.
func deferredQuote(database *db.DB, showQuote bool) tea.Cmd {
	if !showQuote {
		return nil
	}
	return func() tea.Msg {
		q, a := pickQuote(database)
		return quoteMsg{text: q, author: a}
	}
}

// -- Commands --

func (m model) captureSelected() tea.Cmd {
	agent := m.selectedAgent()
	if !m.tmuxAvailable || agent == nil || agent.TmuxPaneID == "" {
		return nil
	}
	paneID := agent.TmuxPaneID
	return func() tea.Msg {
		target := tmux.ResolveTarget(paneID)
		if target == "" {
			return captureResultMsg{lines: nil}
		}
		lines, err := tmux.TmuxCapture(target, 20)
		if err != nil {
			return captureResultMsg{lines: nil}
		}
		return captureResultMsg{lines: trimTrailingBlankLines(lines)}
	}
}

// containsTrustPrompt returns true if the pane buffer contains
// Claude Code's folder trust dialog. Matches the select menu options
// rendered by the trust dialog component.
func containsTrustPrompt(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "Yes, I trust this folder") {
			return true
		}
	}
	return false
}

func (m model) captureSpawning() tea.Cmd {
	if !m.tmuxAvailable || m.spawningTarget == "" || m.trustDetected {
		return nil
	}
	target := m.spawningTarget
	return func() tea.Msg {
		lines, err := tmux.TmuxCapture(target, 30)
		if err != nil {
			return spawningCaptureMsg{lines: nil, target: target}
		}
		return spawningCaptureMsg{lines: trimTrailingBlankLines(lines), target: target}
	}
}

func jumpToTarget(target string) tea.Cmd {
	return func() tea.Msg {
		return jumpResultMsg{err: tmux.TmuxJump(target)}
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
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	sessionsDir := m.cfg.Profile.SessionsDir
	agentCopy := *agent

	// Capture offset state for incremental reading
	sessionKey := projDir + ":" + sessionID
	prevOffset := m.convFileOffset
	var prevEntries []domain.ConversationEntry
	if m.convSessionKey == sessionKey {
		prevEntries = m.conversation
	} else {
		prevOffset = 0 // different session — full re-read
	}

	return func() tea.Msg {
		if sessionID == "" {
			// Resolve topology so we can probe Locate against the worktree
			// root and source repo root as well as the launch cwd. This
			// covers agents launched from a subdirectory of a worktree
			// where Claude's session JSON recorded a different path.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			top, _ := resolveAgentTopology(ctx, agentCopy)
			sessionID = conversation.Locate(sessionsDir, sessionCandidates(agentCopy, top)...)
		}
		if sessionID == "" {
			return conversationMsg{entries: nil}
		}
		entries, newOffset := conversation.ReadConversationIncremental(projDir, sessionID, 50, prevEntries, prevOffset)
		return conversationMsg{
			entries:    entries,
			fileOffset: newOffset,
			sessionKey: projDir + ":" + sessionID,
		}
	}
}

// loadSelectionData loads the right data for the current tree selection.
func (m model) loadSelectionData() tea.Cmd {
	if m.selectedSubagent() != nil {
		return m.loadSubagentActivity()
	}
	return tea.Batch(m.captureSelected(), m.loadConversation(), m.loadPlan(), m.loadDiagrams(), m.loadFilesChanged())
}

// loadFilesChanged computes changed files for the selected agent via git.
//
// `git -C <dir>` from any path inside a working tree resolves to that
// working tree, so EffectiveDir (Cwd or WorktreeCwd) is sufficient even
// when Cwd is a subdirectory. We don't need topology resolution here.
func (m model) loadFilesChanged() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil {
		return nil
	}
	dir := agent.EffectiveDir()
	if dir == "" {
		return nil
	}
	target := agent.Target
	return func() tea.Msg {
		base := findMergeBase(dir)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := gitRunner.Output(ctx, "git", "-C", dir, "diff", "--name-status", base)
		if err != nil {
			return filesChangedMsg{target: target}
		}
		return filesChangedMsg{target: target, files: parseNameStatus(string(out))}
	}
}

// parseNameStatus converts "git diff --name-status" output to ["+file", "~file", "-file"].
func parseNameStatus(out string) []string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	prefixMap := map[byte]string{'A': "+", 'M': "~", 'D': "-"}
	var result []string
	for _, line := range strings.Split(trimmed, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || len(parts[0]) == 0 {
			continue
		}
		file := parts[len(parts)-1] // last field handles renames (R100\told\tnew)
		prefix := "~"
		if p, ok := prefixMap[parts[0][0]]; ok {
			prefix = p
		}
		result = append(result, prefix+file)
	}
	return result
}

// loadSubagentActivity loads activity log for the selected subagent.
func (m model) loadSubagentActivity() tea.Cmd {
	agent := m.selectedAgent()
	sub := m.selectedSubagent()
	if agent == nil || sub == nil || agent.Cwd == "" {
		return nil
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	agentCopy := *agent
	agentID := sub.AgentID
	sessionsDir := m.cfg.Profile.SessionsDir

	return func() tea.Msg {
		if sessionID == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			top, _ := resolveAgentTopology(ctx, agentCopy)
			sessionID = conversation.Locate(sessionsDir, sessionCandidates(agentCopy, top)...)
		}
		if sessionID == "" {
			return activityMsg{entries: nil}
		}
		jsonlPath := conversation.SubagentJSONLPath(projDir, sessionID, agentID)
		entries := conversation.ReadActivityLog(jsonlPath, 500)
		return activityMsg{entries: entries}
	}
}

// loadAllSubagents loads subagent info for all agents.
func (m model) loadAllSubagents() []tea.Cmd {
	projectsDir := m.cfg.Profile.ProjectsDir
	sessionsDir := m.cfg.Profile.SessionsDir
	var cmds []tea.Cmd
	for _, agent := range m.agents {
		if agent.Cwd == "" {
			continue
		}
		a := agent // copy for closure
		cmds = append(cmds, func() tea.Msg {
			sid := a.SessionID
			if sid == "" {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				top, _ := resolveAgentTopology(ctx, a)
				sid = conversation.Locate(sessionsDir, sessionCandidates(a, top)...)
			}
			if sid == "" {
				return subagentsMsg{parentTarget: a.Target, agents: nil}
			}
			slug := conversation.ProjectSlug(a.Cwd)
			projDir := filepath.Join(projectsDir, slug)
			subs := conversation.FindSubagents(projDir, sid)
			return subagentsMsg{parentTarget: a.Target, agents: subs}
		})
	}
	return cmds
}

func pruneDead(statePath string) tea.Cmd {
	return func() tea.Msg {
		livePaneIDs := tmux.TmuxListLivePaneIDs()
		if len(livePaneIDs) == 0 {
			return pruneDeadMsg{removed: 0}
		}
		removed := state.PruneDead(statePath, livePaneIDs)
		return pruneDeadMsg{removed: removed}
	}
}

func persistUsage(database *db.DB, agents []domain.Agent, perAgent map[string]domain.Usage) tea.Cmd {
	today := time.Now().Format("2006-01-02")
	type entry struct {
		sessionID string
		model     string
		usage     domain.Usage
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
			prevCost, err := database.SessionCostExcludingDate(e.sessionID, today)
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

			deltaUsage := domain.Usage{
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

			_ = database.UpsertUsage(today, e.sessionID, e.model, deltaUsage)
		}
		return persistResultMsg{}
	}
}

func loadDBDailyUsage(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		today := time.Now().Format("2006-01-02")
		since := time.Now().AddDate(0, 0, -6) // rolling 7 days including today
		return dbDailyUsageMsg{
			total:     database.TotalCostForProvider("claude"),
			todayCost: database.CostForDateAndProvider(today, "claude"),
			days:      database.UsageByDayForProvider(since, "claude"),
		}
	}
}

// closePane kills a tmux pane and removes its agent state file.
// paneID is the immutable tmux pane ID (%N), sessionID is the agent's session_id.
func closePane(paneID, sessionID, stateDir string) tea.Cmd {
	return func() tea.Msg {
		// Resolve current target from pane ID for the kill command
		target := tmux.ResolveTarget(paneID)
		if target == "" {
			// Pane already dead — just clean up the file
			_ = state.RemoveAgent(stateDir, sessionID)
			return closeResultMsg{err: nil}
		}
		err := tmux.TmuxKillPane(target)
		if err != nil {
			return closeResultMsg{err: err}
		}
		_ = state.RemoveAgent(stateDir, sessionID)
		return closeResultMsg{err: nil}
	}
}

// pinAgentStateCmd writes a pinned_state to the agent's JSON file.
func pinAgentStateCmd(stateDir, sessionID, pinnedState string) tea.Cmd {
	return func() tea.Msg {
		err := state.PinAgentState(stateDir, sessionID, pinnedState)
		return pinStateMsg{err: err}
	}
}

func loadUsage(agents []domain.Agent, projectsDir, sessionsDir string) tea.Cmd {
	agentsCopy := make([]domain.Agent, len(agents))
	copy(agentsCopy, agents)
	return func() tea.Msg {
		perAgent, total := usage.ReadAllUsage(agentsCopy, projectsDir, sessionsDir)
		return usageMsg{perAgent: perAgent, total: total}
	}
}

func loadRateLimit() tea.Cmd {
	return func() tea.Msg {
		rl, _ := usage.FetchRateLimit(context.Background())
		return rateLimitMsg{rateLimit: rl}
	}
}

func loadCodexUsage(sessionsDir string) tea.Cmd {
	return func() tea.Msg {
		since := time.Now().AddDate(0, 0, -6) // rolling 7 days including today
		days := usage.ReadCodexDailyUsage(sessionsDir, since)
		return codexUsageMsg{days: days}
	}
}

func persistCodexUsage(database *db.DB, days []usage.CodexDayUsage) tea.Cmd {
	return func() tea.Msg {
		for _, d := range days {
			if d.OutputTokens == 0 && d.InputTokens == 0 {
				continue
			}
			u := domain.Usage{
				InputTokens:     d.InputTokens,
				OutputTokens:    d.OutputTokens,
				CacheReadTokens: d.CachedInputTokens,
				CostUSD:         d.CostUSD,
			}
			// Use date as session_id component since Codex sessions are date-partitioned
			_ = database.UpsertUsageWithProvider(d.Date, "codex-daily", "codex", "", u)
		}
		return codexPersistMsg{}
	}
}

func loadCodexDBUsage(database *db.DB) tea.Cmd {
	return func() tea.Msg {
		today := time.Now().Format("2006-01-02")
		since := time.Now().AddDate(0, 0, -6) // rolling 7 days including today
		return codexDBUsageMsg{
			days:      database.UsageByDayForProvider(since, "codex"),
			totalCost: database.TotalCostForProvider("codex"),
			todayCost: database.CostForDateAndProvider(today, "codex"),
		}
	}
}

func loadState(path string, tmuxAvailable bool) tea.Cmd {
	return func() tea.Msg {
		sf := state.ReadState(path)
		var paneCwds map[string]string
		if tmuxAvailable {
			state.ResolveAgentTargets(&sf, tmux.TmuxListPaneTargets())
			paneCwds = tmux.TmuxListPaneCwds()
		}
		state.ResolveAgentBranches(&sf, paneCwds)
		state.ApplyPinnedStates(&sf)
		return stateUpdatedMsg{state: sf}
	}
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func jumpToAgent(paneID string) tea.Cmd {
	return func() tea.Msg {
		target := tmux.ResolveTarget(paneID)
		if target == "" {
			return jumpResultMsg{err: fmt.Errorf("pane %s no longer exists", paneID)}
		}
		return jumpResultMsg{err: tmux.TmuxJump(target)}
	}
}

func selectPane(target string) tea.Cmd {
	return func() tea.Msg {
		return selectPaneMsg{err: tmux.TmuxSelectPane(target)}
	}
}

// isSelfPane returns true if paneID matches the dashboard's own pane,
// indicating that the send should be blocked to prevent self-messaging.
func isSelfPane(paneID, selfPaneID string) bool {
	return selfPaneID != "" && paneID == selfPaneID
}

func sendReply(paneID, text, selfPaneID string) tea.Cmd {
	return func() tea.Msg {
		if isSelfPane(paneID, selfPaneID) {
			return sendResultMsg{err: fmt.Errorf("refusing to send to dashboard pane %s", paneID)}
		}
		target := tmux.ResolveTarget(paneID)
		if target == "" {
			return sendResultMsg{err: fmt.Errorf("pane %s no longer exists", paneID)}
		}
		return sendResultMsg{err: tmux.TmuxSendKeys(target, text)}
	}
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

const maxPanesPerWindow = repowin.MaxPanesPerWindow

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

// buildPrompt constructs the initial agent prompt from skill and message.
// Returns "" if both are empty.
func buildPrompt(skill, message string) string {
	switch {
	case skill != "" && message != "":
		return "/" + skill + " " + message
	case skill != "":
		return "/" + skill
	case message != "":
		return message
	default:
		return ""
	}
}

// shellQuote wraps s in single quotes for safe shell interpolation.
// Any embedded single quotes are escaped as '\” (end quote, escaped quote, start quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// createSession creates a new agent session in a tmux pane (no initial prompt).
func createSession(folder string, agents []domain.Agent, selfPaneID string, profile domain.AgentProfile) tea.Cmd {
	return createSessionWithPrompt(folder, agents, selfPaneID, profile, "", "")
}

// createSessionWithPrompt creates a new agent session with an optional skill and message.
func createSessionWithPrompt(folder string, agents []domain.Agent, selfPaneID string, profile domain.AgentProfile, skill, message string) tea.Cmd {
	return func() tea.Msg {
		absFolder, err := validateFolder(folder)
		if err != nil {
			return createSessionMsg{err: err}
		}

		selfTarget := tmux.ResolveTarget(selfPaneID)
		session := tmux.ExtractSession(selfTarget)
		dashboardSW := tmux.ExtractSessionWindow(selfTarget)
		repoName := repowin.SanitizeWindowName(repowin.RepoFromCwd(absFolder))
		if repoName == "" {
			repoName = profile.Command
		}

		var newTarget string

		// Check for existing window
		sw, found := repowin.FindWindowForRepo(agents, absFolder, selfPaneID)
		if !found {
			// Fallback: check window names
			windows, wErr := tmux.TmuxListWindows(session)
			if wErr == nil {
				sw, found = repowin.FindWindowByName(windows, repoName, session, dashboardSW)
			}
		}

		// Build the shell command to run in the new pane.
		// Passing it directly to new-window/split-window makes it the
		// pane's initial process, avoiding tmux send-keys buffer limits.
		cmd := profile.Command
		if prompt := buildPrompt(skill, message); prompt != "" {
			cmd = profile.Command + " " + shellQuote(prompt)
		}

		if found {
			// Check pane limit; if the window no longer exists (stale agent
			// state), fall through and create a fresh window instead.
			count, cErr := tmux.TmuxCountPanes(sw)
			if cErr != nil {
				found = false
			} else if count >= maxPanesPerWindow {
				return createSessionMsg{err: fmt.Errorf("8-pane limit reached for %s", repoName)}
			} else {
				newTarget, err = tmux.TmuxSplitWindow(sw, absFolder, cmd)
			}
		}
		if !found {
			newTarget, err = tmux.TmuxNewWindow(session, repoName, absFolder, cmd)
		}

		if err != nil {
			return createSessionMsg{err: err}
		}

		return createSessionMsg{target: newTarget}
	}
}

func (m model) loadPlan() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil || agent.Cwd == "" {
		return nil
	}
	slug := conversation.ProjectSlug(agent.Cwd)
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	agentCopy := *agent
	plansDir := m.cfg.Profile.PlansDir
	sessionsDir := m.cfg.Profile.SessionsDir
	delegatedToolUseID := agent.DelegatedPlanToolUseID

	return func() tea.Msg {
		if sessionID == "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			top, _ := resolveAgentTopology(ctx, agentCopy)
			sessionID = conversation.Locate(sessionsDir, sessionCandidates(agentCopy, top)...)
		}
		if sessionID == "" {
			return planMsg{content: ""}
		}
		// Prefer the delegated-Plan-subagent pointer if set; the plan is
		// persisted only inside the parent JSONL as a tool_result.
		if delegatedToolUseID != "" {
			if content := conversation.ReadDelegatedPlanContent(projDir, sessionID, delegatedToolUseID); content != "" {
				return planMsg{content: content}
			}
		}
		planSlug := conversation.ReadPlanSlug(projDir, sessionID)
		if planSlug == "" {
			return planMsg{content: ""}
		}
		content := conversation.ReadPlanContent(plansDir, planSlug)
		return planMsg{content: content}
	}
}

// loadDiagrams returns a command that lists all diagrams stored for the
// currently selected agent's session. The dashboard is a pure reader of
// the on-disk layout written by the mermaid-extractor hook.
func (m model) loadDiagrams() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil || agent.SessionID == "" {
		return nil
	}
	sessionID := agent.SessionID
	stateDir := m.cfg.Profile.StateDir
	return func() tea.Msg {
		list, _ := diagrams.Load(stateDir, sessionID)
		return diagramsLoadedMsg{sessionID: sessionID, list: list}
	}
}

// openDiagram emits a temp HTML file for the given diagram and asks the OS
// to open it in the default browser without stealing focus from the terminal
// (the `-g` flag on macOS `open` keeps the dashboard foregrounded; on other
// platforms it's silently ignored by the runner because we use `xdg-open`).
func (m model) openDiagram(d diagrams.Diagram) tea.Cmd {
	return func() tea.Msg {
		path, err := diagrams.WriteTempHTML(d)
		if err != nil {
			return diagramOpenedMsg{err: err}
		}
		if err := gitRunner.Start("open", "-g", path); err != nil {
			return diagramOpenedMsg{err: err}
		}
		return diagramOpenedMsg{}
	}
}

// deleteDiagram removes a diagram file and reloads the session list.
func (m model) deleteDiagram(d diagrams.Diagram) tea.Cmd {
	sessionID := d.SessionID
	stateDir := m.cfg.Profile.StateDir
	return func() tea.Msg {
		_ = diagrams.Delete(d)
		list, _ := diagrams.Load(stateDir, sessionID)
		return diagramsLoadedMsg{sessionID: sessionID, list: list}
	}
}

func openEditor(editor, dir string) tea.Cmd {
	return func() tea.Msg {
		return openEditorMsg{err: gitRunner.Start(editor, dir)}
	}
}

func openWorktreeWindowCmd(session, branch, dir string) tea.Cmd {
	return func() tea.Msg {
		windowName := "shell"
		if branch != "" {
			windowName = repowin.SanitizeWindowName(branch)
		}
		_, err := tmux.TmuxNewWindow(session, windowName, dir)
		return openWorktreeMsg{err: err, dir: dir}
	}
}

func gitRemoteURL(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := gitRunner.Output(ctx, "git", "-C", dir, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseGitHubRepo(remoteURL string) (owner, repo string, ok bool) {
	remoteURL = strings.TrimSpace(remoteURL)

	// SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path := strings.TrimPrefix(remoteURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}

	// HTTPS: https://github.com/owner/repo.git
	if strings.HasPrefix(remoteURL, "https://github.com/") {
		path := strings.TrimPrefix(remoteURL, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}

	return "", "", false
}

func gitDefaultBranch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := gitRunner.Output(ctx, "git", "-C", dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		// refs/remotes/origin/main -> main
		if parts := strings.Split(ref, "/"); len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return "main"
}

func buildPRURL(owner, repo, base, branch string) string {
	return fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s?expand=1",
		url.PathEscape(owner),
		url.PathEscape(repo),
		url.PathEscape(base),
		url.PathEscape(branch),
	)
}

// ghIsAvailable checks if the gh CLI is installed and authenticated.
// Runs `gh auth status` with a 3-second timeout to verify both binary
// existence and valid authentication (token not expired).
func ghIsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return gitRunner.SilentRun(ctx, "gh", "auth", "status") == nil
}

// checkGHAvailable returns a command that asynchronously checks gh auth status
// and sends a ghAvailableMsg. This avoids blocking the TUI at startup.
func checkGHAvailable() tea.Cmd {
	return func() tea.Msg {
		return ghAvailableMsg{available: ghIsAvailable()}
	}
}

// ghExistingPRURL uses `gh pr view` to check if a PR already exists for the
// given branch.  Returns the PR URL if one exists, or "" if not (or if gh is
// not installed).
func ghExistingPRURL(dir, branch string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := gitRunner.CombinedOutputDir(ctx, dir, "gh", "pr", "view", branch,
		"--json", "url", "-q", ".url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolvePRURL returns the URL to open for a branch: the existing PR's files
// page if a PR already exists, or the compare/create page otherwise.
func resolvePRURL(owner, repo, base, branch, existingPRURL string) string {
	if existingPRURL != "" {
		return strings.TrimRight(existingPRURL, "/") + "/files"
	}
	return buildPRURL(owner, repo, base, branch)
}

func openPR(dir, branch string) tea.Cmd {
	return func() tea.Msg {
		base := gitDefaultBranch(dir)
		if branch == base {
			return openPRMsg{err: fmt.Errorf("cannot create PR from %s branch", branch)}
		}

		remoteURL, err := gitRemoteURL(dir)
		if err != nil {
			return openPRMsg{err: fmt.Errorf("failed to get remote: %w", err)}
		}

		owner, repo, ok := parseGitHubRepo(remoteURL)
		if !ok {
			return openPRMsg{err: fmt.Errorf("not a GitHub remote: %s", remoteURL)}
		}

		existing := ghExistingPRURL(dir, branch)
		prURL := resolvePRURL(owner, repo, base, branch, existing)
		parsed, err := url.Parse(prURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" {
			return openPRMsg{err: fmt.Errorf("refusing to open unexpected URL: %s", prURL)}
		}
		return openPRMsg{err: gitRunner.Start("open", prURL), hasPR: existing != ""}
	}
}

// mergePR runs `gh pr merge <branch> --squash` in the given directory.
// If the authenticated user is a CODEOWNERS entry, --admin is appended
// to bypass branch protection rules.
// Branch deletion is left to the cleanup step.
func mergePR(dir, branch string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := gh.MergeArgs(gitRunner, dir, branch)
		out, err := gitRunner.CombinedOutputDir(ctx, dir, "gh", args...)
		if err != nil {
			detail := strings.TrimSpace(string(out))
			if detail != "" {
				return mergePRMsg{err: fmt.Errorf("gh pr merge: %s: %w", detail, err)}
			}
			return mergePRMsg{err: fmt.Errorf("gh pr merge: %w", err)}
		}
		return mergePRMsg{}
	}
}

// postMergeCleanup runs deterministic cleanup after a PR is merged.
//
// Architecture: the function takes the full domain.Agent and resolves the git
// topology (worktree root, source repo root) at function entry. Every git
// operation runs against the *correct* directory for its semantics:
//
//   - `worktree remove` (only when in a linked worktree): runs from Source.
//   - `checkout main` / `pull origin main`: runs from Source.
//   - `branch -d`: runs from Source.
//
// Hardening:
//
//   - Refuses cleanly when the agent worked inside a git submodule.
//   - Pre-flights that the agent's branch actually exists in Source (guards
//     against stale WorktreeCwd hints that would otherwise destroy the wrong
//     repo).
//   - Skips checkout/pull when Source has uncommitted changes (non-fatal).
func postMergeCleanup(agent domain.Agent, stateDir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Resolve topology once. Cwd is primary; WorktreeCwd is a bootstrap
		// fallback for agents whose Cwd has since been removed.
		top, err := resolveAgentTopology(ctx, agent)
		if err != nil {
			if errors.Is(err, repo.ErrInsideSubmodule) {
				return postMergeCleanupMsg{
					err:      errors.New("cleanup refused: agent worked inside a git submodule"),
					progress: "validate",
				}
			}
			return postMergeCleanupMsg{
				err:      fmt.Errorf("resolve topology: %w", err),
				progress: "validate",
			}
		}

		// Branch-existence pre-flight: stale state pointing into the wrong
		// repo would otherwise pass topology resolution silently and cause
		// destructive damage.
		if agent.Branch != "" {
			_, vErr := gitRunner.Output(ctx, "git", "-C", top.Source, "rev-parse",
				"--verify", "refs/heads/"+agent.Branch)
			if vErr != nil {
				return postMergeCleanupMsg{
					err: fmt.Errorf("branch %q not found in %s — agent state may be stale",
						agent.Branch, top.Source),
					progress: "validate",
				}
			}
		}

		defaultBranch := gitDefaultBranch(top.Source)

		// 1. Kill tmux pane (ignore already-dead).
		if target := tmux.ResolveTarget(agent.TmuxPaneID); target != "" {
			_ = tmux.TmuxKillPane(target)
		}

		// 2. Worktree remove only when the agent ran in a linked worktree.
		// When the agent ran directly in the source repo (Linked == false),
		// `git worktree remove` would refuse to remove the main working tree.
		if top.Linked {
			out, wtErr := gitRunner.CombinedOutputDir(ctx, "", "git", "-C", top.Source,
				"worktree", "remove", "--force", top.Worktree)
			if wtErr != nil {
				// Fallback: directory-level removal if git refused (e.g. dirty worktree).
				if rmErr := os.RemoveAll(top.Worktree); rmErr != nil {
					detail := strings.TrimSpace(string(out))
					return postMergeCleanupMsg{
						err: fmt.Errorf("%s: %w", detail, wtErr), progress: "worktree remove"}
				}
			}
			out, pErr := gitRunner.CombinedOutputDir(ctx, "", "git", "-C", top.Source,
				"worktree", "prune")
			if pErr != nil {
				detail := strings.TrimSpace(string(out))
				return postMergeCleanupMsg{
					err: fmt.Errorf("%s: %w", detail, pErr), progress: "worktree prune"}
			}
		}

		// 3. Source-repo branch refresh, gated on a clean tree. A dirty source
		// is a user-state concern, not a cleanup failure — surface as no-op.
		dirtyErr := gitRunner.RunDir(ctx, "", "git", "-C", top.Source, "diff-index",
			"--quiet", "HEAD")
		sourceClean := dirtyErr == nil
		if sourceClean {
			out, coErr := gitRunner.CombinedOutputDir(ctx, "", "git", "-C", top.Source,
				"checkout", defaultBranch)
			if coErr != nil {
				// "is already used by worktree" is a defensive net — should not fire
				// in the new design but tolerated rather than failing cleanup.
				detail := strings.TrimSpace(string(out))
				if !strings.Contains(detail, "is already used by worktree") {
					return postMergeCleanupMsg{
						err:      fmt.Errorf("%s: %w", detail, coErr),
						progress: "checkout " + defaultBranch,
					}
				}
			}
			out, plErr := gitRunner.CombinedOutputDir(ctx, "", "git", "-C", top.Source,
				"pull", "origin", defaultBranch)
			if plErr != nil {
				detail := strings.TrimSpace(string(out))
				return postMergeCleanupMsg{
					err:      fmt.Errorf("%s: %w", detail, plErr),
					progress: "pull origin " + defaultBranch,
				}
			}
		}

		// 4. Delete local branch (ignore errors — GitHub may have already deleted it).
		_ = gitRunner.RunDir(ctx, "", "git", "-C", top.Source, "branch", "-d", agent.Branch)

		// 5. Remove agent state file.
		_ = state.RemoveAgent(stateDir, agent.SessionID)

		return postMergeCleanupMsg{}
	}
}

func sendRawKey(paneID, key, label string) tea.Cmd {
	return func() tea.Msg {
		target := tmux.ResolveTarget(paneID)
		if target == "" {
			return rawKeySentMsg{err: fmt.Errorf("pane %s no longer exists", paneID), label: label}
		}
		return rawKeySentMsg{err: tmux.TmuxSendRaw(target, key), label: label}
	}
}

func WatchStateDir(dir string, p *tea.Program, tmuxReady *atomic.Bool) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watchDir := state.AgentsDir(dir)
	// Ensure the agents directory exists before watching
	_ = os.MkdirAll(watchDir, 0700)
	if err := watcher.Add(watchDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("cannot watch %s: %w", watchDir, err)
	}

	go func() {
		var debounce *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					if debounce != nil {
						debounce.Stop()
					}
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					// Debounce rapid writes from concurrent hooks to read
					// the settled state rather than intermediate values.
					if debounce != nil {
						debounce.Stop()
					}
					debounce = time.AfterFunc(50*time.Millisecond, func() {
						defer func() {
							if r := recover(); r != nil {
								fmt.Fprintf(os.Stderr, "panic in watcher callback: %v\n", r)
							}
						}()
						sf := state.ReadState(dir)
						if tmuxReady.Load() {
							state.ResolveAgentTargets(&sf, tmux.TmuxListPaneTargets())
						}
						var pc map[string]string
						if tmuxReady.Load() {
							pc = tmux.TmuxListPaneCwds()
						}
						state.ResolveAgentBranches(&sf, pc)
						state.ApplyPinnedStates(&sf)
						p.Send(stateUpdatedMsg{state: sf})
					})
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return watcher, nil
}

// startOfWeek returns Monday 00:00 of the week containing t.
func startOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday → 7
	}
	mon := t.AddDate(0, 0, -(weekday - 1))
	return time.Date(mon.Year(), mon.Month(), mon.Day(), 0, 0, 0, 0, t.Location())
}
