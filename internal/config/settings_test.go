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
	if s.Effort.Plan != "high" {
		t.Errorf("Effort.Plan should default to \"high\", got %q", s.Effort.Plan)
	}
	if s.Effort.Default != "high" {
		t.Errorf("Effort.Default should default to \"high\", got %q", s.Effort.Default)
	}
}

func TestLoadSettings_EffortCustom(t *testing.T) {
	dir := t.TempDir()
	content := `[effort]
plan    = "high"
default = "medium"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Effort.Plan != "high" {
		t.Errorf("Effort.Plan = %q, want \"high\"", s.Effort.Plan)
	}
	if s.Effort.Default != "medium" {
		t.Errorf("Effort.Default = %q, want \"medium\"", s.Effort.Default)
	}
}

func TestLoadSettings_EffortPartial(t *testing.T) {
	dir := t.TempDir()
	content := `[effort]
plan = "low"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Effort.Plan != "low" {
		t.Errorf("Effort.Plan = %q, want \"low\"", s.Effort.Plan)
	}
	if s.Effort.Default != "high" {
		t.Errorf("Effort.Default = %q, want \"high\" (omitted key should fall back to default)", s.Effort.Default)
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

func TestDefaultSettings_Harness(t *testing.T) {
	s := DefaultSettings()
	if s.Harness.Default != "claude" {
		t.Errorf("Harness.Default = %q, want \"claude\"", s.Harness.Default)
	}
	if s.Harness.Pi.Provider != "" {
		t.Errorf("Harness.Pi.Provider = %q, want \"\"", s.Harness.Pi.Provider)
	}
	if s.Harness.Pi.Model != "" {
		t.Errorf("Harness.Pi.Model = %q, want \"\"", s.Harness.Pi.Model)
	}
}

func TestLoadSettings_HarnessPi(t *testing.T) {
	dir := t.TempDir()
	content := `[harness]
default = "pi"

[harness.pi]
provider = "openai"
model    = "openai-codex/gpt-5.5"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Harness.Default != "pi" {
		t.Errorf("Harness.Default = %q, want \"pi\"", s.Harness.Default)
	}
	if s.Harness.Pi.Provider != "openai" {
		t.Errorf("Harness.Pi.Provider = %q, want \"openai\"", s.Harness.Pi.Provider)
	}
	if s.Harness.Pi.Model != "openai-codex/gpt-5.5" {
		t.Errorf("Harness.Pi.Model = %q, want \"openai-codex/gpt-5.5\"", s.Harness.Pi.Model)
	}
}

func TestLoadSettings_HarnessPartial(t *testing.T) {
	dir := t.TempDir()
	// Only [harness.pi].provider — default harness key omitted, model omitted.
	content := `[harness.pi]
provider = "anthropic"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadSettings(dir)
	if s.Harness.Default != "claude" {
		t.Errorf("Harness.Default = %q, want \"claude\" (omitted key falls back)", s.Harness.Default)
	}
	if s.Harness.Pi.Provider != "anthropic" {
		t.Errorf("Harness.Pi.Provider = %q, want \"anthropic\"", s.Harness.Pi.Provider)
	}
	if s.Harness.Pi.Model != "" {
		t.Errorf("Harness.Pi.Model = %q, want \"\"", s.Harness.Pi.Model)
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

func TestSaveSettings_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := DefaultSettings()
	s.Harness.Default = "codex"
	s.Harness.Pi.Provider = "openai"
	s.Harness.Pi.Model = "openai-codex/gpt-5.5"

	if err := SaveSettings(dir, s); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	got := LoadSettings(dir)
	if got.Harness.Default != "codex" {
		t.Errorf("round-trip Harness.Default = %q, want %q", got.Harness.Default, "codex")
	}
	if got.Harness.Pi.Provider != "openai" {
		t.Errorf("round-trip Harness.Pi.Provider = %q, want %q", got.Harness.Pi.Provider, "openai")
	}
	if got.Harness.Pi.Model != "openai-codex/gpt-5.5" {
		t.Errorf("round-trip Harness.Pi.Model = %q, want %q", got.Harness.Pi.Model, "openai-codex/gpt-5.5")
	}
	if got.Effort.Default != "high" {
		t.Errorf("round-trip Effort.Default = %q, want %q (untouched defaults must survive)", got.Effort.Default, "high")
	}
}

func TestSaveSettings_NoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSettings(dir, DefaultSettings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.toml.tmp")); !os.IsNotExist(err) {
		t.Errorf("settings.toml.tmp should not exist after successful save, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.toml")); err != nil {
		t.Errorf("settings.toml should exist after successful save: %v", err)
	}
}

func TestSaveSettings_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("# stale\n[harness]\ndefault = \"pi\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := DefaultSettings()
	s.Harness.Default = "codex"
	if err := SaveSettings(dir, s); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	got := LoadSettings(dir)
	if got.Harness.Default != "codex" {
		t.Errorf("after overwrite Harness.Default = %q, want %q", got.Harness.Default, "codex")
	}
}

func TestSaveSettings_MissingDir(t *testing.T) {
	if err := SaveSettings(filepath.Join(t.TempDir(), "does-not-exist"), DefaultSettings()); err == nil {
		t.Error("SaveSettings against non-existent dir should error")
	}
}
