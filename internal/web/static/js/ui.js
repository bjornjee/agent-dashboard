// Reusable UI component factory.
import { escapeHtml, formatCost, formatCostFull, formatTokens, formatDateShort, durationFromTimestamp } from './format.js';
import { STATE_BADGE } from './state.js';
import { ICONS } from './icons.js';
import { Theme } from './theme.js';
import { isBrowserNotifyEnabled } from './notify.js';

export const UI = {
  header(title, opts) {
    const o = opts || {};
    const actions = o.actions || [];
    const showToggle = o.showThemeToggle !== false;
    let controls = '';
    for (const a of actions) {
      controls += `<button class="header-text-btn" onclick="${a.onclick}">${a.label}</button>`;
    }
    if (actions.length && o.cta) {
      controls += '<div class="header-divider"></div>';
    }
    if (showToggle) {
      const notifyIcon = isBrowserNotifyEnabled() ? ICONS.bell : ICONS.bellOff;
      controls += `<button class="header-icon-btn" onclick="Dashboard.toggleNotifications()" title="Toggle notifications" aria-label="Toggle notifications">${notifyIcon}</button>`;
      controls += `<button class="header-icon-btn" onclick="Dashboard.cycleTheme()" title="Toggle theme" aria-label="Toggle theme">${Theme.getIcon()}</button>`;
    }
    if (o.cta) {
      controls += `<button class="header-cta" onclick="${o.cta.onclick}">${o.cta.label}</button>`;
    }
    return `<div class="header"><button class="header-logo" onclick="Dashboard.showList()" aria-label="Home">${ICONS.logo}</button><span class="header-title">${escapeHtml(title)}</span><div class="header-controls">${controls}</div></div>`;
  },

  spinner() {
    return '<span class="spinner spinner-inline"></span>';
  },

  loadingBlock() {
    return '<div class="loading"><span class="spinner"></span></div>';
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

  metricsStrip(cells) {
    let html = '<div class="usage-metrics">';
    for (let i = 0; i < cells.length; i++) {
      const c = cells[i];
      html += '<div class="usage-metric">';
      html += `<div class="usage-metric__label">${escapeHtml(c.label)}</div>`;
      html += `<div class="usage-metric__value">${escapeHtml(c.value)}</div>`;
      if (c.delta) {
        const cls = c.delta.direction === 'up' ? 'usage-metric__delta--up'
          : c.delta.direction === 'down' ? 'usage-metric__delta--down'
          : 'usage-metric__delta--neutral';
        html += `<div class="usage-metric__delta ${cls}">${escapeHtml(c.delta.text)}</div>`;
      }
      html += '</div>';
    }
    html += '</div>';
    return html;
  },

  chartContainer(title, chartHtml, rightAction) {
    return `<div class="usage-chart-card">
      <div class="usage-chart-card__header">
        <div class="usage-chart__title">${escapeHtml(title)}</div>
        ${rightAction || ''}
      </div>
      ${chartHtml}
    </div>`;
  },

  chartBar(opts) {
    const todayCls = opts.isToday ? ' usage-bar--today' : '';
    const tooltip = `<div class="chart-tooltip">${escapeHtml(opts.value)} &middot; ${escapeHtml(opts.label)}</div>`;
    let todayLabel = '';
    if (opts.isToday) {
      todayLabel = '<span class="usage-bar-today-label">Today</span>';
    }
    return `<div class="usage-bar${todayCls}" style="height:${opts.height}%">
      ${tooltip}
      <span class="usage-bar-label">${escapeHtml(opts.label)}</span>
      ${todayLabel}
    </div>`;
  },

  dateRangeSelector(options, active, onclickFn) {
    let html = '<div class="date-range-selector">';
    for (const opt of options) {
      const cls = opt.value === active ? ' date-range-option--active' : '';
      html += `<button class="date-range-option${cls}" onclick="${onclickFn}(${opt.value})">${escapeHtml(opt.label)}</button>`;
    }
    html += '</div>';
    return html;
  },

  tableCard(title, tableHtml) {
    return `<div class="usage-table-container">
      <div class="usage-table__title">${escapeHtml(title)}</div>
      ${tableHtml}
    </div>`;
  },

  subagentRow(sub) {
    const statusColor = sub.status === 'completed' ? 'completed' : sub.status === 'failed' ? 'failed' : 'running';
    const name = escapeHtml(sub.name || sub.type || 'subagent');
    const task = sub.task ? escapeHtml(sub.task) : '';
    const dur = sub.started_at ? durationFromTimestamp(sub.started_at) : '';
    return `<div class="agent-card__subagent-row">
      <span class="subagent-status-dot subagent-status-dot--${statusColor}"></span>
      <span class="agent-card__subagent-name">${name}</span>
      <span class="agent-card__subagent-task">${task}</span>
      <span class="agent-card__subagent-duration">${escapeHtml(dur)}</span>
    </div>`;
  },

  fileStatusIndicator(status) {
    switch (status) {
      case 'added':
        return '<span class="file-status file-status--added">+</span>';
      case 'deleted':
        return '<span class="file-status file-status--deleted">&minus;</span>';
      case 'renamed':
        return '<span class="file-status file-status--renamed">&rarr;</span>';
      default:
        return '<span class="file-status file-status--modified"></span>';
    }
  },

  toggleSwitch(label, key, defaultOn) {
    const checked = defaultOn ? ' checked' : '';
    return `<label class="toggle-switch">
      <span class="toggle-switch__label">${escapeHtml(label)}</span>
      <input type="checkbox" class="toggle-switch__input" data-key="${escapeHtml(key)}"${checked}>
      <span class="toggle-switch__track"></span>
    </label>`;
  },

  nudge(message, opts) {
    const o = opts || {};
    const type = o.type || 'review';
    const agentId = o.agentId || '';
    const autoDismissMs = o.autoDismissMs || 5000;

    // Cap visible nudges at 3
    const existing = document.querySelectorAll('.nudge-banner');
    if (existing.length >= 3) {
      const oldest = existing[0];
      oldest.classList.remove('nudge-visible');
      setTimeout(() => oldest.remove(), 300);
    }

    const el = document.createElement('div');
    el.className = 'nudge-banner nudge-' + type;
    el.setAttribute('role', 'alert');

    // Stack offset: each existing nudge pushes this one down
    const activeCount = document.querySelectorAll('.nudge-banner').length;
    el.style.top = 'calc(' + (48 + activeCount * 56) + 'px + env(safe-area-inset-top, 0px))';

    el.innerHTML =
      '<div class="nudge-content"' + (agentId ? ' onclick="Dashboard.selectAgent(\'' + escapeHtml(agentId) + '\')"' : '') + '>' +
        '<span class="nudge-message">' + escapeHtml(message) + '</span>' +
      '</div>' +
      '<button class="nudge-dismiss" aria-label="Dismiss">&times;</button>';

    el.querySelector('.nudge-dismiss').addEventListener('click', (e) => {
      e.stopPropagation();
      dismiss();
    });

    document.body.appendChild(el);
    requestAnimationFrame(() => el.classList.add('nudge-visible'));

    function dismiss() {
      el.classList.remove('nudge-visible');
      setTimeout(() => {
        el.remove();
        repositionNudges();
      }, 300);
    }

    const timer = setTimeout(dismiss, autoDismissMs);

    function repositionNudges() {
      const banners = document.querySelectorAll('.nudge-banner');
      banners.forEach((b, i) => {
        b.style.top = 'calc(' + (48 + i * 56) + 'px + env(safe-area-inset-top, 0px))';
      });
    }

    return { dismiss, el };
  },

  collapsibleSection(id, label, open) {
    const openAttr = open ? ' open' : '';
    return `<details class="collapsible-section" id="${id}-section"${openAttr}>
      <summary class="collapsible-summary" data-section="${id}">${escapeHtml(label)}</summary>
      <div class="collapsible-body" id="${id}"></div>
    </details>`;
  },

  vitalSigns(opts) {
    const phase = opts.phase || '';
    const totalPhases = opts.totalPhases || 0;
    const elapsed = opts.elapsed || '';
    const tokens = opts.tokens || 0;
    const cost = opts.cost || 0;
    const currentPhase = opts.currentPhase || 0;

    const hasPhase = totalPhases > 0 || phase;
    let phaseValue = '';
    let progressHtml = '';

    if (totalPhases > 0) {
      phaseValue = `<span>${currentPhase} of ${totalPhases}</span>`;
      if (phase) phaseValue += ` <span class="vital-phase-name">&middot; ${escapeHtml(phase)}</span>`;
      progressHtml = '<div class="progress-segments">';
      for (let i = 1; i <= totalPhases; i++) {
        if (i < currentPhase) progressHtml += '<div class="progress-segment progress-segment--complete"></div>';
        else if (i === currentPhase) progressHtml += '<div class="progress-segment progress-segment--current"></div>';
        else progressHtml += '<div class="progress-segment progress-segment--pending"></div>';
      }
      progressHtml += '</div>';
    } else if (phase) {
      phaseValue = escapeHtml(phase);
    }

    let phaseCellHtml = '';
    if (hasPhase) {
      phaseCellHtml = `<div class="vital-cell">
        <span class="vital-label">Phase</span>
        <span class="vital-value vital-value--phase">${phaseValue}</span>
      </div>
      <div class="vital-divider"></div>`;
    }

    return `<div class="vital-signs" role="status">
      ${phaseCellHtml}
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
