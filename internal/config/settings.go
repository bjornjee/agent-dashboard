package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// DefaultSettings returns settings with sensible defaults.
func DefaultSettings() domain.Settings {
	return domain.Settings{
		Banner: domain.BannerSettings{
			ShowMascot: true,
			ShowQuote:  true,
		},
		Notifications: domain.NotificationSettings{
			Enabled:      false,
			Sound:        false,
			SilentEvents: false,
		},
		Usage: domain.UsageSettings{
			RateLimitPollSeconds: 60,
		},
	}
}

// LoadSettings reads settings.toml from stateDir, falling back to defaults
// if the file is missing or malformed.
func LoadSettings(stateDir string) domain.Settings {
	s := DefaultSettings()
	path := filepath.Join(stateDir, "settings.toml")

	if _, err := toml.DecodeFile(path, &s); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s // file absent — keep defaults
		}
		return DefaultSettings() // malformed — fall back to defaults
	}
	return s
}
