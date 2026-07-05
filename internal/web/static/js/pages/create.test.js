// Unit tests for pure helpers in pages/create.js.
// DOM-mounting code lives in renderCreate() and is exercised by the
// Playwright suite (visual verification in Phase C of the polish branch).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let buildRecentFolders;
let formatFolderLabel;
let replaceSelectOptions;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'create.js')).href;
  const mod = await import(url);
  buildRecentFolders = mod.buildRecentFolders;
  formatFolderLabel = mod.formatFolderLabel;
  replaceSelectOptions = mod.replaceSelectOptions;
  assert.equal(typeof buildRecentFolders, 'function');
  assert.equal(typeof formatFolderLabel, 'function');
  assert.equal(typeof replaceSelectOptions, 'function');
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

// Minimal <select> + document stand-ins for replaceSelectOptions — same
// stub-the-DOM approach as sidebar.test.js.
function fakeSelect(optionValues = [], value = '') {
  return {
    value,
    options: optionValues.map(v => ({ value: v })),
    remove(i) { this.options.splice(i, 1); },
    appendChild(opt) { this.options.push(opt); },
  };
}

function withFakeDocument(fn) {
  const orig = globalThis.document;
  globalThis.document = { createElement: () => ({ value: '', textContent: '' }) };
  try { fn(); } finally {
    if (orig === undefined) delete globalThis.document;
    else globalThis.document = orig;
  }
}

test('replaceSelectOptions — null select or non-array values is a no-op', () => {
  withFakeDocument(() => {
    assert.doesNotThrow(() => replaceSelectOptions(null, ['a'], 'Default'));
    const sel = fakeSelect(['old'], 'old');
    replaceSelectOptions(sel, null, 'Default');
    replaceSelectOptions(sel, undefined, 'Default');
    assert.equal(sel.options.length, 1);
    assert.equal(sel.value, 'old');
  });
});

test('replaceSelectOptions — repopulates with default option first', () => {
  withFakeDocument(() => {
    const sel = fakeSelect(['stale1', 'stale2']);
    replaceSelectOptions(sel, ['opus', 'sonnet'], 'Default');
    assert.deepEqual(sel.options.map(o => o.value), ['', 'opus', 'sonnet']);
    assert.equal(sel.options[0].textContent, 'Default');
    assert.equal(sel.value, '');
  });
});

test('replaceSelectOptions — preserves prior selection when still present', () => {
  withFakeDocument(() => {
    const sel = fakeSelect(['', 'opus', 'sonnet'], 'sonnet');
    replaceSelectOptions(sel, ['sonnet', 'haiku'], 'Default');
    assert.equal(sel.value, 'sonnet');
  });
});

test('replaceSelectOptions — resets to default when prior selection is gone', () => {
  withFakeDocument(() => {
    const sel = fakeSelect(['', 'gpt-5.5'], 'gpt-5.5');
    replaceSelectOptions(sel, ['opus', 'sonnet'], 'Default');
    assert.equal(sel.value, '');
  });
});

test('replaceSelectOptions — empty values array leaves only the default option', () => {
  withFakeDocument(() => {
    const sel = fakeSelect(['', 'opus'], '');
    replaceSelectOptions(sel, [], 'Default');
    assert.deepEqual(sel.options.map(o => o.value), ['']);
    assert.equal(sel.value, '');
  });
});
