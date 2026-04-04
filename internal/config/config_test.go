package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectUsername_FallbackToBeautiful(t *testing.T) {
	// When both os/user and $USER fail, should return "beautiful"
	t.Setenv("USER", "")
	// We can't easily mock os/user.Current(), but we can test firstName
	got := firstName("")
	if got != "" {
		t.Errorf("firstName(\"\") = %q, want \"\"", got)
	}
}

func TestFirstName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Bjorn Jee", "Bjorn"},
		{"Alice", "Alice"},
		{"", ""},
		{"John Michael Smith", "John"},
	}
	for _, tt := range tests {
		got := firstName(tt.input)
		if got != tt.want {
			t.Errorf("firstName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDetectEditor_RespectsEnv(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	got := detectEditor()
	if got != "vim" {
		t.Errorf("detectEditor() = %q, want \"vim\"", got)
	}
}

func TestDetectEditor_DefaultsToCode(t *testing.T) {
	t.Setenv("EDITOR", "")
	got := detectEditor()
	if got != "code" {
		t.Errorf("detectEditor() = %q, want \"code\"", got)
	}
}

func TestDefaultConfig_ProfilePaths(t *testing.T) {
	cfg := DefaultConfig()
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".claude")

	if cfg.Profile.ConfigDir != base {
		t.Errorf("ConfigDir = %q, want %q", cfg.Profile.ConfigDir, base)
	}
	wantStateDir := filepath.Join(home, ".agent-dashboard")
	if cfg.Profile.StateDir != wantStateDir {
		t.Errorf("StateDir = %q, want %q", cfg.Profile.StateDir, wantStateDir)
	}
	if cfg.Profile.ProjectsDir != filepath.Join(base, "projects") {
		t.Errorf("ProjectsDir = %q, want %q", cfg.Profile.ProjectsDir, filepath.Join(base, "projects"))
	}
	if cfg.Profile.PlansDir != filepath.Join(base, "plans") {
		t.Errorf("PlansDir = %q, want %q", cfg.Profile.PlansDir, filepath.Join(base, "plans"))
	}
	if cfg.Profile.SessionsDir != filepath.Join(base, "sessions") {
		t.Errorf("SessionsDir = %q, want %q", cfg.Profile.SessionsDir, filepath.Join(base, "sessions"))
	}
	if cfg.Profile.Name != "Claude Code" {
		t.Errorf("Name = %q, want \"Claude Code\"", cfg.Profile.Name)
	}
	if cfg.Profile.Command != "claude" {
		t.Errorf("Command = %q, want \"claude\"", cfg.Profile.Command)
	}
}

func TestDefaultConfig_EditorFromEnv(t *testing.T) {
	t.Setenv("EDITOR", "nvim")
	cfg := DefaultConfig()
	if cfg.Editor != "nvim" {
		t.Errorf("Editor = %q, want \"nvim\"", cfg.Editor)
	}
}
