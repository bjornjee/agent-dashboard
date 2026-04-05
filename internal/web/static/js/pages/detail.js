// Agent detail view with tabs and inline subagents.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState } from '../state.js';
import { escapeHtml, repoName, duration, durationFromTimestamp, formatTime, formatTimeShort, formatCost, formatTokens, renderMarkdown, skeletonLoading } from '../format.js';
import { get, cancelNav, newNavSignal } from '../api.js';
import { showModal, toast } from '../modal.js';

export { showModal, toast };

function kindIcon(kind) {
  const svg = ICONS[kind] || '';
  if (svg) return `<span class="activity-icon">${svg}</span>`;
  return '<span class="activity-icon">&#8226;</span>';
}

function renderActionBar(agent) {
  const st = effectiveState(agent);
  let actions = '';

  actions += UI.btn('Open Claude', { variant: 'primary', onclick: "Dashboard.openClaude()" });

  if (st === 'permission' || st === 'plan') {
    actions += UI.btn('Approve', { variant: 'secondary', onclick: `Dashboard.approve('${agent.session_id}')` });
    actions += UI.btn('Reject', { variant: 'danger', onclick: `Dashboard.reject('${agent.session_id}')` });
  } else if (st === 'question' || st === 'error') {
    actions += `<input class="action-input" id="reply-input" placeholder="Type a reply..." onkeydown="if(event.key==='Enter')Dashboard.sendInput('${agent.session_id}')">`;
    actions += UI.btn('Send', { variant: 'secondary', onclick: `Dashboard.sendInput('${agent.session_id}')` });
  } else if (st === 'pr') {
    actions += UI.btn('Open PR', { variant: 'secondary', onclick: `Dashboard.openPR('${agent.session_id}')` });
    actions += UI.btn('Merge', { variant: 'secondary', onclick: `Dashboard.confirmMerge('${agent.session_id}')` });
  } else if (st === 'merged') {
    actions += UI.btn('Close', { variant: 'ghost', onclick: `Dashboard.confirmClose('${agent.session_id}')` });
  }

  if (st === 'running' || st === 'permission' || st === 'plan' || st === 'question') {
    actions += UI.stopBtn(`Dashboard.confirmStop('${agent.session_id}')`);
  }

  return `<div class="action-bar">${actions}</div>`;
}

let activityFilter = 'all';

function applyActivityFilter(container) {
  container.querySelectorAll('.activity-entry').forEach(el => {
    if (activityFilter === 'all' || el.dataset.kind === activityFilter) {
      el.classList.remove('hidden');
    } else {
      el.classList.add('hidden');
    }
  });
  container.querySelectorAll('.activity-tool-group').forEach(group => {
    if (activityFilter === 'all' || activityFilter === 'tool') {
      group.classList.remove('hidden');
    } else {
      group.classList.add('hidden');
    }
  });
  container.querySelectorAll('.activity-turn').forEach(turn => {
    const hasVisible = turn.querySelector('.activity-entry:not(.hidden)') || turn.querySelector('.activity-tool-group:not(.hidden)');
    turn.classList.toggle('hidden', !hasVisible);
  });
}

export async function renderDetail(app, agents, agentId, setView) {
  cancelNav();
  activityFilter = 'all';
  setView('detail', agentId);
  const agent = agents.find(a => a.session_id === agentId);
  if (!agent) { window.Dashboard.showList(); return; }

  const st = effectiveState(agent);
  const detailHeader = `
    <div class="detail-header">
      <button class="btn btn-ghost" onclick="Dashboard.showList()">&larr; Back</button>
      <div class="detail-title">
        <h2>${escapeHtml(repoName(agent))}</h2>
        ${UI.badge(st, st)}
      </div>
      <div class="detail-meta">
        ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
        ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
        ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
      </div>
      <div class="subagent-summary" id="subagent-summary"></div>
    </div>
  `;

  const vitalSignsPlaceholder = '<div id="vital-signs-container"></div>';

  const tabs = UI.tabs([
    { key: 'conversation', label: 'Chat' },
    { key: 'activity', label: 'Activity' },
    { key: 'diff', label: 'Diff' },
    { key: 'plan', label: 'Plan' },
  ], 'conversation');

  app.innerHTML = `
    ${detailHeader}
    ${vitalSignsPlaceholder}
    ${tabs}
    <div id="tab-conversation" class="tab-content active">${skeletonLoading(4)}</div>
    <div id="tab-activity" class="tab-content">${skeletonLoading(6)}</div>
    <div id="tab-diff" class="tab-content">${skeletonLoading(3)}</div>
    <div id="tab-plan" class="tab-content">${skeletonLoading(3)}</div>
    ${renderActionBar(agent)}
  `;

  // Tab switching
  let currentTab = 'conversation';
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

  // Load initial tab + subagents + vital signs in parallel
  loadTabContent('conversation', agentId);
  loadSubagentSummary(agentId);
  loadVitalSigns(agentId, agent);
}

async function loadVitalSigns(agentId, agent) {
  const container = document.getElementById('vital-signs-container');
  if (!container) return;
  try {
    const usage = await get('/api/agents/' + agentId + '/usage');
    const elapsed = agent.started_at ? duration(agent) : '';
    container.innerHTML = UI.vitalSigns({
      elapsed: elapsed,
      tokens: (usage && usage.InputTokens ? usage.InputTokens + (usage.OutputTokens || 0) : 0),
      cost: usage ? usage.CostUSD : 0,
    });
  } catch {
    container.innerHTML = '';
  }
}

async function loadSubagentSummary(agentId) {
  const container = document.getElementById('subagent-summary');
  if (!container) return;
  const subs = await get('/api/agents/' + agentId + '/subagents');
  if (!subs || subs.length === 0) {
    container.innerHTML = '';
    return;
  }

  const completed = subs.filter(s => s.Completed || s.completed).length;
  const running = subs.length - completed;

  let html = `<div class="subagent-summary-header">${ICONS.subagent} <span>${subs.length} subagent${subs.length !== 1 ? 's' : ''}</span>`;
  if (running > 0) html += ` <span class="badge badge-running">${running} running</span>`;
  if (completed > 0) html += ` <span class="badge badge-completed">${completed} done</span>`;
  html += '</div>';

  html += '<div class="subagent-summary-list">';
  for (const sub of subs) {
    const isDone = sub.Completed || sub.completed;
    const type = sub.AgentType || sub.agent_type || 'agent';
    const desc = sub.Description || sub.description || '';
    const startedAt = sub.StartedAt || sub.started_at || '';
    const dotClass = isDone ? 'status-dot--completed' : 'status-dot--running';
    html += `<div class="subagent-pill">`;
    html += `<span class="status-dot ${dotClass}"></span>`;
    html += `<span class="subagent-type">${escapeHtml(type)}</span>`;
    if (desc) html += `<span class="subagent-desc">${escapeHtml(desc)}</span>`;
    if (startedAt) html += `<span class="subagent-time">${durationFromTimestamp(startedAt)}</span>`;
    html += '</div>';
  }
  html += '</div>';

  container.innerHTML = html;
}

async function loadTabContent(tab, agentId) {
  const signal = newNavSignal();
  const container = document.getElementById('tab-' + tab);
  if (!container) return;

  switch (tab) {
    case 'conversation': {
      const entries = await get('/api/agents/' + agentId + '/conversation');
      if (signal.aborted) return;
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
      if (signal.aborted) return;
      if (!entries || entries.length === 0) {
        container.innerHTML = UI.emptyState(ICONS.activity, 'No activity yet', 'Tool calls and messages will appear here');
        return;
      }
      let html = '<div class="activity-filter-bar">';
      for (const f of ['all', 'human', 'assistant', 'tool']) {
        const cls = f === activityFilter ? ' active' : '';
        html += `<button class="activity-filter-btn${cls}" data-filter="${f}">${f}</button>`;
      }
      html += '</div><div class="activity-log">';

      // Group entries into turns
      const turns = [];
      let currentTurn = [];
      for (const entry of entries) {
        const kind = entry.Kind || entry.kind;
        if (kind === 'human' && currentTurn.length > 0) {
          turns.push(currentTurn);
          currentTurn = [];
        }
        currentTurn.push(entry);
      }
      if (currentTurn.length > 0) turns.push(currentTurn);

      for (const turn of turns) {
        html += '<div class="activity-turn">';
        let toolGroup = [];
        for (const entry of turn) {
          const kind = entry.Kind || entry.kind;
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';
          const toolName = entry.ToolName || entry.tool_name || '';

          if (kind === 'tool') {
            toolGroup.push(entry);
            continue;
          }
          // Flush any pending tool group
          if (toolGroup.length > 0) {
            html += renderToolGroup(toolGroup);
            toolGroup = [];
          }

          const truncated = content.length > 200;
          const display = truncated ? escapeHtml(content.substring(0, 200)) + '...' : escapeHtml(content);
          html += `<div class="activity-entry" data-kind="${kind}">`;
          html += `<div class="activity-entry-header">${kindIcon(kind)} <span class="activity-kind">${kind}</span><span class="activity-time">${formatTimeShort(time)}</span></div>`;
          if (kind === 'assistant') {
            html += `<div class="activity-entry-content">${renderMarkdown(truncated ? content.substring(0, 200) + '...' : content)}</div>`;
          } else {
            html += `<div class="activity-entry-content">${display}</div>`;
          }
          if (truncated) {
            html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
            html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
          }
          html += '</div>';
        }
        // Flush remaining tool group
        if (toolGroup.length > 0) {
          html += renderToolGroup(toolGroup);
        }
        html += '</div>';
      }
      html += '</div>';
      container.innerHTML = html;

      // Wire filter buttons
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

      // Totals for summary bar
      const totalAdds = files.reduce((s, f) => s + (f.additions || 0), 0);
      const totalDels = files.reduce((s, f) => s + (f.deletions || 0), 0);

      // View mode from localStorage
      let viewMode = localStorage.getItem('diff-view-mode') || 'side-by-side';
      // Force unified on narrow screens
      if (window.innerWidth <= 768) viewMode = 'line-by-line';

      // Build sidebar
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

      // Build file section shells
      let sectionsHtml = '';
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        const status = ['added', 'modified', 'deleted'].includes(f.status) ? f.status : 'modified';
        const adds = f.additions || 0;
        const dels = f.deletions || 0;
        sectionsHtml += '<div class="diff-file-section" data-file-idx="' + i + '" id="diff-file-' + i + '">'
          + '<div class="diff-file-header">'
          + '<span class="diff-file-chevron expanded">&#9656;</span>'
          + '<span class="diff-status-dot diff-status-dot-' + status + '"></span>'
          + '<span class="diff-file-path">' + escapeHtml(f.path) + '</span>'
          + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
          + '</div>'
          + '<div class="diff-file-body"><div class="loading"><span class="spinner"></span></div></div>'
          + '</div>';
      }

      // Summary bar
      const summaryHtml = '<div class="diff-summary-bar">'
        + '<span>Showing ' + files.length + ' changed file' + (files.length !== 1 ? 's' : '')
        + ' with <span class="diff-stats-add">+' + totalAdds + '</span> addition' + (totalAdds !== 1 ? 's' : '')
        + ' and <span class="diff-stats-del">-' + totalDels + '</span> deletion' + (totalDels !== 1 ? 's' : '') + '</span>'
        + '<div class="diff-view-toggle">'
        + '<button class="diff-toggle-btn' + (viewMode === 'side-by-side' ? ' active' : '') + '" data-mode="side-by-side">Split</button>'
        + '<button class="diff-toggle-btn' + (viewMode === 'line-by-line' ? ' active' : '') + '" data-mode="line-by-line">Unified</button>'
        + '</div></div>';

      container.innerHTML = '<div class="diff-view">'
        + summaryHtml
        + '<div class="diff-layout">' + sidebarHtml
        + '<div class="diff-content" id="diff-content">' + sectionsHtml + '</div>'
        + '</div></div>';

      // Lazy render with IntersectionObserver
      const rendered = new Set();
      const diffContent = document.getElementById('diff-content');

      function renderSingleFile(bodyEl, idx) {
        const chunk = fileChunks[idx];
        if (!chunk) { bodyEl.innerHTML = ''; return; }
        const lines = chunk.split('\n');
        const maxLines = 2000;
        const truncated = lines.length > maxLines;
        const renderChunk = truncated ? lines.slice(0, maxLines).join('\n') : chunk;
        bodyEl.innerHTML = '';
        try {
          const ui = new Diff2HtmlUI(bodyEl, renderChunk, {
            drawFileList: false,
            matching: 'words',
            outputFormat: viewMode,
            colorScheme: 'dark',
            highlight: true,
          });
          ui.draw();
          requestAnimationFrame(() => {
            if (!signal.aborted) {
              try { ui.highlightCode(); } catch { /* ignore */ }
            }
          });
          if (truncated) {
            bodyEl.insertAdjacentHTML('beforeend',
              '<div style="padding:12px 16px;color:var(--text-secondary);font-size:13px;border-top:1px solid var(--border-default)">'
              + 'Showing first ' + maxLines + ' lines of ' + lines.length + ' total</div>');
          }
        } catch {
          bodyEl.innerHTML = '<div class="empty-state"><div class="empty-state-title">Diff too large to render</div></div>';
        }
      }

      const lazyObserver = new IntersectionObserver((entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const idx = parseInt(entry.target.dataset.fileIdx, 10);
            if (!rendered.has(idx)) {
              rendered.add(idx);
              const body = entry.target.querySelector('.diff-file-body');
              if (body && body.style.display !== 'none') renderSingleFile(body, idx);
              lazyObserver.unobserve(entry.target);
            }
          }
        }
      }, { root: diffContent, rootMargin: '200px' });

      container.querySelectorAll('.diff-file-section').forEach(el => {
        lazyObserver.observe(el);
      });

      // Scroll spy with debounce for sidebar clicks
      let ignoreSpyUntil = 0;
      const spyObserver = new IntersectionObserver((entries) => {
        if (Date.now() < ignoreSpyUntil) return;
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const idx = entry.target.dataset.fileIdx;
            container.querySelectorAll('.diff-sidebar-file').forEach(f => {
              f.classList.toggle('active', f.dataset.fileIdx === idx);
            });
          }
        }
      }, { root: diffContent, threshold: 0.1 });

      container.querySelectorAll('.diff-file-section').forEach(el => {
        spyObserver.observe(el);
      });

      // Clean up observers on navigation
      signal.addEventListener('abort', () => {
        lazyObserver.disconnect();
        spyObserver.disconnect();
      }, { once: true });

      // File header collapse/expand
      container.querySelectorAll('.diff-file-header').forEach(header => {
        header.addEventListener('click', () => {
          const section = header.parentElement;
          const body = section.querySelector('.diff-file-body');
          const chevron = header.querySelector('.diff-file-chevron');
          const isCollapsed = body.style.display === 'none';
          body.style.display = isCollapsed ? '' : 'none';
          chevron.classList.toggle('expanded', isCollapsed);
          // Trigger lazy render if expanding an unrendered file
          if (isCollapsed) {
            const idx = parseInt(section.dataset.fileIdx, 10);
            if (!rendered.has(idx)) {
              rendered.add(idx);
              renderSingleFile(body, idx);
            }
          }
        });
      });

      // Sidebar click → scroll to file
      container.querySelectorAll('.diff-sidebar-file').forEach(el => {
        el.addEventListener('click', () => {
          ignoreSpyUntil = Date.now() + 600;
          container.querySelectorAll('.diff-sidebar-file').forEach(f => f.classList.remove('active'));
          el.classList.add('active');
          const idx = parseInt(el.dataset.fileIdx, 10);
          const section = document.getElementById('diff-file-' + idx);
          if (section) {
            // Expand if collapsed
            const body = section.querySelector('.diff-file-body');
            if (body && body.style.display === 'none') {
              body.style.display = '';
              section.querySelector('.diff-file-chevron').classList.add('expanded');
              if (!rendered.has(idx)) {
                rendered.add(idx);
                renderSingleFile(body, idx);
              }
            }
            // Scroll within the diff-content container, not the whole page
            const dc = document.getElementById('diff-content');
            if (dc) {
              dc.scrollTop = section.offsetTop - dc.offsetTop;
            }
          }
        });
      });

      // Unified/split toggle
      container.querySelectorAll('.diff-toggle-btn').forEach(btn => {
        btn.addEventListener('click', () => {
          const mode = btn.dataset.mode;
          if (mode === viewMode) return;
          viewMode = mode;
          localStorage.setItem('diff-view-mode', mode);
          container.querySelectorAll('.diff-toggle-btn').forEach(b => b.classList.toggle('active', b.dataset.mode === mode));
          // Re-render only expanded files
          rendered.clear();
          container.querySelectorAll('.diff-file-section').forEach(section => {
            const body = section.querySelector('.diff-file-body');
            if (body && body.style.display !== 'none') {
              body.innerHTML = '<div class="loading"><span class="spinner"></span></div>';
              lazyObserver.observe(section);
            }
          });
        });
      });

      break;
    }
    case 'plan': {
      const data = await get('/api/agents/' + agentId + '/plan');
      if (signal.aborted) return;
      if (!data || !data.content) {
        container.innerHTML = UI.emptyState(ICONS.clipboard, 'No plan available', 'Plans appear when the agent outlines its approach before executing');
        return;
      }
      container.innerHTML = '<div class="plan-content">' + renderMarkdown(data.content) + '</div>';
      break;
    }
  }
}

function renderToolGroup(tools) {
  if (tools.length === 0) return '';
  const first = tools[0];
  const time = first.Timestamp || first.timestamp || '';
  let html = '<div class="activity-tool-group">';
  html += `<details><summary class="tool-group-summary">${kindIcon('tool')} <span class="activity-kind">${tools.length} tool call${tools.length !== 1 ? 's' : ''}</span><span class="activity-time">${formatTimeShort(time)}</span></summary>`;
  for (const entry of tools) {
    const content = entry.Content || entry.content || '';
    const toolName = entry.ToolName || entry.tool_name || '';
    const truncated = content.length > 200;
    const display = truncated ? escapeHtml(content.substring(0, 200)) + '...' : escapeHtml(content);
    html += `<div class="activity-entry" data-kind="tool">`;
    html += `<div class="activity-entry-header"><span class="activity-kind tool-name">${escapeHtml(toolName)}</span></div>`;
    html += `<div class="activity-entry-content">${display}</div>`;
    if (truncated) {
      html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
      html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
    }
    html += '</div>';
  }
  html += '</details></div>';
  return html;
}
