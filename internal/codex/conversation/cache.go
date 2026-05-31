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
//     The sessions cache is independent of the sessionsIndex: ParentThreadID
//     and LocateRollout do not touch the index.
//
// Both caches share a TTL so behaviour stays consistent — within the
// window, repeated lookups for the same sessionID hit in O(1).

const defaultCacheTTL = 15 * time.Second

var nowFunc = time.Now

type sessionsIndex struct {
	builtAt  time.Time
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
	builtAt    time.Time
	Path       string
	Parent     string
	Originator string
	MetaRead   bool
}

// sessionKey scopes per-session cache entries by sessionsRoot so two
// independent codex installations (rare; common in tests) cannot collide.
type sessionKey struct {
	root      string
	sessionID string
}

// metaCacheEntry caches one rollout's session_meta keyed by file mtime.
// session_meta is the first line of a codex rollout, written once at
// session creation, so an mtime match means the cached value is still
// correct. ok=false is also cached so we don't re-read malformed files.
type metaCacheEntry struct {
	mtime time.Time
	meta  subagentSessionMeta
	ok    bool
}

// detailsCacheEntry caches one rollout's parsed details keyed by mtime.
// Rollouts are append-only, so any meaningful change shows up as an
// mtime bump.
type detailsCacheEntry struct {
	mtime   time.Time
	details subagentRolloutDetails
}

type cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	indexes  map[string]*sessionsIndex // sessionsRoot → index
	sessions map[sessionKey]sessionEntry
	metas    map[string]metaCacheEntry    // rollout path → session_meta
	details  map[string]detailsCacheEntry // rollout path → rollout details
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
		metas:    map[string]metaCacheEntry{},
		details:  map[string]detailsCacheEntry{},
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

// getMetaForPath returns the cached session_meta for path when mtime
// matches the cached value exactly. Mismatch or absence returns ok=false.
// Mtime is the invalidation signal — there is no TTL because session_meta
// is the first line of a rollout and codex never rewrites it.
func (c *cache) getMetaForPath(path string, mtime time.Time) (subagentSessionMeta, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.metas[path]
	if !ok || !entry.mtime.Equal(mtime) {
		return subagentSessionMeta{}, false, false
	}
	return entry.meta, entry.ok, true
}

func (c *cache) putMetaForPath(path string, mtime time.Time, meta subagentSessionMeta, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metas[path] = metaCacheEntry{mtime: mtime, meta: meta, ok: ok}
}

// getDetailsForPath returns the cached rollout details when mtime matches.
// An mtime bump means the rollout has been appended to and details
// (completion flag, instruction head, mode) need to be re-derived.
func (c *cache) getDetailsForPath(path string, mtime time.Time) (subagentRolloutDetails, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.details[path]
	if !ok || !entry.mtime.Equal(mtime) {
		return subagentRolloutDetails{}, false
	}
	return entry.details, true
}

func (c *cache) putDetailsForPath(path string, mtime time.Time, details subagentRolloutDetails) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.details[path] = detailsCacheEntry{mtime: mtime, details: details}
}

// InvalidateCacheForTest clears the package-level cache. Tests that exercise
// real codex sessions must call this in t.Cleanup to avoid leaking state
// between subtests.
func InvalidateCacheForTest() {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()
	pkgCache.indexes = map[string]*sessionsIndex{}
	pkgCache.sessions = map[sessionKey]sessionEntry{}
	pkgCache.metas = map[string]metaCacheEntry{}
	pkgCache.details = map[string]detailsCacheEntry{}
	// Pointer swap is race-safe even if a previous Do call is still
	// in-flight on the old group — the old group keeps its own mutex
	// alive until the goroutine finishes; new callers see the fresh one.
	pkgCache.group = &singleflight.Group{}
}
