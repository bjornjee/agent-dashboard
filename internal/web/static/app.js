// Agent Dashboard — ES Module entry point
import { renderList } from './js/pages/list.js';
import { renderDetail, showModal, toast } from './js/pages/detail.js';
import { renderUsage } from './js/pages/usage.js';
import { renderCreate } from './js/pages/create.js';
import { get, post, cancelNav } from './js/api.js';

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

  async approve(id) {
    const result = await post('/api/agents/' + id + '/approve');
    if (result && result.ok) toast('Approved', 'success');
    else toast('Failed: ' + (result?.error || 'unknown'), 'error');
  },

  async reject(id) {
    const result = await post('/api/agents/' + id + '/reject');
    if (result && result.ok) toast('Rejected', 'success');
    else toast('Failed: ' + (result?.error || 'unknown'), 'error');
  },

  async sendInput(id) {
    const input = document.getElementById('reply-input');
    if (!input || !input.value.trim()) return;
    const text = input.value.trim();
    input.value = '';
    const result = await post('/api/agents/' + id + '/input', { text });
    if (result && result.ok) toast('Sent', 'success');
    else toast('Failed: ' + (result?.error || 'unknown'), 'error');
  },

  confirmStop(id) {
    showModal('Stop Agent', 'Send Ctrl+C to this agent?', async () => {
      const result = await post('/api/agents/' + id + '/stop');
      if (result && result.ok) toast('Stopped', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  confirmMerge(id) {
    showModal('Merge PR', 'Merge this PR with --squash?', async () => {
      const result = await post('/api/agents/' + id + '/merge');
      if (result && result.ok) toast('Merged', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  confirmClose(id) {
    showModal('Close Agent', 'Kill the tmux pane and remove this agent?', async () => {
      const result = await post('/api/agents/' + id + '/close');
      if (result && result.ok) {
        toast('Closed', 'success');
        renderList(app, agents);
      } else {
        toast('Failed: ' + (result?.error || 'unknown'), 'error');
      }
    });
  },

  openClaude() { window.open('https://claude.ai', '_blank'); },

  openPR(id) {
    const agent = agents.find(a => a.session_id === id);
    if (agent && agent.branch) {
      toast('Opening PR page...', 'success');
    }
  },

  async createAgent() {
    const folder = document.getElementById('create-folder')?.value?.trim();
    const skill = document.getElementById('create-skill')?.value?.trim();
    const message = document.getElementById('create-message')?.value?.trim();

    if (!folder) {
      toast('Folder is required', 'error');
      return;
    }

    const result = await post('/api/agents/create', { folder, skill, message });
    if (result && result.ok) {
      toast('Agent created', 'success');
      renderList(app, agents);
    } else {
      toast('Failed: ' + (result?.error || 'unknown'), 'error');
    }
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
async function init() {
  const data = await get('/api/agents');
  if (data) agents = data;
  renderList(app, agents);
  connectSSE();
}

init();
