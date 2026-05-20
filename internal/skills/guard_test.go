package skills_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/skills"
)

func TestSupportsHarness(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		harness string
		want    bool
	}{
		{"empty skill on codex", "", "codex", true},
		{"feature on codex", "feature", "codex", true},
		{"fix on codex", "fix", "codex", true},
		{"refactor on codex", "refactor", "codex", true},
		{"investigate on codex", "investigate", "codex", true},
		{"chore on codex", "chore", "codex", true},
		{"pr on codex", "pr", "codex", true},
		{"implement on codex", "implement", "codex", false},
		{"rca on codex", "rca", "codex", false},

		{"known skill on claude", "feature", "claude", true},
		{"unknown skill on claude", "custom", "claude", true},
		{"unknown skill on codex", "custom", "codex", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := skills.SupportsHarness(tc.skill, tc.harness); got != tc.want {
				t.Errorf("SupportsHarness(%q, %q) = %v, want %v", tc.skill, tc.harness, got, tc.want)
			}
		})
	}
}
