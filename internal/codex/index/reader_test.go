package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/index"
)

// session_index.jsonl is an append-only flat file at the root of ~/.codex
// (codex CLI 0.130.0 — verified by inspection). Each line is a JSON object
// with at least {id, thread_name, updated_at}. Real example line:
//
//	{"id":"019d752a-987c-77a2-b7d7-b011567ead07",
//	 "thread_name":"Codex Companion Task: ...",
//	 "updated_at":"2026-04-10T02:13:39.090105Z"}
func TestReader_ListSessions_ReadsValidEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	contents := `{"id":"019d752a-987c-77a2-b7d7-b011567ead07","thread_name":"first","updated_at":"2026-04-10T02:13:39.090105Z"}
{"id":"019d760a-d57a-7561-9d83-0a2e15a48826","thread_name":"second","updated_at":"2026-04-10T06:18:34.755129Z"}
{"id":"019d7677-2ddb-7662-bd03-eedf34c124be","thread_name":"third","updated_at":"2026-04-10T08:16:55.264909Z"}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	r := index.NewReader(path)
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() err = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d sessions, want 3", len(got))
	}

	// Sorted by updated_at descending — most-recent first, since the
	// dashboard agent list always renders newest at top.
	if got[0].ID != "019d7677-2ddb-7662-bd03-eedf34c124be" {
		t.Errorf("got[0].ID = %q, want third (newest)", got[0].ID)
	}
	if got[2].ID != "019d752a-987c-77a2-b7d7-b011567ead07" {
		t.Errorf("got[2].ID = %q, want first (oldest)", got[2].ID)
	}
	if got[1].ThreadName != "second" {
		t.Errorf("got[1].ThreadName = %q, want %q", got[1].ThreadName, "second")
	}
}

// Malformed lines are skipped (not fatal) — the file is append-only and a
// partial write at the tail must not crash session discovery.
func TestReader_ListSessions_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	contents := `{"id":"a","thread_name":"good","updated_at":"2026-04-10T02:13:39Z"}
not-json
{"id":"b","thread_name":"also-good","updated_at":"2026-04-11T02:13:39Z"}
`
	_ = os.WriteFile(path, []byte(contents), 0o644)

	r := index.NewReader(path)
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (malformed line skipped)", len(got))
	}
}

// Missing file → empty slice, nil error. The dashboard starts before codex
// has written any session and must not fail at boot.
func TestReader_ListSessions_MissingFile(t *testing.T) {
	r := index.NewReader(filepath.Join(t.TempDir(), "nope.jsonl"))
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() err = %v, want nil for missing file", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0 for missing file", len(got))
	}
}

// Empty file → empty slice, nil error.
func TestReader_ListSessions_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	_ = os.WriteFile(path, []byte(""), 0o644)

	r := index.NewReader(path)
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0 for empty file", len(got))
	}
}

// Blank lines (trailing newline or stray gaps) are skipped without error.
func TestReader_ListSessions_BlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	contents := "\n\n{\"id\":\"a\",\"thread_name\":\"x\",\"updated_at\":\"2026-04-10T02:13:39Z\"}\n\n"
	_ = os.WriteFile(path, []byte(contents), 0o644)

	r := index.NewReader(path)
	got, err := r.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() err = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d entries, want 1", len(got))
	}
}
