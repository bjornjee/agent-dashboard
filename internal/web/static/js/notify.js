// Notification system — state transition detection and browser Notification API.

const STORAGE_KEY = 'notify-enabled';

// Previous state map: sessionId → state
const prevStateMap = new Map();
let seeded = false;

// Cache the SW registration so we don't await navigator.serviceWorker.ready on every notification
let swReg = null;
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.ready.then(reg => { swReg = reg; });
}

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

function fireBrowserNotification(agent, body) {
  const enabled = isBrowserNotifyEnabled();
  const hasAPI = typeof Notification !== 'undefined';
  const perm = hasAPI ? Notification.permission : 'no-api';
  const vis = document.visibilityState;
  const focused = document.hasFocus();
  console.log('[notify] fire check:', { enabled, perm, vis, focused, swReg: !!swReg });

  if (!enabled || !hasAPI || perm !== 'granted') {
    console.log('[notify] BLOCKED:', !enabled ? 'disabled' : !hasAPI ? 'no Notification API' : 'permission=' + perm);
    return;
  }
  if (vis === 'visible' && focused) {
    console.log('[notify] SKIPPED: tab is focused');
    return;
  }

  const title = agentLabel(agent);
  const opts = {
    body,
    icon: '/icon-192.svg',
    tag: agent.session_id,
    data: { agentId: agent.session_id },
  };

  try {
    if (swReg) {
      swReg.showNotification(title, opts).then(
        () => console.log('[notify] SW showNotification OK'),
        err => console.error('[notify] SW showNotification FAILED:', err)
      );
    } else {
      console.log('[notify] no SW reg, using new Notification()');
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
