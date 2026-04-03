package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// BannerSettings controls what appears in the top banner.
type BannerSettings struct {
	ShowMascot bool `toml:"show_mascot"`
	ShowQuote  bool `toml:"show_quote"`
}

// Settings holds all user-facing configuration loaded from settings.toml.
type Settings struct {
	Banner BannerSettings `toml:"banner"`
}

// DefaultSettings returns settings with everything enabled.
func DefaultSettings() Settings {
	return Settings{
		Banner: BannerSettings{
			ShowMascot: true,
			ShowQuote:  true,
		},
	}
}

// defaultSettingsTOML is the initial settings.toml content written on first run.
const defaultSettingsTOML = `# Agent Dashboard settings
# See https://github.com/bjornjee/agent-dashboard for documentation.

[banner]
show_mascot = true   # show the axolotl pixel art
show_quote  = true   # show the daily quote
`

// EnsureSettings creates a default settings.toml in stateDir if one does not
// already exist. The stateDir itself is created if needed.
func EnsureSettings(stateDir string) {
	path := filepath.Join(stateDir, "settings.toml")
	if _, err := os.Stat(path); err == nil {
		return // already exists
	}
	_ = os.MkdirAll(stateDir, 0o755)
	_ = os.WriteFile(path, []byte(defaultSettingsTOML), 0o644)
}

// LoadSettings reads settings.toml from stateDir, falling back to defaults
// if the file is missing or malformed.
func LoadSettings(stateDir string) Settings {
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
