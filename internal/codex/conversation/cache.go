package conversation

import (
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"golang.org/x/sync/singleflight"
)

// Codex rollout files live flat under sessionsRoot/YYYY/MM/DD/. Parent↔child
// edges and per-session metadata are buried inside each file's session_meta
// line, so resolving any one session requires walking the tree and opening
// many files. Doing that once per caller is too slow when the dashboard
// renders many codex agents.
//
// This cache stores a single index per sessionsRoot, built by one walk:
//   - rollouts maps sessionID → resolved rollout path + parsed meta.
//   - children maps parentSessionID → []SubagentInfo, sorted newest-first.
//
// A singleflight.Group coalesces concurrent rebuilds, so N goroutines that
// arrive together during a cold window share one walk. The index expires
// as a unit; we never have a half-warm view.

const defaultCacheTTL = 15 * time.Second

var nowFunc = time.Now

// rolloutEntry records what we learned about a single rollout file.
//   - Path != "" → file exists; Meta is the parsed session_meta (zero value
//     when the file had no session_meta line yet).
//   - Path == "" → no rollout found for this sessionID in the current
//     index build (negative result; valid until TTL expiry).
type rolloutEntry struct {
	Path string
	Meta subagentSessionMeta
}

type sessionsIndex struct {
	builtAt  time.Time
	rollouts map[string]rolloutEntry          // sessionID → entry
	children map[string][]domain.SubagentInfo // parentSessionID → subs
}

type cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	indexes map[string]*sessionsIndex // sessionsRoot → index
	// group is stored by pointer so InvalidateCacheForTest can swap it
	// atomically under cache.mu without racing against an in-flight Do
	// caller's internal mutex on the previous group value.
	group *singleflight.Group
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		ttl:     ttl,
		indexes: map[string]*sessionsIndex{},
		group:   &singleflight.Group{},
	}
}

// callGroup returns the current singleflight group under the cache lock so
// callers see a stable pointer even when a test invalidates the cache mid-flight.
func (c *cache) callGroup() *singleflight.Group {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.group
}

var pkgCache = newCache(defaultCacheTTL)

// getIndex returns the cached index for root if it is still within TTL.
func (c *cache) getIndex(root string) (*sessionsIndex, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx, ok := c.indexes[root]
	if !ok {
		return nil, false
	}
	if nowFunc().Sub(idx.builtAt) > c.ttl {
		return nil, false
	}
	return idx, true
}

func (c *cache) putIndex(root string, idx *sessionsIndex) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.indexes[root] = idx
}

// InvalidateCacheForTest clears the package-level cache. Tests that exercise
// real codex sessions must call this in t.Cleanup to avoid leaking state
// between subtests.
func InvalidateCacheForTest() {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()
	pkgCache.indexes = map[string]*sessionsIndex{}
	// Pointer swap is race-safe even if a previous Do call is still
	// in-flight on the old group — the old group keeps its own mutex
	// alive until the goroutine finishes; new callers see the fresh one.
	pkgCache.group = &singleflight.Group{}
}
