package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if !s.Banner.ShowMascot {
		t.Error("ShowMascot should default to true")
	}
	if !s.Banner.ShowQuote {
		t.Error("ShowQuote should default to true")
	}
	if s.Notifications.Enabled {
		t.Error("Notifications.Enabled should default to false")
	}
	if s.Notifications.Sound {
		t.Error("Notifications.Sound should default to false")
	}
	if s.Notifications.SilentEvents {
		t.Error("Notifications.SilentEvents should default to false")
	}
	if s.Experimental.AsciiPet {
		t.Error("Experimental.AsciiPet should default to false")
	}
	if s.Usage.RateLimitPollSeconds != 60 {
		t.Errorf("Usage.RateLimitPollSeconds should default to 60, got %d", s.Usage.RateLimitPollSeconds)
	}
}

func TestLoadSettings_MissingFile(t *testing.T) {
	s := LoadSettings(t.TempDir())
	if !s.Banner.ShowMascot || !s.Banner.ShowQuote {
		t.Error("missing file should return defaults (all true)")
	}
}

func TestLoadSettings_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	content := `[banner]
show_mascot = false
show_quote = false

[notifications]
enabled       = true
sound         = true
silent_events = true
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Banner.ShowMascot {
		t.Error("ShowMascot should be false")
	}
	if s.Banner.ShowQuote {
		t.Error("ShowQuote should be false")
	}
	if !s.Notifications.Enabled {
		t.Error("Notifications.Enabled should be true")
	}
	if !s.Notifications.Sound {
		t.Error("Notifications.Sound should be true")
	}
	if !s.Notifications.SilentEvents {
		t.Error("Notifications.SilentEvents should be true")
	}
}

func TestLoadSettings_PartialTOML(t *testing.T) {
	dir := t.TempDir()
	content := `[banner]
show_mascot = false
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Banner.ShowMascot {
		t.Error("ShowMascot should be false")
	}
	if !s.Banner.ShowQuote {
		t.Error("ShowQuote should default to true when omitted")
	}
}

func TestLoadSettings_ExperimentalAsciiPet(t *testing.T) {
	dir := t.TempDir()
	content := "[experimental]\nascii_pet = true\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if !s.Experimental.AsciiPet {
		t.Error("expected Experimental.AsciiPet to be true")
	}
}

func TestLoadSettings_UsagePollInterval(t *testing.T) {
	dir := t.TempDir()
	content := "[usage]\nrate_limit_poll_seconds = 30\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Usage.RateLimitPollSeconds != 30 {
		t.Errorf("expected RateLimitPollSeconds=30, got %d", s.Usage.RateLimitPollSeconds)
	}
}

func TestLoadSettings_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("{{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if !s.Banner.ShowMascot || !s.Banner.ShowQuote {
		t.Error("invalid TOML should return defaults (all true)")
	}
}
