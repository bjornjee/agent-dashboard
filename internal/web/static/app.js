// Agent Dashboard — Vanilla JS SPA
(function () {
  'use strict';

  const app = document.getElementById('app');
  let agents = [];
  let selectedAgentId = null;
  let currentView = 'list'; // 'list' | 'detail' | 'usage' | 'create'
  let currentTab = 'conversation';
  let eventSource = null;
  let activityFilter = 'all'; // 'all' | 'human' | 'assistant' | 'tool'
  let navAbort = null; // AbortController for cancelling in-flight tab loads

  // --- SVG Icons ---
  const ICONS = {
    logo: '<svg width="24" height="24" viewBox="0 0 512 512"><rect width="512" height="512" rx="96" fill="var(--accent)"/><text x="256" y="360" font-family="-apple-system,system-ui,sans-serif" font-size="320" font-weight="700" fill="#fff" text-anchor="middle">A</text></svg>',
    robot: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="8" width="18" height="12" rx="2"/><circle cx="9" cy="14" r="1.5"/><circle cx="15" cy="14" r="1.5"/><path d="M12 2v4M8 8V6a4 4 0 018 0v2"/></svg>',
    chat: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/></svg>',
    clipboard: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2"/><rect x="8" y="2" width="8" height="4" rx="1"/></svg>',
    fileDiff: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14,2 14,8 20,8"/><line x1="9" y1="15" x2="15" y2="15"/><line x1="12" y1="12" x2="12" y2="18"/></svg>',
    activity: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="22,12 18,12 15,21 9,3 6,12 2,12"/></svg>',
    chart: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>',
    subagent: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><line x1="12" y1="8" x2="5" y2="16"/><line x1="12" y1="8" x2="19" y2="16"/></svg>',
    // Inline activity icons (small, 16x16)
    human: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M6 21v-2a4 4 0 014-4h4a4 4 0 014 4v2"/></svg>',
    assistant: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="8" width="18" height="12" rx="2"/><circle cx="9" cy="14" r="1"/><circle cx="15" cy="14" r="1"/><path d="M12 2v4"/></svg>',
    tool: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14.7 6.3a1 1 0 000 1.4l1.6 1.6a1 1 0 001.4 0l3.77-3.77a6 6 0 01-7.94 7.94l-6.91 6.91a2.12 2.12 0 01-3-3l6.91-6.91a6 6 0 017.94-7.94l-3.76 3.76z"/></svg>',
    calendar: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>',
    sigma: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 7V4H6l6 8-6 8h12v-3"/></svg>',
  };

  // --- State → badge color mapping ---
  const STATE_BADGE = {
    permission: 'red', plan: 'red',
    question: 'yellow', error: 'yellow',
    running: 'blue',
    idle_prompt: 'green', done: 'green',
    pr: 'purple',
    merged: 'teal',
  };

  // --- State → border color CSS var ---
  const STATE_BORDER = {
    permission: 'var(--status-red)', plan: 'var(--status-red)',
    question: 'var(--status-yellow)', error: 'var(--status-yellow)',
    running: 'var(--status-blue)',
    idle_prompt: 'var(--status-green)', done: 'var(--status-green)',
    pr: 'var(--status-purple)',
    merged: 'var(--status-teal)',
  };

  // --- UI Component Factory ---
  const UI = {
    header(title, actions) {
      return `<div class="header"><h1>${ICONS.logo} ${escapeHtml(title)}</h1><div class="header-actions">${actions || ''}</div></div>`;
    },

    btn(label, opts) {
      const v = (opts && opts.variant) || 'secondary';
      const onclick = (opts && opts.onclick) ? ` onclick="${opts.onclick}"` : '';
      const id = (opts && opts.id) ? ` id="${opts.id}"` : '';
      return `<button class="btn btn-${v}"${id}${onclick}>${label}</button>`;
    },

    badge(text, state) {
      const color = STATE_BADGE[state] || 'blue';
      return `<span class="badge badge-${color}">${escapeHtml(text)}</span>`;
    },

    card(content, opts) {
      const border = (opts && opts.borderColor) ? `border-left: 3px solid ${opts.borderColor};` : '';
      const onclick = (opts && opts.onclick) ? ` onclick="${opts.onclick}"` : '';
      const cls = (opts && opts.className) ? ' ' + opts.className : '';
      return `<div class="card${cls}" style="${border}"${onclick}>${content}</div>`;
    },

    tabs(items, activeTab) {
      let html = '<div class="tabs">';
      for (const item of items) {
        const cls = item.key === activeTab ? ' active' : '';
        html += `<button class="tab${cls}" data-tab="${item.key}">${escapeHtml(item.label)}</button>`;
      }
      html += '</div>';
      return html;
    },

    stateGroupLabel(group, count) {
      return `<div class="state-group-label"><span class="state-dot state-dot-${group}"></span>${group} (${count})</div>`;
    },

    emptyState(icon, title, subtitle) {
      return `<div class="empty-state">${icon}<div class="empty-state-title">${escapeHtml(title)}</div><div class="empty-state-subtitle">${escapeHtml(subtitle)}</div></div>`;
    },
  };

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

  function durationFromTimestamp(ts) {
    if (!ts) return '';
    const start = new Date(ts);
    const now = new Date();
    const mins = Math.floor((now - start) / 60000);
    if (mins < 60) return mins + 'm';
    return Math.floor(mins / 60) + 'h ' + (mins % 60) + 'm';
  }

  function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function formatTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return escapeHtml(iso);
    const now = new Date();
    const diffMs = now - d;
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return diffMins + 'm ago';
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return diffHours + 'h ago';
    return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', hour12: true }) + ', ' + d.toLocaleDateString([], { month: 'short', day: 'numeric' });
  }

  function formatTimeShort(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return escapeHtml(iso);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }

  function stripMarkdown(s) {
    if (!s) return '';
    return s
      .replace(/```[\s\S]*?```/g, '')
      .replace(/`([^`]+)`/g, '$1')
      .replace(/^#{1,6}\s+/gm, '')
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/__([^_]+)__/g, '$1')
      .replace(/\*([^*]+)\*/g, '$1')
      .replace(/_([^_]+)_/g, '$1')
      .replace(/^[-*+]\s+/gm, '')
      .replace(/\n{2,}/g, ' ')
      .replace(/\n/g, ' ')
      .trim();
  }

  function skeletonLoading(count) {
    let html = '<div style="padding:12px">';
    for (let i = 0; i < count; i++) {
      const w = 40 + Math.random() * 50;
      const align = i % 2 === 0 ? 'margin-left:auto' : '';
      html += `<div class="skeleton skeleton-block" style="width:${w}%;${align}"></div>`;
    }
    html += '</div>';
    return html;
  }

  function renderMarkdown(md) {
    if (typeof marked !== 'undefined') {
      try {
        const raw = marked.parse(md);
        const safe = typeof DOMPurify !== 'undefined' ? DOMPurify.sanitize(raw) : escapeHtml(md);
        return '<div class="markdown-body">' + safe + '</div>';
      } catch (e) { /* fallback */ }
    }
    let html = escapeHtml(md);
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>');
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
    html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
    html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/^- (.+)$/gm, '<li>$1</li>');
    html = html.replace(/\n/g, '<br>');
    return '<div class="markdown-body">' + html + '</div>';
  }

  // Activity kind icons — SVG instead of emoji
  function kindIcon(kind) {
    const svg = ICONS[kind] || '';
    if (svg) return `<span class="activity-icon">${svg}</span>`;
    return '<span class="activity-icon">&#8226;</span>';
  }

  // --- Render: Agent List ---
  function cancelNav() {
    if (navAbort) { navAbort.abort(); navAbort = null; }
  }

  function renderList() {
    cancelNav();
    currentView = 'list';
    const grouped = {};
    for (const agent of agents) {
      const g = stateGroup(effectiveState(agent));
      if (!grouped[g]) grouped[g] = [];
      grouped[g].push(agent);
    }

    const order = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];
    let html = UI.header('Agent Dashboard',
      UI.btn('Usage', { variant: 'ghost', onclick: "Dashboard.showUsage()" })
      + UI.btn('+ New', { variant: 'ghost', onclick: "Dashboard.showCreate()" })
    );
    html += '<div class="agent-list">';

    if (agents.length === 0) {
      html += UI.emptyState(ICONS.robot, 'No agents running', 'Create a new agent to get started');
    }

    for (const group of order) {
      if (!grouped[group] || grouped[group].length === 0) continue;
      html += `<div class="state-group">${UI.stateGroupLabel(group, grouped[group].length)}`;
      for (const agent of grouped[group]) {
        const st = effectiveState(agent);
        const cardContent = `
          <div class="agent-card-header">
            <span class="agent-name">${escapeHtml(repoName(agent))}</span>
            ${UI.badge(st, st)}
          </div>
          <div class="agent-meta">
            ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
            ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
            ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
            ${agent.subagent_count > 0 ? '<span>' + agent.subagent_count + ' subagents</span>' : ''}
            ${agent.current_tool ? '<span class="agent-current-tool">' + escapeHtml(agent.current_tool) + '</span>' : ''}
            <span class="agent-cost" data-agent-id="${agent.session_id}"></span>
          </div>
          ${agent.last_message_preview ? '<div class="agent-preview">' + escapeHtml(stripMarkdown(agent.last_message_preview)) + '</div>' : ''}
        `;
        html += UI.card(cardContent, {
          borderColor: STATE_BORDER[st] || 'var(--border-subtle)',
          onclick: `Dashboard.selectAgent('${agent.session_id}')`,
          className: 'agent-card',
        });
      }
      html += '</div>';
    }

    html += '</div>';
    app.innerHTML = html;
    loadAgentCosts();
  }

  async function loadAgentCosts() {
    const els = document.querySelectorAll('.agent-cost[data-agent-id]');
    if (!els.length) return;
    await Promise.all(Array.from(els).map(async (el) => {
      try {
        const u = await get('/api/agents/' + el.dataset.agentId + '/usage');
        if (u && u.CostUSD > 0) {
          el.textContent = '$' + u.CostUSD.toFixed(2);
        }
      } catch { /* ignore */ }
    }));
  }

  // --- Render: Agent Detail ---
  async function renderDetail(agentId) {
    cancelNav();
    currentView = 'detail';
    selectedAgentId = agentId;
    const agent = agents.find(a => a.session_id === agentId);
    if (!agent) { renderList(); return; }

    const st = effectiveState(agent);
    const detailHeader = `
      <div class="detail-header">
        ${UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })}
        <div class="detail-title">
          <h2>${escapeHtml(repoName(agent))}</h2>
          ${UI.badge(st, st)}
        </div>
        <div class="detail-meta">
          ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
          ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
          ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
        </div>
      </div>
    `;

    const tabs = UI.tabs([
      { key: 'conversation', label: 'Chat' },
      { key: 'activity', label: 'Activity' },
      { key: 'diff', label: 'Diff' },
      { key: 'plan', label: 'Plan' },
      { key: 'files', label: 'Files' },
      { key: 'subagents', label: 'Subagents' },
    ], 'conversation');

    app.innerHTML = `
      ${detailHeader}
      ${tabs}
      <div id="tab-conversation" class="tab-content active">${skeletonLoading(4)}</div>
      <div id="tab-activity" class="tab-content">${skeletonLoading(6)}</div>
      <div id="tab-diff" class="tab-content">${skeletonLoading(3)}</div>
      <div id="tab-plan" class="tab-content">${skeletonLoading(3)}</div>
      <div id="tab-files" class="tab-content"></div>
      <div id="tab-subagents" class="tab-content">${skeletonLoading(2)}</div>
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
    cancelNav();
    navAbort = new AbortController();
    const signal = navAbort.signal;
    const container = document.getElementById('tab-' + tab);
    if (!container) return;

    switch (tab) {
      case 'conversation': {
        const entries = await get('/api/agents/' + agentId + '/conversation');
        if (!entries || entries.length === 0) {
          container.innerHTML = UI.emptyState(ICONS.chat, 'No conversation yet', 'Messages will appear here once the agent starts');
          return;
        }
        let html = '<div class="conversation">';
        let lastRole = '';
        for (const entry of entries) {
          const role = entry.Role || entry.role;
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';
          if (role !== lastRole) {
            const icon = role === 'human' ? ICONS.human : ICONS.assistant;
            const label = role === 'human' ? 'You' : 'Claude';
            html += `<div class="msg-role-label">${icon} ${label}</div>`;
            lastRole = role;
          }
          if (role === 'human') {
            html += `<div class="msg msg-human">${escapeHtml(content)}<div class="msg-time">${formatTime(time)}</div></div>`;
          } else {
            html += `<div class="msg msg-assistant">${renderMarkdown(content)}<div class="msg-time">${formatTime(time)}</div></div>`;
          }
        }
        html += '</div>';
        container.innerHTML = html;
        container.scrollTop = container.scrollHeight;
        break;
      }
      case 'activity': {
        const entries = await get('/api/agents/' + agentId + '/activity');
        if (!entries || entries.length === 0) {
          container.innerHTML = UI.emptyState(ICONS.activity, 'No activity yet', 'Tool calls and messages will appear here');
          return;
        }
        let html = '<div class="activity-filter-bar">';
        for (const f of ['all', 'human', 'assistant', 'tool']) {
          html += `<button class="activity-filter-btn${activityFilter === f ? ' active' : ''}" data-filter="${f}">${f}</button>`;
        }
        html += '</div>';
        // Group entries into turns: human prompt -> tool calls -> assistant response
        const turns = [];
        let currentTurn = null;
        for (const e of entries) {
          const kind = e.Kind || e.kind || 'tool';
          if (kind === 'human') {
            currentTurn = { human: e, tools: [], assistant: null };
            turns.push(currentTurn);
          } else if (currentTurn) {
            if (kind === 'tool') currentTurn.tools.push(e);
            else if (kind === 'assistant') currentTurn.assistant = e;
          } else {
            // Entries before first human message
            if (!turns.length) turns.push({ human: null, tools: [], assistant: null });
            if (kind === 'tool') turns[0].tools.push(e);
            else if (kind === 'assistant') turns[0].assistant = e;
          }
        }

        html += '<div class="activity-log">';
        for (const turn of turns) {
          html += '<div class="activity-turn">';
          // Human entry
          if (turn.human) {
            const content = turn.human.Content || turn.human.content || '';
            const time = turn.human.Timestamp || turn.human.timestamp || '';
            const truncated = content.length > 200;
            const display = truncated ? content.substring(0, 200) + '...' : content;
            html += `<div class="activity-entry" data-kind="human">`
              + kindIcon('human')
              + `<span class="activity-time">${formatTimeShort(time)}</span> `
              + `<span class="activity-human" data-full="${escapeHtml(content)}" data-truncated="true">${escapeHtml(display)}</span>`
              + (truncated ? ` <button class="btn btn-ghost" style="padding:2px 6px;font-size:11px" onclick="Dashboard.toggleExpand(this)">Show more</button>` : '')
              + `</div>`;
          }
          // Tool calls — collapsible group
          if (turn.tools.length > 0) {
            html += `<details class="activity-tool-group"><summary class="activity-tool-summary">${kindIcon('tool')} ${turn.tools.length} tool call${turn.tools.length !== 1 ? 's' : ''}</summary>`;
            for (const e of turn.tools) {
              const content = e.Content || e.content || '';
              const time = e.Timestamp || e.timestamp || '';
              const truncated = content.length > 200;
              const display = truncated ? content.substring(0, 200) + '...' : content;
              html += `<div class="activity-entry activity-entry-tool" data-kind="tool">`
                + `<span class="activity-time">${formatTimeShort(time)}</span> `
                + `<span class="activity-tool" data-full="${escapeHtml(content)}" data-truncated="true">${escapeHtml(display)}</span>`
                + (truncated ? ` <button class="btn btn-ghost" style="padding:2px 6px;font-size:11px" onclick="Dashboard.toggleExpand(this)">Show more</button>` : '')
                + `</div>`;
            }
            html += '</details>';
          }
          // Assistant entry
          if (turn.assistant) {
            const content = turn.assistant.Content || turn.assistant.content || '';
            const time = turn.assistant.Timestamp || turn.assistant.timestamp || '';
            const truncated = content.length > 300;
            const display = truncated ? content.substring(0, 300) + '...' : content;
            html += `<div class="activity-entry" data-kind="assistant">`
              + kindIcon('assistant')
              + `<span class="activity-time">${formatTimeShort(time)}</span> `
              + `<span class="activity-assistant" data-full="${escapeHtml(content)}" data-truncated="true">${escapeHtml(display)}</span>`
              + (truncated ? ` <button class="btn btn-ghost" style="padding:2px 6px;font-size:11px" onclick="Dashboard.toggleExpand(this)">Show more</button>` : '')
              + `</div>`;
          }
          html += '</div>';
        }
        html += '</div>';
        container.innerHTML = html;
        applyActivityFilter(container);
        container.querySelectorAll('.activity-filter-btn').forEach(btn => {
          btn.addEventListener('click', () => {
            activityFilter = btn.dataset.filter;
            container.querySelectorAll('.activity-filter-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            applyActivityFilter(container);
          });
        });
        break;
      }
      case 'diff': {
        const data = await get('/api/agents/' + agentId + '/diff');
        if (signal.aborted) return;
        if (!data || !data.raw) {
          container.innerHTML = UI.emptyState(ICONS.fileDiff, 'No diff available', 'Changes will appear here once the agent modifies files');
          return;
        }

        const files = data.files || [];
        // Split raw diff into per-file chunks
        const rawLines = data.raw.split('\n');
        const fileChunks = [];
        let chunkStart = -1;
        for (let i = 0; i < rawLines.length; i++) {
          if (rawLines[i].startsWith('diff --git')) {
            if (chunkStart >= 0) fileChunks.push(rawLines.slice(chunkStart, i).join('\n'));
            chunkStart = i;
          }
        }
        if (chunkStart >= 0) fileChunks.push(rawLines.slice(chunkStart).join('\n'));

        // Group files by directory for collapsible tree
        const dirGroups = {};
        for (let i = 0; i < files.length; i++) {
          const f = files[i];
          const parts = f.path.split('/');
          const fileName = parts.pop();
          const dir = parts.join('/') || '.';
          if (!dirGroups[dir]) dirGroups[dir] = [];
          dirGroups[dir].push({ ...f, fileName, idx: i });
        }

        let sidebarHtml = '<div class="diff-sidebar"><div class="diff-sidebar-header">Files (' + files.length + ')</div>';
        for (const [dir, dirFiles] of Object.entries(dirGroups)) {
          const dirAdds = dirFiles.reduce((s, f) => s + (f.additions || 0), 0);
          const dirDels = dirFiles.reduce((s, f) => s + (f.deletions || 0), 0);
          sidebarHtml += '<details class="diff-dir-group" open>'
            + '<summary class="diff-dir-summary">'
            + '<span class="diff-dir-name">' + escapeHtml(dir) + '</span>'
            + '<span class="diff-stats"><span class="diff-stats-add">+' + dirAdds + '</span> <span class="diff-stats-del">-' + dirDels + '</span></span>'
            + '</summary>';
          for (const f of dirFiles) {
            const status = f.status || 'modified';
            const adds = f.additions || 0;
            const dels = f.deletions || 0;
            sidebarHtml += '<div class="diff-sidebar-file' + (f.idx === 0 ? ' active' : '') + '" data-file-idx="' + f.idx + '" title="' + escapeHtml(f.path) + '">'
              + '<span class="diff-status-dot diff-status-dot-' + status + '"></span>'
              + '<span class="diff-sidebar-name">' + escapeHtml(f.fileName) + '</span>'
              + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
              + '</div>';
          }
          sidebarHtml += '</details>';
        }
        sidebarHtml += '</div>';

        container.innerHTML = '<div class="diff-layout">' + sidebarHtml + '<div class="diff-content" id="diff-content"></div></div>';

        // Render a single file's diff (lazy — only the selected file)
        function renderFileDiff(idx) {
          if (signal.aborted) return;
          const diffTarget = document.getElementById('diff-content');
          if (!diffTarget) return;
          const chunk = fileChunks[idx];
          if (!chunk) {
            diffTarget.innerHTML = '<div class="empty-state"><div class="empty-state-title">Select a file</div></div>';
            return;
          }
          // Show loading state immediately so UI stays responsive
          diffTarget.innerHTML = '<div class="loading"><span class="spinner"></span></div>';
          // Defer the heavy Diff2HtmlUI rendering to next frame
          setTimeout(() => {
            if (signal.aborted) return;
            if (!document.getElementById('diff-content')) return;
            // Limit very large chunks to avoid freezing
            const lines = chunk.split('\n');
            const maxLines = 2000;
            const truncated = lines.length > maxLines;
            const renderChunk = truncated ? lines.slice(0, maxLines).join('\n') : chunk;
            diffTarget.innerHTML = '';
            try {
              const ui = new Diff2HtmlUI(diffTarget, renderChunk, {
                drawFileList: false,
                matching: 'none',
                outputFormat: 'side-by-side',
                colorScheme: 'dark',
                highlight: false,
              });
              ui.draw();
              if (truncated) {
                diffTarget.insertAdjacentHTML('beforeend',
                  '<div style="padding:12px 16px;color:var(--text-secondary);font-size:13px;border-top:1px solid var(--border-subtle)">'
                  + 'Showing first ' + maxLines + ' lines of ' + lines.length + ' total</div>');
              }
            } catch (e) {
              diffTarget.innerHTML = '<div class="empty-state"><div class="empty-state-title">Diff too large to render</div></div>';
            }
          }, 0);
        }

        container.querySelectorAll('.diff-sidebar-file').forEach(el => {
          el.addEventListener('click', () => {
            container.querySelectorAll('.diff-sidebar-file').forEach(f => f.classList.remove('active'));
            el.classList.add('active');
            renderFileDiff(parseInt(el.dataset.fileIdx, 10));
          });
        });

        // Show sidebar immediately, defer first file render
        setTimeout(() => renderFileDiff(0), 10);
        break;
      }
      case 'plan': {
        const data = await get('/api/agents/' + agentId + '/plan');
        if (!data || !data.content) {
          container.innerHTML = UI.emptyState(ICONS.clipboard, 'No plan available', 'Plans appear when the agent outlines its approach before executing');
          return;
        }
        container.innerHTML = '<div class="plan-content">' + renderMarkdown(data.content) + '</div>';
        break;
      }
      case 'subagents': {
        const subs = await get('/api/agents/' + agentId + '/subagents');
        if (!subs || subs.length === 0) {
          container.innerHTML = UI.emptyState(ICONS.subagent, 'No subagents', 'Subagents appear when the main agent delegates work');
          return;
        }
        let html = '<div class="subagent-list">';
        for (const sub of subs) {
          const completed = sub.Completed || sub.completed;
          const type = sub.AgentType || sub.agent_type || 'agent';
          const desc = sub.Description || sub.description || '';
          const startedAt = sub.StartedAt || sub.started_at || '';
          const statusBadge = completed
            ? UI.badge('completed', 'done')
            : UI.badge('running', 'running');
          const iconSvg = completed
            ? '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--status-green)" stroke-width="2"><polyline points="20,6 9,17 4,12"/></svg>'
            : '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--status-blue)" stroke-width="2"><polygon points="5,3 19,12 5,21"/></svg>';
          const cardContent = `
            <div class="subagent-card-icon">${iconSvg}</div>
            <div class="subagent-card-body">
              <div class="subagent-card-title">${escapeHtml(type)} ${statusBadge}</div>
              ${desc ? '<div class="subagent-card-desc">' + escapeHtml(desc) + '</div>' : ''}
              <div class="subagent-card-meta">
                ${startedAt ? '<span>' + formatTime(startedAt) + '</span>' : ''}
                ${startedAt ? '<span>' + durationFromTimestamp(startedAt) + '</span>' : ''}
              </div>
            </div>
          `;
          html += `<div class="subagent-card">${cardContent}</div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        break;
      }
    }
  }

  function applyActivityFilter(container) {
    // Filter individual entries
    container.querySelectorAll('.activity-entry').forEach(el => {
      if (activityFilter === 'all' || el.dataset.kind === activityFilter) {
        el.classList.remove('hidden');
      } else {
        el.classList.add('hidden');
      }
    });
    // Show/hide tool groups based on filter
    container.querySelectorAll('.activity-tool-group').forEach(group => {
      if (activityFilter === 'all' || activityFilter === 'tool') {
        group.classList.remove('hidden');
      } else {
        group.classList.add('hidden');
      }
    });
    // Hide entire turns if all children are hidden
    container.querySelectorAll('.activity-turn').forEach(turn => {
      const hasVisible = turn.querySelector('.activity-entry:not(.hidden)') || turn.querySelector('.activity-tool-group:not(.hidden)');
      turn.classList.toggle('hidden', !hasVisible);
    });
  }

  function renderFilesTab(agent) {
    const container = document.getElementById('tab-files');
    if (!container) return;
    const files = agent.files_changed || [];
    if (files.length === 0) {
      container.innerHTML = UI.emptyState(ICONS.fileDiff, 'No files changed', 'Modified files will appear here');
      return;
    }

    const parsed = files.map(f => {
      let status = 'modified';
      let name = f;
      if (f.startsWith('+')) { status = 'added'; name = f.substring(1); }
      else if (f.startsWith('-')) { status = 'deleted'; name = f.substring(1); }
      else if (f.startsWith('~')) { name = f.substring(1); }
      const parts = name.split('/');
      const fileName = parts.pop();
      const dir = parts.join('/') || '.';
      return { status, name, fileName, dir, original: f };
    });

    const dirs = {};
    for (const f of parsed) {
      if (!dirs[f.dir]) dirs[f.dir] = [];
      dirs[f.dir].push(f);
    }

    let html = `<div class="files-summary">${files.length} file${files.length !== 1 ? 's' : ''} changed</div>`;
    html += '<div class="files-list">';
    for (const [dir, dirFiles] of Object.entries(dirs)) {
      html += `<details class="files-dir-group" open><summary>${escapeHtml(dir)}/</summary>`;
      for (const f of dirFiles) {
        html += `<div class="file-item" data-file-name="${escapeHtml(f.name)}">`
          + `<span class="file-status-icon file-status-${f.status}"></span>`
          + escapeHtml(f.fileName)
          + `</div>`;
      }
      html += '</details>';
    }
    html += '</div>';
    container.innerHTML = html;
    container.querySelectorAll('.file-item[data-file-name]').forEach(el => {
      el.addEventListener('click', () => {
        Dashboard.goToFileDiff(el.dataset.fileName);
      });
    });
  }

  // --- Action bar ---
  function renderActionBar(agent) {
    const st = effectiveState(agent);
    let actions = '';

    actions += UI.btn('Open Claude', { variant: 'secondary', onclick: "Dashboard.openClaude()" });

    if (st === 'permission' || st === 'plan') {
      actions += UI.btn('Approve', { variant: 'primary', onclick: `Dashboard.approve('${agent.session_id}')` });
      actions += UI.btn('Reject', { variant: 'danger', onclick: `Dashboard.reject('${agent.session_id}')` });
    } else if (st === 'question' || st === 'error') {
      actions += `<input class="action-input" id="reply-input" placeholder="Type a reply..." onkeydown="if(event.key==='Enter')Dashboard.sendInput('${agent.session_id}')">`;
      actions += UI.btn('Send', { variant: 'primary', onclick: `Dashboard.sendInput('${agent.session_id}')` });
    } else if (st === 'pr') {
      actions += UI.btn('Open PR', { variant: 'secondary', onclick: `Dashboard.openPR('${agent.session_id}')` });
      actions += UI.btn('Merge', { variant: 'primary', onclick: `Dashboard.confirmMerge('${agent.session_id}')` });
    } else if (st === 'merged') {
      actions += UI.btn('Close', { variant: 'ghost', onclick: `Dashboard.confirmClose('${agent.session_id}')` });
    }

    if (st === 'running' || st === 'permission' || st === 'plan' || st === 'question') {
      actions += UI.btn('Stop', { variant: 'danger', onclick: `Dashboard.confirmStop('${agent.session_id}')` });
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
          ${UI.btn('Cancel', { variant: 'ghost', id: 'modal-cancel' })}
          ${UI.btn('Confirm', { variant: 'danger', id: 'modal-confirm' })}
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
    app.innerHTML = UI.header('Usage',
      UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })
    ) + '<div class="usage-view"><div class="loading"><span class="spinner"></span></div></div>';

    const data = await get('/api/usage/daily');
    if (!data) return;

    const days = data.days || [];
    const maxCost = Math.max(...days.map(d => d.cost_usd), 0.01);

    const ySteps = 4;
    let yAxisHtml = '<div class="usage-y-axis">';
    for (let i = ySteps; i >= 0; i--) {
      const val = (maxCost / ySteps * i);
      yAxisHtml += `<span class="usage-y-label">$${val < 1 ? val.toFixed(2) : val.toFixed(1)}</span>`;
    }
    yAxisHtml += '</div>';

    let chartHtml = '<div class="usage-chart">';
    for (const day of days) {
      const height = Math.max(2, (day.cost_usd / maxCost) * 100);
      const label = day.date.slice(5);
      const value = '$' + day.cost_usd.toFixed(2);
      chartHtml += `<div class="usage-bar" style="height:${height}%"><span class="usage-bar-value">${value}</span><span class="usage-bar-label">${label}</span></div>`;
    }
    chartHtml += '</div>';

    // Compute "This Week" total from daily data
    const weekTotal = days.reduce((sum, d) => sum + d.cost_usd, 0);

    document.querySelector('.usage-view').innerHTML = `
      <div class="usage-summary">
        <div class="usage-card">
          <div class="usage-card-icon">${ICONS.calendar}</div>
          <div class="usage-card-value">$${(data.today_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">Today</div>
        </div>
        <div class="usage-card">
          <div class="usage-card-icon">${ICONS.chart}</div>
          <div class="usage-card-value">$${weekTotal.toFixed(2)}</div>
          <div class="usage-card-label">This Week</div>
        </div>
        <div class="usage-card">
          <div class="usage-card-icon">${ICONS.sigma}</div>
          <div class="usage-card-value">$${(data.total_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">All Time</div>
        </div>
      </div>
      <h3 class="usage-chart-title">Last 7 Days</h3>
      <div class="usage-chart-container">
        ${yAxisHtml}
        ${chartHtml}
      </div>
      <h3 class="usage-chart-title" style="margin-top:24px">Per-Agent Breakdown</h3>
      <div id="usage-agent-breakdown"><div class="loading"><span class="spinner"></span></div></div>
    `;

    // Fetch per-agent cost breakdown
    loadAgentBreakdown();
  }

  async function loadAgentBreakdown() {
    const container = document.getElementById('usage-agent-breakdown');
    if (!container) return;
    // Fetch usage for each active agent in parallel
    const results = await Promise.all(
      agents.map(async (agent) => {
        try {
          const u = await get('/api/agents/' + agent.session_id + '/usage');
          return { agent, usage: u };
        } catch { return null; }
      })
    );
    const valid = results.filter(r => r && r.usage && r.usage.CostUSD > 0);
    valid.sort((a, b) => b.usage.CostUSD - a.usage.CostUSD);

    if (valid.length === 0) {
      container.innerHTML = '<div style="color:var(--text-tertiary);font-size:13px;padding:8px 0">No per-agent cost data available</div>';
      return;
    }

    let html = '<table class="usage-breakdown-table"><thead><tr><th>Agent</th><th>Model</th><th class="num">Input</th><th class="num">Output</th><th class="num">Cache</th><th class="num">Cost</th></tr></thead><tbody>';
    for (const r of valid) {
      const name = repoName(r.agent);
      const u = r.usage;
      const fmtTokens = (n) => {
        if (!n) return '0';
        if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
        if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
        return String(n);
      };
      html += `<tr>
        <td>${escapeHtml(name)}</td>
        <td><span class="badge badge-blue">${escapeHtml(u.Model || r.agent.model || '?')}</span></td>
        <td class="num">${fmtTokens(u.InputTokens)}</td>
        <td class="num">${fmtTokens(u.OutputTokens)}</td>
        <td class="num">${fmtTokens((u.CacheReadTokens || 0) + (u.CacheWriteTokens || 0))}</td>
        <td class="num">$${u.CostUSD.toFixed(2)}</td>
      </tr>`;
    }
    html += '</tbody></table>';
    container.innerHTML = html;
  }

  // --- Create agent view ---
  function renderCreate() {
    currentView = 'create';
    const folders = [...new Set(agents.map(a => a.cwd).filter(Boolean))];

    app.innerHTML = UI.header('New Agent',
      UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })
    ) + `
      <div class="create-form-card">
        <div class="form-group">
          <label class="form-label">Folder</label>
          <input id="create-folder" class="action-input" style="width:100%" placeholder="/path/to/repo" list="folder-suggestions">
          <datalist id="folder-suggestions">
            ${folders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
          </datalist>
          <div id="folder-hint" class="form-hint"></div>
        </div>
        <div class="form-group">
          <label class="form-label">Skill (optional)</label>
          <select id="create-skill" class="action-input" style="width:100%">
            <option value="">None</option>
            <option value="feature">feature</option>
            <option value="chore">chore</option>
            <option value="bugfix">bugfix</option>
            <option value="refactor">refactor</option>
            <option value="test">test</option>
            <option value="docs">docs</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Message (optional)</label>
          <textarea id="create-message" class="action-input" style="width:100%;min-height:80px;resize:vertical" placeholder="What should the agent do?"></textarea>
        </div>
        <div style="margin-top:8px">${UI.btn('Create Agent', { variant: 'primary', onclick: "Dashboard.createAgent()" })}</div>
      </div>
    `;

    // Inline folder validation
    const folderInput = document.getElementById('create-folder');
    const folderHint = document.getElementById('folder-hint');
    if (folderInput && folderHint) {
      folderInput.addEventListener('input', () => {
        const val = folderInput.value.trim();
        if (!val) {
          folderHint.textContent = '';
          folderHint.className = 'form-hint';
        } else if (!val.startsWith('/')) {
          folderHint.textContent = 'Path should be absolute (start with /)';
          folderHint.className = 'form-hint form-hint-error';
        } else if (folders.length > 0 && folders.includes(val)) {
          folderHint.textContent = 'Known folder';
          folderHint.className = 'form-hint form-hint-ok';
        } else {
          folderHint.textContent = '';
          folderHint.className = 'form-hint';
        }
      });
    }
  }

  // --- Public API ---
  window.Dashboard = {
    showList() { renderList(); },
    showUsage() { renderUsage(); },
    showCreate() { renderCreate(); },
    selectAgent(id) { renderDetail(id); },

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
        renderList();
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

    goToFileDiff(fileName) {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      const diffTab = document.querySelector('.tab[data-tab="diff"]');
      const diffContent = document.getElementById('tab-diff');
      if (diffTab && diffContent) {
        diffTab.classList.add('active');
        diffContent.classList.add('active');
        currentTab = 'diff';
        const sidebarFiles = diffContent.querySelectorAll('.diff-sidebar-file');
        for (const el of sidebarFiles) {
          if (el.getAttribute('title') === fileName || el.getAttribute('title')?.endsWith(fileName)) {
            el.click();
            break;
          }
        }
      }
    },
  };

  // Configure marked.js if available
  if (typeof marked !== 'undefined') {
    marked.setOptions({ breaks: true, gfm: true });
  }

  // --- Init ---
  async function init() {
    const data = await get('/api/agents');
    if (data) agents = data;
    renderList();
    connectSSE();
  }

  init();
})();
