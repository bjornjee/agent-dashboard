package state

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/git"
)

// SpawnPin is the dashboard's hand-off record written immediately after
// tmux spawns a new agent pane and consumed by either the JS hook or the
// Go-side state pipeline once the agent's session file appears.
//
// Keyed by tmux pane_id (stable for the life of the pane), so the record
// survives positional renumbering when sibling panes close.
type SpawnPin struct {
	PaneID      string `json:"pane_id"`
	Target      string `json:"target"`
	WorktreeCwd string `json:"worktree_cwd"`
	Branch      string `json:"branch"`
	CreatedAt   string `json:"created_at"`
}

// SpawnPinsDir is the staging subdirectory under stateDir.
func SpawnPinsDir(stateDir string) string {
	return filepath.Join(stateDir, "spawn-pins")
}

// spawnPinFilename converts a tmux pane_id (`%N`) to a filename-safe form.
// `%` is technically POSIX-legal but shell-hostile; replace it with `_` so
// paths print cleanly in logs and tools.
func spawnPinFilename(paneID string) string {
	return strings.ReplaceAll(paneID, "%", "_") + ".json"
}

// WriteSpawnPin persists p to stateDir/spawn-pins/<paneID>.json via the
// same atomic-rename idiom used for agent state files.
func WriteSpawnPin(stateDir string, p SpawnPin) error {
	if p.PaneID == "" {
		return nil
	}
	if err := os.MkdirAll(SpawnPinsDir(stateDir), 0o755); err != nil {
		return err
	}
	path := filepath.Join(SpawnPinsDir(stateDir), spawnPinFilename(p.PaneID))
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, data)
}

// ReadSpawnPin returns the staged pin for paneID, or ok=false when absent
// or malformed.
func ReadSpawnPin(stateDir, paneID string) (SpawnPin, bool) {
	if paneID == "" {
		return SpawnPin{}, false
	}
	path := filepath.Join(SpawnPinsDir(stateDir), spawnPinFilename(paneID))
	data, err := os.ReadFile(path)
	if err != nil {
		return SpawnPin{}, false
	}
	var p SpawnPin
	if err := json.Unmarshal(data, &p); err != nil {
		return SpawnPin{}, false
	}
	return p, true
}

// DeleteSpawnPin removes the staging file. Missing-file is not an error.
func DeleteSpawnPin(stateDir, paneID string) {
	if paneID == "" {
		return
	}
	_ = os.Remove(filepath.Join(SpawnPinsDir(stateDir), spawnPinFilename(paneID)))
}

// ApplySpawnPins merges staged spawn-pin records into agents whose pane_id
// matches. Closes the spawn-to-first-hook window: the dashboard renders the
// pin immediately after spawn, without waiting for the JS hook to fire.
//
// For each agent:
//   - If WorktreeCwd is already set: GC the staging file (it's redundant).
//   - Else if a staged pin exists for the agent's TmuxPaneID and the pin has
//     a non-empty WorktreeCwd: apply it (in-memory + on-disk stamp), then
//     delete the staging file.
//
// Must run BEFORE ResolveAgentWorktree so the marker-scan / scan-on-init
// paths skip already-pinned agents.
func ApplySpawnPins(sf *domain.StateFile, stateDir string) {
	for key, agent := range sf.Agents {
		if agent.TmuxPaneID == "" {
			continue
		}
		pin, ok := ReadSpawnPin(stateDir, agent.TmuxPaneID)
		if !ok {
			continue
		}
		if agent.WorktreeCwd != "" || pin.WorktreeCwd == "" {
			// Either the agent is already pinned (no merge needed) or the
			// staging file was empty (non-worktree spawn). Drop the staging
			// file in both cases — its job is done.
			DeleteSpawnPin(stateDir, agent.TmuxPaneID)
			continue
		}
		// Reuse the existing pin-write helper for consistency with the
		// marker-scan path. Synthesizes a git.Worktree from the staged data.
		pinAgentToWorktree(sf, key, &agent, git.Worktree{Path: pin.WorktreeCwd, Branch: pin.Branch}, stateDir)
		DeleteSpawnPin(stateDir, agent.TmuxPaneID)
	}
}

// ReapStaleMarker deletes the worktree's marker file when its owner sessionID
// no longer has a state file on disk. Safety invariant: never touch a marker
// whose owner is still live — that would let an unrelated sibling agent
// hijack the pin.
//
// Called from the create flow so a new agent spawned into a worktree that
// previously held a (now-dead) agent can claim a fresh marker via the JS
// hook's reconcileWorktree.
//
// No-op when worktreePath is not a linked worktree (its .git is a directory,
// not the per-worktree pointer file).
func ReapStaleMarker(stateDir, worktreePath string) {
	if worktreePath == "" {
		return
	}
	dotgit := filepath.Join(worktreePath, ".git")
	info, err := os.Lstat(dotgit)
	if err != nil || !info.Mode().IsRegular() {
		return // not a linked worktree
	}
	body, err := os.ReadFile(dotgit)
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(body))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	marker := filepath.Join(gitDir, "agent-dashboard-session")
	data, err := os.ReadFile(marker)
	if err != nil {
		return
	}
	owner := strings.TrimSpace(string(data))
	if owner == "" {
		return
	}
	// Live owner = state file still present. Don't touch.
	if _, err := os.Stat(filepath.Join(AgentsDir(stateDir), owner+".json")); err == nil {
		return
	}
	_ = os.Remove(marker)
}

// StageSpawnPin writes a SpawnPin for a freshly-spawned agent pane. Best-
// effort: a missing worktree match leaves WorktreeCwd/Branch empty (which is
// still useful — consumers know "no pin expected" and skip the fallback).
//
// The git lookup uses the package's branchRunner so tests can swap it via
// SetTestRunner, same as ResolveAgentWorktree / ResolveAgentBranches.
func StageSpawnPin(stateDir, absFolder, paneID, target string) error {
	if paneID == "" {
		return nil
	}
	pin := SpawnPin{
		PaneID:    paneID,
		Target:    target,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if wts, err := git.ListWorktrees(ctx, branchRunner, absFolder); err == nil {
		want := canonicalPath(absFolder)
		for _, wt := range wts {
			if canonicalPath(wt.Path) == want {
				pin.WorktreeCwd = wt.Path
				pin.Branch = wt.Branch
				break
			}
		}
	}
	// Clear any stale marker so the new agent's first hook can atomically
	// claim a fresh one. Safe — only removes markers whose owner has no
	// live state file.
	if pin.WorktreeCwd != "" {
		ReapStaleMarker(stateDir, pin.WorktreeCwd)
	}
	return WriteSpawnPin(stateDir, pin)
}

// GCSpawnPins deletes staging files whose CreatedAt is older than the cutoff.
// Files with unparseable CreatedAt are treated as stale and removed.
// Returns the count of files removed.
func GCSpawnPins(stateDir string, olderThan time.Duration) int {
	entries, err := os.ReadDir(SpawnPinsDir(stateDir))
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-olderThan)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(SpawnPinsDir(stateDir), e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p SpawnPin
		stale := false
		if err := json.Unmarshal(data, &p); err != nil {
			stale = true
		} else {
			t, perr := time.Parse(time.RFC3339, p.CreatedAt)
			if perr != nil || t.Before(cutoff) {
				stale = true
			}
		}
		if stale {
			if os.Remove(path) == nil {
				removed++
			}
		}
	}
	return removed
}
