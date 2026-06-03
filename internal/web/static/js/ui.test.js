// Unit tests for UI.message() — guards the three rendering branches
// (user pill, assistant card with avatar, tool footer) against regressions
// as the assistant branch grows an avatar + optional timestamp.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let UI;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'ui.js')).href;
  const mod = await import(url);
  UI = mod.UI;
  assert.equal(typeof UI.message, 'function');
});

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

