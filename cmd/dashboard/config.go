package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// AgentProfile defines how the dashboard discovers and interacts with a coding agent.
type AgentProfile struct {
	Name           string // Display name: "Claude Code"
	Command        string // Binary to launch: "claude"
	ConfigDir      string // Base config dir: ~/.claude
	StateDir       string // Dashboard state: ~/.agent-dashboard
	ProjectsDir    string // Conversations: ~/.claude/projects
	PlansDir       string // Plans: ~/.claude/plans
	SessionsDir    string // Session index: ~/.claude/sessions
	PluginCacheDir string // Plugin cache: ~/.claude/plugins/cache
}

// Config holds all dashboard configuration.
type Config struct {
	Profile  AgentProfile
	Username string   // Greeting name
	Editor   string   // Editor command
	Settings Settings // User-facing settings from settings.toml
}

// DefaultConfig returns a fully populated config with auto-detected values.
func DefaultConfig() Config {
	profile := defaultClaudeProfile()
	return Config{
		Profile:  profile,
		Username: detectUsername(),
		Editor:   detectEditor(),
		Settings: LoadSettings(profile.StateDir),
	}
}

func detectUsername() string {
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Name) != "" {
		return firstName(u.Name)
	}
	for _, env := range []string{"USER", "LOGNAME"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "beautiful"
}

// firstName extracts the first name from a full name string.
func firstName(full string) string {
	parts := strings.Fields(full)
	if len(parts) > 0 {
		return parts[0]
	}
	return full
}

func detectEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return "code"
}

func defaultClaudeProfile() AgentProfile {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".claude")

	stateDir := filepath.Join(home, ".agent-dashboard")
	if env := os.Getenv("AGENT_DASHBOARD_DIR"); env != "" {
		stateDir = env
	}

	return AgentProfile{
		Name:           "Claude Code",
		Command:        "claude",
		ConfigDir:      base,
		StateDir:       stateDir,
		ProjectsDir:    filepath.Join(base, "projects"),
		PlansDir:       filepath.Join(base, "plans"),
		SessionsDir:    filepath.Join(base, "sessions"),
		PluginCacheDir: filepath.Join(base, "plugins", "cache"),
	}
}
