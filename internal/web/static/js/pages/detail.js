// Agent detail view with tabs and inline subagents.
import { UI } from '../ui.js';
import { ICONS } from '../icons.js';
import { effectiveState, stateGroup, prTag, hasOpenPR } from '../state.js';
import { escapeHtml, repoName, duration, durationFromTimestamp, formatTime, formatTimeShort, formatCost, formatTokens, renderMarkdown, skeletonLoading } from '../format.js';
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
  pr: 'PR open',
  merged: 'Merged',
  done: 'Done',
  idle_prompt: 'Idle',
  blocked: 'Blocked',
  waiting: 'Waiting',
  queued: 'Queued',
  review: 'Review',
  failed: 'Failed',
  completed: 'Completed',
};

function inlineStatusPill(state) {
  const group = stateGroup(state).toLowerCase();
  const label = STATE_LABELS[state] || (state ? state.charAt(0).toUpperCase() + state.slice(1) : 'Unknown');
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
function actionBarSignature(agent) {
  return [
    effectiveState(agent),
    hasOpenPR(agent) ? '1' : '0',
    agent.model || '',
    agent.branch || '',
    agent.effort || '',
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
  if (bar.dataset.sig === sig) return;

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
  if (newInput) attachSlashAutocomplete(newInput);
}

// Track optimistic messages so refreshConversation can preserve them
// across the 2s poll until the API echoes the user's message back.
//
// pendingUserMessage  — the text of the in-flight user message
// pendingMessageAcked — false until POST /input resolves OK; once true
//                       the conversation refresh stops re-rendering the
//                       "Sending…" caption (the message is delivered;
//                       only the API echo is still pending).
let pendingUserMessage = null;
let pendingMessageAcked = false;

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

// Optimistically append a Codex-style user message pill to the chat.
// While in flight (pre-POST-ack) the bubble carries .ui-msg--optimistic
// and is followed by a "Sending…" caption sibling. Dashboard.sendInput
// clears the flag (and removes the caption) once the POST resolves OK.
export function appendUserMessage(text) {
  pendingUserMessage = text;
  pendingMessageAcked = false;
  const container = document.querySelector('#tab-conversation .conversation');
  if (!container) return;
  const wrap = document.createElement('div');
  wrap.innerHTML = UI.message('user', text);
  const msgEl = wrap.firstElementChild;
  if (msgEl) {
    msgEl.classList.add('ui-msg--optimistic');
    container.appendChild(msgEl);
    const caption = document.createElement('div');
    caption.className = 'ui-msg__caption ui-msg__caption--sending';
    caption.textContent = 'Sending…';
    container.appendChild(caption);
  }
  const scrollParent = container.closest('.detail-scroll');
  if (scrollParent) scrollParent.scrollTop = scrollParent.scrollHeight;
}

// Called by Dashboard.sendInput when POST resolves OK. Three jobs:
//   1. Lift the in-flight affordance from the optimistic bubble.
//   2. Reset the per-turn tool tally — a new turn just started.
//   3. Optimistically mount the Thinking indicator so the user sees
//      feedback immediately, without waiting for the next SSE tick
//      (~1-2s after POST). The next SSE update confirms or replaces.
export function confirmUserMessageSent() {
  pendingMessageAcked = true;
  const container = document.querySelector('#tab-conversation .conversation');
  if (!container) return;
  container.querySelectorAll('.ui-msg--optimistic').forEach(el => el.classList.remove('ui-msg--optimistic'));
  container.querySelectorAll('.ui-msg__caption--sending').forEach(el => el.remove());
  // New turn — reset tally and the seen-tool watermark so only tools
  // fired AFTER this moment count for this turn.
  toolBuckets = {};
  lastSeenToolTimestamp = new Date().toISOString();
  // Mount the indicator optimistically so the user doesn't stare at
  // an empty chat while SSE catches up. PERSIST the synthetic mid-turn
  // override onto lastKnownAgent so the 2s conversation poll (which
  // also calls refreshWorkingIndicator) doesn't wipe the indicator
  // before SSE has had a chance to report PreToolUse/PostToolUse. SSE
  // updates will overwrite lastKnownAgent with real hook events; the
  // indicator only disappears once last_hook_event === 'Stop'.
  if (lastKnownAgent) {
    lastKnownAgent = {
      ...lastKnownAgent,
      last_hook_event: 'UserPromptSubmit',
      current_tool: '',
    };
    refreshWorkingIndicator(lastKnownAgent);
  }
}

// Last-known agent for the currently-mounted detail view. Used by the
// 2s conversation poll (which doesn't carry an agent reference) so that
// the rebuilt .conversation can re-mount the working indicator.
let lastKnownAgent = null;

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
  return { bucket: 'other', live: 'Running ' + n };
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

// Detect whether the agent is BETWEEN turns. agent.state stays
// "running" for the life of the tmux pane, so it's a coarse signal —
// the real per-turn signal is last_hook_event, which flips to "Stop"
// when Claude Code finishes a turn (matching the Stop hook event).
function isAgentMidTurn(agent) {
  if (!agent) return false;
  if (!WORKING_STATES.has(effectiveState(agent))) return false;
  // "Stop" = turn ended, awaiting user. Anything else (PreToolUse,
  // PostToolUse, UserPromptSubmit, etc.) = mid-turn.
  const hook = agent.last_hook_event || '';
  return hook !== 'Stop';
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
  const classified = classifyTool(agent.current_tool);
  const liveLabel = classified ? classified.live : 'Thinking';
  const tally = renderToolTally(toolBuckets);
  const tallyHtml = tally
    ? '<div class="ui-msg-status__tally">' + escapeHtml(tally) + '</div>'
    : '';
  // Latest activity line — shows what the agent most recently *finished*.
  // Strips the "→ Tool: " prefix and the long arg tail; e.g.
  //   "→ Bash: ls -la /Users/bjornjee/Code/bjornjee/worktrees/..."
  // renders as "Bash · ls -la /Users/bjornjee/Code/…"
  let latestHtml = '';
  if (latestToolEntry) {
    const raw = String(latestToolEntry.content || '').replace(/^→\s*/, '');
    const m = raw.match(/^([^:]+):\s*(.*)$/);
    const tool = m ? m[1].trim() : raw;
    const arg = m ? m[2].trim() : '';
    const c = classifyTool(tool);
    const friendly = c ? c.live : tool;
    const argSnip = arg.length > 64 ? arg.slice(0, 62) + '…' : arg;
    const display = argSnip ? friendly + ' · ' + argSnip : friendly;
    latestHtml = '<div class="ui-msg-status__latest">' + escapeHtml(display) + '</div>';
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
    const el = document.createElement('div');
    el.className = 'ui-msg-status ui-msg-status--working';
    el.innerHTML = html;
    container.appendChild(el);
    // Only pull the indicator into view if the user was already at the
    // bottom. refreshConversation re-runs every 2s and wipes the
    // indicator (innerHTML rewrite), so this mount path fires every
    // poll while an agent is working — an unconditional scroll here
    // overrides whatever scroll position the user just chose.
    if (scrollParent && wasAtBottom) scrollParent.scrollTop = scrollParent.scrollHeight;
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
      // message timestamp) or by confirmUserMessageSent() on a fresh send.
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

function kindLabel(kind) {
  if (kind === 'human') return 'You';
  if (kind === 'assistant') return 'Claude';
  return 'Tool';
}

function renderActionBar(agent) {
  const st = effectiveState(agent);
  const id = agent.session_id;
  let actions = '';
  let panelLabel = '';

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
  // regardless of the agent's terminal state. The stop button only appears
  // while the agent is actively processing; otherwise the send button.
  const STOP_STATES = new Set(['running', 'permission', 'plan', 'question', 'error']);
  const placeholder = (st === 'question' || st === 'error') ? 'Type a reply…'
    : (STOP_STATES.has(st) ? 'Message' : 'Ask for follow-up changes…');
  const trailing = STOP_STATES.has(st)
    ? `<button class="ui-composer__stop" aria-label="Stop" onclick="Dashboard.confirmStop('${id}')"><span></span></button>`
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
  return `<div class="action-bar">${actionRow}${composer}</div>`;
}

let activityFilter = 'all';
let currentPRUrl = '';
let currentDetailTab = 'conversation';
let currentDetailAgentId = null;
let lastAgentState = null;
let conversationPollTimer = null;

// Build conversation HTML from an array of message entries — Codex flat-prose.
function renderConversationHtml(entries) {
  let html = '<div class="conversation">';
  for (const entry of entries) {
    // Skip task-notification messages (internal agent-to-agent noise)
    if (entry.IsNotification) continue;
    const role = entry.Role || entry.role;
    const content = entry.Content || entry.content || '';
    if (!content) continue;
    if (role === 'human') {
      html += UI.message('user', content);
    } else {
      // Assistant prose with rendered markdown — keep HTML, don't escape again
      const body = renderMarkdown(content);
      html += UI.message('assistant', body, { html: true });
    }
  }
  html += '</div>';
  return html;
}

// Signature of every field renderQuestionCard() reads. If this string is
// unchanged across a poll tick, the rebuilt card would be byte-identical
// — and rebuilding anyway would wipe the user's picked radio / typed
// freeform text. refreshConversation uses this to detach-and-re-attach
// the same DOM node across the container.innerHTML wipe. Same pattern
// as actionBarSignature (commit 8106661).
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

// Anatomy matches docs/design/codex-screenshots/mobile/photo_2026-06-01_17-44-47.jpg —
// elevated surface, per-question small-caps category label, radio list with
// bold title + muted description, optional freeform answer input, single
// white submit chip. Submission posts the composed answer to the existing
// /input endpoint; the agent reads it as the user's next message and the
// card disappears on the next poll once HasPendingQuestion clears.
function renderQuestionCard(pending, agentId) {
  if (!pending || !Array.isArray(pending.questions) || pending.questions.length === 0) return '';
  const tid = escapeHtml(pending.tool_use_id || '');
  const sig = escapeHtml(questionCardSignature(pending));
  const blocks = pending.questions.map((q, qi) => {
    const header = q.header ? `<div class="question-card__label">${escapeHtml(q.header)}</div>` : '';
    const text = q.question ? `<div class="question-card__question">${escapeHtml(q.question)}</div>` : '';
    const inputType = q.multi_select ? 'checkbox' : 'radio';
    const name = `qc-${qi}`;
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
    return `<div class="question-card__block" data-qi="${qi}">
      ${header}
      ${text}
      <div class="question-card__options">${opts}</div>
      <div class="question-card__label question-card__label--answer">Answer</div>
      <input type="text" id="${freeId}" name="qc-free-${qi}" class="question-card__answer-input" placeholder="Type a response" oninput="window.Dashboard.questionCardUpdate('${tid}')" />
    </div>`;
  }).join('');
  const submitId = `qc-submit-${tid}`;
  return `<div class="question-card" data-tool-use-id="${tid}" data-sig="${sig}" data-agent-id="${escapeHtml(agentId)}">
    ${blocks}
    <div class="question-card__footer">
      <button type="button" id="${submitId}" class="question-card__submit" disabled onclick="window.Dashboard.answerQuestion('${escapeHtml(agentId)}', '${tid}', event)">Send answer</button>
    </div>
  </div>`;
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

// Collect the composed answer text from a question card and POST it to
// the agent's input endpoint. Format mirrors the TUI's serialisation:
// one block per question, header + question text + picked options +
// freeform text, separated by blank lines.
export async function submitQuestionCard(agentId, toolUseId) {
  const card = document.querySelector(`.question-card[data-tool-use-id="${cssEscape(toolUseId)}"]`);
  if (!card) return false;
  const btn = card.querySelector('.question-card__submit');
  if (btn) btn.disabled = true;

  const blocks = card.querySelectorAll('.question-card__block');
  const parts = [];
  blocks.forEach((block) => {
    const headerEl = block.querySelector('.question-card__label:not(.question-card__label--answer)');
    const qEl = block.querySelector('.question-card__question');
    const header = headerEl ? headerEl.textContent.trim() : '';
    const question = qEl ? qEl.textContent.trim() : '';
    const picks = Array.from(block.querySelectorAll('.question-card__radio-input:checked'))
      .map(i => i.value);
    const free = block.querySelector('.question-card__answer-input');
    const freeText = free && free.value.trim() ? free.value.trim() : '';
    let body = '';
    if (picks.length) body += picks.join(', ');
    if (freeText) body += (body ? ' — ' : '') + freeText;
    const label = header ? `${header}: ${question}` : question;
    parts.push(`${label}\n${body}`);
  });
  const text = parts.join('\n\n');

  // Optimistic transition: collapse to "answered" snapshot so the user
  // sees their submission landed even before the polling loop catches up.
  card.classList.add('question-card--answered');

  try {
    const result = await post('/api/agents/' + agentId + '/input', { text });
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

// Re-fetch and re-render the conversation tab if it is currently active.
// Called by the SSE handler to keep the chat view up to date.
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

  // Check if the API has caught up with our optimistic message
  if (pendingUserMessage) {
    const lastHuman = [...entries].reverse().find(e => (e.Role || e.role) === 'human');
    const lastContent = lastHuman ? (lastHuman.Content || lastHuman.content || '') : '';
    if (lastContent.includes(pendingUserMessage)) {
      pendingUserMessage = null;       // API caught up, clear optimistic state
      pendingMessageAcked = false;
    }
  }

  // Preserve the AskUserQuestion card DOM node across the container wipe
  // when the pending question is unchanged. Rebuilding the card every 2s
  // poll detaches its event listeners and resets any picked option or
  // freeform text the user was filling in — making the card feel
  // "unclickable". Detach first so innerHTML doesn't destroy it, then
  // re-attach the same Node so checked radios / focus survive.
  const existingCard = container.querySelector('.question-card');
  const sig = pending ? questionCardSignature(pending) : '';
  const reuseCard = !!(pending && pending.tool_use_id && existingCard
    && existingCard.dataset.toolUseId === pending.tool_use_id
    && existingCard.dataset.sig === sig);
  if (reuseCard) existingCard.remove();

  container.innerHTML = renderConversationHtml(entries);

  // Append the AskUserQuestion card inline after the last assistant
  // message when one is pending. The card is part of the chat stream,
  // not the action bar — visitors keep prior context in view.
  if (pending && pending.tool_use_id) {
    const conv = container.querySelector('.conversation');
    if (conv) {
      if (reuseCard) {
        conv.appendChild(existingCard);
      } else {
        const wrap = document.createElement('div');
        wrap.innerHTML = renderQuestionCard(pending, agentId);
        const cardEl = wrap.firstElementChild;
        if (cardEl) conv.appendChild(cardEl);
      }
    }
  }

  // Re-append optimistic message if API hasn't caught up yet. The
  // "Sending…" caption (and the muted opacity) ONLY apply while the
  // POST is still in flight (pendingMessageAcked === false). After ack
  // the bubble re-renders at full opacity, no caption — just a normal
  // user message waiting for the API echo.
  if (pendingUserMessage) {
    const conv = container.querySelector('.conversation');
    if (conv) {
      const wrap = document.createElement('div');
      wrap.innerHTML = UI.message('user', pendingUserMessage);
      const msgEl = wrap.firstElementChild;
      if (msgEl) {
        if (!pendingMessageAcked) msgEl.classList.add('ui-msg--optimistic');
        conv.appendChild(msgEl);
        if (!pendingMessageAcked) {
          const caption = document.createElement('div');
          caption.className = 'ui-msg__caption ui-msg__caption--sending';
          caption.textContent = 'Sending…';
          conv.appendChild(caption);
        }
      }
    }
  }

  // Re-mount the working indicator if the agent is processing — the
  // innerHTML rewrite above wiped any previous indicator.
  if (agent) refreshWorkingIndicator(agent);

  if (scrollParent && wasAtBottom) scrollParent.scrollTop = scrollParent.scrollHeight;
}

// Poll conversation every 2s while the chat tab is active.
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
  }, 2000);
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

  // Update duration
  const meta = document.querySelector('.detail-meta');
  if (meta && agent.started_at) {
    const spans = meta.querySelectorAll('span');
    const last = spans[spans.length - 1];
    if (last) last.textContent = duration(agent);
  }

  // Refresh vital signs only on state change
  if (prev !== null && prev !== st) {
    loadVitalSigns(agent.session_id, agent);
    loadSubagentSummary(agent.session_id);
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
  const subline = [branchPart, modelPart, durationPart].filter(Boolean).join(' · ');

  const appBar = UI.appBar({
    back: true,
    title: repoName(agent),
    subtitle: subline,
    trailing: [
      ...(st === 'running' ? ['spinner'] : []),
      { icon: ICONS.kebab, ariaLabel: 'More', onclick: 'Dashboard.openKebab()' },
    ],
  });

  // Suppress the tag chip in the detail header when the status pill
  // already says "PR open" (state === 'pr', the idle case). Sidebar/list
  // rows have no status pill, so they always show the tag.
  const prChip = (prTag(agent) && st !== 'pr')
    ? `<span class="ui-row__tag detail-header__tag">${escapeHtml(prTag(agent))}</span>`
    : '';
  const detailHeader = `
    <div class="detail-header">
      <div class="detail-title">${inlineStatusPill(st)}${prChip}</div>
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
  const subagentOpen = !isMobile && sessionStorage.getItem('collapse-subagent-summary-' + agentId) !== 'true';
  const activeCls = (key) => key === savedTab ? ' active' : '';

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
          ${inlineDisclosure('subagent-summary', 'Subagents', subagentOpen)}
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

  // Load initial tab + subagents + vital signs in parallel
  loadTabContent(savedTab, agentId);
  loadSubagentSummary(agentId);
  loadVitalSigns(agentId, agent);

  // Mount the working indicator if the agent is currently processing.
  // loadTabContent populates .conversation asynchronously so defer the mount.
  lastKnownAgent = agent;
  seedTallyFromTurnBoundary(agentId).then(() => {
    refreshWorkingIndicator(agent);
  });

  // Wire slash-command autocomplete to the composer textarea.
  const composerInput = document.getElementById('reply-input');
  if (composerInput) attachSlashAutocomplete(composerInput);

  // Start conversation polling only when the conversation tab is active.
  if (savedTab === 'conversation') startConversationPoll(agentId);
}

async function loadVitalSigns(agentId, agent) {
  const container = document.getElementById('vital-signs-container');
  if (!container) return;
  try {
    const usage = await get('/api/agents/' + agentId + '/usage');
    const elapsed = agent.started_at ? duration(agent) : '';
    container.innerHTML = inlineVitalStrip({
      elapsed: elapsed,
      tokens: (usage && usage.InputTokens ? usage.InputTokens + (usage.OutputTokens || 0) : 0),
      cost: usage ? usage.CostUSD : 0,
    });
  } catch {
    container.innerHTML = '';
  }
}

async function loadSubagentSummary(agentId) {
  const container = document.getElementById('subagent-summary');
  if (!container) return;
  let subs;
  try {
    subs = await get('/api/agents/' + agentId + '/subagents');
  } catch {
    container.innerHTML = '';
    const section = container.closest('.collapsible-section');
    if (section) section.style.display = 'none';
    return;
  }
  const section = container.closest('.collapsible-section');
  if (!subs || subs.length === 0) {
    container.innerHTML = '';
    if (section) section.style.display = 'none';
    return;
  }
  if (section) section.style.display = '';

  const completed = subs.filter(s => s.Completed || s.completed).length;
  const running = subs.length - completed;

  const MAX_VISIBLE = 3;
  const visible = subs.slice(-MAX_VISIBLE);
  const hidden = subs.length - visible.length;

  let html = '';

  html += '<div class="subagent-summary-list">';
  if (hidden > 0) {
    html += `<div class="subagent-pill subagent-pill--muted"><span class="subagent-type">+${hidden} more</span></div>`;
  }
  for (const sub of visible) {
    const isDone = sub.Completed || sub.completed;
    const type = sub.AgentType || sub.agent_type || 'agent';
    const desc = sub.InstructionHead || sub.instruction_head || sub.Description || sub.description || '';
    const mode = sub.Mode || sub.mode || '';
    const startedAt = sub.StartedAt || sub.started_at || '';
    const dotClass = isDone ? 'status-dot--completed' : 'status-dot--running';
    html += `<div class="subagent-pill">`;
    html += `<span class="status-dot ${dotClass}"></span>`;
    html += `<span class="subagent-type">${escapeHtml(type)}</span>`;
    if (desc) html += `<span class="subagent-desc">${escapeHtml(desc)}</span>`;
    if (mode) html += `<span class="subagent-mode">${escapeHtml(mode)}</span>`;
    if (startedAt) html += `<span class="subagent-time">${durationFromTimestamp(startedAt)}</span>`;
    html += '</div>';
  }
  html += '</div>';

  container.innerHTML = html;
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
        container.innerHTML = inlineEmptyState(ICONS.chat, 'No conversation yet', 'Messages will appear here once the agent starts');
        markLoaded();
        return;
      }
      container.innerHTML = renderConversationHtml(entries);
      if (pending && pending.tool_use_id) {
        const conv = container.querySelector('.conversation');
        if (conv) {
          const wrap = document.createElement('div');
          wrap.innerHTML = renderQuestionCard(pending, agentId);
          const cardEl = wrap.firstElementChild;
          if (cardEl) conv.appendChild(cardEl);
        }
      }
      markLoaded();
      // Only snap to bottom on the *first* conversation load of this
      // detail session — subsequent tab-switches back to Conversation
      // re-render the same content and should preserve the user's
      // scroll position. renderDetail() resets the flag when a new agent
      // detail view mounts.
      if (!conversationScrolledThisSession) {
        const scrollParent = container.closest('.detail-scroll');
        if (scrollParent) scrollParent.scrollTop = scrollParent.scrollHeight;
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

          const truncated = content.length > 200;
          const displayContent = truncated ? content.substring(0, 200) + '...' : content;
          html += `<div class="timeline-entry activity-entry" data-kind="${kind}">`;
          html += timelineIcon(kind);
          html += '<div class="timeline-content">';
          html += `<div class="timeline-header"><span class="timeline-title">${kindLabel(kind)}</span><span class="timeline-timestamp">${formatTimeShort(time)}</span></div>`;
          if (kind === 'assistant') {
            html += `<div class="timeline-detail">${renderMarkdown(displayContent)}</div>`;
          } else {
            html += `<div class="timeline-detail">${escapeHtml(displayContent)}</div>`;
          }
          if (truncated) {
            html += `<span data-full="${escapeHtml(content)}" data-truncated="true" style="display:none"></span>`;
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
        container.innerHTML = inlineEmptyState(ICONS.clipboard, 'No plan available', 'Plans appear when the agent outlines its approach before executing');
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
