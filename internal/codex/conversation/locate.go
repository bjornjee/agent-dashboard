package conversation

// LocateRollout returns the rollout JSONL path for sessionID under codex's
// per-day directory tree (sessionsRoot/YYYY/MM/DD/rollout-*-<sessionID>.jsonl).
// Returns ("", nil) when the session can't be found or the root doesn't
// exist — codex may not be installed, or the session may have been
// pruned. The error return is preserved for API compatibility (callers
// match on the two-value form) but no error is produced today.
//
// A single session ID can map to multiple rollout files when the user
// runs `codex resume <sid>` across day boundaries (codex writes a new
// rollout under the resume day's YYYY/MM/DD dir, not the original).
// locateRolloutFile applies the "lexicographically greatest path wins"
// rule — since the path embeds YYYY/MM/DD and the rollout filename embeds
// ISO8601, the greatest path is always the most recent.
//
// Resolution goes through the per-session cache so the result is shared
// with ParentThreadID; the underlying walk inspects only directory
// entries (no rollout file is opened), keeping each cold call cheap.
func LocateRollout(sessionsRoot, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	entry, _ := resolveSessionEntry(sessionsRoot, sessionID, false)
	return entry.Path, nil
}
