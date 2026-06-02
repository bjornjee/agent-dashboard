// Usage view — codex-iOS register, desktop + mobile.
//
// Phases:
//   1. Render shell (app-bar + loading placeholder).
//   2. Await rate-limit + daily endpoints in parallel.
//   3. Replace placeholder with a single ui-card stack so the previous
//      insertAdjacentHTML race (rate-limit card was getting wiped by the
//      usage-data write) cannot recur.
import { UI } from '../ui.js';
import { escapeHtml, repoName, formatTokens, formatCostFull, formatDateShort } from '../format.js';
import { get } from '../api.js';
import { ICONS } from '../icons.js';

const RANGE_OPTIONS = [
  { label: 'Week', value: 7 },
  { label: '30d', value: 30 },
  { label: '90d', value: 90 },
  { label: 'All', value: 0 },
];

const RANGE_LABELS = { 7: 'This week', 30: 'Last 30 days', 90: 'Last 90 days', 0: 'All time' };

let currentAgents = [];
let currentRange = 7;
let cachedRateLimits = null;
let cachedDaily = null;

export async function renderUsage(app, agents) {
  currentAgents = agents;
  cachedRateLimits = null;
  cachedDaily = null;
  app.innerHTML =
    UI.appBar({
      back: true,
      title: 'Usage',
      trailing: [{ icon: ICONS.kebab, ariaLabel: 'More', onclick: 'Dashboard.openKebab()' }],
    }) +
    '<div class="usage-view"><div class="usage-view__loading">' + UI.loadingBlock() + '</div></div>';

  const [rl, daily] = await Promise.all([fetchRateLimits(), fetchDaily(currentRange)]);
  cachedRateLimits = rl;
  cachedDaily = daily;
  paintAll();
  loadAgentBreakdown(currentAgents);
}

window.Dashboard = window.Dashboard || {};
window.Dashboard.setUsageRange = async function(days) {
  if (days === currentRange) return;
  currentRange = days;
  cachedDaily = await fetchDaily(days);
  paintAll();
  loadAgentBreakdown(currentAgents);
};

// ---------- data layer ----------

async function fetchRateLimits() {
  try {
    const resp = await fetch('/api/usage/ratelimit', { headers: { 'X-Requested-With': 'dashboard' } });
    if (resp.status === 204 || !resp.ok) return null;
    return await resp.json();
  } catch { return null; }
}

async function fetchDaily(days) {
  return await get('/api/usage/daily?days=' + days);
}

// ---------- render layer ----------

function paintAll() {
  const view = document.querySelector('.usage-view');
  if (!view) return;
  const parts = [];
  if (cachedRateLimits) {
    const card = rateLimitCard(cachedRateLimits);
    if (card) parts.push(card);
  }
  if (cachedDaily) {
    parts.push(metricsCard(cachedDaily));
    parts.push(tokenCard(cachedDaily));
    parts.push(chartCard(cachedDaily));
  }
  // Agent breakdown slot — loadAgentBreakdown() writes into it post-paint.
  parts.push('<div id="usage-agent-breakdown" class="usage-agent-breakdown">' + UI.loadingBlock() + '</div>');
  view.innerHTML = parts.join('');
}

function rateLimitCard(rl) {
  const rows = [];
  if (rl.session) rows.push(rateRow('Session (5h)', rl.session));
  if (rl.weekly) rows.push(rateRow('Weekly', rl.weekly));
  if (rl.opus) rows.push(rateRow('Opus (weekly)', rl.opus));
  if (rl.sonnet) rows.push(rateRow('Sonnet (weekly)', rl.sonnet));
  if (rows.length === 0) return null;

  let extra = '';
  if (rl.extra_usage && rl.extra_usage.enabled) {
    extra = `<div class="usage-rate__extra">Extra Usage <span class="usage-rate__extra-value">${formatCostFull(rl.extra_usage.used_credits)} / ${formatCostFull(rl.extra_usage.monthly_limit)}</span></div>`;
  }
  const plan = rl.plan ? `<span class="usage-card__meta">${escapeHtml(rl.plan)}</span>` : '';
  const header = `<div class="usage-card__header"><span class="usage-card__title">Rate Limits</span>${plan}</div>`;
  return UI.card(header + '<div class="usage-rate__rows">' + rows.join('') + '</div>' + extra);
}

function rateRow(label, win) {
  const pct = Math.min(100, Math.max(0, win.used_percent || 0));
  const resetText = win.resets_at ? formatResetDuration(win.resets_at) : '';
  // Status semantics only when threshold crossed. Neutral default per register
  // (no --accent-green fills outside running-status dot).
  let fillCls = 'usage-rate__fill';
  if (pct >= 80) fillCls += ' usage-rate__fill--warn';
  else if (pct >= 60) fillCls += ' usage-rate__fill--caution';
  return `<div class="usage-rate__row">
    <span class="usage-rate__label">${escapeHtml(label)}</span>
    <div class="usage-rate__track"><div class="${fillCls}" style="width:${pct}%"></div></div>
    <span class="usage-rate__pct">${Math.round(pct)}%</span>
    ${resetText ? `<span class="usage-rate__reset">${escapeHtml(resetText)}</span>` : ''}
  </div>`;
}

function formatResetDuration(isoStr) {
  const resetAt = new Date(isoStr);
  const diff = resetAt - Date.now();
  if (diff <= 0) return 'resetting';
  const hours = Math.floor(diff / 3600000);
  const mins = Math.floor((diff % 3600000) / 60000);
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    const remHours = hours % 24;
    return remHours > 0 ? `resets in ${days}d ${remHours}h` : `resets in ${days}d`;
  }
  if (hours > 0) return `resets in ${hours}h ${mins}m`;
  return `resets in ${mins}m`;
}

function metricsCard(data) {
  const days = data.days || [];
  const periodTotal = days.reduce((sum, d) => sum + d.cost_usd, 0);
  const todayCost = data.today_cost || 0;
  const yesterdayStr = new Date(Date.now() - 86400000).toISOString().slice(0, 10);
  const yesterdayEntry = days.find(d => d.date === yesterdayStr);
  const delta = buildDelta(todayCost, yesterdayEntry ? yesterdayEntry.cost_usd : null);
  const periodLabel = currentRange === 0 ? 'All time' : 'This period';
  const cells = [
    metricCell('Today', formatCostFull(todayCost), delta),
    metricCell(periodLabel, formatCostFull(periodTotal), null),
    metricCell('All time', formatCostFull(data.total_cost || 0), null),
  ];
  return UI.card('<div class="usage-metrics">' + cells.join('') + '</div>');
}

function metricCell(label, value, delta) {
  const deltaHtml = delta
    ? `<div class="usage-metric__delta usage-metric__delta--${delta.direction}">${escapeHtml(delta.text)}</div>`
    : '';
  return `<div class="usage-metric">
    <div class="usage-metric__label">${escapeHtml(label)}</div>
    <div class="usage-metric__value">${escapeHtml(value)}</div>
    ${deltaHtml}
  </div>`;
}

function buildDelta(todayCost, yesterdayCost) {
  if (yesterdayCost == null || yesterdayCost === 0) return null;
  const pct = ((todayCost - yesterdayCost) / yesterdayCost) * 100;
  const absPct = Math.abs(pct);
  const fmt = absPct < 10 ? absPct.toFixed(1) + '%' : Math.round(absPct) + '%';
  if (pct > 0) return { direction: 'up', text: '▲ ' + fmt + ' vs yesterday' };
  if (pct < 0) return { direction: 'down', text: '▼ ' + fmt + ' vs yesterday' };
  return { direction: 'neutral', text: '— same as yesterday' };
}

function tokenCard(data) {
  const days = data.days || [];
  const todayStr = new Date().toISOString().slice(0, 10);
  const periodLabel = currentRange === 0 ? 'All time' : 'This period';
  const todayEntry = days.find(d => d.date === todayStr);
  const todayTokens = todayEntry
    ? { input: todayEntry.input_tokens || 0, output: todayEntry.output_tokens || 0, cache: (todayEntry.cache_read_tokens || 0) + (todayEntry.cache_write_tokens || 0) }
    : { input: 0, output: 0, cache: 0 };
  todayTokens.total = todayTokens.input + todayTokens.output + todayTokens.cache;
  const periodTokens = days.reduce((acc, d) => {
    acc.input += d.input_tokens || 0;
    acc.output += d.output_tokens || 0;
    acc.cache += (d.cache_read_tokens || 0) + (d.cache_write_tokens || 0);
    return acc;
  }, { input: 0, output: 0, cache: 0 });
  periodTokens.total = periodTokens.input + periodTokens.output + periodTokens.cache;

  const header = `<div class="usage-card__header"><span class="usage-card__title">Token Usage</span></div>`;
  const table = `<div class="usage-table-scroll"><table class="usage-table">
    <thead><tr>
      <th>Period</th><th class="num">Input</th><th class="num">Output</th>
      <th class="num">Cache</th><th class="num">Total</th>
    </tr></thead>
    <tbody>
      <tr>
        <td data-label="Period">Today</td>
        <td class="num" data-label="Input">${formatTokens(todayTokens.input)}</td>
        <td class="num" data-label="Output">${formatTokens(todayTokens.output)}</td>
        <td class="num" data-label="Cache">${formatTokens(todayTokens.cache)}</td>
        <td class="num" data-label="Total">${formatTokens(todayTokens.total)}</td>
      </tr>
      <tr>
        <td data-label="Period">${escapeHtml(periodLabel)}</td>
        <td class="num" data-label="Input">${formatTokens(periodTokens.input)}</td>
        <td class="num" data-label="Output">${formatTokens(periodTokens.output)}</td>
        <td class="num" data-label="Cache">${formatTokens(periodTokens.cache)}</td>
        <td class="num" data-label="Total">${formatTokens(periodTokens.total)}</td>
      </tr>
    </tbody>
  </table></div>`;
  return UI.card(header + table);
}

function chartCard(data) {
  const days = data.days || [];
  const todayStr = new Date().toISOString().slice(0, 10);
  const title = RANGE_LABELS[currentRange] || 'Usage';
  const tabs = rangeTabs(currentRange);
  const header = `<div class="usage-card__header">
    <span class="usage-card__title">${escapeHtml(title)}</span>
    ${tabs}
  </div>`;
  return UI.card(header + buildChart(days, todayStr));
}

function rangeTabs(active) {
  let html = '<div class="usage-tabs" role="tablist">';
  for (const opt of RANGE_OPTIONS) {
    const cls = opt.value === active ? ' usage-tabs__tab--active' : '';
    html += `<button class="usage-tabs__tab${cls}" role="tab" aria-selected="${opt.value === active}" onclick="Dashboard.setUsageRange(${opt.value})">${escapeHtml(opt.label)}</button>`;
  }
  return html + '</div>';
}

function buildChart(days, todayStr) {
  const maxCost = Math.max(...days.map(d => d.cost_usd), 0.01);
  const useWholeNumbers = maxCost >= 100;
  const ySteps = 4;

  let yAxisHtml = '<div class="usage-y-axis">';
  let gridHtml = '';
  for (let i = ySteps; i >= 0; i--) {
    const val = (maxCost / ySteps) * i;
    const label = useWholeNumbers ? '$' + Math.round(val) : '$' + val.toFixed(2);
    yAxisHtml += `<span class="usage-y-label">${label}</span>`;
    if (i > 0 && i < ySteps) {
      const pct = 100 - (i / ySteps) * 100;
      gridHtml += `<div class="usage-grid-line" style="top:${pct}%"></div>`;
    }
  }
  yAxisHtml += '</div>';

  let barsHtml = '<div class="usage-chart">';
  for (const day of days) {
    const height = Math.max(2, (day.cost_usd / maxCost) * 100);
    const label = formatDateShort(day.date);
    const value = formatCostFull(day.cost_usd);
    const isToday = day.date === todayStr;
    const todayCls = isToday ? ' usage-bar--today' : '';
    const tooltip = `<div class="usage-bar__tooltip">${escapeHtml(value)} &middot; ${escapeHtml(label)}</div>`;
    const todayLabel = isToday ? '<span class="usage-bar__today">Today</span>' : '';
    barsHtml += `<div class="usage-bar${todayCls}" style="height:${height}%">${tooltip}<span class="usage-bar__label">${escapeHtml(label)}</span>${todayLabel}</div>`;
  }
  barsHtml += '</div>';

  return `<div class="usage-chart-container">${yAxisHtml}${gridHtml}${barsHtml}</div>`;
}

async function loadAgentBreakdown(agents) {
  const container = document.getElementById('usage-agent-breakdown');
  if (!container) return;
  const results = await Promise.all(
    agents.map(async (agent) => {
      try {
        const u = await get('/api/agents/' + agent.session_id + '/usage');
        return { agent, usage: u };
      } catch { return null; }
    })
  );
  const valid = results.filter(r => r && r.usage && r.usage.CostUSD > 0);
  valid.sort((a, b) => b.usage.CostUSD - a.usage.CostUSD);

  if (valid.length === 0) {
    container.innerHTML = UI.card(`<div class="usage-card__header"><span class="usage-card__title">Per-agent breakdown</span></div>
      <div class="usage-empty">No per-agent cost data available</div>`);
    return;
  }

  let totals = { input: 0, output: 0, cache: 0, cost: 0 };
  let rows = '';
  for (const r of valid) {
    const name = repoName(r.agent);
    const u = r.usage;
    const input = u.InputTokens || 0;
    const output = u.OutputTokens || 0;
    const cache = (u.CacheReadTokens || 0) + (u.CacheWriteTokens || 0);
    totals.input += input;
    totals.output += output;
    totals.cache += cache;
    totals.cost += u.CostUSD;
    rows += `<tr>
      <td class="usage-table__name" data-label="Agent">${escapeHtml(name)}</td>
      <td class="usage-table__model" data-label="Model">${escapeHtml((u.Model || r.agent.model || '?').toLowerCase())}</td>
      <td class="num" data-label="Input">${formatTokens(input)}</td>
      <td class="num" data-label="Output">${formatTokens(output)}</td>
      <td class="num" data-label="Cache">${formatTokens(cache)}</td>
      <td class="num usage-table__cost" data-label="Cost">${formatCostFull(u.CostUSD)}</td>
    </tr>`;
  }

  let footerHtml = '';
  if (valid.length >= 2) {
    footerHtml = `<tfoot><tr>
      <td class="usage-table__name" data-label="Agent">Total</td>
      <td data-label="Model"></td>
      <td class="num" data-label="Input">${formatTokens(totals.input)}</td>
      <td class="num" data-label="Output">${formatTokens(totals.output)}</td>
      <td class="num" data-label="Cache">${formatTokens(totals.cache)}</td>
      <td class="num usage-table__cost" data-label="Cost">${formatCostFull(totals.cost)}</td>
    </tr></tfoot>`;
  }

  const header = `<div class="usage-card__header"><span class="usage-card__title">Per-agent breakdown</span></div>`;
  const table = `<div class="usage-table-scroll"><table class="usage-table">
    <thead><tr>
      <th>Agent</th><th>Model</th>
      <th class="num">Input</th><th class="num">Output</th>
      <th class="num">Cache</th><th class="num">Cost</th>
    </tr></thead>
    <tbody>${rows}</tbody>
    ${footerHtml}
  </table></div>`;
  container.innerHTML = UI.card(header + table);
}
