// Reusable UI primitives for the Codex-iOS register.
// 9 primitives only. KISS/DRY: 2+ callers or explicit structural exception.
import { escapeHtml } from './format.js';
import { ICONS } from './icons.js';

function actionsHtml(items) {
  if (!items || !items.length) return '';
  let out = '';
  for (const a of items) {
    if (!a) continue;
    if (a === 'spinner') {
      out += '<span class="ui-app-bar__spinner" aria-hidden="true"></span>';
      continue;
    }
    const click = a.onclick ? ` onclick="${a.onclick}"` : '';
    const label = a.ariaLabel || a.label || 'action';
    out += `<button class="ui-app-bar__action" aria-label="${escapeHtml(label)}"${click}>${a.icon || ''}</button>`;
  }
  return out;
}

export const UI = {
  // 1. App bar — top chrome on every view.
  appBar(opts) {
    const o = opts || {};
    const lead = o.back
      ? `<button class="ui-app-bar__action ui-app-bar__back" aria-label="Back" onclick="Dashboard.showList()">${ICONS.back}</button>`
      : o.leading
      ? `<button class="ui-app-bar__action" aria-label="${escapeHtml(o.leading.ariaLabel || 'leading')}" onclick="${o.leading.onclick || ''}">${o.leading.icon}</button>`
      : '<span class="ui-app-bar__spacer"></span>';
    const sub = o.subtitle ? `<span class="ui-app-bar__subtitle">${escapeHtml(o.subtitle)}</span>` : '';
    const title = o.title
      ? `<div class="ui-app-bar__titles"><span class="ui-app-bar__title">${escapeHtml(o.title)}</span>${sub}</div>`
      : '<span class="ui-app-bar__spacer"></span>';
    return `<header class="ui-app-bar">${lead}${title}<div class="ui-app-bar__trailing">${actionsHtml(o.trailing)}</div></header>`;
  },

  // 2. Floating dock — list view only on mobile.
  // On desktop callers pass `placement: 'header'` to render the same
  // actions as inline pills inside an app-bar trailing slot (Phase C
  // dock-migration). The floating variant remains the default so
  // existing mobile callers keep working unchanged. Structural
  // exception (no row primitive applies).
  dock(opts) {
    const o = opts || {};
    const placement = o.placement === 'header' ? 'header' : 'floating';
    if (placement === 'header') {
      const search = o.search
        ? `<button class="ui-dock__search ui-dock--header__search" aria-label="${escapeHtml(o.search.label)}" onclick="${o.search.onclick || ''}">${ICONS.search}</button>`
        : '';
      const cta = o.cta
        ? `<button class="ui-dock__cta ui-dock--header__cta" onclick="${o.cta.onclick || ''}">${o.cta.icon || ''}<span>${escapeHtml(o.cta.label)}</span></button>`
        : '';
      return `<div class="ui-dock ui-dock--header" role="group">${search}${cta}</div>`;
    }
    const search = o.search
      ? `<button class="ui-dock__search" onclick="${o.search.onclick || ''}">${ICONS.search}<span>${escapeHtml(o.search.label)}</span></button>`
      : '';
    const cta = o.cta
      ? `<button class="ui-dock__cta" onclick="${o.cta.onclick || ''}">${o.cta.icon || ''}<span>${escapeHtml(o.cta.label)}</span></button>`
      : '';
    return `<nav class="ui-dock" role="navigation">${search}${cta}</nav>`;
  },

  // 3. Action sheet — kebab-driven, modal focus. Structural exception.
  sheet(items, opts) {
    const o = opts || {};
    let body = '';
    for (const it of items) {
      const click = it.onclick ? ` onclick="${it.onclick}"` : '';
      body += `<button class="ui-sheet__item"${click}>${it.icon || ''}<span>${escapeHtml(it.label)}</span><span class="ui-sheet__chevron">${ICONS.chevronRight}</span></button>`;
    }
    return `<div class="ui-sheet" role="dialog" aria-modal="true">
      <div class="ui-sheet__backdrop" onclick="${o.onDismiss || 'Dashboard.dismissSheet()'}"></div>
      <div class="ui-sheet__panel">${body}</div>
    </div>`;
  },

  // 4. Tappable row — list + create form rows.
  row(opts) {
    const o = opts || {};
    const click = o.onclick ? ` onclick="${o.onclick}"` : '';
    const lead = o.leading ? `<div class="ui-row__leading">${o.leading}</div>` : '';
    const sub = o.subtitle ? `<span class="ui-row__subtitle">${escapeHtml(o.subtitle)}</span>` : '';
    const chevron = (o.onclick && o.chevron !== false) ? `<span class="ui-row__chevron">${ICONS.chevronRight}</span>` : '';
    const trail = (o.trailing || chevron)
      ? `<div class="ui-row__trailing">${o.trailing || ''}${chevron}</div>` : '';
    const tag = o.onclick ? 'button' : 'div';
    return `<${tag} class="ui-row"${click}>${lead}
      <div class="ui-row__body"><span class="ui-row__title">${escapeHtml(o.title || '')}</span>${sub}</div>
      ${trail}
    </${tag}>`;
  },

  // 5. Section label — small-caps muted header. Used everywhere.
  sectionLabel(text, opts) {
    const o = opts || {};
    const meta = o.count != null ? ` <span class="ui-section-label__count">${o.count}</span>` : '';
    const action = o.action ? `<button class="ui-section-label__action" onclick="${o.action.onclick}">${escapeHtml(o.action.label)}</button>` : '';
    return `<div class="ui-section-label"><span>${escapeHtml(text)}${meta}</span>${action}</div>`;
  },

  // 6. Card — layered surface, no border.
  card(content, opts) {
    const o = opts || {};
    const click = o.onclick ? ` onclick="${o.onclick}"` : '';
    return `<div class="ui-card"${click}>${content}</div>`;
  },

  // 7. Message — flat chat. user pill / assistant prose / tool footer.
  message(role, content, opts) {
    const o = opts || {};
    if (o.tool) {
      const label = escapeHtml(o.tool.label || 'tool');
      return `<div class="ui-msg__tool">${ICONS.check || ''}<span>${label}</span>${ICONS.chevronRight}</div>`;
    }
    if (role === 'user') {
      return `<div class="ui-msg ui-msg--user"><div class="ui-msg__bubble">${escapeHtml(content)}</div></div>`;
    }
    const body = o.html ? content : escapeHtml(content);
    const copy = o.copyable === false ? '' : `<button class="ui-msg__copy" aria-label="Copy">${ICONS.copy}</button>`;
    return `<div class="ui-msg ui-msg--assistant"><div class="ui-msg__prose">${body}</div>${copy}</div>`;
  },

  // 8. Composer — sticky bottom input + send. Structural exception.
  composer(opts) {
    const o = opts || {};
    const id = o.id || 'composer-input';
    const placeholder = escapeHtml(o.placeholder || 'Message');
    const onSend = o.onSend || '';
    return `<div class="ui-composer">
      <button class="ui-composer__attach" aria-label="Attach" tabindex="-1">${ICONS.attach}</button>
      <textarea
        class="ui-composer__input"
        id="${id}"
        rows="1"
        placeholder="${placeholder}"
        oninput="UI.composerAutoSize(this)"
        onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();${onSend}}"
      ></textarea>
      <button class="ui-composer__send" aria-label="Send" onclick="${onSend}">${ICONS.send}</button>
    </div>`;
  },

  // 9. Input — rounded text field. Used by create + internally by composer.
  input(opts) {
    const o = opts || {};
    const id = o.id ? ` id="${o.id}"` : '';
    const ph = escapeHtml(o.placeholder || '');
    const val = escapeHtml(o.value || '');
    if (o.multiline) {
      return `<textarea class="ui-input ui-input--multiline"${id} placeholder="${ph}" rows="${o.rows || 3}">${val}</textarea>`;
    }
    return `<input class="ui-input" type="text"${id} placeholder="${ph}" value="${val}">`;
  },

  // Helper for composer auto-grow (called inline; intentionally on UI, not a primitive).
  composerAutoSize(el) {
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 160) + 'px';
  },

  // Loading placeholder — used by Usage view and any future page that
  // needs a spinner while fetching. Intentionally tiny.
  loadingBlock() {
    return '<div class="loading"><span class="spinner"></span></div>';
  },
};
