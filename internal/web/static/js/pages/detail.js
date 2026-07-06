// Agent detail view with tabs and stats disclosure.
import { UI, stripLocalCommandTags } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState, stateGroup, prTag, hasOpenPR, planBadge, questionBadge } from '../state.js';
import { escapeHtml, repoName, duration, formatTime, formatTimeShort, formatCost, formatTokens, renderMarkdown, skeletonLoading } from '../format.js';
import { get, post, cancelNav, newNavSignal } from '../api.js';
import { showModal, toast } from '../modal.js';
import { Theme } from '../theme.js';
import { isDesktop } from '../sidebar.js';
import { attachSlashAutocomplete } from '../slash-autocomplete.js';

export { showModal, toast, stopConversationPoll };

// --- Local primitive helpers (Codex-iOS register; not promoted to ui.js — single caller) ---

function inlineBtn(label, variant, onclick, id) {
  const v = variant === 'primary' ? 'primary'
    : variant === 'danger' ? 'danger'
    : variant === 'ghost' ? 'ghost'
    : 'secondary';
  const idAttr = id ? ` id="${id}"` : '';
  return `<button class="ui-modal-btn ui-modal-btn--${v}" onclick="${onclick}"${idAttr}>${escapeHtml(label)}</button>`;
}

const STATE_LABELS = {
  running: 'Working',
  permission: 'Needs approval',
  plan: 'Plan ready',
  question: 'Needs reply',
  error: 'Errored',
  pr: 'PR',
  merged: 'Merged',
  done: 'Done',
  idle_prompt: 'Idle',
  blocked: 'Blocked',
  waiting: 'Waiting',
  queued: 'Queued',
  review: 'Review',
  failed: 'Failed',
  completed: 'Completed',
  waiting_input: 'Waiting',
};

function inlineStatusPill(state) {
  const group = stateGroup(state).toLowerCase();
  // Unknown states are humanized (underscores → spaces), never rendered
  // raw — a "Waiting_input" pill reads as debug output.
  const label = STATE_LABELS[state]
    || (state ? (state.charAt(0).toUpperCase() + state.slice(1)).replace(/_/g, ' ') : 'Unknown');
  return `<span class="ui-status-pill ui-status-pill--${group}"><span class="status-dot status-dot--${group}"></span>${escapeHtml(label)}</span>`;
}

function inlineEmptyState(icon, title, subtitle) {
  return `<div class="empty-state"><div class="empty-state-icon">${icon}</div><div class="empty-state-title">${escapeHtml(title)}</div><div class="empty-state-subtitle">${escapeHtml(subtitle)}</div></div>`;
}

function inlineLoading() {
  return '<div class="loading"><span class="spinner"></span></div>';
}

function inlineDisclosure(id, label, open) {
  const openAttr = open ? ' open' : '';
  return `<details class="detail-disclosure" id="${id}-section"${openAttr}>
    <summary data-section="${id}">${escapeHtml(label)}</summary>
    <div class="detail-disclosure-body" id="${id}"></div>
  </details>`;
}

function inlineSegmentedTabs(items, active) {
  let html = '<div class="detail-tabs">';
  for (const it of items) {
    const cls = it.key === active ? ' detail-tabs__tab--active' : '';
    html += `<button class="detail-tabs__tab${cls}" data-tab="${it.key}">${escapeHtml(it.label)}</button>`;
  }
  html += '</div>';
  return html;
}

function inlineFileStatus(status) {
  switch (status) {
    case 'added': return '<span class="file-status file-status--added">+</span>';
    case 'deleted': return '<span class="file-status file-status--deleted">&minus;</span>';
    case 'renamed': return '<span class="file-status file-status--renamed">&rarr;</span>';
    default: return '<span class="file-status file-status--modified"></span>';
  }
}

function inlineToggleSwitch(label, key, defaultOn) {
  const checked = defaultOn ? ' checked' : '';
  return `<label class="toggle-switch">
    <span class="toggle-switch__label">${escapeHtml(label)}</span>
    <input type="checkbox" class="toggle-switch__input" data-key="${escapeHtml(key)}"${checked}>
    <span class="toggle-switch__track"></span>
  </label>`;
}

function inlineVitalStrip(opts) {
  const elapsed = escapeHtml(opts.elapsed || '');
  const tokens = formatTokens(opts.tokens || 0);
  const cost = formatCost(opts.cost || 0) || '&mdash;';
  return `<div class="vital-strip">
    <div class="vital-cell"><span class="vital-label">Elapsed</span><span class="vital-value">${elapsed}</span></div>
    <div class="vital-cell"><span class="vital-label">Tokens</span><span class="vital-value">${tokens}</span></div>
    <div class="vital-cell"><span class="vital-label">Cost</span><span class="vital-value">${cost}</span></div>
  </div>`;
}

// Signature of every agent field that renderActionBar() reads. If this
// string is unchanged since the last call, the rebuilt HTML would be
// byte-identical — so we MUST skip the DOM swap. Re-rendering anyway
// detaches the focused <textarea>, which on mobile (iOS Safari, Chrome
// Android) dismisses the virtual keyboard mid-typing.
export function actionBarSignature(agent) {
  return [
    effectiveState(agent),
    hasOpenPR(agent) ? '1' : '0',
    agent.model || '',
    agent.branch || '',
    agent.effort || '',
    // Must include the plan signal. SSE re-render is gated on the signature;
    // without this, flipping permission_mode (EnterPlanMode mid-turn) never
    // surfaces the PLAN chip in the action bar.
    planBadge(agent) ? '1' : '0',
  ].join('|');
}

// Update the action bar in-place when agent state changes via SSE.
export function updateActionBar(agent) {
  const bar = document.querySelector('.action-bar');
  if (!bar) return;

  // SSE fires on every agent-state delta (cost, tokens, hook events,
  // current_tool — all changing many times per second while an agent
  // runs). The action bar only depends on a small subset of fields;
  // bail out when none of them changed so the focused textarea is
  // never detached. This is what keeps the mobile keyboard open.
  const sig = actionBarSignature(agent);
  if (bar.dataset.sig === sig) {
    // Bar HTML is unchanged, but the composer gate rides on
    // agent.pending_question which is not part of the signature.
    applyComposerGate(!!questionBadge(agent));
    return;
  }

  // Capture in-flight composer state so the SSE-driven re-render doesn't
  // wipe what the user is typing.
  const oldInput = bar.querySelector('#reply-input');
  const wasFocused = oldInput && document.activeElement === oldInput;
  const oldValue = oldInput ? oldInput.value : '';
  const selStart = oldInput ? oldInput.selectionStart : 0;
  const selEnd = oldInput ? oldInput.selectionEnd : 0;
  const oldHeight = oldInput ? oldInput.style.height : '';

  const tmp = document.createElement('div');
  tmp.innerHTML = renderActionBar(agent);
  const newBar = tmp.firstElementChild;
  if (!newBar) return;
  newBar.dataset.sig = sig;
  bar.replaceWith(newBar);

  const newInput = newBar.querySelector('#reply-input');
  if (newInput && oldValue) {
    newInput.value = oldValue;
    newInput.dispatchEvent(new Event('input', { bubbles: true }));
    if (oldHeight) newInput.style.height = oldHeight;
    if (wasFocused) {
      newInput.focus();
      try { newInput.setSelectionRange(selStart, selEnd); } catch {}
    }
  }
  if (newInput) attachSlashAutocomplete(newInput, agent.harness);
  applyComposerGate(!!questionBadge(agent));
}

// Track optimistic messages so refreshConversation can preserve them
// across the 2s poll until the API echoes the user's message back.
//
// pendingUserMessage  — the text of the in-flight user message
// pendingMessageAcked — false until POST /input resolves OK; once true
//                       the conversation refresh stops re-rendering the
//                       "Sending…" caption (the message is delivered;
//                       only the API echo is still pending).
// preSendAgentState   — lastKnownAgent.state captured before
//                       appendUserMessage forces 'running', so a failed
//                       POST can restore reality (see
//                       cancelPendingUserMessage).
let pendingUserMessage = null;
let pendingMessageAcked = false;
let preSendAgentState = null;

// Auto-follow threshold. If the user has scrolled more than this many
// pixels above the bottom, treat them as "reading older messages" and
// preserve their position across poll-tick re-renders instead of
// snapping back. 60px keeps the follow behaviour sticky enough that a
// half-line of overscroll still counts as "at bottom".
const SCROLL_BOTTOM_THRESHOLD_PX = 60;
function isAtBottom(scrollParent) {
  if (!scrollParent) return false;
  return scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight < SCROLL_BOTTOM_THRESHOLD_PX;
}

// Whether the conversation tab has been auto-scrolled to bottom for the
// currently-open detail session. Reset by renderDetail() each time a new
// detail view mounts; consulted by loadTabContent('conversation', ...)
// so tab-switches back to Conversation preserve the user's scroll
// position instead of re-snapping.
let conversationScrolledThisSession = false;

// Optimistically append a Codex-style user message pill to the chat AND
// mount the pulsing-orb working indicator in the same frame, so the
// visitor never sees their bubble sit alone while the POST round-trips.
// While in flight (pre-POST-ack) the bubble carries .ui-msg--optimistic
// and is followed by a "Sending…" caption sibling. Dashboard.sendInput
// clears the flag (and removes the caption) once the POST resolves OK.
//
// The bubble is stamped with data-optimistic="1" so refreshConversation
// can find it (after confirmUserMessageSent strips the visual class) and
// remove it once the API echoes the message back as a real entry.
//
// The orb mount is synthetic: we override lastKnownAgent.state='running'
// before calling refreshWorkingIndicator so isAgentMidTurn returns true
// even if the agent's last known state was idle_prompt/done. SSE updates
// will overwrite lastKnownAgent; the orb clears the moment agent.state
// leaves WORKING_STATES. Persisting the override onto lastKnownAgent
// (rather than just passing it inline) keeps the 2s conversation poll
// — which calls refreshWorkingIndicator(lastKnownAgent) — from wiping
// the orb before SSE has reported the real running state.
export function appendUserMessage(text) {
  pendingUserMessage = text;
  pendingMessageAcked = false;
  // Stash the real state so cancelPendingUserMessage can undo the
  // synthetic 'running' override below if the POST fails.
  preSendAgentState = lastKnownAgent ? lastKnownAgent.state : null;
  const tab = document.getElementById('tab-conversation');
  let container = tab ? tab.querySelector('.conversation') : null;
  if (!container && tab) {
    tab.innerHTML = '<div class="conversation"></div>';
    container = tab.querySelector('.conversation');
  }
  if (!container) return;
  const wrap = document.createElement('div');
  wrap.innerHTML = UI.message('user', text);
  const msgEl = wrap.firstElementChild;
  if (msgEl) {
    msgEl.dataset.optimistic = '1';
    msgEl.classList.add('ui-msg--optimistic');
    container.appendChild(msgEl);
    const caption = document.createElement('div');
    caption.className = 'ui-msg__caption ui-msg__caption--sending';
    caption.textContent = 'Sending…';
    container.appendChild(caption);
  }
  // Mount the working indicator in the same frame as the bubble.
  // Reset the per-turn tally, the seen-tool watermark, AND the latest-
  // activity entry from the prior turn so the pre-tool orb branch in
  // refreshWorkingIndicator fires (it gates on all three being empty).
  // A new turn just started; the prior turn's "Last: …" line would read
  // stale beside the visitor's fresh message.
  toolBuckets = {};
  lastSeenToolTimestamp = new Date().toISOString();
  latestToolEntry = null;
  if (lastKnownAgent) {
    lastKnownAgent = {
      ...lastKnownAgent,
      state: 'running',
      current_tool: '',
    };
    refreshWorkingIndicator(lastKnownAgent);
  }
  const scrollParent = container.closest('.detail-scroll');
  if (scrollParent) scrollParent.scrollTop = scrollParent.scrollHeight;
}

// Called by Dashboard.sendInput when POST resolves OK. Lifts the
// in-flight affordance from the optimistic bubble; the orb mount + tally
// reset already happened synchronously in appendUserMessage so the
// visitor saw immediate feedback.
export function confirmUserMessageSent() {
  pendingMessageAcked = true;
  preSendAgentState = null;
  const container = document.querySelector('#tab-conversation .conversation');
  if (!container) return;
  container.querySelectorAll('.ui-msg--optimistic').forEach(el => el.classList.remove('ui-msg--optimistic'));
  container.querySelectorAll('.ui-msg__caption--sending').forEach(el => el.remove());
}

// Failure path for Dashboard.sendInput: unwind everything
// appendUserMessage staged optimistically — the bubble, the "Sending…"
// caption, and the forced working indicator — so the chat stops claiming
// a message is in flight. Restores the pre-send agent state, but only
// while lastKnownAgent still carries the synthetic 'running' override;
// if SSE delivered a real update in the meantime, that truth wins.
export function cancelPendingUserMessage() {
  pendingUserMessage = null;
  pendingMessageAcked = false;
  toolBuckets = {};
  latestToolEntry = null;
  const container = document.querySelector('#tab-conversation .conversation');
  if (container) {
    container.querySelectorAll('[data-optimistic="1"]').forEach(el => el.remove());
    container.querySelectorAll('.ui-msg__caption--sending').forEach(el => el.remove());
    const working = container.querySelector('.ui-msg-status--working');
    if (working) working.remove();
  }
  if (lastKnownAgent && lastKnownAgent.state === 'running' && preSendAgentState !== null) {
    lastKnownAgent = { ...lastKnownAgent, state: preSendAgentState };
    refreshWorkingIndicator(lastKnownAgent);
  }
  preSendAgentState = null;
}

// Last-known agent for the currently-mounted detail view. Used by the
// 2s conversation poll (which doesn't carry an agent reference) so that
// the rebuilt .conversation can re-mount the working indicator.
let lastKnownAgent = null;

// --- Composer gate (pending AskUserQuestion) ---
// While a question card is pending, the composer is a lying affordance:
// typed text POSTs to /input but Claude Code's native picker swallows it
// (see submitQuestionCard). One gate, one input — disable the composer
// and point at the card until the question resolves.
const GATED_PLACEHOLDER = 'Answer the question card above';

function applyComposerGate(gated) {
  const input = document.getElementById('reply-input');
  if (!input) return;
  const composer = input.closest('.ui-composer');
  const send = composer ? composer.querySelector('.ui-composer__send') : null;
  if (gated) {
    if (input.dataset.qcGated !== '1') {
      input.dataset.qcGated = '1';
      input.dataset.qcPlaceholder = input.placeholder || '';
      input.placeholder = GATED_PLACEHOLDER;
      input.disabled = true;
    }
    if (send) send.disabled = true;
  } else if (input.dataset.qcGated === '1') {
    delete input.dataset.qcGated;
    input.placeholder = input.dataset.qcPlaceholder || '';
    delete input.dataset.qcPlaceholder;
    input.disabled = false;
    if (send) send.disabled = false;
  }
}

// --- Jump-to-latest chip ---
// Floating affordance shown when new conversation activity (entries or
// a question card) lands while the reader is scrolled up in history.
// Clicking jumps to the question card when one is pending (it is the
// gate), else to the bottom. Removed on click, on reaching the bottom,
// and on tab/view changes.
function showJumpChip(hasQuestion) {
  let chip = document.querySelector('.jump-latest');
  if (!chip) {
    chip = document.createElement('button');
    chip.className = 'jump-latest';
    chip.setAttribute('aria-label', 'Jump to latest activity');
    const host = document.querySelector('.detail-layout') || document.body;
    host.appendChild(chip);
    chip.addEventListener('click', () => {
      const sp = document.querySelector('.detail-scroll');
      if (sp) {
        const card = sp.querySelector('.question-card');
        if (card) {
          const parentRect = sp.getBoundingClientRect();
          const cardRect = card.getBoundingClientRect();
          sp.scrollTop += (cardRect.top - parentRect.top) - 12;
        } else {
          sp.scrollTop = sp.scrollHeight;
        }
      }
      hideJumpChip();
    });
  }
  chip.textContent = hasQuestion ? '↓ 1 question' : '↓ New activity';
}

function hideJumpChip() {
  document.querySelectorAll('.jump-latest').forEach(el => el.remove());
}

const WORKING_STATES = new Set(['running', 'permission', 'plan', 'question', 'error']);

// Map raw tool names to user-legible action verbs. Anything not in this
// table falls into the "ran tool" bucket; the bucket is what surfaces
// in the rolled-up tally so internal names like "TaskUpdate" never
// reach the chat copy.
//   buckets: 'file_read' | 'file_edit' | 'search' | 'command' | 'task' |
//            'browser' | 'thinking' | 'other'
function classifyTool(name) {
  if (!name) return null;
  const n = String(name);
  if (n === 'Read' || n === 'NotebookRead') return { bucket: 'file_read', live: 'Reading file' };
  if (n === 'Edit' || n === 'Write' || n === 'MultiEdit' || n === 'NotebookEdit') return { bucket: 'file_edit', live: 'Editing file' };
  if (n === 'Grep' || n === 'Glob') return { bucket: 'search', live: 'Searching' };
  if (n === 'Bash' || n === 'BashOutput' || n === 'KillShell') return { bucket: 'command', live: 'Running command' };
  if (n.startsWith('Task') || n === 'TodoWrite') return { bucket: 'task', live: 'Updating tasks' };
  if (n.startsWith('mcp__plugin_playwright') || n.startsWith('mcp__playwright')) return { bucket: 'browser', live: 'Driving browser' };
  if (n.startsWith('mcp__')) return { bucket: 'mcp', live: 'Calling MCP tool' };
  if (n === 'WebFetch' || n === 'WebSearch') return { bucket: 'web', live: 'Fetching web content' };
  // Unknown tools get a generic verb — raw internal names like
  // "TaskUpdate" or "AskUserQuestion" must never reach the chat copy.
  return { bucket: 'other', live: 'Working' };
}

const BUCKET_LABELS = {
  file_read: ['Read', 'file', 'files'],
  file_edit: ['Edited', 'file', 'files'],
  search:    ['Ran', 'search', 'searches'],
  command:   ['Ran', 'command', 'commands'],
  task:      ['Updated', 'task', 'tasks'],
  browser:   ['Drove browser', 'step', 'steps'],
  mcp:       ['Called', 'MCP tool', 'MCP tools'],
  web:       ['Fetched', 'page', 'pages'],
  other:     ['Ran', 'tool', 'tools'],
};

// Running tally of tools fired during this working session, bucketed
// by category. Cleared when the agent leaves a working state.
let toolBuckets = {};
// Most-recently-completed tool entry (for the inline "Last: …" line).
// Holds { content, bucket } where content is the raw activity payload
// like "→ Bash: ls -la".
let latestToolEntry = null;
let toolStreamPollTimer = null;
let lastSeenToolTimestamp = null;

// Extract the tool name from an activity entry like "→ Bash: …" or
// "→ mcp__playwright__browser_take_screenshot: …".
function parseToolName(content) {
  const m = String(content || '').match(/^→\s+([^:\s]+)/);
  return m ? m[1] : '';
}

// Pure helper: turn a latest-tool entry into the short display string for
// the "ui-msg-status__latest" line. Two failure modes we explicitly guard
// against:
//   1. Playwright/MCP browser calls dump their raw JS payload (often an
//      arrow-function body) into `arg`. We drop it and surface the bare
//      method name instead (e.g. `browser_click`).
//   2. Bash calls that wrap JS / heredocs / multi-line scripts blow past
//      the truncation budget and look like noise. We replace with the
//      literal `<inline code>` label.
// Exported so node tests can exercise it without a DOM.
export function formatLatestToolDisplay(entry) {
  if (!entry || !entry.content) return '';
  const raw = String(entry.content).replace(/^→\s*/, '');
  const m = raw.match(/^([^:]+):\s*([\s\S]*)$/);
  const tool = m ? m[1].trim() : raw;
  const arg = m ? m[2].trim() : '';
  const c = classifyTool(tool);
  const friendly = c ? c.live : tool;
  let argSnip;
  if (c && c.bucket === 'browser') {
    // Drop the JS payload — surface the bare method name (e.g.
    // `mcp__plugin_playwright__browser_click` → `browser_click`).
    const parts = tool.split('__');
    argSnip = parts.length > 1 ? parts[parts.length - 1] : tool;
  } else if (
    c && c.bucket === 'command' &&
    (arg.indexOf('\n') !== -1 || /^\(\s*\)\s*=>/.test(arg) || /^function\b/.test(arg))
  ) {
    argSnip = '<inline code>';
  } else {
    argSnip = arg.length > 64 ? arg.slice(0, 62) + '…' : arg;
  }
  return argSnip ? friendly + ' · ' + argSnip : friendly;
}

// Render the tally as "Read 3 files · ran 2 commands · edited 1 file".
function renderToolTally(buckets) {
  const parts = [];
  for (const [bucket, count] of Object.entries(buckets)) {
    if (!count) continue;
    const [verb, singular, plural] = BUCKET_LABELS[bucket] || BUCKET_LABELS.other;
    const noun = count === 1 ? singular : plural;
    parts.push(verb.toLowerCase() + ' ' + count + ' ' + noun);
  }
  if (!parts.length) return '';
  // Capitalise the first chunk.
  parts[0] = parts[0].charAt(0).toUpperCase() + parts[0].slice(1);
  return parts.join(' · ');
}

// Extract the user-visible prose of an assistant message for the
// clipboard. `btn` is the .ui-msg__copy element clicked. Returns '' if
// the surrounding .ui-msg__card / .ui-msg__prose can't be located.
export function getMessageCopyText(btn) {
  if (!btn) return '';
  const card = btn.closest('.ui-msg__card');
  if (!card) return '';
  const prose = card.querySelector('.ui-msg__prose');
  if (!prose) return '';
  return String(prose.innerText || '').trim();
}

// Wire the assistant-message copy button via a single document-level
// delegated click — conversation rerenders replace .ui-msg children, so
// per-mount listeners would have to be re-registered each refresh.
if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
  document.addEventListener('click', (e) => {
    const btn = e.target && e.target.closest && e.target.closest('.ui-msg__copy');
    if (!btn) return;
    e.preventDefault();
    const text = getMessageCopyText(btn);
    if (!text) return;
    const showCopied = () => {
      btn.classList.add('ui-msg__copy--copied');
      btn.innerHTML = ICONS.check;
      setTimeout(() => {
        btn.classList.remove('ui-msg__copy--copied');
        btn.innerHTML = ICONS.copy;
      }, 1200);
    };
    const onFail = () => { try { toast('Copy failed'); } catch {} };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(showCopied, onFail);
    } else {
      onFail();
    }
  });
}

// Detect whether the agent is BETWEEN turns. Deterministic on agent.state:
// the backend transitions out of WORKING_STATES (running/permission/plan/
// question/error) the moment the turn ends, and SSE pushes that state
// change. Relying on last_hook_event === 'Stop' was racy — a dropped or
// late Stop event left the indicator stuck on a non-working state.
export function isAgentMidTurn(agent) {
  if (!agent) return false;
  return WORKING_STATES.has(effectiveState(agent));
}

// Subset of WORKING_STATES where the agent is waiting on the HUMAN, not
// computing. Mid-turn (the poll keeps running, the indicator stays
// mounted) but the visual treatment must invert: static notice, no
// shimmer, no orb — motion here would say "busy, don't act" at exactly
// the moment action is needed.
const BLOCKED_STATES = new Set(['permission', 'plan', 'question', 'error']);

export function isAgentBlockedOnUser(agent) {
  if (!agent) return false;
  return BLOCKED_STATES.has(effectiveState(agent));
}

// Static "the agent needs you" line for blocked-on-human states.
// Exported for unit tests.
const BLOCKED_COPY = {
  permission: 'Waiting for your approval',
  plan: 'Plan ready for your review',
  question: 'Waiting for your reply',
  error: 'Stopped on an error',
};

export function blockedNoticeHtml(state) {
  const label = BLOCKED_COPY[state];
  if (!label) return '';
  return '<div class="ui-msg-status__blocked">'
    + '<span class="ui-msg-status__blocked-dot" aria-hidden="true"></span>'
    + escapeHtml(label)
    + '</div>';
}

// Shared factory for the inline status element so every variant (orb,
// tally+shimmer, blocked notice) carries the same live-region wiring —
// screen readers hear turn progress without a dedicated announcement.
function createStatusEl() {
  const el = document.createElement('div');
  el.className = 'ui-msg-status ui-msg-status--working';
  el.setAttribute('role', 'status');
  el.setAttribute('aria-live', 'polite');
  return el;
}

// Mounts / updates / removes the inline "working" block at the end of
// the conversation stream. Two stacked lines:
//   1. (optional) muted tally — "Read 3 files · ran 2 commands"
//   2. live pulsing line     — "Reading file…" / "Thinking…"
// The tally is derived from the activity-stream poll; the live line is
// derived from agent.current_tool via the classifyTool() bucket table.
export function refreshWorkingIndicator(agent) {
  if (agent) lastKnownAgent = agent;
  const container = document.querySelector('#tab-conversation .conversation');
  if (!container) return;
  const existing = container.querySelector('.ui-msg-status--working');
  if (!isAgentMidTurn(agent)) {
    if (existing) existing.remove();
    // Turn ended — reset the tally so the next turn starts clean.
    toolBuckets = {};
    return;
  }
  // Blocked-on-human: static notice instead of shimmer/orb. Keep the
  // tally line — it summarizes what the turn did before it stopped.
  if (isAgentBlockedOnUser(agent)) {
    const blockedTally = renderToolTally(toolBuckets);
    const blockedHtml =
      (blockedTally ? '<div class="ui-msg-status__tally">' + escapeHtml(blockedTally) + '</div>' : '')
      + blockedNoticeHtml(effectiveState(agent));
    if (existing) {
      existing.classList.add('ui-msg-status--blocked');
      if (existing.innerHTML !== blockedHtml) existing.innerHTML = blockedHtml;
    } else {
      const scrollParent = container.closest('.detail-scroll');
      const wasAtBottom = isAtBottom(scrollParent);
      const el = createStatusEl();
      el.classList.add('ui-msg-status--blocked');
      el.innerHTML = blockedHtml;
      container.appendChild(el);
      const hasCard = !!container.querySelector('.question-card');
      if (scrollParent && wasAtBottom && !hasCard) scrollParent.scrollTop = scrollParent.scrollHeight;
    }
    return;
  }
  if (existing) existing.classList.remove('ui-msg-status--blocked');
  const classified = classifyTool(agent.current_tool);
  const liveLabel = classified ? classified.live : 'Thinking';
  const tally = renderToolTally(toolBuckets);
  const tallyHtml = tally
    ? '<div class="ui-msg-status__tally">' + escapeHtml(tally) + '</div>'
    : '';
  // Pre-tool state: agent has acknowledged the prompt but no tool has
  // fired yet (no tally, no latest entry, no current_tool). Show a
  // pulsing orb as the placeholder — Codex-mobile pattern. Once the
  // first PreToolUse hook arrives, this branch falls through to the
  // regular tally + shimmer render below.
  if (!tally && !latestToolEntry && !classified) {
    const orbHtml = '<div class="ui-msg-status__orb-wrap"><span class="ui-msg-status__orb" aria-hidden="true"></span></div>';
    if (existing) {
      if (existing.innerHTML !== orbHtml) existing.innerHTML = orbHtml;
    } else {
      const scrollParent = container.closest('.detail-scroll');
      const wasAtBottom = isAtBottom(scrollParent);
      const el = createStatusEl();
      el.innerHTML = orbHtml;
      container.appendChild(el);
      // When a question card is pending, the visitor's focus belongs at
      // the card (the workflow gate), not the running-status caption
      // below it. The initial conv-load scroll already positioned the
      // card top; do not overwrite that here.
      const hasCard = !!container.querySelector('.question-card');
      if (scrollParent && wasAtBottom && !hasCard) scrollParent.scrollTop = scrollParent.scrollHeight;
    }
    if (!toolStreamPollTimer && agent.session_id) {
      startToolStreamPoll(agent.session_id);
    }
    return;
  }
  // Latest activity line — shows what the agent most recently *finished*.
  // Display rendering (incl. bucket-aware sanitisation for browser MCP
  // calls and inline-code bash payloads) lives in formatLatestToolDisplay
  // so it can be unit-tested without a DOM.
  let latestHtml = '';
  if (latestToolEntry) {
    const display = formatLatestToolDisplay(latestToolEntry);
    if (display) {
      latestHtml = '<div class="ui-msg-status__latest">' + escapeHtml(display) + '</div>';
    }
  }
  const html =
    tallyHtml +
    latestHtml +
    '<div class="ui-msg-status__live">' +
      '<span class="ui-msg-status__label">' + escapeHtml(liveLabel) + '</span>' +
    '</div>';
  if (existing) {
    if (existing.innerHTML !== html) existing.innerHTML = html;
  } else {
    // Capture follow-state BEFORE appending — the mount itself extends
    // scrollHeight, which would otherwise flip wasAtBottom to false.
    const scrollParent = container.closest('.detail-scroll');
    const wasAtBottom = isAtBottom(scrollParent);
    const el = createStatusEl();
    el.innerHTML = html;
    container.appendChild(el);
    // Only pull the indicator into view if the user was already at the
    // bottom. An unconditional scroll here would override whatever
    // scroll position the user just chose. When a question card is
    // mounted, the visitor's focus belongs at the card, not the running
    // caption below it — keep the existing scroll position regardless.
    const hasCard = !!container.querySelector('.question-card');
    if (scrollParent && wasAtBottom && !hasCard) scrollParent.scrollTop = scrollParent.scrollHeight;
  }
  // Lazy-start the activity poll so the tool count keeps incrementing
  // while the agent is working. The poll auto-stops itself when the
  // agent leaves a working state.
  if (!toolStreamPollTimer && agent.session_id) {
    startToolStreamPoll(agent.session_id);
  }
}

// Seed the tally based on the latest user-message timestamp from the
// conversation. Tools fired AFTER that timestamp count toward the
// current turn's tally; tools fired BEFORE are from prior turns and
// are ignored. Called on detail-view mount (before the poll starts).
async function seedTallyFromTurnBoundary(agentId) {
  toolBuckets = {};
  try {
    const entries = await get('/api/agents/' + agentId + '/conversation');
    let cutoff = new Date(0).toISOString();
    if (Array.isArray(entries) && entries.length) {
      const lastHuman = [...entries].reverse().find(e => (e.Role || e.role) === 'human');
      if (lastHuman) cutoff = lastHuman.Timestamp || lastHuman.timestamp || cutoff;
    }
    lastSeenToolTimestamp = cutoff;
    const activity = await get('/api/agents/' + agentId + '/activity');
    if (Array.isArray(activity)) {
      for (const t of activity) {
        if ((t.Kind || t.kind) !== 'tool') continue;
        const ts = t.Timestamp || t.timestamp || '';
        if (ts <= cutoff) continue;
        const name = parseToolName(t.Content || t.content);
        const c = classifyTool(name);
        if (!c) continue;
        toolBuckets[c.bucket] = (toolBuckets[c.bucket] || 0) + 1;
        if (ts > lastSeenToolTimestamp) lastSeenToolTimestamp = ts;
      }
    }
  } catch { /* ignore — tally stays at zero */ }
}

// Poll the activity endpoint while the agent is mid-turn, bucketing
// fresh tool entries into toolBuckets so the on-screen tally updates.
function startToolStreamPoll(agentId) {
  stopToolStreamPoll();
  const tick = async () => {
    if (!isAgentMidTurn(lastKnownAgent)) {
      toolBuckets = {};
      latestToolEntry = null;
      lastSeenToolTimestamp = null;
      stopToolStreamPoll();
      return;
    }
    if (currentDetailAgentId !== agentId || currentDetailTab !== 'conversation') return;
    try {
      const entries = await get('/api/agents/' + agentId + '/activity');
      if (!Array.isArray(entries)) return;
      const tools = entries.filter(e => (e.Kind || e.kind) === 'tool');
      // Only count tools fired after the turn boundary. lastSeenToolTimestamp
      // is set either by seedTallyFromTurnBoundary() on mount (latest human
      // message timestamp) or by appendUserMessage() on a fresh send.
      const fresh = lastSeenToolTimestamp
        ? tools.filter(t => (t.Timestamp || t.timestamp) > lastSeenToolTimestamp)
        : [];
      if (fresh.length) {
        for (const t of fresh) {
          const name = parseToolName(t.Content || t.content);
          const c = classifyTool(name);
          if (!c) continue;
          toolBuckets[c.bucket] = (toolBuckets[c.bucket] || 0) + 1;
        }
        // Latest entry drives the "Last: …" inline line — show the
        // user the most recent thing the agent finished doing.
        const last = fresh[fresh.length - 1];
        latestToolEntry = { content: last.Content || last.content || '' };
        lastSeenToolTimestamp = tools[tools.length - 1].Timestamp || tools[tools.length - 1].timestamp;
        if (lastKnownAgent) refreshWorkingIndicator(lastKnownAgent);
      }
    } catch { /* ignore */ }
  };
  tick();
  toolStreamPollTimer = setInterval(tick, 1500);
}

function stopToolStreamPoll() {
  if (toolStreamPollTimer) {
    clearInterval(toolStreamPollTimer);
    toolStreamPollTimer = null;
  }
}

function timelineIcon(kind) {
  const svg = ICONS[kind] || '';
  const cls = kind === 'human' ? 'timeline-icon--human'
    : kind === 'assistant' ? 'timeline-icon--assistant'
    : 'timeline-icon--tool';
  return `<div class="timeline-icon ${cls}">${svg}</div>`;
}

function kindLabel(kind, harness) {
  if (kind === 'human') return 'You';
  if (kind === 'assistant') return harness === 'codex' ? 'Codex' : 'Claude';
  return 'Tool';
}

export function renderActionBar(agent) {
  const st = effectiveState(agent);
  const id = agent.session_id;
  let actions = '';
  let panelLabel = '';
  // Plan signal: surfaced as a small chip above the action panel whenever
  // either permission_mode='plan' or state='plan'. Covers the mid-planning
  // case (running inside plan mode but ExitPlanMode hasn't fired) which
  // would otherwise have no visible badge in detail.
  const planChip = planBadge(agent)
    ? `<div class="action-bar__status">`
      + `<span class="chip chip--plan">`
      + `<span aria-hidden="true">${escapeHtml(planBadge(agent))}</span>`
      + `<span class="visually-hidden">agent is in plan mode</span>`
      + `</span></div>`
    : '';

  // State-specific chips live above the composer (Codex pattern: action chips stacked above input).
  if (st === 'permission' || st === 'plan') {
    actions += inlineBtn('Approve', 'primary', `Dashboard.approve('${id}', event)`);
    actions += inlineBtn('Reject', 'danger', `Dashboard.reject('${id}', event)`);
    panelLabel = st === 'plan' ? 'Plan review' : 'Permission request';
  } else if (st === 'merged') {
    actions += inlineBtn('Close', 'ghost', `Dashboard.confirmClose('${id}')`);
    panelLabel = 'Branch merged';
  }
  // PR chips appear whenever the agent has an open PR — whether the
  // live state is "pr" (idle, backend swapped pinned_state in), "running"
  // (active turn but PR was created earlier), or anything else that
  // isn't "merged". hasOpenPR() consolidates the signal.
  if (hasOpenPR(agent) && st !== 'merged') {
    actions += inlineBtn('Open PR', 'secondary', `Dashboard.openPR('${id}')`);
    actions += inlineBtn('Merge', 'primary', `Dashboard.confirmMerge('${id}')`);
    panelLabel = panelLabel || 'Pull request';
  }

  // Composer is always present so the user can ask follow-up questions
  // regardless of the agent's terminal state. Stop only fits while the
  // agent's own stream can be interrupted (running) or while a paired
  // action-panel chip is the primary affordance (permission, plan). For
  // idle reply-expecting states (question, error) the placeholder below
  // says "Type a reply…" — the trailing button must agree and offer send.
  const STOP_STATES = new Set(['running', 'permission', 'plan']);
  const placeholder = (st === 'question' || st === 'error') ? 'Type a reply…'
    : (STOP_STATES.has(st) ? 'Message' : 'Ask for follow-up changes…');
  const trailing = STOP_STATES.has(st)
    ? `<button class="ui-composer__stop" aria-label="Stop" onclick="Dashboard.confirmStop('${id}')">${ICONS.stop}</button>`
    : `<button class="ui-composer__send" aria-label="Send" onclick="Dashboard.sendInput('${id}')">${ICONS.send}</button>`;
  const modelLabel = agent.model ? escapeHtml(agent.model) : 'auto';
  const branchLabel = agent.branch ? escapeHtml(agent.branch) : 'no branch';
  const effortLabel = agent.effort ? escapeHtml(agent.effort) : 'high';
  const composer = `<div class="ui-composer detail-composer">
    <textarea
      class="ui-composer__input"
      id="reply-input"
      rows="1"
      placeholder="${escapeHtml(placeholder)}"
      oninput="UI.composerAutoSize(this)"
      onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();Dashboard.sendInput('${id}')}"
    ></textarea>
    <div class="ui-composer__rail">
      <button class="ui-composer__attach" aria-label="Attach file" title="Attach file from your Mac" onclick="Dashboard.attachFile()">${ICONS.attach}</button>
      <button class="ui-composer__chip" data-chip="model" tabindex="-1" aria-label="Model"><span>${modelLabel}</span></button>
      <button class="ui-composer__chip" data-chip="branch" tabindex="-1" aria-label="Branch"><span>${branchLabel}</span></button>
      <button class="ui-composer__chip" data-chip="effort" tabindex="-1" aria-label="Effort"><span>⚡ ${effortLabel}</span></button>
      <span class="ui-composer__rail-spacer"></span>
      <button class="ui-composer__mic" aria-label="Voice input" tabindex="-1">${ICONS.mic || '<svg viewBox=\"0 0 24 24\" width=\"18\" height=\"18\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"1.75\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><rect x=\"9\" y=\"3\" width=\"6\" height=\"12\" rx=\"3\"/><path d=\"M5 11a7 7 0 0014 0\"/><path d=\"M12 18v3\"/></svg>'}</button>
      ${trailing}
    </div>
  </div>`;

  // Wrap the action chips in a labeled panel — gives the floating
  // buttons context ("Pull request", "Permission request", …) and a
  // surface that visually pairs with the composer card below.
  const actionRow = actions
    ? `<div class="action-panel">
         <span class="action-panel__label">${escapeHtml(panelLabel)}</span>
         <div class="action-panel__chips">${actions}</div>
       </div>`
    : '';
  return `<div class="action-bar">${planChip}${actionRow}${composer}</div>`;
}

let activityFilter = 'all';
let currentPRUrl = '';
let currentDetailTab = 'conversation';
let currentDetailAgentId = null;
let lastAgentState = null;
let conversationPollTimer = null;
const CONVERSATION_POLL_MS = 750;

// Render a chat-stream plan-link as an assistant message bubble.
// Wrapped in .ui-msg--assistant + .ui-msg__card so it sits in the
// conversation flow with the same surface treatment as a regular
// agent reply — just clickable. Anchored to the backend-emitted
// plan-saved synthetic entry's timestamp (ExitPlanMode tool_use or
// first-slug entry) so the bubble stays in its chronological slot
// across subsequent polls.
//
// Approve/Reject buttons live on the card itself so they persist with
// the message — the action panel above the composer disappears once
// state moves on (and for codex never appears at all, since codex has
// no ExitPlanMode-equivalent state transition).
function renderPlanLinkCard(agentId, suppressActions) {
  const inner = `<button class="chat-plan-link" type="button" onclick="Dashboard.openDetailTab('plan')">
    <span class="chat-plan-link__icon">${ICONS.clipboard}</span>
    <span class="chat-plan-link__body">
      <span class="chat-plan-link__label">Plan</span>
      <span class="chat-plan-link__title">View plan</span>
    </span>
    <span class="chat-plan-link__chevron">${ICONS.chevronRight}</span>
  </button>`;
  // Suppress in-card buttons whenever the action panel is rendering
  // them already (claude state=plan or state=permission). Otherwise
  // — codex and post-plan claude states — the in-card row is the
  // only decision affordance and stays visible. Single button system,
  // never co-visible with the action-panel duplicate.
  const actions = (agentId && !suppressActions) ? `<div class="chat-plan-link__actions">
    ${inlineBtn('Approve', 'primary', `Dashboard.approve('${agentId}', event)`)}
    ${inlineBtn('Reject', 'danger', `Dashboard.reject('${agentId}', event)`)}
  </div>` : '';
  return `<div class="ui-msg ui-msg--assistant ui-msg--plan-link"><div class="ui-msg__card ui-msg__card--plan-link">${inner}${actions}</div></div>`;
}

// Drop entries the renderer wouldn't display (internal notifications,
// empty content). plan-saved synthetic entries pass through (no
// content body — they render the plan-link card from their role
// alone). Pure — exported for unit tests so the same predicate
// drives appendNewEntries' count math.
export function visibleEntries(entries) {
  if (!Array.isArray(entries)) return [];
  const out = [];
  for (const entry of entries) {
    if (entry.IsNotification) continue;
    const role = entry.Role || entry.role;
    if (role === 'plan-saved') { out.push(entry); continue; }
    const content = entry.Content || entry.content || '';
    if (!content) continue;
    const prev = out[out.length - 1];
    const prevRole = prev ? (prev.Role || prev.role) : '';
    if (role === 'assistant' && prevRole === 'assistant') {
      const prevContent = prev.Content || prev.content || '';
      const sep = prevContent.endsWith('\n') || content.startsWith('\n') || content.startsWith(' ')
        ? ''
        : '\n\n';
      out[out.length - 1] = {
        ...prev,
        Content: prevContent + sep + content,
        content: undefined,
        Timestamp: entry.Timestamp || entry.timestamp || prev.Timestamp || prev.timestamp,
      };
      continue;
    }
    out.push(entry);
  }
  return out;
}

// Render a single visible entry to HTML. Extracted so both the initial
// full-render path (renderConversationHtml) and the incremental poll
// path (appendNewEntries) emit identical markup. agentId is threaded
// through so plan-saved entries can render their inline approve/reject
// buttons against the right session.
function renderEntryHtml(entry, agentId, suppressPlanActions) {
  const role = entry.Role || entry.role;
  if (role === 'plan-saved') return renderPlanLinkCard(agentId, suppressPlanActions);
  const content = entry.Content || entry.content || '';
  if (role === 'human') return UI.message('user', content);
  return UI.message('assistant', renderMarkdown(content), { html: true });
}

function entryRenderSignature(entry, suppressPlanActions) {
  const role = entry.Role || entry.role || '';
  const content = entry.Content || entry.content || '';
  return JSON.stringify([role, content, role === 'plan-saved' && !!suppressPlanActions]);
}

// True when the action panel above the composer is already rendering
// Approve/Reject for this agent — in that case the in-card buttons on
// the plan-saved entry must be suppressed to avoid four buttons in one
// viewport.
function actionPanelHasApproveReject(agent) {
  if (!agent) return false;
  const st = effectiveState(agent);
  return st === 'plan' || st === 'permission';
}

// Build conversation HTML from an array of message entries — Codex flat-prose.
function renderConversationHtml(entries, agentId, suppressPlanActions) {
  let html = '<div class="conversation">';
  for (const entry of visibleEntries(entries)) html += renderEntryHtml(entry, agentId, suppressPlanActions);
  html += '</div>';
  return html;
}

// Signature of every field renderQuestionCard() reads. If this string is
// unchanged across a poll tick, reconcileQuestionCard leaves the existing
// DOM node in place — preserving the user's picked radio / typed freeform
// text. Same pattern as actionBarSignature.
function questionCardSignature(pending) {
  if (!pending || !Array.isArray(pending.questions)) return '';
  const parts = [pending.tool_use_id || ''];
  for (const q of pending.questions) {
    parts.push(`${q.header || ''}\x1f${q.question || ''}\x1f${q.multi_select ? '1' : '0'}`);
    const opts = (q.options || []).map(o => `${o.label || ''}\x1f${o.description || ''}`);
    parts.push(opts.join('\x1e'));
  }
  return parts.join('|');
}

function hasPendingQuestionPayload(pending) {
  return !!(pending && Array.isArray(pending.questions) && pending.questions.length > 0);
}

function hashString(s) {
  let h = 2166136261;
  for (let i = 0; i < String(s || '').length; i++) {
    h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  }
  return (h >>> 0).toString(36);
}

export function questionCardId(pending) {
  if (pending && pending.tool_use_id) return pending.tool_use_id;
  const sig = questionCardSignature(pending);
  return sig ? `qc-${hashString(sig)}` : '';
}

// Anatomy matches docs/design/codex-screenshots/mobile/photo_2026-06-01_17-44-47.jpg —
// elevated surface, per-question small-caps category label, radio list with
// bold title + muted description, optional freeform answer input, single
// white submit chip. Submission posts the composed answer to the existing
// /input endpoint; the agent reads it as the user's next message and the
// card disappears on the next poll once HasPendingQuestion clears.
export function renderQuestionCard(pending, agentId) {
  if (!hasPendingQuestionPayload(pending)) return '';
  const tid = escapeHtml(questionCardId(pending));
  const sig = escapeHtml(questionCardSignature(pending));
  const total = pending.questions.length;
  const blocks = pending.questions.map((q, qi) => {
    const headerId = `qc-h-${qi}`;
    const questionId = `qc-q-${qi}`;
    const header = q.header ? `<div class="question-card__label" id="${headerId}">${escapeHtml(q.header)}</div>` : '';
    const text = q.question ? `<div class="question-card__question" id="${questionId}">${escapeHtml(q.question)}</div>` : '';
    const inputType = q.multi_select ? 'checkbox' : 'radio';
    const groupRole = q.multi_select ? 'group' : 'radiogroup';
    const name = `qc-${qi}`;
    // aria-labelledby chains the visible category label + question text as
    // the group's accessible name, so screen readers announce "Auth method,
    // Which auth method should we use for the new admin route, radio group,
    // Session cookie, 1 of 3" instead of an unanchored "Session cookie".
    const groupLabelledBy = [q.header ? headerId : '', q.question ? questionId : '']
      .filter(Boolean).join(' ');
    const opts = (q.options || []).map((o, oi) => {
      const label = escapeHtml(o.label || '');
      const desc = o.description ? `<div class="question-card__option-desc">${escapeHtml(o.description)}</div>` : '';
      const inputId = `qc-${qi}-${oi}`;
      return `<label class="question-card__option" for="${inputId}">
        <input type="${inputType}" id="${inputId}" name="${name}" value="${label}" class="question-card__radio-input" oninput="window.Dashboard.questionCardUpdate('${tid}')" />
        <span class="question-card__radio" aria-hidden="true"></span>
        <span class="question-card__option-body">
          <span class="question-card__option-title">${label}</span>
          ${desc}
        </span>
      </label>`;
    }).join('');
    const freeId = `qc-free-${qi}`;
    // data-qi is the snap-target index (0..N-1) that the pager observer
    // reads to update the active dot.
    // data-option-count and data-multi feed submitQuestionCard's POST body
    // — the server needs the option count to compute the picker's "Other"
    // digit (= options.length + 1) and the multi flag to know whether to
    // append Tab between per-option digit keys.
    const optCount = (q.options || []).length;
    return `<div class="question-card__block" data-qi="${qi}" data-option-count="${optCount}" data-multi="${q.multi_select ? '1' : '0'}">
      ${header}
      ${text}
      <div class="question-card__options" role="${groupRole}"${groupLabelledBy ? ` aria-labelledby="${groupLabelledBy}"` : ''}>${opts}</div>
      <label class="question-card__answer-label" for="${freeId}">Or type a response</label>
      <input type="text" id="${freeId}" name="qc-free-${qi}" class="question-card__answer-input" placeholder="Type a response" oninput="window.Dashboard.questionCardUpdate('${tid}')" />
    </div>`;
  }).join('');
  // Pager has three visible elements: a count label ("1 of N"), a row of
  // dot buttons for direct navigation, and prev/next chevrons that
  // advance one question per click. The label gives scope at a glance;
  // the dots give position; the chevrons give an explicit nav affordance
  // that desktop users expect. The whole pager is a role="status"
  // aria-live region so its accessible name — kept current by the
  // IntersectionObserver in attachQuestionCardInteractions — announces
  // the active question to screen readers.
  const pager = total > 1
    ? `<div class="question-card__pager" role="status" aria-live="polite" aria-atomic="true" aria-label="Question 1 of ${total}">
        <span class="question-card__pager-count" aria-hidden="true">1 of ${total}</span>
        <div class="question-card__pager-dots">
          ${Array.from({ length: total }, (_, i) =>
            `<button type="button" class="question-card__pager-dot${i === 0 ? ' question-card__pager-dot--active' : ''}" aria-label="Go to question ${i + 1}" data-qi-target="${i}"></button>`
          ).join('')}
        </div>
        <div class="question-card__pager-nav">
          <button type="button" class="question-card__pager-prev" aria-label="Previous question" data-qi-step="-1" disabled>‹</button>
          <button type="button" class="question-card__pager-next" aria-label="Next question" data-qi-step="1">›</button>
        </div>
      </div>`
    : '';
  const submitId = `qc-submit-${tid}`;
  return `<div class="question-card" role="region" aria-label="Agent question" data-tool-use-id="${tid}" data-sig="${sig}" data-agent-id="${escapeHtml(agentId)}">
    ${pager}
    <div class="question-card__track">${blocks}</div>
    <div class="question-card__footer">
      <button type="button" id="${submitId}" class="question-card__submit" disabled>Send answer</button>
    </div>
  </div>`;
}

// Wire mobile-only carousel pager + the pointerdown Send handler on a
// freshly-inserted card element. Idempotent — looks for a stamp on the
// card and bails if already attached.
//
// Pointerdown (not click) is required: on iOS Safari PWA, tapping the
// Send button while a freeform <input> has focus blurs the input first,
// which dismisses the soft keyboard and triggers a viewport reflow. The
// button moves off the touch point before `click` fires, so the tap is
// lost. `pointerdown` fires before that blur cascade — paired with
// `mousedown`'s preventDefault on desktop Safari, the tap reliably
// reaches the handler.
function attachQuestionCardInteractions(cardEl, agentId, toolUseId) {
  if (!cardEl || cardEl.dataset.qcWired === '1') return;
  cardEl.dataset.qcWired = '1';

  const btn = cardEl.querySelector('.question-card__submit');
  if (btn) {
    const fire = (e) => {
      if (btn.disabled) return;
      if (e && typeof e.preventDefault === 'function') e.preventDefault();
      const dash = (typeof window !== 'undefined') && window.Dashboard;
      if (dash && typeof dash.answerQuestion === 'function') {
        dash.answerQuestion(agentId, toolUseId);
      }
    };
    btn.addEventListener('pointerdown', fire);
    btn.addEventListener('mousedown', (e) => { if (!btn.disabled) e.preventDefault(); });
    // `click` is the cross-browser fallback for keyboards (Enter/Space) and
    // Playwright's default action. pointerdown already preventDefault'd the
    // tap on touch devices; click here lets the same logic run for keyboard
    // users who never produced a pointer event.
    btn.addEventListener('click', fire);
  }

  const track = cardEl.querySelector('.question-card__track');
  const pager = cardEl.querySelector('.question-card__pager');
  const count = cardEl.querySelector('.question-card__pager-count');
  const dots = cardEl.querySelectorAll('.question-card__pager-dot');
  if (track && dots.length > 1 && typeof IntersectionObserver === 'function') {
    const total = dots.length;
    const blocks = track.querySelectorAll('.question-card__block');
    const prev = cardEl.querySelector('.question-card__pager-prev');
    const next = cardEl.querySelector('.question-card__pager-next');
    let activeIdx = 0;
    const setActive = (i) => {
      activeIdx = i;
      dots.forEach((d, di) => d.classList.toggle('question-card__pager-dot--active', di === i));
      // Keep the screen-reader-visible status aligned with the visible
      // dot. aria-live="polite" announces the change without interrupting.
      if (pager) pager.setAttribute('aria-label', `Question ${i + 1} of ${total}`);
      if (count) count.textContent = `${i + 1} of ${total}`;
      if (prev) prev.disabled = i <= 0;
      if (next) next.disabled = i >= total - 1;
    };
    const io = new IntersectionObserver((entries) => {
      // Pick the entry with the largest intersectionRatio — the slide most
      // visible in the track viewport — and mark its dot active.
      let best = null;
      for (const e of entries) {
        if (e.isIntersecting && (!best || e.intersectionRatio > best.intersectionRatio)) best = e;
      }
      if (!best) return;
      const idx = parseInt(best.target.dataset.qi || '0', 10);
      setActive(idx);
    }, { root: track, threshold: [0.5, 0.75, 0.95] });
    blocks.forEach((b) => io.observe(b));
    cardEl._qcPagerObserver = io;
    // Click a dot → scroll the matching block into view. Desktop users
    // can't swipe and rely on this; mobile users get it as a bonus tap
    // target. The block's scroll-snap-align: start handles the snap.
    const scrollToIdx = (i) => {
      const target = blocks[i];
      if (target && typeof target.scrollIntoView === 'function') {
        target.scrollIntoView({ behavior: 'smooth', inline: 'start', block: 'nearest' });
      }
    };
    dots.forEach((dot) => {
      dot.addEventListener('click', (e) => {
        e.preventDefault();
        scrollToIdx(parseInt(dot.dataset.qiTarget || '0', 10));
      });
    });
    if (prev) prev.addEventListener('click', (e) => {
      e.preventDefault();
      if (activeIdx > 0) scrollToIdx(activeIdx - 1);
    });
    if (next) next.addEventListener('click', (e) => {
      e.preventDefault();
      if (activeIdx < total - 1) scrollToIdx(activeIdx + 1);
    });
  }
}

// Re-evaluate the submit-button enabled state for a question card.
// Submit is enabled once every question has either a picked option or
// non-empty freeform text. Kept in detail.js so it can read the
// per-card DOM directly without touching app.js's module scope.
export function updateQuestionCardSubmit(toolUseId) {
  const card = document.querySelector(`.question-card[data-tool-use-id="${cssEscape(toolUseId)}"]`);
  if (!card) return;
  const blocks = card.querySelectorAll('.question-card__block');
  let allAnswered = true;
  blocks.forEach((block) => {
    const picked = block.querySelector('.question-card__radio-input:checked');
    const free = block.querySelector('.question-card__answer-input');
    const freeText = free && free.value.trim() ? free.value.trim() : '';
    if (!picked && !freeText) allAnswered = false;
  });
  const btn = card.querySelector('.question-card__submit');
  if (btn) btn.disabled = !allAnswered;
}

// Read structured answers from a question card and POST them to the
// agent's /answer-question endpoint. Each block contributes one
// askAnswerEntry — option_indices (0-based picks against the original
// payload), freeform text (used when the user typed instead of/in
// addition to picking), and the multi-select flag. The server translates
// the payload into Claude Code's picker keystroke sequence: per-question
// digit keys (auto-advance on single-select, Tab to advance on
// multi-select), Other digit + text + Enter for freeform, final Enter on
// the Submit tab.
//
// We POST a structured payload — not formatted text — because Claude
// Code's AskUserQuestion is a native picker that reads number/arrow keys,
// not stdin text. The previous text-based path was silently dropped by
// the picker (regression caught only after #366 shipped).
export async function submitQuestionCard(agentId, toolUseId) {
  const card = document.querySelector(`.question-card[data-tool-use-id="${cssEscape(toolUseId)}"]`);
  if (!card) return false;
  const btn = card.querySelector('.question-card__submit');
  if (btn) btn.disabled = true;

  const blocks = card.querySelectorAll('.question-card__block');
  const answers = [];
  const optionCounts = [];
  blocks.forEach((block) => {
    const optCount = parseInt(block.dataset.optionCount || '0', 10);
    const multi = block.dataset.multi === '1';
    // Picker options are rendered in original payload order, so the
    // checked input's index-in-its-NodeList is exactly the 0-based
    // option_index the server expects.
    const inputs = Array.from(block.querySelectorAll('.question-card__radio-input'));
    const optionIndices = inputs.reduce((acc, input, idx) => {
      if (input.checked) acc.push(idx);
      return acc;
    }, []);
    const free = block.querySelector('.question-card__answer-input');
    const freeform = free && free.value.trim() ? free.value.trim() : '';
    answers.push({ option_indices: optionIndices, freeform, multi });
    optionCounts.push(optCount);
  });

  // Optimistic transition: collapse to "answered" snapshot so the user
  // sees their submission landed even before the polling loop catches up.
  card.classList.add('question-card--answered');

  try {
    const result = await post('/api/agents/' + agentId + '/answer-question', {
      answers,
      option_counts: optionCounts,
    });
    if (!result || !result.ok) {
      toast('Failed: ' + (result?.error || 'unknown'), 'error');
      card.classList.remove('question-card--answered');
      if (btn) btn.disabled = false;
      return false;
    }
    return true;
  } catch (err) {
    toast('Failed: ' + err.message, 'error');
    card.classList.remove('question-card--answered');
    if (btn) btn.disabled = false;
    return false;
  }
}

// Tiny CSS.escape polyfill — the dashboard needs to support older Safari
// on iOS PWA. Tool-use IDs are alphanumeric-with-underscores; this only
// covers that safely.
function cssEscape(s) {
  if (typeof window !== 'undefined' && window.CSS && typeof window.CSS.escape === 'function') {
    return window.CSS.escape(s);
  }
  return String(s).replace(/([^a-zA-Z0-9_-])/g, '\\$1');
}

// Where new entries should be inserted: ahead of any decoration node
// (question card, optimistic-message bubble + caption, working
// indicator) so the rendered chat order stays
//   entries → question card → optimistic msg → caption → working indicator
// the way appendUserMessage and the original full-render path lay it out.
// Returns null when nothing decorates the bottom yet, meaning "append".
function entryInsertAnchor(conv) {
  return conv.querySelector(
    '.question-card, [data-optimistic="1"], .ui-msg__caption--sending, .ui-msg-status--working'
  );
}

// Idempotent: keep rendered entries aligned with the latest visible
// transcript. Unchanged entries are left alone so their DOM survives the
// poll; entries whose content changed in place are replaced, which is
// how Codex partial assistant text grows without waiting for a full page
// refresh. Decoration siblings are kept outside the indexed entry set.
function appendNewEntries(conv, entries, agentId, suppressPlanActions) {
  const visible = visibleEntries(entries);
  // Report whether anything actually changed. A count comparison is not
  // enough: consecutive assistant entries MERGE in visibleEntries, so a
  // growing transcript often re-renders one entry in place instead of
  // adding a node. The jump-chip trigger rides on this signal.
  let changed = false;

  for (let i = 0; i < visible.length; i++) {
    const sig = entryRenderSignature(visible[i], suppressPlanActions);
    const existing = conv.querySelector(`:scope > [data-entry-idx="${i}"]`);
    if (existing && existing.dataset.entrySig === sig) continue;

    const wrap = document.createElement('div');
    wrap.innerHTML = renderEntryHtml(visible[i], agentId, suppressPlanActions);
    const el = wrap.firstElementChild;
    if (!el) continue;
    el.dataset.entryIdx = String(i);
    el.dataset.entrySig = sig;
    if (existing) {
      existing.replaceWith(el);
    } else {
      conv.insertBefore(el, entryInsertAnchor(conv));
    }
    changed = true;
  }
  conv.querySelectorAll(':scope > [data-entry-idx]').forEach(el => {
    const idx = parseInt(el.dataset.entryIdx || '-1', 10);
    if (idx < 0 || idx >= visible.length) {
      el.remove();
      changed = true;
    }
  });
  conv.dataset.renderedCount = String(visible.length);
  return changed;
}

// Reconcile the AskUserQuestion card without touching it when the
// pending payload is unchanged. This is the core of the fix: the card's
// DOM node is preserved across polls, so focus, caret, input.value and
// :checked all survive instead of being wiped + manually re-applied.
function reconcileQuestionCard(conv, pending, agentId) {
  const existing = conv.querySelector('.question-card');
  if (!hasPendingQuestionPayload(pending)) {
    if (existing) {
      if (existing._qcPagerObserver) existing._qcPagerObserver.disconnect();
      existing.remove();
    }
    return;
  }
  const sig = questionCardSignature(pending);
  const pendingCardId = questionCardId(pending);
  if (existing
      && existing.dataset.toolUseId === pendingCardId
      && existing.dataset.sig === sig) {
    return; // identical card — leave it alone
  }
  const isNewCard = !existing || existing.dataset.toolUseId !== pendingCardId;
  if (existing) {
    if (existing._qcPagerObserver) existing._qcPagerObserver.disconnect();
    existing.remove();
  }
  const wrap = document.createElement('div');
  wrap.innerHTML = renderQuestionCard(pending, agentId);
  const cardEl = wrap.firstElementChild;
  if (!cardEl) return;
  // Insert ahead of optimistic msg / caption / working indicator, after
  // all entry messages — same slot the original full-render path used.
  const scrollParent = conv.closest('.detail-scroll');
  // "Following" is looser here than the strict isAtBottom threshold:
  // async layout shifts (webfont reflow, indicator mounts) can leave a
  // passive follower a few hundred px off the exact bottom. Within one
  // viewport of the bottom still counts as following; only a reader
  // genuinely up in history keeps their position.
  const nearBottom = !!scrollParent
    && (scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight)
       < scrollParent.clientHeight;
  const anchor = conv.querySelector('[data-optimistic="1"], .ui-msg__caption--sending, .ui-msg-status--working');
  conv.insertBefore(cardEl, anchor);
  attachQuestionCardInteractions(cardEl, agentId, pendingCardId);
  // Arrival visibility: the card is the turn's gate — a mid-session
  // mount below the fold with no cue costs the user the whole point of
  // the page. If the reader was following the bottom, align the card's
  // top edge (same anchor the initial-load path uses); if they were up
  // reading history, don't yank — surface the jump chip instead.
  if (isNewCard && scrollParent) {
    if (nearBottom) {
      requestAnimationFrame(() => {
        const parentRect = scrollParent.getBoundingClientRect();
        const cardRect = cardEl.getBoundingClientRect();
        scrollParent.scrollTop += (cardRect.top - parentRect.top) - 12;
      });
    } else {
      showJumpChip(true);
    }
  }
}

// Reconcile the in-flight optimistic user message. Three states:
//   1. No pending message      → remove any leftover bubble / caption
//   2. Pending + not yet acked → ensure bubble + "Sending…" caption present
//   3. Pending + acked         → bubble present without caption (acked but
//                                API hasn't echoed yet)
// The bubble persists across polls — we never tear it down just to put
// it back, so the .ui-msg--optimistic class transitions cleanly without
// the bubble flickering.
function reconcileOptimisticMessage(conv) {
  const bubble = conv.querySelector('[data-optimistic="1"]');
  const caption = conv.querySelector('.ui-msg__caption--sending');
  if (!pendingUserMessage) {
    if (bubble) bubble.remove();
    if (caption) caption.remove();
    return;
  }
  if (!bubble) {
    const wrap = document.createElement('div');
    wrap.innerHTML = UI.message('user', pendingUserMessage);
    const el = wrap.firstElementChild;
    if (el) {
      el.dataset.optimistic = '1';
      if (!pendingMessageAcked) el.classList.add('ui-msg--optimistic');
      const anchor = conv.querySelector('.ui-msg__caption--sending, .ui-msg-status--working');
      conv.insertBefore(el, anchor);
    }
  } else {
    bubble.classList.toggle('ui-msg--optimistic', !pendingMessageAcked);
  }
  if (!pendingMessageAcked) {
    if (!caption) {
      const c = document.createElement('div');
      c.className = 'ui-msg__caption ui-msg__caption--sending';
      c.textContent = 'Sending…';
      const anchor = conv.querySelector('.ui-msg-status--working');
      conv.insertBefore(c, anchor);
    }
  } else if (caption) {
    caption.remove();
  }
}

// Re-fetch and incrementally update the conversation tab if it is currently
// active. Called by the 2s poll and by the SSE handler.
//
// Incremental by design: only new entries are appended; the question card,
// any optimistic message, and the working indicator are reconciled in
// place. Nothing already in the DOM is detached, which is what lets focus,
// caret position, :checked radios, and input.value survive every poll.
async function refreshConversation(agentId, agent) {
  if (currentDetailTab !== 'conversation' || currentDetailAgentId !== agentId) return;
  const container = document.getElementById('tab-conversation');
  if (!container) return;
  const [entries, pending] = await Promise.all([
    get('/api/agents/' + agentId + '/conversation'),
    get('/api/agents/' + agentId + '/pending-question'),
  ]);
  if (!entries || entries.length === 0) return; // don't wipe existing content with empty state
  const scrollParent = container.closest('.detail-scroll');
  const wasAtBottom = isAtBottom(scrollParent);

  // Detect API catching up with our optimistic message before deciding
  // whether to render an optimistic bubble; the bubble removal happens
  // inside reconcileOptimisticMessage on the next line.
  if (pendingUserMessage) {
    const lastHuman = [...entries].reverse().find(e => (e.Role || e.role) === 'human');
    const lastContent = lastHuman ? (lastHuman.Content || lastHuman.content || '') : '';
    if (lastContent.includes(pendingUserMessage)) {
      pendingUserMessage = null;
      pendingMessageAcked = false;
    }
  }

  // First poll after the empty-state placeholder ran — initialise the
  // .conversation skeleton so the incremental path has somewhere to
  // write. Main's appendNewEntries is incremental and doesn't wipe DOM,
  // so tables / cards / focused inputs persist naturally across polls
  // (replaces the earlier snapshot-and-restore-scrollLeft hack).
  let conv = container.querySelector('.conversation');
  if (!conv) {
    container.innerHTML = '<div class="conversation"></div>';
    conv = container.querySelector('.conversation');
  }

  const grew = appendNewEntries(conv, entries, agentId, actionPanelHasApproveReject(agent));
  reconcileQuestionCard(conv, pending, agentId);
  reconcileOptimisticMessage(conv);
  if (agent) refreshWorkingIndicator(agent);
  applyComposerGate(hasPendingQuestionPayload(pending));
  // New entries landed while the reader was up in history — offer the
  // jump affordance instead of silently growing the page below them.
  if (!wasAtBottom && grew) showJumpChip(!!conv.querySelector('.question-card'));
  else if (wasAtBottom) hideJumpChip();

  // Don't snap to bottom when a question card is mounted — the visitor
  // is filling out the card and must keep it in view.
  const hasCard = !!conv.querySelector('.question-card');
  if (scrollParent && wasAtBottom && !hasCard) scrollParent.scrollTop = scrollParent.scrollHeight;
}

// Poll conversation while the chat tab is active.
// This provides near-realtime streaming of agent responses since the JSONL
// is written by Claude Code (not the dashboard), so fsnotify/SSE doesn't
// trigger on new conversation lines.
function startConversationPoll(agentId) {
  stopConversationPoll();
  conversationPollTimer = setInterval(() => {
    if (currentDetailTab === 'conversation' && currentDetailAgentId === agentId) {
      refreshConversation(agentId, lastKnownAgent);
    } else {
      stopConversationPoll();
    }
  }, CONVERSATION_POLL_MS);
}

function stopConversationPoll() {
  if (conversationPollTimer) {
    clearInterval(conversationPollTimer);
    conversationPollTimer = null;
  }
}

// Refresh whichever tab is currently active. Called on SSE events.
// Conversation uses its own fetch path (no nav signal). Activity and plan
// use loadTabContent which creates a nav signal, so we debounce to avoid
// rapid SSE events causing cancellation churn.
let refreshTimer = null;
export function refreshActiveTab(agentId, agent) {
  if (currentDetailAgentId !== agentId) return;
  if (currentDetailTab === 'conversation') {
    refreshConversation(agentId, agent);
    return;
  }
  if (currentDetailTab === 'diff') return; // expensive, skip
  // Debounce activity/plan refreshes to avoid nav signal churn
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => {
    if (currentDetailAgentId !== agentId) return;
    if (currentDetailTab === 'activity' || currentDetailTab === 'plan') {
      loadTabContent(currentDetailTab, agentId);
    }
  }, 500);
}

// Update the detail header (status badge, duration) from SSE agent data.
export function refreshDetailHeader(agent) {
  if (!agent) return;
  const st = effectiveState(agent);
  const prev = lastAgentState;
  lastAgentState = st;

  // Update status pill
  const pill = document.querySelector('.detail-title .ui-status-pill');
  if (pill) {
    const tmp = document.createElement('div');
    tmp.innerHTML = inlineStatusPill(st);
    const fresh = tmp.firstElementChild;
    if (fresh) pill.replaceWith(fresh);
  }

  // Update duration — pick the last non-separator span (B1 split each
  // meta token into its own <span>, separated by .detail-meta__sep nodes).
  const meta = document.querySelector('.detail-meta');
  if (meta && agent.started_at) {
    const spans = meta.querySelectorAll('span:not(.detail-meta__sep)');
    const last = spans[spans.length - 1];
    if (last) last.textContent = duration(agent);
  }

  // Refresh vital signs only on state change
  if (prev !== null && prev !== st) {
    loadVitalSigns(agent.session_id, agent);
  }
}

function applyActivityFilter(container) {
  container.querySelectorAll('.timeline-entry').forEach(el => {
    const kind = el.dataset.kind;
    if (!kind) return;
    if (activityFilter === 'all' || kind === activityFilter) {
      el.classList.remove('hidden');
    } else {
      el.classList.add('hidden');
    }
  });
}

export async function renderDetail(app, agents, agentId, setView) {
  cancelNav();
  stopConversationPoll();
  pendingUserMessage = null;
  pendingMessageAcked = false;
  activityFilter = 'all';
  setView('detail', agentId);
  const agent = agents.find(a => a.session_id === agentId);
  if (!agent) { window.Dashboard.showList(); return; }
  currentPRUrl = agent.pr_url || '';

  const st = effectiveState(agent);
  const branchPart = agent.branch ? escapeHtml(agent.branch) : '';
  const modelPart = agent.model ? escapeHtml(agent.model) : '';
  const durationPart = agent.started_at ? duration(agent) : '';
  // Render each meta token as its own <span> so refreshDetailHeader()
  // can poke the *last* span (the live duration) without rebuilding the
  // whole row.
  const metaSpans = [branchPart, modelPart, durationPart]
    .filter(Boolean)
    .map(t => `<span>${t}</span>`)
    .join('<span class="detail-meta__sep">·</span>');

  // App bar: back arrow + title + trailing (theme / more). The running
  // spinner used to live in the trailing slot; it has been removed because
  // (a) it duplicated the "● Working" status pill already in the row
  // below, and (b) it rendered at identical visual weight to the theme +
  // kebab buttons, fusing passive status with interactive controls and
  // causing the 3-icon huddle that read as "weird and unnatural."
  // Status of the session remains visible via the inline status pill in
  // `.detail-title`.
  const appBar = UI.appBar({
    back: true,
    title: repoName(agent),
    trailing: [
      Theme.trailingEntry(),
      { icon: ICONS.kebab, ariaLabel: 'More', onclick: `Dashboard.openDetailKebab('${agent.session_id}')` },
    ],
  });

  // Suppress the tag chip in the detail header when the status pill
  // already says "PR open" (state === 'pr', the idle case). Sidebar/list
  // rows have no status pill, so they always show the tag.
  const prChip = (prTag(agent) && st !== 'pr')
    ? `<span class="ui-row__tag detail-header__tag">${escapeHtml(prTag(agent))}</span>`
    : '';
  // Title moved to the app bar; this row carries only status + meta now,
  // so the page reads top-to-bottom as [Back · Title · Actions] →
  // [Status pill · PR chip] → [branch · model · duration].
  const metaLine = metaSpans
    ? `<div class="detail-meta">${metaSpans}</div>`
    : '';
  const detailHeader = `
    <div class="detail-header">
      <div class="detail-title">
        ${inlineStatusPill(st)}
        ${prChip}
      </div>
      ${metaLine}
    </div>
  `;

  const TAB_KEYS = ['conversation', 'activity', 'diff', 'plan'];
  let savedTab = 'conversation';
  try {
    const stored = sessionStorage.getItem('detail-tab-' + agentId);
    if (stored && TAB_KEYS.includes(stored)) savedTab = stored;
  } catch {}

  const tabs = inlineSegmentedTabs([
    { key: 'conversation', label: 'Chat' },
    { key: 'activity', label: 'Activity' },
    { key: 'diff', label: 'Diff' },
    { key: 'plan', label: 'Plan' },
  ], savedTab);

  const isMobile = window.innerWidth <= 480;
  const vitalOpen = !isMobile && sessionStorage.getItem('collapse-vital-signs-container-' + agentId) !== 'true';
  const activeCls = (key) => key === savedTab ? ' active' : '';

  // STATS disclosure is metadata about the current conversation
  // (elapsed / tokens / cost) — only meaningful on the Chat tab. It's
  // CSS-scoped to the active Chat tab in style.css (.detail-scroll
  // rules), so it stays hidden on Activity / Diff / Plan (C1b).
  // Subagents disclosure removed entirely (C1) — subagent activity is
  // implied by the live tool-tally line.

  app.innerHTML = `
    <div class="detail-layout">
      <div class="detail-pinned">
        ${appBar}
        ${detailHeader}
        ${tabs}
      </div>
      <div class="detail-scroll">
        <div class="detail-supplementary">
          ${inlineDisclosure('vital-signs-container', 'Stats', vitalOpen)}
        </div>
        <div id="tab-conversation" class="tab-content${activeCls('conversation')}">${savedTab === 'conversation' ? skeletonLoading(4) : ''}</div>
        <div id="tab-activity" class="tab-content${activeCls('activity')}">${savedTab === 'activity' ? skeletonLoading(6) : ''}</div>
        <div id="tab-diff" class="tab-content${activeCls('diff')}">${savedTab === 'diff' ? skeletonLoading(3) : ''}</div>
        <div id="tab-plan" class="tab-content${activeCls('plan')}">${savedTab === 'plan' ? skeletonLoading(3) : ''}</div>
      </div>
      ${renderActionBar(agent)}
    </div>
  `;

  // Phase C dock-migration: on desktop, prepend the header-placement dock
  // into the app-bar trailing slot so + New / Search are reachable from
  // detail view without traversing to the sidebar. Floating dock remains
  // mobile-only (rendered by list.js).
  if (isDesktop()) {
    const trailing = app.querySelector('.ui-app-bar__trailing');
    if (trailing) {
      const wrap = document.createElement('div');
      wrap.innerHTML = UI.dock({
        placement: 'header',
        search: { label: 'Search agents', onclick: 'Dashboard.searchAgents()' },
        cta: { label: 'New', icon: ICONS.pencil, onclick: 'Dashboard.showCreate()' },
      });
      trailing.insertAdjacentElement('afterbegin', wrap.firstElementChild);
    }
  }

  // Seed the action-bar signature so the first SSE tick after mount
  // doesn't trigger a redundant DOM swap (and dismiss the mobile keyboard).
  const initialBar = app.querySelector('.action-bar');
  if (initialBar) initialBar.dataset.sig = actionBarSignature(agent);

  // Tab switching
  currentDetailTab = savedTab;
  currentDetailAgentId = agentId;
  lastAgentState = st;
  // Fresh detail-view mount — re-enable the one-shot initial scroll so
  // navigating Agent A → B → A snaps back to the bottom of A's
  // conversation on the second visit (the DOM was torn down).
  conversationScrolledThisSession = false;
  document.querySelectorAll('.detail-tabs__tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.detail-tabs__tab').forEach(t => t.classList.remove('detail-tabs__tab--active'));
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
      tab.classList.add('detail-tabs__tab--active');
      const target = tab.dataset.tab;
      const container = document.getElementById('tab-' + target);
      container.classList.add('active');
      // Only show skeleton when the tab is empty (first visit) — avoids flicker on re-clicks.
      if (!container.dataset.loaded) container.innerHTML = skeletonLoading(target === 'activity' ? 6 : target === 'conversation' ? 4 : 3);
      currentDetailTab = target;
      if (target !== 'conversation') hideJumpChip();
      try { sessionStorage.setItem('detail-tab-' + agentId, target); } catch {}
      loadTabContent(target, agentId);
      if (target === 'conversation') startConversationPoll(agentId);
      else stopConversationPoll();
    });
  });

  // Persist disclosure state
  document.querySelectorAll('.detail-disclosure').forEach(details => {
    details.addEventListener('toggle', () => {
      const summary = details.querySelector('summary');
      if (!summary) return;
      const sectionId = summary.dataset.section;
      try { sessionStorage.setItem('collapse-' + sectionId + '-' + agentId, String(!details.open)); } catch {}
    });
  });

  // Load initial tab + vital signs in parallel
  loadTabContent(savedTab, agentId);
  loadVitalSigns(agentId, agent);

  // Mount the working indicator if the agent is currently processing.
  // loadTabContent populates .conversation asynchronously so defer the mount.
  lastKnownAgent = agent;
  seedTallyFromTurnBoundary(agentId).then(() => {
    refreshWorkingIndicator(agent);
  });

  // Wire slash-command autocomplete to the composer textarea. Harness
  // controls the trigger sigil ("/" for claude, "$" for codex) and the
  // skill list returned from /api/skills?harness=.
  const composerInput = document.getElementById('reply-input');
  if (composerInput) attachSlashAutocomplete(composerInput, agent.harness);
  applyComposerGate(!!questionBadge(agent));

  // Jump chip is per-mount state — clear leftovers from a previous
  // detail view, and dismiss automatically once the reader reaches the
  // bottom on their own.
  hideJumpChip();
  const detailScrollEl = app.querySelector('.detail-scroll');
  if (detailScrollEl) {
    detailScrollEl.addEventListener('scroll', () => {
      if (isAtBottom(detailScrollEl)) hideJumpChip();
    }, { passive: true });
  }

  // Start conversation polling only when the conversation tab is active.
  if (savedTab === 'conversation') startConversationPoll(agentId);
}

// Per-agent vital signs cache. Keyed by agentId so switching between
// agents doesn't bleed values. Used to suppress redundant innerHTML
// rewrites — without this, every state-change SSE event wipes the
// strip and re-mounts it, producing a visible cost flicker on mobile.
const vitalSignsCache = new Map(); // agentId → { tokens, cost }

async function loadVitalSigns(agentId, agent) {
  const container = document.getElementById('vital-signs-container');
  if (!container) return;
  try {
    const usage = await get('/api/agents/' + agentId + '/usage');
    const elapsed = agent.started_at ? duration(agent) : '';
    const tokens = (usage && usage.InputTokens ? usage.InputTokens + (usage.OutputTokens || 0) : 0);
    const cost = usage ? usage.CostUSD : 0;
    const prev = vitalSignsCache.get(agentId);

    // If the strip is already mounted AND tokens/cost are unchanged,
    // just patch the elapsed cell so the duration ticks without
    // wiping the rest of the strip (which is what causes the flicker).
    if (prev && prev.tokens === tokens && prev.cost === cost) {
      const cells = container.querySelectorAll('.vital-value');
      if (cells.length === 3) {
        cells[0].textContent = elapsed;
        return;
      }
    }
    vitalSignsCache.set(agentId, { tokens, cost });
    container.innerHTML = inlineVitalStrip({ elapsed, tokens, cost });
  } catch {
    container.innerHTML = '';
  }
}

async function loadTabContent(tab, agentId) {
  const signal = newNavSignal();
  const container = document.getElementById('tab-' + tab);
  if (!container) return;
  // Mark loaded after this fetch so subsequent tab-switches don't re-show skeleton.
  const markLoaded = () => { try { container.dataset.loaded = '1'; } catch {} };

  switch (tab) {
    case 'conversation': {
      const [entries, pending] = await Promise.all([
        get('/api/agents/' + agentId + '/conversation'),
        get('/api/agents/' + agentId + '/pending-question'),
      ]);
      if (signal.aborted) return;
      if (!entries || entries.length === 0) {
        if (pendingUserMessage) {
          container.innerHTML = '<div class="conversation"></div>';
          const conv = container.querySelector('.conversation');
          if (conv) {
            reconcileQuestionCard(conv, pending, agentId);
            reconcileOptimisticMessage(conv);
          }
          markLoaded();
          break;
        }
        container.innerHTML = inlineEmptyState(ICONS.chat, 'No conversation yet', 'Messages will appear here once the agent starts');
        markLoaded();
        return;
      }
      container.innerHTML = renderConversationHtml(entries, agentId, actionPanelHasApproveReject(lastKnownAgent));
      const conv = container.querySelector('.conversation');
      if (conv) {
        // Seed the incremental-render bookkeeping so the next poll's
        // appendNewEntries knows what's already in the DOM. Stamping
        // data-entry-idx on each rendered message keeps the rewind
        // fallback (history shrank) able to find and prune them.
        const visible = visibleEntries(entries);
        const msgs = conv.querySelectorAll(':scope > .ui-msg');
        for (let i = 0; i < msgs.length && i < visible.length; i++) {
          msgs[i].dataset.entryIdx = String(i);
          msgs[i].dataset.entrySig = entryRenderSignature(visible[i], actionPanelHasApproveReject(lastKnownAgent));
        }
        conv.dataset.renderedCount = String(visible.length);
        if (hasPendingQuestionPayload(pending)) {
          const wrap = document.createElement('div');
          wrap.innerHTML = renderQuestionCard(pending, agentId);
          const cardEl = wrap.firstElementChild;
          if (cardEl) {
            conv.appendChild(cardEl);
            attachQuestionCardInteractions(cardEl, agentId, questionCardId(pending));
          }
        }
        reconcileOptimisticMessage(conv);
      }
      markLoaded();
      // Only snap to bottom on the *first* conversation load of this
      // detail session — subsequent tab-switches back to Conversation
      // re-render the same content and should preserve the user's
      // scroll position. renderDetail() resets the flag when a new agent
      // detail view mounts.
      if (!conversationScrolledThisSession) {
        const scrollParent = container.closest('.detail-scroll');
        if (scrollParent) {
          // When a question card is present, prefer aligning the card's
          // TOP rather than snapping to the conversation bottom — the
          // card is the workflow gate and its first eye-stop (the pager
          // count + question header) must be in view, not its Submit
          // footer. rAF lets layout settle before measuring.
          const card = conv && conv.querySelector('.question-card');
          if (card && typeof card.getBoundingClientRect === 'function') {
            requestAnimationFrame(() => {
              const parentRect = scrollParent.getBoundingClientRect();
              const cardRect = card.getBoundingClientRect();
              scrollParent.scrollTop += (cardRect.top - parentRect.top) - 12;
            });
          } else {
            scrollParent.scrollTop = scrollParent.scrollHeight;
          }
        }
        conversationScrolledThisSession = true;
      }
      break;
    }
    case 'activity': {
      const entries = await get('/api/agents/' + agentId + '/activity');
      if (signal.aborted) return;
      if (!entries || entries.length === 0) {
        container.innerHTML = inlineEmptyState(ICONS.activity, 'No activity yet', 'Tool calls and messages will appear here');
        markLoaded();
        return;
      }
      let html = '<div class="activity-filter-bar">';
      for (const f of ['all', 'human', 'assistant', 'tool']) {
        const cls = f === activityFilter ? ' active' : '';
        const label = f === 'all' ? 'All' : f === 'human' ? 'Human' : f === 'assistant' ? 'Assistant' : 'Tool';
        html += `<button class="activity-filter-btn${cls}" data-filter="${f}">${label}</button>`;
      }
      html += '</div><div class="activity-log">';

      // Group entries into turns
      const turns = [];
      let currentTurn = [];
      for (const entry of entries) {
        const kind = entry.Kind || entry.kind;
        if (kind === 'human' && currentTurn.length > 0) {
          turns.push(currentTurn);
          currentTurn = [];
        }
        currentTurn.push(entry);
      }
      if (currentTurn.length > 0) turns.push(currentTurn);

      for (const turn of turns) {
        let toolGroup = [];
        for (const entry of turn) {
          const kind = entry.Kind || entry.kind;
          const content = entry.Content || entry.content || '';
          const time = entry.Timestamp || entry.timestamp || '';

          if (kind === 'tool') {
            toolGroup.push(entry);
            continue;
          }
          // Flush any pending tool group
          if (toolGroup.length > 0) {
            html += renderToolGroup(toolGroup);
            toolGroup = [];
          }

          // Strip <local-command-*> wrappers from human entries — same
          // surface as the Chat tab (C3). Assistant content is markdown
          // and never carries those tags.
          const cleanContent = kind === 'human' ? stripLocalCommandTags(content) : content;
          const truncated = cleanContent.length > 200;
          const displayContent = truncated ? cleanContent.substring(0, 200) + '...' : cleanContent;
          html += `<div class="timeline-entry activity-entry" data-kind="${kind}">`;
          html += timelineIcon(kind);
          html += '<div class="timeline-content">';
          html += `<div class="timeline-header"><span class="timeline-title">${kindLabel(kind, lastKnownAgent && lastKnownAgent.harness)}</span><span class="timeline-timestamp">${formatTimeShort(time)}</span></div>`;
          if (kind === 'assistant') {
            html += `<div class="timeline-detail">${renderMarkdown(displayContent)}</div>`;
          } else {
            html += `<div class="timeline-detail">${escapeHtml(displayContent)}</div>`;
          }
          if (truncated) {
            html += `<span data-full="${escapeHtml(cleanContent)}" data-truncated="true" style="display:none"></span>`;
            html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
          }
          html += '</div></div>';
        }
        // Flush remaining tool group
        if (toolGroup.length > 0) {
          html += renderToolGroup(toolGroup);
        }
      }
      html += '</div>';
      container.innerHTML = html;
      markLoaded();

      // Wire filter buttons
      container.querySelectorAll('.activity-filter-btn').forEach(btn => {
        btn.addEventListener('click', () => {
          activityFilter = btn.dataset.filter;
          container.querySelectorAll('.activity-filter-btn').forEach(b => b.classList.remove('active'));
          btn.classList.add('active');
          applyActivityFilter(container);
        });
      });
      break;
    }
    case 'diff': {
      const data = await get('/api/agents/' + agentId + '/diff');
      if (signal.aborted) return;
      if (!data || !data.raw) {
        const status = data && data.status;
        let title = 'No diff available';
        let subtitle = 'Changes will appear here once the agent modifies files';
        if (status === 'empty') {
          title = 'No changes yet';
          subtitle = 'The agent hasn’t modified files in this worktree.';
        } else if (status === 'error') {
          title = 'Unable to load diff';
          subtitle = 'git reported an error reading this worktree.';
        }
        container.innerHTML = inlineEmptyState(ICONS.fileDiff, title, subtitle);
        markLoaded();
        return;
      }

      // On mobile, show a PR link instead of rendering diffs
      if (window.innerWidth <= 768) {
        const files = data.files || [];
        const totalAdds = files.reduce((s, f) => s + (f.additions || 0), 0);
        const totalDels = files.reduce((s, f) => s + (f.deletions || 0), 0);
        const prUrl = currentPRUrl ? currentPRUrl + '/files' : '';
        const hasPR = !!prUrl;

        let html = '<div class="mobile-diff-summary">';
        html += '<div class="mobile-diff-stats">'
          + '<span class="diff-stats-add">+' + totalAdds + '</span> '
          + '<span class="diff-stats-del">-' + totalDels + '</span> '
          + 'across ' + files.length + ' file' + (files.length !== 1 ? 's' : '')
          + '</div>';

        // File list
        html += '<div class="mobile-diff-files">';
        for (const f of files) {
          const status = f.status || 'modified';
          html += '<div class="mobile-diff-file">'
            + inlineFileStatus(status)
            + '<span class="mobile-diff-file-path">' + escapeHtml(f.path) + '</span>'
            + '<span class="diff-stats"><span class="diff-stats-add">+' + (f.additions || 0) + '</span> <span class="diff-stats-del">-' + (f.deletions || 0) + '</span></span>'
            + '</div>';
        }
        html += '</div>';

        if (hasPR) {
          html += '<a class="mobile-pr-link mobile-pr-link--active" href="' + escapeHtml(prUrl) + '" target="_blank" rel="noopener">'
            + '&#x2197; View Diff on GitHub</a>';
        } else {
          html += '<div class="mobile-pr-link mobile-pr-link--inactive">'
            + 'No PR available &mdash; create a PR to view diffs on mobile</div>';
        }
        html += '</div>';
        container.innerHTML = html;
        markLoaded();
        return;
      }

      const files = data.files || [];
      const rawLines = data.raw.split('\n');
      const fileChunks = [];
      let chunkStart = -1;
      for (let i = 0; i < rawLines.length; i++) {
        if (rawLines[i].startsWith('diff --git')) {
          if (chunkStart >= 0) fileChunks.push(rawLines.slice(chunkStart, i).join('\n'));
          chunkStart = i;
        }
      }
      if (chunkStart >= 0) fileChunks.push(rawLines.slice(chunkStart).join('\n'));

      // Totals for summary bar
      const totalAdds = files.reduce((s, f) => s + (f.additions || 0), 0);
      const totalDels = files.reduce((s, f) => s + (f.deletions || 0), 0);

      // View mode from localStorage
      let viewMode = localStorage.getItem('diff-view-mode') || 'side-by-side';
      // Force unified on narrow screens
      if (window.innerWidth <= 768) viewMode = 'line-by-line';

      // Build sidebar
      const dirGroups = {};
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        const parts = f.path.split('/');
        const fileName = parts.pop();
        const dir = parts.join('/') || '.';
        if (!dirGroups[dir]) dirGroups[dir] = [];
        dirGroups[dir].push({ ...f, fileName, idx: i });
      }

      let sidebarHtml = '<div class="diff-sidebar"><div class="diff-sidebar-header">Files (' + files.length + ')</div>';
      for (const [dir, dirFiles] of Object.entries(dirGroups)) {
        const dirAdds = dirFiles.reduce((s, f) => s + (f.additions || 0), 0);
        const dirDels = dirFiles.reduce((s, f) => s + (f.deletions || 0), 0);
        sidebarHtml += '<details class="diff-dir-group" open>'
          + '<summary class="diff-dir-summary">'
          + '<span class="diff-dir-name">' + escapeHtml(dir) + '</span>'
          + '<span class="diff-stats"><span class="diff-stats-add">+' + dirAdds + '</span> <span class="diff-stats-del">-' + dirDels + '</span></span>'
          + '</summary>';
        for (const f of dirFiles) {
          const status = f.status || 'modified';
          const adds = f.additions || 0;
          const dels = f.deletions || 0;
          sidebarHtml += '<div class="diff-sidebar-file' + (f.idx === 0 ? ' active' : '') + '" data-file-idx="' + f.idx + '" title="' + escapeHtml(f.path) + '">'
            + inlineFileStatus(status)
            + '<span class="diff-sidebar-name">' + escapeHtml(f.fileName) + '</span>'
            + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
            + '</div>';
        }
        sidebarHtml += '</details>';
      }
      sidebarHtml += '</div>';

      // Build file section shells
      let sectionsHtml = '';
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        const status = ['added', 'modified', 'deleted', 'renamed'].includes(f.status) ? f.status : 'modified';
        const adds = f.additions || 0;
        const dels = f.deletions || 0;
        sectionsHtml += '<div class="diff-file-section" data-file-idx="' + i + '" id="diff-file-' + i + '">'
          + '<div class="diff-file-header">'
          + '<span class="diff-file-chevron expanded">&#9656;</span>'
          + inlineFileStatus(status)
          + '<span class="diff-file-path">' + escapeHtml(f.path) + '</span>'
          + '<span class="diff-stats"><span class="diff-stats-add">+' + adds + '</span> <span class="diff-stats-del">-' + dels + '</span></span>'
          + '</div>'
          + '<div class="diff-file-body">' + inlineLoading() + '</div>'
          + '</div>';
      }

      // Summary bar
      const summaryHtml = '<div class="diff-summary-bar">'
        + '<span>Showing ' + files.length + ' changed file' + (files.length !== 1 ? 's' : '')
        + ' with <span class="diff-stats-add">+' + totalAdds + '</span> addition' + (totalAdds !== 1 ? 's' : '')
        + ' and <span class="diff-stats-del">-' + totalDels + '</span> deletion' + (totalDels !== 1 ? 's' : '') + '</span>'
        + '<div class="diff-controls">'
        + inlineToggleSwitch('Wrap', 'diff-wrap-lines', sessionStorage.getItem('diff-wrap-lines') === 'true')
        + '<div class="diff-view-toggle">'
        + '<button class="diff-toggle-btn' + (viewMode === 'side-by-side' ? ' active' : '') + '" data-mode="side-by-side">Split</button>'
        + '<button class="diff-toggle-btn' + (viewMode === 'line-by-line' ? ' active' : '') + '" data-mode="line-by-line">Unified</button>'
        + '</div></div></div>';

      container.innerHTML = '<div class="diff-view">'
        + summaryHtml
        + '<div class="diff-layout">' + sidebarHtml
        + '<div class="diff-content" id="diff-content">' + sectionsHtml + '</div>'
        + '</div></div>';
      markLoaded();

      // Lazy render with IntersectionObserver
      const rendered = new Set();
      const diffContent = document.getElementById('diff-content');

      function renderSingleFile(bodyEl, idx) {
        const chunk = fileChunks[idx];
        if (!chunk) { bodyEl.innerHTML = ''; return; }
        const lines = chunk.split('\n');
        const maxLines = 2000;
        const truncated = lines.length > maxLines;
        const renderChunk = truncated ? lines.slice(0, maxLines).join('\n') : chunk;
        bodyEl.innerHTML = '';
        try {
          const ui = new Diff2HtmlUI(bodyEl, renderChunk, {
            drawFileList: false,
            matching: 'words',
            outputFormat: viewMode,
            colorScheme: Theme.getEffective(),
            highlight: true,
          });
          ui.draw();
          requestAnimationFrame(() => {
            if (!signal.aborted) {
              try { ui.highlightCode(); } catch { /* ignore */ }
            }
          });
          if (truncated) {
            bodyEl.insertAdjacentHTML('beforeend',
              '<div style="padding:12px 16px;color:var(--text-secondary);font-size:13px;border-top:1px solid var(--border-default)">'
              + 'Showing first ' + maxLines + ' lines of ' + lines.length + ' total</div>');
          }
        } catch {
          bodyEl.innerHTML = '<div class="empty-state"><div class="empty-state-title">Diff too large to render</div></div>';
        }
      }

      const lazyObserver = new IntersectionObserver((entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const idx = parseInt(entry.target.dataset.fileIdx, 10);
            if (!rendered.has(idx)) {
              rendered.add(idx);
              const body = entry.target.querySelector('.diff-file-body');
              if (body && body.style.display !== 'none') renderSingleFile(body, idx);
              lazyObserver.unobserve(entry.target);
            }
          }
        }
      }, { root: diffContent, rootMargin: '200px' });

      container.querySelectorAll('.diff-file-section').forEach(el => {
        lazyObserver.observe(el);
      });

      // Scroll spy with debounce for sidebar clicks
      let ignoreSpyUntil = 0;
      const spyObserver = new IntersectionObserver((entries) => {
        if (Date.now() < ignoreSpyUntil) return;
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const idx = entry.target.dataset.fileIdx;
            container.querySelectorAll('.diff-sidebar-file').forEach(f => {
              f.classList.toggle('active', f.dataset.fileIdx === idx);
            });
          }
        }
      }, { root: diffContent, threshold: 0.1 });

      container.querySelectorAll('.diff-file-section').forEach(el => {
        spyObserver.observe(el);
      });

      // Clean up observers on navigation
      signal.addEventListener('abort', () => {
        lazyObserver.disconnect();
        spyObserver.disconnect();
      }, { once: true });

      // File header collapse/expand
      container.querySelectorAll('.diff-file-header').forEach(header => {
        header.addEventListener('click', () => {
          const section = header.parentElement;
          const body = section.querySelector('.diff-file-body');
          const chevron = header.querySelector('.diff-file-chevron');
          const isCollapsed = body.style.display === 'none';
          body.style.display = isCollapsed ? '' : 'none';
          chevron.classList.toggle('expanded', isCollapsed);
          // Trigger lazy render if expanding an unrendered file
          if (isCollapsed) {
            const idx = parseInt(section.dataset.fileIdx, 10);
            if (!rendered.has(idx)) {
              rendered.add(idx);
              renderSingleFile(body, idx);
            }
          }
        });
      });

      // Sidebar click → scroll to file
      container.querySelectorAll('.diff-sidebar-file').forEach(el => {
        el.addEventListener('click', () => {
          ignoreSpyUntil = Date.now() + 600;
          container.querySelectorAll('.diff-sidebar-file').forEach(f => f.classList.remove('active'));
          el.classList.add('active');
          const idx = parseInt(el.dataset.fileIdx, 10);
          const section = document.getElementById('diff-file-' + idx);
          if (section) {
            // Expand if collapsed
            const body = section.querySelector('.diff-file-body');
            if (body && body.style.display === 'none') {
              body.style.display = '';
              section.querySelector('.diff-file-chevron').classList.add('expanded');
              if (!rendered.has(idx)) {
                rendered.add(idx);
                renderSingleFile(body, idx);
              }
            }
            // Scroll within the diff-content container, not the whole page
            const dc = document.getElementById('diff-content');
            if (dc) {
              dc.scrollTop = section.offsetTop - dc.offsetTop;
            }
          }
        });
      });

      // Unified/split toggle
      container.querySelectorAll('.diff-toggle-btn').forEach(btn => {
        btn.addEventListener('click', () => {
          const mode = btn.dataset.mode;
          if (mode === viewMode) return;
          viewMode = mode;
          localStorage.setItem('diff-view-mode', mode);
          container.querySelectorAll('.diff-toggle-btn').forEach(b => b.classList.toggle('active', b.dataset.mode === mode));
          // Re-render only expanded files
          rendered.clear();
          container.querySelectorAll('.diff-file-section').forEach(section => {
            const body = section.querySelector('.diff-file-body');
            if (body && body.style.display !== 'none') {
              body.innerHTML = inlineLoading();
              lazyObserver.observe(section);
            }
          });
        });
      });

      // Wrap toggle
      const wrapInput = container.querySelector('.toggle-switch__input[data-key="diff-wrap-lines"]');
      if (wrapInput && diffContent) {
        if (sessionStorage.getItem('diff-wrap-lines') === 'true') diffContent.classList.add('diff-wrap');
        wrapInput.addEventListener('change', () => {
          diffContent.classList.toggle('diff-wrap', wrapInput.checked);
          sessionStorage.setItem('diff-wrap-lines', wrapInput.checked);
        });
      }

      break;
    }
    case 'plan': {
      const data = await get('/api/agents/' + agentId + '/plan');
      if (signal.aborted) return;
      if (!data || !data.content) {
        container.innerHTML = inlineEmptyState(ICONS.clipboard, 'No plan available', 'Plans appear when the agent outlines its approach before executing.');
        markLoaded();
        return;
      }
      container.innerHTML = '<div class="plan-content">' + renderMarkdown(data.content) + '</div>';
      markLoaded();
      break;
    }
  }
}

function renderToolGroup(tools) {
  if (tools.length === 0) return '';
  const first = tools[0];
  const time = first.Timestamp || first.timestamp || '';
  let html = '<div class="timeline-entry activity-tool-group" data-kind="tool">';
  html += timelineIcon('tool');
  html += '<div class="timeline-content">';
  html += `<details><summary class="tool-group-summary"><span class="timeline-title">${tools.length} tool call${tools.length !== 1 ? 's' : ''}</span><span class="timeline-timestamp">${formatTimeShort(time)}</span></summary>`;
  for (const entry of tools) {
    const content = entry.Content || entry.content || '';
    const toolName = entry.ToolName || entry.tool_name || '';
    const truncated = content.length > 200;
    const display = truncated ? escapeHtml(content.substring(0, 200)) + '...' : escapeHtml(content);
    html += `<div class="tool-call">`;
    html += `<div class="tool-call__name">${escapeHtml(toolName)}</div>`;
    html += `<div class="timeline-detail">${display}</div>`;
    if (truncated) {
      html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
      html += `<button class="btn btn-ghost btn-sm" onclick="Dashboard.toggleExpand(this)">Show more</button>`;
    }
    html += '</div>';
  }
  html += '</details></div></div>';
  return html;
}
