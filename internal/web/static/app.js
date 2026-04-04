// Agent Dashboard — Vanilla JS SPA
(function () {
  'use strict';

  const app = document.getElementById('app');
  let agents = [];
  let selectedAgentId = null;
  let currentView = 'list'; // 'list' | 'detail' | 'usage' | 'create'
  let currentTab = 'conversation';
  let eventSource = null;

  // --- API helpers ---
  async function api(method, path, body) {
    const opts = {
      method,
      headers: { 'X-Requested-With': 'dashboard' },
    };
    if (body) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const resp = await fetch(path, opts);
    if (resp.status === 401) {
      window.location.href = '/auth/login';
      return null;
    }
    return resp.json();
  }

  function get(path) { return api('GET', path); }
  function post(path, body) { return api('POST', path, body); }

  // --- SSE ---
  function connectSSE() {
    if (eventSource) eventSource.close();
    eventSource = new EventSource('/events');
    eventSource.onmessage = (e) => {
      try {
        agents = JSON.parse(e.data);
        if (currentView === 'list') renderList();
      } catch (err) { /* ignore parse errors */ }
    };
    eventSource.onerror = () => {
      // Auto-reconnects; could show indicator
    };
  }

  // --- Toast ---
  function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast ' + (type || '');
    el.textContent = msg;
    document.body.appendChild(el);
    requestAnimationFrame(() => el.classList.add('visible'));
    setTimeout(() => {
      el.classList.remove('visible');
      setTimeout(() => el.remove(), 300);
    }, 2500);
  }

  // --- Helpers ---
  function repoName(agent) {
    const dir = agent.worktree_cwd || agent.cwd || '';
    const parts = dir.replace(/\/+$/, '').split('/');
    return parts[parts.length - 1] || 'unknown';
  }

  function statePriority(state) {
    const map = { permission: 1, plan: 1, question: 2, error: 2, running: 3, idle_prompt: 4, done: 4, pr: 5, merged: 6 };
    return map[state] || 99;
  }

  function stateGroup(state) {
    const p = statePriority(state);
    if (p === 1) return 'BLOCKED';
    if (p === 2) return 'WAITING';
    if (p === 3) return 'RUNNING';
    if (p === 4) return 'REVIEW';
    if (p === 5) return 'PR';
    if (p === 6) return 'MERGED';
    return 'OTHER';
  }

  function effectiveState(agent) {
    return agent.pinned_state || agent.state;
  }

  function duration(agent) {
    if (!agent.started_at) return '';
    const start = new Date(agent.started_at);
    const now = new Date();
    const mins = Math.floor((now - start) / 60000);
    if (mins < 60) return mins + 'm';
    return Math.floor(mins / 60) + 'h ' + (mins % 60) + 'm';
  }

  function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // --- Render: Agent List ---
  function renderList() {
    currentView = 'list';
    const grouped = {};
    for (const agent of agents) {
      const g = stateGroup(effectiveState(agent));
      if (!grouped[g]) grouped[g] = [];
      grouped[g].push(agent);
    }

    const order = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];
    let html = `
      <div class="header">
        <h1>Agent Dashboard</h1>
        <div class="header-actions">
          <button class="header-btn" onclick="Dashboard.showUsage()">Usage</button>
          <button class="header-btn" onclick="Dashboard.showCreate()">+ New</button>
        </div>
      </div>
      <div class="agent-list">
    `;

    if (agents.length === 0) {
      html += `<div class="empty-state"><h2>No agents running</h2><p>Create a new agent to get started</p></div>`;
    }

    for (const group of order) {
      if (!grouped[group] || grouped[group].length === 0) continue;
      html += `<div class="state-group"><div class="state-group-label">${group} (${grouped[group].length})</div>`;
      for (const agent of grouped[group]) {
        const st = effectiveState(agent);
        html += `
          <div class="agent-card" onclick="Dashboard.selectAgent('${agent.session_id}')">
            <div class="agent-card-header">
              <span class="agent-name">${escapeHtml(repoName(agent))}</span>
              <span class="badge badge-${st}">${st}</span>
            </div>
            <div class="agent-meta">
              ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
              ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
              ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
              ${agent.subagent_count > 0 ? '<span>' + agent.subagent_count + ' subagents</span>' : ''}
            </div>
            ${agent.last_message_preview ? '<div class="agent-preview">' + escapeHtml(agent.last_message_preview) + '</div>' : ''}
          </div>
        `;
      }
      html += `</div>`;
    }

    html += `</div>`;
    app.innerHTML = html;
  }

  // --- Render: Agent Detail ---
  async function renderDetail(agentId) {
    currentView = 'detail';
    selectedAgentId = agentId;
    const agent = agents.find(a => a.session_id === agentId);
    if (!agent) { renderList(); return; }

    const st = effectiveState(agent);
    app.innerHTML = `
      <div class="detail-header">
        <button class="detail-back" onclick="Dashboard.showList()">&larr; Back</button>
        <div class="detail-title">
          <h2>${escapeHtml(repoName(agent))}</h2>
          <span class="badge badge-${st}">${st}</span>
        </div>
        <div class="detail-meta">
          ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
          ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
          ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
        </div>
      </div>
      <div class="tabs">
        <button class="tab active" data-tab="conversation">Chat</button>
        <button class="tab" data-tab="activity">Activity</button>
        <button class="tab" data-tab="diff">Diff</button>
        <button class="tab" data-tab="plan">Plan</button>
        <button class="tab" data-tab="files">Files</button>
        <button class="tab" data-tab="subagents">Subagents</button>
      </div>
      <div id="tab-conversation" class="tab-content active"><div class="loading"><span class="spinner"></span></div></div>
      <div id="tab-activity" class="tab-content"><div class="loading"><span class="spinner"></span></div></div>
      <div id="tab-diff" class="tab-content"><div class="loading"><span class="spinner"></span></div></div>
      <div id="tab-plan" class="tab-content"><div class="loading"><span class="spinner"></span></div></div>
      <div id="tab-files" class="tab-content"></div>
      <div id="tab-subagents" class="tab-content"><div class="loading"><span class="spinner"></span></div></div>
      ${renderActionBar(agent)}
    `;

    // Tab switching
    document.querySelectorAll('.tab').forEach(tab => {
      tab.addEventListener('click', () => {
        document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
        tab.classList.add('active');
        const target = tab.dataset.tab;
        document.getElementById('tab-' + target).classList.add('active');
        currentTab = target;
        loadTabContent(target, agentId);
      });
    });

    // Load initial tab
    loadTabContent('conversation', agentId);
    renderFilesTab(agent);
  }

  async function loadTabContent(tab, agentId) {
    const container = document.getElementById('tab-' + tab);
    if (!container) return;

    switch (tab) {
      case 'conversation': {
        const entries = await get('/api/agents/' + agentId + '/conversation');
        if (!entries || entries.length === 0) {
          container.innerHTML = '<div class="empty-state"><p>No conversation yet</p></div>';
          return;
        }
        let html = '<div class="conversation">';
        for (const entry of entries) {
          const role = entry.Role || entry.role;
          const cls = role === 'human' ? 'msg-human' : 'msg-assistant';
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';
          html += `<div class="msg ${cls}">${escapeHtml(content)}<div class="msg-time">${escapeHtml(time)}</div></div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        container.scrollTop = container.scrollHeight;
        break;
      }
      case 'activity': {
        const entries = await get('/api/agents/' + agentId + '/activity');
        if (!entries || entries.length === 0) {
          container.innerHTML = '<div class="empty-state"><p>No activity yet</p></div>';
          return;
        }
        let html = '<div class="activity-log">';
        for (const e of entries) {
          const kind = e.Kind || e.kind || 'tool';
          const cls = 'activity-' + kind;
          html += `<div class="activity-entry"><span class="activity-time">[${escapeHtml(e.Timestamp || e.timestamp || '')}]</span> <span class="${cls}">${escapeHtml(e.Content || e.content || '')}</span></div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        break;
      }
      case 'diff': {
        const data = await get('/api/agents/' + agentId + '/diff');
        if (!data || !data.raw) {
          container.innerHTML = '<div class="empty-state"><p>No diff available</p></div>';
          return;
        }
        container.innerHTML = '<div class="diff-container">' + renderDiffHtml(data.raw) + '</div>';
        break;
      }
      case 'plan': {
        const data = await get('/api/agents/' + agentId + '/plan');
        if (!data || !data.content) {
          container.innerHTML = '<div class="empty-state"><p>No plan available</p></div>';
          return;
        }
        container.innerHTML = '<div class="plan-content">' + renderMarkdown(data.content) + '</div>';
        break;
      }
      case 'subagents': {
        const subs = await get('/api/agents/' + agentId + '/subagents');
        if (!subs || subs.length === 0) {
          container.innerHTML = '<div class="empty-state"><p>No subagents</p></div>';
          return;
        }
        let html = '<div class="subagent-list">';
        for (const sub of subs) {
          const icon = sub.Completed || sub.completed ? '&#10003;' : '&#9654;';
          const cls = sub.Completed || sub.completed ? 'subagent-icon-completed' : 'subagent-icon-running';
          const type = sub.AgentType || sub.agent_type || '';
          const desc = sub.Description || sub.description || '';
          html += `<div class="subagent-item"><span class="${cls}">${icon}</span> <strong>${escapeHtml(type)}</strong>: ${escapeHtml(desc)}</div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        break;
      }
    }
  }

  function renderFilesTab(agent) {
    const container = document.getElementById('tab-files');
    if (!container) return;
    const files = agent.files_changed || [];
    if (files.length === 0) {
      container.innerHTML = '<div class="empty-state"><p>No files changed</p></div>';
      return;
    }
    let html = '<div class="files-list">';
    for (const f of files) {
      let cls = 'file-modified';
      if (f.startsWith('+')) cls = 'file-added';
      else if (f.startsWith('-')) cls = 'file-deleted';
      html += `<div class="file-item ${cls}">${escapeHtml(f)}</div>`;
    }
    html += '</div>';
    container.innerHTML = html;
  }

  function renderDiffHtml(raw) {
    const lines = raw.split('\n');
    let html = '';
    for (const line of lines) {
      if (line.startsWith('diff --git')) {
        html += `<div class="diff-file-header">${escapeHtml(line)}</div>`;
      } else if (line.startsWith('@@')) {
        html += `<div class="diff-hunk-header">${escapeHtml(line)}</div>`;
      } else if (line.startsWith('+') && !line.startsWith('+++')) {
        html += `<div class="diff-line diff-add">${escapeHtml(line)}</div>`;
      } else if (line.startsWith('-') && !line.startsWith('---')) {
        html += `<div class="diff-line diff-del">${escapeHtml(line)}</div>`;
      } else {
        html += `<div class="diff-line">${escapeHtml(line)}</div>`;
      }
    }
    return html;
  }

  function renderMarkdown(md) {
    // Basic markdown rendering — headings, code blocks, bold, lists
    let html = escapeHtml(md);
    // Code blocks
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>');
    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    // Headers
    html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
    html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
    html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
    // Bold
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    // Lists
    html = html.replace(/^- (.+)$/gm, '<li>$1</li>');
    // Line breaks
    html = html.replace(/\n/g, '<br>');
    return html;
  }

  // --- Action bar ---
  function renderActionBar(agent) {
    const st = effectiveState(agent);
    let actions = '';

    // Claude deep link
    actions += `<button class="action-btn action-btn-claude" onclick="Dashboard.openClaude()">Open Claude</button>`;

    if (st === 'permission' || st === 'plan') {
      actions += `<button class="action-btn action-btn-approve" onclick="Dashboard.approve('${agent.session_id}')">Approve</button>`;
      actions += `<button class="action-btn action-btn-reject" onclick="Dashboard.reject('${agent.session_id}')">Reject</button>`;
    } else if (st === 'question' || st === 'error') {
      actions += `<input class="action-input" id="reply-input" placeholder="Type a reply..." onkeydown="if(event.key==='Enter')Dashboard.sendInput('${agent.session_id}')">`;
      actions += `<button class="action-btn action-btn-send" onclick="Dashboard.sendInput('${agent.session_id}')">Send</button>`;
    } else if (st === 'pr') {
      actions += `<button class="action-btn action-btn-open-pr" onclick="Dashboard.openPR('${agent.session_id}')">Open PR</button>`;
      actions += `<button class="action-btn action-btn-merge" onclick="Dashboard.confirmMerge('${agent.session_id}')">Merge</button>`;
    } else if (st === 'merged') {
      actions += `<button class="action-btn action-btn-close" onclick="Dashboard.confirmClose('${agent.session_id}')">Close</button>`;
    }

    // Stop button always present for running agents
    if (st === 'running' || st === 'permission' || st === 'plan' || st === 'question') {
      actions += `<button class="action-btn action-btn-stop" onclick="Dashboard.confirmStop('${agent.session_id}')">Stop</button>`;
    }

    return `<div class="action-bar">${actions}</div>`;
  }

  // --- Modal ---
  function showModal(title, message, onConfirm) {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
      <div class="modal">
        <h3>${escapeHtml(title)}</h3>
        <p>${escapeHtml(message)}</p>
        <div class="modal-actions">
          <button class="modal-cancel" id="modal-cancel">Cancel</button>
          <button class="modal-confirm" id="modal-confirm">Confirm</button>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);
    overlay.querySelector('#modal-cancel').onclick = () => overlay.remove();
    overlay.querySelector('#modal-confirm').onclick = () => {
      overlay.remove();
      onConfirm();
    };
    overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
  }

  // --- Usage view ---
  async function renderUsage() {
    currentView = 'usage';
    app.innerHTML = `
      <div class="header">
        <h1>Usage</h1>
        <div class="header-actions">
          <button class="header-btn" onclick="Dashboard.showList()">&larr; Back</button>
        </div>
      </div>
      <div class="usage-view"><div class="loading"><span class="spinner"></span></div></div>
    `;

    const data = await get('/api/usage/daily');
    if (!data) return;

    const days = data.days || [];
    const maxCost = Math.max(...days.map(d => d.cost_usd), 0.01);

    let chartHtml = '<div class="usage-chart">';
    for (const day of days) {
      const height = Math.max(2, (day.cost_usd / maxCost) * 100);
      const label = day.date.slice(5); // MM-DD
      chartHtml += `<div class="usage-bar" style="height:${height}%"><span class="usage-bar-label">${label}</span></div>`;
    }
    chartHtml += '</div>';

    document.querySelector('.usage-view').innerHTML = `
      <div class="usage-summary">
        <div class="usage-card">
          <div class="usage-card-value">$${(data.today_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">Today</div>
        </div>
        <div class="usage-card">
          <div class="usage-card-value">$${(data.total_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">All Time</div>
        </div>
      </div>
      <h3 style="margin-bottom:8px">Last 7 Days</h3>
      ${chartHtml}
    `;
  }

  // --- Create agent view ---
  function renderCreate() {
    currentView = 'create';
    // Gather recent folders from existing agents
    const folders = [...new Set(agents.map(a => a.cwd).filter(Boolean))];

    app.innerHTML = `
      <div class="header">
        <h1>New Agent</h1>
        <div class="header-actions">
          <button class="header-btn" onclick="Dashboard.showList()">&larr; Back</button>
        </div>
      </div>
      <div style="padding:16px">
        <div style="margin-bottom:16px">
          <label style="display:block;font-size:13px;color:var(--subtext0);margin-bottom:4px">Folder</label>
          <input id="create-folder" class="action-input" style="width:100%" placeholder="/path/to/repo" list="folder-suggestions">
          <datalist id="folder-suggestions">
            ${folders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
          </datalist>
        </div>
        <div style="margin-bottom:16px">
          <label style="display:block;font-size:13px;color:var(--subtext0);margin-bottom:4px">Skill (optional)</label>
          <input id="create-skill" class="action-input" style="width:100%" placeholder="e.g. feature, chore">
        </div>
        <div style="margin-bottom:16px">
          <label style="display:block;font-size:13px;color:var(--subtext0);margin-bottom:4px">Message (optional)</label>
          <textarea id="create-message" class="action-input" style="width:100%;min-height:80px;resize:vertical" placeholder="What should the agent do?"></textarea>
        </div>
        <button class="action-btn action-btn-approve" style="width:100%;max-width:none" onclick="Dashboard.createAgent()">Create Agent</button>
      </div>
    `;
  }

  // --- Public API (exposed as Dashboard global) ---
  window.Dashboard = {
    showList() {
      renderList();
    },

    showUsage() {
      renderUsage();
    },

    showCreate() {
      renderCreate();
    },

    selectAgent(id) {
      renderDetail(id);
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
          renderList();
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    },

    openClaude() {
      window.open('https://claude.ai', '_blank');
    },

    openPR(id) {
      // TODO: construct GitHub PR URL from agent branch
      const agent = agents.find(a => a.session_id === id);
      if (agent && agent.branch) {
        // Try to open GitHub compare page
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
        renderList();
      } else {
        toast('Failed: ' + (result?.error || 'unknown'), 'error');
      }
    },
  };

  // --- Init ---
  async function init() {
    // Load initial agents
    const data = await get('/api/agents');
    if (data) agents = data;
    renderList();
    connectSSE();
  }

  init();
})();
