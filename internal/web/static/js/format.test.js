// Unit tests for format.js helpers added for the PWA state-gap fix.
// Run via `node --test internal/web/static/js/format.test.js` (chained
// from `make test-js`).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let durationFromUpdate;

test('load format module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'format.js')).href;
  const mod = await import(url);
  durationFromUpdate = mod.durationFromUpdate;
  assert.equal(typeof durationFromUpdate, 'function');
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
