package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

// -- Deferred startup commands --

// deferredStartup runs all blocking startup work (tmux probes, stale cleanup)
// concurrently via errgroup so the TUI can render immediately.
func deferredStartup(stateDir string, db *DB, cfg Config) tea.Cmd {
	return func() tea.Msg {
		var (
			tmuxAvail   bool
			selfPane    string
			livePaneIDs map[string]bool
		)

		g := new(errgroup.Group)

		g.Go(func() error {
			tmuxAvail = TmuxIsAvailable()
			return nil
		})

		g.Go(func() error {
			selfPane = TmuxResolvePaneID()
			return nil
		})

		g.Go(func() error {
			livePaneIDs = TmuxListLivePaneIDs()
			return nil
		})

		_ = g.Wait()

		CleanStale(stateDir, 10*60, livePaneIDs)

		return startupMsg{tmuxAvailable: tmuxAvail, selfPaneID: selfPane}
	}
}

// deferredQuote fetches the daily quote in the background so the banner
// renders instantly with an empty quote, then fills in once ready.
func deferredQuote(db *DB, showQuote bool) tea.Cmd {
	if !showQuote {
		return nil
	}
	return func() tea.Msg {
		q, a := pickQuote(db)
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
		target := ResolveTarget(paneID)
		if target == "" {
			return captureResultMsg{lines: nil}
		}
		lines, err := TmuxCapture(target, 20)
		if err != nil {
			return captureResultMsg{lines: nil}
		}
		return captureResultMsg{lines: trimTrailingBlankLines(lines)}
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
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd
	sessionsDir := m.cfg.Profile.SessionsDir

	// Capture offset state for incremental reading
	sessionKey := projDir + ":" + sessionID
	prevOffset := m.convFileOffset
	var prevEntries []ConversationEntry
	if m.convSessionKey == sessionKey {
		prevEntries = m.conversation
	} else {
		prevOffset = 0 // different session — full re-read
	}

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = findSessionIDIn(sessionsDir, cwd)
		}
		if sessionID == "" {
			return conversationMsg{entries: nil}
		}
		entries, newOffset := ReadConversationIncremental(projDir, sessionID, 50, prevEntries, prevOffset)
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
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd
	agentID := sub.AgentID
	sessionsDir := m.cfg.Profile.SessionsDir

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = findSessionIDIn(sessionsDir, cwd)
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
				sid = findSessionIDIn(sessionsDir, a.Cwd)
			}
			if sid == "" {
				return subagentsMsg{parentTarget: a.Target, agents: nil}
			}
			slug := ProjectSlug(a.Cwd)
			projDir := filepath.Join(projectsDir, slug)
			subs := FindSubagents(projDir, sid)
			return subagentsMsg{parentTarget: a.Target, agents: subs}
		})
	}
	return cmds
}

func pruneDead(statePath string) tea.Cmd {
	return func() tea.Msg {
		livePaneIDs := TmuxListLivePaneIDs()
		if len(livePaneIDs) == 0 {
			return pruneDeadMsg{removed: 0}
		}
		removed := PruneDead(statePath, livePaneIDs)
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

// closePane kills a tmux pane and removes its agent state file.
// paneID is the immutable tmux pane ID (%N), sessionID is the agent's session_id.
func closePane(paneID, sessionID, stateDir string) tea.Cmd {
	return func() tea.Msg {
		// Resolve current target from pane ID for the kill command
		target := ResolveTarget(paneID)
		if target == "" {
			// Pane already dead — just clean up the file
			_ = RemoveAgent(stateDir, sessionID)
			return closeResultMsg{err: nil}
		}
		err := TmuxKillPane(target)
		if err != nil {
			return closeResultMsg{err: err}
		}
		_ = RemoveAgent(stateDir, sessionID)
		return closeResultMsg{err: nil}
	}
}

// pinAgentStateCmd writes a pinned_state to the agent's JSON file.
func pinAgentStateCmd(stateDir, sessionID, pinnedState string) tea.Cmd {
	return func() tea.Msg {
		err := PinAgentState(stateDir, sessionID, pinnedState)
		return pinStateMsg{err: err}
	}
}

func loadUsage(agents []Agent, projectsDir, sessionsDir string) tea.Cmd {
	agentsCopy := make([]Agent, len(agents))
	copy(agentsCopy, agents)
	return func() tea.Msg {
		perAgent, total := ReadAllUsage(agentsCopy, projectsDir, sessionsDir)
		return usageMsg{perAgent: perAgent, total: total}
	}
}

func loadState(path string, tmuxAvailable bool) tea.Cmd {
	return func() tea.Msg {
		sf := ReadState(path)
		var paneCwds map[string]string
		if tmuxAvailable {
			ResolveAgentTargets(&sf, TmuxListPaneTargets())
			paneCwds = TmuxListPaneCwds()
		}
		ResolveAgentBranches(&sf, paneCwds)
		ApplyPinnedStates(&sf)
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
		target := ResolveTarget(paneID)
		if target == "" {
			return jumpResultMsg{err: fmt.Errorf("pane %s no longer exists", paneID)}
		}
		return jumpResultMsg{err: TmuxJump(target)}
	}
}

func selectPane(target string) tea.Cmd {
	return func() tea.Msg {
		return selectPaneMsg{err: TmuxSelectPane(target)}
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
		target := ResolveTarget(paneID)
		if target == "" {
			return sendResultMsg{err: fmt.Errorf("pane %s no longer exists", paneID)}
		}
		return sendResultMsg{err: TmuxSendKeys(target, text)}
	}
}

// findWindowForRepo finds an existing tmux session:window for a given folder
// by scanning existing agents' working directories.
// It first tries an exact path match, then falls back to repo-name matching
// only when at least one side is a worktree path (to avoid false collisions
// between unrelated repos that share the same basename).
// selfPaneID is the dashboard's own pane ID (%N) to exclude.
func findWindowForRepo(agents []Agent, folder, selfPaneID string) (string, bool) {
	// Pass 1: exact path match
	for _, agent := range agents {
		if agent.TmuxPaneID == selfPaneID {
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
		if agent.TmuxPaneID == selfPaneID {
			continue
		}
		agentIsWorktree := strings.Contains(agent.Cwd, "/worktrees/") || agent.WorktreeCwd != ""
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
// excluding the window identified by dashboardSW (e.g. "main:2").
func findWindowByName(windows []TmuxWindowInfo, repoName, session, dashboardSW string) (string, bool) {
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

const maxPanesPerWindow = 8

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
func createSession(folder string, agents []Agent, selfPaneID string, profile AgentProfile) tea.Cmd {
	return createSessionWithPrompt(folder, agents, selfPaneID, profile, "", "")
}

// createSessionWithPrompt creates a new agent session with an optional skill and message.
func createSessionWithPrompt(folder string, agents []Agent, selfPaneID string, profile AgentProfile, skill, message string) tea.Cmd {
	return func() tea.Msg {
		absFolder, err := validateFolder(folder)
		if err != nil {
			return createSessionMsg{err: err}
		}

		selfTarget := ResolveTarget(selfPaneID)
		session := extractSession(selfTarget)
		dashboardSW := extractSessionWindow(selfTarget)
		repoName := sanitizeWindowName(repoFromCwd(absFolder))
		if repoName == "" {
			repoName = profile.Command
		}

		var newTarget string

		// Check for existing window
		sw, found := findWindowForRepo(agents, absFolder, selfPaneID)
		if !found {
			// Fallback: check window names
			windows, wErr := TmuxListWindows(session)
			if wErr == nil {
				sw, found = findWindowByName(windows, repoName, session, dashboardSW)
			}
		}

		if found {
			// Check pane limit; if the window no longer exists (stale agent
			// state), fall through and create a fresh window instead.
			count, cErr := TmuxCountPanes(sw)
			if cErr != nil {
				found = false
			} else if count >= maxPanesPerWindow {
				return createSessionMsg{err: fmt.Errorf("8-pane limit reached for %s", repoName)}
			} else {
				newTarget, err = TmuxSplitWindow(sw, absFolder)
			}
		}
		if !found {
			newTarget, err = TmuxNewWindow(session, repoName, absFolder)
		}

		if err != nil {
			return createSessionMsg{err: err}
		}

		// Build the command with optional initial prompt.
		// The prompt is shell-quoted so metacharacters (>, |, &, etc.)
		// are passed literally to the claude CLI as a single argument.
		cmd := profile.Command
		if prompt := buildPrompt(skill, message); prompt != "" {
			cmd = profile.Command + " " + shellQuote(prompt)
		}

		if sendErr := TmuxSendKeys(newTarget, cmd); sendErr != nil {
			return createSessionMsg{err: fmt.Errorf("failed to launch %s: %w", profile.Command, sendErr)}
		}

		return createSessionMsg{target: newTarget}
	}
}

func (m model) loadPlan() tea.Cmd {
	agent := m.selectedAgent()
	if agent == nil || agent.Cwd == "" {
		return nil
	}
	slug := ProjectSlug(agent.Cwd)
	projDir := filepath.Join(m.cfg.Profile.ProjectsDir, slug)
	sessionID := agent.SessionID
	cwd := agent.Cwd
	plansDir := m.cfg.Profile.PlansDir
	sessionsDir := m.cfg.Profile.SessionsDir

	return func() tea.Msg {
		if sessionID == "" {
			sessionID = findSessionIDIn(sessionsDir, cwd)
		}
		if sessionID == "" {
			return planMsg{content: ""}
		}
		planSlug := ReadPlanSlug(projDir, sessionID)
		if planSlug == "" {
			return planMsg{content: ""}
		}
		content := ReadPlanContent(plansDir, planSlug)
		return planMsg{content: content}
	}
}

func openEditor(editor, dir string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(editor, dir)
		return openEditorMsg{err: silentStart(cmd)}
	}
}

func gitRemoteURL(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
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
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
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

// GHIsAvailable checks if the gh CLI is installed and authenticated.
// Runs `gh auth status` with a 3-second timeout to verify both binary
// existence and valid authentication (token not expired).
func GHIsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return silentRun(exec.CommandContext(ctx, "gh", "auth", "status")) == nil
}

// checkGHAvailable returns a command that asynchronously checks gh auth status
// and sends a ghAvailableMsg. This avoids blocking the TUI at startup.
func checkGHAvailable() tea.Cmd {
	return func() tea.Msg {
		return ghAvailableMsg{available: GHIsAvailable()}
	}
}

// ghExistingPRURL uses `gh pr view` to check if a PR already exists for the
// given branch.  Returns the PR URL if one exists, or "" if not (or if gh is
// not installed).
func ghExistingPRURL(dir, branch string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", branch,
		"--json", "url", "-q", ".url")
	cmd.Dir = dir
	out, err := cmd.Output()
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
		cmd := exec.Command("open", prURL)
		return openPRMsg{err: silentStart(cmd), hasPR: existing != ""}
	}
}

// mergePR runs `gh pr merge <branch> --squash --delete-branch` in the given
// directory. Returns mergePRMsg with error details on failure.
func mergePR(dir, branch string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "gh", "pr", "merge", branch, "--squash", "--delete-branch")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
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

func sendRawKey(paneID, key string) tea.Cmd {
	return func() tea.Msg {
		target := ResolveTarget(paneID)
		if target == "" {
			return rawKeySentMsg{err: fmt.Errorf("pane %s no longer exists", paneID)}
		}
		return rawKeySentMsg{err: TmuxSendRaw(target, key)}
	}
}

func watchStateDir(dir string, p *tea.Program, tmuxReady *atomic.Bool) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watchDir := agentsDir(dir)
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
						sf := ReadState(dir)
						if tmuxReady.Load() {
							ResolveAgentTargets(&sf, TmuxListPaneTargets())
						}
						var pc map[string]string
						if tmuxReady.Load() {
							pc = TmuxListPaneCwds()
						}
						ResolveAgentBranches(&sf, pc)
						ApplyPinnedStates(&sf)
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
