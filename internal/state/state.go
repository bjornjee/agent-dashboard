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
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
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

// ResolveAgentBranches overwrites each agent's Branch with the live value
// from git using a hierarchical resolution strategy:
//  1. WorktreeCwd — the agent may be operating in a worktree
//  2. Cwd — the launch directory from the state file
//  3. paneCwds — live tmux pane working directory (fallback when hooks omit cwd)
//
// When an agent has no Cwd but a tmux pane cwd is available, Cwd is
// backfilled so that agentLabel() and the detail header can display it.
// Agents where all sources fail are left unchanged.
func ResolveAgentBranches(sf *domain.StateFile, paneCwds map[string]string) {
	for key, agent := range sf.Agents {
		// Backfill Cwd from tmux pane when the state file lacks it
		if agent.Cwd == "" && agent.TmuxPaneID != "" && paneCwds != nil {
			if pc, ok := paneCwds[agent.TmuxPaneID]; ok {
				agent.Cwd = pc
			}
		}

		var branch string
		if agent.WorktreeCwd != "" {
			branch = gitBranch(agent.WorktreeCwd)
		}
		if branch == "" && agent.Cwd != "" {
			branch = gitBranch(agent.Cwd)
		}
		if branch == "" {
			continue
		}
		agent.Branch = branch
		sf.Agents[key] = agent
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
func ApplyIdleOverrides(sf *domain.StateFile, projectsDir string) {
	for key, agent := range sf.Agents {
		if agent.State != "idle_prompt" && agent.State != "permission" {
			continue
		}
		cwd := agent.Cwd
		if cwd == "" {
			continue
		}
		projDir := filepath.Join(projectsDir, conversation.ProjectSlug(cwd))
		sessionID := agent.SessionID
		if sessionID == "" {
			continue
		}
		if override := conversation.LastPendingBlockingTool(projDir, sessionID); override != "" {
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
