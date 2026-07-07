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

const defaultEffortCacheTTL = time.Hour

// DefaultEffortCache caches per-harness default effort hints. The zero value is
// usable, but NewDefaultEffortCache makes the clock explicit for tests.
type DefaultEffortCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]domain.DefaultEffortInfo
}

// NewDefaultEffortCache constructs a one-hour in-memory cache for effort hints.
func NewDefaultEffortCache() *DefaultEffortCache {
	return &DefaultEffortCache{
		now:     time.Now,
		entries: make(map[string]domain.DefaultEffortInfo),
	}
}

// SetNowForTest replaces the cache clock. It is intended for package tests.
func (c *DefaultEffortCache) SetNowForTest(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// Resolve returns the effective effort shown for the New Agent "(default)"
// option. It never returns config read errors to callers; a missing or
// malformed native config falls back to the harness's built-in behavior.
func (c *DefaultEffortCache) Resolve(h domain.Harness, settings domain.Settings, force bool) domain.DefaultEffortInfo {
	if c == nil {
		c = NewDefaultEffortCache()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensure()

	now := c.now()
	key := h.Name()
	if cached, ok := c.entries[key]; ok && !force && now.Before(cached.ExpiresAt) {
		return cached
	}

	info := resolveDefaultEffort(h, settings)
	info.CachedAt = now
	info.ExpiresAt = now.Add(defaultEffortCacheTTL)
	c.entries[key] = info
	return info
}

func (c *DefaultEffortCache) ensure() {
	if c.now == nil {
		c.now = time.Now
	}
	if c.entries == nil {
		c.entries = make(map[string]domain.DefaultEffortInfo)
	}
}

func resolveDefaultEffort(h domain.Harness, settings domain.Settings) domain.DefaultEffortInfo {
	switch h.Name() {
	case "codex":
		if settings.Harness.Codex.DefaultReasoningEffort != "" {
			return domain.DefaultEffortInfo{Effort: settings.Harness.Codex.DefaultReasoningEffort, Source: "agent-dashboard settings"}
		}
		if effort := readCodexConfigEffort(filepath.Join(h.ConfigDir(), "config.toml")); effort != "" {
			return domain.DefaultEffortInfo{Effort: effort, Source: "~/.codex/config.toml"}
		}
	case "claude":
		if settings.Effort.Default != "" {
			return domain.DefaultEffortInfo{Effort: settings.Effort.Default, Source: "agent-dashboard settings"}
		}
		if effort := readClaudeSettingsEffort(filepath.Join(h.ConfigDir(), "settings.json")); effort != "" {
			return domain.DefaultEffortInfo{Effort: effort, Source: "~/.claude/settings.json"}
		}
	}
	return domain.DefaultEffortInfo{Source: "harness built-in"}
}

func readClaudeSettingsEffort(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var settings struct {
		Effort string `json:"effortLevel"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return ""
	}
	return settings.Effort
}

func readCodexConfigEffort(path string) string {
	var cfg struct {
		Effort string `toml:"model_reasoning_effort"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ""
	}
	return cfg.Effort
}
