// Usage view.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { escapeHtml, repoName } from '../format.js';
import { get } from '../api.js';

export async function renderUsage(app, agents) {
  app.innerHTML = UI.header('Usage',
    UI.btn('&larr; Back', { variant: 'ghost', onclick: "Dashboard.showList()" })
  ) + '<div class="usage-view">' + UI.loadingBlock() + '</div>';

  const data = await get('/api/usage/daily');
  if (!data) return;

  const days = data.days || [];
  const maxCost = Math.max(...days.map(d => d.cost_usd), 0.01);

  const ySteps = 4;
  let yAxisHtml = '<div class="usage-y-axis">';
  for (let i = ySteps; i >= 0; i--) {
    const val = (maxCost / ySteps * i);
    yAxisHtml += `<span class="usage-y-label">$${val < 1 ? val.toFixed(2) : val.toFixed(1)}</span>`;
  }
  yAxisHtml += '</div>';

  let chartHtml = '<div class="usage-chart">';
  for (const day of days) {
    const height = Math.max(2, (day.cost_usd / maxCost) * 100);
    const label = day.date.slice(5);
    const value = '$' + day.cost_usd.toFixed(2);
    chartHtml += `<div class="usage-bar" style="height:${height}%"><span class="usage-bar-value">${value}</span><span class="usage-bar-label">${label}</span></div>`;
  }
  chartHtml += '</div>';

  const weekTotal = days.reduce((sum, d) => sum + d.cost_usd, 0);

  document.querySelector('.usage-view').innerHTML = `
    <div class="usage-summary">
      <div class="usage-card">
        <div class="usage-card-icon">${ICONS.calendar}</div>
        <div class="usage-card-value">$${(data.today_cost || 0).toFixed(2)}</div>
        <div class="usage-card-label">Today</div>
      </div>
      <div class="usage-card">
        <div class="usage-card-icon">${ICONS.chart}</div>
        <div class="usage-card-value">$${weekTotal.toFixed(2)}</div>
        <div class="usage-card-label">This Week</div>
      </div>
      <div class="usage-card">
        <div class="usage-card-icon">${ICONS.sigma}</div>
        <div class="usage-card-value">$${(data.total_cost || 0).toFixed(2)}</div>
        <div class="usage-card-label">All Time</div>
      </div>
    </div>
    <h3 class="usage-chart-title">Last 7 Days</h3>
    <div class="usage-chart-container">
      ${yAxisHtml}
      ${chartHtml}
    </div>
    <h3 class="usage-chart-title" style="margin-top:24px">Per-Agent Breakdown</h3>
    <div id="usage-agent-breakdown">${UI.loadingBlock()}</div>
  `;

  loadAgentBreakdown(agents);
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

  const fmtTokens = (n) => {
    if (!n) return '0';
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
    return String(n);
  };

  let html = '<table class="usage-breakdown-table"><thead><tr><th>Agent</th><th>Model</th><th class="num">Input</th><th class="num">Output</th><th class="num">Cache</th><th class="num">Cost</th></tr></thead><tbody>';
  for (const r of valid) {
    const name = repoName(r.agent);
    const u = r.usage;
    html += `<tr>
      <td>${escapeHtml(name)}</td>
      <td><span class="badge badge-blue">${escapeHtml(u.Model || r.agent.model || '?')}</span></td>
      <td class="num">${fmtTokens(u.InputTokens)}</td>
      <td class="num">${fmtTokens(u.OutputTokens)}</td>
      <td class="num">${fmtTokens((u.CacheReadTokens || 0) + (u.CacheWriteTokens || 0))}</td>
      <td class="num">$${u.CostUSD.toFixed(2)}</td>
    </tr>`;
  }
  html += '</tbody></table>';
  container.innerHTML = html;
}
