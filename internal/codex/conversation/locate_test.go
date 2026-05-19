package conversation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/conversation"
)

// LocateRollout finds the rollout JSONL for a given session under codex's
// year/month/day directory tree:
//
//	<sessionsRoot>/YYYY/MM/DD/rollout-<ISO8601>-<sessionID>.jsonl
//
// codex CLI 0.130.0 — verified by inspection of ~/.codex/sessions/.
// We don't depend on the timestamp prefix; the sessionID suffix is the
// stable handle.
func TestLocateRollout_FindsMatchingFile(t *testing.T) {
	root := t.TempDir()
	day := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "019e39d0-581e-7f81-beb0-ea3c04ffa2e4"
	target := filepath.Join(day, "rollout-2026-05-18T14-40-15-"+sid+".jsonl")
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Decoy on a different day with a different sid.
	other := filepath.Join(root, "2026", "05", "17")
	_ = os.MkdirAll(other, 0o755)
	_ = os.WriteFile(filepath.Join(other, "rollout-2026-05-17T01-01-01-019aaaaa.jsonl"), []byte("{}"), 0o644)

	got, err := conversation.LocateRollout(root, sid)
	if err != nil {
		t.Fatalf("LocateRollout err = %v", err)
	}
	if got != target {
		t.Errorf("got %q, want %q", got, target)
	}
}

// When `codex resume <sid>` is run across a day boundary, codex writes a
// new rollout file under the resume day's YYYY/MM/DD dir using the same
// session UUID. LocateRollout must return the NEWER file (lexicographic
// max of the full path — YYYY/MM/DD embedding makes lex order == time
// order). Regression guard against the original "first match wins"
// implementation which returned the oldest day.
func TestLocateRollout_PicksNewestAcrossResume(t *testing.T) {
	root := t.TempDir()
	sid := "019e39d0-581e-7f81-beb0-ea3c04ffa2e4"

	// Original session, day 1.
	day1 := filepath.Join(root, "2026", "05", "18")
	_ = os.MkdirAll(day1, 0o755)
	day1Path := filepath.Join(day1, "rollout-2026-05-18T14-40-15-"+sid+".jsonl")
	_ = os.WriteFile(day1Path, []byte("{}"), 0o644)

	// Resume next day, same sid → new rollout file.
	day2 := filepath.Join(root, "2026", "05", "19")
	_ = os.MkdirAll(day2, 0o755)
	day2Path := filepath.Join(day2, "rollout-2026-05-19T09-15-00-"+sid+".jsonl")
	_ = os.WriteFile(day2Path, []byte("{}"), 0o644)

	got, err := conversation.LocateRollout(root, sid)
	if err != nil {
		t.Fatalf("LocateRollout err = %v", err)
	}
	if got != day2Path {
		t.Errorf("got %q, want newest (day 2) %q", got, day2Path)
	}
}

// Missing session → "" with no error so callers can render "no transcript
// yet" cleanly rather than crashing.
func TestLocateRollout_MissingSession(t *testing.T) {
	got, err := conversation.LocateRollout(t.TempDir(), "nope")
	if err != nil {
		t.Fatalf("LocateRollout err = %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// Missing sessions root → "" with no error. Codex may not be installed
// (yet) when the dashboard boots.
func TestLocateRollout_MissingRoot(t *testing.T) {
	got, err := conversation.LocateRollout(filepath.Join(t.TempDir(), "does-not-exist"), "anything")
	if err != nil {
		t.Fatalf("LocateRollout err = %v, want nil for missing root", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
