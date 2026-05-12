package config

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
)

// DefaultConfig returns a fully populated config with auto-detected values.
// The active Harness is selected from settings.toml's [harness] default — a
// user who sets default = "pi" gets the pi-mono harness with their
// [harness.pi] provider+model preferences applied at spawn time.
//
// An unrecognized [harness] default (typo, removed harness) logs a warning
// and falls back to claude so the dashboard still boots. The web layer
// surfaces unknown harness names from request bodies as HTTP 400 — only
// the boot path is forgiving.
func DefaultConfig() domain.Config {
	profile := defaultClaudeProfile()
	settings := LoadSettings(profile.StateDir)

	h, err := harness.Resolve(settings.Harness.Default, profile)
	if err != nil {
		log.Printf("config: %v — falling back to claude", err)
		h, _ = harness.Resolve("claude", profile)
	}

	return domain.Config{
		Profile:  profile,
		Harness:  h,
		Username: detectUsername(),
		Editor:   detectEditor(),
		Settings: settings,
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
		HomeDir:        home,
	}
}
