package conversation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/codex/conversation"
)

func TestFindSubagents_DiscoversChildrenForParentThread(t *testing.T) {
	root := t.TempDir()
	parentID := "019e4afc-c556-7812-a70d-4d3b87d6a8a2"
	childID := "019e4afe-59aa-7720-a201-74439b281444"

	writeRollout(t, root, "2026", "05", "21", childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"`+childID+`","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"`+parentID+`","agent_nickname":"Nietzsche","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Nietzsche","agent_role":"explorer"}}
{"timestamp":"2026-05-21T14:44:03.700Z","type":"event_msg","payload":{"type":"task_started"}}
{"timestamp":"2026-05-21T14:45:00.000Z","type":"event_msg","payload":{"type":"task_complete"}}
`)
	writeRollout(t, root, "2026", "05", "21", "unrelated", `{"timestamp":"2026-05-21T14:46:03.645Z","type":"session_meta","payload":{"id":"unrelated","timestamp":"2026-05-21T14:46:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"other-parent","agent_nickname":"Spinoza","agent_role":"worker"}}},"thread_source":"subagent","agent_nickname":"Spinoza","agent_role":"worker"}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(got), got)
	}
	if got[0].AgentID != childID {
		t.Errorf("AgentID = %q, want %q", got[0].AgentID, childID)
	}
	if got[0].AgentType != "explorer" {
		t.Errorf("AgentType = %q, want explorer", got[0].AgentType)
	}
	if got[0].Description != "Nietzsche" {
		t.Errorf("Description = %q, want nickname", got[0].Description)
	}
	if !got[0].Completed {
		t.Error("Completed = false, want true after task_complete")
	}
	if got[0].StartedAt != "2026-05-21T14:44:03.645Z" {
		t.Errorf("StartedAt = %q, want session timestamp", got[0].StartedAt)
	}
}

func TestFindSubagents_SortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	parentID := "parent"

	writeRollout(t, root, "2026", "05", "20", "older", `{"timestamp":"2026-05-20T14:44:03.645Z","type":"session_meta","payload":{"id":"older","timestamp":"2026-05-20T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"explorer"}}},"thread_source":"subagent","agent_role":"explorer"}}
`)
	writeRollout(t, root, "2026", "05", "21", "newer", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"newer","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"worker"}}},"thread_source":"subagent","agent_role":"worker"}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 2 {
		t.Fatalf("got %d subagents, want 2", len(got))
	}
	if got[0].AgentID != "newer" || got[1].AgentID != "older" {
		t.Errorf("order = [%s, %s], want [newer, older]", got[0].AgentID, got[1].AgentID)
	}
}

func TestFindSubagents_DescriptionFallsBackToThreadRole(t *testing.T) {
	root := t.TempDir()
	parentID := "parent"
	writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"worker"}}},"thread_source":"subagent"}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1", len(got))
	}
	if got[0].Description != "worker" {
		t.Errorf("Description = %q, want worker", got[0].Description)
	}
}

func TestFindSubagents_InstructionHeadPrefersUserInstruction(t *testing.T) {
	root := t.TempDir()
	parentID := "parent"
	writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_nickname":"Kant","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Kant","agent_role":"explorer"}}
{"timestamp":"2026-05-21T14:44:04.000Z","type":"event_msg","payload":{"type":"user_message","message":"\n\nResearch the parser path for Codex subagents.\nReturn exact files and tests."}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1", len(got))
	}
	if got[0].InstructionHead != "Research the parser path for Codex subagents." {
		t.Errorf("InstructionHead = %q, want first meaningful instruction line", got[0].InstructionHead)
	}
	if got[0].Description != got[0].InstructionHead {
		t.Errorf("Description = %q, want instruction head for backward-compatible display", got[0].Description)
	}
}

func TestFindSubagents_InstructionHeadFallsBackWhenNoUserMessage(t *testing.T) {
	root := t.TempDir()
	parentID := "parent"
	writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"worker"}}},"thread_source":"subagent"}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1", len(got))
	}
	if got[0].InstructionHead != "worker" {
		t.Errorf("InstructionHead = %q, want role fallback", got[0].InstructionHead)
	}
}

func TestFindSubagents_ModeFromTurnContext(t *testing.T) {
	root := t.TempDir()
	parentID := "parent"
	writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"explorer"}}},"thread_source":"subagent"}}
{"timestamp":"2026-05-21T14:44:04.000Z","type":"turn_context","payload":{"approval_policy":"on-request","sandbox_policy":{"type":"workspace-write"},"collaboration_mode":{"mode":"plan"}}}
`)

	got := conversation.FindSubagents(root, parentID)
	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1", len(got))
	}
	if got[0].Mode != "plan / on-request / workspace-write" {
		t.Errorf("Mode = %q, want compact Codex context", got[0].Mode)
	}
}

func TestParentThreadID_ReturnsParentForSubagentSession(t *testing.T) {
	root := t.TempDir()
	writeRollout(t, root, "2026", "05", "21", "child", `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","agent_role":"worker"}}},"thread_source":"subagent"}}
`)

	got := conversation.ParentThreadID(root, "child")
	if got != "parent" {
		t.Errorf("ParentThreadID = %q, want parent", got)
	}
}

func writeRollout(t *testing.T, root, year, month, day, sessionID, contents string) string {
	t.Helper()

	dir := filepath.Join(root, year, month, day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+year+"-"+month+"-"+day+"T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
}
