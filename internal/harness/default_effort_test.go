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

func TestDefaultEffortCacheClaudeDashboardSettingWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"effortLevel":"xhigh"}`), 0600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})
	settings := domain.Settings{
		Effort: domain.EffortSettings{Default: "high"},
	}
	cache := harness.NewDefaultEffortCache()
	cache.SetNowForTest(func() time.Time { return time.Unix(100, 0) })

	got := cache.Resolve(h, settings, false)

	if got.Effort != "high" {
		t.Errorf("Effort = %q, want dashboard default", got.Effort)
	}
	if got.Source != "agent-dashboard settings" {
		t.Errorf("Source = %q, want agent-dashboard settings", got.Source)
	}
	if got.CachedAt.IsZero() || got.ExpiresAt.Sub(got.CachedAt) != time.Hour {
		t.Errorf("cache timestamps = %v/%v, want one-hour TTL", got.CachedAt, got.ExpiresAt)
	}
}

func TestDefaultEffortCacheClaudeSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"effortLevel":"xhigh"}`), 0600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})

	got := harness.NewDefaultEffortCache().Resolve(h, domain.Settings{}, false)

	if got.Effort != "xhigh" {
		t.Errorf("Effort = %q, want native xhigh passthrough", got.Effort)
	}
	if got.Source != "~/.claude/settings.json" {
		t.Errorf("Source = %q, want ~/.claude/settings.json", got.Source)
	}
}

func TestDefaultEffortCacheCodexDashboardSettingWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model_reasoning_effort = "medium"`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})
	settings := domain.Settings{
		Harness: domain.HarnessSettings{
			Codex: domain.CodexHarnessSettings{DefaultReasoningEffort: "high"},
		},
	}

	got := harness.NewDefaultEffortCache().Resolve(h, settings, false)

	if got.Effort != "high" {
		t.Errorf("Effort = %q, want dashboard default", got.Effort)
	}
	if got.Source != "agent-dashboard settings" {
		t.Errorf("Source = %q, want agent-dashboard settings", got.Source)
	}
}

func TestDefaultEffortCacheCodexConfigFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model_reasoning_effort = "high"`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})

	got := harness.NewDefaultEffortCache().Resolve(h, domain.Settings{}, false)

	if got.Effort != "high" {
		t.Errorf("Effort = %q, want config effort", got.Effort)
	}
	if got.Source != "~/.codex/config.toml" {
		t.Errorf("Source = %q, want ~/.codex/config.toml", got.Source)
	}
}

func TestDefaultEffortCacheMissingConfigFallsBackToHarnessBuiltIn(t *testing.T) {
	dir := t.TempDir()
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})

	got := harness.NewDefaultEffortCache().Resolve(h, domain.Settings{}, false)

	if got.Effort != "" {
		t.Errorf("Effort = %q, want empty fallback", got.Effort)
	}
	if got.Source != "harness built-in" {
		t.Errorf("Source = %q, want harness built-in", got.Source)
	}
}

func TestDefaultEffortCacheMalformedConfigFallsBackToHarnessBuiltIn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model_reasoning_effort =`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := codex.New(codex.Config{Command: "codex", ConfigDir: dir})

	got := harness.NewDefaultEffortCache().Resolve(h, domain.Settings{}, false)

	if got.Effort != "" {
		t.Errorf("Effort = %q, want empty fallback", got.Effort)
	}
	if got.Source != "harness built-in" {
		t.Errorf("Source = %q, want harness built-in", got.Source)
	}
}

func TestDefaultEffortCacheForceRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"effortLevel":"low"}`), 0600); err != nil {
		t.Fatalf("write initial settings: %v", err)
	}
	h := claude.New(domain.AgentProfile{Command: "claude", ConfigDir: dir})
	cache := harness.NewDefaultEffortCache()
	now := time.Unix(100, 0)
	cache.SetNowForTest(func() time.Time { return now })

	first := cache.Resolve(h, domain.Settings{}, false)
	if first.Effort != "low" {
		t.Fatalf("initial Effort = %q, want low", first.Effort)
	}
	if err := os.WriteFile(path, []byte(`{"effortLevel":"max"}`), 0600); err != nil {
		t.Fatalf("write updated settings: %v", err)
	}
	cached := cache.Resolve(h, domain.Settings{}, false)
	if cached.Effort != "low" {
		t.Errorf("cached Effort = %q, want low before TTL expiry", cached.Effort)
	}
	refreshed := cache.Resolve(h, domain.Settings{}, true)
	if refreshed.Effort != "max" {
		t.Errorf("refreshed Effort = %q, want max", refreshed.Effort)
	}
}

func TestDefaultEffortCachePerHarnessKeying(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"effortLevel":"max"}`), 0600); err != nil {
		t.Fatalf("write claude settings: %v", err)
	}
	codexDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`model_reasoning_effort = "high"`), 0600); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
	cache := harness.NewDefaultEffortCache()

	claudeInfo := cache.Resolve(claude.New(domain.AgentProfile{Command: "claude", ConfigDir: claudeDir}), domain.Settings{}, false)
	codexInfo := cache.Resolve(codex.New(codex.Config{Command: "codex", ConfigDir: codexDir}), domain.Settings{}, false)

	if claudeInfo.Effort != "max" {
		t.Errorf("claude Effort = %q, want max", claudeInfo.Effort)
	}
	if codexInfo.Effort != "high" {
		t.Errorf("codex Effort = %q, want high", codexInfo.Effort)
	}
}
