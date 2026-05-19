package conversation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Read dispatches by agent.Harness so callers don't have to know per-harness
// JSONL schemas. Codex agents read from <CodexSessionsRoot>/YYYY/MM/DD/
// rollout-*-<sid>.jsonl; everything else falls through to the legacy
// claude path (projDir/<sid>.jsonl).
func TestRead_RoutesCodexAgentToRolloutParser(t *testing.T) {
	root := t.TempDir()
	day := filepath.Join(root, "2026", "05", "18")
	_ = os.MkdirAll(day, 0o755)
	sid := "019e39d0"
	rollout := filepath.Join(day, "rollout-2026-05-18T14-40-15-"+sid+".jsonl")
	contents := `{"timestamp":"t1","type":"event_msg","payload":{"type":"user_message","message":"hi from codex"}}
{"timestamp":"t2","type":"event_msg","payload":{"type":"agent_message","message":"hello"}}
`
	_ = os.WriteFile(rollout, []byte(contents), 0o644)

	agent := domain.Agent{SessionID: sid, Harness: "codex"}
	got := conversation.Read(agent, conversation.Roots{CodexSessionsRoot: root}, 100)

	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
	if got[0].Role != "human" || got[0].Content != "hi from codex" {
		t.Errorf("got[0] = %+v, want human 'hi from codex'", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "hello" {
		t.Errorf("got[1] = %+v, want assistant 'hello'", got[1])
	}
}

// Claude agents fall through to the legacy projDir-based parser. We don't
// re-test claude's parsing here (covered by conversation_test.go); we only
// confirm the router dispatches correctly when Harness is empty/claude.
func TestRead_RoutesClaudeAgentToProjDirParser(t *testing.T) {
	proj := t.TempDir()
	sid := "claude-session-1"
	// claude JSONL has its own schema; an empty file means the parser
	// returns no entries — that's enough to verify the call path didn't
	// go through the codex parser (which would crash on the missing
	// rollout file otherwise).
	_ = os.WriteFile(filepath.Join(proj, sid+".jsonl"), []byte{}, 0o644)

	agent := domain.Agent{SessionID: sid, ProjDir: proj, Harness: "claude"}
	got := conversation.Read(agent, conversation.Roots{}, 100)
	if len(got) != 0 {
		t.Errorf("got %d, want 0 (empty claude jsonl)", len(got))
	}
}

// Empty Harness ("" — pre-codex state files) defaults to claude routing
// so existing agents keep working after the upgrade.
func TestRead_EmptyHarnessDefaultsToClaude(t *testing.T) {
	proj := t.TempDir()
	sid := "legacy"
	_ = os.WriteFile(filepath.Join(proj, sid+".jsonl"), []byte{}, 0o644)

	agent := domain.Agent{SessionID: sid, ProjDir: proj} // Harness omitted
	got := conversation.Read(agent, conversation.Roots{}, 100)
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}
