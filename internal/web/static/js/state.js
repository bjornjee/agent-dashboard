// Agent state mappings and helpers.

export const STATE_BADGE = {
  permission: 'blocked', plan: 'blocked',
  question: 'waiting', error: 'waiting',
  running: 'running',
  idle_prompt: 'review', done: 'review',
  pr: 'pr',
  merged: 'merged',
  unregistered: 'unregistered',
};

export const STATE_BORDER = {
  permission: 'var(--accent-red)', plan: 'var(--accent-red)',
  question: 'var(--accent-amber)', error: 'var(--accent-amber)',
  running: 'var(--accent-green)',
  idle_prompt: 'var(--accent-green)', done: 'var(--accent-green)',
  pr: 'var(--accent-indigo)',
  merged: 'var(--text-tertiary)',
  unregistered: 'var(--text-tertiary)',
};

export function statePriority(state) {
  const map = { permission: 1, plan: 1, question: 2, error: 2, running: 3, idle_prompt: 4, done: 4, pr: 5, merged: 6, unregistered: 7 };
  return map[state] || 99;
}

export function stateGroup(state) {
  const p = statePriority(state);
  if (p === 1) return 'BLOCKED';
  if (p === 2) return 'WAITING';
  if (p === 3) return 'RUNNING';
  if (p === 4) return 'REVIEW';
  if (p === 5) return 'PR';
  if (p === 6) return 'MERGED';
  if (p === 7) return 'UNREGISTERED';
  return 'OTHER';
}

// Trust the backend's `ApplyPinnedStates` — it only swaps `state` to
// `pinned_state` when the agent is idle. So a running agent with a PR
// pin keeps state="running" (renders under RUNNING); an idle agent
// with a PR pin gets state="pr" (renders under PR). Reading raw
// `state` here matches what the TUI does via `SortedAgents`.
export function effectiveState(agent) {
  return agent.state;
}

// Returns a "PR" tag when the user has pinned a PR on this agent.
// `pinned_state === 'pr'` is the single source of truth — it persists
// across state transitions (running, idle_prompt, …) so the tag shows
// alongside the live state regardless of what the agent is doing.
//
// Suppressed when a pending question is active: a row with both PR and
// ASK chips is the pill-pileup anti-pattern — question always beats PR.
// The PR indicator survives in the status dot (status-dot--pr) so the
// signal is not lost.
export function prTag(agent) {
  if (!agent || agent.pinned_state !== 'pr') return '';
  if (questionBadge(agent)) return '';
  return 'PR';
}

// stateLabel returns the human-readable label for a state group, used as
// the aria-label on .status-dot so screen readers announce the meaning
// of the color signal. Mirrors STATE_BADGE keys.
export function stateLabel(state) {
  switch (state) {
    case 'permission':  return 'Needs approval';
    case 'plan':        return 'Plan review';
    case 'question':    return 'Needs reply';
    case 'error':       return 'Error';
    case 'running':     return 'Running';
    case 'idle_prompt': return 'Ready for review';
    case 'done':        return 'Done';
    case 'pr':          return 'PR open';
    case 'merged':      return 'Merged';
    case 'unregistered': return 'Unregistered';
    default:            return '';
  }
}

// Returns a "Trust" tag when the dashboard's post-spawn poller has
// seen a harness folder-trust dialog and the user has not accepted yet.
// Text-only to match the PR open precedent; the amber toast carries the
// folder name, the chip carries the verb.
export function trustTag(agent) {
  return agent && agent.trust_prompt_detected ? 'Trust' : '';
}

// rowTag picks the most actionable tag to render on a list row. Trust
// supersedes PR because it blocks the agent from making any progress
// until the user accepts in tmux.
export function rowTag(agent) {
  return trustTag(agent) || prTag(agent);
}

// True when the agent has an open PR — used to gate Open PR / Merge
// action chips. Same single signal as prTag.
export function hasOpenPR(agent) {
  return !!(agent && agent.pinned_state === 'pr');
}

// planBadge returns 'PLAN' when the agent is operating in plan mode by
// either signal: the hook-set state='plan' (ExitPlanMode pending) or
// Claude Code's permission_mode='plan' (EnterPlanMode active). Keeps the
// doctrine from ~/.claude/rules/core.md ("dashboard renders a visible
// plan badge") in one place so list and detail render the same chip.
export function planBadge(agent) {
  if (!agent) return '';
  return agent.state === 'plan' || agent.permission_mode === 'plan' ? 'PLAN' : '';
}

// subagentBadge returns '↳ N' when the agent has at least one live
// subagent (subagent_count > 0). Empty otherwise. Defensive against
// malformed state files: rejects non-numeric / negative counts.
export function subagentBadge(agent) {
  const n = agent && agent.subagent_count;
  if (typeof n !== 'number' || !Number.isFinite(n) || n <= 0) return '';
  return '↳ ' + n;
}

// questionBadge returns 'ASK' when the agent has a pending question.
// Independent of state group — an agent with pinned_state='pr' that's
// also asking still has a blocking question. No counter: a row badge is
// a signal, not a status panel; the count lives in the detail view.
export function questionBadge(agent) {
  const q = agent && agent.pending_question;
  if (!q || !Array.isArray(q.questions) || q.questions.length === 0) return '';
  return 'ASK';
}
