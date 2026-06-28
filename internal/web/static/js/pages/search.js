// Search overlay — Codex-style fuzzy command palette for agents.
// Mounted as a floating centered panel; filters the in-memory agents list.
import { escapeHtml, repoName } from '../format.js';
import { effectiveState, stateGroup } from '../state.js';
import { fuzzyRank } from '../fuzzy.js';
import { ICONS } from '../icons.js';

const OVERLAY_ID = 'search-overlay-root';
const INPUT_ID = 'search-overlay-input';

let currentResults = [];
let selectedIndex = 0;
let keydownHandler = null;
// When true, the palette shows only restart-survivor (resumable) orphans.
// Toggled by the header chip or Cmd/Ctrl+O while the overlay is open.
let orphanOnly = false;

// Wraps matched character indices in <strong> tags and HTML-escapes the
// rest. `indices` is an ordered ascending array; falsy ⇒ no highlights.
export function highlightMatched(text, indices) {
  const s = text || '';
  if (!indices || indices.length === 0) return escapeHtml(s);
  let out = '';
  let cursor = 0;
  for (const idx of indices) {
    if (idx > cursor) out += escapeHtml(s.slice(cursor, idx));
    out += '<strong class="search-overlay__hit">' + escapeHtml(s.charAt(idx)) + '</strong>';
    cursor = idx + 1;
  }
  if (cursor < s.length) out += escapeHtml(s.slice(cursor));
  return out;
}

// Pure ranker — fuzzy-matches `needle` against the agent's repo name, branch,
// cwd, and last message preview so restart-survivors are findable by more than
// repo. When `onlyResumable` is true the pool is pre-filtered to resumable
// orphans. Returns sorted result objects { item, score, indicesByField }.
export function searchAgents(agents, needle, max, onlyResumable) {
  const cap = typeof max === 'number' ? max : 50;
  const pool = onlyResumable ? agents.filter((a) => a.resumable) : agents;
  const ranked = fuzzyRank(needle || '', pool, (a) => [
    repoName(a),
    a.branch || '',
    a.cwd || '',
    a.last_message_preview || '',
  ]);
  return ranked.slice(0, cap);
}

// resumableBadge returns the "resumable" pill HTML for a restart-survivor agent
// (pane died but its session can be continued), or '' for a live agent.
export function resumableBadge(agent) {
  if (!agent || !agent.resumable) return '';
  return '<span class="search-overlay__tag search-overlay__tag--resumable">resumable</span>';
}

function statusDotHTML(state) {
  const g = stateGroup(state);
  return `<span class="status-dot status-dot--${g.toLowerCase()}"></span>`;
}

// Maps an agent state to the accent modifier class for the search-overlay
// status pill. Mirrors the sidebar's state-dot semantics so the WAITING /
// RUNNING / REVIEW / PR / BLOCKED tags carry their accent colour instead
// of the default neutral grey. Returns '' for merged / unknown so callers
// can append unconditionally without producing dangling spaces.
export function searchTagClass(state) {
  const g = stateGroup(state);
  switch (g) {
    case 'WAITING': return 'search-overlay__tag--waiting';
    case 'BLOCKED': return 'search-overlay__tag--blocked';
    case 'RUNNING': return 'search-overlay__tag--running';
    case 'REVIEW':  return 'search-overlay__tag--review';
    case 'PR':      return 'search-overlay__tag--pr';
    default:        return '';
  }
}

function rowHTML(result, index) {
  const a = result.item;
  const indicesByField = result.indicesByField || [null, null];
  const titleHTML = highlightMatched(repoName(a), indicesByField[0]);
  const subtitleHTML = a.branch ? highlightMatched(a.branch, indicesByField[1]) : '';
  const state = effectiveState(a);
  const tag = stateGroup(state);
  const tagMod = searchTagClass(state);
  const tagCls = tagMod ? 'search-overlay__tag ' + tagMod : 'search-overlay__tag';
  const sel = index === selectedIndex ? ' search-overlay__row--selected' : '';
  return (
    `<button class="search-overlay__row${sel}" data-agent-id="${escapeHtml(a.session_id)}" data-index="${index}" role="option">` +
      `<span class="search-overlay__leading">${statusDotHTML(state)}</span>` +
      `<span class="search-overlay__rowbody">` +
        `<span class="search-overlay__title">${titleHTML}</span>` +
        (subtitleHTML ? `<span class="search-overlay__subtitle">${subtitleHTML}</span>` : '') +
      `</span>` +
      resumableBadge(a) +
      `<span class="${tagCls}">${escapeHtml(tag)}</span>` +
    `</button>`
  );
}

function renderResults(container, needle, agents) {
  currentResults = searchAgents(agents, needle, 50, orphanOnly);
  if (selectedIndex >= currentResults.length) {
    selectedIndex = Math.max(0, currentResults.length - 1);
  }
  if (currentResults.length === 0) {
    const trimmed = (needle || '').trim();
    container.innerHTML = trimmed
      ? `<div class="search-overlay__empty">No agents match "${escapeHtml(trimmed)}"</div>`
      : `<div class="search-overlay__empty">No agents yet.</div>`;
    return;
  }
  container.innerHTML = currentResults.map((r, i) => rowHTML(r, i)).join('');
}

function moveSelection(delta, container) {
  if (currentResults.length === 0) return;
  selectedIndex = (selectedIndex + delta + currentResults.length) % currentResults.length;
  container.querySelectorAll('.search-overlay__row').forEach((el, i) => {
    el.classList.toggle('search-overlay__row--selected', i === selectedIndex);
    if (i === selectedIndex) el.scrollIntoView({ block: 'nearest' });
  });
}

function activateSelected() {
  if (currentResults.length === 0) return;
  const item = currentResults[selectedIndex].item;
  const id = item.session_id;
  closeSearch();
  // A restart-survivor orphan can't be jumped to (its pane is dead) — resume
  // it instead. Live agents keep the jump-to-pane behaviour.
  if (item.resumable && window.Dashboard && typeof window.Dashboard.resumeAgent === 'function') {
    window.Dashboard.resumeAgent(id);
    return;
  }
  if (window.Dashboard && typeof window.Dashboard.selectAgent === 'function') {
    window.Dashboard.selectAgent(id);
  }
}

export function openSearch(agents) {
  if (document.getElementById(OVERLAY_ID)) return;
  selectedIndex = 0;
  orphanOnly = false;
  const overlay = document.createElement('div');
  overlay.id = OVERLAY_ID;
  overlay.className = 'search-overlay';
  overlay.innerHTML = `
    <div class="search-overlay__panel" role="dialog" aria-modal="true" aria-label="Search agents">
      <div class="search-overlay__inputrow">
        <span class="search-overlay__icon" aria-hidden="true">${ICONS.search || ''}</span>
        <input id="${INPUT_ID}" class="search-overlay__input" type="text" placeholder="Search agents" autocomplete="off" spellcheck="false" />
        <button type="button" class="search-overlay__filter" aria-pressed="false" title="Show only resumable agents (⌘O)">Orphaned only</button>
      </div>
      <div class="search-overlay__results" role="listbox"></div>
    </div>
  `;
  document.body.appendChild(overlay);

  const input = overlay.querySelector('#' + INPUT_ID);
  const results = overlay.querySelector('.search-overlay__results');
  const filterChip = overlay.querySelector('.search-overlay__filter');
  renderResults(results, '', agents);

  const toggleOrphan = () => {
    orphanOnly = !orphanOnly;
    filterChip.classList.toggle('search-overlay__filter--on', orphanOnly);
    filterChip.setAttribute('aria-pressed', orphanOnly ? 'true' : 'false');
    selectedIndex = 0;
    renderResults(results, input.value, agents);
    input.focus();
  };

  input.addEventListener('input', () => {
    selectedIndex = 0;
    renderResults(results, input.value, agents);
  });

  filterChip.addEventListener('click', toggleOrphan);

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closeSearch();
  });

  results.addEventListener('click', (e) => {
    const btn = e.target.closest('.search-overlay__row');
    if (!btn) return;
    const idx = parseInt(btn.getAttribute('data-index') || '0', 10);
    selectedIndex = isNaN(idx) ? 0 : idx;
    activateSelected();
  });

  keydownHandler = (e) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeSearch();
    } else if ((e.metaKey || e.ctrlKey) && (e.key === 'o' || e.key === 'O')) {
      e.preventDefault();
      toggleOrphan();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      moveSelection(1, results);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      moveSelection(-1, results);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      activateSelected();
    }
  };
  document.addEventListener('keydown', keydownHandler);

  setTimeout(() => input.focus(), 0);
}

export function closeSearch() {
  const overlay = document.getElementById(OVERLAY_ID);
  if (overlay) overlay.remove();
  if (keydownHandler) {
    document.removeEventListener('keydown', keydownHandler);
    keydownHandler = null;
  }
  currentResults = [];
  selectedIndex = 0;
  orphanOnly = false;
}

export function isSearchOpen() {
  return !!document.getElementById(OVERLAY_ID);
}
