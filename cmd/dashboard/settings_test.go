package main

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
