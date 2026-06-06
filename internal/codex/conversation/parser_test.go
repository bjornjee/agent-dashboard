package conversation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/conversation"
)

// Rollout JSONL line schema as emitted by codex CLI 0.130.0 (verified by
// inspection of ~/.codex/sessions/...). Each line has shape:
//
//	{"timestamp":"<ISO8601>","type":"<kind>","payload":{...}}
//
// Kinds we care about for conversation rendering:
//   - event_msg with payload.type == "user_message"  → human turn
//   - event_msg with payload.type == "agent_message" → assistant turn
//
// Codex also emits role=user/role=assistant response_item entries that
// duplicate the same content (with extra envelope text like
// <environment_context>...</environment_context>). We prefer the event_msg
// form because the text is already cleaned.
func TestRead_HumanAndAssistantTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"2026-04-02T09:57:22.377Z","type":"session_meta","payload":{"id":"019d4da0","timestamp":"2026-04-02T09:57:22Z","cwd":"/repo","cli_version":"0.130.0","model_provider":"openai"}}
{"timestamp":"2026-04-02T09:57:22.377Z","type":"event_msg","payload":{"type":"task_started","turn_id":"019d","started_at":1759399042}}
{"timestamp":"2026-04-02T09:57:22.377Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>..."}]}}
{"timestamp":"2026-04-02T09:57:22.377Z","type":"event_msg","payload":{"type":"user_message","message":"Optimize the counter implementation for correctness and brevity."}}
{"timestamp":"2026-04-02T09:57:24.762Z","type":"response_item","payload":{"type":"reasoning","summary":[],"encrypted_content":"opaque..."}}
{"timestamp":"2026-04-02T09:57:29.710Z","type":"event_msg","payload":{"type":"agent_message","message":"I'm checking the current counter.py first."}}
{"timestamp":"2026-04-02T09:57:30.000Z","type":"turn_context","payload":{"turn_id":"019d","approval_policy":"never","collaboration_mode":{"mode":"default"}}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := conversation.Read(path, 100)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (human + assistant). entries: %+v", len(got), got)
	}
	if got[0].Role != "human" {
		t.Errorf("got[0].Role = %q, want \"human\"", got[0].Role)
	}
	if got[0].Content != "Optimize the counter implementation for correctness and brevity." {
		t.Errorf("got[0].Content = %q, want the user message", got[0].Content)
	}
	if got[0].Timestamp != "2026-04-02T09:57:22.377Z" {
		t.Errorf("got[0].Timestamp = %q, want first timestamp", got[0].Timestamp)
	}
	if got[1].Role != "assistant" {
		t.Errorf("got[1].Role = %q, want \"assistant\"", got[1].Role)
	}
	if got[1].Content != "I'm checking the current counter.py first." {
		t.Errorf("got[1].Content = %q, want assistant message", got[1].Content)
	}
}

// Missing file → empty slice, nil error. Codex sessions may be referenced
// from the index before their rollout file is fully on disk.
func TestRead_MissingFile(t *testing.T) {
	got, err := conversation.Read(filepath.Join(t.TempDir(), "nope.jsonl"), 100)
	if err != nil {
		t.Fatalf("Read err = %v, want nil for missing file", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

// limit==0 → return everything. limit>0 → cap from the most recent.
func TestRead_LimitTrimsHead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"event_msg","payload":{"type":"user_message","message":"u1"}}
{"timestamp":"t2","type":"event_msg","payload":{"type":"agent_message","message":"a1"}}
{"timestamp":"t3","type":"event_msg","payload":{"type":"user_message","message":"u2"}}
{"timestamp":"t4","type":"event_msg","payload":{"type":"agent_message","message":"a2"}}
`
	_ = os.WriteFile(path, []byte(contents), 0o644)

	got, err := conversation.Read(path, 2)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	// limit=2 → keep last 2 (u2, a2), oldest first
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Content != "u2" || got[1].Content != "a2" {
		t.Errorf("got %+v, want [u2, a2]", got)
	}
}

// Codex emits a clean `item_completed` event with `item.type=="Plan"` when
// a plan is finalized (the same `<proposed_plan>` content the user sees in
// the codex TUI, with the wrapper stripped). The parser must emit a
// synthetic plan-saved ConversationEntry at that timestamp so the frontend
// can render the chat-stream "View plan" navigation card — mirroring how
// the claude parser handles ExitPlanMode tool_use.
func TestRead_EmitsPlanSavedOnItemCompletedPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"2026-06-06T06:33:02.202Z","type":"event_msg","payload":{"type":"user_message","message":"create the migration plan"}}
{"timestamp":"2026-06-06T06:39:26.092Z","type":"event_msg","payload":{"type":"item_completed","thread_id":"019e9ba2","turn_id":"019e9ba8","item":{"type":"Plan","id":"019e9ba8-plan","text":"# Flatten Move-in Items Checklist\n\nArchive non-table blocks."}}}
{"timestamp":"2026-06-06T06:39:30.000Z","type":"event_msg","payload":{"type":"agent_message","message":"Plan ready for review."}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := conversation.Read(path, 100)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (human, plan-saved, assistant). entries: %+v", len(got), got)
	}
	if got[1].Role != "plan-saved" {
		t.Errorf("got[1].Role = %q, want \"plan-saved\"", got[1].Role)
	}
	if got[1].Timestamp != "2026-06-06T06:39:26.092Z" {
		t.Errorf("got[1].Timestamp = %q, want plan item_completed timestamp", got[1].Timestamp)
	}
}

// ReadPlanContent returns the most recent plan text emitted in this codex
// session — the `item.text` payload of the latest item_completed event
// whose item.type is "Plan". Missing files and sessions with no plan
// return "" without error so the handler can fall through to an empty
// state.
func TestReadPlanContent_ReturnsLatestPlanText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"2026-06-06T06:39:26.092Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","text":"# First plan\n\nold"}}}
{"timestamp":"2026-06-06T06:39:30.000Z","type":"event_msg","payload":{"type":"agent_message","message":"chatter"}}
{"timestamp":"2026-06-06T06:45:00.000Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","text":"# Second plan\n\nlatest"}}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got := conversation.ReadPlanContent(path)
	if got != "# Second plan\n\nlatest" {
		t.Errorf("ReadPlanContent = %q, want second plan text", got)
	}
}

func TestReadPlanContent_NoPlanReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"event_msg","payload":{"type":"user_message","message":"hi"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := conversation.ReadPlanContent(path); got != "" {
		t.Errorf("ReadPlanContent = %q, want empty", got)
	}
}

func TestReadPlanContent_MissingFileReturnsEmpty(t *testing.T) {
	if got := conversation.ReadPlanContent(filepath.Join(t.TempDir(), "nope.jsonl")); got != "" {
		t.Errorf("ReadPlanContent = %q, want empty for missing file", got)
	}
}

// Malformed JSON lines and unrecognized event types are skipped without
// affecting valid entries (resilient to partial tail writes from codex).
func TestRead_SkipsMalformedAndUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"event_msg","payload":{"type":"user_message","message":"good"}}
not-json
{"timestamp":"t2","type":"event_msg","payload":{"type":"some_future_kind","data":42}}
{"timestamp":"t3","type":"event_msg","payload":{"type":"agent_message","message":"also-good"}}
`
	_ = os.WriteFile(path, []byte(contents), 0o644)

	got, err := conversation.Read(path, 0)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (skip malformed + unknown)", len(got))
	}
}
