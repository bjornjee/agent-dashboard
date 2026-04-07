package diagrams

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFixture creates a .mmd file in the session's diagram dir with the
// given timestamp and source. Returns the absolute path.
func writeFixture(t *testing.T, stateDir, sessionID string, ts time.Time, source string) string {
	t.Helper()
	dir := Dir(stateDir, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name := Filename(ts, Hash(source))
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(source), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestLoad_EmptyDir(t *testing.T) {
	state := t.TempDir()
	got, err := Load(state, "missing-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestLoad_SortsLatestFirst(t *testing.T) {
	state := t.TempDir()
	sess := "sess-abc"

	writeFixture(t, state, sess, time.Unix(1000, 0), "%% title: First\nflowchart TD\n  A --> B")
	writeFixture(t, state, sess, time.Unix(3000, 0), "%% title: Third\nsequenceDiagram\n  A->>B: x")
	writeFixture(t, state, sess, time.Unix(2000, 0), "%% title: Second\nstateDiagram\n  [*] --> X")

	got, err := Load(state, sess)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 diagrams, got %d", len(got))
	}
	if got[0].Title != "Third" {
		t.Errorf("[0].Title: got %q want %q", got[0].Title, "Third")
	}
	if got[1].Title != "Second" {
		t.Errorf("[1].Title: got %q want %q", got[1].Title, "Second")
	}
	if got[2].Title != "First" {
		t.Errorf("[2].Title: got %q want %q", got[2].Title, "First")
	}
	if got[0].Type != "sequenceDiagram" {
		t.Errorf("[0].Type: got %q", got[0].Type)
	}
}

func TestLoad_IgnoresNonMmdFiles(t *testing.T) {
	state := t.TempDir()
	sess := "sess-xyz"
	dir := Dir(state, sess)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// valid
	writeFixture(t, state, sess, time.Unix(1000, 0), "flowchart TD\n  A --> B")
	// invalid: wrong extension
	os.WriteFile(filepath.Join(dir, "1000-abcd1234.txt"), []byte("nope"), 0o644)
	// invalid: malformed name
	os.WriteFile(filepath.Join(dir, "garbage.mmd"), []byte("nope"), 0o644)

	got, err := Load(state, sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagram, got %d", len(got))
	}
}

func TestExists_ByHash(t *testing.T) {
	state := t.TempDir()
	sess := "sess-1"
	src := "flowchart TD\n  A --> B"
	writeFixture(t, state, sess, time.Unix(1234, 0), src)

	if !Exists(state, sess, Hash(src)) {
		t.Errorf("expected Exists=true for existing hash")
	}
	if Exists(state, sess, "00000000") {
		t.Errorf("expected Exists=false for missing hash")
	}
}

func TestDelete(t *testing.T) {
	state := t.TempDir()
	sess := "sess-del"
	p := writeFixture(t, state, sess, time.Unix(500, 0), "flowchart TD\n  A --> B")

	list, _ := Load(state, sess)
	if len(list) != 1 {
		t.Fatalf("precondition: expected 1, got %d", len(list))
	}
	if err := Delete(list[0]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("expected file removed, err=%v", err)
	}

	// Delete of already-missing file should not error
	if err := Delete(list[0]); err != nil {
		t.Errorf("second delete should be idempotent, got %v", err)
	}
}

func TestCleanupSession(t *testing.T) {
	state := t.TempDir()
	sess := "sess-clean"
	writeFixture(t, state, sess, time.Unix(1, 0), "flowchart TD\n  A")
	writeFixture(t, state, sess, time.Unix(2, 0), "sequenceDiagram\n  A->>B: x")

	if err := CleanupSession(state, sess); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(Dir(state, sess)); !os.IsNotExist(err) {
		t.Errorf("expected session dir removed")
	}

	// Cleanup of missing session should not error
	if err := CleanupSession(state, "never-existed"); err != nil {
		t.Errorf("cleanup of missing session should be idempotent, got %v", err)
	}
}

func TestLoad_ReadsSourceAndDerivedFields(t *testing.T) {
	state := t.TempDir()
	sess := "sess-read"
	src := "%% title: Request Lifecycle\nsequenceDiagram\n  User->>API: GET /foo"
	writeFixture(t, state, sess, time.Unix(1500, 0), src)

	got, err := Load(state, sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	d := got[0]
	if d.SessionID != sess {
		t.Errorf("SessionID: %q", d.SessionID)
	}
	if d.Title != "Request Lifecycle" {
		t.Errorf("Title: %q", d.Title)
	}
	if d.Type != "sequenceDiagram" {
		t.Errorf("Type: %q", d.Type)
	}
	if d.Source != src {
		t.Errorf("Source mismatch")
	}
	if d.Timestamp.Unix() != 1500 {
		t.Errorf("Timestamp: %d", d.Timestamp.Unix())
	}
	if d.Path == "" {
		t.Errorf("Path should be populated")
	}
}
