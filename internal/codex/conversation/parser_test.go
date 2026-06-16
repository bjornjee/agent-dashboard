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

// ReadPendingQuestion returns the parsed payload of the most recent
// unanswered request_user_input function_call in a codex rollout. A
// matching function_call_output (same call_id) means the question was
// answered — return nil.
func TestReadPendingQuestion_ReturnsLatestUnanswered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"2026-06-06T06:39:26.092Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"fmt_target\",\"header\":\"Fmt target\",\"question\":\"Add make fmt?\",\"options\":[{\"label\":\"Add target (Recommended)\",\"description\":\"Adds a fmt gate.\"},{\"label\":\"Skip formatting\",\"description\":\"Proceed without.\"}]}]}","call_id":"call_xyz"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got := conversation.ReadPendingQuestion(path)
	if got == nil {
		t.Fatal("expected non-nil PendingQuestion")
	}
	if got.ToolUseID != "call_xyz" {
		t.Errorf("ToolUseID = %q, want %q", got.ToolUseID, "call_xyz")
	}
	if len(got.Questions) != 1 {
		t.Fatalf("Questions len = %d, want 1", len(got.Questions))
	}
	q := got.Questions[0]
	if q.ID != "fmt_target" {
		t.Errorf("Questions[0].ID = %q, want %q", q.ID, "fmt_target")
	}
	if q.Question != "Add make fmt?" {
		t.Errorf("Questions[0].Question = %q", q.Question)
	}
	if q.Header != "Fmt target" {
		t.Errorf("Questions[0].Header = %q", q.Header)
	}
	if len(q.Options) != 2 || q.Options[0].Label != "Add target (Recommended)" {
		t.Errorf("Questions[0].Options unexpected: %+v", q.Options)
	}
}

func TestReadPendingQuestion_AnsweredReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"2026-06-06T06:39:26.092Z","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"x\",\"options\":[{\"label\":\"a\"}]}]}","call_id":"call_ans"}}
{"timestamp":"2026-06-06T06:39:30.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_ans","output":"{\"answers\":{\"q1\":{\"answers\":[\"a\"]}}}"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := conversation.ReadPendingQuestion(path); got != nil {
		t.Errorf("expected nil for answered question, got %+v", got)
	}
}

func TestReadPendingQuestion_MissingFileReturnsNil(t *testing.T) {
	if got := conversation.ReadPendingQuestion(filepath.Join(t.TempDir(), "nope.jsonl")); got != nil {
		t.Errorf("expected nil for missing file, got %+v", got)
	}
}

func TestReadPendingQuestion_NoQuestionReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"event_msg","payload":{"type":"user_message","message":"hi"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := conversation.ReadPendingQuestion(path); got != nil {
		t.Errorf("expected nil with no request_user_input, got %+v", got)
	}
}

// LastPendingBlockingToolCodex is the codex symmetric of
// conversation.LastPendingBlockingTool. It exists so state.ApplyIdleOverrides
// can promote codex agents to state="question" without itself depending on
// the rollout-parser internals.
func TestLastPendingBlockingToolCodex_QuestionWhenUnanswered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"x\",\"options\":[{\"label\":\"a\"}]}]}","call_id":"call_unans"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got := conversation.LastPendingBlockingToolCodex(path)
	if got != "question" {
		t.Errorf("got %q, want \"question\"", got)
	}
}

func TestLastPendingBlockingToolCodex_EmptyWhenAnswered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	contents := `{"timestamp":"t1","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"x\",\"options\":[{\"label\":\"a\"}]}]}","call_id":"call_ans"}}
{"timestamp":"t2","type":"response_item","payload":{"type":"function_call_output","call_id":"call_ans","output":"{\"answers\":{\"q1\":{\"answers\":[\"a\"]}}}"}}
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := conversation.LastPendingBlockingToolCodex(path); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLastPendingBlockingToolCodex_EmptyWhenMissingFile(t *testing.T) {
	if got := conversation.LastPendingBlockingToolCodex(filepath.Join(t.TempDir(), "nope.jsonl")); got != "" {
		t.Errorf("got %q, want empty for missing file", got)
	}
}

// LastPendingBlockingToolCodex must promote codex agents to state="plan"
// when an `item_completed` Plan event is the most recent blocking signal in
// the rollout — the codex symmetric of claude's ExitPlanMode promotion in
// conversation.LastPendingBlockingTool. Real rollouts open with multiple
// user-role response_item messages (AGENTS.md instructions, environment_context,
// the user's prompt) before any blocking event; those must not be treated as
// "the user already responded to the plan" — the same skip-bootstrap-user-turns
// pattern claude's scanner uses.
func TestLastPendingBlockingToolCodex_PlanPriority(t *testing.T) {
	const planLine = `{"timestamp":"t-plan","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","text":"# proposal"}}}`
	const questionLine = `{"timestamp":"t-q","type":"response_item","payload":{"type":"function_call","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"q1\",\"question\":\"x\",\"options\":[{\"label\":\"a\"}]}]}","call_id":"cid"}}`
	const questionAnswered = `{"timestamp":"t-qa","type":"response_item","payload":{"type":"function_call_output","call_id":"cid","output":"{}"}}`
	const userMsg = `{"timestamp":"t-u","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"approve"}]}}`
	const bootstrapUserMsg = `{"timestamp":"t-boot","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions"}]}}`

	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "plan when plan is last",
			lines: []string{planLine},
			want:  "plan",
		},
		{
			name:  "empty when plan followed by user message",
			lines: []string{planLine, userMsg},
			want:  "",
		},
		{
			name:  "question when question after plan",
			lines: []string{planLine, questionLine},
			want:  "question",
		},
		{
			name:  "plan when plan after answered question",
			lines: []string{questionLine, questionAnswered, planLine},
			want:  "plan",
		},
		{
			name:  "plan unaffected by session bootstrap user messages",
			lines: []string{bootstrapUserMsg, planLine},
			want:  "plan",
		},
		{
			name:  "empty when question followed by user message reply",
			lines: []string{questionLine, userMsg},
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rollout.jsonl")
			contents := ""
			for _, l := range tc.lines {
				contents += l + "\n"
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := conversation.LastPendingBlockingToolCodex(path); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
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
