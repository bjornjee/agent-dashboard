package web

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

func TestPausedOnQuestion_PrefersSidecar(t *testing.T) {
	// Sidecar PendingQuestion short-circuits the JSONL fallback. Zero I/O.
	agent := domain.Agent{
		SessionID: "p1",
		PendingQuestion: &domain.PendingQuestion{
			ToolUseID: "tool_x",
			Questions: []domain.PendingQuestionPrompt{{Question: "from sidecar"}},
		},
	}
	got := PausedOnQuestion(agent, conversation.Roots{})
	if got == nil || got.ToolUseID != "tool_x" {
		t.Fatalf("expected sidecar payload, got %+v", got)
	}
}

func TestPausedOnQuestion_FallsBackToJSONLWhenStateIsQuestion(t *testing.T) {
	// No sidecar + state="question" (set by ApplyIdleOverrides) → scan
	// the claude JSONL.
	projDir := t.TempDir()
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool_jsonl","name":"AskUserQuestion","input":{"questions":[{"question":"from jsonl","options":[{"label":"A"}]}]}}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, "p2.jsonl"), []byte(jsonl), 0644)

	agent := domain.Agent{
		SessionID: "p2",
		ProjDir:   projDir,
		Harness:   "", // default = claude
		State:     "question",
	}
	got := PausedOnQuestion(agent, conversation.Roots{})
	if got == nil {
		t.Fatal("expected JSONL fallback to find the question, got nil")
	}
	if got.ToolUseID != "tool_jsonl" {
		t.Errorf("ToolUseID = %q, want tool_jsonl", got.ToolUseID)
	}
}

func TestPausedOnQuestion_SkipsJSONLScanWhenStateNotQuestion(t *testing.T) {
	// Perf contract: state != "question" guarantees no I/O. The poll
	// hitting /api/agents/{id}/pending-question every 2s for a running
	// or done agent must short-circuit to nil without scanning the
	// JSONL — ApplyIdleOverrides would have already promoted state to
	// "question" if the JSONL had a pending question.
	//
	// We assert this by pointing ProjDir at a JSONL that DOES have an
	// unanswered AskUserQuestion; if the gate works, the scan never
	// runs and we get nil.
	projDir := t.TempDir()
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool_jsonl","name":"AskUserQuestion","input":{"questions":[{"question":"would find this","options":[{"label":"A"}]}]}}]},"timestamp":"2026-06-02T10:00:00Z"}
`
	os.WriteFile(filepath.Join(projDir, "p_running.jsonl"), []byte(jsonl), 0644)

	for _, state := range []string{"running", "done", "idle_prompt", "permission", "pr", "merged", "error", "plan"} {
		agent := domain.Agent{
			SessionID: "p_running",
			ProjDir:   projDir,
			Harness:   "",
			State:     state,
		}
		if got := PausedOnQuestion(agent, conversation.Roots{}); got != nil {
			t.Errorf("state=%q: expected nil (gate should skip scan), got %+v", state, got)
		}
	}
}

func TestPausedOnQuestion_NilWhenEmpty(t *testing.T) {
	// No sidecar, no SessionID → nil without I/O.
	if got := PausedOnQuestion(domain.Agent{}, conversation.Roots{}); got != nil {
		t.Errorf("expected nil for empty agent, got %+v", got)
	}
}

func TestPausedOnQuestion_NilWhenSessionMissing(t *testing.T) {
	// PendingQuestion nil + SessionID empty → return nil before touching I/O.
	agent := domain.Agent{Harness: "claude", ProjDir: "/tmp/nope"}
	if got := PausedOnQuestion(agent, conversation.Roots{}); got != nil {
		t.Errorf("expected nil for agent without SessionID, got %+v", got)
	}
}
