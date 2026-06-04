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

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'detail.js')).href;
  const mod = await import(url);
  questionCardId = mod.questionCardId;
  renderQuestionCard = mod.renderQuestionCard;
  formatLatestToolDisplay = mod.formatLatestToolDisplay;
  assert.equal(typeof questionCardId, 'function');
  assert.equal(typeof renderQuestionCard, 'function');
  assert.equal(typeof formatLatestToolDisplay, 'function');
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
