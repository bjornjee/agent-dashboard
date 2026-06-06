// Agent Dashboard — ES Module entry point
import { renderList } from './js/pages/list.js';
import { renderDetail, showModal, toast, updateActionBar, appendUserMessage, confirmUserMessageSent, refreshWorkingIndicator, refreshActiveTab, refreshDetailHeader, stopConversationPoll, updateQuestionCardSubmit, submitQuestionCard } from './js/pages/detail.js';
import { renderUsage } from './js/pages/usage.js';
import { renderCreate } from './js/pages/create.js';
import { get, post, cancelNav } from './js/api.js';
import { UI } from './js/ui.js';
import { ICONS } from './js/icons.js';
import { Theme } from './js/theme.js';
import { initNotify, processNotifications, toggleBrowserNotifications } from './js/notify.js';
import { renderSidebar, isDesktop, DESKTOP_MQ } from './js/sidebar.js';
import { promptInstall, maybeShowIOSHint, consumeNewAgentShortcut } from './js/install.js';
import { openSearch, closeSearch, isSearchOpen } from './js/pages/search.js';

// Configure marked.js if available
if (typeof marked !== 'undefined') {
  marked.setOptions({ breaks: true, gfm: true });
}

// --- Shared state ---
const app = document.getElementById('app');
let agents = [];
let selectedAgentId = null;
let currentView = 'list'; // 'list' | 'detail' | 'usage' | 'create'
let eventSource = null;

// Sheet focus management — `aria-modal="true"` implies focus lives inside
// the dialog while it is open. We move focus to the first row on mount,
// trap Tab within the sheet, and restore focus to the opener on dismiss.
let sheetOpener = null;
let sheetKeydownHandler = null;

function mountSheet(html) {
  document.querySelectorAll('.ui-sheet').forEach(el => el.remove());
  sheetOpener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const wrap = document.createElement('div');
  wrap.innerHTML = html;
  const sheet = wrap.firstElementChild;
  document.body.appendChild(sheet);
  const items = sheet.querySelectorAll('.ui-sheet__item');
  if (items.length > 0) items[0].focus();
  sheetKeydownHandler = (e) => {
    if (e.key !== 'Tab' || items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  };
  sheet.addEventListener('keydown', sheetKeydownHandler);
}

function restoreSheetFocus() {
  if (sheetOpener && typeof sheetOpener.focus === 'function') {
    try { sheetOpener.focus(); } catch {}
  }
  sheetOpener = null;
  sheetKeydownHandler = null;
}

function setView(view, agentId) {
  currentView = view;
  selectedAgentId = agentId || null;
  document.body.classList.toggle('view-detail', view === 'detail');
  // Dock + sheet are persistent body children, scoped to the list view only.
  if (view !== 'list') document.querySelectorAll('.ui-dock').forEach(el => el.remove());
  document.querySelectorAll('.ui-sheet').forEach(el => el.remove());
  try { sessionStorage.setItem('dashboard-view', JSON.stringify({ view, agentId: agentId || null })); } catch {}
}

// --- History navigation ---
function pushView(view, agentId) {
  const state = { view, agentId: agentId || null };
  history.pushState(state, '', null);
}

function navigateTo(view, agentId, push) {
  const desktop = isDesktop();

  switch (view) {
    case 'list':
      cancelNav();
      stopConversationPoll();
      if (desktop) {
        // On desktop the agent list lives in the sidebar — the main pane
        // has no standalone "list" page. Default the main pane to create
        // and tell the sidebar so "+ New agent" reads as selected.
        setView('create');
        renderCreate(app, agents);
      } else {
        setView('list');
        renderList(app, agents);
      }
      break;
    case 'detail':
      if (agentId) {
        renderDetail(app, agents, agentId, setView);
      } else {
        navigateTo('list', null, false);
        return;
      }
      break;
    case 'usage':
      stopConversationPoll();
      setView('usage');
      renderUsage(app, agents);
      break;
    case 'create':
      stopConversationPoll();
      setView('create');
      renderCreate(app, agents);
      break;
    default:
      navigateTo('list', null, false);
      return;
  }

  if (desktop) renderSidebar(agents, selectedAgentId, currentView);

  if (push) pushView(view, agentId);
}

window.addEventListener('popstate', (e) => {
  const state = e.state;
  if (state && state.view) {
    navigateTo(state.view, state.agentId, false);
  } else {
    navigateTo('list', null, false);
  }
});

// Wrap an async action with button spinner feedback.
async function withSpinner(evt, fn) {
  const btn = evt && evt.target ? evt.target.closest('button') : null;
  let origHtml;
  if (btn) { origHtml = btn.innerHTML; btn.disabled = true; btn.innerHTML += UI.spinner(); }
  try { await fn(); } finally { if (btn) { btn.innerHTML = origHtml; btn.disabled = false; } }
}

// --- SSE ---
function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/events');
  eventSource.onmessage = (e) => {
    try {
      agents = JSON.parse(e.data);
      const desktop = isDesktop();
      if (currentView === 'list' && !desktop) {
        // On mobile the list view IS the main pane.
        renderList(app, agents);
      } else if (currentView === 'detail' && selectedAgentId) {
        const agent = agents.find(a => a.session_id === selectedAgentId);
        if (agent) {
          updateActionBar(agent);
          refreshDetailHeader(agent);
          refreshWorkingIndicator(agent);
        }
        refreshActiveTab(selectedAgentId, agents.find(a => a.session_id === selectedAgentId));
      }
      // On desktop, refresh sidebar on every SSE tick — but never re-mount
      // the main pane (so a half-filled create form / scroll position is
      // preserved while agents update).
      if (desktop) renderSidebar(agents, selectedAgentId, currentView);
      try { processNotifications(agents); } catch (err) { console.error('[notify] error:', err); }
    } catch (err) { /* ignore parse errors */ }
  };
  eventSource.onerror = () => {
    // Auto-reconnects
  };
}

// --- Public API (used by onclick handlers in HTML strings) ---
window.Dashboard = {
  showList() {
    navigateTo('list', null, true);
  },

  showUsage() {
    navigateTo('usage', null, true);
  },

  showCreate() {
    navigateTo('create', null, true);
  },

  installApp() {
    promptInstall();
  },

  openKebab() {
    mountSheet(UI.sheet([
      { icon: ICONS.spark, label: 'Usage', navigating: false, onclick: 'Dashboard.dismissSheet();Dashboard.showUsage()' },
      { icon: ICONS.bell, label: 'Notifications', navigating: false, onclick: 'Dashboard.toggleNotifications();Dashboard.dismissSheet()' },
    ]));
  },

  openDetailKebab(agentId) {
    mountSheet(UI.sheet([
      { icon: ICONS.spark, label: 'Usage', navigating: false, onclick: 'Dashboard.dismissSheet();Dashboard.showUsage()' },
      { icon: ICONS.bell, label: 'Notifications', navigating: false, onclick: 'Dashboard.toggleNotifications();Dashboard.dismissSheet()' },
      { icon: ICONS.close, label: 'Terminate agent', navigating: false, variant: 'danger', onclick: `Dashboard.dismissSheet();Dashboard.confirmClose('${agentId}')` },
    ]));
  },

  dismissSheet() {
    document.querySelectorAll('.ui-sheet').forEach(el => el.remove());
    restoreSheetFocus();
  },

  openShortcuts() {
    const rows = [
      ['⌘', 'K', 'Search agents'],
      ['?', '', 'Keyboard shortcuts'],
      ['↵', '', 'Send (chat composer)'],
      ['⌘', '↵', 'Spawn agent (new-agent composer)'],
      ['Esc', '', 'Close overlay'],
    ];
    const list = rows.map(r => `<li class="ui-shortcuts__row"><span class="ui-shortcuts__keys"><kbd>${r[0]}</kbd>${r[1] ? `<kbd>${r[1]}</kbd>` : ''}</span><span class="ui-shortcuts__label">${r[2]}</span></li>`).join('');
    const wrap = document.createElement('div');
    wrap.className = 'ui-shortcuts';
    wrap.setAttribute('role', 'dialog');
    wrap.setAttribute('aria-modal', 'true');
    wrap.setAttribute('aria-label', 'Keyboard shortcuts');
    wrap.innerHTML = `<div class="ui-shortcuts__backdrop" onclick="Dashboard.dismissShortcuts()"></div><div class="ui-shortcuts__panel"><div class="ui-shortcuts__title">Keyboard shortcuts</div><ul class="ui-shortcuts__list">${list}</ul></div>`;
    document.body.appendChild(wrap);
  },

  dismissShortcuts() {
    document.querySelectorAll('.ui-shortcuts').forEach(el => el.remove());
  },

  searchAgents() {
    openSearch(agents);
  },

  selectAgent(id) {
    navigateTo('detail', id, true);
  },

  // Programmatic tab switch — used by the chat plan-link card so it
  // can jump to the Plan tab without simulating a click event handler.
  // Mirrors the inline behaviour of the .detail-tabs__tab click listener.
  openDetailTab(target) {
    const tab = document.querySelector('.detail-tabs__tab[data-tab="' + target + '"]');
    if (tab) tab.click();
  },

  async approve(id, evt) {
    await withSpinner(evt, async () => {
      const result = await post('/api/agents/' + id + '/approve');
      if (result && result.ok) toast('Approved', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  async reject(id, evt) {
    await withSpinner(evt, async () => {
      const result = await post('/api/agents/' + id + '/reject');
      if (result && result.ok) toast('Rejected', 'success');
      else toast('Failed: ' + (result?.error || 'unknown'), 'error');
    });
  },

  // Open a native macOS Choose File dialog via the local server
  // (POST /api/file-picker → osascript), then insert the chosen
  // absolute path at the textarea cursor. The dashboard binds to
  // localhost so the dialog can only be triggered from the user's
  // own browser.
  async attachFile() {
    const input = document.getElementById('reply-input');
    if (!input) return;
    let path = '';
    try {
      const result = await post('/api/file-picker');
      path = (result && result.path) || '';
    } catch (err) {
      toast('File picker failed: ' + err.message, 'error');
      return;
    }
    if (!path) return; // user cancelled
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? input.value.length;
    const sep = (start > 0 && input.value[start - 1] && !/\s/.test(input.value[start - 1])) ? ' ' : '';
    const insertion = sep + path + ' ';
    input.value = input.value.slice(0, start) + insertion + input.value.slice(end);
    input.focus();
    const cursor = start + insertion.length;
    try { input.setSelectionRange(cursor, cursor); } catch {}
    input.dispatchEvent(new Event('input', { bubbles: true }));
  },

  // Called inline from the card markup on any input change.
  questionCardUpdate(toolUseId) {
    updateQuestionCardSubmit(toolUseId);
  },

  // Submit the assembled AskUserQuestion answer to the agent. Reuses
  // the existing /input endpoint — the answer is just the user's next
  // message; the agent's tool_use waits on that message as its result.
  async answerQuestion(id, toolUseId, evt) {
    if (evt && evt.preventDefault) evt.preventDefault();
    await submitQuestionCard(id, toolUseId);
  },

  async sendInput(id) {
    const input = document.getElementById('reply-input');
    if (!input || !input.value.trim()) return;
    const text = input.value.trim();
    input.value = '';
    input.disabled = true;
    appendUserMessage(text);
    try {
      const result = await post('/api/agents/' + id + '/input', { text });
      if (result && result.ok) {
        confirmUserMessageSent();
      } else {
        toast('Failed: ' + (result?.error || 'unknown'), 'error');
      }
    } finally {
      if (input) input.disabled = false;
    }
  },

  confirmStop(id) {
    showModal('Stop Agent', 'Send Ctrl+C to this agent?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/stop');
        if (result && result.ok) toast('Stopped', 'success');
        else toast('Failed: ' + (result?.error || 'unknown'), 'error');
      });
    });
  },

  confirmMerge(id) {
    // Capture branch before async merge — SSE may update agents mid-flight
    const agentPre = agents.find(a => a.session_id === id);
    const branch = agentPre ? agentPre.branch : '';
    showModal('Merge PR', 'Merge this PR with --squash?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/merge');
        if (result && result.ok) {
          toast('Merged', 'success');
          const label = branch ? `Clean up ${branch}?` : 'Clean up worktree and branch?';
          showModal('Post-Merge Cleanup', label + ' This will remove the worktree, checkout the default branch, pull, and delete the local feature branch.', async (cleanEvt) => {
            await withSpinner(cleanEvt, async () => {
              const cleanResult = await post('/api/agents/' + id + '/cleanup');
              if (cleanResult && cleanResult.ok) {
                toast('Cleaned up', 'success');
                navigateTo('list', null, true);
              } else {
                toast('Cleanup failed: ' + (cleanResult?.error || 'unknown'), 'error');
              }
            });
          });
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    });
  },

  confirmClose(id) {
    showModal('Close Agent', 'Kill the tmux pane and remove this agent?', async (evt) => {
      await withSpinner(evt, async () => {
        const result = await post('/api/agents/' + id + '/close');
        if (result && result.ok) {
          toast('Closed', 'success');
          navigateTo('list', null, true);
        } else {
          toast('Failed: ' + (result?.error || 'unknown'), 'error');
        }
      });
    });
  },

  cycleTheme() { Theme.cycle(); },

  async toggleNotifications() {
    const result = await toggleBrowserNotifications();
    if (result.permission === 'unsupported') {
      toast('Notifications not supported in this browser', 'error');
    } else if (result.permission === 'denied') {
      toast('Notifications blocked — check browser settings', 'error');
    } else if (result.enabled) {
      toast('Notifications enabled', 'success');
    } else {
      toast('Notifications disabled', 'success');
    }
    // Re-render current view to update bell icon
    if (currentView === 'list') renderList(app, agents);
    else if (currentView === 'detail' && selectedAgentId) renderDetail(app, agents, selectedAgentId, setView);
    else if (currentView === 'usage') renderUsage(app, agents);
    else if (currentView === 'create') renderCreate(app, agents);
  },

  async openPR(id) {
    const agent = agents.find(a => a.session_id === id);
    if (agent && agent.pr_url) {
      window.open(agent.pr_url, '_blank');
      return;
    }
    try {
      const resp = await fetch(`/api/agents/${encodeURIComponent(id)}/pr-url`);
      if (!resp.ok) throw new Error('failed to resolve PR URL');
      const data = await resp.json();
      if (data.url) {
        window.open(data.url, '_blank');
        return;
      }
    } catch {}
    toast('No PR URL available', 'error');
  },

  async createAgent(evt) {
    const folder = document.getElementById('create-folder')?.value?.trim();
    const skill = document.getElementById('create-skill')?.value?.trim();
    const message = document.getElementById('create-message')?.value?.trim();
    const harness = document.getElementById('create-harness')?.value?.trim();

    if (!folder) {
      toast('Folder is required', 'error');
      return;
    }

    await withSpinner(evt, async () => {
      const result = await post('/api/agents/create', { folder, skill, message, harness });
      if (result && result.ok) {
        toast('Agent created', 'success');
        navigateTo('list', null, true);
      } else {
        toast('Failed: ' + (result?.error || 'unknown'), 'error');
      }
    });
  },

  toggleExpand(btn) {
    const span = btn.previousElementSibling;
    if (!span) return;
    const full = span.getAttribute('data-full');
    const isTruncated = span.getAttribute('data-truncated') === 'true';
    if (isTruncated) {
      span.textContent = full;
      span.setAttribute('data-truncated', 'false');
      btn.textContent = 'Show less';
    } else {
      span.textContent = full.substring(0, 200) + '...';
      span.setAttribute('data-truncated', 'true');
      btn.textContent = 'Show more';
    }
  },
};

// --- Viewport breakpoint changes ---
// When the user crosses the desktop breakpoint, re-mount the current view
// so the right content lands in the right slot (mobile: #app; desktop:
// sidebar + #app).
const desktopMql = window.matchMedia(DESKTOP_MQ);
const onBreakpointChange = () => {
  if (!desktopMql.matches) {
    // Leaving desktop — clear the sidebar so it can re-hide.
    const host = document.getElementById('app-sidebar');
    if (host) { host.innerHTML = ''; host.hidden = true; }
  }
  navigateTo(currentView, selectedAgentId, false);
};
if (desktopMql.addEventListener) desktopMql.addEventListener('change', onBreakpointChange);
else if (desktopMql.addListener) desktopMql.addListener(onBreakpointChange);

// --- Service worker messages ---
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data && e.data.type === 'navigate-agent' && e.data.agentId) {
      navigateTo('detail', e.data.agentId, true);
    }
  });
}

// --- Theme ---
Theme.init();

// Cmd/Ctrl-K toggles the fuzzy search overlay from anywhere in the app.
document.addEventListener('keydown', (e) => {
  if (!(e.metaKey || e.ctrlKey) || e.key !== 'k' || e.repeat) return;
  e.preventDefault();
  if (isSearchOpen()) closeSearch();
  else openSearch(agents);
});

// "?" opens the keyboard cheatsheet from anywhere when not typing.
document.addEventListener('keydown', (e) => {
  if (e.key !== '?' || e.repeat) return;
  const t = e.target;
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
  e.preventDefault();
  if (document.querySelector('.ui-shortcuts')) Dashboard.dismissShortcuts();
  else Dashboard.openShortcuts();
});

// Esc closes the cheatsheet or any open action sheet. Sheet dismissal
// also restores focus to the element that opened it (see mountSheet).
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  if (document.querySelector('.ui-shortcuts')) { Dashboard.dismissShortcuts(); return; }
  if (document.querySelector('.ui-sheet')) Dashboard.dismissSheet();
});

// --- Init ---
async function init() {
  const data = await get('/api/agents');
  if (data) {
    agents = data;
    initNotify(agents);
  }

  // Manifest shortcut deep-link (?action=new-agent) overrides restore.
  const shortcutHandled = consumeNewAgentShortcut(navigateTo);

  let restored = shortcutHandled;
  if (!restored) {
    try {
      const saved = JSON.parse(sessionStorage.getItem('dashboard-view'));
      if (saved && saved.view) {
        if (saved.view === 'detail' && saved.agentId && agents.find(a => a.session_id === saved.agentId)) {
          navigateTo('detail', saved.agentId, false);
          restored = true;
        } else if (saved.view === 'usage') {
          navigateTo('usage', null, false);
          restored = true;
        } else if (saved.view === 'create') {
          navigateTo('create', null, false);
          restored = true;
        }
      }
    } catch {}
  }

  if (!restored) navigateTo('list', null, false);

  // Set initial history entry so popstate has something to land on
  history.replaceState({ view: currentView, agentId: selectedAgentId }, '', null);

  connectSSE();
  maybeShowIOSHint(showModal);
}

init();
