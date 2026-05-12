package claude_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/claude"
)

// New(profile).Name() must be "claude" so the harness registry can key by it.
func TestClaude_Name(t *testing.T) {
	h := claude.New(domain.AgentProfile{Command: "claude"})
	if got := h.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

// SpawnCommand mirrors the legacy buildAgentCommand contract: opted-in skills
// get CLAUDE_CODE_EFFORT_LEVEL=<level> + --effort <level>; non-opted ones
// inherit Claude Code's default effort.
func TestClaude_SpawnCommand(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		message string
		opts    domain.SpawnOpts
		want    string
	}{
		{"empty skill empty message", "", "", domain.SpawnOpts{DefaultEffort: "high"}, "claude"},
		{"empty skill with message", "", "do the thing", domain.SpawnOpts{DefaultEffort: "high"}, "claude 'do the thing'"},
		{"feature opted-in", "feature", "", domain.SpawnOpts{DefaultEffort: "high"}, "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/feature'"},
		{"fix opted-in", "fix", "", domain.SpawnOpts{DefaultEffort: "high"}, "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/fix'"},
		{"refactor opted-in", "refactor", "", domain.SpawnOpts{DefaultEffort: "high"}, "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/refactor'"},
		{"chore not opted-in", "chore", "", domain.SpawnOpts{DefaultEffort: "high"}, "claude '/chore'"},
		{"investigate not opted-in", "investigate", "", domain.SpawnOpts{DefaultEffort: "high"}, "claude '/investigate'"},
		{"feature with message", "feature", "add login", domain.SpawnOpts{DefaultEffort: "high"}, "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/feature add login'"},
		{"custom default effort", "feature", "", domain.SpawnOpts{DefaultEffort: "medium"}, "CLAUDE_CODE_EFFORT_LEVEL=medium claude --effort medium '/feature'"},
		{"empty default effort omits flag", "feature", "", domain.SpawnOpts{}, "claude '/feature'"},
	}

	h := claude.New(domain.AgentProfile{Command: "claude"})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.SpawnCommand(tc.skill, tc.message, tc.opts)
			if got != tc.want {
				t.Errorf("SpawnCommand(%q, %q, %+v) = %q, want %q", tc.skill, tc.message, tc.opts, got, tc.want)
			}
		})
	}
}

// SessionsDir returns the profile's claude session log directory unchanged.
func TestClaude_SessionsDir(t *testing.T) {
	p := domain.AgentProfile{SessionsDir: "/tmp/claude-sessions"}
	if got := claude.New(p).SessionsDir(); got != "/tmp/claude-sessions" {
		t.Errorf("SessionsDir() = %q, want %q", got, "/tmp/claude-sessions")
	}
}

// ConfigDir returns the profile's claude config dir unchanged.
func TestClaude_ConfigDir(t *testing.T) {
	p := domain.AgentProfile{ConfigDir: "/tmp/.claude"}
	if got := claude.New(p).ConfigDir(); got != "/tmp/.claude" {
		t.Errorf("ConfigDir() = %q, want %q", got, "/tmp/.claude")
	}
}

// Provider/Model fields on SpawnOpts are ignored by claude — only pi-mono
// dispatches on them. This locks that contract.
func TestClaude_SpawnCommand_IgnoresProviderAndModel(t *testing.T) {
	h := claude.New(domain.AgentProfile{Command: "claude"})
	got := h.SpawnCommand("feature", "", domain.SpawnOpts{DefaultEffort: "high", Provider: "openai", Model: "openai-codex/gpt-5.5"})
	want := "CLAUDE_CODE_EFFORT_LEVEL=high claude --effort high '/feature'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
