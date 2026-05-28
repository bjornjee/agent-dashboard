package usage

import (
	"os"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// usageCacheEntry stores one session's incremental scan state. ReadUsage
// resumes from offset on the next call if the file's (size, mtime) shows
// it grew; on a shrink (truncate/rewrite) it falls back to a full rescan
// from offset 0.
//
// seenIDs is the message.id dedup set built by previous scans. ReadUsage
// must clone this on read because subsequent calls mutate it as new lines
// are processed; the cache is owned by `usageCache` and shared across
// goroutines (loadUsage runs on a bubbletea goroutine, but a future
// caller could parallelize per agent).
type usageCacheEntry struct {
	usage   domain.Usage
	seenIDs map[string]struct{}
	offset  int64
	size    int64
	mtime   time.Time
}

type usageCacheHit int

const (
	usageCacheMiss   usageCacheHit = iota // no cache or file shrank — full rescan from 0
	usageCacheFull                        // file unchanged — return cached usage
	usageCacheResume                      // file grew — seek to offset, scan only new bytes
)

type usageCacheT struct {
	mu      sync.Mutex
	entries map[string]usageCacheEntry
}

var usageCache = &usageCacheT{entries: map[string]usageCacheEntry{}}

// statFn lets tests override how ReadUsage stats the file (e.g. to count
// calls or simulate errors). Production uses os.Stat.
var statFn = os.Stat

// get returns a clone of the cached entry plus a hit mode telling the
// caller whether to return the cached usage as-is, resume scanning from
// the cached offset, or perform a full rescan.
//
// Cloning seenIDs keeps the cache immutable from the caller's perspective —
// the caller may freely mutate its copy.
func (c *usageCacheT) get(key string, currentSize int64, currentMtime time.Time) (usageCacheEntry, usageCacheHit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return usageCacheEntry{}, usageCacheMiss
	}
	if currentSize < entry.size {
		// File shrank (truncate / rewrite) — cache is stale; force full rescan.
		return usageCacheEntry{}, usageCacheMiss
	}
	if currentSize == entry.size && entry.mtime.Equal(currentMtime) {
		return cloneUsageEntry(entry), usageCacheFull
	}
	return cloneUsageEntry(entry), usageCacheResume
}

func (c *usageCacheT) put(key string, entry usageCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry
}

func cloneUsageEntry(e usageCacheEntry) usageCacheEntry {
	cloned := e
	cloned.seenIDs = make(map[string]struct{}, len(e.seenIDs))
	for k := range e.seenIDs {
		cloned.seenIDs[k] = struct{}{}
	}
	return cloned
}

// InvalidateUsageCacheForTest clears the package-level usage cache.
// Tests that exercise ReadUsage must call this in t.Cleanup to avoid
// leaking state between subtests.
func InvalidateUsageCacheForTest() {
	usageCache.mu.Lock()
	defer usageCache.mu.Unlock()
	usageCache.entries = map[string]usageCacheEntry{}
}
