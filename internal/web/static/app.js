// Agent Dashboard — ES Module entry point
import { renderList } from './js/pages/list.js';
import { renderDetail, showModal, toast, updateActionBar, appendUserMessage, refreshActiveTab, refreshDetailHeader, stopConversationPoll } from './js/pages/detail.js';
import { renderUsage } from './js/pages/usage.js';
import { renderCreate } from './js/pages/create.js';
import { get, post, cancelNav } from './js/api.js';
import { UI } from './js/ui.js';
import { Theme } from './js/theme.js';
import { initNotify, processNotifications, toggleBrowserNotifications } from './js/notify.js';

// Configure marked.js if available
if (typeof marked !== 'undefined') {
  marked.setOptions({ breaks: true, gfm: true });
}

// --- Shared state ---
const app = document.getElementById('app');
let agents = [];
let selectedAgentId = null;
let currentView = 'list'; // 'list' | 'detail' | 'usage' | 'create'
let eventSource = null;

function setView(view, agentId) {
  currentView = view;
  selectedAgentId = agentId || null;
  document.body.classList.toggle('view-detail', view === 'detail');
  try { sessionStorage.setItem('dashboard-view', JSON.stringify({ view, agentId: agentId || null })); } catch {}
}

// --- History navigation ---
function pushView(view, agentId) {
  const state = { view, agentId: agentId || null };
  history.pushState(state, '', null);
}

function navigateTo(view, agentId, push) {
  switch (view) {
    case 'list':
      cancelNav();
      stopConversationPoll();
      setView('list');
      renderList(app, agents);
      break;
    case 'detail':
      if (agentId) renderDetail(app, agents, agentId, setView);
      else navigateTo('list', null, false);
      break;
    case 'usage':
      stopConversationPoll();
      setView('usage');
      renderUsage(app, agents);
      break;
    case 'create':
      stopConversationPoll();
      setView('create');
      renderCreate(app, agents);
      break;
    default:
      navigateTo('list', null, false);
  }
  if (push) pushView(view, agentId);
}

window.addEventListener('popstate', (e) => {
  const state = e.state;
  if (state && state.view) {
    navigateTo(state.view, state.agentId, false);
  } else {
    navigateTo('list', null, false);
  }
});

// Wrap an async action with button spinner feedback.
async function withSpinner(evt, fn) {
  const btn = evt && evt.target ? evt.target.closest('button') : null;
  let origHtml;
  if (btn) { origHtml = btn.innerHTML; btn.disabled = true; btn.innerHTML += UI.spinner(); }
  try { await fn(); } finally { if (btn) { btn.innerHTML = origHtml; btn.disabled = false; } }
}

// --- SSE ---
function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/events');
  eventSource.onmessage = (e) => {
    try {
      agents = JSON.parse(e.data);
      if (currentView === 'list') renderList(app, agents);
      else if (currentView === 'detail' && selectedAgentId) {
        const agent = agents.find(a => a.session_id === selectedAgentId);
        if (agent) {
          updateActionBar(agent);
          refreshDetailHeader(agent);
        }
        refreshActiveTab(selectedAgentId);
      }
      try { processNotifications(agents); } catch (err) { console.error('[notify] error:', err); }
    } catch (err) { /* ignore parse errors */ }
  };
  eventSource.onerror = () => {
    // Auto-reconnects
  };
}

// --- Public API (used by onclick handlers in HTML strings) ---
window.Dashboard = {
  showList() {
    navigateTo('list', null, true);
  },

  showUsage() {
    navigateTo('usage', null, true);
  },

  showCreate() {
    navigateTo('create', null, true);
  },

  selectAgent(id) {
    navigateTo('detail', id, true);
  },

  async approve(id, evt) {
    await withSpinner(evt, async () => {
      const result = await post('/api/agents/' + id + '/approve');
      if (result && result.ok) toast('Approved', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  async reject(id, evt) {
    await withSpinner(evt, async () => {
      const result = await post('/api/agents/' + id + '/reject');
      if (result && result.ok) toast('Rejected', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  async sendInput(id) {
    const input = document.getElementById('reply-input');
    if (!input || !input.value.trim()) return;
    const text = input.value.trim();
    input.value = '';
    input.disabled = true;
    appendUserMessage(text);
    try {
      const result = await post('/api/agents/' + id + '/input', { text });
      if (!result || !result.ok) toast('Failed: ' + (result?.error || 'unknown'), 'error');
    } finally {
      if (input) input.disabled = false;
    }
  },

  confirmStop(id) {
    showModal('Stop Agent', 'Send Ctrl+C to this agent?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/stop');
        if (result && result.ok) toast('Stopped', 'success');
        else toast('Failed: ' + (result?.error || 'unknown'), 'error');
      });
    });
  },

  confirmMerge(id) {
    // Capture branch before async merge — SSE may update agents mid-flight
    const agentPre = agents.find(a => a.session_id === id);
    const branch = agentPre ? agentPre.branch : '';
    showModal('Merge PR', 'Merge this PR with --squash?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/merge');
        if (result && result.ok) {
          toast('Merged', 'success');
          const label = branch ? `Clean up ${branch}?` : 'Clean up worktree and branch?';
          showModal('Post-Merge Cleanup', label + ' This will remove the worktree, checkout the default branch, pull, and delete the local feature branch.', async (cleanEvt) => {
            await withSpinner(cleanEvt, async () => {
              const cleanResult = await post('/api/agents/' + id + '/cleanup');
              if (cleanResult && cleanResult.ok) {
                toast('Cleaned up', 'success');
                navigateTo('list', null, true);
              } else {
                toast('Cleanup failed: ' + (cleanResult?.error || 'unknown'), 'error');
              }
            });
          });
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    });
  },

  confirmClose(id) {
    showModal('Close Agent', 'Kill the tmux pane and remove this agent?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/close');
        if (result && result.ok) {
          toast('Closed', 'success');
          navigateTo('list', null, true);
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    });
  },

  cycleTheme() { Theme.cycle(); },

  async toggleNotifications() {
    const result = await toggleBrowserNotifications();
    if (result.permission === 'unsupported') {
      toast('Notifications not supported in this browser', 'error');
    } else if (result.permission === 'denied') {
      toast('Notifications blocked — check browser settings', 'error');
    } else if (result.enabled) {
      toast('Notifications enabled', 'success');
    } else {
      toast('Notifications disabled', 'success');
    }
    // Re-render current view to update bell icon
    if (currentView === 'list') renderList(app, agents);
    else if (currentView === 'detail' && selectedAgentId) renderDetail(app, agents, selectedAgentId, setView);
    else if (currentView === 'usage') renderUsage(app, agents);
    else if (currentView === 'create') renderCreate(app, agents);
  },

  async openPR(id) {
    const agent = agents.find(a => a.session_id === id);
    if (agent && agent.pr_url) {
      window.open(agent.pr_url, '_blank');
      return;
    }
    try {
      const resp = await fetch(`/api/agents/${encodeURIComponent(id)}/pr-url`);
      if (!resp.ok) throw new Error('failed to resolve PR URL');
      const data = await resp.json();
      if (data.url) {
        window.open(data.url, '_blank');
        return;
      }
    } catch {}
    toast('No PR URL available', 'error');
  },

  async createAgent(evt) {
    const folder = document.getElementById('create-folder')?.value?.trim();
    const skill = document.getElementById('create-skill')?.value?.trim();
    const message = document.getElementById('create-message')?.value?.trim();

    if (!folder) {
      toast('Folder is required', 'error');
      return;
    }

    await withSpinner(evt, async () => {
      const result = await post('/api/agents/create', { folder, skill, message });
      if (result && result.ok) {
        toast('Agent created', 'success');
        navigateTo('list', null, true);
      } else {
        toast('Failed: ' + (result?.error || 'unknown'), 'error');
      }
    });
  },

  toggleExpand(btn) {
    const span = btn.previousElementSibling;
    if (!span) return;
    const full = span.getAttribute('data-full');
    const isTruncated = span.getAttribute('data-truncated') === 'true';
    if (isTruncated) {
      span.textContent = full;
      span.setAttribute('data-truncated', 'false');
      btn.textContent = 'Show less';
    } else {
      span.textContent = full.substring(0, 200) + '...';
      span.setAttribute('data-truncated', 'true');
      btn.textContent = 'Show more';
    }
  },
};

// --- Service worker messages ---
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data && e.data.type === 'navigate-agent' && e.data.agentId) {
      navigateTo('detail', e.data.agentId, true);
    }
  });
}

// --- Theme ---
Theme.init();

// --- Init ---
async function init() {
  const data = await get('/api/agents');
  if (data) {
    agents = data;
    initNotify(agents);
  }

  // Restore saved view state
  let restored = false;
  try {
    const saved = JSON.parse(sessionStorage.getItem('dashboard-view'));
    if (saved && saved.view) {
      if (saved.view === 'detail' && saved.agentId && agents.find(a => a.session_id === saved.agentId)) {
        navigateTo('detail', saved.agentId, false);
        restored = true;
      } else if (saved.view === 'usage') {
        navigateTo('usage', null, false);
        restored = true;
      } else if (saved.view === 'create') {
        navigateTo('create', null, false);
        restored = true;
      }
    }
  } catch {}

  if (!restored) navigateTo('list', null, false);

  // Set initial history entry so popstate has something to land on
  history.replaceState({ view: currentView, agentId: selectedAgentId }, '', null);

  connectSSE();
}

init();
