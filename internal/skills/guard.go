package skills

// claudeOnlySkills is an explicit allowlist of dashboard skills that
// depend on Claude-only tools: EnterPlanMode, ExitPlanMode, AskUserQuestion,
// and the Agent tool (subagent dispatch). Codex CLI 0.130.0 does not
// implement any of these (evidence E13 — gh search across openai/codex
// returned zero hits), so spawning these skills against a codex session
// lets the agent boot but crash on the first plan-mode / interactive call.
//
// IMPORTANT: this is an allowlist, not a denylist. New skills are NOT
// blocked by default — when adding a skill that uses any of the
// Claude-only tools above, add its name here. CLAUDE.md documents this
// convention in the per-harness section.
var claudeOnlySkills = map[string]struct{}{
	"feature":     {},
	"fix":         {},
	"refactor":    {},
	"implement":   {},
	"investigate": {},
	"chore":       {},
	"rca":         {},
	"pr":          {},
}

// RequiresClaude reports whether the named skill needs a Claude session.
// Empty skill names (free-prompt spawns) are never blocked.
//
// Callers (web spawn handler) should surface a 400 with a clear message
// when this returns true and the active harness is "codex" — never silently
// fall through, since the session will appear to spawn and then crash.
func RequiresClaude(skill string) bool {
	if skill == "" {
		return false
	}
	_, ok := claudeOnlySkills[skill]
	return ok
}
