// Unit tests for pure helpers in pages/create.js.
// DOM-mounting code lives in renderCreate() and is exercised by the
// Playwright suite (visual verification in Phase C of the polish branch).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let buildRecentFolders;
let formatFolderLabel;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'create.js')).href;
  const mod = await import(url);
  buildRecentFolders = mod.buildRecentFolders;
  formatFolderLabel = mod.formatFolderLabel;
  assert.equal(typeof buildRecentFolders, 'function');
  assert.equal(typeof formatFolderLabel, 'function');
});

test('buildRecentFolders — empty input returns []', () => {
  assert.deepEqual(buildRecentFolders([]), []);
  assert.deepEqual(buildRecentFolders(null), []);
  assert.deepEqual(buildRecentFolders(undefined), []);
});

test('buildRecentFolders — sorts by count desc, basename label', () => {
  const agents = [
    { cwd: '/repo/alpha' },
    { cwd: '/repo/alpha' },
    { cwd: '/repo/alpha' },
    { cwd: '/repo/beta' },
    { cwd: '/repo/gamma' },
  ];
  const out = buildRecentFolders(agents);
  assert.equal(out.length, 3);
  assert.deepEqual(out[0], { cwd: '/repo/alpha', count: 3, label: 'alpha' });
  // beta and gamma both count=1 — order between them is insertion order.
  assert.equal(out[1].count, 1);
  assert.equal(out[2].count, 1);
});

test('buildRecentFolders — caps at limit (default 3)', () => {
  const agents = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map(n => ({ cwd: `/r/${n}` }));
  assert.equal(buildRecentFolders(agents).length, 3);
  assert.equal(buildRecentFolders(agents, 5).length, 5);
});

test('buildRecentFolders — ignores falsy cwds', () => {
  const agents = [{ cwd: '' }, { cwd: null }, { cwd: '/r/a' }, {}];
  const out = buildRecentFolders(agents);
  assert.equal(out.length, 1);
  assert.equal(out[0].cwd, '/r/a');
});

test('buildRecentFolders — trailing slash still yields basename', () => {
  const out = buildRecentFolders([{ cwd: '/repo/alpha/' }]);
  assert.equal(out[0].label, 'alpha');
});

test('formatFolderLabel — basename of absolute path', () => {
  assert.equal(formatFolderLabel('/Users/me/code/repo'), 'repo');
});

test('formatFolderLabel — trailing slash trimmed', () => {
  assert.equal(formatFolderLabel('/Users/me/code/repo/'), 'repo');
});

test('formatFolderLabel — empty / null falls back to default', () => {
  assert.equal(formatFolderLabel(''), 'Work in a project');
  assert.equal(formatFolderLabel(null), 'Work in a project');
  assert.equal(formatFolderLabel(undefined), 'Work in a project');
});

test('formatFolderLabel — single segment is returned as-is', () => {
  assert.equal(formatFolderLabel('repo'), 'repo');
});
