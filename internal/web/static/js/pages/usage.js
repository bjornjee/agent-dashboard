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
  app.innerHTML = UI.header('Usage',
    UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })
  ) + '<div class="usage-view">' + UI.loadingBlock() + '</div>';

  await loadUsageData();
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

async function loadUsageData() {
  const data = await get('/api/usage/daily?days=' + currentRange);
  if (!data) return;

  const days = data.days || [];
  const todayStr = new Date().toISOString().slice(0, 10);
  const periodTotal = days.reduce((sum, d) => sum + d.cost_usd, 0);

  // Delta: compare today vs yesterday
  const todayCost = data.today_cost || 0;
  const yesterday = days.length >= 2 ? days[days.length - 2] : null;
  const delta = buildDelta(todayCost, yesterday ? yesterday.cost_usd : null);

  const periodLabel = currentRange === 0 ? 'All time' : 'This period';
  const metricsHtml = UI.metricsStrip([
    { label: 'Today', value: formatCostFull(todayCost), delta },
    { label: periodLabel, value: formatCostFull(periodTotal) },
    { label: 'All time', value: formatCostFull(data.total_cost || 0) },
  ]);

  const rangeSelector = UI.dateRangeSelector(RANGE_OPTIONS, currentRange, 'Dashboard.setUsageRange');
  const chartHtml = buildChart(days, todayStr);
  const chartCard = UI.chartContainer(RANGE_LABELS[currentRange] || 'Usage', chartHtml, rangeSelector);

  const view = document.querySelector('.usage-view');
  if (!view) return;
  view.innerHTML =
    metricsHtml +
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
