// Unit tests for the pure helpers in pages/list.js.
// DOM-mounting lives in renderList and is exercised by the Playwright suite.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let metaLine;
let rowBadges;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'list.js')).href;
  const mod = await import(url);
  metaLine = mod.metaLine;
  rowBadges = mod.rowBadges;
  assert.equal(typeof metaLine, 'function');
  assert.equal(typeof rowBadges, 'function');
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
  assert.match(html, /class="chip chip--plan"/);
  assert.match(html, /aria-hidden="true">PLAN</);
  assert.match(html, /class="visually-hidden">agent is in plan mode</);
});

test('rowBadges: renders subagent chip when subagent_count > 0', () => {
  const html = rowBadges({ subagent_count: 3 });
  assert.match(html, /class="chip chip--sub"/);
  assert.match(html, /aria-hidden="true">↳ 3</);
});

test('rowBadges: combines PLAN + subagent chips', () => {
  const html = rowBadges({ permission_mode: 'plan', subagent_count: 2 });
  assert.match(html, /chip--plan"/);
  assert.match(html, /chip--sub"/);
  assert.match(html, /aria-hidden="true">PLAN</);
  assert.match(html, /aria-hidden="true">↳ 2</);
});

test('rowBadges: ASK chip exposes "asking" expansion to screen readers', () => {
  // The 3-char "ASK" token is meaningless to a screen reader; the
  // visually-hidden span carries the meaning.
  const html = rowBadges({
    pending_question: { questions: [{ question: 'A?' }] },
  });
  assert.match(html, /class="chip chip--ask"/);
  assert.match(html, /aria-hidden="true">ASK</);
  assert.match(html, /class="visually-hidden">agent is asking a question</);
});

test('rowBadges: ASK precedes PLAN precedes ↳N in render order', () => {
  // Most-blocking signal first — the eye lands on the badge that needs
  // the user soonest.
  const html = rowBadges({
    pending_question: { questions: [{ question: 'A?' }] },
    permission_mode: 'plan',
    subagent_count: 2,
  });
  const askIdx = html.indexOf('chip--ask');
  const planIdx = html.indexOf('chip--plan');
  const subIdx = html.indexOf('chip--sub');
  assert.ok(askIdx !== -1 && planIdx !== -1 && subIdx !== -1, 'all three chips render');
  assert.ok(askIdx < planIdx && planIdx < subIdx, 'order: ASK < PLAN < ↳N');
});

test('rowBadges: chips are not double-escaped', () => {
  // ↳ is a literal Unicode glyph in the source — must not be HTML-encoded.
  const html = rowBadges({ subagent_count: 1 });
  assert.ok(html.includes('↳'));
  assert.ok(!html.includes('&#'));
});
