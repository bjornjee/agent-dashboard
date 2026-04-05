// Agent state mappings and helpers.

export const STATE_BADGE = {
  permission: 'red', plan: 'red',
  question: 'yellow', error: 'yellow',
  running: 'blue',
  idle_prompt: 'green', done: 'green',
  pr: 'purple',
  merged: 'teal',
};

export const STATE_BORDER = {
  permission: 'var(--status-red)', plan: 'var(--status-red)',
  question: 'var(--status-yellow)', error: 'var(--status-yellow)',
  running: 'var(--status-blue)',
  idle_prompt: 'var(--status-green)', done: 'var(--status-green)',
  pr: 'var(--status-purple)',
  merged: 'var(--status-teal)',
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

export function effectiveState(agent) {
  return agent.pinned_state || agent.state;
}
