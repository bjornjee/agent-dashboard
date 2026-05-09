package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettings_ExampleFileIntegration(t *testing.T) {
	src, err := os.ReadFile("../../settings.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), src, 0644); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings(dir)
	t.Logf("Loaded: effort.plan=%q effort.default=%q banner.show_mascot=%v usage.poll=%d",
		s.Effort.Plan, s.Effort.Default, s.Banner.ShowMascot, s.Usage.RateLimitPollSeconds)
	if s.Effort.Plan != "max" {
		t.Errorf("effort.plan = %q, want \"max\"", s.Effort.Plan)
	}
	if s.Effort.Default != "high" {
		t.Errorf("effort.default = %q, want \"high\"", s.Effort.Default)
	}
	if !s.Banner.ShowMascot {
		t.Error("banner.show_mascot should parse as true")
	}
	if s.Usage.RateLimitPollSeconds != 60 {
		t.Errorf("usage.rate_limit_poll_seconds = %d, want 60", s.Usage.RateLimitPollSeconds)
	}
}
