// Slash-command autocomplete for the detail composer.
//
// Triggered when the user types "/" at the start of the textarea or
// after whitespace. Fetches /api/skills (cached) and renders a small
// popup of matching commands. Arrow keys navigate, Enter / Tab inserts,
// Esc dismisses.

import { get } from './api.js';
import { escapeHtml } from './format.js';

let skillsCache = null;
let activeIndex = 0;
let currentMatches = [];

// agent-dashboard:* plugin commands. The /api/skills endpoint returns
// bare skill names; we render them with the plugin prefix the agent
// expects (Claude Code uses /plugin:skill syntax).
const COMMAND_PREFIX = '/agent-dashboard:';

async function loadSkills() {
  if (skillsCache) return skillsCache;
  try {
    const skills = await get('/api/skills');
    skillsCache = Array.isArray(skills) ? skills : [];
  } catch {
    skillsCache = [];
  }
  return skillsCache;
}

// Returns the slash-fragment the user is typing at the cursor, or null
// if the cursor isn't in a slash context. Recognises "/foo" at the
// start of the input or directly after whitespace.
function detectSlashFragment(input) {
  const value = input.value || '';
  const cursor = input.selectionStart ?? value.length;
  // Walk backwards from the cursor to find the start of the current
  // word. The word starts after the last whitespace or at index 0.
  let start = cursor;
  while (start > 0 && !/\s/.test(value[start - 1])) start--;
  const word = value.slice(start, cursor);
  if (!word.startsWith('/')) return null;
  return { start, end: cursor, fragment: word };
}

function ensurePopup() {
  let popup = document.getElementById('slash-autocomplete');
  if (popup) return popup;
  popup = document.createElement('div');
  popup.id = 'slash-autocomplete';
  popup.className = 'slash-autocomplete';
  popup.hidden = true;
  document.body.appendChild(popup);
  return popup;
}

function hidePopup() {
  const popup = document.getElementById('slash-autocomplete');
  if (popup) popup.hidden = true;
  currentMatches = [];
  activeIndex = 0;
}

function render(popup, matches, fragment) {
  if (!matches.length) {
    popup.hidden = true;
    return;
  }
  popup.hidden = false;
  popup.innerHTML = matches.map((m, i) => {
    const cls = i === activeIndex ? ' slash-autocomplete__item--active' : '';
    return `<button class="slash-autocomplete__item${cls}" data-idx="${i}" type="button">
      <span class="slash-autocomplete__cmd">${escapeHtml(m.full)}</span>
      <span class="slash-autocomplete__hint">${escapeHtml(m.hint || '')}</span>
    </button>`;
  }).join('');
}

function positionPopup(popup, input) {
  // Anchor the popup directly above the textarea, left edge aligned.
  const r = input.getBoundingClientRect();
  popup.style.left = r.left + 'px';
  popup.style.bottom = (window.innerHeight - r.top + 6) + 'px';
  popup.style.maxWidth = Math.max(280, Math.min(420, r.width)) + 'px';
}

async function update(input) {
  const ctx = detectSlashFragment(input);
  if (!ctx) {
    hidePopup();
    return;
  }
  const skills = await loadSkills();
  // Build the full command set. We support the bare prefix (/) to show
  // everything, and partial fragments after the colon or prefix.
  const all = skills.map((s) => ({
    full: COMMAND_PREFIX + s,
    skill: s,
    hint: '',
  }));
  // Filter: match against fragment minus the leading "/".
  const needle = ctx.fragment.slice(1).toLowerCase();
  currentMatches = all.filter((c) => {
    const hay = c.full.slice(1).toLowerCase();
    return needle === '' || hay.includes(needle);
  }).slice(0, 8);
  if (!currentMatches.length) {
    hidePopup();
    return;
  }
  activeIndex = Math.min(activeIndex, currentMatches.length - 1);
  const popup = ensurePopup();
  render(popup, currentMatches, ctx.fragment);
  positionPopup(popup, input);
}

function applySelection(input, match) {
  const ctx = detectSlashFragment(input);
  if (!ctx) return;
  const before = input.value.slice(0, ctx.start);
  const after = input.value.slice(ctx.end);
  // Append a trailing space so the user can continue typing arguments.
  input.value = before + match.full + ' ' + after;
  const cursor = ctx.start + match.full.length + 1;
  input.focus();
  try { input.setSelectionRange(cursor, cursor); } catch {}
  input.dispatchEvent(new Event('input', { bubbles: true }));
  hidePopup();
}

// Public — wire up a textarea to drive the popup. Returns a cleanup
// function. Safe to call multiple times on the same input; subsequent
// calls replace the prior listeners.
export function attachSlashAutocomplete(input) {
  if (!input) return () => {};
  // Prevent double-binding.
  if (input.dataset.slashBound === 'true') return () => {};
  input.dataset.slashBound = 'true';

  const onInput = () => update(input);
  const onKeyDown = (e) => {
    const popup = document.getElementById('slash-autocomplete');
    const isOpen = popup && !popup.hidden && currentMatches.length;
    if (!isOpen) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = (activeIndex + 1) % currentMatches.length;
      render(popup, currentMatches);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = (activeIndex - 1 + currentMatches.length) % currentMatches.length;
      render(popup, currentMatches);
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      // Only intercept Enter when the popup is open; otherwise the
      // normal Enter-to-send handler in detail.js takes over.
      e.preventDefault();
      e.stopPropagation();
      applySelection(input, currentMatches[activeIndex]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      hidePopup();
    }
  };
  const onBlur = () => {
    // Defer so click on a popup item can fire before the popup goes away.
    setTimeout(() => hidePopup(), 120);
  };

  input.addEventListener('input', onInput);
  input.addEventListener('keydown', onKeyDown, true); // capture so we beat Enter-to-send
  input.addEventListener('blur', onBlur);

  // Delegate clicks on the popup to applySelection.
  const popup = ensurePopup();
  popup.addEventListener('mousedown', (e) => {
    const item = e.target.closest('.slash-autocomplete__item');
    if (!item) return;
    e.preventDefault();
    const idx = parseInt(item.dataset.idx, 10);
    if (currentMatches[idx]) applySelection(input, currentMatches[idx]);
  });

  return () => {
    input.removeEventListener('input', onInput);
    input.removeEventListener('keydown', onKeyDown, true);
    input.removeEventListener('blur', onBlur);
    input.dataset.slashBound = '';
  };
}
