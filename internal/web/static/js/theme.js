// Theme management — light/dark switching with localStorage persistence.
const STORAGE_KEY = 'theme-preference';
const HLJS_DARK = 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github-dark.min.css';
const HLJS_LIGHT = 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github.min.css';

const ICON_SUN = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>';
const ICON_MOON = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>';

export const Theme = {
  init() {
    this.apply(this.getPreference());
  },

  getPreference() {
    return localStorage.getItem(STORAGE_KEY) || 'dark';
  },

  getEffective() {
    return this.getPreference();
  },

  apply(mode) {
    document.documentElement.dataset.theme = mode;
    // Swap highlight.js stylesheet
    const link = document.getElementById('hljs-theme');
    if (link) link.href = mode === 'light' ? HLJS_LIGHT : HLJS_DARK;
    // Update meta theme-color
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = mode === 'light' ? '#FFFFFF' : '#0B0E14';
  },

  cycle() {
    const next = this.getPreference() === 'dark' ? 'light' : 'dark';
    localStorage.setItem(STORAGE_KEY, next);
    // Cross-fade window — CSS .theme-cycling rule enables a 240ms
    // transition on color tokens during this turn only, so element
    // hover/focus transitions keep their normal cadence after.
    const root = document.documentElement;
    root.classList.add('theme-cycling');
    this.apply(next);
    setTimeout(() => root.classList.remove('theme-cycling'), 280);
    // Refresh every theme-toggle button on the page (app-bar + sidebar).
    document.querySelectorAll('[data-theme-toggle]').forEach(btn => {
      const slot = btn.querySelector('[data-theme-icon]');
      if (slot) slot.innerHTML = this.getIcon();
      else btn.innerHTML = this.getIcon();
      const label = this.getNextLabel();
      btn.setAttribute('aria-label', label);
      const text = btn.querySelector('[data-theme-label]');
      if (text) text.textContent = label;
    });
  },

  getIcon() {
    return this.getPreference() === 'light' ? ICON_SUN : ICON_MOON;
  },

  // Label for the *next* state — used in sidebar row text.
  getNextLabel() {
    return this.getPreference() === 'light' ? 'Switch to dark' : 'Switch to light';
  },

  // App-bar trailing entry config, drop-in for UI.appBar({ trailing: [...] }).
  trailingEntry() {
    return {
      icon: this.getIcon(),
      ariaLabel: this.getNextLabel(),
      onclick: 'Dashboard.cycleTheme()',
      cls: 'ui-app-bar__theme',
      dataAttr: 'theme-toggle',
    };
  },
};
