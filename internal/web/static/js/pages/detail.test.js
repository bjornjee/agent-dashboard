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
      question: 'Plan has 3 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Continue inline (Recommended)', description: 'Stay here.' },
        { label: 'Hand off', description: 'Use implementation dispatch.' },
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
      question: 'Plan has 3 phases. Continue inline here, or hand off?',
      header: 'Dispatch',
      multi_select: false,
      options: [
        { label: 'Continue inline (Recommended)', description: 'Stay here.' },
        { label: 'Hand off', description: 'Use implementation dispatch.' },
      ],
    }],
  };

  const html = renderQuestionCard(pending, 'agent-1');
  assert.match(html, /class="question-card"/);
  assert.match(html, /data-tool-use-id="[^"]+"/);
  assert.match(html, /Plan has 3 phases/);
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

test('formatLatestToolDisplay falls back to raw tool name when not classified', () => {
  const entry = { content: '→ SomethingNovel: arg here' };
  // classifyTool returns { bucket: 'other', live: 'Running SomethingNovel' }
  assert.equal(formatLatestToolDisplay(entry), 'Running SomethingNovel · arg here');
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

test('isAgentMidTurn keeps other working states (permission/plan/question/error/waiting_input) as mid-turn', () => {
	assert.equal(isAgentMidTurn({ state: 'permission' }), true);
	assert.equal(isAgentMidTurn({ state: 'plan' }), true);
	assert.equal(isAgentMidTurn({ state: 'question' }), true);
	assert.equal(isAgentMidTurn({ state: 'error' }), true);
	assert.equal(isAgentMidTurn({ state: 'waiting_input' }), true);
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
