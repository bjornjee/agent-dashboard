// Reusable UI component factory.
import { escapeHtml, formatCost, formatTokens } from './format.js';
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

  stopBtn(onclick) {
    return `<button class="btn-stop" onclick="${onclick}"><span class="btn-stop__icon"></span></button>`;
  },

  badge(text, state) {
    const color = STATE_BADGE[state] || 'running';
    return `<span class="badge badge-${color}">${escapeHtml(text)}</span>`;
  },

  card(content, opts) {
    const onclick = (opts && opts.onclick) ? ` onclick="${opts.onclick}"` : '';
    const cls = (opts && opts.className) ? ' ' + opts.className : '';
    return `<div class="card${cls}"${onclick}>${content}</div>`;
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

  vitalSigns(opts) {
    const phase = opts.phase || '';
    const totalPhases = opts.totalPhases || 0;
    const elapsed = opts.elapsed || '';
    const tokens = opts.tokens || 0;
    const cost = opts.cost || 0;
    const currentPhase = opts.currentPhase || 0;

    let phaseValue = '';
    if (totalPhases > 0) {
      phaseValue = `<span>${currentPhase} of ${totalPhases}</span>`;
      if (phase) phaseValue += ` <span class="vital-phase-name">&middot; ${escapeHtml(phase)}</span>`;
    } else if (phase) {
      phaseValue = escapeHtml(phase);
    } else {
      phaseValue = '&mdash;';
    }

    let progressHtml = '';
    if (totalPhases > 0) {
      progressHtml = '<div class="progress-segments">';
      for (let i = 1; i <= totalPhases; i++) {
        if (i < currentPhase) progressHtml += '<div class="progress-segment progress-segment--complete"></div>';
        else if (i === currentPhase) progressHtml += '<div class="progress-segment progress-segment--current"></div>';
        else progressHtml += '<div class="progress-segment progress-segment--pending"></div>';
      }
      progressHtml += '</div>';
    }

    return `<div class="vital-signs" role="status">
      <div class="vital-cell">
        <span class="vital-label">Phase</span>
        <span class="vital-value vital-value--phase">${phaseValue}</span>
      </div>
      <div class="vital-divider"></div>
      <div class="vital-cell">
        <span class="vital-label">Elapsed</span>
        <span class="vital-value">${escapeHtml(elapsed)}</span>
      </div>
      <div class="vital-divider"></div>
      <div class="vital-cell">
        <span class="vital-label">Tokens</span>
        <span class="vital-value">${formatTokens(tokens)}</span>
      </div>
      <div class="vital-divider"></div>
      <div class="vital-cell">
        <span class="vital-label">Cost</span>
        <span class="vital-value vital-value--cost">${formatCost(cost) || '&mdash;'}</span>
      </div>
      ${progressHtml}
    </div>`;
  },
};
