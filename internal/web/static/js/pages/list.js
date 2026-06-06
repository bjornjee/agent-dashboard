// Agent list — Codex-iOS register. Thin glue over the primitives.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { Theme } from '../theme.js';
import { effectiveState, stateGroup, prTag, planBadge, subagentBadge, questionBadge } from '../state.js';
import { escapeHtml, repoName, durationFromUpdate, lastMessagePreview } from '../format.js';

const GROUP_ORDER = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];

function statusDot(state) {
  const group = stateGroup(state);
  const cls = `status-dot status-dot--${group.toLowerCase()}`;
  return `<span class="${cls}"></span>`;
}

// metaLine is the demoted "branch · model · duration" line. Lives under
// the preview when there is one; replaces the subtitle when there isn't,
// so older state files keep their pre-fix appearance.
export function metaLine(agent) {
  const parts = [];
  if (agent.branch) parts.push(escapeHtml(agent.branch));
  if (agent.model) parts.push(escapeHtml(agent.model));
  const dur = durationFromUpdate(agent);
  if (dur) parts.push(escapeHtml(dur));
  return parts.join(' · ');
}

// rowBadges concatenates stateful chips for the title line. PLAN and
// subagent chips both come from state.js helpers; the existing PR tag
// keeps its dedicated `tag` slot in UI.row.
export function rowBadges(agent) {
  let html = '';
  // ASK chip first — questions are the most blocking signal; they should
  // be the first thing the eye lands on, even when the agent also has a
  // PR pin or is in plan mode.
  const ask = questionBadge(agent);
  if (ask) html += `<span class="chip chip--ask">${escapeHtml(ask)}</span>`;
  const plan = planBadge(agent);
  if (plan) html += `<span class="chip chip--plan">${escapeHtml(plan)}</span>`;
  const sub = subagentBadge(agent);
  if (sub) html += `<span class="chip chip--sub">${escapeHtml(sub)}</span>`;
  return html;
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
      const preview = lastMessagePreview(agent);
      const meta = metaLine(agent);
      // Preview wins the subtitle slot; meta drops to the muted line below.
      // Without a preview (older state files) the meta stays in the subtitle
      // so the row keeps its pre-fix appearance and we never render an empty
      // line.
      body += UI.row({
        leading: statusDot(effectiveState(agent)),
        title: repoName(agent),
        subtitle: preview || meta,
        meta: preview ? meta : '',
        tag: prTag(agent),
        badges: rowBadges(agent),
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
}
