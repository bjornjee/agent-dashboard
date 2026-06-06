package web

import (
	"github.com/bjornjee/agent-dashboard/internal/conversation"
	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// PausedOnQuestion returns the pending question payload for an agent, or
// nil if no question is pending. Single source of truth for both the
// detail endpoint (which serves the payload to the question card) and
// the answer endpoint (which decides whether to drive the tmux picker).
//
// Priority:
//  1. Sidecar Agent.PendingQuestion — free, stamped by the fast hook on
//     PreToolUse(AskUserQuestion | request_user_input). Survives pinned-
//     state overrides (e.g. agent.State rewritten to "pr") because the
//     sidecar is read independently.
//  2. JSONL / rollout scan — paid. Routed by harness through
//     conversation.ReadPendingQuestion. Fires only when the sidecar is
//     empty, so the cost is bounded to cases the hook missed (e.g. codex
//     Stop firing before the dashboard observed the request_user_input
//     PreToolUse stamp).
//
// Callers gate this on agent.State == "question" when they want the
// perf optimisation — the state-side promotion in
// state.ApplyIdleOverrides already enforces that the JSONL has an
// unanswered question by the time State == "question" lands. Callers
// outside that gate skip the scan entirely.
func PausedOnQuestion(agent domain.Agent, roots conversation.Roots) *domain.PendingQuestion {
	if agent.PendingQuestion != nil {
		return agent.PendingQuestion
	}
	if agent.SessionID == "" {
		return nil
	}
	return conversation.ReadPendingQuestion(agent, roots)
}
