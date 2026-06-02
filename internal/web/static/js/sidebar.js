// Persistent left sidebar — desktop only (≥ 1024px).
// Renders the agent list grouped by status into #app-sidebar. The mobile
// list view is untouched; this module only mounts when the desktop media
// query matches.
import { UI } from './ui.js';
import { ICONS } from './icons.js';
import { Theme } from './theme.js';
import { effectiveState, stateGroup, prTag } from './state.js';
import { escapeHtml, repoName, durationShort } from './format.js';

const GROUP_ORDER = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];

export const DESKTOP_MQ = '(min-width: 900px)';

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
// currentView ('list' | 'detail' | 'create' | 'usage') drives which nav
// row gets the active fill.
export function renderSidebar(agents, selectedAgentId, currentView) {
  const host = document.getElementById('app-sidebar');
  if (!host) return;

  const grouped = {};
  for (const agent of agents) {
    const g = stateGroup(effectiveState(agent));
    (grouped[g] = grouped[g] || []).push(agent);
  }

  const sel = (active) => active ? ' app-sidebar__nav-row--selected' : '';

  let html = '<div class="app-sidebar__inner">';

  // Top CTA — "New agent" + non-interactive Search placeholder slot.
  html += '<div class="app-sidebar__top">';
  html += `<div class="app-sidebar__nav-row app-sidebar__nav-row--cta${sel(currentView === 'create')}">`;
  html += UI.row({
    leading: ICONS.pencil,
    title: 'New agent',
    onclick: 'Dashboard.showCreate()',
    chevron: false,
  });
  html += '</div>';
  // Search slot is a declared placeholder per docs/design/desktop-flow-map.md
  // (slot 2). v1 is non-interactive — the row exists so the layout reads as
  // intended; wiring lands in a follow-up.
  html += '<div class="app-sidebar__nav-row app-sidebar__nav-row--placeholder" aria-disabled="true">';
  html += UI.row({
    leading: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"></circle><path d="M21 21l-4.3-4.3"></path></svg>',
    title: 'Search agents',
    chevron: false,
  });
  html += '</div>';
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
        tag: prTag(agent),
        onclick: `Dashboard.selectAgent('${id}')`,
        chevron: false,
      });
      html += '</div>';
    }
  }
  html += '</div>';

  // Bottom anchor — Install (when offered) + Usage + Settings.
  // The install row is always rendered; CSS hides it unless
  // body.can-install is set by js/install.js after beforeinstallprompt fires.
  html += '<div class="app-sidebar__bottom">';
  html += '<div class="app-sidebar__nav-row app-sidebar__install">';
  html += UI.row({
    leading: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"></path><path d="M7 10l5 5 5-5"></path><path d="M5 21h14"></path></svg>',
    title: 'Install app',
    onclick: 'Dashboard.installApp()',
    chevron: false,
  });
  html += '</div>';
  html += `<div class="app-sidebar__nav-row${sel(currentView === 'usage')}">`;
  html += UI.row({
    title: 'Usage',
    onclick: 'Dashboard.showUsage()',
    chevron: false,
  });
  html += '</div>';
  // Theme toggle replaces Settings — one click instead of opening a modal.
  // data-theme-toggle hooks Theme.cycle() to refresh icon + label in place.
  html += '<div class="app-sidebar__nav-row" data-theme-toggle="1">';
  html += `<button class="ui-row" onclick="Dashboard.cycleTheme()">
    <div class="ui-row__leading"><span data-theme-icon>${Theme.getIcon()}</span></div>
    <div class="ui-row__body">
      <span class="ui-row__title-line">
        <span class="ui-row__title" data-theme-label>${Theme.getNextLabel()}</span>
      </span>
    </div>
  </button>`;
  html += '</div>';
  html += '</div>';

  html += '</div>';

  host.innerHTML = html;
  host.hidden = false;
}
