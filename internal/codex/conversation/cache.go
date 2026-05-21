package conversation

import (
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// codex rollout files are flat under sessionsRoot — the parent↔child link is
// inside each file's session_meta. Discovering it requires walking the tree
// and opening many files, which is too expensive to run on the TUI input
// thread. This package-level cache absorbs repeated lookups within a short
// TTL window so background polling and stateUpdated handling don't replay
// the walk every few seconds.

const defaultCacheTTL = 5 * time.Second

var nowFunc = time.Now

type cacheKey struct {
	root      string
	sessionID string
}

// rolloutEntry has two valid shapes:
//   - Path == ""                 → no rollout found for this sessionID
//   - Path != "" && !MetaRead    → LocateRollout has resolved the path but
//     ParentThreadID has not parsed session_meta yet
//   - Path != "" && MetaRead     → meta read attempt complete (Meta may
//     still be the zero value if the file had no session_meta line)
//
// The MetaRead flag is what lets ParentThreadID short-circuit a re-read
// when LocateRollout warmed the entry first. Without it, Meta.ID == ""
// would be ambiguous between "not read yet" and "read but no meta in
// file" — the latter would re-open the file on every call.
type rolloutEntry struct {
	Path     string
	Meta     subagentSessionMeta
	MetaRead bool
	expires  time.Time
}

type subagentListEntry struct {
	subs    []domain.SubagentInfo
	expires time.Time
}

type cache struct {
	mu                sync.Mutex
	ttl               time.Duration
	rollouts          map[cacheKey]rolloutEntry
	subagentsByParent map[cacheKey]subagentListEntry
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		ttl:               ttl,
		rollouts:          map[cacheKey]rolloutEntry{},
		subagentsByParent: map[cacheKey]subagentListEntry{},
	}
}

var pkgCache = newCache(defaultCacheTTL)

func (c *cache) getRollout(key cacheKey) (rolloutEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.rollouts[key]
	if !ok || nowFunc().After(e.expires) {
		return rolloutEntry{}, false
	}
	return e, true
}

func (c *cache) putRollout(key cacheKey, e rolloutEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.expires = nowFunc().Add(c.ttl)
	c.rollouts[key] = e
}

func (c *cache) getSubagentList(key cacheKey) ([]domain.SubagentInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.subagentsByParent[key]
	if !ok || nowFunc().After(e.expires) {
		return nil, false
	}
	return e.subs, true
}

func (c *cache) putSubagentList(key cacheKey, subs []domain.SubagentInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subagentsByParent[key] = subagentListEntry{
		subs:    subs,
		expires: nowFunc().Add(c.ttl),
	}
}

// InvalidateCacheForTest clears the package-level cache. Tests that exercise
// real codex sessions must call this in t.Cleanup to avoid leaking state
// between subtests.
func InvalidateCacheForTest() {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()
	pkgCache.rollouts = map[cacheKey]rolloutEntry{}
	pkgCache.subagentsByParent = map[cacheKey]subagentListEntry{}
}
