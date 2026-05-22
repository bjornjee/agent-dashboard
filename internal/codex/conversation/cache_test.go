package conversation

import (
	"sync"
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
	key := cacheKey{root: "/tmp/root", sessionID: "sid-1"}
	c.putRollout(key, rolloutEntry{Path: "/tmp/root/a.jsonl", MetaRead: true})

	// Advance 4s — still within 5s TTL.
	*clock = clock.Add(4 * time.Second)

	got, ok := c.getRollout(key)
	if !ok {
		t.Fatal("expected cache hit within TTL, got miss")
	}
	if got.Path != "/tmp/root/a.jsonl" {
		t.Errorf("Path = %q, want %q", got.Path, "/tmp/root/a.jsonl")
	}
	if !got.MetaRead {
		t.Errorf("MetaRead = false, want true")
	}
}

func TestCache_PutThenGet_PastTTL_ReturnsMiss(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	clock := withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	key := cacheKey{root: "/tmp/root", sessionID: "sid-1"}
	c.putRollout(key, rolloutEntry{Path: "/tmp/root/a.jsonl"})

	// Advance 6s — past 5s TTL.
	*clock = clock.Add(6 * time.Second)

	if _, ok := c.getRollout(key); ok {
		t.Fatal("expected cache miss past TTL, got hit")
	}
}

func TestCache_NegativeEntry_Cached(t *testing.T) {
	// Locating a missing session should also cache the negative (empty path)
	// so repeated lookups for sessions without rollout files don't re-walk.
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	key := cacheKey{root: "/tmp/root", sessionID: "missing-sid"}
	c.putRollout(key, rolloutEntry{Path: ""})

	got, ok := c.getRollout(key)
	if !ok {
		t.Fatal("expected hit (negative entry within TTL), got miss")
	}
	if got.Path != "" {
		t.Errorf("Path = %q, want empty", got.Path)
	}
}

func TestCache_SubagentList_RoundTrip(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	clock := withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	key := cacheKey{root: "/tmp/root", sessionID: "parent-1"}
	subs := []domain.SubagentInfo{
		{AgentID: "child-a", Description: "a"},
		{AgentID: "child-b", Description: "b"},
	}
	c.putSubagentList(key, subs)

	got, ok := c.getSubagentList(key)
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if len(got) != 2 || got[0].AgentID != "child-a" || got[1].AgentID != "child-b" {
		t.Errorf("got = %+v, want 2 entries [child-a, child-b]", got)
	}

	// Past TTL → miss.
	*clock = clock.Add(6 * time.Second)
	if _, ok := c.getSubagentList(key); ok {
		t.Fatal("expected miss past TTL, got hit")
	}
}

func TestCache_InvalidateForTest_ClearsAll(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	// Use the package-level singleton to verify the helper clears it.
	key := cacheKey{root: "/tmp/root", sessionID: "sid-1"}
	pkgCache.putRollout(key, rolloutEntry{Path: "x"})
	pkgCache.putSubagentList(key, []domain.SubagentInfo{{AgentID: "y"}})

	InvalidateCacheForTest()

	if _, ok := pkgCache.getRollout(key); ok {
		t.Error("rollout still cached after invalidate")
	}
	if _, ok := pkgCache.getSubagentList(key); ok {
		t.Error("subagent list still cached after invalidate")
	}
}

func TestCache_ConcurrentReadsAndWrites_NoRace(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	withFrozenClock(t, time.Unix(1_700_000_000, 0))

	c := newCache(5 * time.Second)
	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := cacheKey{root: "/r", sessionID: "sid"}
				c.putRollout(key, rolloutEntry{Path: "x"})
				_, _ = c.getRollout(key)
				c.putSubagentList(key, []domain.SubagentInfo{{AgentID: "a"}})
				_, _ = c.getSubagentList(key)
			}
		}(i)
	}
	wg.Wait()
}
