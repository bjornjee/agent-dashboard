package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// twoLineUsage produces two distinct assistant turns totaling
// (300 input, 150 output, 3000 cache_read, 500 cache_write).
const twoLineUsage = `{"type":"assistant","message":{"id":"msg_a","model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200}},"timestamp":"2026-03-28T10:00:00Z"}
{"type":"assistant","message":{"id":"msg_b","model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_read_input_tokens":2000,"cache_creation_input_tokens":300}},"timestamp":"2026-03-28T10:01:00Z"}
`

const oneMoreLineUsage = `{"type":"assistant","message":{"id":"msg_c","model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":50,"output_tokens":25,"cache_read_input_tokens":500,"cache_creation_input_tokens":100}},"timestamp":"2026-03-28T10:02:00Z"}
`

func TestReadUsage_CacheHit_NoFileChange_ReturnsCached(t *testing.T) {
	t.Cleanup(InvalidateUsageCacheForTest)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sess.jsonl")
	if err := os.WriteFile(path, []byte(twoLineUsage), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := ReadUsage(tmp, "sess")
	if first.InputTokens != 300 {
		t.Fatalf("first read InputTokens = %d, want 300", first.InputTokens)
	}

	// Poke the cached entry with a sentinel. If ReadUsage consults the
	// cache before opening the file, the sentinel comes back unchanged;
	// if it re-reads the file, the real 300 input tokens come back.
	key := tmp + ":sess"
	usageCache.mu.Lock()
	entry := usageCache.entries[key]
	entry.usage = domain.Usage{InputTokens: 99999}
	usageCache.entries[key] = entry
	usageCache.mu.Unlock()

	second := ReadUsage(tmp, "sess")
	if second.InputTokens != 99999 {
		t.Errorf("second read InputTokens = %d, want 99999 (cached) — cache was not consulted",
			second.InputTokens)
	}
}

func TestReadUsage_IncrementalResume_AfterAppend(t *testing.T) {
	t.Cleanup(InvalidateUsageCacheForTest)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sess.jsonl")
	if err := os.WriteFile(path, []byte(twoLineUsage), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := ReadUsage(tmp, "sess")
	if first.InputTokens != 300 {
		t.Fatalf("first read InputTokens = %d, want 300", first.InputTokens)
	}

	// Append a third line. Cumulative should be 300+50=350 in, 150+25=175 out.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("append open: %v", err)
	}
	if _, err := f.WriteString(oneMoreLineUsage); err != nil {
		t.Fatalf("append write: %v", err)
	}
	f.Close()

	resumed := ReadUsage(tmp, "sess")
	if resumed.InputTokens != 350 {
		t.Errorf("resumed InputTokens = %d, want 350", resumed.InputTokens)
	}
	if resumed.OutputTokens != 175 {
		t.Errorf("resumed OutputTokens = %d, want 175", resumed.OutputTokens)
	}
	if resumed.CacheReadTokens != 3500 {
		t.Errorf("resumed CacheReadTokens = %d, want 3500", resumed.CacheReadTokens)
	}

	// Cross-check: a fresh full read (after invalidation) must produce
	// the same totals — proves incremental resume didn't lose or double-count.
	InvalidateUsageCacheForTest()
	fresh := ReadUsage(tmp, "sess")
	if fresh != resumed {
		t.Errorf("incremental result diverged from fresh full read:\n  resumed=%+v\n  fresh=  %+v", resumed, fresh)
	}
}

func TestReadUsage_FullRescanOnShrink(t *testing.T) {
	t.Cleanup(InvalidateUsageCacheForTest)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sess.jsonl")
	if err := os.WriteFile(path, []byte(twoLineUsage), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := ReadUsage(tmp, "sess")
	if first.InputTokens != 300 {
		t.Fatalf("first read InputTokens = %d, want 300", first.InputTokens)
	}

	// Truncate-and-rewrite with smaller content (just the first line).
	smaller := `{"type":"assistant","message":{"id":"msg_only","model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":7,"output_tokens":3,"cache_read_input_tokens":11,"cache_creation_input_tokens":13}},"timestamp":"2026-03-28T11:00:00Z"}
`
	if err := os.WriteFile(path, []byte(smaller), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	second := ReadUsage(tmp, "sess")
	if second.InputTokens != 7 {
		t.Errorf("after shrink InputTokens = %d, want 7 (full rescan, not stale-cached 300)", second.InputTokens)
	}
	if second.OutputTokens != 3 {
		t.Errorf("after shrink OutputTokens = %d, want 3", second.OutputTokens)
	}
}

func TestReadUsage_SameSizeNewMtime_TriggersFullRescan(t *testing.T) {
	t.Cleanup(InvalidateUsageCacheForTest)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sess.jsonl")

	// Write the original two-line content (sums to 300 input).
	if err := os.WriteFile(path, []byte(twoLineUsage), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if u := ReadUsage(tmp, "sess"); u.InputTokens != 300 {
		t.Fatalf("first read InputTokens = %d, want 300", u.InputTokens)
	}

	// Replace with different content of the same byte count (rotation /
	// in-place rewrite). Verify length matches before bumping mtime —
	// otherwise the test isn't exercising the same-size code path.
	replacement := makeUsageOfSameLength(t, twoLineUsage)
	if len(replacement) != len(twoLineUsage) {
		t.Fatalf("replacement length = %d, want %d (test setup bug)", len(replacement), len(twoLineUsage))
	}
	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// Force a different mtime (Go on macOS gives nanosecond resolution but
	// back-to-back writes can still collide).
	future := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got := ReadUsage(tmp, "sess")
	// The replacement is a single 7-input-token entry repeated as needed
	// to fill the original byte count — exact count comes from
	// makeUsageOfSameLength. We assert NOT the stale 300.
	if got.InputTokens == 300 {
		t.Errorf("InputTokens = 300 (stale cached value) — same-size/different-mtime did NOT trigger full rescan")
	}
}

// makeUsageOfSameLength returns JSONL content with a different token
// payload than twoLineUsage but the same total byte count, so the
// usage cache cannot distinguish on size alone.
func makeUsageOfSameLength(t *testing.T, ref string) string {
	t.Helper()
	// One short line shaped to be padded with whitespace to reach the
	// target length. The padding goes inside the message.id (claude
	// accepts any string), so the JSON stays valid.
	const prefix = `{"type":"assistant","message":{"id":"`
	const suffix = `","model":"claude-haiku-4-5","role":"assistant","content":[],"usage":{"input_tokens":7,"output_tokens":3,"cache_read_input_tokens":11,"cache_creation_input_tokens":13}},"timestamp":"2026-03-28T11:00:00Z"}` + "\n"
	target := len(ref)
	padLen := target - len(prefix) - len(suffix)
	if padLen < 0 {
		t.Fatalf("ref too short for padding scheme: %d chars", target)
	}
	pad := make([]byte, padLen)
	for i := range pad {
		pad[i] = 'x'
	}
	return prefix + string(pad) + suffix
}

func TestReadUsage_DedupSurvivesIncrementalResume(t *testing.T) {
	t.Cleanup(InvalidateUsageCacheForTest)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sess.jsonl")
	if err := os.WriteFile(path, []byte(twoLineUsage), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := ReadUsage(tmp, "sess")
	if first.InputTokens != 300 {
		t.Fatalf("first read InputTokens = %d, want 300", first.InputTokens)
	}

	// Append a duplicate of msg_a (same message.id). Dedup must persist
	// across the resume — otherwise the input bumps by another 100.
	dupLine := `{"type":"assistant","message":{"id":"msg_a","model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200}},"timestamp":"2026-03-28T10:00:00Z"}
`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("append open: %v", err)
	}
	if _, err := f.WriteString(dupLine); err != nil {
		t.Fatalf("append: %v", err)
	}
	f.Close()

	second := ReadUsage(tmp, "sess")
	if second.InputTokens != 300 {
		t.Errorf("second InputTokens = %d, want 300 (duplicate msg_a must be deduped across resume)", second.InputTokens)
	}
}
