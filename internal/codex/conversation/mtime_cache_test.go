package conversation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRolloutFile writes a rollout JSONL at a fixed path and returns it.
// Tests below need the file path independent of the YYYY/MM/DD layout the
// external writeRollout helper enforces, so we use a flat temp dir.
func writeRolloutFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
}

const metaLineForCacheTests = `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"sid-cached","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-real","agent_nickname":"RealNick","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"RealNick","agent_role":"explorer"}}
{"timestamp":"2026-05-21T14:44:03.700Z","type":"event_msg","payload":{"type":"task_started"}}
`

func TestReadSubagentSessionMeta_CacheHitForMatchingMtime(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	dir := t.TempDir()
	path := writeRolloutFile(t, dir, "rollout-real.jsonl", metaLineForCacheTests)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Pre-populate the cache with a sentinel that the real file does NOT
	// contain. If readSubagentSessionMeta consults the cache before
	// reading, the sentinel comes back; if it re-reads the file, the
	// real ParentThreadID comes back.
	sentinel := subagentSessionMeta{ID: "sid-cached"}
	sentinel.Source.Subagent.ThreadSpawn.ParentThreadID = "parent-cached"
	pkgCache.putMetaForPath(path, info.ModTime(), sentinel, true)

	got, ok := readSubagentSessionMeta(path)
	if !ok {
		t.Fatal("expected ok=true from cache, got false")
	}
	if got.Source.Subagent.ThreadSpawn.ParentThreadID != "parent-cached" {
		t.Errorf("ParentThreadID = %q, want %q (cached) — function did not consult mtime cache",
			got.Source.Subagent.ThreadSpawn.ParentThreadID, "parent-cached")
	}
}

func TestReadSubagentSessionMeta_CacheMissAfterMtimeChange(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	dir := t.TempDir()
	path := writeRolloutFile(t, dir, "rollout-real.jsonl", metaLineForCacheTests)

	// Cache a stale sentinel at an old mtime.
	staleSentinel := subagentSessionMeta{ID: "sid-cached"}
	staleSentinel.Source.Subagent.ThreadSpawn.ParentThreadID = "parent-stale"
	pkgCache.putMetaForPath(path, time.Unix(1, 0), staleSentinel, true)

	// Bump file mtime to a different value than the cached mtime.
	newTime := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(path, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, ok := readSubagentSessionMeta(path)
	if !ok {
		t.Fatal("expected ok=true from fresh read, got false")
	}
	if got.Source.Subagent.ThreadSpawn.ParentThreadID != "parent-real" {
		t.Errorf("ParentThreadID = %q, want %q (fresh read) — mtime change did not invalidate cache",
			got.Source.Subagent.ThreadSpawn.ParentThreadID, "parent-real")
	}
}

func TestReadSubagentSessionMeta_MissingFile_ReturnsFalseDoesNotCache(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-rollout.jsonl")

	_, ok := readSubagentSessionMeta(missing)
	if ok {
		t.Fatal("expected ok=false for missing file")
	}
	if _, _, hit := pkgCache.getMetaForPath(missing, time.Time{}); hit {
		t.Error("missing file should not populate cache")
	}
}

const rolloutWithDetails = `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"sid-details","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-real","agent_role":"explorer"}}},"agent_role":"explorer"}}
{"timestamp":"2026-05-21T14:44:04.000Z","type":"event_msg","payload":{"type":"user_message","message":"real instruction line"}}
{"timestamp":"2026-05-21T14:44:05.000Z","type":"event_msg","payload":{"type":"task_complete"}}
`

func TestReadSubagentRolloutDetails_CacheHitForMatchingMtime(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	dir := t.TempDir()
	path := writeRolloutFile(t, dir, "rollout-real.jsonl", rolloutWithDetails)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Sentinel: instruction head the file does NOT contain. A hit returns it;
	// a miss returns the real "real instruction line" parsed from the file.
	pkgCache.putDetailsForPath(path, info.ModTime(), subagentRolloutDetails{
		InstructionHead: "from cache",
		Mode:            "from cache",
		Completed:       false,
	})

	got := readSubagentRolloutDetails(path)
	if got.InstructionHead != "from cache" {
		t.Errorf("InstructionHead = %q, want %q (cached) — function did not consult mtime cache",
			got.InstructionHead, "from cache")
	}
}

func TestReadSubagentRolloutDetails_CacheMissAfterMtimeChange(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	dir := t.TempDir()
	path := writeRolloutFile(t, dir, "rollout-real.jsonl", rolloutWithDetails)

	pkgCache.putDetailsForPath(path, time.Unix(1, 0), subagentRolloutDetails{
		InstructionHead: "stale",
		Mode:            "stale",
	})
	newTime := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(path, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got := readSubagentRolloutDetails(path)
	if got.InstructionHead != "real instruction line" {
		t.Errorf("InstructionHead = %q, want %q (fresh) — mtime change did not invalidate cache",
			got.InstructionHead, "real instruction line")
	}
	if !got.Completed {
		t.Error("Completed = false; want true (rollout contains task_complete)")
	}
}
