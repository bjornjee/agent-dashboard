// Package index reads codex's session metadata from
// ~/.codex/session_index.jsonl — an append-only file (one JSON object per
// line) maintained by codex CLI 0.130.0. Each entry carries enough to
// populate the dashboard's agent-list row without parsing the full
// per-session rollout: id, thread_name (human-readable title), updated_at.
//
// Why not SQLite? Earlier codex versions used a state_5.sqlite with a
// threads table (the cmux project references this), but codex 0.130.0
// stores session metadata in this flat JSONL file. logs_2.sqlite at the
// same path holds debug logs only — not session state.
package index

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
)

// Session is a single entry from session_index.jsonl.
type Session struct {
	ID         string `json:"id"`
	ThreadName string `json:"thread_name"`
	UpdatedAt  string `json:"updated_at"`
}

// Reader scans codex's session_index.jsonl on demand. Construct one per
// codex root via NewReader; the reader holds the path only, no open file
// handle, so it's safe to share across goroutines.
type Reader struct {
	path string
}

// NewReader returns a Reader bound to the given session_index.jsonl path
// (typically ~/.codex/session_index.jsonl).
func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// ListSessions returns every well-formed entry in the index file, sorted
// by UpdatedAt descending (most-recent first). A missing file returns an
// empty slice and nil error — the dashboard starts before codex has
// produced any session. Malformed lines are silently skipped: codex
// appends without locking, so a partial write at the tail must not crash
// session discovery.
func (r *Reader) ListSessions() ([]Session, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Session
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s Session
		if json.Unmarshal(line, &s) != nil {
			continue
		}
		if s.ID == "" {
			continue
		}
		out = append(out, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
