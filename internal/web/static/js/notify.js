// Notification system — state transition detection and browser Notification API.

import { toast } from './modal.js';

const STORAGE_KEY = 'notify-enabled';

// Previous state map: sessionId → state
const prevStateMap = new Map();
// Previous trust-prompt flag map: sessionId → bool, used to toast once
// per false→true transition (so the in-app hint doesn't re-fire on every
// SSE tick while the agent stays stuck on trust).
const prevTrustMap = new Map();
let seeded = false;

// Cache the SW registration so we don't await navigator.serviceWorker.ready on every notification
let swReg = null;
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.ready.then(reg => { swReg = reg; });
}

// States that warrant a notification when an agent transitions INTO them.
// Labels stay short so the title fits on a narrow lock screen.
const NOTIFY_STATES = {
  permission: 'Needs permission',
  plan:       'Plan ready',
  question:   'Question',
  error:      'Error',
  done:       'Finished',
  idle_prompt:'Finished',
  pr:         'PR ready',
};

const DESC_MAX = 140;

function basename(p) {
  if (!p) return '';
  const trimmed = p.replace(/\/+$/, '');
  const i = trimmed.lastIndexOf('/');
  return i === -1 ? trimmed : trimmed.slice(i + 1);
}

function shortDescription(agent) {
  let src = '';
  if (agent.state === 'question') {
    const q = agent.pending_question && agent.pending_question.questions && agent.pending_question.questions[0];
    if (q && q.question) src = q.question;
  }
  if (!src) src = agent.last_message_preview || '';
  src = src.trim();
  if (src.length > DESC_MAX) src = src.slice(0, DESC_MAX - 1) + '…';
  return src;
}

export function formatNotification(agent, stateLabel) {
  const branch = (agent.branch || '').trim();
  const sid = agent.session_id || '';
  const head = branch || sid.slice(0, 7);
  const title = stateLabel ? `${head} · ${stateLabel}` : head;

  const dir = basename(agent.worktree_cwd || agent.cwd || '');
  const desc = shortDescription(agent);

  const lines = [];
  if (dir) lines.push(dir);
  if (desc) lines.push(desc);
  const body = lines.length ? lines.join('\n') : stateLabel;

  return { title, body };
}

function fireBrowserNotification(agent, stateLabel) {
  const enabled = isBrowserNotifyEnabled();
  const hasAPI = typeof Notification !== 'undefined';
  const perm = hasAPI ? Notification.permission : 'no-api';

  if (!enabled || !hasAPI || perm !== 'granted') return;
  if (document.visibilityState === 'visible' && document.hasFocus()) return;

  const { title, body } = formatNotification(agent, stateLabel);
  const opts = {
    body,
    icon: '/icons/icon-192.png',
    tag: agent.session_id,
    data: { agentId: agent.session_id },
  };

  try {
    if (swReg) {
      swReg.showNotification(title, opts).then(
        null,
        err => console.error('[notify] SW showNotification FAILED:', err)
      );
    } else {
      const n = new Notification(title, opts);
      n.onclick = () => {
        window.focus();
        window.Dashboard.selectAgent(agent.session_id);
        n.close();
      };
    }
  } catch (err) {
    console.error('[notify] ERROR:', err);
  }
}

// Seed the state map without firing notifications
export function initNotify(agents) {
  if (seeded) return;
  for (const agent of agents) {
    prevStateMap.set(agent.session_id, agent.state);
    prevTrustMap.set(agent.session_id, !!agent.trust_prompt_detected);
  }
  seeded = true;
  console.log('[notify] seeded with', agents.length, 'agents');
}

// Diff new agent states against previous, fire browser notification for transitions
export function processNotifications(newAgents) {
  if (!seeded) {
    initNotify(newAgents);
    return;
  }

  const currentIds = new Set();
  for (const agent of newAgents) {
    const id = agent.session_id;
    currentIds.add(id);
    const newState = agent.state;
    const oldState = prevStateMap.get(id);
    prevStateMap.set(id, newState);

    const label = NOTIFY_STATES[newState];
    if (newState !== oldState && label) {
      console.log('[notify] transition:', id, oldState, '→', newState);
      fireBrowserNotification(agent, label);
    }

    const newTrust = !!agent.trust_prompt_detected;
    const oldTrust = !!prevTrustMap.get(id);
    prevTrustMap.set(id, newTrust);
    if (newTrust && !oldTrust) {
      const dir = basename(agent.worktree_cwd || agent.cwd || '') || 'this folder';
      toast(`Trust prompt in ${dir} — accept in tmux to continue`, 'error');
    }
  }

  // Clean up removed agents
  for (const id of prevStateMap.keys()) {
    if (!currentIds.has(id)) {
      prevStateMap.delete(id);
      prevTrustMap.delete(id);
    }
  }
}

export function isBrowserNotifyEnabled() {
  try { return localStorage.getItem(STORAGE_KEY) === 'true'; } catch { return false; }
}

// Toggle browser notifications. Returns { enabled, permission }.
export async function toggleBrowserNotifications() {
  const wasEnabled = isBrowserNotifyEnabled();

  if (wasEnabled) {
    try { localStorage.setItem(STORAGE_KEY, 'false'); } catch {}
    return { enabled: false, permission: typeof Notification !== 'undefined' ? Notification.permission : 'unsupported' };
  }

  // Enabling — request permission if needed
  if (typeof Notification === 'undefined') {
    return { enabled: false, permission: 'unsupported' };
  }

  let permission = Notification.permission;
  if (permission === 'default') {
    permission = await Notification.requestPermission();
  }

  if (permission === 'granted') {
    try { localStorage.setItem(STORAGE_KEY, 'true'); } catch {}
    return { enabled: true, permission };
  }

  return { enabled: false, permission };
}
