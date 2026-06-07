// Unit tests for the shared agent-row module. Locks the row contract
// so future row-level edits don't accidentally drift between list.js
// and sidebar.js (both consume this module — no duplication).

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let statusDot;
let chip;
let rowBadges;
let metaLine;
let agentRowOpts;

test('load module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'agent-row.js')).href;
  const mod = await import(url);
  statusDot = mod.statusDot;
  chip = mod.chip;
  rowBadges = mod.rowBadges;
  metaLine = mod.metaLine;
  agentRowOpts = mod.agentRowOpts;
  for (const fn of [statusDot, chip, rowBadges, metaLine, agentRowOpts]) {
    assert.equal(typeof fn, 'function');
  }
});

// -- statusDot --

test('statusDot: includes aria-label for known states', () => {
  const html = statusDot('question');
  assert.match(html, /role="img"/);
  assert.match(html, /aria-label="Needs reply"/);
  assert.match(html, /status-dot--waiting/);
});

test('statusDot: omits aria when state is unknown', () => {
  const html = statusDot('');
  assert.doesNotMatch(html, /aria-label/);
});

// -- chip --

test('chip: visible token + visually-hidden expansion', () => {
  const html = chip('chip--ask', 'ASK', 'agent is asking a question');
  assert.match(html, /class="chip chip--ask"/);
  assert.match(html, /aria-hidden="true">ASK</);
  assert.match(html, /class="visually-hidden">agent is asking a question</);
});

test('chip: escapes HTML in both slots', () => {
  const html = chip('chip--x', '<bad>', '<sr>');
  assert.match(html, /&lt;bad&gt;/);
  assert.match(html, /&lt;sr&gt;/);
});

// -- rowBadges (order + suppression) --

test('rowBadges: ASK first when question pending', () => {
  const html = rowBadges({
    pending_question: { questions: [{ question: 'A?' }] },
    permission_mode: 'plan',
    subagent_count: 2,
  });
  const askIdx = html.indexOf('chip--ask');
  const planIdx = html.indexOf('chip--plan');
  const subIdx = html.indexOf('chip--sub');
  assert.ok(askIdx !== -1 && planIdx > askIdx && subIdx > planIdx, 'ASK < PLAN < sub');
});

test('rowBadges: empty when no signals', () => {
  assert.equal(rowBadges({ state: 'running' }), '');
});

// -- metaLine --

test('metaLine: joins branch · model · duration', () => {
  const updated = new Date(Date.now() - 4 * 60_000).toISOString();
  assert.equal(
    metaLine({ branch: 'feat/x', model: 'opus', updated_at: updated }),
    'feat/x · opus · 4m',
  );
});

test('metaLine: omits missing parts', () => {
  assert.equal(metaLine({ branch: 'feat/x' }), 'feat/x');
  assert.equal(metaLine({}), '');
});

// -- agentRowOpts (the contract between both views) --

test('agentRowOpts: returns UI.row opts with leading dot, title, subtitle, badges', () => {
  const updated = new Date(Date.now() - 5 * 60_000).toISOString();
  const agent = {
    session_id: 'a1',
    state: 'running',
    branch: 'feat/x',
    model: 'opus',
    updated_at: updated,
    worktree_cwd: '/work/myrepo',
    pending_question: { questions: [{ question: 'A?' }] },
  };
  const opts = agentRowOpts(agent, { onclick: 'Dashboard.selectAgent(\'a1\')' });
  assert.match(opts.leading, /status-dot--running/);
  assert.equal(opts.title, 'myrepo');
  assert.equal(opts.subtitle, 'feat/x · opus · 5m');
  assert.equal(opts.tag, ''); // ASK suppresses PR
  assert.match(opts.badges, /chip--ask/);
  assert.equal(opts.onclick, 'Dashboard.selectAgent(\'a1\')');
  assert.equal(opts.chevron, true);
});

test('agentRowOpts: chevron false threads through (sidebar opt-out)', () => {
  const opts = agentRowOpts({ state: 'done' }, { onclick: 'x', chevron: false });
  assert.equal(opts.chevron, false);
});

test('agentRowOpts: PR tag survives when no ASK chip', () => {
  const opts = agentRowOpts({
    state: 'pr',
    pinned_state: 'pr',
    branch: 'feat/y',
  }, { onclick: 'x' });
  assert.equal(opts.tag, 'PR');
  assert.doesNotMatch(opts.badges, /chip--ask/);
});
