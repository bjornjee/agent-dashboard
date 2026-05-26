package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/repo"
)

// BranchRunner abstracts git command execution so tests can swap in a mock.
type BranchRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execBranchRunner struct{}

func (r *execBranchRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// branchRunner is the package-level runner used by gitBranch.
// Tests replace this with a mock.
var branchRunner BranchRunner = &execBranchRunner{}

// SetTestRunner swaps the package-level branchRunner and returns a restore function.
// This allows test packages outside of state to inject a mock runner.
func SetTestRunner(r BranchRunner) func() {
	orig := branchRunner
	branchRunner = r
	return func() { branchRunner = orig }
}

// AgentsDir returns the agents subdirectory within the state directory.
func AgentsDir(dir string) string {
	return filepath.Join(dir, "agents")
}

// ReadState reads all per-agent JSON files from dir/agents/*.json.
// Agents are keyed by session_id (the filename stem). Returns empty state on error.
func ReadState(dir string) domain.StateFile {
	sf := domain.StateFile{Agents: make(map[string]domain.Agent)}

	entries, err := os.ReadDir(AgentsDir(dir))
	if err != nil {
		return sf
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(AgentsDir(dir), entry.Name()))
		if err != nil {
			continue
		}
		var agent domain.Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		// Use session_id as the key; fall back to filename stem
		key := agent.SessionID
		if key == "" {
			key = strings.TrimSuffix(entry.Name(), ".json")
		}
		if key != "" {
			sf.Agents[key] = agent
		}
	}

	return sf
}

// agentFileMap returns a map of session_id -> file path for all agent files.
func agentFileMap(dir string) map[string]string {
	m := make(map[string]string)
	entries, err := os.ReadDir(AgentsDir(dir))
	if err != nil {
		return m
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(AgentsDir(dir), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent domain.Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		key := agent.SessionID
		if key == "" {
			key = strings.TrimSuffix(entry.Name(), ".json")
		}
		if key != "" {
			m[key] = path
		}
	}
	return m
}

// ResolveAgentTargets updates each agent's Target, Session, Window, and Pane
// to match the live tmux state. paneTargets maps pane IDs (%N) to their current
// coordinates. Agents without a TmuxPaneID or without a match are left unchanged.
func ResolveAgentTargets(sf *domain.StateFile, paneTargets map[string]domain.PaneTarget) {
	if paneTargets == nil {
		return
	}
	for key, agent := range sf.Agents {
		if agent.TmuxPaneID == "" {
			continue
		}
		pt, ok := paneTargets[agent.TmuxPaneID]
		if !ok {
			continue
		}
		agent.Target = pt.Target
		agent.Session = pt.Session
		agent.Window = pt.Window
		agent.Pane = pt.Pane
		sf.Agents[key] = agent
	}
}

// ResolveAgentWorktree self-heals empty `agent.WorktreeCwd` by scanning the
// agent's JSONL for the most recent `git worktree add <path>` command and
// validating that the candidate path is a real worktree of the same source
// repo as the agent's cwd. On a successful match, the candidate is stamped
// in-memory and persisted to the agent's state file so subsequent loads —
// and downstream features keyed off worktree_cwd (diff, PR, cleanup) — see
// the recovered value. The branch the worktree is currently on is stamped
// in the same write, so ResolveAgentBranches can short-circuit the live
// read on the next refresh. Branch stamping is best-effort: a failed
// lookup (detached HEAD, deleted dir) leaves Branch empty for the backfill
// path. Failures of the worktree validation itself (no JSONL match,
// candidate gone, different source repo) leave WorktreeCwd empty;
// ResolveAgentBranches then clears Branch the same way it does for any
// unresolvable agent.
//
// Must run AFTER ResolveAgentProjDir (needs agent.ProjDir) and BEFORE
// ResolveAgentBranches (so the recovered WorktreeCwd is the source of truth
// for branch lookup).
func ResolveAgentWorktree(sf *domain.StateFile, stateDir string) {
	for key, agent := range sf.Agents {
		if agent.WorktreeCwd != "" {
			continue
		}
		if agent.SessionID == "" || agent.ProjDir == "" {
			continue
		}
		candidate := conversation.LastGitWorktreeAdd(agent.ProjDir, agent.SessionID)
		if candidate == "" {
			continue
		}
		if !filepath.IsAbs(candidate) && agent.Cwd != "" {
			candidate = filepath.Clean(filepath.Join(agent.Cwd, candidate))
		}
		if !validateWorktreeCandidate(agent.Cwd, candidate) {
			continue
		}
		branch := gitBranch(candidate)
		agent.WorktreeCwd = candidate
		if branch != "" {
			agent.Branch = branch
		}
		sf.Agents[key] = agent
		updates := map[string]any{"worktree_cwd": candidate}
		if branch != "" {
			updates["branch"] = branch
		}
		_ = stampAgentFields(stateDir, agent.SessionID, updates)
	}
}

// validateWorktreeCandidate returns true when candidate is a real worktree
// whose source repo matches the agent's source repo.
func validateWorktreeCandidate(agentCwd, candidate string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := branchRunner.Output(ctx, "git", "-C", candidate, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	top := strings.TrimSpace(string(out))
	normTop, err := filepath.EvalSymlinks(top)
	if err != nil {
		normTop = top
	}
	normCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		normCandidate = candidate
	}
	if normTop != normCandidate {
		return false
	}

	agentTop, err := repo.Resolve(ctx, branchRunner, agentCwd)
	if err != nil {
		return false
	}
	candTop, err := repo.Resolve(ctx, branchRunner, candidate)
	if err != nil {
		return false
	}
	return filepath.Clean(agentTop.Source) == filepath.Clean(candTop.Source)
}

// stampAgentFields merges a set of field updates into the agent's state JSON.
// Mirrors PinAgentState: read → merge → atomic write. Callers ignore the
// error: a missing or malformed agent file just means the in-memory update
// stands until the hook writes a clean file on the next event.
func stampAgentFields(stateDir, sessionID string, updates map[string]any) error {
	files := agentFileMap(stateDir)
	path, ok := files[sessionID]
	if !ok {
		return fmt.Errorf("agent %s not found", sessionID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range updates {
		raw[k] = v
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
}

// ResolveAgentBranches sets each agent's Branch field, with branch-pinning
// semantics tied to whether the agent has a WorktreeCwd:
//
//  1. WorktreeCwd != "" && Branch != "": PINNED. The stored branch is
//     authoritative — no git call, no overwrite. This is the steady state
//     for agents created via a worktree-aware skill (or recovered by
//     ResolveAgentWorktree). The pin survives the agent's own `git checkout
//     main` and sibling-agent cleanups that switch the source repo.
//
//  2. WorktreeCwd != "" && Branch == "": ONE-SHOT BACKFILL. Legacy state
//     files (or agents whose stamp landed without a branch signal) get one
//     live read against WorktreeCwd. The result is written back to disk
//     when stateDir is non-empty, so the next refresh skips the git call.
//
//  3. WorktreeCwd == "" && Cwd != "": LIVE. Vanilla source-repo agents
//     have no pin to anchor to — Branch reflects whatever the source
//     repo's HEAD is on each refresh. Pre-existing behavior; documented
//     in the PR as the unfixable drift case for unpinned agents.
//
//  4. both empty: Branch cleared to "".
//
// Pane cwds backfill empty agent.Cwd before dir selection (display only —
// never used to resolve branches, since the agent's project dir is
// intentionally static).
//
// stateDir is optional: when "", backfill happens in memory only.
func ResolveAgentBranches(sf *domain.StateFile, paneCwds map[string]string, stateDir string) {
	// First pass: backfill Cwd from tmux pane cwd (display-only).
	for key, agent := range sf.Agents {
		if agent.Cwd == "" && agent.TmuxPaneID != "" && paneCwds != nil {
			if pc, ok := paneCwds[agent.TmuxPaneID]; ok {
				agent.Cwd = pc
				sf.Agents[key] = agent
			}
		}
	}

	// Plan the git lookups. Pinned agents (case 1) contribute nothing.
	// Backfill candidates (case 2) carry persist=true; live-only agents
	// (case 3) carry persist=false.
	type lookup struct {
		dir     string
		persist bool
	}
	lookups := make(map[string]lookup, len(sf.Agents))
	for key, agent := range sf.Agents {
		if agent.WorktreeCwd != "" {
			if agent.Branch != "" {
				continue // pinned
			}
			lookups[key] = lookup{dir: agent.WorktreeCwd, persist: true}
			continue
		}
		if agent.Cwd != "" {
			lookups[key] = lookup{dir: agent.Cwd, persist: false}
		}
	}

	// gitBranch runs `git rev-parse` with a 500ms timeout per agent. With
	// many agents that sequential cost stacks; cap concurrency to 8 so a
	// dashboard refresh costs ceil(N/8) × 500ms in the worst case.
	type result struct {
		branch  string
		persist bool
	}
	results := make(map[string]result, len(lookups))
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for key, lk := range lookups {
		wg.Add(1)
		go func(key string, lk lookup) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			branch := gitBranch(lk.dir)
			mu.Lock()
			results[key] = result{branch: branch, persist: lk.persist}
			mu.Unlock()
		}(key, lk)
	}
	wg.Wait()

	for key, agent := range sf.Agents {
		// Pinned agents: skip — Branch already authoritative.
		if agent.WorktreeCwd != "" && agent.Branch != "" {
			continue
		}
		r := results[key] // zero value "" when no lookup or git failed
		agent.Branch = r.branch
		sf.Agents[key] = agent
		// Persist the backfilled branch so the next refresh hits case 1.
		// Skip the write when the branch is empty (nothing useful to
		// pin) or when no stateDir was provided.
		if r.persist && r.branch != "" && stateDir != "" && agent.SessionID != "" {
			_ = stampAgentFields(stateDir, agent.SessionID, map[string]any{"branch": r.branch})
		}
	}
}

// gitBranch returns the current branch name for a directory, or "" on error.
func gitBranch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := branchRunner.Output(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ResolveAgentProjDir stamps each agent's ProjDir — the absolute path of
// the ~/.claude/projects/<slug> directory containing <SessionID>.jsonl —
// using `repo.Resolve` to get topology candidates, then asking the
// conversation layer which slug actually contains the JSONL.
//
// Single source of truth for the ProjectSlug → JSONL translation.
// Consumers downstream read `agent.ProjDir` directly.
//
// Also backfills `agent.SessionID` via `conversation.Locate` when the
// hook hasn't yet stamped one — preserving d4803f4's empty-SessionID
// recovery behaviour without requiring callers to know about it.
func ResolveAgentProjDir(sf *domain.StateFile, projectsDir, sessionsDir string) {
	for key, agent := range sf.Agents {
		if agent.SessionID == "" {
			agent.SessionID = conversation.Locate(sessionsDir, agent.Cwd, agent.WorktreeCwd)
		}
		if agent.SessionID == "" {
			sf.Agents[key] = agent
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		top, _ := repo.Resolve(ctx, branchRunner, agent.Cwd, agent.WorktreeCwd)
		cancel()
		agent.ProjDir = conversation.PickProjDir(projectsDir, agent.SessionID,
			agent.Cwd, agent.WorktreeCwd, top.Worktree, top.Source)
		if agent.ProjDir == "" {
			// No candidate matched (typical when the agent's worktree has been
			// deleted, so repo.Resolve returns empty topology). Fall back to a
			// scan of projectsDir as last resort — the JSONL is still there.
			agent.ProjDir = conversation.FindProjDirByScan(projectsDir, agent.SessionID)
		}
		sf.Agents[key] = agent
	}
}

// idleStates are the agent states where a pinned_state override may be applied.
// Active states (running, permission, error, plan) pass through unchanged so
// the dashboard reflects live work.
var idleStates = map[string]bool{
	"idle_prompt": true, "done": true, "question": true,
}

// ApplyPinnedStates restores each agent's State to its PinnedState, but only
// when the agent is idle. Active states (running, permission, error, plan)
// pass through so the dashboard reflects live work.
func ApplyPinnedStates(sf *domain.StateFile) {
	for key, agent := range sf.Agents {
		if agent.PinnedState != "" && idleStates[agent.State] {
			agent.State = agent.PinnedState
			sf.Agents[key] = agent
		}
	}
}

// ApplyIdleOverrides checks each idle_prompt or permission agent for a pending
// plan review (ExitPlanMode) or pending question (AskUserQuestion) and
// overrides the state accordingly. The most recently seen blocking tool wins —
// if AskUserQuestion appears after ExitPlanMode, "question" takes precedence.
// Permission agents are included because the plan selection menu is
// classified as "permission" by hooks. Running agents are skipped — the
// PostToolUse hook's stop-state guard prevents the race that used to leave
// agents stuck at "running" when actually idle.
func ApplyIdleOverrides(sf *domain.StateFile) {
	for key, agent := range sf.Agents {
		if agent.State != "idle_prompt" && agent.State != "permission" {
			continue
		}
		if agent.ProjDir == "" || agent.SessionID == "" {
			continue
		}
		if override := conversation.LastPendingBlockingTool(agent.ProjDir, agent.SessionID); override != "" {
			agent.State = override
			sf.Agents[key] = agent
		}
	}
}

// PinAgentState writes a pinned_state field to the agent's JSON file.
// Hook updates merge into the file and preserve this field, so the
// dashboard-driven state survives while the agent continues working.
func PinAgentState(dir, sessionID, pinnedState string) error {
	files := agentFileMap(dir)
	path, ok := files[sessionID]
	if !ok {
		return fmt.Errorf("agent %s not found", sessionID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	raw["pinned_state"] = pinnedState
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
}

// SortedAgents returns agents sorted by state priority, then by updated_at.
// selfPaneID is excluded from the list (the dashboard's own pane, e.g. "%5").
func SortedAgents(sf domain.StateFile, selfPaneID string) []domain.Agent {
	agents := make([]domain.Agent, 0, len(sf.Agents))
	for _, a := range sf.Agents {
		if a.State == "" {
			continue
		}
		if selfPaneID != "" && a.TmuxPaneID == selfPaneID {
			continue
		}
		agents = append(agents, a)
	}

	sort.Slice(agents, func(i, j int) bool {
		pi := domain.StatePriority[agents[i].State]
		pj := domain.StatePriority[agents[j].State]
		if pi == 0 {
			pi = 99
		}
		if pj == 0 {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		// Stable sort by window, then pane within same priority group
		if agents[i].Window != agents[j].Window {
			return agents[i].Window < agents[j].Window
		}
		return agents[i].Pane < agents[j].Pane
	})

	return agents
}

// CleanStale removes agent files that haven't been updated within maxAgeSecs.
// Agents whose tmux panes are still alive (present in livePaneIDs) are kept
// regardless of age — an idle agent waiting for input generates no hook events
// but should not be evicted from the dashboard.
//
// When multiple agents share the same pane ID (e.g. a pane was reused for a
// different process), only the most recently updated agent is kept; older
// duplicates are removed regardless of pane liveness.
func CleanStale(dir string, maxAgeSecs int, livePaneIDs map[string]bool) {
	now := time.Now()
	entries, err := os.ReadDir(AgentsDir(dir))
	if err != nil {
		return
	}

	type agentFile struct {
		path      string
		agent     domain.Agent
		updatedAt time.Time
	}

	// First pass: read all agent files and track the newest per pane.
	var files []agentFile
	newestPerPane := make(map[string]time.Time) // paneID -> newest updatedAt

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(AgentsDir(dir), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent domain.Agent
		if err := json.Unmarshal(data, &agent); err != nil || agent.UpdatedAt == "" {
			_ = os.Remove(path)
			continue
		}
		t, err := time.Parse(time.RFC3339, agent.UpdatedAt)
		if err != nil {
			_ = os.Remove(path)
			continue
		}
		files = append(files, agentFile{path: path, agent: agent, updatedAt: t})
		if agent.TmuxPaneID != "" && t.After(newestPerPane[agent.TmuxPaneID]) {
			newestPerPane[agent.TmuxPaneID] = t
		}
	}

	// Second pass: remove stale and duplicate-pane agents.
	for _, f := range files {
		// When multiple agents share a pane, remove all but the newest.
		if f.agent.TmuxPaneID != "" && f.updatedAt.Before(newestPerPane[f.agent.TmuxPaneID]) {
			_ = os.Remove(f.path)
			continue
		}
		// Keep agents whose tmux pane is still alive
		if f.agent.TmuxPaneID != "" && livePaneIDs[f.agent.TmuxPaneID] {
			continue
		}
		if now.Sub(f.updatedAt).Seconds() > float64(maxAgeSecs) {
			_ = os.Remove(f.path)
		}
	}
}

// PruneDead removes agent files whose tmux panes no longer exist and
// deduplicates agents sharing the same live pane (keeps only the newest).
// livePaneIDs is the set of currently live tmux pane IDs (%N format).
// Returns the number of agents removed.
func PruneDead(dir string, livePaneIDs map[string]bool) int {
	entries, err := os.ReadDir(AgentsDir(dir))
	if err != nil {
		return 0
	}

	type agentFile struct {
		path      string
		agent     domain.Agent
		updatedAt time.Time
	}

	var files []agentFile
	newestPerPane := make(map[string]time.Time) // paneID -> newest updatedAt

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(AgentsDir(dir), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var agent domain.Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			continue
		}
		var t time.Time
		if agent.UpdatedAt != "" {
			t, _ = time.Parse(time.RFC3339, agent.UpdatedAt)
		}
		files = append(files, agentFile{path: path, agent: agent, updatedAt: t})
		if agent.TmuxPaneID != "" && t.After(newestPerPane[agent.TmuxPaneID]) {
			newestPerPane[agent.TmuxPaneID] = t
		}
	}

	// Classify removals into two buckets: dedup (always safe) and dead-pane
	// (subject to the safety net). Keeping them separate ensures the safety
	// net only fires when every agent appears dead — not when dedup removals
	// inflate the count.
	var dedupPaths, deadPaths []string
	for _, f := range files {
		// Dedup: when multiple agents share a pane, remove all but the newest.
		if f.agent.TmuxPaneID != "" && f.updatedAt.Before(newestPerPane[f.agent.TmuxPaneID]) {
			dedupPaths = append(dedupPaths, f.path)
			continue
		}
		// Dead: pane no longer exists
		if f.agent.TmuxPaneID == "" || !livePaneIDs[f.agent.TmuxPaneID] {
			deadPaths = append(deadPaths, f.path)
		}
	}

	// Safety net: refuse to wipe all agents at once — almost certainly
	// a transient tmux issue. CleanStale handles truly dead agents.
	// Only dead-pane removals are gated; dedup removals are always applied.
	applyDead := true
	if len(deadPaths)+len(dedupPaths) == len(files) && len(files) > 0 && len(deadPaths) > 0 {
		applyDead = false
	}

	removed := 0
	for _, path := range dedupPaths {
		if os.Remove(path) == nil {
			removed++
		}
	}
	if applyDead {
		for _, path := range deadPaths {
			if os.Remove(path) == nil {
				removed++
			}
		}
	}
	return removed
}

// RemoveAgent removes an agent's file by session_id.
func RemoveAgent(dir, sessionID string) error {
	files := agentFileMap(dir)
	path, ok := files[sessionID]
	if !ok {
		return nil // not found, nothing to do
	}
	return os.Remove(path)
}

// FormatDuration returns a human-readable duration since the given ISO8601 timestamp.
func FormatDuration(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return ""
	}

	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, secs%60)
	}
	hours := mins / 60
	return fmt.Sprintf("%dh %dm", hours, mins%60)
}
