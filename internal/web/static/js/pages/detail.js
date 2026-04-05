// Agent detail view with tabs and inline subagents.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState } from '../state.js';
import { escapeHtml, repoName, duration, durationFromTimestamp, formatTime, formatTimeShort, renderMarkdown, skeletonLoading } from '../format.js';
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
      <div class="subagent-summary" id="subagent-summary"></div>
    </div>
  `;

  const tabs = UI.tabs([
    { key: 'conversation', label: 'Chat' },
    { key: 'activity', label: 'Activity' },
    { key: 'diff', label: 'Diff' },
    { key: 'plan', label: 'Plan' },
  ], 'conversation');

  app.innerHTML = `
    ${detailHeader}
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

  // Load initial tab + subagents in parallel
  loadTabContent('conversation', agentId);
  loadSubagentSummary(agentId);
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
  if (running > 0) html += ` <span class="badge badge-blue">${running} running</span>`;
  if (completed > 0) html += ` <span class="badge badge-green">${completed} done</span>`;
  html += '</div>';

  html += '<div class="subagent-summary-list">';
  for (const sub of subs) {
    const isDone = sub.Completed || sub.completed;
    const type = sub.AgentType || sub.agent_type || 'agent';
    const desc = sub.Description || sub.description || '';
    const startedAt = sub.StartedAt || sub.started_at || '';
    const statusCls = isDone ? 'subagent-done' : 'subagent-running';
    html += `<div class="subagent-summary-item ${statusCls}">`;
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
        const icon = f === 'all' ? '' : (ICONS[f] || '');
        html += `<button class="activity-filter-btn${cls}" data-filter="${f}">${icon} ${f}</button>`;
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

      function renderFileDiff(idx) {
        if (signal.aborted) return;
        const diffTarget = document.getElementById('diff-content');
        if (!diffTarget) return;
        const chunk = fileChunks[idx];
        if (!chunk) {
          diffTarget.innerHTML = '<div class="empty-state"><div class="empty-state-title">Select a file</div></div>';
          return;
        }
        diffTarget.innerHTML = '<div class="loading"><span class="spinner"></span></div>';
        setTimeout(() => {
          if (signal.aborted) return;
          if (!document.getElementById('diff-content')) return;
          const lines = chunk.split('\n');
          const maxLines = 2000;
          const truncated = lines.length > maxLines;
          const renderChunk = truncated ? lines.slice(0, maxLines).join('\n') : chunk;
          diffTarget.innerHTML = '';
          try {
            const ui = new Diff2HtmlUI(diffTarget, renderChunk, {
              drawFileList: false,
              matching: 'words',
              outputFormat: 'side-by-side',
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

      setTimeout(() => renderFileDiff(0), 10);
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
