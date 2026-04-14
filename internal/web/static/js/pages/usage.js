// Usage view.
import { UI } from '../ui.js';
import { escapeHtml, repoName, formatTokens, formatCostFull, formatDateShort } from '../format.js';
import { get } from '../api.js';

const RANGE_OPTIONS = [
  { label: '7d', value: 7 },
  { label: '30d', value: 30 },
  { label: '90d', value: 90 },
  { label: 'All', value: 0 },
];

const RANGE_LABELS = { 7: 'Last 7 days', 30: 'Last 30 days', 90: 'Last 90 days', 0: 'All time' };

let currentAgents = [];
let currentRange = 7;

export async function renderUsage(app, agents) {
  currentAgents = agents;
  app.innerHTML = UI.header('Agent Dashboard', {
    actions: [{ label: '&larr; Back', onclick: 'Dashboard.showList()' }],
  }) + '<div class="usage-view">' + UI.loadingBlock() + '</div>';

  // Load rate limits and usage data in parallel
  await Promise.all([loadRateLimits(), loadUsageData()]);
}

// Exposed globally for onclick from dateRangeSelector
window.Dashboard = window.Dashboard || {};
window.Dashboard.setUsageRange = async function(days) {
  if (days === currentRange) return;
  currentRange = days;
  const chartSection = document.getElementById('usage-chart-section');
  if (chartSection) chartSection.innerHTML = UI.loadingBlock();
  await loadUsageData();
};

async function loadRateLimits() {
  try {
    const resp = await fetch('/api/usage/ratelimit', {
      headers: { 'X-Requested-With': 'dashboard' },
    });
    if (resp.status === 204 || !resp.ok) return;
    const rl = await resp.json();
    const view = document.querySelector('.usage-view');
    if (!view) return;

    const windows = [];
    if (rl.session) windows.push(renderRateBar('Session (5h)', rl.session));
    if (rl.weekly) windows.push(renderRateBar('Weekly', rl.weekly));
    if (rl.opus) windows.push(renderRateBar('Opus (weekly)', rl.opus));
    if (rl.sonnet) windows.push(renderRateBar('Sonnet (weekly)', rl.sonnet));

    if (windows.length === 0) return;

    let extraHtml = '';
    if (rl.extra_usage && rl.extra_usage.enabled) {
      extraHtml = `<div class="rate-limit-extra">Extra Usage: ${formatCostFull(rl.extra_usage.used_credits)} / ${formatCostFull(rl.extra_usage.monthly_limit)}</div>`;
    }

    const planLabel = rl.plan ? ` <span class="rate-limit-plan">${escapeHtml(rl.plan)}</span>` : '';
    const card = `<div class="card rate-limit-card">
      <div class="card-header">Rate Limits${planLabel}</div>
      <div class="card-body">${windows.join('')}${extraHtml}</div>
    </div>`;

    // Insert before existing content
    view.insertAdjacentHTML('afterbegin', card);
  } catch { /* silently skip if unavailable */ }
}

function renderRateBar(label, window) {
  const pct = Math.min(100, Math.max(0, window.used_percent || 0));
  const resetText = window.resets_at ? formatResetDuration(window.resets_at) : '';
  const barColor = pct >= 80 ? 'var(--accent-red)' : pct >= 60 ? 'var(--accent-amber)' : 'var(--accent-green)';
  return `<div class="rate-limit-row">
    <span class="rate-limit-label">${escapeHtml(label)}</span>
    <div class="rate-limit-bar-track">
      <div class="rate-limit-bar-fill" style="width:${pct}%;background:${barColor}"></div>
    </div>
    <span class="rate-limit-pct">${Math.round(pct)}%</span>
    ${resetText ? `<span class="rate-limit-reset">${escapeHtml(resetText)}</span>` : ''}
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

async function loadUsageData() {
  const data = await get('/api/usage/daily?days=' + currentRange);
  if (!data) return;

  const days = data.days || [];
  const todayStr = new Date().toISOString().slice(0, 10);
  const periodTotal = days.reduce((sum, d) => sum + d.cost_usd, 0);

  // Delta: compare today vs yesterday by exact date
  const todayCost = data.today_cost || 0;
  const yesterdayStr = new Date(Date.now() - 86400000).toISOString().slice(0, 10);
  const yesterdayEntry = days.find(d => d.date === yesterdayStr);
  const delta = buildDelta(todayCost, yesterdayEntry ? yesterdayEntry.cost_usd : null);

  const periodLabel = currentRange === 0 ? 'All time' : 'This period';
  const metricsHtml = UI.metricsStrip([
    { label: 'Today', value: formatCostFull(todayCost), delta },
    { label: periodLabel, value: formatCostFull(periodTotal) },
    { label: 'All time', value: formatCostFull(data.total_cost || 0) },
  ]);

  const rangeSelector = UI.dateRangeSelector(RANGE_OPTIONS, currentRange, 'Dashboard.setUsageRange');
  const chartHtml = buildChart(days, todayStr);
  const chartCard = UI.chartContainer(RANGE_LABELS[currentRange] || 'Usage', chartHtml, rangeSelector);

  // Token summary table
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

  const tokenTableHtml = UI.tableCard('Token Usage', `<table class="usage-breakdown-table">
    <thead><tr>
      <th>Period</th><th class="num">Input</th><th class="num">Output</th>
      <th class="num">Cache</th><th class="num">Total</th>
    </tr></thead>
    <tbody>
      <tr>
        <td>Today</td>
        <td class="num">${formatTokens(todayTokens.input)}</td>
        <td class="num">${formatTokens(todayTokens.output)}</td>
        <td class="num">${formatTokens(todayTokens.cache)}</td>
        <td class="num">${formatTokens(todayTokens.total)}</td>
      </tr>
      <tr>
        <td>${periodLabel}</td>
        <td class="num">${formatTokens(periodTokens.input)}</td>
        <td class="num">${formatTokens(periodTokens.output)}</td>
        <td class="num">${formatTokens(periodTokens.cache)}</td>
        <td class="num">${formatTokens(periodTokens.total)}</td>
      </tr>
    </tbody>
  </table>`);

  const view = document.querySelector('.usage-view');
  if (!view) return;
  view.innerHTML =
    metricsHtml +
    tokenTableHtml +
    '<div id="usage-chart-section">' + chartCard + '</div>' +
    '<div id="usage-agent-breakdown">' + UI.loadingBlock() + '</div>';

  loadAgentBreakdown(currentAgents);
}

function buildDelta(todayCost, yesterdayCost) {
  if (yesterdayCost == null || yesterdayCost === 0) return null;
  const pct = ((todayCost - yesterdayCost) / yesterdayCost) * 100;
  const absPct = Math.abs(pct);
  const fmt = absPct < 10 ? absPct.toFixed(1) + '%' : Math.round(absPct) + '%';
  if (pct > 0) return { direction: 'up', text: '\u25B2 ' + fmt + ' vs yesterday' };
  if (pct < 0) return { direction: 'down', text: '\u25BC ' + fmt + ' vs yesterday' };
  return { direction: 'neutral', text: '\u2014 same as yesterday' };
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
    barsHtml += UI.chartBar({ height, label, value, isToday });
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
    container.innerHTML = '<div style="color:var(--text-tertiary);font-size:13px;padding:8px 0">No per-agent cost data available</div>';
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
      <td class="agent-name">${escapeHtml(name)}</td>
      <td class="model">${escapeHtml((u.Model || r.agent.model || '?').toLowerCase())}</td>
      <td class="num">${formatTokens(input)}</td>
      <td class="num">${formatTokens(output)}</td>
      <td class="num">${formatTokens(cache)}</td>
      <td class="cost">${formatCostFull(u.CostUSD)}</td>
    </tr>`;
  }

  let footerHtml = '';
  if (valid.length >= 2) {
    footerHtml = `<tfoot><tr>
      <td class="agent-name">Total</td>
      <td></td>
      <td class="num">${formatTokens(totals.input)}</td>
      <td class="num">${formatTokens(totals.output)}</td>
      <td class="num">${formatTokens(totals.cache)}</td>
      <td class="cost">${formatCostFull(totals.cost)}</td>
    </tr></tfoot>`;
  }

  const tableHtml = `<table class="usage-breakdown-table">
    <thead><tr>
      <th>Agent</th><th>Model</th>
      <th class="num">Input</th><th class="num">Output</th>
      <th class="num">Cache</th><th class="num">Cost</th>
    </tr></thead>
    <tbody>${rows}</tbody>
    ${footerHtml}
  </table>`;

  container.innerHTML = UI.tableCard('Per-agent breakdown', tableHtml);
}
