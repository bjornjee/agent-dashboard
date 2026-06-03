// SVG icon constants — pruned to in-scope callers + new Codex-iOS set.
// SF-symbol weight: stroke-width 1.5 on 24x24; 2.0 on smaller 16x16.

const s24 = 'viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"';
const s20 = 'viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"';
const s16 = 'viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';

export const ICONS = {
  // Brand
  logo: '<img src="/icon-logo.png" width="20" height="20" alt="Agent Dashboard" style="object-fit:contain">',

  // App-bar actions
  back: `<svg ${s20}><path d="M15 18l-6-6 6-6"/></svg>`,
  close: `<svg ${s20}><path d="M18 6L6 18M6 6l12 12"/></svg>`,
  kebab: `<svg ${s20}><circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/></svg>`,

  // Dock + composer
  search: `<svg ${s20}><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></svg>`,
  pencil: `<svg ${s20}><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 113 3L7 19l-4 1 1-4 12.5-12.5z"/></svg>`,
  send: `<svg ${s20}><path d="M12 19V5M5 12l7-7 7 7"/></svg>`,
  attach: `<svg ${s20}><path d="M12 5v14M5 12h14"/></svg>`,

  // Row affordances
  chevronRight: `<svg ${s16}><path d="M9 18l6-6-6-6"/></svg>`,
  folder: `<svg ${s24}><path d="M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"/></svg>`,
  chat: `<svg ${s24}><path d="M21 12a8 8 0 11-3.2 6.4L3 21l2.6-4.4A8 8 0 1121 12z"/></svg>`,
  check: `<svg ${s16}><path d="M5 12l5 5 9-11"/></svg>`,
  copy: `<svg ${s16}><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15V5a2 2 0 012-2h10"/></svg>`,

  // Sheet items
  gear: `<svg ${s20}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 11-4 0v-.09a1.65 1.65 0 00-1-1.51 1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 110-4h.09a1.65 1.65 0 001.51-1 1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 112.83-2.83l.06.06a1.65 1.65 0 001.82.33h0a1.65 1.65 0 001-1.51V3a2 2 0 114 0v.09a1.65 1.65 0 001 1.51h0a1.65 1.65 0 001.82-.33l.06-.06a2 2 0 112.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82v0a1.65 1.65 0 001.51 1H21a2 2 0 110 4h-.09a1.65 1.65 0 00-1.51 1z"/></svg>`,
  spark: `<svg ${s20}><polyline points="22,12 18,12 15,21 9,3 6,12 2,12"/></svg>`,
  bell: `<svg ${s20}><path d="M18 8A6 6 0 006 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 01-3.46 0"/></svg>`,
  bellOff: `<svg ${s20}><path d="M13.73 21a2 2 0 01-3.46 0"/><path d="M18.63 13A17.89 17.89 0 0118 8"/><path d="M6.26 6.26A5.86 5.86 0 006 8c0 7-3 9-3 9h17"/><path d="M18 8a6 6 0 00-9.33-5"/><path d="M1 1l22 22"/></svg>`,

  // Empty states + transcript role badges (kept compact; used inline)
  robot: `<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="8" width="18" height="12" rx="2"/><circle cx="9" cy="14" r="1.25"/><circle cx="15" cy="14" r="1.25"/><path d="M12 2v4M8 8V6a4 4 0 018 0v2"/></svg>`,
  fileDiff: `<svg ${s24}><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14,2 14,8 20,8"/><line x1="9" y1="15" x2="15" y2="15"/><line x1="12" y1="12" x2="12" y2="18"/></svg>`,
  clipboard: `<svg ${s24}><path d="M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2"/><rect x="8" y="2" width="8" height="4" rx="1"/></svg>`,
  human: `<svg ${s16}><circle cx="12" cy="8" r="4"/><path d="M6 21v-2a4 4 0 014-4h4a4 4 0 014 4v2"/></svg>`,
  assistant: `<svg ${s16}><rect x="3" y="8" width="18" height="12" rx="2"/><circle cx="9" cy="14" r="1"/><circle cx="15" cy="14" r="1"/><path d="M12 2v4"/></svg>`,
  tool: `<svg ${s16}><path d="M14.7 6.3a1 1 0 000 1.4l1.6 1.6a1 1 0 001.4 0l3.77-3.77a6 6 0 01-7.94 7.94l-6.91 6.91a2.12 2.12 0 01-3-3l6.91-6.91a6 6 0 017.94-7.94l-3.76 3.76z"/></svg>`,
  // Activity-event icons used by detail.js
  activity: `<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"><polyline points="22,12 18,12 15,21 9,3 6,12 2,12"/></svg>`,
  calendar: `<svg ${s16}><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>`,
  sigma: `<svg ${s16}><path d="M18 7V4H6l6 8-6 8h12v-3"/></svg>`,
  subagent: `<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="5" r="3"/><circle cx="5" cy="19" r="3"/><circle cx="19" cy="19" r="3"/><line x1="12" y1="8" x2="5" y2="16"/><line x1="12" y1="8" x2="19" y2="16"/></svg>`,
  chart: `<svg ${s16}><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>`,
  // Nav-row variant of chart (20px, matches search/pencil/install) — used
  // as the leading slot for the sidebar's "Usage" entry.
  chartBar: `<svg ${s20}><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>`,
};
