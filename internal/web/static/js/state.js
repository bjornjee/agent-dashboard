// Agent state mappings and helpers.

export const STATE_BADGE = {
  permission: 'blocked', plan: 'blocked',
  question: 'waiting', error: 'waiting',
  running: 'running',
  idle_prompt: 'review', done: 'review',
  pr: 'pr',
  merged: 'merged',
};

export const STATE_BORDER = {
  permission: 'var(--accent-red)', plan: 'var(--accent-red)',
  question: 'var(--accent-amber)', error: 'var(--accent-amber)',
  running: 'var(--accent-green)',
  idle_prompt: 'var(--accent-green)', done: 'var(--accent-green)',
  pr: 'var(--accent-indigo)',
  merged: 'var(--text-tertiary)',
};

export function statePriority(state) {
  const map = { permission: 1, plan: 1, question: 2, error: 2, running: 3, idle_prompt: 4, done: 4, pr: 5, merged: 6 };
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

// Returns a "PR open" tag when the user has pinned a PR on this agent.
// `pinned_state === 'pr'` is the single source of truth — it persists
// across state transitions (running, idle_prompt, …) so the tag shows
// alongside the live state regardless of what the agent is doing.
export function prTag(agent) {
  return agent && agent.pinned_state === 'pr' ? 'PR open' : '';
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
