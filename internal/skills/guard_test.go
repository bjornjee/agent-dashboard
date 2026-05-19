package skills_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/skills"
)

// RequiresClaude returns true for skills that depend on Claude-only tools
// (EnterPlanMode / ExitPlanMode / AskUserQuestion / Agent with subagent_type).
// Per evidence E13 (gh search across openai/codex returned zero hits for
// these tool names), these skills cannot complete on a codex session.
// The dashboard's spawn path must runtime-block them with a clear error
// rather than letting the session start and then crash on the first
// EnterPlanMode call.
func TestRequiresClaude(t *testing.T) {
	tests := []struct {
		skill string
		want  bool
	}{
		// Active dashboard skills that call EnterPlanMode/ExitPlanMode/AskUserQuestion/Agent
		{"feature", true},
		{"fix", true},
		{"refactor", true},
		{"implement", true},
		{"investigate", true},
		{"chore", true},
		{"rca", true},
		{"pr", true},

		// Empty skill (free prompt, no skill envelope) — never blocked.
		{"", false},

		// Hypothetical codex-compatible skill (not yet defined).
		{"codex-rescue", false},
	}
	for _, tc := range tests {
		t.Run(tc.skill, func(t *testing.T) {
			if got := skills.RequiresClaude(tc.skill); got != tc.want {
				t.Errorf("RequiresClaude(%q) = %v, want %v", tc.skill, got, tc.want)
			}
		})
	}
}
