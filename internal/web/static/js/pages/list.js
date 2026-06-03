// Agent list — Codex-iOS register. Thin glue over the primitives.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { Theme } from '../theme.js';
import { effectiveState, stateGroup, prTag } from '../state.js';
import { escapeHtml, repoName, durationShort, formatCost } from '../format.js';
import { get } from '../api.js';

const GROUP_ORDER = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];

function statusDot(state) {
  const group = stateGroup(state);
  const cls = `status-dot status-dot--${group.toLowerCase()}`;
  return `<span class="${cls}"></span>`;
}

function metaLine(agent) {
  const parts = [];
  if (agent.branch) parts.push(escapeHtml(agent.branch));
  if (agent.model) parts.push(escapeHtml(agent.model));
  if (agent.started_at) parts.push(escapeHtml(durationShort(agent)));
  return parts.join(' · ');
}

export function renderList(app, agents) {
  const grouped = {};
  for (const agent of agents) {
    const g = stateGroup(effectiveState(agent));
    (grouped[g] = grouped[g] || []).push(agent);
  }

  let pinned = UI.appBar({
    title: 'Agents',
    trailing: [
      Theme.trailingEntry(),
      { icon: ICONS.kebab, ariaLabel: 'More', onclick: 'Dashboard.openKebab()' },
    ],
  });

  let body = '';
  if (agents.length === 0) {
    body += `<div class="empty-state">${ICONS.robot}
      <div class="empty-state-title">No agents</div>
      <div class="empty-state-subtitle">Tap + New to start one.</div>
    </div>`;
  }

  for (const group of GROUP_ORDER) {
    const list = grouped[group];
    if (!list || !list.length) continue;
    body += UI.sectionLabel(group, { count: list.length });
    for (const agent of list) {
      const id = agent.session_id;
      const trailing = `<span class="ui-row__trailing-cost" data-agent-id="${id}"></span>`;
      body += UI.row({
        leading: statusDot(effectiveState(agent)),
        title: repoName(agent),
        subtitle: metaLine(agent),
        tag: prTag(agent),
        trailing,
        onclick: `Dashboard.selectAgent('${id}')`,
      });
    }
  }

  // PWA shape: app-bar pinned, list body scrolls inside its own region
  // so the page doesn't scroll as a whole. The desktop @media collapses
  // .page-layout to display:block (see style.css).
  app.innerHTML =
    '<div class="page-layout">' +
      '<div class="page-pinned">' + pinned + '</div>' +
      '<div class="page-scroll">' + body + '</div>' +
    '</div>';

  // Dock floats over the list (position:fixed); append to body to escape #app stacking.
  // Sheets are owned by openKebab/dismissSheet — don't touch them here, SSE re-renders this frequently.
  document.querySelectorAll('.ui-dock').forEach(el => el.remove());
  const dock = document.createElement('div');
  dock.innerHTML = UI.dock({
    search: { label: 'Search', onclick: 'Dashboard.searchAgents()' },
    cta: { label: '+ New', icon: ICONS.pencil, onclick: 'Dashboard.showCreate()' },
  });
  document.body.appendChild(dock.firstElementChild);

  loadAgentCosts();
}

async function loadAgentCosts() {
  const els = document.querySelectorAll('.ui-row__trailing-cost[data-agent-id]');
  if (!els.length) return;
  await Promise.all(Array.from(els).map(async (el) => {
    try {
      const u = await get('/api/agents/' + el.dataset.agentId + '/usage');
      if (u && u.CostUSD > 0) el.textContent = formatCost(u.CostUSD);
    } catch { /* ignore */ }
  }));
}
