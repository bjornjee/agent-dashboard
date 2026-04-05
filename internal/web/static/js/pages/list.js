// Agent list view.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { STATE_BORDER, effectiveState, stateGroup } from '../state.js';
import { escapeHtml, repoName, duration, stripMarkdown } from '../format.js';
import { get } from '../api.js';

export function renderList(app, agents) {
  const grouped = {};
  for (const agent of agents) {
    const g = stateGroup(effectiveState(agent));
    if (!grouped[g]) grouped[g] = [];
    grouped[g].push(agent);
  }

  const order = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];
  let html = UI.header('Agent Dashboard',
    UI.btn('Usage', { variant: 'ghost', onclick: "Dashboard.showUsage()" })
    + UI.btn('+ New', { variant: 'ghost', onclick: "Dashboard.showCreate()" })
  );
  html += '<div class="agent-list">';

  if (agents.length === 0) {
    html += UI.emptyState(ICONS.robot, 'No agents running', 'Create a new agent to get started');
  }

  for (const group of order) {
    if (!grouped[group] || grouped[group].length === 0) continue;
    html += `<div class="state-group">${UI.stateGroupLabel(group, grouped[group].length)}`;
    for (const agent of grouped[group]) {
      const st = effectiveState(agent);
      const cardContent = `
        <div class="agent-card-header">
          <span class="agent-name">${escapeHtml(repoName(agent))}</span>
          ${UI.badge(st, st)}
        </div>
        <div class="agent-meta">
          ${agent.branch ? '<span>' + escapeHtml(agent.branch) + '</span>' : ''}
          ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
          ${agent.started_at ? '<span>' + duration(agent) + '</span>' : ''}
          ${agent.subagent_count > 0 ? '<span>' + agent.subagent_count + ' subagents</span>' : ''}
          ${agent.current_tool ? '<span class="agent-current-tool">' + escapeHtml(agent.current_tool) + '</span>' : ''}
          <span class="agent-cost" data-agent-id="${agent.session_id}"></span>
        </div>
        ${agent.last_message_preview ? '<div class="agent-preview">' + escapeHtml(stripMarkdown(agent.last_message_preview)) + '</div>' : ''}
      `;
      html += UI.card(cardContent, {
        borderColor: STATE_BORDER[st] || 'var(--border-subtle)',
        onclick: `Dashboard.selectAgent('${agent.session_id}')`,
        className: 'agent-card',
      });
    }
    html += '</div>';
  }

  html += '</div>';
  app.innerHTML = html;
  loadAgentCosts();
}

async function loadAgentCosts() {
  const els = document.querySelectorAll('.agent-cost[data-agent-id]');
  if (!els.length) return;
  await Promise.all(Array.from(els).map(async (el) => {
    try {
      const u = await get('/api/agents/' + el.dataset.agentId + '/usage');
      if (u && u.CostUSD > 0) {
        el.textContent = '$' + u.CostUSD.toFixed(2);
      }
    } catch { /* ignore */ }
  }));
}
