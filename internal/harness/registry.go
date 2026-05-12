// Package harness exposes a name-to-implementation registry so callers
// (config bootstrap, per-spawn override in web handlers) construct
// harnesses through a single seam.
package harness

import (
	"path/filepath"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/claude"
	"github.com/bjornjee/agent-dashboard/internal/harness/pi"
)

// Resolve constructs a Harness by name. Unknown names fall back to claude —
// the dashboard always boots with *some* harness, and unrecognized values
// from settings.toml or request bodies do not crash the spawn flow.
func Resolve(name string, profile domain.AgentProfile) domain.Harness {
	switch name {
	case "pi":
		return pi.New(pi.Config{
			Command:     "pi",
			SessionsDir: filepath.Join(profile.HomeDir, ".pi", "agent", "sessions"),
			ConfigDir:   filepath.Join(profile.HomeDir, ".pi"),
		})
	default:
		return claude.New(profile)
	}
}
