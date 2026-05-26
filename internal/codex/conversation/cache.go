package conversation

import (
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"golang.org/x/sync/singleflight"
)

// Codex rollout files live flat under sessionsRoot/YYYY/MM/DD/. Parent↔child
// edges and per-session metadata are buried inside each file's session_meta
// line. Two caches live here, scoped to different access patterns:
//
//  1. sessionsIndex — built by one walk per sessionsRoot. Stores the
//     parentSessionID → []SubagentInfo mapping needed by FindSubagents,
//     which is fundamentally a fan-out query (one parent, many children).
//     The build walks every rollout and opens each file twice; FindSubagents
//     runs in a goroutine (loadAllSubagents every 5s), so paying the cost
//     once per TTL window is acceptable. A singleflight.Group coalesces
//     concurrent rebuilds so N goroutines arriving together share one walk.
//
//  2. sessions (per sessionEntry) — per-(root, sessionID) cache used by
//     ParentThreadID and LocateRollout. TopLevelAgents calls ParentThreadID
//     for every codex agent on every stateUpdatedMsg on the main bubbletea
//     goroutine; routing those calls through (1) would freeze the UI for
//     the duration of the walk on every TTL expiry. Per-session lookups
//     locate the rollout by directory entries only (no file opens for
//     unrelated rollouts) and open exactly the matching file.
//
// Both caches share a TTL so behaviour stays consistent — within the
// window, repeated lookups for the same sessionID hit in O(1).

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

// sessionEntry is the per-(root, sessionID) lookup record used by
// ParentThreadID and LocateRollout. The shared sessionsIndex above is
// retained for FindSubagents — which legitimately needs the parent→child
// mapping — but per-session callers (TopLevelAgents on every
// stateUpdatedMsg) must not pay the cost of opening every rollout file.
//
//   - Path != "" → rollout located by directory-only walk (no file open).
//   - MetaRead == true → session_meta was read for this rollout; Parent
//     holds the parsed parent_thread_id ("" when the session is top-level).
//   - Path == "" or MetaRead == false → negative result; valid until TTL.
type sessionEntry struct {
	builtAt  time.Time
	Path     string
	Parent   string
	MetaRead bool
}

// sessionKey scopes per-session cache entries by sessionsRoot so two
// independent codex installations (rare; common in tests) cannot collide.
type sessionKey struct {
	root      string
	sessionID string
}

type cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	indexes  map[string]*sessionsIndex // sessionsRoot → index
	sessions map[sessionKey]sessionEntry
	// group is stored by pointer so InvalidateCacheForTest can swap it
	// atomically under cache.mu without racing against an in-flight Do
	// caller's internal mutex on the previous group value.
	group *singleflight.Group
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		ttl:      ttl,
		indexes:  map[string]*sessionsIndex{},
		sessions: map[sessionKey]sessionEntry{},
		group:    &singleflight.Group{},
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

// getSession returns the cached per-session entry if it is still within
// TTL. The (root, sessionID) tuple keys the entry; a missing or expired
// entry returns ok=false.
func (c *cache) getSession(root, sessionID string) (sessionEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.sessions[sessionKey{root: root, sessionID: sessionID}]
	if !ok {
		return sessionEntry{}, false
	}
	if nowFunc().Sub(entry.builtAt) > c.ttl {
		return sessionEntry{}, false
	}
	return entry, true
}

func (c *cache) putSession(root, sessionID string, entry sessionEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry.builtAt.IsZero() {
		entry.builtAt = nowFunc()
	}
	c.sessions[sessionKey{root: root, sessionID: sessionID}] = entry
}

// InvalidateCacheForTest clears the package-level cache. Tests that exercise
// real codex sessions must call this in t.Cleanup to avoid leaking state
// between subtests.
func InvalidateCacheForTest() {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()
	pkgCache.indexes = map[string]*sessionsIndex{}
	pkgCache.sessions = map[sessionKey]sessionEntry{}
	// Pointer swap is race-safe even if a previous Do call is still
	// in-flight on the old group — the old group keeps its own mutex
	// alive until the goroutine finishes; new callers see the fresh one.
	pkgCache.group = &singleflight.Group{}
}
