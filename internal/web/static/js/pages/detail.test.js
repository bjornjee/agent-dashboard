// Unit tests for pure helpers in pages/detail.js.
// DOM-bound code lives in refreshWorkingIndicator / renderDetail and is
// exercised by the Playwright suite.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let questionCardId;
let renderQuestionCard;
let formatLatestToolDisplay;
let isAgentMidTurn;
let getMessageCopyText;
let renderActionBar;
let actionBarSignature;

test('load module', async () => {
  // Stub a minimal global `document` so any module-level click delegation
  // setup in detail.js doesn't crash under node:test (no jsdom).
  if (typeof globalThis.document === 'undefined') {
    globalThis.document = { addEventListener() {} };
  }
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  questionCardId = mod.questionCardId;
  renderQuestionCard = mod.renderQuestionCard;
  formatLatestToolDisplay = mod.formatLatestToolDisplay;
  isAgentMidTurn = mod.isAgentMidTurn;
  getMessageCopyText = mod.getMessageCopyText;
  renderActionBar = mod.renderActionBar;
  actionBarSignature = mod.actionBarSignature;
  assert.equal(typeof questionCardId, 'function');
  assert.equal(typeof renderQuestionCard, 'function');
  assert.equal(typeof formatLatestToolDisplay, 'function');
  assert.equal(typeof isAgentMidTurn, 'function');
  assert.equal(typeof getMessageCopyText, 'function');
  assert.equal(typeof renderActionBar, 'function');
  assert.equal(typeof actionBarSignature, 'function');
});

test('questionCardId prefers real tool_use_id', () => {
  const pending = {
    tool_use_id: 'toolu_123',
    questions: [{ question: 'Which?', options: [{ label: 'A' }] }],
  };
  assert.equal(questionCardId(pending), 'toolu_123');
});

test('questionCardId is stable when tool_use_id is blank', () => {
  const pending = {
    tool_use_id: '',
    questions: [{
      question: 'Plan has 6 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Hand off to /agent-dashboard:implement (Recommended for 6+ phases)', description: 'Use implementation dispatch.' },
        { label: 'Continue inline', description: 'Stay here.' },
      ],
    }],
  };
  assert.equal(questionCardId(pending), questionCardId({ ...pending }));
  assert.match(questionCardId(pending), /^qc-[a-z0-9]+$/);
});

test('renderQuestionCard renders a card when tool_use_id is blank', () => {
  const pending = {
    tool_use_id: '',
    questions: [{
      question: 'Plan has 6 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Hand off to /agent-dashboard:implement (Recommended for 6+ phases)', description: 'Use implementation dispatch.' },
        { label: 'Continue inline', description: 'Stay here.' },
      ],
    }],
  };

  const html = renderQuestionCard(pending, 'agent-1');
  assert.match(html, /class="question-card"/);
  assert.match(html, /data-tool-use-id="[^"]+"/);
  assert.match(html, /Plan has 6 phases/);
});

test('formatLatestToolDisplay returns empty for null/empty entry', () => {
  assert.equal(formatLatestToolDisplay(null), '');
  assert.equal(formatLatestToolDisplay({}), '');
  assert.equal(formatLatestToolDisplay({ content: '' }), '');
});

test('formatLatestToolDisplay renders a normal Bash command with friendly verb', () => {
  const entry = { content: '→ Bash: ls -la /tmp' };
  assert.equal(formatLatestToolDisplay(entry), 'Running command · ls -la /tmp');
});

test('formatLatestToolDisplay truncates long single-line args at 64 chars', () => {
  const long = 'echo ' + 'x'.repeat(80);
  const entry = { content: '→ Bash: ' + long };
  const out = formatLatestToolDisplay(entry);
  // Header + middot + truncated arg (62 chars + ellipsis)
  assert.ok(out.startsWith('Running command · '));
  const arg = out.slice('Running command · '.length);
  assert.equal(arg.length, 63); // 62 + '…'
  assert.ok(arg.endsWith('…'));
});

test('formatLatestToolDisplay drops the JS payload for playwright browser tools', () => {
  const entry = {
    content: "→ mcp__plugin_playwright__browser_click: { element: 'Submit', ref: 'e123' }",
  };
  // Browser bucket: arg dropped, tool method name surfaced instead.
  assert.equal(
    formatLatestToolDisplay(entry),
    'Driving browser · browser_click',
  );
});

test('formatLatestToolDisplay also handles the mcp__playwright (no plugin_) prefix', () => {
  const entry = {
    content: '→ mcp__playwright__browser_take_screenshot: {}',
  };
  assert.equal(
    formatLatestToolDisplay(entry),
    'Driving browser · browser_take_screenshot',
  );
});

test('formatLatestToolDisplay shows <inline code> for arrow-function evaluate payloads', () => {
  const entry = {
    content: "→ Bash: () => { return document.querySelectorAll('button').length; }",
  };
  assert.equal(formatLatestToolDisplay(entry), 'Running command · <inline code>');
});

test('formatLatestToolDisplay shows <inline code> for function() payloads', () => {
  const entry = {
    content: '→ Bash: function() { return 1; }',
  };
  assert.equal(formatLatestToolDisplay(entry), 'Running command · <inline code>');
});

test('formatLatestToolDisplay shows <inline code> for multi-line bash heredocs', () => {
  const entry = {
    content: '→ Bash: cat <<EOF\nhello\nworld\nEOF',
  };
  assert.equal(formatLatestToolDisplay(entry), 'Running command · <inline code>');
});

test('formatLatestToolDisplay uses a generic verb for unclassified tools', () => {
  const entry = { content: '→ SomethingNovel: arg here' };
  // classifyTool returns { bucket: 'other', live: 'Working' } — raw
  // internal tool names must never reach the chat copy.
  assert.equal(formatLatestToolDisplay(entry), 'Working · arg here');
});

// isAgentMidTurn must be deterministic on agent.state — independent of
// last_hook_event. Otherwise the inline working indicator lingers after
// the agent leaves a working state but before/without a Stop hook event.
test('isAgentMidTurn returns false for null agent', () => {
  assert.equal(isAgentMidTurn(null), false);
  assert.equal(isAgentMidTurn(undefined), false);
});

test('isAgentMidTurn returns true while state is running, regardless of hook', () => {
  assert.equal(isAgentMidTurn({ state: 'running' }), true);
  assert.equal(isAgentMidTurn({ state: 'running', last_hook_event: 'PostToolUse' }), true);
  assert.equal(isAgentMidTurn({ state: 'running', last_hook_event: '' }), true);
});

test('isAgentMidTurn returns false once state leaves WORKING_STATES — Stop hook NOT required', () => {
  // The bug: spinner would persist while last_hook_event was anything but 'Stop'.
  // After fix: state alone is the deterministic trigger.
  assert.equal(isAgentMidTurn({ state: 'done', last_hook_event: 'PostToolUse' }), false);
  assert.equal(isAgentMidTurn({ state: 'idle_prompt', last_hook_event: 'PreToolUse' }), false);
  assert.equal(isAgentMidTurn({ state: 'pr' }), false);
  assert.equal(isAgentMidTurn({ state: 'merged' }), false);
});

test('isAgentMidTurn keeps other working states (permission/plan/question/error) as mid-turn', () => {
  assert.equal(isAgentMidTurn({ state: 'permission' }), true);
  assert.equal(isAgentMidTurn({ state: 'plan' }), true);
  assert.equal(isAgentMidTurn({ state: 'question' }), true);
  assert.equal(isAgentMidTurn({ state: 'error' }), true);
});

// Pure DOM helper: read the assistant-message prose for the clipboard.
// The button delegate calls this with the .ui-msg__copy button element.
test('getMessageCopyText returns trimmed innerText from the ui-msg__prose sibling', () => {
  const mockBtn = {
    closest(sel) {
      assert.equal(sel, '.ui-msg__card');
      return {
        querySelector(s) {
          assert.equal(s, '.ui-msg__prose');
          return { innerText: '  Hello, world!\n\n  ' };
        },
      };
    },
  };
  assert.equal(getMessageCopyText(mockBtn), 'Hello, world!');
});

test('getMessageCopyText returns empty string when no ui-msg__card ancestor', () => {
  const mockBtn = { closest() { return null; } };
  assert.equal(getMessageCopyText(mockBtn), '');
});

test('getMessageCopyText returns empty string when no .ui-msg__prose inside', () => {
  const mockBtn = {
    closest() { return { querySelector() { return null; } }; },
  };
  assert.equal(getMessageCopyText(mockBtn), '');
});

// -- action-bar plan badge (PWA state-gap fix, Phase C) --

test('renderActionBar: surfaces PLAN chip when permission_mode=plan', () => {
  // Mid-planning: agent is running inside CC's plan_mode but ExitPlanMode
  // hasn't fired yet. State is 'running', so without the chip the user
  // has no visible signal that the agent is in plan mode at all.
  const agent = {
    session_id: 'a1',
    state: 'running',
    permission_mode: 'plan',
    branch: 'feat/x',
  };
  const html = renderActionBar(agent);
  assert.match(html, /class="chip chip--plan"/);
  assert.match(html, /aria-hidden="true">PLAN</);
  assert.match(html, /class="visually-hidden">agent is in plan mode</);
});

test('renderActionBar: surfaces PLAN chip when state=plan', () => {
  const agent = { session_id: 'a1', state: 'plan', branch: 'feat/x' };
  const html = renderActionBar(agent);
  assert.match(html, /class="chip chip--plan"/);
  assert.match(html, /aria-hidden="true">PLAN</);
});

test('renderActionBar: no PLAN chip in plain running state', () => {
  const agent = {
    session_id: 'a1',
    state: 'running',
    permission_mode: 'bypassPermissions',
  };
  const html = renderActionBar(agent);
  assert.doesNotMatch(html, /chip--plan/);
});

test('actionBarSignature: changes when permission_mode flips into/out of plan', () => {
  // SSE-driven re-render bails out when the signature is unchanged, so the
  // signature MUST include any field whose value affects the rendered HTML —
  // otherwise the plan chip never appears in production despite the source
  // change.
  const base = { session_id: 'a1', state: 'running', branch: 'feat/x' };
  const sigOff = actionBarSignature({ ...base, permission_mode: 'bypassPermissions' });
  const sigOn = actionBarSignature({ ...base, permission_mode: 'plan' });
  assert.notEqual(sigOff, sigOn);
});

test('blockedNoticeHtml: static notice per blocked-on-human state', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  const { blockedNoticeHtml } = mod;
  assert.equal(typeof blockedNoticeHtml, 'function');
  assert.match(blockedNoticeHtml('permission'), /Waiting for your approval/);
  assert.match(blockedNoticeHtml('plan'), /Plan ready for your review/);
  assert.match(blockedNoticeHtml('question'), /Waiting for your reply/);
  assert.match(blockedNoticeHtml('error'), /Stopped on an error/);
  // Not a shimmer line — the shimmer class must never appear in the
  // blocked notice, that treatment means "busy".
  for (const st of ['permission', 'plan', 'question', 'error']) {
    assert.doesNotMatch(blockedNoticeHtml(st), /ui-msg-status__label/);
  }
  // Non-blocked states produce nothing.
  assert.equal(blockedNoticeHtml('running'), '');
  assert.equal(blockedNoticeHtml('done'), '');
});

test('isAgentBlockedOnUser: blocked set is permission/plan/question/error', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  const { isAgentBlockedOnUser } = mod;
  assert.equal(typeof isAgentBlockedOnUser, 'function');
  for (const st of ['permission', 'plan', 'question', 'error']) {
    assert.equal(isAgentBlockedOnUser({ state: st }), true, st + ' should be blocked-on-user');
  }
  for (const st of ['running', 'done', 'pr', 'merged', 'idle_prompt']) {
    assert.equal(isAgentBlockedOnUser({ state: st }), false, st + ' should not be blocked-on-user');
  }
  assert.equal(isAgentBlockedOnUser(null), false);
});

test('planCardActionsVisible: only the final visible entry may arm a plan card', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  const { planCardActionsVisible } = mod;
  assert.equal(typeof planCardActionsVisible, 'function');
  const planEntry = { role: 'plan-saved', timestamp: '2026-06-04T10:02:00Z' };
  const human = { role: 'human', content: 'plan it' };
  const assistant = { role: 'assistant', content: 'done' };
  const running = { state: 'running' };

  // Mid-history plan card: transcript moved on — never armed.
  assert.equal(planCardActionsVisible([human, planEntry, assistant], 1, running), false);
  // Final entry + running (codex flow): armed.
  assert.equal(planCardActionsVisible([human, planEntry], 1, running), true);
  // Final entry but the action panel already shows Approve/Reject
  // (claude plan/permission states): suppressed — single button system.
  assert.equal(planCardActionsVisible([human, planEntry], 1, { state: 'plan' }), false);
  assert.equal(planCardActionsVisible([human, planEntry], 1, { state: 'permission' }), false);
  // Terminal state never re-arms.
  assert.equal(planCardActionsVisible([human, planEntry], 1, { state: 'merged' }), false);
  // Non-plan entries are never armed.
  assert.equal(planCardActionsVisible([human, assistant], 1, running), false);
  // Index out of final position.
  assert.equal(planCardActionsVisible([planEntry, human], 0, running), false);
});

test('visibleEntries: consecutive assistant messages stay separate blocks', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  const { visibleEntries } = mod;
  assert.equal(typeof visibleEntries, 'function');
  const entries = [
    { Role: 'human', Content: 'please explain', Timestamp: '2026-06-02T10:00:00Z' },
    { Role: 'assistant', Content: 'First message.', Timestamp: '2026-06-02T10:00:01Z' },
    { Role: 'assistant', Content: 'Second message.', Timestamp: '2026-06-02T10:00:02Z' },
    { Role: 'assistant', Content: 'Third message.', Timestamp: '2026-06-02T10:00:03Z' },
  ];
  const visible = visibleEntries(entries);
  // One block per backend entry — a turn must not collapse into one wall.
  assert.equal(visible.length, 4);
  assert.equal(visible[1].Content, 'First message.');
  assert.equal(visible[2].Content, 'Second message.');
  assert.equal(visible[3].Content, 'Third message.');
});

test('visibleEntries: still filters notifications and empty content', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  const { visibleEntries } = mod;
  const entries = [
    { Role: 'human', Content: 'hi' },
    { Role: 'assistant', Content: 'internal', IsNotification: true },
    { Role: 'assistant', Content: '' },
    { Role: 'plan-saved' },
    { Role: 'assistant', Content: 'reply' },
  ];
  const visible = visibleEntries(entries);
  assert.deepEqual(
    visible.map(e => e.Role),
    ['human', 'plan-saved', 'assistant'],
  );
  assert.equal(visible[2].Content, 'reply');
});

test('renderActionBar: interruptible states render both Stop and Send, no dead controls', () => {
  const agent = { session_id: 'a1', state: 'running', model: 'opus', branch: 'feat/x', effort: 'high' };
  const html = renderActionBar(agent);
  assert.match(html, /ui-composer__stop/);
  assert.match(html, /ui-composer__send/, 'send must stay reachable while running');
  assert.match(html, /ui-composer--interruptible/);
  // Dead controls are gone: no mic, no button-shaped chips.
  assert.doesNotMatch(html, /ui-composer__mic/);
  assert.doesNotMatch(html, /ui-composer__chip/);
});

test('renderActionBar: idle states render Send only', () => {
  const html = renderActionBar({ session_id: 'a1', state: 'done' });
  assert.match(html, /ui-composer__send/);
  assert.doesNotMatch(html, /ui-composer__stop/);
  assert.doesNotMatch(html, /ui-composer--interruptible/);
});

test('renderActionBar: meta rail shows only real values, never fabricated defaults', () => {
  // All three set → joined meta text.
  const full = renderActionBar({ session_id: 'a1', state: 'done', model: 'opus', branch: 'feat/x', effort: 'high' });
  assert.match(full, /ui-composer__meta/);
  assert.match(full, /opus · feat\/x · high/);
  // Meta is plain text, not a button.
  assert.doesNotMatch(full, /<button[^>]*ui-composer__meta/);
  // Nothing set → no meta span, and no invented "auto"/"no branch"/"⚡".
  const bare = renderActionBar({ session_id: 'a1', state: 'done' });
  assert.doesNotMatch(bare, /ui-composer__meta/);
  assert.doesNotMatch(bare, />auto</);
  assert.doesNotMatch(bare, /no branch/);
  assert.doesNotMatch(bare, /⚡/);
});
