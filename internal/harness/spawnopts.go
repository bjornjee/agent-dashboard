package harness

import "github.com/bjornjee/agent-dashboard/internal/domain"

// SpawnOptsFor builds the SpawnOpts payload for the given harness name from
// user settings plus per-spawn model/effort overrides. Each harness consumes a
// disjoint subset:
//   - claude: DefaultEffort, Model, Effort
//   - codex:  DefaultEffort, Model, Effort, Approval, Sandbox
//
// Unused fields stay zero-valued (and are ignored by the receiving harness).
// Per-harness routing here keeps the spawn flag surface honest as new
// harnesses are added; the alternative (pass everything everywhere) leaks
// codex's Approval into every harness's contract.
//
// Shared by web/actions.go and tui/commands.go so both spawn paths route
// the same per-harness settings the same way.
func SpawnOptsFor(harnessName string, settings domain.Settings, model, effort string) domain.SpawnOpts {
	opts := domain.SpawnOpts{Effort: effort}
	switch harnessName {
	case "codex":
		// Codex effort comes from its own settings field rather than
		// settings.Effort.Default — codex levels (minimal/low/medium/high)
		// share names with Claude but the resolution chain differs.
		opts.DefaultEffort = settings.Harness.Codex.DefaultReasoningEffort
		opts.Model = settings.Harness.Codex.Model
		opts.Approval = settings.Harness.Codex.Approval
		opts.Sandbox = settings.Harness.Codex.Sandbox
		if model != "" {
			opts.Model = model
		}
		return opts
	default:
		opts.DefaultEffort = settings.Effort.Default
		opts.Model = model
		return opts
	}
}
