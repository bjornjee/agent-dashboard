// Formatting and text utilities.

export function escapeHtml(s) {
  if (!s) return '';
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function repoName(agent) {
  const dir = agent.worktree_cwd || agent.cwd || '';
  const parts = dir.replace(/\/+$/, '').split('/');
  return parts[parts.length - 1] || 'unknown';
}

export function duration(agent) {
  if (!agent.started_at) return '';
  const start = new Date(agent.started_at);
  const now = new Date();
  const totalSecs = Math.floor((now - start) / 1000);
  const mins = Math.floor(totalSecs / 60);
  const secs = totalSecs % 60;
  if (mins >= 60) {
    const hours = Math.floor(mins / 60);
    return hours + 'h ' + (mins % 60) + 'm';
  }
  return mins + 'm ' + secs + 's';
}

export function durationFromTimestamp(ts) {
  if (!ts) return '';
  const start = new Date(ts);
  const now = new Date();
  const totalSecs = Math.floor((now - start) / 1000);
  const mins = Math.floor(totalSecs / 60);
  const secs = totalSecs % 60;
  if (mins >= 60) {
    const hours = Math.floor(mins / 60);
    return hours + 'h ' + (mins % 60) + 'm';
  }
  return mins + 'm ' + secs + 's';
}

export function durationShort(agent) {
  if (!agent.started_at) return '';
  const start = new Date(agent.started_at);
  const now = new Date();
  const mins = Math.floor((now - start) / 60000);
  if (mins >= 60) return Math.floor(mins / 60) + 'h ' + (mins % 60) + 'm';
  return mins + 'm';
}

export function formatCost(value) {
  if (value == null || value <= 0) return '';
  if (value >= 1000) return '$' + value.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return '$' + value.toFixed(2);
}

export function formatTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return escapeHtml(iso);
  const now = new Date();
  const diffMs = now - d;
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return diffMins + 'm ago';
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return diffHours + 'h ago';
  return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', hour12: true }) + ', ' + d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

export function formatTimeShort(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return escapeHtml(iso);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
}

export function formatTokens(count) {
  if (!count) return '0';
  if (count >= 1000) return (count / 1000).toFixed(1) + 'k';
  return String(count);
}

export function stripMarkdown(s) {
  if (!s) return '';
  return s
    .replace(/```[\s\S]*?```/g, '')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    .replace(/^[-*+]\s+/gm, '')
    .replace(/\n{2,}/g, ' ')
    .replace(/\n/g, ' ')
    .trim();
}

export function renderMarkdown(md) {
  if (typeof marked !== 'undefined') {
    try {
      const raw = marked.parse(md);
      const safe = typeof DOMPurify !== 'undefined' ? DOMPurify.sanitize(raw) : escapeHtml(md);
      return '<div class="markdown-body">' + safe + '</div>';
    } catch (e) { /* fallback */ }
  }
  let html = escapeHtml(md);
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>');
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/^- (.+)$/gm, '<li>$1</li>');
  html = html.replace(/\n/g, '<br>');
  return '<div class="markdown-body">' + html + '</div>';
}

export function skeletonLoading(count) {
  let html = '<div style="padding:12px">';
  for (let i = 0; i < count; i++) {
    const w = 40 + Math.random() * 50;
    const align = i % 2 === 0 ? 'margin-left:auto' : '';
    html += `<div class="skeleton skeleton-block" style="width:${w}%;${align}"></div>`;
  }
  html += '</div>';
  return html;
}
