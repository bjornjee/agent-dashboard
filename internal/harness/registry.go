// Package harness exposes a name-to-implementation registry so callers
// (config bootstrap, per-spawn override in web handlers) construct
// harnesses through a single seam.
package harness

import (
	"fmt"
	"path/filepath"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/claude"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
)

// ErrUnknownHarness is returned by Resolve when name doesn't match a known
// harness. Callers decide how to react — DefaultConfig falls back to claude
// with a log line, while web handlers return HTTP 400.
type ErrUnknownHarness struct{ Name string }

func (e ErrUnknownHarness) Error() string {
	return fmt.Sprintf("unknown harness %q (known: claude, codex)", e.Name)
}

// Resolve constructs a Harness by name. Returns ErrUnknownHarness for any
// name the registry doesn't recognize; the empty string resolves to claude
// (the documented default for unset settings.toml fields and request bodies
// that omit the harness override).
func Resolve(name string, profile domain.AgentProfile) (domain.Harness, error) {
	switch name {
	case "claude", "":
		return claude.New(profile), nil
	case "codex":
		return codex.New(codex.Config{
			Command:     "codex",
			SessionsDir: filepath.Join(profile.HomeDir, ".codex", "sessions"),
			ConfigDir:   filepath.Join(profile.HomeDir, ".codex"),
		}), nil
	default:
		return nil, ErrUnknownHarness{Name: name}
	}
}
