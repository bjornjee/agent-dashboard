package conversation

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestSessionsIndex_ConcurrentCallersShareSingleWalk asserts that when N
// goroutines call ParentThreadID/FindSubagents concurrently against a cold
// cache, exactly one underlying sessions-tree walk is performed. The
// pre-shared-index implementation did one walk per call (per agent),
// which is the regression we are closing.
func TestSessionsIndex_ConcurrentCallersShareSingleWalk(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	root := mkRolloutRoot(t)

	var walks int64
	orig := walkSessionsRootFn
	walkSessionsRootFn = func(r string, visit func(string, subagentSessionMeta)) {
		atomic.AddInt64(&walks, 1)
		orig(r, visit)
	}
	t.Cleanup(func() { walkSessionsRootFn = orig })

	const goroutines = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				_ = FindSubagents(root, "parent")
			} else {
				_ = ParentThreadID(root, "child")
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt64(&walks); got != 1 {
		t.Errorf("walks = %d, want 1 (single-flight across %d concurrent callers)", got, goroutines)
	}
}

// TestSessionsIndex_OneBuildSharedAcrossLookupKinds asserts that mixing
// FindSubagents and ParentThreadID/LocateRollout calls during the cold
// window all read from the same single index build.
func TestSessionsIndex_OneBuildSharedAcrossLookupKinds(t *testing.T) {
	t.Cleanup(InvalidateCacheForTest)
	root := mkRolloutRoot(t)

	var walks int64
	orig := walkSessionsRootFn
	walkSessionsRootFn = func(r string, visit func(string, subagentSessionMeta)) {
		atomic.AddInt64(&walks, 1)
		orig(r, visit)
	}
	t.Cleanup(func() { walkSessionsRootFn = orig })

	_ = FindSubagents(root, "parent")
	if got := ParentThreadID(root, "child"); got != "parent" {
		t.Errorf("ParentThreadID = %q, want parent", got)
	}
	if p, _ := LocateRollout(root, "child"); p == "" {
		t.Error("LocateRollout returned empty after warm index")
	}

	if got := atomic.LoadInt64(&walks); got != 1 {
		t.Errorf("walks = %d, want 1 (sibling lookups must reuse the shared index)", got)
	}
}

func mkRolloutRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	day := filepath.Join(root, "2026", "05", "21")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(day, "rollout-2026-05-21T00-00-00-child.jsonl")
	contents := `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"worker"}}}}}` + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
