// Agent detail view with tabs and inline subagents.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState } from '../state.js';
import { escapeHtml, repoName, duration, durationFromTimestamp, formatTime, formatTimeShort, formatCost, formatTokens, renderMarkdown, skeletonLoading } from '../format.js';
import { get, cancelNav, newNavSignal } from '../api.js';
import { showModal, toast } from '../modal.js';
import { Theme } from '../theme.js';

export { showModal, toast, stopConversationPoll };

// Update the action bar in-place when agent state changes via SSE.
export function updateActionBar(agent) {
  const bar = document.querySelector('.action-bar');
  if (!bar) return;
  const tmp = document.createElement('div');
  tmp.innerHTML = renderActionBar(agent);
  const newBar = tmp.firstElementChild;
  if (newBar) bar.replaceWith(newBar);
}

// Track optimistic messages so refreshConversation can preserve them
let pendingUserMessage = null;

// Optimistically append a user message bubble to the chat.
export function appendUserMessage(text) {
  pendingUserMessage = text;
  const container = document.querySelector('#tab-conversation .conversation');
  if (!container) return;
  // Add role label if the last message was not from the user
  const lastLabel = container.querySelector('.msg-role-label:last-of-type');
  const lastLabelText = lastLabel ? lastLabel.textContent.trim() : '';
  if (!lastLabelText.includes('You')) {
    const labelDiv = document.createElement('div');
    labelDiv.className = 'msg-role-label';
    labelDiv.innerHTML = `${ICONS.human} You`;
    container.appendChild(labelDiv);
  }
  const msgDiv = document.createElement('div');
  msgDiv.className = 'msg msg-human msg-optimistic';
  msgDiv.textContent = text;
  const timeDiv = document.createElement('div');
  timeDiv.className = 'msg-time';
  timeDiv.textContent = formatTimeShort(new Date().toISOString());
  msgDiv.appendChild(timeDiv);
  container.appendChild(msgDiv);
  const scrollParent = container.closest('.detail-scroll');
  if (scrollParent) scrollParent.scrollTop = scrollParent.scrollHeight;
}

function timelineIcon(kind) {
  const svg = ICONS[kind] || '';
  const cls = kind === 'human' ? 'timeline-icon--human'
    : kind === 'assistant' ? 'timeline-icon--assistant'
    : 'timeline-icon--tool';
  return `<div class="timeline-icon ${cls}">${svg}</div>`;
}

function kindLabel(kind) {
  if (kind === 'human') return 'You';
  if (kind === 'assistant') return 'Claude';
  return 'Tool';
}

function renderActionBar(agent) {
  const st = effectiveState(agent);
  const id = agent.session_id;
  let actions = '';

  // Reply input for active agent states
  const INPUT_STATES = ['running', 'permission', 'plan', 'question', 'error', 'pr', 'idle_prompt'];
  if (INPUT_STATES.includes(st)) {
    const placeholder = (st === 'question' || st === 'error') ? 'Type a reply...' : 'Send a message...';
    actions += `<input class="action-input" id="reply-input" placeholder="${placeholder}" onkeydown="if(event.key==='Enter')Dashboard.sendInput('${id}')">`;
    actions += UI.btn('Send', { variant: 'secondary', onclick: `Dashboard.sendInput('${id}')` });
  }

  // State-specific buttons
  if (st === 'permission' || st === 'plan') {
    actions += UI.btn('Approve', { variant: 'secondary', onclick: `Dashboard.approve('${id}', event)` });
    actions += UI.btn('Reject', { variant: 'danger', onclick: `Dashboard.reject('${id}', event)` });
  } else if (st === 'pr') {
    actions += UI.btn('Open PR', { variant: 'secondary', onclick: `Dashboard.openPR('${id}')` });
    actions += UI.btn('Merge', { variant: 'secondary', onclick: `Dashboard.confirmMerge('${id}')` });
  } else if (st === 'merged') {
    actions += UI.btn('Close', { variant: 'ghost', onclick: `Dashboard.confirmClose('${id}')` });
  }

  if (st === 'running' || st === 'permission' || st === 'plan' || st === 'question' || st === 'error') {
    actions += UI.stopBtn(`Dashboard.confirmStop('${id}')`);
  }

  return `<div class="action-bar">${actions}</div>`;
}

let activityFilter = 'all';
let currentPRUrl = '';
let currentDetailTab = 'conversation';
let currentDetailAgentId = null;
let lastAgentState = null;
let conversationPollTimer = null;

// Build conversation HTML from an array of message entries.
function renderConversationHtml(entries) {
  let html = '<div class="conversation">';
  let lastRole = '';
  for (const entry of entries) {
    // Skip task-notification messages (internal agent-to-agent noise)
    if (entry.IsNotification) continue;
    const role = entry.Role || entry.role;
    const content = entry.Content || entry.content || '';
    if (!content) continue;
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
  return html;
}

// Re-fetch and re-render the conversation tab if it is currently active.
// Called by the SSE handler to keep the chat view up to date.
export async function refreshConversation(agentId) {
  if (currentDetailTab !== 'conversation' || currentDetailAgentId !== agentId) return;
  const container = document.getElementById('tab-conversation');
  if (!container) return;
  const entries = await get('/api/agents/' + agentId + '/conversation');
  if (!entries || entries.length === 0) return; // don't wipe existing content with empty state
  const scrollParent = container.closest('.detail-scroll');
  const wasAtBottom = scrollParent && (scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight < 60);

  // Check if the API has caught up with our optimistic message
  if (pendingUserMessage) {
    const lastHuman = [...entries].reverse().find(e => (e.Role || e.role) === 'human');
    const lastContent = lastHuman ? (lastHuman.Content || lastHuman.content || '') : '';
    if (lastContent.includes(pendingUserMessage)) {
      pendingUserMessage = null; // API caught up, clear optimistic state
    }
  }

  container.innerHTML = renderConversationHtml(entries);

  // Re-append optimistic message if API hasn't caught up yet
  if (pendingUserMessage) {
    const conv = container.querySelector('.conversation');
    if (conv) {
      const labelDiv = document.createElement('div');
      labelDiv.className = 'msg-role-label';
      labelDiv.innerHTML = `${ICONS.human} You`;
      conv.appendChild(labelDiv);
      const msgDiv = document.createElement('div');
      msgDiv.className = 'msg msg-human msg-optimistic';
      msgDiv.textContent = pendingUserMessage;
      const timeDiv = document.createElement('div');
      timeDiv.className = 'msg-time';
      timeDiv.textContent = formatTimeShort(new Date().toISOString());
      msgDiv.appendChild(timeDiv);
      conv.appendChild(msgDiv);
    }
  }

  if (scrollParent && wasAtBottom) scrollParent.scrollTop = scrollParent.scrollHeight;
}

// Poll conversation every 2s while the chat tab is active.
// This provides near-realtime streaming of agent responses since the JSONL
// is written by Claude Code (not the dashboard), so fsnotify/SSE doesn't
// trigger on new conversation lines.
function startConversationPoll(agentId) {
  stopConversationPoll();
  conversationPollTimer = setInterval(() => {
    if (currentDetailTab === 'conversation' && currentDetailAgentId === agentId) {
      refreshConversation(agentId);
    } else {
      stopConversationPoll();
    }
  }, 2000);
}

function stopConversationPoll() {
  if (conversationPollTimer) {
    clearInterval(conversationPollTimer);
    conversationPollTimer = null;
  }
}

// Refresh whichever tab is currently active. Called on SSE events.
// Conversation uses its own fetch path (no nav signal). Activity and plan
// use loadTabContent which creates a nav signal, so we debounce to avoid
// rapid SSE events causing cancellation churn.
let refreshTimer = null;
export function refreshActiveTab(agentId) {
  if (currentDetailAgentId !== agentId) return;
  if (currentDetailTab === 'conversation') {
    refreshConversation(agentId);
    return;
  }
  if (currentDetailTab === 'diff') return; // expensive, skip
  // Debounce activity/plan refreshes to avoid nav signal churn
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => {
    if (currentDetailAgentId !== agentId) return;
    if (currentDetailTab === 'activity' || currentDetailTab === 'plan') {
      loadTabContent(currentDetailTab, agentId);
    }
  }, 500);
}

// Update the detail header (status badge, duration) from SSE agent data.
export function refreshDetailHeader(agent) {
  if (!agent) return;
  const st = effectiveState(agent);
  const prev = lastAgentState;
  lastAgentState = st;

  // Update status badge
  const badge = document.querySelector('.detail-title .badge');
  if (badge) {
    const tmp = document.createElement('div');
    tmp.innerHTML = UI.badge(st, st);
    const newBadge = tmp.firstElementChild;
    if (newBadge) badge.replaceWith(newBadge);
  }

  // Update duration
  const meta = document.querySelector('.detail-meta');
  if (meta && agent.started_at) {
    const spans = meta.querySelectorAll('span');
    const last = spans[spans.length - 1];
    if (last) last.textContent = duration(agent);
  }

  // Refresh vital signs only on state change
  if (prev !== null && prev !== st) {
    loadVitalSigns(getAgentId(agent), agent);
    loadSubagentSummary(getAgentId(agent));
  }
}

function getAgentId(agent) {
  return agent.session_id;
}

function applyActivityFilter(container) {
  container.querySelectorAll('.timeline-entry').forEach(el => {
    const kind = el.dataset.kind;
    if (!kind) return;
    if (activityFilter === 'all' || kind === activityFilter) {
      el.classList.remove('hidden');
    } else {
      el.classList.add('hidden');
    }
  });
}

export async function renderDetail(app, agents, agentId, setView) {
  cancelNav();
  stopConversationPoll();
  pendingUserMessage = null;
  activityFilter = 'all';
  setView('detail', agentId);
  const agent = agents.find(a => a.session_id === agentId);
  if (!agent) { window.Dashboard.showList(); return; }
  currentPRUrl = agent.pr_url || '';

  const st = effectiveState(agent);
  const detailHeader = `
    <div class="detail-header">
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
  ], 'conversation');

  const isMobile = window.innerWidth <= 480;
  const vitalOpen = !isMobile && sessionStorage.getItem('collapse-vital-signs-container-' + agentId) !== 'true';
  const subagentOpen = !isMobile && sessionStorage.getItem('collapse-subagent-summary-' + agentId) !== 'true';

  app.innerHTML = `
    <div class="detail-layout">
      <div class="detail-pinned">
        ${UI.header('Agent Dashboard', {
          actions: [{ label: '&larr; Back', onclick: 'Dashboard.showList()' }],
        })}
        ${detailHeader}
        ${UI.collapsibleSection('vital-signs-container', 'Stats', vitalOpen)}
        ${UI.collapsibleSection('subagent-summary', 'Subagents', subagentOpen)}
        ${tabs}
      </div>
      <div class="detail-scroll">
        <div id="tab-conversation" class="tab-content active">${skeletonLoading(4)}</div>
        <div id="tab-activity" class="tab-content">${skeletonLoading(6)}</div>
        <div id="tab-diff" class="tab-content">${skeletonLoading(3)}</div>
        <div id="tab-plan" class="tab-content">${skeletonLoading(3)}</div>
      </div>
      ${renderActionBar(agent)}
    </div>
  `;

  // Tab switching
  currentDetailTab = 'conversation';
  currentDetailAgentId = agentId;
  lastAgentState = st;
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      tab.classList.add('active');
      const target = tab.dataset.tab;
      document.getElementById('tab-' + target).classList.add('active');
      currentDetailTab = target;
      loadTabContent(target, agentId);
      // Start/stop conversation polling based on active tab
      if (target === 'conversation') startConversationPoll(agentId);
      else stopConversationPoll();
    });
  });

  // Persist collapsible section state
  document.querySelectorAll('.collapsible-section').forEach(details => {
    details.addEventListener('toggle', () => {
      const summary = details.querySelector('.collapsible-summary');
      if (!summary) return;
      const sectionId = summary.dataset.section;
      try { sessionStorage.setItem('collapse-' + sectionId + '-' + agentId, String(!details.open)); } catch {}
    });
  });

  // Load initial tab + subagents + vital signs in parallel
  loadTabContent('conversation', agentId);
  loadSubagentSummary(agentId);
  loadVitalSigns(agentId, agent);

  // Start conversation polling for near-realtime updates
  startConversationPoll(agentId);
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
  let subs;
  try {
    subs = await get('/api/agents/' + agentId + '/subagents');
  } catch {
    container.innerHTML = '';
    const section = container.closest('.collapsible-section');
    if (section) section.style.display = 'none';
    return;
  }
  const section = container.closest('.collapsible-section');
  if (!subs || subs.length === 0) {
    container.innerHTML = '';
    if (section) section.style.display = 'none';
    return;
  }
  if (section) section.style.display = '';

  const completed = subs.filter(s => s.Completed || s.completed).length;
  const running = subs.length - completed;

  const MAX_VISIBLE = 3;
  const visible = subs.slice(-MAX_VISIBLE);
  const hidden = subs.length - visible.length;

  let html = '';

  html += '<div class="subagent-summary-list">';
  if (hidden > 0) {
    html += `<div class="subagent-pill subagent-pill--muted"><span class="subagent-type">+${hidden} more</span></div>`;
  }
  for (const sub of visible) {
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
      container.innerHTML = renderConversationHtml(entries);
      const scrollParent = container.closest('.detail-scroll');
      if (scrollParent) scrollParent.scrollTop = scrollParent.scrollHeight;
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
        const label = f === 'all' ? 'All' : f === 'human' ? 'Human' : f === 'assistant' ? 'Assistant' : 'Tool';
        html += `<button class="activity-filter-btn${cls}" data-filter="${f}">${label}</button>`;
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
        let toolGroup = [];
        for (const entry of turn) {
          const kind = entry.Kind || entry.kind;
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';

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
          const displayContent = truncated ? content.substring(0, 200) + '...' : content;
          html += `<div class="timeline-entry activity-entry" data-kind="${kind}">`;
          html += timelineIcon(kind);
          html += '<div class="timeline-content">';
          html += `<div class="timeline-header"><span class="timeline-title">${kindLabel(kind)}</span><span class="timeline-timestamp">${formatTimeShort(time)}</span></div>`;
          if (kind === 'assistant') {
            html += `<div class="timeline-detail">${renderMarkdown(displayContent)}</div>`;
          } else {
            html += `<div class="timeline-detail">${escapeHtml(displayContent)}</div>`;
          }
          if (truncated) {
            html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
            html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
          }
          html += '</div></div>';
        }
        // Flush remaining tool group
        if (toolGroup.length > 0) {
          html += renderToolGroup(toolGroup);
        }
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

      // On mobile, show a PR link instead of rendering diffs
      if (window.innerWidth <= 768) {
        const files = data.files || [];
        const totalAdds = files.reduce((s, f) => s + (f.additions || 0), 0);
        const totalDels = files.reduce((s, f) => s + (f.deletions || 0), 0);
        const prUrl = currentPRUrl ? currentPRUrl + '/files' : '';
        const hasPR = !!prUrl;

        let html = '<div class="mobile-diff-summary">';
        html += '<div class="mobile-diff-stats">'
          + '<span class="diff-stats-add">+' + totalAdds + '</span> '
          + '<span class="diff-stats-del">-' + totalDels + '</span> '
          + 'across ' + files.length + ' file' + (files.length !== 1 ? 's' : '')
          + '</div>';

        // File list
        html += '<div class="mobile-diff-files">';
        for (const f of files) {
          const status = f.status || 'modified';
          html += '<div class="mobile-diff-file">'
            + UI.fileStatusIndicator(status)
            + '<span class="mobile-diff-file-path">' + escapeHtml(f.path) + '</span>'
            + '<span class="diff-stats"><span class="diff-stats-add">+' + (f.additions || 0) + '</span> <span class="diff-stats-del">-' + (f.deletions || 0) + '</span></span>'
            + '</div>';
        }
        html += '</div>';

        if (hasPR) {
          html += '<a class="mobile-pr-link mobile-pr-link--active" href="' + escapeHtml(prUrl) + '" target="_blank" rel="noopener">'
            + '&#x2197; View Diff on GitHub</a>';
        } else {
          html += '<div class="mobile-pr-link mobile-pr-link--inactive">'
            + 'No PR available &mdash; create a PR to view diffs on mobile</div>';
        }
        html += '</div>';
        container.innerHTML = html;
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
            + UI.fileStatusIndicator(status)
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
        const status = ['added', 'modified', 'deleted', 'renamed'].includes(f.status) ? f.status : 'modified';
        const adds = f.additions || 0;
        const dels = f.deletions || 0;
        sectionsHtml += '<div class="diff-file-section" data-file-idx="' + i + '" id="diff-file-' + i + '">'
          + '<div class="diff-file-header">'
          + '<span class="diff-file-chevron expanded">&#9656;</span>'
          + UI.fileStatusIndicator(status)
          + '<span class="diff-file-path">' + escapeHtml(f.path) + '</span>'
          + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
          + '</div>'
          + '<div class="diff-file-body">' + UI.loadingBlock() + '</div>'
          + '</div>';
      }

      // Summary bar
      const summaryHtml = '<div class="diff-summary-bar">'
        + '<span>Showing ' + files.length + ' changed file' + (files.length !== 1 ? 's' : '')
        + ' with <span class="diff-stats-add">+' + totalAdds + '</span> addition' + (totalAdds !== 1 ? 's' : '')
        + ' and <span class="diff-stats-del">-' + totalDels + '</span> deletion' + (totalDels !== 1 ? 's' : '') + '</span>'
        + '<div class="diff-controls">'
        + UI.toggleSwitch('Wrap', 'diff-wrap-lines', sessionStorage.getItem('diff-wrap-lines') === 'true')
        + '<div class="diff-view-toggle">'
        + '<button class="diff-toggle-btn' + (viewMode === 'side-by-side' ? ' active' : '') + '" data-mode="side-by-side">Split</button>'
        + '<button class="diff-toggle-btn' + (viewMode === 'line-by-line' ? ' active' : '') + '" data-mode="line-by-line">Unified</button>'
        + '</div></div></div>';

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
            colorScheme: Theme.getEffective(),
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
              body.innerHTML = UI.loadingBlock();
              lazyObserver.observe(section);
            }
          });
        });
      });

      // Wrap toggle
      const wrapInput = container.querySelector('.toggle-switch__input[data-key="diff-wrap-lines"]');
      if (wrapInput && diffContent) {
        if (sessionStorage.getItem('diff-wrap-lines') === 'true') diffContent.classList.add('diff-wrap');
        wrapInput.addEventListener('change', () => {
          diffContent.classList.toggle('diff-wrap', wrapInput.checked);
          sessionStorage.setItem('diff-wrap-lines', wrapInput.checked);
        });
      }

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
  let html = '<div class="timeline-entry activity-tool-group" data-kind="tool">';
  html += timelineIcon('tool');
  html += '<div class="timeline-content">';
  html += `<details><summary class="tool-group-summary"><span class="timeline-title">${tools.length} tool call${tools.length !== 1 ? 's' : ''}</span><span class="timeline-timestamp">${formatTimeShort(time)}</span></summary>`;
  for (const entry of tools) {
    const content = entry.Content || entry.content || '';
    const toolName = entry.ToolName || entry.tool_name || '';
    const truncated = content.length > 200;
    const display = truncated ? escapeHtml(content.substring(0, 200)) + '...' : escapeHtml(content);
    html += `<div class="tool-call">`;
    html += `<div class="tool-call__name">${escapeHtml(toolName)}</div>`;
    html += `<div class="timeline-detail">${display}</div>`;
    if (truncated) {
      html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
      html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
    }
    html += '</div>';
  }
  html += '</details></div></div>';
  return html;
}
