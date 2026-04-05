// Theme management — light/dark/system switching with localStorage persistence.
const STORAGE_KEY = 'theme-preference';
const HLJS_DARK = 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github-dark.min.css';
const HLJS_LIGHT = 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github.min.css';

const ICON_SUN = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>';
const ICON_MOON = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>';
const ICON_SYSTEM = '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>';

function systemPrefersDark() {
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export const Theme = {
  init() {
    // Listen for OS-level theme changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (this.getPreference() === 'system') {
        this.apply(systemPrefersDark() ? 'dark' : 'light');
      }
    });
    // Apply stored preference
    const pref = this.getPreference();
    const effective = pref === 'system' ? (systemPrefersDark() ? 'dark' : 'light') : pref;
    this.apply(effective);
  },

  getPreference() {
    return localStorage.getItem(STORAGE_KEY) || 'system';
  },

  getEffective() {
    const pref = this.getPreference();
    if (pref === 'system') return systemPrefersDark() ? 'dark' : 'light';
    return pref;
  },

  apply(mode) {
    document.documentElement.dataset.theme = mode;
    // Swap highlight.js stylesheet
    const link = document.getElementById('hljs-theme');
    if (link) link.href = mode === 'light' ? HLJS_LIGHT : HLJS_DARK;
    // Update meta theme-color
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = mode === 'light' ? '#2A2724' : '#0B0E14';
  },

  cycle() {
    const order = ['system', 'light', 'dark'];
    const current = this.getPreference();
    const next = order[(order.indexOf(current) + 1) % order.length];
    localStorage.setItem(STORAGE_KEY, next);
    const effective = next === 'system' ? (systemPrefersDark() ? 'dark' : 'light') : next;
    this.apply(effective);
    // Update toggle button icon
    const btn = document.querySelector('.btn-theme-toggle');
    if (btn) btn.innerHTML = this.getIcon();
  },

  getIcon() {
    const pref = this.getPreference();
    if (pref === 'light') return ICON_SUN;
    if (pref === 'dark') return ICON_MOON;
    return ICON_SYSTEM;
  },
};
