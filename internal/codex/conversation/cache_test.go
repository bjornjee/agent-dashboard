package conversation

import (
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func withFrozenClock(t *testing.T, start time.Time) *time.Time {
	t.Helper()
	now := start
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })
	return &now
}

func TestCache_PutThenGet_WithinTTL_ReturnsHit(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	clock := withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	idx := &sessionsIndex{
		builtAt:  *clock,
		rollouts: map[string]rolloutEntry{"sid-1": {Path: "/tmp/root/a.jsonl"}},
		children: map[string][]domain.SubagentInfo{},
	}
	c.putIndex("/tmp/root", idx)

	*clock = clock.Add(4 * time.Second)

	got, ok := c.getIndex("/tmp/root")
	if !ok {
		t.Fatal("expected cache hit within TTL, got miss")
	}
	if got.rollouts["sid-1"].Path != "/tmp/root/a.jsonl" {
		t.Errorf("Path = %q, want %q", got.rollouts["sid-1"].Path, "/tmp/root/a.jsonl")
	}
}

func TestCache_PutThenGet_PastTTL_ReturnsMiss(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	clock := withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	idx := &sessionsIndex{
		builtAt:  *clock,
		rollouts: map[string]rolloutEntry{"sid-1": {Path: "/tmp/root/a.jsonl"}},
	}
	c.putIndex("/tmp/root", idx)

	*clock = clock.Add(6 * time.Second)

	if _, ok := c.getIndex("/tmp/root"); ok {
		t.Fatal("expected cache miss past TTL, got hit")
	}
}

func TestCache_NegativeEntry_Cached(t *testing.T) {
	// Locating a missing session is represented as an empty rolloutEntry in
	// the index — the index itself is still cached, so the next lookup
	// hits without re-walking.
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	idx := &sessionsIndex{
		builtAt:  nowFunc(),
		rollouts: map[string]rolloutEntry{},
		children: map[string][]domain.SubagentInfo{},
	}
	c.putIndex("/tmp/root", idx)

	got, ok := c.getIndex("/tmp/root")
	if !ok {
		t.Fatal("expected hit on empty index within TTL, got miss")
	}
	if got.rollouts["missing-sid"].Path != "" {
		t.Errorf("Path = %q, want empty", got.rollouts["missing-sid"].Path)
	}
}

func TestCache_InvalidateForTest_ClearsAll(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	idx := &sessionsIndex{
		builtAt:  nowFunc(),
		rollouts: map[string]rolloutEntry{"sid-1": {Path: "x"}},
		children: map[string][]domain.SubagentInfo{"p": {{AgentID: "y"}}},
	}
	pkgCache.putIndex("/tmp/root", idx)

	InvalidateCacheForTest()

	if _, ok := pkgCache.getIndex("/tmp/root"); ok {
		t.Error("index still cached after invalidate")
	}
}
