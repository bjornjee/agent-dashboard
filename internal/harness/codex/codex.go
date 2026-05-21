// Package codex adapts the OpenAI codex CLI as an agent-dashboard harness.
//
// Spawn-command construction targets codex CLI 0.130.0 surface:
//
//	codex [-c model_reasoning_effort=<E>] [--model <M>] [-a <APPROVAL>] [-s <SANDBOX>] <prompt>
//	codex resume <session-id>
//
// Effort flag uses the `-c model_reasoning_effort=<level>` config-override
// form because codex 0.130.0 has no top-level `--effort` flag (cf.
// `codex --help`). Claude's `max` level has no codex peer — codex's
// ReasoningEffort enum tops at `high` (codex-rs/protocol/src/config_types.rs).
// We map `max` → `high` so opt-in skills work uniformly across harnesses.
//
// Plan-mode, permission-mode, and tool-use signaling come over codex's
// native hook protocol (1:1 with Claude's; see internal/codex/ readers and
// adapters/claude-code/scripts/hooks/*.js). The harness itself only
// composes spawn strings.
package codex

import (
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Config is the static configuration the dashboard hands to the codex
// harness at construction time. SessionsDir/ConfigDir are pre-resolved by
// the dashboard's bootstrap; Command lets tests override the binary name.
type Config struct {
	Command     string // typically "codex"
	SessionsDir string // typically ~/.codex/sessions
	ConfigDir   string // typically ~/.codex
}

// Codex implements domain.Harness for the OpenAI codex CLI.
type Codex struct {
	cfg Config
}

// New constructs a Codex harness from a Config.
func New(cfg Config) *Codex {
	return &Codex{cfg: cfg}
}

// Name implements domain.Harness.
func (c *Codex) Name() string { return "codex" }

// SessionsDir implements domain.Harness.
func (c *Codex) SessionsDir() string { return c.cfg.SessionsDir }

// ConfigDir implements domain.Harness.
func (c *Codex) ConfigDir() string { return c.cfg.ConfigDir }

// SpawnCommand implements domain.Harness.
func (c *Codex) SpawnCommand(skill, message string, opts domain.SpawnOpts) string {
	if opts.ResumeSessionID != "" {
		return c.cfg.Command + " resume " + shellQuote(opts.ResumeSessionID)
	}

	cmd := c.cfg.Command
	if _, opted := effortOptedSkills[skill]; opted && opts.DefaultEffort != "" {
		cmd += " -c model_reasoning_effort=" + mapEffort(opts.DefaultEffort)
	}
	if opts.Model != "" {
		cmd += " --model " + shellQuote(opts.Model)
	}
	if opts.Approval != "" {
		cmd += " -a " + shellQuote(opts.Approval)
	}
	if opts.Sandbox != "" {
		cmd += " -s " + shellQuote(opts.Sandbox)
	}

	prompt := buildPrompt(skill, message)
	if prompt != "" {
		cmd += " " + shellQuote(prompt)
	}
	return cmd
}

// effortOptedSkills mirrors claude's set (internal/harness/claude/claude.go:53-57)
// so a skill's effort behavior stays harness-agnostic.
var effortOptedSkills = map[string]struct{}{
	"feature":  {},
	"fix":      {},
	"refactor": {},
}

// mapEffort translates dashboard-level effort to codex's reasoning_effort
// CLI value. Claude's `max` has no peer in codex's documented enum
// (minimal/low/medium/high — codex-rs/protocol/src/config_types.rs
// ReasoningEffort), so we clamp it to `high`. Other values pass through
// unchanged, including `xhigh` which some codex deployments accept as a
// premium tier even though it isn't in the public enum.
func mapEffort(level string) string {
	if level == "max" {
		return "high"
	}
	return level
}

// dashboardPluginNamespace is the plugin name codex must dispatch through.
// DiscoverSkills (internal/skills/skills.go) only scans the agent-dashboard
// plugin cache, so every skill it returns lives under this namespace.
const dashboardPluginNamespace = "agent-dashboard"

// buildPrompt composes the prompt argument codex receives on the command
// line. Codex CLI uses `$` (not `/`) as its plugin/skill sigil and requires
// the fully-qualified `$<plugin>:<skill>` form — see codex's own help text:
// "Use one by name, for example: $skills:terminal-ops run the failing
// tests". Bare `$<skill>` is treated as plain prompt text and the plugin
// is never dispatched.
func buildPrompt(skill, message string) string {
	var parts []string
	if skill != "" {
		parts = append(parts, "$"+dashboardPluginNamespace+":"+skill)
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
