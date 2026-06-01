// Persistent left sidebar — desktop only (≥ 1024px).
// Renders the agent list grouped by status into #app-sidebar. The mobile
// list view is untouched; this module only mounts when the desktop media
// query matches.
import { UI } from './ui.js';
import { effectiveState, stateGroup } from './state.js';
import { escapeHtml, repoName, durationShort } from './format.js';

const GROUP_ORDER = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];

export const DESKTOP_MQ = '(min-width: 1024px)';

export function isDesktop() {
  return typeof window !== 'undefined' && window.matchMedia(DESKTOP_MQ).matches;
}

function statusDot(state) {
  const group = stateGroup(state);
  return `<span class="status-dot status-dot--${group.toLowerCase()}"></span>`;
}

function metaLine(agent) {
  const parts = [];
  if (agent.branch) parts.push(escapeHtml(agent.branch));
  if (agent.model) parts.push(escapeHtml(agent.model));
  if (agent.started_at) parts.push(escapeHtml(durationShort(agent)));
  return parts.join(' · ');
}

// Renders the sidebar into #app-sidebar. Pass the currently selected
// agent id (if any) so the corresponding row gets the selected class.
export function renderSidebar(agents, selectedAgentId) {
  const host = document.getElementById('app-sidebar');
  if (!host) return;

  const grouped = {};
  for (const agent of agents) {
    const g = stateGroup(effectiveState(agent));
    (grouped[g] = grouped[g] || []).push(agent);
  }

  let html = '<div class="app-sidebar__inner">';

  // Top CTA — "+ New agent"
  html += '<div class="app-sidebar__top">';
  html += UI.row({
    title: '+ New agent',
    onclick: 'Dashboard.showCreate()',
    chevron: false,
  });
  html += '</div>';

  // Agent groups
  html += '<div class="app-sidebar__list">';
  for (const group of GROUP_ORDER) {
    const list = grouped[group];
    if (!list || !list.length) continue;
    html += UI.sectionLabel(group, { count: list.length });
    for (const agent of list) {
      const id = agent.session_id;
      const selectedCls = id === selectedAgentId ? ' app-sidebar__row--selected' : '';
      // Wrap UI.row in a marker div so we can target selection without
      // touching the primitive's signature.
      html += `<div class="app-sidebar__row${selectedCls}" data-agent-id="${escapeHtml(id)}">`;
      html += UI.row({
        leading: statusDot(effectiveState(agent)),
        title: repoName(agent),
        subtitle: metaLine(agent),
        onclick: `Dashboard.selectAgent('${id}')`,
        chevron: false,
      });
      html += '</div>';
    }
  }
  html += '</div>';

  // Bottom anchor — Usage + Settings
  html += '<div class="app-sidebar__bottom">';
  html += UI.row({
    title: 'Usage',
    onclick: 'Dashboard.showUsage()',
    chevron: false,
  });
  html += UI.row({
    title: 'Settings',
    onclick: 'Dashboard.openSettings()',
    chevron: false,
  });
  html += '</div>';

  html += '</div>';

  host.innerHTML = html;
  host.hidden = false;
}
