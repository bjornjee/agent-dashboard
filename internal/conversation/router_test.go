package conversation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	codexconv "github.com/bjornjee/agent-dashboard/internal/codex/conversation"
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

func TestReadSubagents_RoutesCodexAgentToRolloutDiscovery(t *testing.T) {
	t.Cleanup(codexconv.InvalidateCacheForTest)
	root := t.TempDir()
	parentID := "parent-codex"
	childID := "child-codex"
	writeCodexRollout(t, root, childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-codex","agent_nickname":"Nietzsche","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Nietzsche","agent_role":"explorer"}}
`)

	agent := domain.Agent{SessionID: parentID, Harness: "codex"}
	got := conversation.ReadSubagents(agent, conversation.Roots{CodexSessionsRoot: root})

	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(got), got)
	}
	if got[0].AgentID != childID {
		t.Errorf("AgentID = %q, want %q", got[0].AgentID, childID)
	}
}

func TestReadSubagents_RoutesClaudeAgentToProjectDiscovery(t *testing.T) {
	proj := t.TempDir()
	sessionID := "claude-parent"
	subDir := filepath.Join(proj, sessionID, "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]string{
		"agentType":   "worker",
		"description": "Claude worker",
	})
	if err := os.WriteFile(filepath.Join(subDir, "agent-abc.meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	jsonl := `{"sessionId":"claude-parent","timestamp":"2026-05-21T10:00:00Z"}
{"type":"result"}
`
	if err := os.WriteFile(filepath.Join(subDir, "agent-abc.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := domain.Agent{SessionID: sessionID, ProjDir: proj, Harness: "claude"}
	got := conversation.ReadSubagents(agent, conversation.Roots{})

	if len(got) != 1 {
		t.Fatalf("got %d subagents, want 1: %+v", len(got), got)
	}
	if got[0].AgentID != "abc" {
		t.Errorf("AgentID = %q, want abc", got[0].AgentID)
	}
}

func TestTopLevelAgents_FiltersCodexSubagentSessions(t *testing.T) {
	t.Cleanup(codexconv.InvalidateCacheForTest)

	root := t.TempDir()
	parentID := "parent-codex"
	childID := "child-codex"
	writeCodexRollout(t, root, childID, `{"timestamp":"2026-05-21T14:44:03.645Z","type":"session_meta","payload":{"id":"child-codex","timestamp":"2026-05-21T14:44:03.645Z","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-codex","agent_nickname":"Nietzsche","agent_role":"explorer"}}},"thread_source":"subagent","agent_nickname":"Nietzsche","agent_role":"explorer"}}
`)
	agents := []domain.Agent{
		{SessionID: parentID, Harness: "codex", Target: "main:1.0"},
		{SessionID: childID, Harness: "codex", Target: "main:2.0"},
		{SessionID: "claude", Harness: "claude", Target: "main:3.0"},
	}

	got := conversation.TopLevelAgents(agents, conversation.Roots{CodexSessionsRoot: root})
	if len(got) != 2 {
		t.Fatalf("got %d agents, want parent codex + claude: %+v", len(got), got)
	}
	if got[0].SessionID != parentID || got[1].SessionID != "claude" {
		t.Errorf("agents = %+v, want child codex filtered", got)
	}
}

// Codex Desktop app rollouts share the ~/.codex/sessions/ tree with the
// codex CLI, but the user only wants CLI threads on the dashboard. The
// discriminator is session_meta payload.originator: "codex-tui" for the
// CLI vs "Codex Desktop" for the desktop app. TopLevelAgents must drop
// the desktop-app rollouts while keeping codex-tui sessions and other
// harnesses untouched.
func TestTopLevelAgents_FiltersCodexDesktopAppSessions(t *testing.T) {
	t.Cleanup(codexconv.InvalidateCacheForTest)

	root := t.TempDir()
	cliSID := "cli-codex"
	desktopSID := "desktop-codex"
	writeCodexRollout(t, root, cliSID, `{"timestamp":"2026-05-29T20:23:40.347Z","type":"session_meta","payload":{"id":"cli-codex","timestamp":"2026-05-29T20:23:40.132Z","originator":"codex-tui","source":"cli","thread_source":"user"}}
`)
	writeCodexRollout(t, root, desktopSID, `{"timestamp":"2026-05-29T23:33:05.658Z","type":"session_meta","payload":{"id":"desktop-codex","timestamp":"2026-05-29T23:33:05.658Z","originator":"Codex Desktop","source":"vscode","thread_source":"user"}}
`)
	agents := []domain.Agent{
		{SessionID: cliSID, Harness: "codex", Target: "main:1.0"},
		{SessionID: desktopSID, Harness: "codex", Target: "main:2.0"},
		{SessionID: "claude", Harness: "claude", Target: "main:3.0"},
	}

	got := conversation.TopLevelAgents(agents, conversation.Roots{CodexSessionsRoot: root})
	if len(got) != 2 {
		t.Fatalf("got %d agents, want codex-tui + claude (desktop filtered): %+v", len(got), got)
	}
	if got[0].SessionID != cliSID || got[1].SessionID != "claude" {
		t.Errorf("agents = %+v, want codex-tui first, claude second, Desktop filtered", got)
	}
}

// Non-codex harnesses must not trigger any codex filesystem work. With a
// non-existent CodexSessionsRoot, claude/legacy agents still flow through
// unchanged — the per-agent guard short-circuits before any walk.
func TestTopLevelAgents_NonCodexHarnessSkipsCodexLookup(t *testing.T) {
	t.Cleanup(codexconv.InvalidateCacheForTest)

	agents := []domain.Agent{
		{SessionID: "claude-1", Harness: "claude", Target: "main:1.0"},
		{SessionID: "legacy-1", Harness: "", Target: "main:2.0"},
	}

	got := conversation.TopLevelAgents(agents, conversation.Roots{CodexSessionsRoot: "/nonexistent-root-aaa"})
	if len(got) != 2 {
		t.Fatalf("got %d agents, want 2: %+v", len(got), got)
	}
}

// A codex agent that has no rollout file (or no parent_thread_id in its
// session_meta) is a top-level agent and must NOT be filtered.
func TestTopLevelAgents_CodexParentWithoutRolloutKept(t *testing.T) {
	t.Cleanup(codexconv.InvalidateCacheForTest)

	root := t.TempDir()
	agents := []domain.Agent{
		{SessionID: "parent-no-rollout", Harness: "codex", Target: "main:1.0"},
	}

	got := conversation.TopLevelAgents(agents, conversation.Roots{CodexSessionsRoot: root})
	if len(got) != 1 || got[0].SessionID != "parent-no-rollout" {
		t.Errorf("got %+v, want the codex parent kept", got)
	}
}

func writeCodexRollout(t *testing.T, root, sessionID, contents string) string {
	t.Helper()

	dir := filepath.Join(root, "2026", "05", "21")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rollout dir: %v", err)
	}
	path := filepath.Join(dir, "rollout-2026-05-21T00-00-00-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	return path
}
