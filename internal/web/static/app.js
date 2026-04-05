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

  // --- SVG Icons ---
  const ICONS = {
    logo: '<svg width="24" height="24" viewBox="0 0 512 512"><rect width="512" height="512" rx="96" fill="var(--blue)"/><text x="256" y="360" font-family="-apple-system,system-ui,sans-serif" font-size="320" font-weight="700" fill="#232634" text-anchor="middle">A</text></svg>',
    robot: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="8" width="18" height="12" rx="2"/><circle cx="9" cy="14" r="1.5"/><circle cx="15" cy="14" r="1.5"/><path d="M12 2v4M8 8V6a4 4 0 018 0v2"/></svg>',
    chat: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/></svg>',
    clipboard: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2"/><rect x="8" y="2" width="8" height="4" rx="1"/></svg>',
    fileDiff: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14,2 14,8 20,8"/><line x1="9" y1="15" x2="15" y2="15"/><line x1="12" y1="12" x2="12" y2="18"/></svg>',
    activity: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="22,12 18,12 15,21 9,3 6,12 2,12"/></svg>',
    chart: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>',
    subagent: '<svg class="empty-state-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><line x1="12" y1="8" x2="5" y2="16"/><line x1="12" y1="8" x2="19" y2="16"/></svg>',
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

  // Format ISO timestamp to relative or short absolute
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

  // Format ISO timestamp to HH:MM:SS
  function formatTimeShort(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return escapeHtml(iso);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }

  // Strip markdown markers for plain-text previews
  function stripMarkdown(s) {
    if (!s) return '';
    return s
      .replace(/```[\s\S]*?```/g, '')   // fenced code blocks
      .replace(/`([^`]+)`/g, '$1')       // inline code
      .replace(/^#{1,6}\s+/gm, '')       // headings
      .replace(/\*\*([^*]+)\*\*/g, '$1') // bold
      .replace(/__([^_]+)__/g, '$1')     // bold alt
      .replace(/\*([^*]+)\*/g, '$1')     // italic
      .replace(/_([^_]+)_/g, '$1')       // italic alt
      .replace(/^[-*+]\s+/gm, '')        // list markers
      .replace(/\n{2,}/g, ' ')           // collapse blank lines
      .replace(/\n/g, ' ')              // single newlines to space
      .trim();
  }

  // Consistent empty state with icon
  function emptyState(icon, title, subtitle) {
    return `<div class="empty-state">${icon}<div class="empty-state-title">${escapeHtml(title)}</div><div class="empty-state-subtitle">${escapeHtml(subtitle)}</div></div>`;
  }

  // Skeleton loading blocks
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

  // Markdown rendering via marked.js with DOMPurify sanitization
  function renderMarkdown(md) {
    if (typeof marked !== 'undefined') {
      try {
        const raw = marked.parse(md);
        const safe = typeof DOMPurify !== 'undefined' ? DOMPurify.sanitize(raw) : escapeHtml(md);
        return '<div class="markdown-body">' + safe + '</div>';
      } catch (e) { /* fallback */ }
    }
    // Fallback: basic regex rendering
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

  // Activity kind icons
  function kindIcon(kind) {
    switch (kind) {
      case 'human': return '<span class="activity-icon">&#128100;</span>';
      case 'assistant': return '<span class="activity-icon">&#129302;</span>';
      case 'tool': return '<span class="activity-icon">&#128295;</span>';
      default: return '<span class="activity-icon">&#8226;</span>';
    }
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
        <h1>${ICONS.logo} Agent Dashboard</h1>
        <div class="header-actions">
          <button class="btn-ghost" onclick="Dashboard.showUsage()">Usage</button>
          <button class="btn-ghost" onclick="Dashboard.showCreate()">+ New</button>
        </div>
      </div>
      <div class="agent-list">
    `;

    if (agents.length === 0) {
      html += emptyState(ICONS.robot, 'No agents running', 'Create a new agent to get started');
    }

    for (const group of order) {
      if (!grouped[group] || grouped[group].length === 0) continue;
      html += `<div class="state-group"><div class="state-group-label"><span class="state-dot state-dot-${group}"></span>${group} (${grouped[group].length})</div>`;
      for (const agent of grouped[group]) {
        const st = effectiveState(agent);
        html += `
          <div class="agent-card agent-card-border-${st}" onclick="Dashboard.selectAgent('${agent.session_id}')">
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
            ${agent.last_message_preview ? '<div class="agent-preview">' + escapeHtml(stripMarkdown(agent.last_message_preview)) + '</div>' : ''}
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
    const container = document.getElementById('tab-' + tab);
    if (!container) return;

    switch (tab) {
      case 'conversation': {
        const entries = await get('/api/agents/' + agentId + '/conversation');
        if (!entries || entries.length === 0) {
          container.innerHTML = emptyState(ICONS.chat, 'No conversation yet', 'Messages will appear here once the agent starts');
          return;
        }
        let html = '<div class="conversation">';
        let lastRole = '';
        for (const entry of entries) {
          const role = entry.Role || entry.role;
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';
          // Role label when role changes
          if (role !== lastRole) {
            html += `<div class="msg-role-label">${role === 'human' ? 'You' : 'Claude'}</div>`;
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
          container.innerHTML = emptyState(ICONS.activity, 'No activity yet', 'Tool calls and messages will appear here');
          return;
        }
        // Filter bar
        let html = '<div class="activity-filter-bar">';
        for (const f of ['all', 'human', 'assistant', 'tool']) {
          html += `<button class="activity-filter-btn${activityFilter === f ? ' active' : ''}" data-filter="${f}">${f}</button>`;
        }
        html += '</div>';
        html += '<div class="activity-log">';
        let lastKind = '';
        for (const e of entries) {
          const kind = e.Kind || e.kind || 'tool';
          const content = e.Content || e.content || '';
          const time = e.Timestamp || e.timestamp || '';
          const cls = 'activity-' + kind;
          // Separator between turns (when going back to human)
          if (kind === 'human' && lastKind && lastKind !== 'human') {
            html += '<hr class="activity-separator">';
          }
          lastKind = kind;
          // Truncation for long entries
          const truncated = content.length > 200;
          const displayContent = truncated ? content.substring(0, 200) + '...' : content;
          html += `<div class="activity-entry" data-kind="${kind}">`
            + kindIcon(kind)
            + `<span class="activity-time">${formatTimeShort(time)}</span> `
            + `<span class="${cls}" data-full="${escapeHtml(content)}" data-truncated="true">${escapeHtml(displayContent)}</span>`
            + (truncated ? ` <button class="activity-expand-btn" onclick="Dashboard.toggleExpand(this)">Show more</button>` : '')
            + `</div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        // Apply current filter
        applyActivityFilter(container);
        // Filter button handlers
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
        if (!data || !data.raw) {
          container.innerHTML = emptyState(ICONS.fileDiff, 'No diff available', 'Changes will appear here once the agent modifies files');
          return;
        }

        // Build file sidebar from parsed files (stats come from server)
        const files = data.files || [];
        let sidebarHtml = '<div class="diff-sidebar"><div class="diff-sidebar-header">Files (' + files.length + ')</div>';
        for (let i = 0; i < files.length; i++) {
          const f = files[i];
          const name = f.path.split('/').pop();
          const dir = f.path.substring(0, f.path.length - name.length);
          const status = f.status || 'modified';
          const adds = f.additions || 0;
          const dels = f.deletions || 0;
          sidebarHtml += '<div class="diff-sidebar-file' + (i === 0 ? ' active' : '') + '" data-file-idx="' + i + '" title="' + escapeHtml(f.path) + '">'
            + '<span class="diff-status-dot diff-status-dot-' + status + '"></span>'
            + '<span class="diff-sidebar-name" style="flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + escapeHtml(dir) + '<strong>' + escapeHtml(name) + '</strong></span>'
            + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
            + '</div>';
        }
        sidebarHtml += '</div>';

        container.innerHTML = '<div class="diff-layout">' + sidebarHtml + '<div class="diff-content" id="diff-content"></div></div>';

        // Render diff with Diff2HtmlUI for syntax highlighting
        const diffTarget = document.getElementById('diff-content');
        const diff2htmlUi = new Diff2HtmlUI(diffTarget, data.raw, {
          drawFileList: false,
          matching: 'words',
          outputFormat: 'line-by-line',
          colorScheme: 'dark',
          highlight: true,
        });
        diff2htmlUi.draw();
        diff2htmlUi.highlightCode();

        // Click file in sidebar to scroll to it
        container.querySelectorAll('.diff-sidebar-file').forEach(el => {
          el.addEventListener('click', () => {
            container.querySelectorAll('.diff-sidebar-file').forEach(f => f.classList.remove('active'));
            el.classList.add('active');
            const idx = parseInt(el.dataset.fileIdx, 10);
            const fileHeaders = diffTarget.querySelectorAll('.d2h-file-wrapper');
            if (fileHeaders[idx]) {
              fileHeaders[idx].scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
          });
        });

        // Auto-scroll to first file
        const firstWrapper = diffTarget.querySelector('.d2h-file-wrapper');
        if (firstWrapper) firstWrapper.scrollIntoView({ block: 'start' });
        break;
      }
      case 'plan': {
        const data = await get('/api/agents/' + agentId + '/plan');
        if (!data || !data.content) {
          container.innerHTML = emptyState(ICONS.clipboard, 'No plan available', 'Plans appear when the agent outlines its approach before executing');
          return;
        }
        container.innerHTML = '<div class="plan-content">' + renderMarkdown(data.content) + '</div>';
        break;
      }
      case 'subagents': {
        const subs = await get('/api/agents/' + agentId + '/subagents');
        if (!subs || subs.length === 0) {
          container.innerHTML = emptyState(ICONS.subagent, 'No subagents', 'Subagents appear when the main agent delegates work');
          return;
        }
        let html = '<div class="subagent-list">';
        for (const sub of subs) {
          const completed = sub.Completed || sub.completed;
          const type = sub.AgentType || sub.agent_type || 'agent';
          const desc = sub.Description || sub.description || '';
          const startedAt = sub.StartedAt || sub.started_at || '';
          const statusBadge = completed
            ? '<span class="badge badge-done">completed</span>'
            : '<span class="badge badge-running">running</span>';
          const icon = completed ? '&#10003;' : '&#9654;';
          const iconColor = completed ? 'var(--green)' : 'var(--blue)';
          html += `<div class="subagent-card">
            <div class="subagent-card-icon" style="color:${iconColor}">${icon}</div>
            <div class="subagent-card-body">
              <div class="subagent-card-title">${escapeHtml(type)} ${statusBadge}</div>
              ${desc ? '<div class="subagent-card-desc">' + escapeHtml(desc) + '</div>' : ''}
              <div class="subagent-card-meta">
                ${startedAt ? '<span>' + formatTime(startedAt) + '</span>' : ''}
                ${startedAt ? '<span>' + durationFromTimestamp(startedAt) + '</span>' : ''}
              </div>
            </div>
          </div>`;
        }
        html += '</div>';
        container.innerHTML = html;
        break;
      }
    }
  }

  // Apply activity filter
  function applyActivityFilter(container) {
    container.querySelectorAll('.activity-entry').forEach(el => {
      if (activityFilter === 'all' || el.dataset.kind === activityFilter) {
        el.classList.remove('hidden');
      } else {
        el.classList.add('hidden');
      }
    });
  }

  function renderFilesTab(agent) {
    const container = document.getElementById('tab-files');
    if (!container) return;
    const files = agent.files_changed || [];
    if (files.length === 0) {
      container.innerHTML = emptyState(ICONS.fileDiff, 'No files changed', 'Modified files will appear here');
      return;
    }

    // Parse files and group by directory
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

    // Group by directory
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
    // Attach click handlers via addEventListener (avoids onclick injection)
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
        <h1>${ICONS.logo} Usage</h1>
        <div class="header-actions">
          <button class="btn-ghost" onclick="Dashboard.showList()">&larr; Back</button>
        </div>
      </div>
      <div class="usage-view"><div class="loading"><span class="spinner"></span></div></div>
    `;

    const data = await get('/api/usage/daily');
    if (!data) return;

    const days = data.days || [];
    const maxCost = Math.max(...days.map(d => d.cost_usd), 0.01);

    // Y-axis labels
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
      const label = day.date.slice(5); // MM-DD
      const value = '$' + day.cost_usd.toFixed(2);
      chartHtml += `<div class="usage-bar" style="height:${height}%"><span class="usage-bar-value">${value}</span><span class="usage-bar-label">${label}</span></div>`;
    }
    chartHtml += '</div>';

    document.querySelector('.usage-view').innerHTML = `
      <div class="usage-summary">
        <div class="usage-card">
          <div class="usage-card-icon">&#128197;</div>
          <div class="usage-card-value">$${(data.today_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">Today</div>
        </div>
        <div class="usage-card">
          <div class="usage-card-icon">&#8721;</div>
          <div class="usage-card-value">$${(data.total_cost || 0).toFixed(2)}</div>
          <div class="usage-card-label">All Time</div>
        </div>
      </div>
      <h3 class="usage-chart-title">Last 7 Days</h3>
      <div class="usage-chart-container">
        ${yAxisHtml}
        ${chartHtml}
      </div>
    `;
  }

  // --- Create agent view ---
  function renderCreate() {
    currentView = 'create';
    // Gather recent folders from existing agents
    const folders = [...new Set(agents.map(a => a.cwd).filter(Boolean))];

    app.innerHTML = `
      <div class="header">
        <h1>${ICONS.logo} New Agent</h1>
        <div class="header-actions">
          <button class="btn-ghost" onclick="Dashboard.showList()">&larr; Back</button>
        </div>
      </div>
      <div class="create-form-card">
        <div class="form-group">
          <label class="form-label">Folder</label>
          <input id="create-folder" class="action-input" style="width:100%" placeholder="/path/to/repo" list="folder-suggestions">
          <datalist id="folder-suggestions">
            ${folders.map(f => `<option value="${escapeHtml(f)}">`).join('')}
          </datalist>
        </div>
        <div class="form-group">
          <label class="form-label">Skill (optional)</label>
          <input id="create-skill" class="action-input" style="width:100%" placeholder="e.g. feature, chore">
        </div>
        <div class="form-group">
          <label class="form-label">Message (optional)</label>
          <textarea id="create-message" class="action-input" style="width:100%;min-height:80px;resize:vertical" placeholder="What should the agent do?"></textarea>
        </div>
        <button class="btn-primary" style="width:100%" onclick="Dashboard.createAgent()">Create Agent</button>
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

    // Toggle expand/collapse for truncated activity entries
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

    // Navigate from Files tab to Diff tab and scroll to file
    goToFileDiff(fileName) {
      // Switch to diff tab
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      const diffTab = document.querySelector('.tab[data-tab="diff"]');
      const diffContent = document.getElementById('tab-diff');
      if (diffTab && diffContent) {
        diffTab.classList.add('active');
        diffContent.classList.add('active');
        currentTab = 'diff';
        // Find matching file in sidebar and click it
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
    marked.setOptions({
      breaks: true,
      gfm: true,
    });
  }

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
