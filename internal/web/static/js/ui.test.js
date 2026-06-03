// Unit tests for UI primitives in ui.js.
// Run via `node --test internal/web/static/js/ui.test.js` (chained from `make test`).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let UI;
let stripLocalCommandTags;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'ui.js')).href;
  const mod = await import(url);
  UI = mod.UI;
  stripLocalCommandTags = mod.stripLocalCommandTags;
  assert.equal(typeof UI, 'object');
  assert.equal(typeof UI.message, 'function');
  assert.equal(typeof stripLocalCommandTags, 'function');
});

test('stripLocalCommandTags unwraps <local-command-caveat>', () => {
  const input = '<local-command-caveat>be careful</local-command-caveat>';
  assert.equal(stripLocalCommandTags(input), 'be careful');
});

test('stripLocalCommandTags unwraps <local-command-stdout>', () => {
  const input = '<local-command-stdout>hello world</local-command-stdout>';
  assert.equal(stripLocalCommandTags(input), 'hello world');
});

test('stripLocalCommandTags unwraps <local-command-stderr>', () => {
  const input = '<local-command-stderr>boom</local-command-stderr>';
  assert.equal(stripLocalCommandTags(input), 'boom');
});

test('stripLocalCommandTags unwraps multi-line content', () => {
  const input = '<local-command-stdout>line 1\nline 2\nline 3</local-command-stdout>';
  assert.equal(stripLocalCommandTags(input), 'line 1\nline 2\nline 3');
});

test('stripLocalCommandTags handles multiple wrappers in one string', () => {
  const input =
    'pre <local-command-stdout>out</local-command-stdout> mid ' +
    '<local-command-stderr>err</local-command-stderr> post';
  assert.equal(stripLocalCommandTags(input), 'pre out mid err post');
});

test('stripLocalCommandTags is a no-op on bare text', () => {
  const input = 'just a regular user message';
  assert.equal(stripLocalCommandTags(input), 'just a regular user message');
});

test('stripLocalCommandTags handles empty string', () => {
  assert.equal(stripLocalCommandTags(''), '');
});

test('stripLocalCommandTags handles null/undefined safely', () => {
  assert.equal(stripLocalCommandTags(null), '');
  assert.equal(stripLocalCommandTags(undefined), '');
});

test('UI.message user role strips local-command-stdout wrapper before escaping', () => {
  const html = UI.message('user', '<local-command-stdout>hello</local-command-stdout>');
  // The wrapper must be gone (no escaped <local-command-stdout> in output).
  assert.ok(!html.includes('local-command-stdout'), 'wrapper tag should be stripped');
  assert.ok(html.includes('hello'), 'inner text should survive');
});

test('UI.message user role strips local-command-caveat wrapper', () => {
  const html = UI.message('user', '<local-command-caveat>warning</local-command-caveat>');
  assert.ok(!html.includes('local-command-caveat'));
  assert.ok(html.includes('warning'));
});

test('UI.message user role still escapes HTML in stripped inner text', () => {
  const html = UI.message('user', '<local-command-stdout><script>alert(1)</script></local-command-stdout>');
  assert.ok(!html.includes('<script>'), 'inner HTML must remain escaped');
  assert.ok(html.includes('&lt;script&gt;'), 'inner text should be HTML-escaped');
});

test('UI.message user role leaves bare text unchanged', () => {
  const html = UI.message('user', 'plain message');
  assert.ok(html.includes('plain message'));
});
