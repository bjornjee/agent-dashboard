// Agent Dashboard — ES Module entry point
import { renderList } from './js/pages/list.js';
import { renderDetail, showModal, toast } from './js/pages/detail.js';
import { renderUsage } from './js/pages/usage.js';
import { renderCreate } from './js/pages/create.js';
import { get, post, cancelNav } from './js/api.js';
import { UI } from './js/ui.js';
import { Theme } from './js/theme.js';

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
  try { sessionStorage.setItem('dashboard-view', JSON.stringify({ view, agentId: agentId || null })); } catch {}
}

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
    } catch (err) { /* ignore parse errors */ }
  };
  eventSource.onerror = () => {
    // Auto-reconnects
  };
}

// --- Public API (used by onclick handlers in HTML strings) ---
window.Dashboard = {
  showList() {
    cancelNav();
    setView('list');
    renderList(app, agents);
  },

  showUsage() {
    setView('usage');
    renderUsage(app, agents);
  },

  showCreate() {
    setView('create');
    renderCreate(app, agents);
  },

  selectAgent(id) {
    renderDetail(app, agents, id, setView);
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

  async sendInput(id, evt) {
    const input = document.getElementById('reply-input');
    if (!input || !input.value.trim()) return;
    const text = input.value.trim();
    input.value = '';
    await withSpinner(evt, async () => {
      const result = await post('/api/agents/' + id + '/input', { text });
      if (result && result.ok) toast('Sent', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
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
    showModal('Merge PR', 'Merge this PR with --squash?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/merge');
        if (result && result.ok) toast('Merged', 'success');
        else toast('Failed: ' + (result?.error || 'unknown'), 'error');
      });
    });
  },

  confirmClose(id) {
    showModal('Close Agent', 'Kill the tmux pane and remove this agent?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/close');
        if (result && result.ok) {
          toast('Closed', 'success');
          renderList(app, agents);
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    });
  },

  cycleTheme() { Theme.cycle(); },

  openClaude() { window.open('https://claude.ai', '_blank'); },

  openPR(id) {
    const agent = agents.find(a => a.session_id === id);
    if (agent && agent.pr_url) {
      window.open(agent.pr_url, '_blank');
    } else {
      toast('No PR URL available', 'error');
    }
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
        renderList(app, agents);
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

// --- Init ---
// --- Theme ---
Theme.init();

// --- Init ---
async function init() {
  const data = await get('/api/agents');
  if (data) agents = data;

  // Restore saved view state
  let restored = false;
  try {
    const saved = JSON.parse(sessionStorage.getItem('dashboard-view'));
    if (saved && saved.view) {
      if (saved.view === 'detail' && saved.agentId && agents.find(a => a.session_id === saved.agentId)) {
        renderDetail(app, agents, saved.agentId, setView);
        restored = true;
      } else if (saved.view === 'usage') {
        setView('usage');
        renderUsage(app, agents);
        restored = true;
      } else if (saved.view === 'create') {
        setView('create');
        renderCreate(app, agents);
        restored = true;
      }
    }
  } catch {}

  if (!restored) renderList(app, agents);
  connectSSE();
}

init();
