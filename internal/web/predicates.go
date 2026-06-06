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
// Resolution order:
//  1. Sidecar Agent.PendingQuestion — free, stamped by the fast hook on
//     PreToolUse(AskUserQuestion | request_user_input). Survives pinned-
//     state overrides (e.g. agent.State rewritten to "pr") because the
//     sidecar is read independently.
//  2. State == "question" gate — state.ApplyIdleOverrides promotes both
//     claude and codex agents to "question" when their conversation
//     file shows an unanswered blocking tool. Outside that state, the
//     conversation scan is guaranteed to return nil, so we short-circuit
//     and avoid the JSONL/rollout I/O. This is the 99% case on the 2s
//     poll: `/api/agents/{id}/pending-question` for a running or done
//     agent returns nil without touching disk.
//  3. JSONL / rollout scan — paid. Reached only when sidecar is nil AND
//     state == "question". Routed by harness through
//     conversation.ReadPendingQuestion.
func PausedOnQuestion(agent domain.Agent, roots conversation.Roots) *domain.PendingQuestion {
	if agent.PendingQuestion != nil {
		return agent.PendingQuestion
	}
	if agent.SessionID == "" || agent.State != "question" {
		return nil
	}
	return conversation.ReadPendingQuestion(agent, roots)
}
