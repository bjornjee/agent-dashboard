// Reusable UI component factory.
import { escapeHtml } from './format.js';
import { STATE_BADGE } from './state.js';
import { ICONS } from './icons.js';

export const UI = {
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
