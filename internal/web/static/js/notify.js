// Notification system — state transition detection and browser Notification API.

const STORAGE_KEY = 'notify-enabled';

// Previous state map: sessionId → state
const prevStateMap = new Map();
let seeded = false;

// States that warrant a notification when an agent transitions INTO them
const NOTIFY_STATES = {
  permission: 'Needs permission',
  plan:       'Plan ready for review',
  question:   'Has a question',
  error:      'Hit an error',
  done:       'Finished',
  idle_prompt:'Finished',
  pr:         'PR ready',
};

function agentLabel(agent) {
  return agent.task || agent.worktree || agent.session_id;
}

async function fireBrowserNotification(agent, body) {
  const enabled = isBrowserNotifyEnabled();
  const hasAPI = typeof Notification !== 'undefined';
  const perm = hasAPI ? Notification.permission : 'no-api';
  const vis = document.visibilityState;
  const focused = document.hasFocus();
  console.log('[notify] fire check:', { enabled, hasAPI, perm, vis, focused, agent: agent.session_id });

  if (!enabled || !hasAPI || perm !== 'granted') {
    console.log('[notify] BLOCKED:', !enabled ? 'disabled' : !hasAPI ? 'no Notification API' : 'permission=' + perm);
    return;
  }
  if (vis === 'visible' && focused) {
    console.log('[notify] SKIPPED: tab is focused');
    return;
  }
  try {
    const hasSW = !!navigator.serviceWorker;
    console.log('[notify] dispatching via', hasSW ? 'SW showNotification' : 'new Notification');
    const reg = hasSW && await navigator.serviceWorker.ready;
    if (reg) {
      await reg.showNotification(agentLabel(agent), {
        body,
        icon: '/icon-192.svg',
        tag: agent.session_id,
        data: { agentId: agent.session_id },
      });
      console.log('[notify] SW showNotification OK');
    } else {
      const n = new Notification(agentLabel(agent), {
        body,
        icon: '/icon-192.svg',
        tag: agent.session_id,
        data: { agentId: agent.session_id },
      });
      n.onclick = () => {
        window.focus();
        window.Dashboard.selectAgent(agent.session_id);
        n.close();
      };
      console.log('[notify] new Notification OK');
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

    if (newState !== oldState && NOTIFY_STATES[newState]) {
      console.log('[notify] transition:', id, oldState, '→', newState);
      fireBrowserNotification(agent, NOTIFY_STATES[newState]);
    }
  }

  // Clean up removed agents
  for (const id of prevStateMap.keys()) {
    if (!currentIds.has(id)) prevStateMap.delete(id);
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
