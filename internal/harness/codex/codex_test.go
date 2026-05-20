package codex_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
)

// New(cfg).Name() must be "codex" so the harness registry can key by it.
func TestCodex_Name(t *testing.T) {
	h := codex.New(codex.Config{Command: "codex"})
	if got := h.Name(); got != "codex" {
		t.Errorf("Name() = %q, want %q", got, "codex")
	}
}

// SpawnCommand assembles `codex [-c model_reasoning_effort=<E>] [--model <M>]
// [-a <APPROVAL>] [-s <SANDBOX>] <prompt>` per the codex CLI 0.130.0 surface
// (config flags from inspection of ~/.codex/config.toml).
//
// Effort opt-in skills (feature/fix/refactor) match the Claude opt-in set —
// see internal/harness/claude/claude.go:53-57.
func TestCodex_SpawnCommand(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		message string
		opts    domain.SpawnOpts
		want    string
	}{
		{"empty everything", "", "", domain.SpawnOpts{}, "codex"},
		{"prompt only", "", "hello", domain.SpawnOpts{}, "codex 'hello'"},
		{"feature opted-in pins effort", "feature", "", domain.SpawnOpts{DefaultEffort: "high"}, "codex -c model_reasoning_effort=high '/feature'"},
		{"fix opted-in pins effort", "fix", "", domain.SpawnOpts{DefaultEffort: "medium"}, "codex -c model_reasoning_effort=medium '/fix'"},
		{"refactor opted-in pins effort", "refactor", "", domain.SpawnOpts{DefaultEffort: "low"}, "codex -c model_reasoning_effort=low '/refactor'"},
		// Claude has a `max` level; codex's enum tops out at `high`
		// (codex-rs/protocol/src/config_types.rs ReasoningEffort). Map max → high.
		{"max effort maps to high", "feature", "", domain.SpawnOpts{DefaultEffort: "max"}, "codex -c model_reasoning_effort=high '/feature'"},
		{"chore not opted-in", "chore", "", domain.SpawnOpts{DefaultEffort: "high"}, "codex '/chore'"},
		{"model flag", "", "", domain.SpawnOpts{Model: "gpt-5.5"}, "codex --model 'gpt-5.5'"},
		{"approval flag", "", "", domain.SpawnOpts{Approval: "on-request"}, "codex -a 'on-request'"},
		{"sandbox flag", "", "", domain.SpawnOpts{Sandbox: "workspace-write"}, "codex -s 'workspace-write'"},
		{"all flags + prompt", "feature", "add login", domain.SpawnOpts{DefaultEffort: "high", Model: "gpt-5.5", Approval: "on-request", Sandbox: "workspace-write"}, "codex -c model_reasoning_effort=high --model 'gpt-5.5' -a 'on-request' -s 'workspace-write' '/feature add login'"},
	}

	h := codex.New(codex.Config{Command: "codex"})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.SpawnCommand(tc.skill, tc.message, tc.opts)
			if got != tc.want {
				t.Errorf("SpawnCommand(%q, %q, %+v) = %q, want %q", tc.skill, tc.message, tc.opts, got, tc.want)
			}
		})
	}
}

// Resume mode emits `codex resume <sid>` and drops the prompt + per-session
// flags (codex stores those with the session itself). Verified against
// `codex resume --help` in codex CLI 0.130.0.
func TestCodex_SpawnCommand_Resume(t *testing.T) {
	h := codex.New(codex.Config{Command: "codex"})
	opts := domain.SpawnOpts{
		ResumeSessionID: "019e39d0-581e-7f81-beb0-ea3c04ffa2e4",
		// Flags below must be ignored on resume — codex remembers them.
		DefaultEffort: "high",
		Model:         "gpt-5.5",
		Approval:      "on-request",
	}
	got := h.SpawnCommand("feature", "add login", opts)
	want := "codex resume '019e39d0-581e-7f81-beb0-ea3c04ffa2e4'"
	if got != want {
		t.Errorf("resume SpawnCommand = %q, want %q", got, want)
	}
}

// SessionsDir returns the per-instance codex sessions root (e.g. ~/.codex/sessions).
func TestCodex_SessionsDir(t *testing.T) {
	h := codex.New(codex.Config{SessionsDir: "/tmp/codex/sessions"})
	if got := h.SessionsDir(); got != "/tmp/codex/sessions" {
		t.Errorf("SessionsDir() = %q, want %q", got, "/tmp/codex/sessions")
	}
}

// ConfigDir returns the codex config root (e.g. ~/.codex).
func TestCodex_ConfigDir(t *testing.T) {
	h := codex.New(codex.Config{ConfigDir: "/tmp/.codex"})
	if got := h.ConfigDir(); got != "/tmp/.codex" {
		t.Errorf("ConfigDir() = %q, want %q", got, "/tmp/.codex")
	}
}
