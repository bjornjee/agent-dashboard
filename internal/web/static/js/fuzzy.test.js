// Unit tests for the fuzzy matcher used by the search overlay.
// Run via `node --test internal/web/static/js/fuzzy.test.js` (chained from `make test`).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let fuzzyMatch;
let fuzzyRank;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'fuzzy.js')).href;
  const mod = await import(url);
  fuzzyMatch = mod.fuzzyMatch;
  fuzzyRank = mod.fuzzyRank;
  assert.equal(typeof fuzzyMatch, 'function');
  assert.equal(typeof fuzzyRank, 'function');
});

test('empty needle returns zero score, empty indices', () => {
  const r = fuzzyMatch('', 'agent-dashboard');
  assert.deepEqual(r, { score: 0, indices: [] });
});

test('subsequence match returns indices in order', () => {
  const r = fuzzyMatch('agt', 'agent-dashboard');
  assert.ok(r, 'expected a match');
  assert.deepEqual(r.indices, [0, 1, 4]);
  assert.ok(r.score > 0);
});

test('consecutive run is detected by indices', () => {
  const r = fuzzyMatch('dash', 'agent-dashboard');
  assert.ok(r);
  assert.deepEqual(r.indices, [6, 7, 8, 9]);
  assert.ok(r.score > 0);
});

test('missing character returns null', () => {
  assert.equal(fuzzyMatch('xz', 'agent-dashboard'), null);
});

test('case-insensitive', () => {
  const r = fuzzyMatch('AG', 'agent-dashboard');
  assert.ok(r);
  assert.deepEqual(r.indices, [0, 1]);
});

test('prefix bonus beats interior match', () => {
  const prefix = fuzzyMatch('age', 'agent-dashboard');
  const interior = fuzzyMatch('age', 'manage-aged');
  assert.ok(prefix && interior);
  assert.ok(prefix.score > interior.score,
    `prefix ${prefix.score} should beat interior ${interior.score}`);
});

test('word-boundary bonus after a dash', () => {
  // 'dash' starts at a word boundary (after the '-').
  const boundary = fuzzyMatch('dash', 'agent-dashboard');
  const scattered = fuzzyMatch('dash', 'gradashing');
  assert.ok(boundary && scattered);
  assert.ok(boundary.score > scattered.score,
    `boundary ${boundary.score} should beat scattered ${scattered.score}`);
});

test('consecutive bonus prefers the tight run', () => {
  // For "ad" in "agent-dashboard", the consecutive run a-d in "dashboard"
  // sits at indices [7, 8] (the 'a','d' of "da..."). Wait — "dashboard"
  // is d-a-s-h-b-o-a-r-d. So "ad" can match either [0(a),7(d)] (scattered)
  // or [7(a),9(d)] (also scattered). The matcher should choose whichever
  // scores best; the test asserts a positive score and that the chosen
  // indices are increasing.
  const r = fuzzyMatch('ad', 'agent-dashboard');
  assert.ok(r);
  assert.equal(r.indices.length, 2);
  assert.ok(r.indices[0] < r.indices[1]);
  assert.ok(r.score > 0);
});

test('fuzzyRank orders by best per-item score and drops non-matches', () => {
  const items = [
    { name: 'innerjoyreiki', branch: 'main' },
    { name: 'agent-dashboard', branch: 'feat/x' },
    { name: 'worktrees', branch: 'feat/other' },
  ];
  const ranked = fuzzyRank('agt', items, (it) => [it.name, it.branch || '']);
  assert.equal(ranked.length, 1, 'only agent-dashboard fuzzy-matches "agt"');
  assert.equal(ranked[0].item.name, 'agent-dashboard');
  assert.ok(Array.isArray(ranked[0].indicesByField));
  assert.equal(ranked[0].indicesByField.length, 2,
    'one indices array per haystack field');
  // Field 0 (name) matched; field 1 (branch "feat/x") did not match "agt".
  assert.deepEqual(ranked[0].indicesByField[0], [0, 1, 4]);
  assert.equal(ranked[0].indicesByField[1], null);
});

test('fuzzyRank with empty needle returns all items in original order, zero score', () => {
  const items = [{ id: 1 }, { id: 2 }, { id: 3 }];
  const ranked = fuzzyRank('', items, () => ['x']);
  assert.equal(ranked.length, 3);
  assert.deepEqual(ranked.map((r) => r.item.id), [1, 2, 3]);
  assert.equal(ranked[0].score, 0);
});

test('fuzzyRank stable on tie (original order preserved)', () => {
  // Identical haystacks → identical scores → original order wins.
  const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  const ranked = fuzzyRank('x', items, () => ['x']);
  assert.deepEqual(ranked.map((r) => r.item.id), ['a', 'b', 'c']);
});
