// Agent list view.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState, stateGroup } from '../state.js';
import { escapeHtml, repoName, durationShort, stripMarkdown, formatCost } from '../format.js';
import { get } from '../api.js';

export function renderList(app, agents) {
  const grouped = {};
  for (const agent of agents) {
    const g = stateGroup(effectiveState(agent));
    if (!grouped[g]) grouped[g] = [];
    grouped[g].push(agent);
  }

  const order = ['BLOCKED', 'WAITING', 'RUNNING', 'REVIEW', 'PR', 'MERGED'];
  let html = UI.header('Agent Dashboard', {
    actions: [{ label: 'Usage', onclick: 'Dashboard.showUsage()' }],
    cta: { label: '+ New', onclick: 'Dashboard.showCreate()' },
  });
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
        ${agent.branch ? '<div class="agent-branch">' + escapeHtml(agent.branch) + '</div>' : ''}
        <div class="agent-meta">
          ${agent.model ? '<span>' + escapeHtml(agent.model) + '</span>' : ''}
          ${agent.started_at ? '<span>' + durationShort(agent) + '</span>' : ''}
          ${agent.subagent_count > 0 ? '<span>' + agent.subagent_count + ' subagents</span>' : ''}
        </div>
        <div class="agent-card__subagents" data-agent-id="${agent.session_id}" data-subagent-count="${agent.subagent_count || 0}"></div>
        <div class="agent-bottom-row">
          ${agent.current_tool ? '<span class="agent-current-tool">' + escapeHtml(agent.current_tool) + '</span>' : ''}
          <span class="agent-cost" data-agent-id="${agent.session_id}"></span>
        </div>
      `;
      html += UI.card(cardContent, {
        onclick: `Dashboard.selectAgent('${agent.session_id}')`,
        className: 'agent-card',
      });
    }
    html += '</div>';
  }

  html += '</div>';
  app.innerHTML = html;
  loadAgentCosts();
  loadSubagentRows();
}

async function loadAgentCosts() {
  const els = document.querySelectorAll('.agent-cost[data-agent-id]');
  if (!els.length) return;
  await Promise.all(Array.from(els).map(async (el) => {
    try {
      const u = await get('/api/agents/' + el.dataset.agentId + '/usage');
      if (u && u.CostUSD > 0) {
        el.innerHTML = '<span class="agent-cost-label">Cost</span>' + formatCost(u.CostUSD);
      }
    } catch { /* ignore */ }
  }));
}

async function loadSubagentRows() {
  const els = document.querySelectorAll('.agent-card__subagents[data-agent-id]');
  if (!els.length) return;
  await Promise.all(Array.from(els).map(async (el) => {
    const count = parseInt(el.dataset.subagentCount, 10);
    if (!count) return;
    try {
      const subs = await get('/api/agents/' + el.dataset.agentId + '/subagents');
      if (!subs || !subs.length) return;
      let html = '';
      for (const sub of subs) {
        html += UI.subagentRow({
          status: sub.Completed ? 'completed' : 'running',
          name: sub.AgentType || 'subagent',
          task: sub.Description || '',
          started_at: sub.StartedAt,
        });
      }
      el.innerHTML = html;
    } catch { /* ignore */ }
  }));
}
