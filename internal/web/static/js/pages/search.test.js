// Unit tests for the pure helpers in pages/search.js.
// DOM-mounting code lives in openSearch/closeSearch and is exercised by
// the Playwright suite (tests/playwright/tests/search.spec.js).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let highlightMatched;
let searchAgents;
let searchTagClass;
let resumableBadge;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'search.js')).href;
  const mod = await import(url);
  highlightMatched = mod.highlightMatched;
  searchAgents = mod.searchAgents;
  searchTagClass = mod.searchTagClass;
  resumableBadge = mod.resumableBadge;
  assert.equal(typeof highlightMatched, 'function');
  assert.equal(typeof searchAgents, 'function');
  assert.equal(typeof searchTagClass, 'function');
  assert.equal(typeof resumableBadge, 'function');
});

test('highlightMatched wraps matched chars in <strong>', () => {
  // Indices reference positions in the raw input (pre-escape).
  const html = highlightMatched('agent-dashboard', [0, 1, 4]);
  assert.equal(
    html,
    '<strong class="search-overlay__hit">a</strong>' +
    '<strong class="search-overlay__hit">g</strong>' +
    'en' +
    '<strong class="search-overlay__hit">t</strong>' +
    '-dashboard'
  );
});

test('highlightMatched HTML-escapes the surrounding text', () => {
  const html = highlightMatched('<script>x</script>', []);
  assert.equal(html, '&lt;script&gt;x&lt;/script&gt;');
});

test('highlightMatched escapes the matched chars too', () => {
  const html = highlightMatched('<x>', [0, 1]);
  assert.equal(
    html,
    '<strong class="search-overlay__hit">&lt;</strong>' +
    '<strong class="search-overlay__hit">x</strong>' +
    '&gt;'
  );
});

test('highlightMatched with empty indices just escapes', () => {
  assert.equal(highlightMatched('plain', []), 'plain');
});

test('highlightMatched with null indices just escapes', () => {
  assert.equal(highlightMatched('plain', null), 'plain');
});

test('searchAgents returns all agents in order for empty needle', () => {
  const agents = [
    { session_id: '1', cwd: '/home/u/myapp', branch: 'main' },
    { session_id: '2', cwd: '/home/u/api', branch: 'feat/x' },
  ];
  const results = searchAgents(agents, '', 50);
  assert.equal(results.length, 2);
  assert.equal(results[0].item.session_id, '1');
  assert.equal(results[1].item.session_id, '2');
});

test('searchAgents matches against repo name and branch', () => {
  const agents = [
    { session_id: '1', cwd: '/home/u/myapp', branch: 'main' },
    { session_id: '2', cwd: '/home/u/api', branch: 'feat/dashboard' },
    { session_id: '3', cwd: '/home/u/worktrees', branch: 'main' },
  ];
  const byBranch = searchAgents(agents, 'feat', 50);
  assert.equal(byBranch.length, 1, 'only the feat branch agent should match');
  assert.equal(byBranch[0].item.session_id, '2');

  const byRepo = searchAgents(agents, 'wrk', 50);
  assert.equal(byRepo.length, 1);
  assert.equal(byRepo[0].item.session_id, '3');
});

test('searchAgents matches cwd and last message preview', () => {
  const agents = [
    { session_id: '1', cwd: '/home/u/alpha', branch: 'main', last_message_preview: 'fix the auth flow' },
    { session_id: '2', cwd: '/home/u/beta', branch: 'main', last_message_preview: 'add search' },
  ];
  // "auth" appears only in agent 1's last_message_preview.
  const byPreview = searchAgents(agents, 'auth', 50);
  assert.equal(byPreview.length, 1);
  assert.equal(byPreview[0].item.session_id, '1');
  // "beta" appears in agent 2's cwd path.
  const byCwd = searchAgents(agents, 'beta', 50);
  assert.equal(byCwd[0].item.session_id, '2');
});

test('searchAgents orphanOnly filters to resumable agents', () => {
  const agents = [
    { session_id: '1', cwd: '/home/u/alpha', resumable: true },
    { session_id: '2', cwd: '/home/u/beta' },
    { session_id: '3', cwd: '/home/u/gamma', resumable: true },
  ];
  const res = searchAgents(agents, '', 50, true);
  const ids = res.map((r) => r.item.session_id).sort();
  assert.deepEqual(ids, ['1', '3'], 'only resumable agents survive orphanOnly');
  // Without the flag, all agents are searchable.
  assert.equal(searchAgents(agents, '', 50, false).length, 3);
});

test('resumableBadge renders a pill only for resumable agents', () => {
  assert.ok(resumableBadge({ resumable: true }).toLowerCase().includes('resumable'));
  assert.equal(resumableBadge({ resumable: false }), '');
  assert.equal(resumableBadge({}), '');
});

test('searchAgents respects max cap', () => {
  const agents = Array.from({ length: 25 }, (_, i) => ({
    session_id: 'a' + i,
    cwd: '/home/u/agentrepo' + i,
    branch: 'main',
  }));
  const results = searchAgents(agents, 'agent', 5);
  assert.equal(results.length, 5);
});

test('searchAgents handles missing branch gracefully', () => {
  const agents = [{ session_id: '1', cwd: '/home/u/myapp' }];
  const results = searchAgents(agents, 'app', 50);
  assert.equal(results.length, 1);
  assert.equal(results[0].item.session_id, '1');
});

// G1: status pills in the search overlay must mirror the sidebar's
// state-dot semantics — each agent state maps to its existing accent-*
// token so the pill carries a status colour instead of neutral grey.
test('searchTagClass maps waiting/blocked states to amber accent modifier', () => {
  assert.equal(searchTagClass('question'), 'search-overlay__tag--waiting');
  assert.equal(searchTagClass('error'), 'search-overlay__tag--waiting');
  assert.equal(searchTagClass('permission'), 'search-overlay__tag--blocked');
  assert.equal(searchTagClass('plan'), 'search-overlay__tag--blocked');
});

test('searchTagClass maps running state to running accent modifier', () => {
  assert.equal(searchTagClass('running'), 'search-overlay__tag--running');
});

test('searchTagClass maps review states to review accent modifier', () => {
  assert.equal(searchTagClass('idle_prompt'), 'search-overlay__tag--review');
  assert.equal(searchTagClass('done'), 'search-overlay__tag--review');
});

test('searchTagClass maps pr state to pr accent modifier', () => {
  assert.equal(searchTagClass('pr'), 'search-overlay__tag--pr');
});

test('searchTagClass returns empty for merged / unknown states (neutral)', () => {
  assert.equal(searchTagClass('merged'), '');
  assert.equal(searchTagClass('somethingelse'), '');
  assert.equal(searchTagClass(''), '');
  assert.equal(searchTagClass(undefined), '');
});
