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

// NotificationSettings controls desktop notifications sent by adapter hooks.
type NotificationSettings struct {
	Enabled      bool `toml:"enabled"`
	Sound        bool `toml:"sound"`
	SilentEvents bool `toml:"silent_events"`
}

// DebugSettings controls debug/diagnostic features.
type DebugSettings struct {
	KeyLog bool `toml:"key_log"` // write key/mouse/focus events to debug-keys.log
}

// Settings holds all user-facing configuration loaded from settings.toml.
type Settings struct {
	Banner        BannerSettings       `toml:"banner"`
	Notifications NotificationSettings `toml:"notifications"`
	Debug         DebugSettings        `toml:"debug"`
}

// DefaultSettings returns settings with sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		Banner: BannerSettings{
			ShowMascot: true,
			ShowQuote:  true,
		},
		Notifications: NotificationSettings{
			Enabled:      false,
			Sound:        false,
			SilentEvents: false,
		},
	}
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
