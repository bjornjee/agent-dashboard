package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

const defaultModelCacheTTL = time.Hour

// DefaultModelCache caches per-harness default model hints. The zero value is
// usable, but NewDefaultModelCache makes the clock explicit for tests.
type DefaultModelCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]domain.DefaultModelInfo
}

// NewDefaultModelCache constructs a one-hour in-memory cache for model hints.
func NewDefaultModelCache() *DefaultModelCache {
	return &DefaultModelCache{
		now:     time.Now,
		entries: make(map[string]domain.DefaultModelInfo),
	}
}

// SetNowForTest replaces the cache clock. It is intended for package tests.
func (c *DefaultModelCache) SetNowForTest(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// Resolve returns the effective model shown for the New Agent "(default)"
// option. It never returns config read errors to callers; a missing or
// malformed native config falls back to the harness's first advertised model.
func (c *DefaultModelCache) Resolve(h domain.Harness, settings domain.Settings, force bool) domain.DefaultModelInfo {
	if c == nil {
		c = NewDefaultModelCache()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()

	now := c.now()
	key := h.Name()
	if cached, ok := c.entries[key]; ok && !force && now.Before(cached.ExpiresAt) {
		return cached
	}

	info := resolveDefaultModel(h, settings)
	info.CachedAt = now
	info.ExpiresAt = now.Add(defaultModelCacheTTL)
	c.entries[key] = info
	return info
}

func (c *DefaultModelCache) ensure() {
	if c.now == nil {
		c.now = time.Now
	}
	if c.entries == nil {
		c.entries = make(map[string]domain.DefaultModelInfo)
	}
}

func resolveDefaultModel(h domain.Harness, settings domain.Settings) domain.DefaultModelInfo {
	switch h.Name() {
	case "codex":
		if settings.Harness.Codex.Model != "" {
			return domain.DefaultModelInfo{Model: settings.Harness.Codex.Model, Source: "agent-dashboard settings"}
		}
		if model := readCodexConfigModel(filepath.Join(h.ConfigDir(), "config.toml")); model != "" {
			return domain.DefaultModelInfo{Model: model, Source: "~/.codex/config.toml"}
		}
	case "claude":
		if model := readClaudeSettingsModel(filepath.Join(h.ConfigDir(), "settings.json")); model != "" {
			return domain.DefaultModelInfo{Model: model, Source: "~/.claude/settings.json"}
		}
	}
	models := h.Models()
	if len(models) == 0 {
		return domain.DefaultModelInfo{Source: "harness default"}
	}
	return domain.DefaultModelInfo{Model: models[0], Source: "harness default"}
}

func readClaudeSettingsModel(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var settings struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return ""
	}
	return settings.Model
}

func readCodexConfigModel(path string) string {
	var cfg struct {
		Model string `toml:"model"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ""
	}
	return cfg.Model
}
