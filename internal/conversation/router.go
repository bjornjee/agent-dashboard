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
//
// Read is a thin wrapper for ReadIncremental(agent, roots, limit, nil, 0);
// callers that maintain prev+offset state should use ReadIncremental
// directly to skip re-decoding bytes from previous calls.
func Read(agent domain.Agent, roots Roots, limit int) []domain.ConversationEntry {
	entries, _ := ReadIncremental(agent, roots, limit, nil, 0)
	return entries
}

// ReadIncremental returns conversation entries for agent and the file
// byte offset to pass back on the next call. Both per-harness parsers
// are append-only-safe: when prev+prevOffset match a previously-seen
// state, only newly appended bytes are decoded. Pass nil/0 for a full
// read.
//
// For codex, the rollout's filesystem path is part of the (callee-owned)
// resume contract — a `codex resume <sid>` lands the next turn in a
// brand-new rollout file under a later date directory, so the caller's
// sessionKey must include the path (not just sessionID) to detect that
// transition and reset prevOffset.
func ReadIncremental(agent domain.Agent, roots Roots, limit int, prev []domain.ConversationEntry, prevOffset int64) ([]domain.ConversationEntry, int64) {
	switch agent.Harness {
	case "codex":
		path, err := codexconv.LocateRollout(roots.CodexSessionsRoot, agent.SessionID)
		if err != nil || path == "" {
			return nil, 0
		}
		entries, newOffset, _ := codexconv.ReadIncremental(path, limit, prev, prevOffset)
		return entries, newOffset
	default:
		return ReadConversationIncremental(agent.ProjDir, agent.SessionID, limit, prev, prevOffset)
	}
}

func ReadSubagents(agent domain.Agent, roots Roots) []domain.SubagentInfo {
	switch agent.Harness {
	case "codex":
		return codexconv.FindSubagents(roots.CodexSessionsRoot, agent.SessionID)
	default:
		return FindSubagents(agent.ProjDir, agent.SessionID)
	}
}

// TopLevelAgents filters out codex sessions that should not appear at the
// top of the dashboard:
//
//  1. Codex subagents of another session (those with a parent_thread_id in
//     their session_meta) — the parent shows them under its subagent list.
//  2. Sessions written by the Codex desktop app (payload.originator ==
//     OriginatorDesktopApp). They share ~/.codex/sessions/ with codex-tui
//     but represent the desktop GUI, which the dashboard intentionally
//     does not surface.
//
// The check is per-agent (not a global walk). ParentThreadID and Originator
// share a per-session cache, so both lookups for the same agent hit the
// same cached session_meta read within the TTL window.
func TopLevelAgents(agents []domain.Agent, roots Roots) []domain.Agent {
	if len(agents) == 0 {
		return nil
	}
	out := make([]domain.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.Harness == "codex" {
			if codexconv.ParentThreadID(roots.CodexSessionsRoot, agent.SessionID) != "" {
				continue
			}
			if codexconv.Originator(roots.CodexSessionsRoot, agent.SessionID) == codexconv.OriginatorDesktopApp {
				continue
			}
		}
		out = append(out, agent)
	}
	return out
}
