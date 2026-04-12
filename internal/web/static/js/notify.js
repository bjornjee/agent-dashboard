// Notification system — state transition detection, in-app nudge, and browser notifications.
import { UI } from './ui.js';

const STORAGE_KEY = 'notify-enabled';

// Previous state map: sessionId → state
const prevStateMap = new Map();
let seeded = false;

// States that warrant a notification when an agent transitions INTO them
const NOTIFY_STATES = {
  permission: { type: 'blocked', message: 'needs permission' },
  plan:       { type: 'blocked', message: 'plan ready for review' },
  question:   { type: 'waiting', message: 'has a question' },
  error:      { type: 'waiting', message: 'hit an error' },
  done:       { type: 'review',  message: 'finished' },
  idle_prompt:{ type: 'review',  message: 'finished' },
  pr:         { type: 'review',  message: 'PR ready' },
};

function agentLabel(agent) {
  return agent.task || agent.worktree || agent.session_id;
}

function fireBrowserNotification(agent, info) {
  if (!isBrowserNotifyEnabled() || typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
  try {
    const n = new Notification(agentLabel(agent), {
      body: info.message.charAt(0).toUpperCase() + info.message.slice(1),
      icon: '/icon-192.svg',
      tag: agent.session_id,
      data: { agentId: agent.session_id },
    });
    n.onclick = () => {
      window.focus();
      window.Dashboard.selectAgent(agent.session_id);
      n.close();
    };
  } catch { /* notification constructor can throw in some contexts */ }
}

// Seed the state map without firing notifications (called on first SSE message)
export function initNotify(agents) {
  if (seeded) return;
  const snapshot = {};
  for (const agent of agents) {
    prevStateMap.set(agent.session_id, agent.state);
    snapshot[agent.session_id] = agent.state;
  }
  seeded = true;
  console.log('[notify] seeded with', agents.length, 'agents:', snapshot);
}

// Diff new agent states against previous, fire nudge + browser notification for transitions
export function processNotifications(newAgents) {
  if (!seeded) {
    console.log('[notify] not seeded yet, seeding from SSE');
    initNotify(newAgents);
    return;
  }

  const currentIds = new Set();
  for (const agent of newAgents) {
    const id = agent.session_id;
    currentIds.add(id);
    const newState = agent.state;
    const oldState = prevStateMap.get(id);

    if (newState !== oldState) {
      console.log('[notify] transition:', id, oldState, '→', newState,
        NOTIFY_STATES[newState] ? '(WILL NUDGE)' : '(no nudge for this state)');
    }

    prevStateMap.set(id, newState);

    // Only notify on transitions into notify-worthy states
    if (newState !== oldState && NOTIFY_STATES[newState]) {
      const info = NOTIFY_STATES[newState];
      const label = agentLabel(agent);
      console.log('[notify] firing nudge:', label, info.message, info.type);
      UI.nudge(label + ' ' + info.message, {
        type: info.type,
        agentId: id,
        agentName: label,
      });
      fireBrowserNotification(agent, info);
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
