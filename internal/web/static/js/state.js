// Agent state mappings and helpers.

export const STATE_BADGE = {
  permission: 'blocked', plan: 'blocked',
  question: 'waiting', error: 'waiting',
  running: 'running',
  idle_prompt: 'review', done: 'review',
  merged: 'merged',
};

export const STATE_BORDER = {
  permission: 'var(--accent-red)', plan: 'var(--accent-red)',
  question: 'var(--accent-amber)', error: 'var(--accent-amber)',
  running: 'var(--accent-green)',
  idle_prompt: 'var(--accent-green)', done: 'var(--accent-green)',
  merged: 'var(--text-tertiary)',
};

export function statePriority(state) {
  const map = { permission: 1, plan: 1, question: 2, error: 2, running: 3, idle_prompt: 4, done: 4, merged: 6 };
  return map[state] || 99;
}

export function stateGroup(state) {
  const p = statePriority(state);
  if (p === 1) return 'BLOCKED';
  if (p === 2) return 'WAITING';
  if (p === 3) return 'RUNNING';
  if (p === 4) return 'REVIEW';
  if (p === 6) return 'MERGED';
  return 'OTHER';
}

// Trust the backend's `ApplyPinnedStates` — it has already merged the
// pin into `state` where appropriate (idle agents only). Reading raw
// `state` here means a running agent with a PR pin renders as
// "running", and the PR is surfaced separately as a tag (see prTag).
export function effectiveState(agent) {
  return agent.state;
}

// Returns a small tag label when the agent has an open PR. Decoupled
// from `effectiveState` so a running agent with a PR shows both the
// live state dot and the PR tag.
export function prTag(agent) {
  return agent && agent.pr_url ? 'PR open' : '';
}
