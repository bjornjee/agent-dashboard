package tui

import (
	"context"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/repo"
)

// resolveAgentTopology is the single entry point from the tui layer into
// repo.Resolve. It enforces the seed order (agent.Cwd primary, agent.WorktreeCwd
// as bootstrap fallback) so consumers cannot accidentally drop the robustness
// of the variadic seed list.
func resolveAgentTopology(ctx context.Context, a domain.Agent) (repo.Topology, error) {
	return repo.Resolve(ctx, gitRunner, a.Cwd, a.WorktreeCwd)
}

// sessionCandidates returns the explicit, deduplicated list of paths to use
// when locating a Claude session for an agent. Order is most-specific first
// (Cwd, then Worktree root, then Source root) — Locate's tie-break uses the
// session's StartedAt, so order is informational rather than functional, but
// it documents the expected match priority.
func sessionCandidates(a domain.Agent, top repo.Topology) []string {
	return []string{a.Cwd, top.Worktree, top.Source}
}
