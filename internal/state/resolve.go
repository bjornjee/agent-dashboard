package state

import (
	"path/filepath"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

// ResolveOptions carries the directories and knobs ResolveChain needs. All
// fields are plain data so both the TUI and the web server (and any future
// caller, e.g. a doctor command) resolve agents identically.
type ResolveOptions struct {
	StateDir          string
	ClaudeProjectsDir string
	ClaudeSessionsDir string
	// CodexSessionsDir is the $CODEX_HOME/sessions root (conversation.Roots
	// form); its parent is the codex home used for identity lookups.
	CodexSessionsDir string
	TmuxAvailable    bool
	// SelfPaneID excludes the dashboard's own pane from the sorted output.
	SelfPaneID string
	// Store may be nil — every Store method degrades to a no-op.
	Store *Store
}

// ResolveChain is the canonical refresh pipeline shared by every surface.
// It encodes the order invariants that used to live only in comments:
// Hydrate must run on the raw file state before targets resolve, the
// reconcilers need resolved targets, FlagResumable must precede sorting,
// and Sync runs last so identity-adopted agents persist.
func ResolveChain(opts ResolveOptions) []domain.Agent {
	sf := ReadState(opts.StateDir)
	var paneCwds map[string]string
	var livePanes map[string]bool
	var serverPID string
	if opts.TmuxAvailable {
		targets, cwds, cmds, pid := tmux.TmuxListPanes()
		// targets is keyed by pane ID (%N) — the live-pane set used to flag
		// restart-survivor orphans. Leave livePanes nil when targets is nil
		// (tmux enumeration failed) so IsResumableOrphan can't misclassify
		// live agents as orphans; a non-nil empty targets (zero panes)
		// yields a non-nil empty set (genuinely all dead).
		livePanes = livePanesFromTargets(targets)
		opts.Store.Hydrate(&sf, livePanes, pid)
		ResolveAgentTargets(&sf, targets)
		ReconcileUnregistered(&sf, targets, cwds, cmds, time.Now())
		ReconcileIdentities(&sf, ReconcileIdentityOptions{
			Store:             opts.Store,
			ClaudeProjectsDir: opts.ClaudeProjectsDir,
			ClaudeSessionsDir: opts.ClaudeSessionsDir,
			CodexRoot:         filepath.Dir(opts.CodexSessionsDir),
		})
		paneCwds = cwds
		serverPID = pid
	}
	ResolveAgentProjDir(&sf, opts.ClaudeProjectsDir, opts.ClaudeSessionsDir)
	// Apply spawn-pins BEFORE marker-scan / scan-on-init so freshly-spawned
	// agents render with the dashboard-staged pin even when the JS hook
	// hasn't fired yet.
	ApplySpawnPins(&sf, opts.StateDir)
	ResolveAgentWorktree(&sf, opts.StateDir)
	ResolveAgentBranches(&sf, paneCwds, opts.StateDir)
	GCSpawnPins(opts.StateDir, 10*time.Minute)
	ApplyStateArbitration(&sf, opts.CodexSessionsDir)
	// Flag survivors before sorting so they sink into the RESUMABLE group.
	FlagResumable(&sf, livePanes, serverPID, time.Now())
	agents := conversation.TopLevelAgents(
		SortedAgents(sf, opts.SelfPaneID),
		conversation.Roots{CodexSessionsRoot: opts.CodexSessionsDir},
	)
	opts.Store.Sync(&sf)
	return agents
}

func livePanesFromTargets(targets map[string]domain.PaneTarget) map[string]bool {
	if targets == nil {
		return nil
	}
	livePanes := make(map[string]bool, len(targets))
	for paneID := range targets {
		livePanes[paneID] = true
	}
	return livePanes
}
