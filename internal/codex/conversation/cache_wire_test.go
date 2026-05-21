package conversation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/conversation"
)

// After LocateRollout warms the cache, subsequent calls within the TTL
// window must return the cached path. We assert this by deleting the
// underlying file between calls — a fresh walk would return "" but a cache
// hit returns the original path.
func TestLocateRollout_UsesCacheAcrossCalls(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	sid := "019e39d0-581e-7f81-beb0-ea3c04ffa2e4"
	target := writeRollout(t, root, "2026", "05", "21", sid, "{}")

	first, err := conversation.LocateRollout(root, sid)
	if err != nil || first != target {
		t.Fatalf("first call: got %q err %v, want %q", first, err, target)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	second, err := conversation.LocateRollout(root, sid)
	if err != nil {
		t.Fatal(err)
	}
	if second != target {
		t.Errorf("second call after file removed: got %q, want cached %q", second, target)
	}
}

// Negative results must also be cached so repeated lookups for missing
// sessions don't replay the walk.
func TestLocateRollout_CachesNegativeResult(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	first, _ := conversation.LocateRollout(root, "missing-sid")
	if first != "" {
		t.Fatalf("first call: got %q, want empty for missing sid", first)
	}

	// Add the file after the negative cache entry is written. Within the
	// TTL the cached empty result must win — proving negative caching is
	// active.
	target := writeRollout(t, root, "2026", "05", "21", "missing-sid", "{}")
	_ = target

	second, _ := conversation.LocateRollout(root, "missing-sid")
	if second != "" {
		t.Errorf("second call: got %q, want cached empty (negative cache not wired?)", second)
	}
}

// ParentThreadID must use the cache so per-agent lookups in TopLevelAgents
// don't re-read rollout files within the TTL window.
func TestParentThreadID_UsesCacheAcrossCalls(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	sid := "child-sid"
	target := writeRollout(t, root, "2026", "05", "21", sid, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-sid","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-sid"}}}}}
`)

	if got := conversation.ParentThreadID(root, sid); got != "parent-sid" {
		t.Fatalf("first call: got %q, want parent-sid", got)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	if got := conversation.ParentThreadID(root, sid); got != "parent-sid" {
		t.Errorf("second call after file removed: got %q, want cached parent-sid", got)
	}
}

// FindSubagents results must be cached by parent session ID so background
// polling every 5s doesn't re-walk the full sessions tree.
func TestFindSubagents_UsesCacheAcrossCalls(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	parentID := "parent-sid"
	target := writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-sid","agent_role":"worker"}}}}}
`)

	first := conversation.FindSubagents(root, parentID)
	if len(first) != 1 {
		t.Fatalf("first call: got %d, want 1", len(first))
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	second := conversation.FindSubagents(root, parentID)
	if len(second) != 1 {
		t.Errorf("second call after file removed: got %d, want cached 1", len(second))
	}
}

// When the sessions root does not exist (codex not yet installed), the
// negative result must be cached so the next TopLevelAgents tick doesn't
// re-stat. We can't observe Stat directly; we observe by mutating the
// filesystem between calls and confirming the cached empty result wins.
func TestLocateRollout_CachesMissingRoot(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	parent := t.TempDir()
	root := filepath.Join(parent, "no-codex-yet")
	// First call against a missing root — should return "".
	first, err := conversation.LocateRollout(root, "any-sid")
	if err != nil || first != "" {
		t.Fatalf("first call: got %q err %v, want empty", first, err)
	}

	// Create the dir + a matching rollout. Without negative caching the
	// next call would find it; with negative caching it must still return
	// "" until TTL expires.
	target := writeRollout(t, root, "2026", "05", "21", "any-sid", "{}")
	_ = target

	second, err := conversation.LocateRollout(root, "any-sid")
	if err != nil {
		t.Fatal(err)
	}
	if second != "" {
		t.Errorf("second call: got %q, want cached empty (missing-root negative cache not wired)", second)
	}
}

// ParentThreadID must short-circuit after LocateRollout warms the cache
// with a Path-only entry. Without MetaRead, the cache hit falls through
// and re-opens the file every call — exactly the hot-path regression we
// shipped this fix to close.
func TestParentThreadID_ShortCircuitsAfterLocateRolloutWarmedCache(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	sid := "child-sid"
	target := writeRollout(t, root, "2026", "05", "21", sid, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-sid","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-sid"}}}}}
`)

	// Warm the cache via LocateRollout — Meta is NOT yet populated.
	if _, err := conversation.LocateRollout(root, sid); err != nil {
		t.Fatal(err)
	}

	// First ParentThreadID call opens the file once, populates Meta.
	if got := conversation.ParentThreadID(root, sid); got != "parent-sid" {
		t.Fatalf("first ParentThreadID: got %q, want parent-sid", got)
	}

	// Delete the file. A subsequent call MUST hit the MetaRead cache.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if got := conversation.ParentThreadID(root, sid); got != "parent-sid" {
		t.Errorf("second ParentThreadID: got %q, want cached parent-sid (MetaRead short-circuit not wired)", got)
	}
}

// A rollout file that exists but contains no session_meta line (newly
// spawned session that hasn't written events yet) must NOT cause every
// poll tick to re-open the file. The cache stores the negative meta-read
// outcome.
func TestParentThreadID_CachesNegativeMetaRead(t *testing.T) {
	t.Cleanup(conversation.InvalidateCacheForTest)

	root := t.TempDir()
	sid := "newly-spawned"
	// Rollout file exists but has no session_meta line.
	target := writeRollout(t, root, "2026", "05", "21", sid, "{}\n")

	if got := conversation.ParentThreadID(root, sid); got != "" {
		t.Fatalf("first call: got %q, want empty (no session_meta in file)", got)
	}

	// Add a session_meta line. Without the negative-meta-read cache, the
	// next call would parse the file. With the cache, it must still
	// return "" until TTL expires.
	contents := `{"timestamp":"t1","type":"session_meta","payload":{"id":"newly-spawned","timestamp":"t1","source":{"subagent":{"thread_spawn":{"parent_thread_id":"some-parent"}}}}}
`
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := conversation.ParentThreadID(root, sid); got != "" {
		t.Errorf("second call: got %q, want cached empty (negative meta-read cache not wired)", got)
	}
}

// Invalidating the cache must force a fresh walk on the next call.
func TestLocateRollout_InvalidateForcesRewalk(t *testing.T) {
	root := t.TempDir()
	sid := "sid-1"
	target := writeRollout(t, root, "2026", "05", "21", sid, "{}")

	if _, err := conversation.LocateRollout(root, sid); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	conversation.InvalidateCacheForTest()

	got, _ := conversation.LocateRollout(root, sid)
	if got != "" {
		t.Errorf("after invalidate + file removed: got %q, want empty (cache should be cold)", got)
	}
}
