// Unit tests for UI.message() and stripLocalCommandTags() in ui.js.
// Run via `node --test internal/web/static/js/ui.test.js` (chained from `make test`).
//
// Coverage:
//   - UI.message() guards the three rendering branches (user pill,
//     assistant card with avatar, tool footer) against regressions
//     as the assistant branch grew an avatar + optional timestamp.
//   - stripLocalCommandTags() unwraps Claude Code's
//     <local-command-{caveat,stdout,stderr}> tags from user messages
//     before display.

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

// -- UI.message regression coverage --

test('user branch — pill markup is preserved (regression)', () => {
  const html = UI.message('user', 'hi');
  assert.match(html, /class="ui-msg ui-msg--user"/);
  assert.match(html, /class="ui-msg__bubble">hi</);
});

test('user branch — HTML in user content is escaped', () => {
  const html = UI.message('user', '<script>x</script>');
  assert.match(html, /&lt;script&gt;x&lt;\/script&gt;/);
  assert.doesNotMatch(html, /<script>/);
});

test('assistant — default avatar is "A" and prose is escaped', () => {
  const html = UI.message('assistant', 'plain text');
  assert.match(html, /class="ui-msg ui-msg--assistant"/);
  assert.match(html, /class="ui-msg__avatar"[^>]*>A</);
  assert.match(html, /class="ui-msg__card"/);
  assert.match(html, /class="ui-msg__prose">plain text</);
  // Default copy button present.
  assert.match(html, /class="ui-msg__copy"/);
});

test('assistant — html:true bypasses prose escape', () => {
  const html = UI.message('assistant', '<p>x</p>', { html: true });
  assert.match(html, /class="ui-msg__prose"><p>x<\/p></);
});

test('assistant — custom avatar + timestamp render', () => {
  const html = UI.message('assistant', 'x', { avatar: 'C', timestamp: '12:34' });
  assert.match(html, /class="ui-msg__avatar"[^>]*>C</);
  assert.match(html, /class="ui-msg__meta">12:34</);
});

test('assistant — copyable:false omits copy button', () => {
  const html = UI.message('assistant', 'x', { copyable: false });
  assert.doesNotMatch(html, /class="ui-msg__copy"/);
});

test('assistant — avatar text is HTML-escaped', () => {
  const html = UI.message('assistant', 'x', { avatar: '<b>' });
  assert.match(html, /class="ui-msg__avatar"[^>]*>&lt;b&gt;</);
});

test('assistant — timestamp text is HTML-escaped', () => {
  const html = UI.message('assistant', 'x', { timestamp: '<i>now</i>' });
  assert.match(html, /class="ui-msg__meta">&lt;i&gt;now&lt;\/i&gt;</);
});

test('tool footer branch — markup is preserved (regression)', () => {
  const html = UI.message('assistant', '', { tool: { label: 'bash' } });
  assert.match(html, /class="ui-msg__tool"/);
  assert.match(html, /<span>bash<\/span>/);
});

test('tool footer — label is HTML-escaped', () => {
  const html = UI.message('assistant', '', { tool: { label: '<x>' } });
  assert.match(html, /<span>&lt;x&gt;<\/span>/);
});

// -- stripLocalCommandTags coverage --

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
