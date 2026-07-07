package harness_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness"
	"github.com/bjornjee/agent-dashboard/internal/harness/claude"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
)

func TestDefaultModelCacheClaudeSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model":"claude-fable-5"}`), 0600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})
	cache := harness.NewDefaultModelCache()
	cache.SetNowForTest(func() time.Time { return time.Unix(100, 0) })

	got := cache.Resolve(h, domain.Settings{}, false)

	if got.Model != "claude-fable-5" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-fable-5")
	}
	if got.Source != "~/.claude/settings.json" {
		t.Errorf("Source = %q, want ~/.claude/settings.json", got.Source)
	}
	if got.CachedAt.IsZero() || got.ExpiresAt.Sub(got.CachedAt) != time.Hour {
		t.Errorf("cache timestamps = %v/%v, want one-hour TTL", got.CachedAt, got.ExpiresAt)
	}
}

func TestDefaultModelCacheCodexDashboardSettingWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model = "gpt-5.4"`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})
	settings := domain.Settings{
		Harness: domain.HarnessSettings{
			Codex: domain.CodexHarnessSettings{Model: "gpt-5.5"},
		},
	}

	got := harness.NewDefaultModelCache().Resolve(h, settings, false)

	if got.Model != "gpt-5.5" {
		t.Errorf("Model = %q, want dashboard override", got.Model)
	}
	if got.Source != "agent-dashboard settings" {
		t.Errorf("Source = %q, want agent-dashboard settings", got.Source)
	}
}

func TestDefaultModelCacheCodexConfigFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model = "gpt-5.4"`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})

	got := harness.NewDefaultModelCache().Resolve(h, domain.Settings{}, false)

	if got.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want config model", got.Model)
	}
	if got.Source != "~/.codex/config.toml" {
		t.Errorf("Source = %q, want ~/.codex/config.toml", got.Source)
	}
}

func TestDefaultModelCacheMalformedConfigFallsBackToHarnessDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model =`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})

	got := harness.NewDefaultModelCache().Resolve(h, domain.Settings{}, false)

	if got.Model != "gpt-5.5" {
		t.Errorf("Model = %q, want first harness model fallback", got.Model)
	}
	if got.Source != "harness default" {
		t.Errorf("Source = %q, want harness default", got.Source)
	}
}

func TestDefaultModelCacheForceRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"sonnet"}`), 0600); err != nil {
		t.Fatalf("write initial settings: %v", err)
	}
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})
	cache := harness.NewDefaultModelCache()
	now := time.Unix(100, 0)
	cache.SetNowForTest(func() time.Time { return now })

	first := cache.Resolve(h, domain.Settings{}, false)
	if first.Model != "sonnet" {
		t.Fatalf("initial Model = %q, want sonnet", first.Model)
	}
	if err := os.WriteFile(path, []byte(`{"model":"opus"}`), 0600); err != nil {
		t.Fatalf("write updated settings: %v", err)
	}
	cached := cache.Resolve(h, domain.Settings{}, false)
	if cached.Model != "sonnet" {
		t.Errorf("cached Model = %q, want sonnet before TTL expiry", cached.Model)
	}
	refreshed := cache.Resolve(h, domain.Settings{}, true)
	if refreshed.Model != "opus" {
		t.Errorf("refreshed Model = %q, want opus", refreshed.Model)
	}
}
