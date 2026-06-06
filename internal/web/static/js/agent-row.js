// Shared agent-row primitives consumed by both pages/list.js (mobile
// list view) and sidebar.js (desktop sidebar). Before this module the
// two renderers each held verbatim copies of statusDot/chip/rowBadges
// — every row-level change had to edit two files in lockstep. Anything
// row-specific (chips, dot a11y, the UI.row opts shape) lives here.
//
// Each view still owns its own group iteration, section labels, and
// surrounding chrome. The shared surface is only what a row is.

import { stateGroup, prTag, planBadge, subagentBadge, questionBadge, stateLabel } from './state.js';
import { escapeHtml, repoName, durationFromUpdate } from './format.js';

// statusDot renders the leading 6px circle. role="img" + aria-label
// expose the color signal for screen readers. Empty state returns the
// classed dot without aria (defensive — no STATE_BADGE entry).
export function statusDot(state) {
  const group = stateGroup(state);
  const cls = `status-dot status-dot--${group.toLowerCase()}`;
  const label = stateLabel(state);
  const aria = label ? ` role="img" aria-label="${escapeHtml(label)}"` : '';
  return `<span class="${cls}"${aria}></span>`;
}

// chip renders a state badge with visible 1-3 char ALL CAPS token and a
// visually-hidden expansion for screen readers. Callers pass the chip
// modifier class (chip--ask, chip--plan, chip--sub).
export function chip(cls, visible, srText) {
  return `<span class="chip ${cls}">`
    + `<span aria-hidden="true">${escapeHtml(visible)}</span>`
    + `<span class="visually-hidden">${escapeHtml(srText)}</span>`
    + `</span>`;
}

// rowBadges concatenates stateful chips for the title line. Order is
// ASK → PLAN → ↳N, most-blocking signal first. The existing PR tag
// keeps its dedicated `tag` slot in UI.row (and prTag suppresses
// itself when ASK fires, so the row never carries both).
export function rowBadges(agent) {
  let html = '';
  if (questionBadge(agent)) html += chip('chip--ask', 'ASK', 'agent is asking a question');
  if (planBadge(agent))     html += chip('chip--plan', 'PLAN', 'agent is in plan mode');
  const sub = subagentBadge(agent);
  if (sub) html += chip('chip--sub', sub, 'live subagents');
  return html;
}

// metaLine returns the muted subtitle text: branch · model · duration.
// durationFromUpdate prefers updated_at over started_at so the duration
// reflects last activity, not session age.
export function metaLine(agent) {
  const parts = [];
  if (agent.branch) parts.push(escapeHtml(agent.branch));
  if (agent.model) parts.push(escapeHtml(agent.model));
  const dur = durationFromUpdate(agent);
  if (dur) parts.push(escapeHtml(dur));
  return parts.join(' · ');
}

// agentRowOpts returns the UI.row opts dict for an agent. Both views
// call UI.row(agentRowOpts(agent, {...})) and add their own wrapper
// chrome (sidebar's app-sidebar__row marker div with selected class,
// list's page-scroll container).
//
// opts:
//   onclick   — required, passes through to UI.row.
//   chevron   — defaults to true (list view); pass false for sidebar.
//
// The leading + title + subtitle + tag + badges shape is the same for
// both views — that's why this exists.
export function agentRowOpts(agent, opts) {
  const o = opts || {};
  return {
    leading: statusDot(agent.state),
    title: repoName(agent),
    subtitle: metaLine(agent),
    tag: prTag(agent),
    badges: rowBadges(agent),
    onclick: o.onclick,
    chevron: o.chevron !== false,
  };
}
