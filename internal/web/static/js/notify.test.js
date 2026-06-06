// Unit tests for formatNotification() in notify.js.
// Run via `node --test internal/web/static/js/notify.test.js` (chained from `make test`).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let formatNotification;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'notify.js')).href;
  const mod = await import(url);
  formatNotification = mod.formatNotification;
  assert.equal(typeof formatNotification, 'function');
});

test('title combines branch and state label', () => {
  const { title } = formatNotification(
    { branch: 'feat/foo', worktree_cwd: '/a/b/c', last_message_preview: 'hi' },
    'Plan ready',
  );
  assert.equal(title, 'feat/foo · Plan ready');
});

test('body line 1 is basename of worktree_cwd, not full path', () => {
  const { body } = formatNotification(
    { branch: 'feat/foo', worktree_cwd: '/a/b/c', last_message_preview: 'hi' },
    'Plan ready',
  );
  const [line1] = body.split('\n');
  assert.equal(line1, 'c');
});

test('prefers worktree_cwd over cwd for body line 1', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/a/wt', cwd: '/a/launch', last_message_preview: 'hi' },
    'Finished',
  );
  const [line1] = body.split('\n');
  assert.equal(line1, 'wt');
});

test('falls back to cwd when worktree_cwd empty', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '', cwd: '/a/launch', last_message_preview: 'hi' },
    'Finished',
  );
  const [line1] = body.split('\n');
  assert.equal(line1, 'launch');
});

test('strips trailing slash before taking basename', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/a/b/c/', last_message_preview: 'hi' },
    'Finished',
  );
  const [line1] = body.split('\n');
  assert.equal(line1, 'c');
});

test('state=question uses pending_question text for description', () => {
  const { body } = formatNotification(
    {
      branch: 'b',
      worktree_cwd: '/x',
      state: 'question',
      pending_question: { questions: [{ question: 'Pick one?' }] },
      last_message_preview: 'old preview',
    },
    'Question',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2, 'Pick one?');
});

test('non-question state uses last_message_preview', () => {
  const { body } = formatNotification(
    {
      branch: 'b',
      worktree_cwd: '/x',
      state: 'done',
      pending_question: { questions: [{ question: 'ignored' }] },
      last_message_preview: 'the preview',
    },
    'Finished',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2, 'the preview');
});

test('question state without pending_question falls back to preview', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/x', state: 'question', last_message_preview: 'the preview' },
    'Question',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2, 'the preview');
});

test('truncates long preview at 140 chars with trailing ellipsis', () => {
  const long = 'a'.repeat(200);
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/x', last_message_preview: long },
    'Finished',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2.length, 140);
  assert.equal(line2.endsWith('…'), true);
  assert.equal(line2.startsWith('a'.repeat(139)), true);
});

test('does not append ellipsis when preview fits', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/x', last_message_preview: 'short' },
    'Finished',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2, 'short');
});

test('falls back to short session_id when branch empty', () => {
  const { title } = formatNotification(
    { branch: '', session_id: 'abcdef1234567890', last_message_preview: 'hi' },
    'Finished',
  );
  assert.equal(title, 'abcdef1 · Finished');
});

test('body is just the state label when dir and preview both empty', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '', cwd: '', last_message_preview: '' },
    'Finished',
  );
  assert.equal(body, 'Finished');
});

test('omits empty body line 1 without leading newline', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '', cwd: '', last_message_preview: 'just a preview' },
    'Finished',
  );
  assert.equal(body, 'just a preview');
});

test('strips leading and trailing whitespace from preview', () => {
  const { body } = formatNotification(
    { branch: 'b', worktree_cwd: '/x', last_message_preview: '  padded  \n' },
    'Finished',
  );
  const [, line2] = body.split('\n');
  assert.equal(line2, 'padded');
});
