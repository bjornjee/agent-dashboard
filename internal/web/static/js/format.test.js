// Unit tests for format.js helpers added for the PWA state-gap fix.
// Run via `node --test internal/web/static/js/format.test.js` (chained
// from `make test-js`).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let lastMessagePreview;
let durationFromUpdate;

test('load format module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'format.js')).href;
  const mod = await import(url);
  lastMessagePreview = mod.lastMessagePreview;
  durationFromUpdate = mod.durationFromUpdate;
  assert.equal(typeof lastMessagePreview, 'function');
  assert.equal(typeof durationFromUpdate, 'function');
});

// -- lastMessagePreview --

test('lastMessagePreview: empty agent returns empty string', () => {
  assert.equal(lastMessagePreview({}), '');
  assert.equal(lastMessagePreview({ last_message_preview: '' }), '');
});

test('lastMessagePreview: plain assistant text passes through', () => {
  assert.equal(
    lastMessagePreview({ last_message_preview: 'Refactoring the auth module' }),
    'Refactoring the auth module',
  );
});

test('lastMessagePreview: strips markdown emphasis and code fences', () => {
  const agent = {
    last_message_preview: '**Done** with `auth.go` — see ```snippet```',
  };
  const out = lastMessagePreview(agent);
  assert.equal(out.includes('**'), false);
  assert.equal(out.includes('`'), false);
  assert.equal(out.includes('Done'), true);
});

test('lastMessagePreview: truncates with ellipsis past max', () => {
  const long = 'a'.repeat(200);
  const out = lastMessagePreview({ last_message_preview: long }, 20);
  assert.equal(out.length, 20);
  assert.equal(out.endsWith('…'), true);
});

test('lastMessagePreview: prefers pending_question whenever it is populated, regardless of state', () => {
  // The question is the blocking thing — surface it even when the agent's
  // state group has been overridden (e.g. pinned_state=pr → state=pr, but
  // the agent is still asking). last_message_preview holds the assistant
  // message that preceded the tool call, which is NOT the question.
  for (const state of ['question', 'pr', 'running', 'permission', 'merged', 'done']) {
    const agent = {
      state,
      pending_question: { questions: [{ question: 'Which database driver should I use?' }] },
      last_message_preview: 'I will now ask about the driver.',
    };
    assert.equal(
      lastMessagePreview(agent),
      'Which database driver should I use?',
      `state=${state}`,
    );
  }
});

test('lastMessagePreview: falls back to last_message_preview when pending_question is null', () => {
  const agent = {
    state: 'running',
    pending_question: null,
    last_message_preview: 'Working on tests',
  };
  assert.equal(lastMessagePreview(agent), 'Working on tests');
});

// -- durationFromUpdate --

test('durationFromUpdate: prefers updated_at over started_at', () => {
  const now = Date.now();
  const updated = new Date(now - 3 * 60_000).toISOString(); // 3m ago
  const started = new Date(now - 60 * 60_000).toISOString(); // 1h ago
  const out = durationFromUpdate({ updated_at: updated, started_at: started });
  // Should reflect 3m (since updated_at), not 1h.
  assert.equal(out, '3m');
});

test('durationFromUpdate: falls back to started_at when updated_at missing', () => {
  const now = Date.now();
  const started = new Date(now - 5 * 60_000).toISOString();
  assert.equal(durationFromUpdate({ started_at: started }), '5m');
});

test('durationFromUpdate: returns empty when both timestamps missing', () => {
  assert.equal(durationFromUpdate({}), '');
});

test('durationFromUpdate: formats >60m as Xh Ym', () => {
  const now = Date.now();
  const updated = new Date(now - 75 * 60_000).toISOString(); // 1h 15m ago
  assert.equal(durationFromUpdate({ updated_at: updated }), '1h 15m');
});
