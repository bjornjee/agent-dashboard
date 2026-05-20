package harness

import "github.com/bjornjee/agent-dashboard/internal/domain"

// SpawnOptsFor builds the SpawnOpts payload for the given harness name from
// user settings. Each harness consumes a disjoint subset:
//   - claude: DefaultEffort
//   - codex:  DefaultEffort, Model, Approval, Sandbox
//
// Unused fields stay zero-valued (and are ignored by the receiving harness).
// Per-harness routing here keeps the spawn flag surface honest as new
// harnesses are added; the alternative (pass everything everywhere) leaks
// codex's Approval into every harness's contract.
//
// Shared by web/actions.go and tui/commands.go so both spawn paths route
// the same per-harness settings the same way.
func SpawnOptsFor(harnessName string, settings domain.Settings) domain.SpawnOpts {
	switch harnessName {
	case "codex":
		// Codex effort comes from its own settings field rather than
		// settings.Effort.Default — codex levels (minimal/low/medium/high)
		// share names with Claude but the resolution chain differs.
		return domain.SpawnOpts{
			DefaultEffort: settings.Harness.Codex.DefaultReasoningEffort,
			Model:         settings.Harness.Codex.Model,
			Approval:      settings.Harness.Codex.Approval,
			Sandbox:       settings.Harness.Codex.Sandbox,
		}
	default:
		return domain.SpawnOpts{DefaultEffort: settings.Effort.Default}
	}
}
