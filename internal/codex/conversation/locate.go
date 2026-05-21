package conversation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LocateRollout returns the rollout JSONL path for sessionID under codex's
// per-day directory tree (sessionsRoot/YYYY/MM/DD/rollout-*-<sessionID>.jsonl).
// Returns ("", nil) when the session can't be found or the root doesn't
// exist — codex may not be installed, or the session may have been
// pruned. Errors only surface for unexpected filesystem failures.
//
// A single session ID can map to multiple rollout files when the user
// runs `codex resume <sid>` across day boundaries (codex writes a new
// rollout under the resume day's YYYY/MM/DD dir, not the original).
// LocateRollout returns the lexicographically greatest matching path —
// since the path embeds YYYY/MM/DD and the rollout filename embeds
// ISO8601, the greatest path is always the most recent.
func LocateRollout(sessionsRoot, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	key := cacheKey{root: sessionsRoot, sessionID: sessionID}
	if entry, ok := pkgCache.getRollout(key); ok {
		return entry.Path, nil
	}
	if _, err := os.Stat(sessionsRoot); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// codex isn't installed yet — cache the negative so the next
			// TopLevelAgents tick doesn't replay this stat.
			pkgCache.putRollout(key, rolloutEntry{Path: "", MetaRead: true})
			return "", nil
		}
		return "", err
	}

	suffix := "-" + sessionID + ".jsonl"
	var newest string
	walkErr := filepath.WalkDir(sessionsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // ignore unreadable subtrees; the file might still be elsewhere
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, suffix) && path > newest {
			newest = path
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	// For a missing session (newest == ""), MetaRead=true short-circuits any
	// subsequent ParentThreadID call too — no file to open.
	pkgCache.putRollout(key, rolloutEntry{Path: newest, MetaRead: newest == ""})
	return newest, nil
}
