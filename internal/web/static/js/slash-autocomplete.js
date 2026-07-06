// Slash-command autocomplete for the detail composer.
//
// Triggered when the user types the harness's command sigil at the start
// of the textarea or after whitespace ("/" for claude, "$" for codex).
// Fetches /api/skills (cached per harness) and renders a small popup of
// matching commands. Arrow keys navigate, Enter / Tab inserts, Esc
// dismisses.

import { get } from './api.js';
import { escapeHtml } from './format.js';

const skillsCacheByHarness = new Map();
let activeIndex = 0;
let currentMatches = [];

// Claude uses /agent-dashboard:<skill>; codex uses $agent-dashboard:<skill>.
// commandConfig returns the trigger sigil + rendered prefix for a harness.
function commandConfig(harness) {
  if (harness === 'codex') {
    return { sigil: '$', prefix: '$agent-dashboard:' };
  }
  return { sigil: '/', prefix: '/agent-dashboard:' };
}

async function loadSkills(harness) {
  if (skillsCacheByHarness.has(harness)) return skillsCacheByHarness.get(harness);
  const url = harness ? `/api/skills?harness=${encodeURIComponent(harness)}` : '/api/skills';
  let skills;
  try {
    const res = await get(url);
    skills = Array.isArray(res) ? res : [];
  } catch {
    skills = [];
  }
  skillsCacheByHarness.set(harness, skills);
  return skills;
}

// Returns the command-fragment the user is typing at the cursor, or null
// if the cursor isn't in a command context. Recognises "<sigil>foo" at
// the start of the input or directly after whitespace.
function detectCommandFragment(input, sigil) {
  const value = input.value || '';
  const cursor = input.selectionStart ?? value.length;
  let start = cursor;
  while (start > 0 && !/\s/.test(value[start - 1])) start--;
  const word = value.slice(start, cursor);
  if (!word.startsWith(sigil)) return null;
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
    const hint = m.hint ? `<span class="slash-autocomplete__hint">${escapeHtml(m.hint)}</span>` : '';
    return `<button class="slash-autocomplete__item${cls}" data-idx="${i}" type="button">
      <span class="slash-autocomplete__cmd">${escapeHtml(m.full)}</span>
      ${hint}
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

async function update(input, harness) {
  const cfg = commandConfig(harness);
  const ctx = detectCommandFragment(input, cfg.sigil);
  if (!ctx) {
    hidePopup();
    return;
  }
  const skills = await loadSkills(harness);
  const all = skills.map((s) => ({
    full: cfg.prefix + s,
    skill: s,
    hint: '',
  }));
  // Filter: match against fragment minus the leading sigil.
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

function applySelection(input, match, harness) {
  const cfg = commandConfig(harness);
  const ctx = detectCommandFragment(input, cfg.sigil);
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
// calls replace the prior listeners. The harness argument selects the
// command sigil ("/" for claude, "$" for codex) and the skill list.
export function attachSlashAutocomplete(input, harness) {
  if (!input) return () => {};
  // Prevent double-binding.
  if (input.dataset.slashBound === 'true') return () => {};
  input.dataset.slashBound = 'true';

  const onInput = () => update(input, harness);
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
      const cfg = commandConfig(harness);
      const ctx = detectCommandFragment(input, cfg.sigil);
      const complete = ctx && currentMatches.some((m) => m.full === ctx.fragment);
      if (e.key === 'Enter' && complete) {
        hidePopup();
        return;
      }
      // Only intercept Enter when the popup is open; otherwise the
      // normal Enter-to-send handler in detail.js takes over.
      e.preventDefault();
      e.stopPropagation();
      applySelection(input, currentMatches[activeIndex], harness);
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
    if (currentMatches[idx]) applySelection(input, currentMatches[idx], harness);
  });

  return () => {
    input.removeEventListener('input', onInput);
    input.removeEventListener('keydown', onKeyDown, true);
    input.removeEventListener('blur', onBlur);
    input.dataset.slashBound = '';
  };
}
