package claude

import (
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Claude is the agent-dashboard harness adapter for Claude Code.
//
// Spawn-command construction matches the historical buildAgentCommand
// contract: skills declared in `effortOptedSkills` (those whose SKILL.md
// frontmatter `effort:` is not consistently honored after EnterPlanMode
// flips permission_mode) get a CLAUDE_CODE_EFFORT_LEVEL env-var prefix
// (so the SessionStart hook can persist the level for display) and the
// `--effort <level>` CLI flag (so CC pins session-level effort).
type Claude struct {
	profile domain.AgentProfile
}

// New constructs a Claude harness from an AgentProfile. The profile carries
// the binary name (Profile.Command) and the harness-owned dirs (ConfigDir,
// SessionsDir).
func New(profile domain.AgentProfile) *Claude {
	return &Claude{profile: profile}
}

// Name implements domain.Harness.
func (c *Claude) Name() string { return "claude" }

// SessionsDir implements domain.Harness.
func (c *Claude) SessionsDir() string { return c.profile.SessionsDir }

// ConfigDir implements domain.Harness.
func (c *Claude) ConfigDir() string { return c.profile.ConfigDir }

// SpawnCommand implements domain.Harness.
func (c *Claude) SpawnCommand(skill, message string, opts domain.SpawnOpts) string {
	cmd := c.profile.Command
	if _, opted := effortOptedSkills[skill]; opted && opts.DefaultEffort != "" {
		cmd = "CLAUDE_CODE_EFFORT_LEVEL=" + opts.DefaultEffort + " " + cmd + " --effort " + opts.DefaultEffort
	}
	prompt := buildPrompt(skill, message)
	if prompt != "" {
		cmd = cmd + " " + shellQuote(prompt)
	}
	return cmd
}

// effortOptedSkills lists the skills that opt into spawn-time effort pinning.
// Kept here (alongside SpawnCommand) rather than in web/ because it's a
// claude-specific contract: pi-mono has no equivalent flag today.
var effortOptedSkills = map[string]struct{}{
	"feature":  {},
	"fix":      {},
	"refactor": {},
}

func buildPrompt(skill, message string) string {
	var parts []string
	if skill != "" {
		parts = append(parts, "/"+skill)
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
