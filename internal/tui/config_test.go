package tui

import (
	"github.com/bjornjee/agent-dashboard/internal/config"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// testConfig returns a domain.Config suitable for tests, with the given stateDir.
// Uses a non-existent PluginCacheDir so skill discovery is deterministic.
func testConfig(stateDir string) domain.Config {
	cfg := config.DefaultConfig()
	cfg.Profile.PluginCacheDir = "/nonexistent/plugin/cache"
	cfg.Settings = config.DefaultSettings() // pin to known state, ignore real filesystem
	if stateDir != "" {
		cfg.Profile.StateDir = stateDir
	}
	return cfg
}
