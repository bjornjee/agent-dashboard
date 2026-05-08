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
