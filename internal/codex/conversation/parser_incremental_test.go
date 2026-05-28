package conversation_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/conversation"
)

const codexInitialRollout = `{"timestamp":"2026-04-02T09:57:22.377Z","type":"session_meta","payload":{"id":"sid","timestamp":"2026-04-02T09:57:22Z","cwd":"/repo"}}
{"timestamp":"2026-04-02T09:57:22.500Z","type":"event_msg","payload":{"type":"user_message","message":"first user turn"}}
{"timestamp":"2026-04-02T09:57:24.000Z","type":"event_msg","payload":{"type":"agent_message","message":"first assistant turn"}}
`

const codexAppendedTurns = `{"timestamp":"2026-04-02T09:57:30.000Z","type":"event_msg","payload":{"type":"user_message","message":"second user turn"}}
{"timestamp":"2026-04-02T09:57:32.000Z","type":"event_msg","payload":{"type":"agent_message","message":"second assistant turn"}}
`

func TestReadIncremental_ResumeAfterAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexInitialRollout), 0o644); err != nil {
		t.Fatal(err)
	}

	first, offset1, err := conversation.ReadIncremental(path, 0, nil, 0)
	if err != nil {
		t.Fatalf("first ReadIncremental: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first read: %d entries, want 2", len(first))
	}
	if offset1 == 0 {
		t.Fatal("first offset should be > 0 (file has content)")
	}

	// Append two more turns.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(codexAppendedTurns); err != nil {
		t.Fatal(err)
	}
	f.Close()

	resumed, offset2, err := conversation.ReadIncremental(path, 0, first, offset1)
	if err != nil {
		t.Fatalf("resumed ReadIncremental: %v", err)
	}
	if len(resumed) != 4 {
		t.Errorf("resumed read: %d entries, want 4 (2 prev + 2 new)", len(resumed))
	}
	if offset2 <= offset1 {
		t.Errorf("resumed offset = %d, want > %d (file grew)", offset2, offset1)
	}

	// Cross-check: a fresh full read must produce the same entries.
	fresh, _, err := conversation.ReadIncremental(path, 0, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed, fresh) {
		t.Errorf("incremental result diverged from fresh full read:\n  resumed=%+v\n  fresh=  %+v", resumed, fresh)
	}
}

func TestReadIncremental_NoChange_ReturnsPrevAsIs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexInitialRollout), 0o644); err != nil {
		t.Fatal(err)
	}

	first, offset1, err := conversation.ReadIncremental(path, 0, nil, 0)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Marker mutation in the prev slice — if ReadIncremental copies and
	// returns a fresh slice, this mutation should not affect the result;
	// if it short-circuits and returns prev as-is on no-change, we expect
	// the same slice back (the implementation's chosen behaviour). Either
	// way the content must match.
	second, offset2, err := conversation.ReadIncremental(path, 0, first, offset1)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if offset2 != offset1 {
		t.Errorf("offset2 = %d, want %d (no change)", offset2, offset1)
	}
	if !reflect.DeepEqual(second, first) {
		t.Errorf("no-change second read diverged:\n  first=%+v\n  second=%+v", first, second)
	}
}

func TestReadIncremental_ShrinkFallsBackToFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexInitialRollout+codexAppendedTurns), 0o644); err != nil {
		t.Fatal(err)
	}

	first, offset1, err := conversation.ReadIncremental(path, 0, nil, 0)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if len(first) != 4 {
		t.Fatalf("first: %d entries, want 4", len(first))
	}

	// Shrink: rewrite to just the initial content (less data).
	if err := os.WriteFile(path, []byte(codexInitialRollout), 0o644); err != nil {
		t.Fatal(err)
	}

	second, offset2, err := conversation.ReadIncremental(path, 0, first, offset1)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(second) != 2 {
		t.Errorf("after shrink: %d entries, want 2 (full rescan, not the stale 4)", len(second))
	}
	if offset2 >= offset1 {
		t.Errorf("offset2 = %d, want < %d (file shrank)", offset2, offset1)
	}
}

func TestReadIncremental_MissingFile_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such.jsonl")

	entries, offset, err := conversation.ReadIncremental(missing, 0, nil, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil for missing file", err)
	}
	if entries != nil {
		t.Errorf("entries = %v, want nil", entries)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
}

func TestRead_StillWorksAsThinWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexInitialRollout), 0o644); err != nil {
		t.Fatal(err)
	}

	wrapper, err := conversation.Read(path, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	incremental, _, err := conversation.ReadIncremental(path, 0, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wrapper, incremental) {
		t.Errorf("Read wrapper diverged from ReadIncremental:\n  wrapper=    %+v\n  incremental=%+v", wrapper, incremental)
	}
}
