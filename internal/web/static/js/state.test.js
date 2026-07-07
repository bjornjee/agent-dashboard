// Unit tests for state.js badge helpers added for the PWA state-gap fix.

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

let planBadge;
let subagentBadge;
let questionBadge;
let prTag;
let stateLabel;

test('load state module', async () => {
  const url = pathToFileURL(path.join(__dirname, 'state.js')).href;
  const mod = await import(url);
  planBadge = mod.planBadge;
  subagentBadge = mod.subagentBadge;
  questionBadge = mod.questionBadge;
  prTag = mod.prTag;
  stateLabel = mod.stateLabel;
  assert.equal(typeof planBadge, 'function');
  assert.equal(typeof subagentBadge, 'function');
  assert.equal(typeof questionBadge, 'function');
  assert.equal(typeof prTag, 'function');
  assert.equal(typeof stateLabel, 'function');
});

// -- planBadge --

test('planBadge: permission_mode=plan returns PLAN', () => {
  assert.equal(planBadge({ permission_mode: 'plan' }), 'PLAN');
});

test('planBadge: state=plan returns PLAN', () => {
  // Covers Claude Code's "plan" state (set by hook on ExitPlanMode
  // PreToolUse) even when permission_mode is empty.
  assert.equal(planBadge({ state: 'plan' }), 'PLAN');
});

test('planBadge: state=plan + permission_mode=default still returns PLAN', () => {
  // After plan approval, permission_mode flips back but state may briefly
  // remain 'plan' until the next hook. The badge is a status signal, not
  // a mode echo — either signal should fire it.
  assert.equal(planBadge({ state: 'plan', permission_mode: 'default' }), 'PLAN');
});

test('planBadge: running with no plan signal returns empty', () => {
  assert.equal(planBadge({ state: 'running', permission_mode: 'bypassPermissions' }), '');
});

test('planBadge: empty agent returns empty', () => {
  assert.equal(planBadge({}), '');
});

// -- subagentBadge --

test('subagentBadge: zero count returns empty', () => {
  assert.equal(subagentBadge({ subagent_count: 0 }), '');
  assert.equal(subagentBadge({}), '');
});

test('subagentBadge: positive count returns glyph + count', () => {
  assert.equal(subagentBadge({ subagent_count: 1 }), '↳ 1');
  assert.equal(subagentBadge({ subagent_count: 3 }), '↳ 3');
});

test('subagentBadge: rejects non-numeric or negative', () => {
  // Defensive: a malformed state file shouldn't render garbage.
  assert.equal(subagentBadge({ subagent_count: -1 }), '');
  assert.equal(subagentBadge({ subagent_count: 'x' }), '');
});

// -- questionBadge --

test('questionBadge: empty when no pending_question', () => {
  assert.equal(questionBadge({}), '');
  assert.equal(questionBadge({ pending_question: null }), '');
});

test('questionBadge: returns "ASK" for any pending question', () => {
  // No counter — the chip is a presence signal, not a status panel; the
  // detail view owns the question count.
  assert.equal(questionBadge({ pending_question: { questions: [{ question: 'A?' }] } }), 'ASK');
  assert.equal(questionBadge({
    pending_question: { questions: [{ question: 'A?' }, { question: 'B?' }] },
  }), 'ASK');
});

test('questionBadge: empty when pending_question has no questions array', () => {
  assert.equal(questionBadge({ pending_question: {} }), '');
  assert.equal(questionBadge({ pending_question: { questions: [] } }), '');
});

// -- prTag (harmonized 1-3 char ALL CAPS) --

test('prTag: returns "PR" for pinned_state=pr (was "PR open", now harmonized)', () => {
  assert.equal(prTag({ pinned_state: 'pr' }), 'PR');
});

test('prTag: returns "" when no PR pin', () => {
  assert.equal(prTag({}), '');
  assert.equal(prTag({ pinned_state: '' }), '');
  assert.equal(prTag({ pinned_state: 'merged' }), '');
});

test('prTag: suppressed when a pending question is active', () => {
  // Pill-pileup anti-pattern: PR + ASK on the same row creates two competing
  // signals. Question is more blocking — it stays; PR drops to the dot only.
  const agent = {
    pinned_state: 'pr',
    pending_question: { questions: [{ question: 'Confirm?' }] },
  };
  assert.equal(prTag(agent), '');
});

// -- stateLabel (a11y expansion for status dots) --

test('stateLabel: maps each state group to a human label for aria', () => {
  assert.equal(stateLabel('permission'), 'Needs approval');
  assert.equal(stateLabel('plan'), 'Plan review');
  assert.equal(stateLabel('question'), 'Needs reply');
  assert.equal(stateLabel('error'), 'Error');
  assert.equal(stateLabel('running'), 'Running');
  assert.equal(stateLabel('idle_prompt'), 'Ready for review');
  assert.equal(stateLabel('done'), 'Done');
  assert.equal(stateLabel('pr'), 'PR open');
  assert.equal(stateLabel('merged'), 'Merged');
  assert.equal(stateLabel('unregistered'), 'Unregistered');
});

test('stateLabel: returns "" for unknown states', () => {
  assert.equal(stateLabel(''), '');
  assert.equal(stateLabel('garbage'), '');
});
