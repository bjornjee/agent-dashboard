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
		children: map[string][]domain.SubagentInfo{"p": {{AgentID: "c"}}},
	}
	c.putIndex("/tmp/root", idx)

	*clock = clock.Add(4 * time.Second)

	got, ok := c.getIndex("/tmp/root")
	if !ok {
		t.Fatal("expected cache hit within TTL, got miss")
	}
	if len(got.children["p"]) != 1 {
		t.Errorf("children[p] len = %d, want 1", len(got.children["p"]))
	}
}

func TestCache_PutThenGet_PastTTL_ReturnsMiss(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	clock := withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	idx := &sessionsIndex{
		builtAt:  *clock,
		children: map[string][]domain.SubagentInfo{},
	}
	c.putIndex("/tmp/root", idx)

	*clock = clock.Add(6 * time.Second)

	if _, ok := c.getIndex("/tmp/root"); ok {
		t.Fatal("expected cache miss past TTL, got hit")
	}
}

func TestCache_NegativeEntry_Cached(t *testing.T) {
	// An empty children map still caches — the next FindSubagents call hits
	// without re-walking (negative result within TTL).
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	idx := &sessionsIndex{
		builtAt:  nowFunc(),
		children: map[string][]domain.SubagentInfo{},
	}
	c.putIndex("/tmp/root", idx)

	got, ok := c.getIndex("/tmp/root")
	if !ok {
		t.Fatal("expected hit on empty index within TTL, got miss")
	}
	if got.children["missing-parent"] != nil {
		t.Errorf("children[missing-parent] = %v, want nil", got.children["missing-parent"])
	}
}

func TestCache_InvalidateForTest_ClearsAll(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	idx := &sessionsIndex{
		builtAt:  nowFunc(),
		children: map[string][]domain.SubagentInfo{"p": {{AgentID: "y"}}},
	}
	pkgCache.putIndex("/tmp/root", idx)

	InvalidateCacheForTest()

	if _, ok := pkgCache.getIndex("/tmp/root"); ok {
		t.Error("index still cached after invalidate")
	}
}
