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

	codexconv "github.com/bjornjee/agent-dashboard/internal/codex/conversation"
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/git"
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

func normalizeAgentState(state string) string {
	switch state {
	case "waiting_input":
		return "idle_prompt"
	default:
		return state
	}
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
		agent.State = normalizeAgentState(agent.State)
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

// ResolveAgentWorktree self-heals empty `agent.WorktreeCwd` by asking git
// which worktrees exist under the agent's source repo, then matching each
// against a marker file (`<git-dir>/agent-dashboard-session`) dropped by
// the JS hook at worktree-claim time. Match = byte-equal session id; no
// regex, no command parsing.
//
// Two paths run per unpinned agent:
//
//  1. Marker scan (read-only): walk the porcelain list, pin the first
//     worktree whose marker file equals the agent's session id. Steady
//     state for any agent the JS hook has already claimed.
//
//  2. Scan-on-init fallback (claims the marker via O_CREAT|O_EXCL): only
//     when the agent's Cwd IS a linked worktree path. This narrow signal
//     is enough to disambiguate ownership — the agent is running there,
//     no sibling agent can share that exact Cwd. Without this fallback,
//     dashboards upgraded past PR #330 show empty pins for legacy agents
//     until each agent fires its next hook event. Main-worktree agents
//     remain unpinned: too many agents share that Cwd to attribute safely.
//
// Must run BEFORE ResolveAgentBranches (so the recovered WorktreeCwd is
// the source of truth for branch lookup).
func ResolveAgentWorktree(sf *domain.StateFile, stateDir string) {
	for key, agent := range sf.Agents {
		if agent.WorktreeCwd != "" || agent.SessionID == "" || agent.Cwd == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		wts, err := git.ListWorktrees(ctx, branchRunner, agent.Cwd)
		cancel()
		if err != nil {
			continue
		}

		pinned := false
		for _, wt := range wts {
			if wt.GitDir == "" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(wt.GitDir, "agent-dashboard-session"))
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(data)) != agent.SessionID {
				continue
			}
			pinAgentToWorktree(sf, key, &agent, wt, stateDir)
			pinned = true
			break
		}
		if pinned {
			continue
		}

		normCwd := canonicalPath(agent.Cwd)
		if normCwd == "" {
			continue
		}
		for _, wt := range wts {
			if wt.GitDir == "" || canonicalPath(wt.Path) != normCwd {
				continue
			}
			if !isLinkedWorktree(wt.Path) {
				break
			}
			if !claimMarker(wt.GitDir, agent.SessionID) {
				break
			}
			pinAgentToWorktree(sf, key, &agent, wt, stateDir)
			break
		}
	}
}

// pinAgentToWorktree updates the in-memory state and persists worktree_cwd /
// branch to the agent's state file. Mutates *agent and sf.Agents[key].
func pinAgentToWorktree(sf *domain.StateFile, key string, agent *domain.Agent, wt git.Worktree, stateDir string) {
	agent.WorktreeCwd = wt.Path
	agent.Branch = wt.Branch
	sf.Agents[key] = *agent
	updates := map[string]any{"worktree_cwd": wt.Path}
	if wt.Branch != "" {
		updates["branch"] = wt.Branch
	}
	_ = stampAgentFields(stateDir, agent.SessionID, updates)
}

// canonicalPath returns the absolute, symlink-resolved form of p. Falls
// back to filepath.Abs when EvalSymlinks fails (typical on paths whose
// last segment doesn't exist), and finally to p itself.
func canonicalPath(p string) string {
	if p == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// isLinkedWorktree reports whether `wtPath/.git` is a regular file — git's
// signal for a linked worktree (the file contains `gitdir: <per-wt-dir>`).
// Main worktrees have `.git` as a directory; missing or anything else
// returns false so the caller never claims on weak evidence.
func isLinkedWorktree(wtPath string) bool {
	info, err := os.Lstat(filepath.Join(wtPath, ".git"))
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// claimMarker atomically creates <gitDir>/agent-dashboard-session and writes
// sessionID, mirroring the JS hook's claim semantics so concurrent claims
// from either side resolve deterministically — first writer wins.
//
// Returns true when this call wrote the marker (including the race-recovery
// case where the marker appeared between our read and write but turns out to
// hold our own sessionID). Returns false on EEXIST with a different owner
// or any other IO failure.
func claimMarker(gitDir, sessionID string) bool {
	marker := filepath.Join(gitDir, "agent-dashboard-session")
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, writeErr := f.WriteString(sessionID)
		_ = f.Close()
		if writeErr != nil {
			// Remove the empty/partial marker so it doesn't permanently
			// shadow a future legitimate claim — an empty marker reads as
			// "owned by no one" and would block this code path forever.
			_ = os.Remove(marker)
			return false
		}
		return true
	}
	if !os.IsExist(err) {
		return false
	}
	data, readErr := os.ReadFile(marker)
	if readErr != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == sessionID
}

// stampAgentFields merges a set of field updates into the agent's state JSON.
// Read → merge → atomic write inside a sidecar file lock so concurrent
// hook subprocesses and dashboard pins don't overwrite each other's fields
// (lost-update race) or read torn content (non-atomic write race).
//
// Callers ignore the error: a missing or malformed agent file just means
// the in-memory update stands until the hook writes a clean file on the
// next event.
func stampAgentFields(stateDir, sessionID string, updates map[string]any) error {
	path := filepath.Join(AgentsDir(stateDir), sessionID+".json")
	return withFileLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("agent %s not found: %w", sessionID, err)
		}
		var raw map[string]any
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
		return writeJSONAtomic(path, out)
	})
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

// arbitrateState resolves one agent's displayed state through a single declared
// precedence:
//
//  1. A pinned pr/merged wins over an idle resting state (idleStates).
//  2. Otherwise a transcript structural blocker (pending plan/question)
//     overrides an idle-candidate state.
//  3. Otherwise the raw hook-reported State stands.
//
// Pinned is checked first, mirroring ApplyPinnedStates running before
// ApplyIdleOverrides in the former pipeline, so a pinned idle agent resolves
// without consulting the transcript. Pure and table-testable; transcriptOverride
// is "" when nothing is pending or the agent is not an idle candidate.
func arbitrateState(agent domain.Agent, transcriptOverride string) string {
	if agent.PinnedState != "" && idleStates[agent.State] {
		return agent.PinnedState
	}
	if transcriptOverride != "" {
		return transcriptOverride
	}
	return agent.State
}

// transcriptOverrideFor recovers a pending plan review (claude's ExitPlanMode /
// codex's item.type="Plan") or pending question (claude's AskUserQuestion /
// codex's request_user_input) from the agent's transcript. The most recent
// unanswered blocking tool wins — if AskUserQuestion appears after ExitPlanMode,
// "question" takes precedence. Returns "" when nothing is pending or the
// transcript can't be located.
//
// Per-harness dispatch:
//   - claude: scans the JSONL via conversation.LastPendingBlockingTool.
//   - codex: scans the rollout via codexconv.LastPendingBlockingToolCodex.
//     Pass codexSessionsRoot="" when codex isn't installed; the branch then
//     short-circuits to "" and behavior matches the claude-only path.
func transcriptOverrideFor(agent domain.Agent, codexSessionsRoot string) string {
	switch agent.Harness {
	case "codex":
		// LocateRollout returns "" on any lookup/IO error; treat that the same as
		// "no rollout found" → no override (mirrors the claude ProjDir guard below).
		path, _ := codexconv.LocateRollout(codexSessionsRoot, agent.SessionID)
		if path == "" {
			return ""
		}
		return codexconv.LastPendingBlockingToolCodex(path)
	default:
		if agent.ProjDir == "" {
			return ""
		}
		return conversation.LastPendingBlockingTool(agent.ProjDir, agent.SessionID)
	}
}

// ApplyStateArbitration resolves every agent's displayed State through the single
// arbitrateState precedence, replacing the former ApplyPinnedStates →
// ApplyIdleOverrides in-place pipeline whose precedence lived only in call order.
// Behavior is identical: a pinned idle agent resolves without a transcript read,
// and only idle candidates (isIdleCandidate) consult the transcript.
func ApplyStateArbitration(sf *domain.StateFile, codexSessionsRoot string) {
	for key, agent := range sf.Agents {
		override := ""
		// Skip the transcript read when a pin already decides the state (matches
		// ApplyPinnedStates running before the candidate-gated override).
		pinWins := agent.PinnedState != "" && idleStates[agent.State]
		if !pinWins && isIdleCandidate(agent) && agent.SessionID != "" {
			override = transcriptOverrideFor(agent, codexSessionsRoot)
		}
		if next := arbitrateState(agent, override); next != agent.State {
			agent.State = next
			sf.Agents[key] = agent
		}
	}
}

// isIdleCandidate is the per-harness candidate-state predicate for
// ApplyStateArbitration's transcript override. The "done" case is codex-only
// because claude's PostToolUse stop-state guard already prevents the equivalent
// stuck state for claude agents.
func isIdleCandidate(agent domain.Agent) bool {
	if agent.State == "idle_prompt" || agent.State == "permission" {
		return true
	}
	if agent.Harness == "codex" && agent.State == "done" {
		return true
	}
	return false
}

// PinAgentState writes a pinned_state field to the agent's JSON file.
// Hook updates merge into the file and preserve this field, so the
// dashboard-driven state survives while the agent continues working.
//
// Wrapped in the same sidecar lock as stampAgentFields and the JS
// writeState so concurrent writers cannot wipe pinned_state via the
// lost-update race.
func PinAgentState(dir, sessionID, pinnedState string) error {
	path := filepath.Join(AgentsDir(dir), sessionID+".json")
	return withFileLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("agent %s not found: %w", sessionID, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		raw["pinned_state"] = pinnedState
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		return writeJSONAtomic(path, out)
	})
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
		pi := sortPriority(agents[i])
		pj := sortPriority(agents[j])
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

// sortPriority is the sort key for SortedAgents: restart-survivor orphans sink
// to the dedicated RESUMABLE group after every live state group; everything
// else sorts by its state's priority. Callers must flag Resumable (see
// FlagResumable) before sorting or orphans interleave with live groups.
func sortPriority(a domain.Agent) int {
	if a.Resumable {
		return domain.ResumablePriority
	}
	p := domain.StatePriority[a.State]
	if p == 0 {
		p = 99
	}
	return p
}

// finishedStates are terminal/shipped states that are not worth resuming: a
// done task, an opened PR, or a merged branch. Orphans in these states are
// still GC'd by PruneDead; only active sessions are retained for resume.
var finishedStates = map[string]bool{"done": true, "pr": true, "merged": true}

// resumableTTL bounds how long a restart-survivor stays resumable. It is the
// backstop for shipped work the structural checks can't see — e.g. a
// squash-merged branch (invisible to merge-base ancestry) whose worktree
// still exists.
const resumableTTL = 72 * time.Hour

// IsResumableOrphan reports whether an agent is a restart-survivor worth
// continuing. Resumability is event-scoped to tmux server restarts: a pane
// that died while the current server kept running was closed deliberately and
// is never resumable. livePanes is the set of currently live pane IDs (%N
// format) and serverPID the current tmux server PID — both must come from the
// same enumeration (tmux.TmuxListPanes / TmuxListLivePaneIDs return them
// atomically). Used by PruneDead (retention), the web JSON (`resumable` flag),
// and both Cmd+K palettes (orphan marker + resume action), so "orphan" means
// the same thing everywhere.
func IsResumableOrphan(a domain.Agent, livePanes map[string]bool, serverPID string, now time.Time) bool {
	if a.SessionID == "" || a.State == "" || a.TmuxPaneID == "" {
		return false
	}
	// A nil set means tmux pane enumeration failed — we can't distinguish a live
	// pane from a dead one, so refuse to classify anything as an orphan rather
	// than risk treating a live agent as resumable. A non-nil empty set (tmux
	// succeeded, zero panes) correctly falls through: the agent is dead. The
	// server PID rides on the same enumeration, so it can only be empty when
	// the set is nil — the guard keeps the invariant explicit anyway.
	if livePanes == nil || serverPID == "" {
		return false
	}
	if livePanes[a.TmuxPaneID] {
		return false // pane is alive — not an orphan
	}
	// Event scope: no stamped server PID (pre-upgrade file, or the hook ran
	// outside tmux) or a PID matching the current server means the pane died
	// under a living server — a deliberate close, not a restart-survivor.
	if a.TmuxServerPID == "" || a.TmuxServerPID == serverPID {
		return false
	}
	// Finished work is not worth resuming. Pin-aware: arbitrateState applies a
	// pinned pr/merged over an idle resting state, so a pr-pinned idle_prompt
	// agent counts as finished even though its raw state field does not.
	if finishedStates[arbitrateState(a, "")] {
		return false
	}
	// No branch means no work product to resume toward (ad-hoc interactive
	// sessions); the branch is stamped by the worktree-reconcile hook for
	// every dashboard-spawned agent, including ones on the default branch.
	if a.Branch == "" {
		return false
	}
	// The resume spawns into EffectiveDir — a cleaned-up worktree means the
	// session cannot be meaningfully continued. Filesystem is the source of
	// truth, not the cached state fields.
	if _, err := os.Stat(a.EffectiveDir()); err != nil {
		return false
	}
	// Age backstop: a survivor nobody resumed within the TTL is stale.
	if a.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, a.UpdatedAt); err == nil && now.Sub(t) > resumableTTL {
			return false
		}
	}
	return true
}

// FlagResumable stamps the transient Resumable flag on every agent in the
// state file. Must run before SortedAgents so orphans sort into the
// RESUMABLE group (see sortPriority).
func FlagResumable(sf *domain.StateFile, livePanes map[string]bool, serverPID string, now time.Time) {
	for key, agent := range sf.Agents {
		if r := IsResumableOrphan(agent, livePanes, serverPID, now); r != agent.Resumable {
			agent.Resumable = r
			sf.Agents[key] = agent
		}
	}
}

// PruneDead removes agent files whose tmux panes no longer exist and
// deduplicates agents sharing the same live pane (keeps only the newest).
// livePaneIDs is the set of currently live tmux pane IDs (%N format) and
// serverPID the current tmux server PID from the same enumeration. Only
// restart-survivors (IsResumableOrphan) are retained on a dead pane; a pane
// closed under a living server is pruned like before the resume feature.
// isBranchMerged, when non-nil, additionally GCs survivors whose branch is
// already merged (checked in the survivor's EffectiveDir; the caller owns
// caching and the default-branch guard). Returns the number of agents removed.
func PruneDead(dir string, livePaneIDs map[string]bool, serverPID string, isBranchMerged func(dir, branch string) bool) int {
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
	now := time.Now()
	var dedupPaths, deadPaths []string
	for _, f := range files {
		// Dedup: when multiple agents share a pane, remove all but the newest.
		if f.agent.TmuxPaneID != "" && f.updatedAt.Before(newestPerPane[f.agent.TmuxPaneID]) {
			dedupPaths = append(dedupPaths, f.path)
			continue
		}
		// Dead: pane no longer exists. Restart-survivors (an active session
		// whose pane died with its tmux server) are kept on disk so the user
		// can find and continue them, and so resuming one doesn't
		// cascade-delete its siblings once it gets a live pane — unless the
		// survivor's branch is already merged, which means the work shipped.
		if f.agent.TmuxPaneID == "" || !livePaneIDs[f.agent.TmuxPaneID] {
			if IsResumableOrphan(f.agent, livePaneIDs, serverPID, now) {
				if isBranchMerged == nil || !isBranchMerged(f.agent.EffectiveDir(), f.agent.Branch) {
					continue
				}
			}
			deadPaths = append(deadPaths, f.path)
		}
	}

	// Safety net: refuse to wipe all agents at once — almost certainly
	// a transient tmux issue (empty livePaneIDs from a failed list-panes).
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

// ReadAgent reads a single agent's persisted state by session ID without
// running the resolve chain (worktree/branch/projdir/usage). This is the fast
// path for resume: the stored record already holds session_id, harness, cwd,
// worktree_cwd, and branch — everything needed to re-spawn. Returns false when
// no file matches the session ID.
func ReadAgent(dir, sessionID string) (domain.Agent, bool) {
	path, ok := agentFileMap(dir)[sessionID]
	if !ok {
		return domain.Agent{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Agent{}, false
	}
	var agent domain.Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return domain.Agent{}, false
	}
	agent.State = normalizeAgentState(agent.State)
	return agent, true
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
