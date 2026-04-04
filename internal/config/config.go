package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// DefaultConfig returns a fully populated config with auto-detected values.
func DefaultConfig() domain.Config {
	profile := defaultClaudeProfile()
	return domain.Config{
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

func defaultClaudeProfile() domain.AgentProfile {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".claude")

	stateDir := filepath.Join(home, ".agent-dashboard")
	if env := os.Getenv("AGENT_DASHBOARD_DIR"); env != "" {
		stateDir = env
	}

	return domain.AgentProfile{
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
