package pi_test

import (
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/pi"
)

func TestPi_Name(t *testing.T) {
	h := pi.New(pi.Config{Command: "pi"})
	if got := h.Name(); got != "pi" {
		t.Errorf("Name() = %q, want %q", got, "pi")
	}
}

func TestPi_SpawnCommand(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		message string
		opts    domain.SpawnOpts
		want    string
	}{
		{"bare", "", "", domain.SpawnOpts{}, "pi"},
		{"message only", "", "do the thing", domain.SpawnOpts{}, "pi 'do the thing'"},
		{"skill only", "feature", "", domain.SpawnOpts{}, "pi '/feature'"},
		{"skill + message", "feature", "add login", domain.SpawnOpts{}, "pi '/feature add login'"},
		{
			"openai codex routing",
			"feature", "",
			domain.SpawnOpts{Provider: "openai", Model: "openai-codex/gpt-5.5"},
			"pi --provider 'openai' --model 'openai-codex/gpt-5.5' '/feature'",
		},
		{
			"provider only (no model)",
			"", "explore the repo",
			domain.SpawnOpts{Provider: "anthropic"},
			"pi --provider 'anthropic' 'explore the repo'",
		},
		{
			"model only (no provider)",
			"", "",
			domain.SpawnOpts{Model: "openai-codex/gpt-5.5"},
			"pi --model 'openai-codex/gpt-5.5'",
		},
		{
			"provider with space gets quoted safely",
			"", "",
			domain.SpawnOpts{Provider: "open ai", Model: "model with space"},
			"pi --provider 'open ai' --model 'model with space'",
		},
		{
			"default effort ignored — pi-mono issue #4249",
			"feature", "",
			domain.SpawnOpts{DefaultEffort: "high"},
			"pi '/feature'",
		},
	}

	h := pi.New(pi.Config{Command: "pi"})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.SpawnCommand(tc.skill, tc.message, tc.opts)
			if got != tc.want {
				t.Errorf("SpawnCommand(%q, %q, %+v) = %q, want %q", tc.skill, tc.message, tc.opts, got, tc.want)
			}
		})
	}
}

func TestPi_SessionsDir(t *testing.T) {
	h := pi.New(pi.Config{SessionsDir: "/tmp/.pi/sessions"})
	if got := h.SessionsDir(); got != "/tmp/.pi/sessions" {
		t.Errorf("SessionsDir() = %q, want %q", got, "/tmp/.pi/sessions")
	}
}

func TestPi_ConfigDir(t *testing.T) {
	h := pi.New(pi.Config{ConfigDir: "/tmp/.pi"})
	if got := h.ConfigDir(); got != "/tmp/.pi" {
		t.Errorf("ConfigDir() = %q, want %q", got, "/tmp/.pi")
	}
}

// Pi-mono shell-quotes prompts the same way claude does, so '/feature with $vars'
// stays safe.
func TestPi_SpawnCommand_QuotesPromptSafely(t *testing.T) {
	h := pi.New(pi.Config{Command: "pi"})
	got := h.SpawnCommand("", "echo $HOME", domain.SpawnOpts{})
	want := "pi 'echo $HOME'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
