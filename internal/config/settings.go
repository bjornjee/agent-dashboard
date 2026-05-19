package config

import (
	"bytes"
	"errors"
	"fmt"
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
		Effort: domain.EffortSettings{
			Plan:    "high",
			Default: "high",
		},
		Harness: domain.HarnessSettings{
			Default: "claude",
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

// SaveSettings writes settings to $stateDir/settings.toml atomically (encode
// to a unique temp file in the same dir, then rename). Round-tripping
// through BurntSushi/toml drops comments and unknown keys — callers
// exposing a UI over this must warn users that hand-edited annotations
// will be lost.
//
// The temp file uses os.CreateTemp so concurrent SaveSettings calls don't
// share a single ".tmp" path and clobber each other's in-flight bytes.
func SaveSettings(stateDir string, s domain.Settings) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return fmt.Errorf("save settings: encode: %w", err)
	}

	tmp, err := os.CreateTemp(stateDir, "settings-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("save settings: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails before rename succeeds.
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("save settings: chmod temp: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("save settings: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("save settings: close temp: %w", err)
	}

	finalPath := filepath.Join(stateDir, "settings.toml")
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("save settings: rename: %w", err)
	}
	return nil
}
