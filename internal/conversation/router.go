package conversation

import (
	codexconv "github.com/bjornjee/agent-dashboard/internal/codex/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Roots carries the per-harness filesystem roots the router needs to
// resolve a per-agent JSONL path. Each field is consumed by exactly one
// harness; unused fields stay zero-valued.
type Roots struct {
	// CodexSessionsRoot is typically ~/.codex/sessions.
	CodexSessionsRoot string
}

// Read returns the conversation entries for agent, routing by
// agent.Harness to the right per-harness parser. claude (and empty/legacy
// state files) uses the projDir+sessionID JSONL parser; codex uses the
// rollout JSONL parser under roots.CodexSessionsRoot.
//
// Errors from per-harness parsers are swallowed and surfaced as an empty
// slice — matches the existing ReadConversation contract that consumers
// (TUI/web) rely on for "no transcript yet" rendering.
func Read(agent domain.Agent, roots Roots, limit int) []domain.ConversationEntry {
	switch agent.Harness {
	case "codex":
		path, err := codexconv.LocateRollout(roots.CodexSessionsRoot, agent.SessionID)
		if err != nil || path == "" {
			return nil
		}
		entries, _ := codexconv.Read(path, limit)
		return entries
	default:
		return ReadConversation(agent.ProjDir, agent.SessionID, limit)
	}
}
