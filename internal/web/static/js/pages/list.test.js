// Unit tests for the pure helpers in pages/list.js.
// DOM-mounting lives in renderList and is exercised by the Playwright suite.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let previewLine;
let metaLine;
let rowBadges;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'list.js')).href;
  const mod = await import(url);
  previewLine = mod.previewLine;
  metaLine = mod.metaLine;
  rowBadges = mod.rowBadges;
  assert.equal(typeof previewLine, 'function');
  assert.equal(typeof metaLine, 'function');
  assert.equal(typeof rowBadges, 'function');
});

// -- previewLine --

test('previewLine: empty when agent has no preview source', () => {
  assert.equal(previewLine({ branch: 'feat/x' }), '');
});

test('previewLine: returns assistant preview text', () => {
  assert.equal(
    previewLine({ last_message_preview: 'Refactoring the auth module' }),
    'Refactoring the auth module',
  );
});

test('previewLine: surfaces the pending question when state=question', () => {
  // Tool-based questions: the actual question text is what the user needs to
  // see at a glance, not the assistant message that came before the tool call.
  const agent = {
    state: 'question',
    pending_question: {
      questions: [{ question: 'Should I add tests for the migration?' }],
    },
  };
  assert.equal(previewLine(agent), 'Should I add tests for the migration?');
});

// -- metaLine --

test('metaLine: joins branch · model · duration', () => {
  // Use a recent updated_at so duration is a small minute count.
  const updated = new Date(Date.now() - 4 * 60_000).toISOString();
  const out = metaLine({
    branch: 'feat/x',
    model: 'opus',
    updated_at: updated,
  });
  assert.equal(out, 'feat/x · opus · 4m');
});

test('metaLine: falls back to started_at when updated_at missing', () => {
  // Backwards-compat: older state files don't have updated_at yet.
  const started = new Date(Date.now() - 7 * 60_000).toISOString();
  const out = metaLine({ branch: 'feat/x', model: 'opus', started_at: started });
  assert.equal(out, 'feat/x · opus · 7m');
});

test('metaLine: omits missing parts cleanly', () => {
  assert.equal(metaLine({ branch: 'feat/x' }), 'feat/x');
});

test('metaLine: empty when nothing to show', () => {
  assert.equal(metaLine({}), '');
});

// -- rowBadges --

test('rowBadges: empty when no badge signals', () => {
  assert.equal(rowBadges({ state: 'running' }), '');
});

test('rowBadges: renders PLAN chip when permission_mode=plan', () => {
  const html = rowBadges({ permission_mode: 'plan' });
  assert.match(html, /class="chip chip--plan">PLAN</);
});

test('rowBadges: renders subagent chip when subagent_count > 0', () => {
  const html = rowBadges({ subagent_count: 3 });
  assert.match(html, /class="chip chip--sub">↳ 3</);
});

test('rowBadges: combines PLAN + subagent chips', () => {
  const html = rowBadges({ permission_mode: 'plan', subagent_count: 2 });
  assert.match(html, /chip--plan">PLAN</);
  assert.match(html, /chip--sub">↳ 2</);
});

test('rowBadges: chips are not double-escaped', () => {
  // ↳ is a literal Unicode glyph in the source — must not be HTML-encoded.
  const html = rowBadges({ subagent_count: 1 });
  assert.ok(html.includes('↳'));
  assert.ok(!html.includes('&#'));
});
