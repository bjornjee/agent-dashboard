package domain

// Harness abstracts the per-coding-agent operational surface — the parts that
// actually differ between claude-code, pi-mono, and any future harness.
//
// What does NOT belong on this interface: directories that are dashboard-owned
// (e.g. AgentProfile.StateDir is the same ~/.agent-dashboard for every
// harness) and behavior that lives in shared packages (e.g. tmux spawning,
// state file writes). The interface captures only spawn-command construction
// and the two harness-owned filesystem locations the dashboard reads from.
type Harness interface {
	// Name returns a stable, lowercase identifier ("claude", "pi"). Used as
	// the registry key in settings.toml ([harness] default = "<Name>").
	Name() string

	// SpawnCommand builds the shell command that launches an interactive
	// session in a tmux pane. The returned string is passed directly to
	// tmux new-window / split-window as the pane's initial process.
	SpawnCommand(skill, message string, opts SpawnOpts) string

	// SessionsDir returns the directory where this harness writes session
	// logs (used for usage/billing parsing).
	SessionsDir() string

	// ConfigDir returns this harness's user-config root (e.g. ~/.claude,
	// ~/.pi). The dashboard reads plugin/skill metadata from this location.
	ConfigDir() string
}

// SpawnOpts carries the per-invocation knobs a harness may consult.
// Fields that don't apply to a given harness are ignored — Provider and
// Model only affect pi-mono today; DefaultEffort only affects claude-code.
type SpawnOpts struct {
	DefaultEffort string // e.g. "high" | "medium" | "" (omit flag)
	Provider      string // pi-mono only: "" (default) | "openai" | "anthropic" | ...
	Model         string // pi-mono only: e.g. "openai-codex/gpt-5.5"
}
